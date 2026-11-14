package koreaderplugin

import (
	"context"
	"time"

	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
)

// Statuses and Ratings go both ways (ADR 0016). The plugin sends a change
// with the value it last synced with this server, so the server can tell
// whether Shisho changed too. When both did, the later change wins by day,
// since KOReader dates a status only to the day, and Shisho wins a same-day
// tie. A losing change is dropped, and the plugin's next pull puts Shisho's
// value back on the device.

// cursorOverlap is how far before a pull's cursor the next pull looks for
// changes, so a change whose transaction began before the cursor and
// finished after it isn't missed. A change sent twice is written once.
const cursorOverlap = time.Minute

// koreaderStatus is how KOReader shows a Reading Status, or "" for To Read
// and none, which KOReader has nothing for.
func koreaderStatus(status *string) string {
	if status == nil {
		return ""
	}
	switch *status {
	case models.ReadingStatusRead:
		return KOReaderStatusFinished
	case models.ReadingStatusPaused, models.ReadingStatusDidNotFinish:
		return KOReaderStatusOnHold
	case models.ReadingStatusCurrentlyReading:
		return KOReaderStatusReading
	default:
		return ""
	}
}

// readingStatus is the Reading Status a KOReader status sets.
func readingStatus(status string) string {
	switch status {
	case KOReaderStatusFinished:
		return models.ReadingStatusRead
	case KOReaderStatusOnHold:
		return models.ReadingStatusPaused
	default:
		return models.ReadingStatusCurrentlyReading
	}
}

// wholeStars is a Rating in quarter-star units as KOReader shows it: whole
// stars, halves up, and at least one. 0 is unrated.
func wholeStars(units *int) int {
	if units == nil {
		return 0
	}
	return max(1, (*units+2)/4)
}

// changeDay is the day a status or Rating change happened, in loc: the day
// of its instant when the plugin saw it happen, and otherwise the day
// KOReader recorded, taken at noon. Either is corrected for the device's
// clock. A day-only change on the day the Device was created counts, even
// though its noon may come before. False is a change whose time can't be
// right.
func (c deviceClock) changeDay(event EventPayload, loc *time.Location) (string, bool) {
	if event.At > 0 {
		at, ok := c.instant(event.At)
		if !ok {
			return "", false
		}
		return at.In(loc).Format(time.DateOnly), true
	}
	day, err := time.ParseInLocation(time.DateOnly, event.Day, loc)
	if err != nil {
		return "", false
	}
	at := time.Unix(day.Add(12*time.Hour).Unix()+c.shift, 0)
	if at.Before(c.floor) {
		if at.In(loc).Format(time.DateOnly) != c.floor.In(loc).Format(time.DateOnly) {
			return "", false
		}
		at = c.floor
	}
	if at.After(c.arrival) {
		at = c.arrival
	}
	return at.In(loc).Format(time.DateOnly), true
}

// statesBefore returns, by Book, the status summaries of the Books that
// events change the status or Rating of, as Shisho holds them before any of
// the upload is applied.
func (h *handler) statesBefore(ctx context.Context, user *models.User, events []EventPayload,
	resolve func(string) (*models.DocumentHash, error)) (map[int]*reading.StatusSummary, error) {
	var bookIDs []int
	for _, event := range events {
		if event.Type != PluginEventStatus && event.Type != PluginEventRating {
			continue
		}
		file, err := resolve(event.Document)
		if err != nil {
			return nil, err
		}
		if file != nil && file.File != nil {
			bookIDs = append(bookIDs, file.File.BookID)
		}
	}
	return h.reading.RetrieveStatusSummaries(ctx, user.ID, bookIDs)
}

// change is a status or Rating event about a Book, dated by changeDay in
// the user's time zone, loc.
type change struct {
	file *models.DocumentHash
	day  string
	loc  *time.Location
}

// resolveChange finds the Book a status or Rating event is about and the day
// it happened. It returns nil for a book Shisho doesn't recognize, or a
// supplement, which isn't the Book, both acknowledged and ignored, and false
// for one refused for its time.
func (h *handler) resolveChange(ctx context.Context, user *models.User, clock deviceClock, event EventPayload,
	resolve func(string) (*models.DocumentHash, error)) (*change, bool, error) {
	file, err := resolve(event.Document)
	if err != nil || file == nil || file.File == nil || file.File.FileRole == models.FileRoleSupplement {
		return nil, true, err
	}
	loc, err := h.reading.Location(ctx, user.ID)
	if err != nil {
		return nil, false, err
	}
	day, ok := clock.changeDay(event, loc)
	if !ok {
		return nil, false, nil
	}
	return &change{file: file, day: day, loc: loc}, true, nil
}

// quarterStars is whole stars as a Rating in quarter-star units, nil for 0.
func quarterStars(stars int) *int {
	if stars <= 0 {
		return nil
	}
	units := stars * 4
	return &units
}

// applyChange takes in a status or Rating change from the device, judging
// whether Shisho changed too by before (statesBefore). It returns false for
// one refused for its time.
func (h *handler) applyChange(ctx context.Context, user *models.User, clock deviceClock, event EventPayload,
	resolve func(string) (*models.DocumentHash, error), before map[int]*reading.StatusSummary) (bool, error) {
	c, ok, err := h.resolveChange(ctx, user, clock, event, resolve)
	if c == nil || err != nil {
		return ok, err
	}
	file, day, loc := c.file, c.day, c.loc
	summaries, err := h.reading.RetrieveStatusSummaries(ctx, user.ID, []int{file.File.BookID})
	if err != nil {
		return false, err
	}
	summary := summaries[file.File.BookID]
	if summary == nil {
		return true, nil
	}
	prior := before[file.File.BookID]
	if prior == nil {
		prior = summary
	}
	// Shisho's own change wins a same-day tie with the device's.
	shishoLater := func(changed *time.Time) bool {
		return changed != nil && changed.In(loc).Format(time.DateOnly) >= day
	}

	if event.Type == PluginEventRating {
		shisho := wholeStars(summary.RatingUnits)
		if event.Rating == shisho {
			// Nothing to change, and a quarter-star Rating keeps its quarter.
			return true, nil
		}
		if wholeStars(prior.RatingUnits) != event.PreviousRating && shishoLater(prior.RatingAt) {
			return true, nil
		}
		return true, h.reading.SetRating(ctx, user.ID, file.File.BookID, quarterStars(event.Rating))
	}

	shisho := koreaderStatus(summary.Status)
	if event.Status == shisho {
		// Such as On hold for a Book Did Not Finish, which stays that. A
		// finish reaching the end made on its own is now one the reader
		// chose, so paging back doesn't undo it.
		if shisho == KOReaderStatusFinished {
			return true, h.reading.KeepFinished(ctx, user.ID, file.FileID)
		}
		return true, nil
	}
	// A book whose status the plugin never synced was reading as far as
	// KOReader knew, as KOReader takes a book with no status to be, so a read
	// its pages started in Shisho isn't a change of Shisho's own.
	previous := event.PreviousStatus
	if previous == "" {
		previous = KOReaderStatusReading
	}
	// To Read and no status are never written to the device, so they're
	// nothing the device could have overruled.
	if prev := koreaderStatus(prior.Status); prev != "" && prev != previous && shishoLater(prior.ReadThroughAt) {
		return true, nil
	}
	return true, h.reading.SetDeviceStatus(ctx, reading.DeviceStatus{
		UserID: user.ID,
		FileID: file.FileID,
		Status: readingStatus(event.Status),
		Day:    day,
	})
}

// applyHistory takes in a status or Rating from the device's history, which
// fills in only what Shisho has none of (ADR 0016): a status makes the
// Book's one Read-through when it has none, and a Rating sets one when it
// has none. It returns false for one refused for its time.
func (h *handler) applyHistory(ctx context.Context, user *models.User, clock deviceClock, event EventPayload,
	resolve func(string) (*models.DocumentHash, error)) (bool, error) {
	c, ok, err := h.resolveChange(ctx, user, clock, event, resolve)
	if c == nil || err != nil {
		return ok, err
	}
	if event.Type == PluginEventStatus {
		return true, h.reading.RecordHistoryStatus(ctx, reading.DeviceStatus{
			UserID: user.ID,
			FileID: c.file.FileID,
			Status: readingStatus(event.Status),
			Day:    c.day,
		})
	}
	units := quarterStars(event.Rating)
	if units == nil {
		return true, nil
	}
	return true, h.reading.FillRating(ctx, user.ID, c.file.File.BookID, *units)
}

// pullStates returns the status and Rating the device should hold for each
// book payload names, and each book changed after its Since, that this
// device reaches.
func (h *handler) pullStates(ctx context.Context, user *models.User, device *devices.Device, payload PullPayload) ([]PulledState, error) {
	states := []PulledState{}
	documents := []string{}
	seen := map[string]bool{}
	add := func(document string) {
		if !seen[document] {
			seen[document] = true
			documents = append(documents, document)
		}
	}
	for _, document := range payload.Statuses {
		add(document)
	}
	var since time.Time
	if payload.Since > 0 {
		since = time.Unix(payload.Since, 0).Add(-cursorOverlap)
		bookIDs, err := h.reading.ChangedSince(ctx, user.ID, since)
		if err != nil {
			return nil, err
		}
		hashes, err := h.hashes.DocumentHashesOfBooks(ctx, bookIDs)
		if err != nil {
			return nil, err
		}
		for _, hash := range hashes {
			add(hash)
		}
	}

	books := map[string]int{}
	var bookIDs []int
	for _, document := range documents {
		file, err := h.documents.Resolve(ctx, user, device, document)
		if err != nil {
			return nil, err
		}
		if file == nil || file.File == nil || file.File.FileRole == models.FileRoleSupplement {
			continue
		}
		books[document] = file.File.BookID
		bookIDs = append(bookIDs, file.File.BookID)
	}
	summaries, err := h.reading.RetrieveStatusSummaries(ctx, user.ID, bookIDs)
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		bookID, ok := books[document]
		if !ok {
			continue
		}
		summary := summaries[bookID]
		if summary == nil {
			continue
		}
		state := PulledState{Document: document, Status: koreaderStatus(summary.Status)}
		switch {
		case summary.RatingUnits != nil:
			stars := wholeStars(summary.RatingUnits)
			state.Rating = &stars
		// A Rating removed since the last pull is removed on the device too.
		// One that was never there leaves the device's alone.
		case !since.IsZero() && summary.RatingAt != nil && summary.RatingAt.After(since):
			none := 0
			state.Rating = &none
		}
		states = append(states, state)
	}
	return states, nil
}
