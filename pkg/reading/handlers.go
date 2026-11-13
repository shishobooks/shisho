package reading

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	readingService *Service
}

// requireBook resolves the :id Book the caller may reach, returning its id.
func (h *handler) requireBook(c echo.Context) (int, error) {
	id, err := httputil.ParamID(c, "id", "Book")
	if err != nil {
		return 0, err
	}
	libraryID, err := h.readingService.BookLibraryID(c.Request().Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errcodes.NotFound("Book")
	}
	if err != nil {
		return 0, err
	}
	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return 0, err
	}
	return id, nil
}

// requireOwnReadThrough resolves the caller's own :id Read-through, whether
// or not its Book still exists or is in a library they can reach: it's their
// record either way, which Reading History edits. Anyone else's is not found.
func (h *handler) requireOwnReadThrough(c echo.Context, user *models.User) (*models.ReadThrough, error) {
	id, err := httputil.ParamID(c, "id", "Read-through")
	if err != nil {
		return nil, err
	}
	rt, err := h.readingService.RetrieveReadThrough(c.Request().Context(), user.ID, id)
	if errors.Is(err, ErrReadThroughNotFound) {
		return nil, errcodes.NotFound("Read-through")
	}
	if err != nil {
		return nil, err
	}
	return rt, nil
}

// readThroughBook returns rt's Book when user can reach it, or false when
// it was deleted or is in a library they no longer have access to.
func (h *handler) readThroughBook(c echo.Context, user *models.User, rt *models.ReadThrough) (int, bool, error) {
	if rt.BookID == nil {
		return 0, false, nil
	}
	libraryID, err := h.readingService.BookLibraryID(c.Request().Context(), *rt.BookID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return *rt.BookID, user.HasLibraryAccess(libraryID), nil
}

// requireReadThrough resolves the caller's own :id Read-through of a Book
// in a library they can still reach. Anyone else's, or a detached one, is
// not found.
func (h *handler) requireReadThrough(c echo.Context, user *models.User) (*models.ReadThrough, error) {
	rt, err := h.requireOwnReadThrough(c, user)
	if err != nil {
		return nil, err
	}
	_, reachable, err := h.readThroughBook(c, user, rt)
	if err != nil {
		return nil, err
	}
	if !reachable {
		return nil, errcodes.NotFound("Read-through")
	}
	return rt, nil
}

// respondReadThrough answers a change to rt with rt and, when the user can
// still reach its Book, the Book's reading record.
func (h *handler) respondReadThrough(c echo.Context, user *models.User, rt *models.ReadThrough) error {
	bookID, reachable, err := h.readThroughBook(c, user, rt)
	if err != nil {
		return err
	}
	resp := ReadThroughChangeResponse{ReadThrough: rt}
	if reachable {
		reading, err := h.buildBookReadingResponse(c, user, bookID)
		if err != nil {
			return err
		}
		resp.Reading = &reading
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) respond(c echo.Context, user *models.User, bookID int) error {
	resp, err := h.buildBookReadingResponse(c, user, bookID)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) buildBookReadingResponse(c echo.Context, user *models.User, bookID int) (BookReadingResponse, error) {
	ctx := c.Request().Context()
	reading, err := h.readingService.RetrieveBookReading(ctx, user.ID, bookID)
	if err != nil {
		return BookReadingResponse{}, err
	}
	var place *int
	if reading.OnToRead {
		p, err := h.readingService.ToReadPlace(ctx, user.ID, bookID, user.GetAccessibleLibraryIDs())
		if err != nil {
			return BookReadingResponse{}, err
		}
		place = &p
	}
	counts, err := h.readingService.sessionCounts(ctx, reading.ReadThroughs)
	if err != nil {
		return BookReadingResponse{}, err
	}
	sessionCounts := make(map[int]int, len(reading.ReadThroughs))
	for _, rt := range reading.ReadThroughs {
		sessionCounts[rt.ID] = counts[rt.ID]
	}
	resp := BookReadingResponse{
		BookID:        bookID,
		SessionCounts: sessionCounts,
		Status:        reading.Status,
		OnToRead:      reading.OnToRead,
		ToReadPlace:   place,
		ReadThroughs:  reading.ReadThroughs,
		PageFlow:      reading.PageFlow,
		Positions:     reading.Positions,
		Progress:      reading.Progress,
		LastReadAt:    reading.LastReadAt,
	}
	resp.Rating = RatingStars(reading.RatingUnits)
	return resp, nil
}

type refusal struct {
	err     error
	message string
}

// refusedTransitions are the transitions the Book's Read-throughs don't
// allow, rendered as 422 invalid_state.
var refusedTransitions = []refusal{
	{ErrAlreadyUnderway, "This book is already being read."},
	{ErrNothingUnderway, "This book isn't being read."},
	{ErrAlreadyPaused, "This book is already paused."},
	{ErrNotPaused, "This book isn't paused."},
	{ErrNotAutoFinished, "This finish can't be undone."},
	{ErrReadThroughEnded, "This Read-through has already ended."},
}

// rejectedDays are the date edits rendered as 422 validation_error.
var rejectedDays = []refusal{
	{ErrEndDayWhileUnderway, "A Read-through still underway can't have an end date."},
	{ErrEndBeforeBegin, "The end date can't be before the start date."},
}

func refusedMessage(refusals []refusal, err error) (string, bool) {
	for _, r := range refusals {
		if errors.Is(err, r.err) {
			return r.message, true
		}
	}
	return "", false
}

// transitionError maps a refused transition to its 422.
func transitionError(err error) error {
	if message, ok := refusedMessage(refusedTransitions, err); ok {
		return errcodes.InvalidState(message)
	}
	return err
}

// furthestAheadZone is the earliest time zone to reach each new day. The
// server doesn't know the user's zone, so a day is in the future only once
// it's after today here.
var furthestAheadZone = time.FixedZone("UTC+14", 14*60*60)

// checkDays rejects a day that isn't a real calendar day, such as
// 2026-02-30, which the binder's date format check lets through, and a day
// that hasn't begun anywhere yet: reading can't start or end in the future.
func checkDays(days ...*string) error {
	latest := time.Now().In(furthestAheadZone).Format(time.DateOnly)
	for _, day := range days {
		if day == nil {
			continue
		}
		if _, err := time.Parse(time.DateOnly, *day); err != nil {
			return errcodes.ValidationError(fmt.Sprintf("%q is not a valid date.", *day))
		}
		// Days are YYYY-MM-DD, so they compare as strings.
		if *day > latest {
			return errcodes.ValidationError(fmt.Sprintf("%q is in the future.", *day))
		}
	}
	return nil
}

func (h *handler) retrieve(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	bookID, err := h.requireBook(c)
	if err != nil {
		return err
	}
	return h.respond(c, user, bookID)
}

// dated runs a transition that needs the user's today.
func (h *handler) dated(c echo.Context, run func(userID, bookID int, today string) error) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	bookID, err := h.requireBook(c)
	if err != nil {
		return err
	}
	params := DayPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	if err := checkDays(&params.Today); err != nil {
		return err
	}
	if err := run(user.ID, bookID, params.Today); err != nil {
		return transitionError(err)
	}
	return h.respond(c, user, bookID)
}

func (h *handler) start(c echo.Context) error {
	return h.dated(c, func(userID, bookID int, today string) error {
		return h.readingService.Start(c.Request().Context(), userID, bookID, today)
	})
}

func (h *handler) finish(c echo.Context) error {
	return h.dated(c, func(userID, bookID int, today string) error {
		return h.readingService.End(c.Request().Context(), userID, bookID, models.ReadThroughOutcomeFinished, today)
	})
}

func (h *handler) abandon(c echo.Context) error {
	return h.dated(c, func(userID, bookID int, today string) error {
		return h.readingService.End(c.Request().Context(), userID, bookID, models.ReadThroughOutcomeAbandoned, today)
	})
}

func (h *handler) setPaused(c echo.Context, paused bool) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	bookID, err := h.requireBook(c)
	if err != nil {
		return err
	}
	if err := h.readingService.SetPaused(c.Request().Context(), user.ID, bookID, paused); err != nil {
		return transitionError(err)
	}
	return h.respond(c, user, bookID)
}

func (h *handler) pause(c echo.Context) error {
	return h.setPaused(c, true)
}

func (h *handler) resume(c echo.Context) error {
	return h.setPaused(c, false)
}

func (h *handler) setRating(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	bookID, err := h.requireBook(c)
	if err != nil {
		return err
	}
	params := SetRatingPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	var units *int
	if params.Rating != nil {
		u, ok := RatingUnits(*params.Rating)
		if !ok {
			return errcodes.ValidationError("A rating is in quarter stars from 0.25 to 5.")
		}
		units = &u
	}
	if err := h.readingService.SetRating(c.Request().Context(), user.ID, bookID, units); err != nil {
		return err
	}
	return h.respond(c, user, bookID)
}

func (h *handler) setPageFlow(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	bookID, err := h.requireBook(c)
	if err != nil {
		return err
	}
	params := SetPageFlowPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	if err := h.readingService.SetPageFlow(c.Request().Context(), user.ID, bookID, params.PageFlow); err != nil {
		return err
	}
	return h.respond(c, user, bookID)
}

func (h *handler) updateReadThrough(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	rt, err := h.requireOwnReadThrough(c, user)
	if err != nil {
		return err
	}
	params := UpdateReadThroughPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	if err := checkDays(params.BeginDay, params.EndDay); err != nil {
		return err
	}
	err = h.readingService.UpdateReadThroughDays(c.Request().Context(), rt, params.BeginDay, params.EndDay)
	if message, ok := refusedMessage(rejectedDays, err); ok {
		return errcodes.ValidationError(message)
	}
	if err != nil {
		return err
	}
	return h.respondReadThrough(c, user, rt)
}

func (h *handler) deleteReadThrough(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	rt, err := h.requireOwnReadThrough(c, user)
	if err != nil {
		return err
	}
	if err := h.readingService.DeleteReadThrough(c.Request().Context(), rt); err != nil {
		return err
	}
	return h.respondReadThrough(c, user, rt)
}

// listReadThroughSessions lists the Reading Sessions of the caller's own :id
// Read-through, whether or not its Book still exists or is in reach.
func (h *handler) listReadThroughSessions(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	rt, err := h.requireOwnReadThrough(c, user)
	if err != nil {
		return err
	}
	resp, err := h.readingService.ReadThroughSessions(c.Request().Context(), user.ID, rt.ID)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

// endReadThrough ends the caller's own :id Read-through underway with
// outcome, for Reading History, where its Book may be gone.
func (h *handler) endReadThrough(c echo.Context, outcome string) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	rt, err := h.requireOwnReadThrough(c, user)
	if err != nil {
		return err
	}
	params := DayPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	if err := checkDays(&params.Today); err != nil {
		return err
	}
	ctx := c.Request().Context()
	if err := h.readingService.EndReadThrough(ctx, rt, outcome, params.Today); err != nil {
		return transitionError(err)
	}
	rt, err = h.readingService.RetrieveReadThrough(ctx, user.ID, rt.ID)
	if err != nil {
		return err
	}
	return h.respondReadThrough(c, user, rt)
}

func (h *handler) finishReadThrough(c echo.Context) error {
	return h.endReadThrough(c, models.ReadThroughOutcomeFinished)
}

func (h *handler) abandonReadThrough(c echo.Context) error {
	return h.endReadThrough(c, models.ReadThroughOutcomeAbandoned)
}

// listHistory pages through the caller's Reading History. It takes no user,
// so it only ever lists the caller's own.
func (h *handler) listHistory(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	params := ListReadingHistoryQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	ctx := c.Request().Context()
	readThroughs, total, err := h.readingService.ListHistory(ctx, user.ID, HistoryOptions{
		Outcome: params.Outcome,
		Removed: params.Removed,
		Limit:   params.Limit,
		Offset:  params.Offset,
	})
	if err != nil {
		return err
	}
	items, err := h.readingService.BuildHistoryEntries(ctx, user, readThroughs)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, ListReadingHistoryResponse{Items: items, Total: total}))
}

// stats returns the caller's Reading Stats for one year, or all time. It
// takes no user, so it only ever covers the caller's own reading.
func (h *handler) stats(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	params := ReadingStatsQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	resp, err := h.readingService.Stats(c.Request().Context(), user, params.Year)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

// listStatsFinished pages through the caller's finished Read-throughs for
// Reading Stats. Like stats, it takes no user.
func (h *handler) listStatsFinished(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	params := ListReadingStatsFinishedQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	if params.Rating != nil {
		if _, ok := RatingUnits(*params.Rating); !ok {
			return errcodes.ValidationError("A rating is in quarter stars from 0.25 to 5.")
		}
	}
	resp, err := h.readingService.StatsFinished(c.Request().Context(), user, StatsFinishedOptions(params))
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

// statsHeatmap returns the caller's reading for a year, or the last 12
// months, a day at a time. Like stats, it takes no user.
func (h *handler) statsHeatmap(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	params := ReadingStatsHeatmapQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	resp, err := h.readingService.StatsHeatmap(c.Request().Context(), user, params.Year)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

// statsCalendar returns one month of the caller's Reading Stats calendar.
// Like stats, it takes no user.
func (h *handler) statsCalendar(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	params := ReadingStatsCalendarQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	resp, err := h.readingService.StatsCalendar(c.Request().Context(), user, params.Year, params.Month)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

// statsDay returns one day of the caller's Reading Sessions for Reading
// Stats. Like stats, it takes no user.
func (h *handler) statsDay(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	params := ReadingStatsDayQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	resp, err := h.readingService.StatsDay(c.Request().Context(), user, params.Day)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

// deleteSession deletes the caller's own :id Reading Session for good, from
// a day on Reading Stats. Anyone else's is not found.
func (h *handler) deleteSession(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	id, err := httputil.ParamID(c, "id", "Reading Session")
	if err != nil {
		return err
	}
	deleted, err := h.readingService.DeleteSession(c.Request().Context(), user.ID, id)
	if err != nil {
		return err
	}
	if !deleted {
		return errcodes.NotFound("Reading Session")
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}

func (h *handler) addToRead(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	bookID, err := h.requireBook(c)
	if err != nil {
		return err
	}
	if err := h.readingService.AddToRead(c.Request().Context(), user.ID, bookID); err != nil {
		return transitionError(err)
	}
	return h.respond(c, user, bookID)
}

func (h *handler) removeFromToRead(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	bookID, err := h.requireBook(c)
	if err != nil {
		return err
	}
	if err := h.readingService.RemoveFromToRead(c.Request().Context(), user.ID, bookID); err != nil {
		return err
	}
	return h.respond(c, user, bookID)
}

// reorderToRead needs no library check: it only moves the caller's own
// positions, and ids not on their To Read are ignored.
func (h *handler) reorderToRead(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	params := ReorderToReadPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	if err := h.readingService.ReorderToRead(c.Request().Context(), user.ID, params.BookIDs); err != nil {
		return err
	}
	return errors.WithStack(c.NoContent(http.StatusNoContent))
}

// maxSessionLength and sessionClockSlack bound the Reading Sessions a web
// reader reports.
const (
	maxSessionLength  = 24 * time.Hour
	sessionClockSlack = 5 * time.Minute
)

// requireFile resolves the :id File the caller may reach, returning its id.
func (h *handler) requireFile(c echo.Context) (int, error) {
	id, err := httputil.ParamID(c, "id", "File")
	if err != nil {
		return 0, err
	}
	libraryID, err := h.readingService.FileLibraryID(c.Request().Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errcodes.NotFound("File")
	}
	if err != nil {
		return 0, err
	}
	if err := auth.RequireLibraryAccessFor(c, libraryID); err != nil {
		return 0, err
	}
	return id, nil
}

func (h *handler) retrievePosition(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	fileID, err := h.requireFile(c)
	if err != nil {
		return err
	}
	position, err := h.readingService.RetrieveWebReaderPosition(c.Request().Context(), user.ID, fileID)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, FilePositionResponse{Position: position}))
}

func (h *handler) updatePosition(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	fileID, err := h.requireFile(c)
	if err != nil {
		return err
	}
	params := UpdatePositionPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}
	forms := 0
	for _, given := range []bool{params.CFI != nil, params.Page != nil, params.Seconds != nil} {
		if given {
			forms++
		}
	}
	if forms == 0 && params.Session == nil {
		return errcodes.ValidationError("A position is required.")
	}
	if forms > 1 {
		return errcodes.ValidationError("A position has one form.")
	}
	if forms == 1 && params.Percentage == nil {
		return errcodes.ValidationError("A position needs its percentage.")
	}
	update := PositionUpdate{
		UserID:     user.ID,
		FileID:     fileID,
		Source:     models.ReadingSourceWebReader,
		Locator:    models.ReadingLocator{CFI: params.CFI, Page: params.Page, Seconds: params.Seconds},
		AtEnd:      params.AtEnd,
		Opened:     params.Opened,
		OccurredAt: params.OccurredAt,
	}
	if params.Percentage != nil {
		update.Percentage = *params.Percentage
	}
	if params.ListenedSeconds != nil {
		update.ListenedSeconds = *params.ListenedSeconds
	}
	// It backdates a Read-through and claims what was recorded since, so a
	// value out of range is left out. A sitting older than a day has nothing
	// left to decide, and a device's fast clock shouldn't cost the listener the
	// place this update saves.
	if since := params.ListeningSince; since != nil {
		now := time.Now()
		if !since.Before(now.Add(-maxSessionLength)) && !since.After(now.Add(sessionClockSlack)) {
			update.ListeningSince = since
		}
	}
	if s := params.Session; s != nil {
		if s.EndedAt.Before(s.StartedAt) {
			return errcodes.ValidationError("A reading session can't end before it starts.")
		}
		// A session ends after a few idle minutes, so one longer than a day,
		// or ending later than a clock running a little fast allows, is
		// wrong and would skew reading time.
		if s.EndedAt.Sub(s.StartedAt) > maxSessionLength || s.EndedAt.After(time.Now().Add(sessionClockSlack)) {
			return errcodes.ValidationError("A reading session must be in the past and last at most a day.")
		}
		update.Session = &SessionInput{
			StartedAt:       s.StartedAt,
			EndedAt:         s.EndedAt,
			StartPercentage: *s.StartPercentage,
			EndPercentage:   *s.EndPercentage,
			Measured:        true,
		}
	}
	// A session with no position is a reader closed where it opened: it
	// records only the time.
	if forms == 0 {
		bookID, err := h.readingService.RecordSession(c.Request().Context(), user.ID, fileID, update.Source, *update.Session)
		if errors.Is(err, ErrFileNotFound) {
			return errcodes.NotFound("File")
		}
		if err != nil {
			return err
		}
		return errors.WithStack(c.JSON(http.StatusOK, UpdatePositionResponse{BookID: bookID}))
	}
	result, err := h.readingService.UpdatePosition(c.Request().Context(), update)
	if errors.Is(err, ErrFileNotFound) {
		return errcodes.NotFound("File")
	}
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, result))
}

// undoFinish undoes a finish a position update made, for Undo on the
// reader's finish toast.
func (h *handler) undoFinish(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	rt, err := h.requireReadThrough(c, user)
	if err != nil {
		return err
	}
	if err := h.readingService.UndoAutoFinish(c.Request().Context(), rt); err != nil {
		return transitionError(err)
	}
	return h.respond(c, user, *rt.BookID)
}
