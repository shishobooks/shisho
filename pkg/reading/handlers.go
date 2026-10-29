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

// requireReadThrough resolves the caller's own :id Read-through in a
// library they can still reach. Anyone else's is not found.
func (h *handler) requireReadThrough(c echo.Context, user *models.User) (*models.ReadThrough, error) {
	id, err := httputil.ParamID(c, "id", "Read-through")
	if err != nil {
		return nil, err
	}
	ctx := c.Request().Context()
	rt, err := h.readingService.RetrieveReadThrough(ctx, user.ID, id)
	if errors.Is(err, ErrReadThroughNotFound) {
		return nil, errcodes.NotFound("Read-through")
	}
	if err != nil {
		return nil, err
	}
	libraryID, err := h.readingService.BookLibraryID(ctx, rt.BookID)
	if err != nil {
		return nil, err
	}
	if !user.HasLibraryAccess(libraryID) {
		return nil, errcodes.NotFound("Read-through")
	}
	return rt, nil
}

func (h *handler) respond(c echo.Context, userID, bookID int) error {
	resp, err := h.buildBookReadingResponse(c, userID, bookID)
	if err != nil {
		return err
	}
	return errors.WithStack(c.JSON(http.StatusOK, resp))
}

func (h *handler) buildBookReadingResponse(c echo.Context, userID, bookID int) (BookReadingResponse, error) {
	reading, err := h.readingService.RetrieveBookReading(c.Request().Context(), userID, bookID)
	if err != nil {
		return BookReadingResponse{}, err
	}
	resp := BookReadingResponse{
		BookID:       bookID,
		Status:       reading.Status,
		OnToRead:     reading.OnToRead,
		ReadThroughs: reading.ReadThroughs,
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

// checkDays rejects a day that isn't a real calendar day, such as
// 2026-02-30, which the binder's date format check lets through.
func checkDays(days ...*string) error {
	for _, day := range days {
		if day == nil {
			continue
		}
		if _, err := time.Parse(time.DateOnly, *day); err != nil {
			return errcodes.ValidationError(fmt.Sprintf("%q is not a valid date.", *day))
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
	return h.respond(c, user.ID, bookID)
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
	return h.respond(c, user.ID, bookID)
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
	return h.respond(c, user.ID, bookID)
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
	return h.respond(c, user.ID, bookID)
}

func (h *handler) updateReadThrough(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	rt, err := h.requireReadThrough(c, user)
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
	return h.respond(c, user.ID, rt.BookID)
}

func (h *handler) deleteReadThrough(c echo.Context) error {
	user, err := auth.RequireUser(c)
	if err != nil {
		return err
	}
	rt, err := h.requireReadThrough(c, user)
	if err != nil {
		return err
	}
	if err := h.readingService.DeleteReadThrough(c.Request().Context(), rt); err != nil {
		return err
	}
	return h.respond(c, user.ID, rt.BookID)
}
