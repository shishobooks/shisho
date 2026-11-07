package kobo

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/reflowable"
	"github.com/uptrace/bun"
)

// readingStates keeps a Kobo's reading state and Shisho's in step, for the
// books Shisho synced to it: what the Kobo reports goes through the
// position update service, and what other readers change goes back to the
// Kobo with the next sync.
type readingStates struct {
	db      *bun.DB
	reading *reading.Service
}

// stateInputs is what one entry's reading state is built from.
type stateInputs struct {
	entry    *SyncEntry
	file     *models.File
	position *models.ReadingPosition
	book     *bookReadingRow
}

// bookReadingRow is a user's reading of a Book, as far as a Kobo can show it.
type bookReadingRow struct {
	BookID int     `bun:"book_id"`
	Status *string `bun:"status"`
	// ReadThroughAt and UserBookAt are the latest changes to the Book's
	// Read-throughs and its To Read place, which date the status.
	ReadThroughAt *time.Time `bun:"read_through_at"`
	UserBookAt    *time.Time `bun:"user_book_at"`
	ReadThroughs  int        `bun:"read_throughs"`
	// LastBeginDay is when the latest Read-through began, when it says.
	LastBeginDay *string `bun:"last_begin_day"`
}

// tracked reports whether the user has read or tracked the entry's book at
// all, so there's a state worth sending with a book new to the Kobo.
func (in *stateInputs) tracked() bool {
	return in.position != nil || in.book != nil && in.book.Status != nil
}

// version is what the Kobo would have to have heard for its state to be
// current: the File the entry holds, the status and when it changed, and the
// position and when it was stored. It changes whenever the state Shisho
// would send does, and building the state, which can mean translating the
// position, waits until it has. A book nobody read or tracked has nothing to
// send, whichever File the entry holds.
func (in *stateInputs) version() string {
	if !in.tracked() {
		return "untracked"
	}
	status, statusAt := "", int64(0)
	if in.book != nil {
		if in.book.Status != nil {
			status = *in.book.Status
		}
		if at := in.statusAt(); at != nil {
			statusAt = at.UnixNano()
		}
	}
	position := "none"
	if in.position != nil {
		position = fmt.Sprintf("%s@%d", in.position.Source, in.position.UpdatedAt.UnixNano())
	}
	return fmt.Sprintf("%d|%s|%d|%s", in.file.ID, status, statusAt, position)
}

// statusAt is when the status last changed, as far as Shisho recorded.
func (in *stateInputs) statusAt() *time.Time {
	if in.book == nil {
		return nil
	}
	latest := in.book.ReadThroughAt
	if in.book.UserBookAt != nil && (latest == nil || in.book.UserBookAt.After(*latest)) {
		latest = in.book.UserBookAt
	}
	return latest
}

// load returns the inputs of each of entries that holds a File, by entry id.
func (r *readingStates) load(ctx context.Context, userID int, entries []*SyncEntry) (map[string]*stateInputs, error) {
	inputs := make(map[string]*stateInputs, len(entries))
	var fileIDs []int
	for _, e := range entries {
		if e.FileID != nil {
			fileIDs = append(fileIDs, *e.FileID)
		}
	}
	if len(fileIDs) == 0 {
		return inputs, nil
	}

	var files []*models.File
	err := r.db.NewSelect().
		Model(&files).
		Column("f.id", "f.book_id", "f.file_type", "f.filepath", "f.page_count").
		Where("f.id IN (?)", bun.List(fileIDs)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	filesByID := make(map[int]*models.File, len(files))
	var bookIDs []int
	for _, f := range files {
		filesByID[f.ID] = f
		bookIDs = append(bookIDs, f.BookID)
	}

	var positions []*models.ReadingPosition
	err = r.db.NewSelect().
		Model(&positions).
		Where("rp.user_id = ? AND rp.file_id IN (?)", userID, bun.List(fileIDs)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	positionsByFile := make(map[int]*models.ReadingPosition, len(positions))
	for _, p := range positions {
		positionsByFile[p.FileID] = p
	}

	var rows []*bookReadingRow
	if len(bookIDs) > 0 {
		err = r.db.NewSelect().
			TableExpr("books AS b").
			ColumnExpr("b.id AS book_id").
			ColumnExpr(reading.StatusExpr(userID, "b.id")+" AS status").
			ColumnExpr("(SELECT MAX(krt.updated_at) FROM read_throughs krt WHERE krt.user_id = ? AND krt.book_id = b.id) AS read_through_at", userID).
			ColumnExpr("(SELECT kub.updated_at FROM user_books kub WHERE kub.user_id = ? AND kub.book_id = b.id) AS user_book_at", userID).
			ColumnExpr("(SELECT COUNT(*) FROM read_throughs krt WHERE krt.user_id = ? AND krt.book_id = b.id) AS read_throughs", userID).
			ColumnExpr("(SELECT krt.begin_day FROM read_throughs krt WHERE krt.user_id = ? AND krt.book_id = b.id ORDER BY krt.id DESC LIMIT 1) AS last_begin_day", userID).
			Where("b.id IN (?)", bun.List(bookIDs)).
			Scan(ctx, &rows)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	booksByID := make(map[int]*bookReadingRow, len(rows))
	for _, row := range rows {
		booksByID[row.BookID] = row
	}

	for _, e := range entries {
		if e.FileID == nil {
			continue
		}
		file := filesByID[*e.FileID]
		if file == nil {
			continue
		}
		inputs[e.EntryID] = &stateInputs{
			entry:    e,
			file:     file,
			position: positionsByFile[file.ID],
			book:     booksByID[file.BookID],
		}
	}
	return inputs, nil
}

// build makes the reading state the Kobo gets for in. Each part is dated by
// when it changed in Shisho, so a Kobo holding something newer of its own,
// such as reading done offline, keeps it.
func (r *readingStates) build(ctx context.Context, in *stateInputs) *ReadingState {
	created := in.entry.CreatedAt
	state := &ReadingState{EntitlementID: in.entry.EntryID, Created: NewTimestamp(created)}
	latest := created
	later := func(at time.Time) {
		if at.After(latest) {
			latest = at
		}
	}

	statusAt := created
	if at := in.statusAt(); at != nil {
		statusAt = *at
	} else if in.position != nil {
		statusAt = in.position.OccurredAt
	}
	later(statusAt)
	info := &StatusInfo{LastModified: NewTimestamp(statusAt), Status: koboStatusReadyToRead}
	if in.book != nil {
		info.Status = koboStatus(in.book.Status)
		times := in.book.ReadThroughs
		info.TimesStartedReading = &times
		if in.book.LastBeginDay != nil {
			if day, err := time.Parse(time.DateOnly, *in.book.LastBeginDay); err == nil {
				info.LastTimeStartedReading = NewTimestamp(day)
			}
		}
	}
	state.StatusInfo = info

	if in.entry.ReadingMinutes != nil && in.entry.ReadingMinutesAt != nil {
		spent := *in.entry.ReadingMinutes
		state.Statistics = &Statistics{LastModified: NewTimestamp(*in.entry.ReadingMinutesAt), SpentReadingMinutes: &spent}
		later(*in.entry.ReadingMinutesAt)
	}

	if in.position != nil {
		state.CurrentBookmark = r.bookmark(ctx, in)
		later(in.position.OccurredAt)
	}

	state.LastModified = NewTimestamp(latest)
	state.PriorityTimestamp = NewTimestamp(latest)
	return state
}

// koboStatus is the Kobo status closest to a Reading Status. A Kobo has no
// paused or abandoned book, so both stay Reading, which keeps the bookmark.
func koboStatus(status *string) string {
	if status == nil {
		return koboStatusReadyToRead
	}
	switch *status {
	case models.ReadingStatusCurrentlyReading, models.ReadingStatusPaused, models.ReadingStatusDidNotFinish:
		return koboStatusReading
	case models.ReadingStatusRead:
		return koboStatusFinished
	default:
		return koboStatusReadyToRead
	}
}

// bookmark is in's position as a Kobo bookmark: a span in the KePub the Kobo
// holds, exactly as stored when this or another Kobo wrote it in that copy,
// and otherwise the nearest span to where another reader is. A position no
// span can stand for, as in an EPUB delivered as itself, goes with its
// percentage only.
func (r *readingStates) bookmark(ctx context.Context, in *stateInputs) *CurrentBookmark {
	position := in.position
	percent := position.Percentage * 100
	bookmark := &CurrentBookmark{LastModified: NewTimestamp(position.OccurredAt), ProgressPercent: &percent}
	setSpan := func(span reflowable.KoboSpan, chapter *float64) {
		bookmark.Location = &BookmarkLocation{Value: span.ID, Type: koboSpanLocation, Source: span.Source}
		if chapter != nil {
			chapterPercent := *chapter * 100
			bookmark.ContentSourceProgressPercent = &chapterPercent
		}
	}

	if position.KoboSpan != nil && reflowable.WrittenIn(position, in.file.FileType) == in.entry.DeliveredFileType {
		if span, ok := reflowable.ParseKoboSpan(*position.KoboSpan); ok {
			setSpan(span, position.ChapterPercentage)
			return bookmark
		}
	}
	if in.entry.DeliveredFileType != models.FileTypeKePub {
		return bookmark
	}
	translated, err := reflowable.TranslateForKobo(ctx, in.file.Filepath, in.file.FileType, position, in.entry.DeliveredFileType)
	switch {
	case err == nil:
		setSpan(translated.Span, &translated.ChapterFraction)
	case errors.Is(err, reflowable.ErrNotTranslatable), errors.Is(err, reflowable.ErrNoKoboSpans):
	default:
		logger.FromContext(ctx).Warn("reading position not translated for kobo", logger.Data{
			"file_id": in.file.ID, "entry_id": in.entry.EntryID, "error": err.Error(),
		})
	}
	return bookmark
}

// retrieve returns the reading state of entry for userID.
func (r *readingStates) retrieve(ctx context.Context, userID int, entry *SyncEntry) (*ReadingState, error) {
	inputs, err := r.load(ctx, userID, []*SyncEntry{entry})
	if err != nil {
		return nil, err
	}
	in := inputs[entry.EntryID]
	if in == nil {
		return &ReadingState{EntitlementID: entry.EntryID, StatusInfo: &StatusInfo{Status: koboStatusReadyToRead}}, nil
	}
	return r.build(ctx, in), nil
}

// forNewEntries returns the reading state each of entryIDs, about to reach
// the Kobo, goes with: one for every book the user has read or tracked, by
// entry id. Each entry is then current, so a later sync sends its state
// again only once it changes.
func (r *readingStates) forNewEntries(ctx context.Context, userID int, deviceID string, entryIDs []string) (map[string]*ReadingState, error) {
	states := map[string]*ReadingState{}
	entries, err := r.entries(ctx, deviceID, entryIDs)
	if err != nil {
		return nil, err
	}
	inputs, err := r.load(ctx, userID, entries)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		in := inputs[e.EntryID]
		if in == nil {
			continue
		}
		if in.tracked() {
			states[e.EntryID] = r.build(ctx, in)
		}
		if err := r.remember(ctx, deviceID, e.EntryID, in.version()); err != nil {
			return nil, err
		}
	}
	return states, nil
}

// changed returns the reading states of entryIDs that changed since the Kobo
// last heard, at most limit of them, in entry id order, and whether more
// remain. Each one returned is then current. A book never read or tracked,
// that the Kobo hasn't heard of either, has nothing to send.
func (r *readingStates) changed(ctx context.Context, userID int, deviceID string, entryIDs []string, limit int) ([]*ReadingState, bool, error) {
	entries, err := r.entries(ctx, deviceID, entryIDs)
	if err != nil {
		return nil, false, err
	}
	inputs, err := r.load(ctx, userID, entries)
	if err != nil {
		return nil, false, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].EntryID < entries[j].EntryID })
	var states []*ReadingState
	for _, e := range entries {
		in := inputs[e.EntryID]
		if in == nil {
			continue
		}
		version := in.version()
		if e.ReadingStateVersion != nil && *e.ReadingStateVersion == version {
			continue
		}
		if e.ReadingStateVersion == nil && !in.tracked() {
			if err := r.remember(ctx, deviceID, e.EntryID, version); err != nil {
				return nil, false, err
			}
			continue
		}
		if len(states) == limit {
			return states, true, nil
		}
		states = append(states, r.build(ctx, in))
		if err := r.remember(ctx, deviceID, e.EntryID, version); err != nil {
			return nil, false, err
		}
	}
	return states, false, nil
}

// entries returns deviceID's entries among entryIDs.
func (r *readingStates) entries(ctx context.Context, deviceID string, entryIDs []string) ([]*SyncEntry, error) {
	var entries []*SyncEntry
	if len(entryIDs) == 0 {
		return entries, nil
	}
	err := r.db.NewSelect().
		Model(&entries).
		Where("kse.device_id = ?", deviceID).
		Where("kse.entry_id IN (?)", bun.List(entryIDs)).
		Scan(ctx)
	return entries, errors.WithStack(err)
}

// remember records that the Kobo has heard version of entryID's state. An
// empty version means it's heard nothing current, so the next sync sends it.
func (r *readingStates) remember(ctx context.Context, deviceID, entryID, version string) error {
	var stored *string
	if version != "" {
		stored = &version
	}
	_, err := r.db.NewUpdate().
		Model((*SyncEntry)(nil)).
		Set("reading_state_version = ?", stored).
		Where("device_id = ? AND entry_id = ?", deviceID, entryID).
		Exec(ctx)
	return errors.WithStack(err)
}

// apply takes in a Kobo's reading state for entry. Kobo's own times date
// everything, so reading done offline lands on the day it happened:
//
//   - Reading starts or resumes the Read-through, and Finished ends it, for
//     good. ReadyToRead says nothing about reading, so it changes nothing.
//   - The bookmark, a span with how far through the chapter and the book it
//     is, is stored as the position.
//   - The increase in the Kobo's reading minutes since its last report is an
//     estimated Reading Session ending when the Kobo reported it.
func (r *readingStates) apply(ctx context.Context, user *models.User, device *devices.Device, entry *SyncEntry, state *ReadingState) error {
	now := time.Now()
	file := &models.File{}
	err := r.db.NewSelect().
		Model(file).
		Column("f.id", "f.file_type", "f.page_count").
		Where("f.id = ?", *entry.FileID).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	stored, err := r.reading.RetrievePosition(ctx, user.ID, file.ID)
	if err != nil {
		return err
	}

	update := reading.PositionUpdate{
		UserID:     user.ID,
		FileID:     file.ID,
		Source:     models.ReadingSourceKobo,
		DeviceID:   &device.ID,
		OccurredAt: state.occurredAt(),
	}
	if state.StatusInfo != nil {
		switch state.StatusInfo.Status {
		case koboStatusReading:
			update.ReportedStatus = reading.ReportedReading
		case koboStatusFinished:
			update.ReportedStatus = reading.ReportedFinished
		}
	}
	if state.CurrentBookmark != nil {
		bookmarkLocator(&update, state.CurrentBookmark, file, entry.DeliveredFileType)
	}

	minutes, minutesAt := entry.ReadingMinutes, entry.ReadingMinutesAt
	if stats := state.Statistics; stats != nil && stats.SpentReadingMinutes != nil {
		reported := *stats.SpentReadingMinutes
		at := now
		if when := stats.LastModified.at(); when != nil && when.Before(now) {
			at = *when
		} else if update.OccurredAt != nil {
			at = *update.OccurredAt
		}
		if minutes != nil && reported > *minutes {
			endPercentage := update.Percentage
			startPercentage := endPercentage
			if stored != nil {
				startPercentage = stored.Percentage
			}
			if update.Locator.IsZero() {
				endPercentage = startPercentage
			}
			started := at.Add(-time.Duration(reported-*minutes) * time.Minute)
			update.Session = &reading.SessionInput{
				StartedAt:       started,
				EndedAt:         at,
				StartPercentage: startPercentage,
				EndPercentage:   endPercentage,
			}
			// A read this sitting starts began when the sitting did.
			update.ListeningSince = &started
		}
		minutes, minutesAt = &reported, &at
	}

	accepted := true
	switch {
	case !update.Locator.IsZero() || update.ReportedStatus != reading.ReportedNothing:
		result, err := r.reading.UpdatePosition(ctx, update)
		if err != nil {
			return err
		}
		accepted = result.Accepted
	case update.Session != nil:
		if _, err := r.reading.RecordEstimatedSession(ctx, user.ID, file.ID, update.Source, *update.Session); err != nil {
			return err
		}
	}

	// The Kobo now has what Shisho has, unless its update was older than
	// what Shisho has, which the next sync then sends it.
	version := ""
	if accepted {
		entry.ReadingMinutes, entry.ReadingMinutesAt = minutes, minutesAt
		inputs, err := r.load(ctx, user.ID, []*SyncEntry{entry})
		if err != nil {
			return err
		}
		if in := inputs[entry.EntryID]; in != nil {
			version = in.version()
		}
	}
	var storedVersion *string
	if version != "" {
		storedVersion = &version
	}
	_, err = r.db.NewUpdate().
		Model((*SyncEntry)(nil)).
		Set("reading_minutes = ?", minutes).
		Set("reading_minutes_at = ?", minutesAt).
		Set("reading_state_version = ?", storedVersion).
		Where("device_id = ? AND entry_id = ?", entry.DeviceID, entry.EntryID).
		Exec(ctx)
	return errors.WithStack(err)
}

// occurredAt is when the Kobo was in this state: its LastModified, or the
// latest of its parts', or nil when it gives none.
func (s *ReadingState) occurredAt() *time.Time {
	if at := s.LastModified.at(); at != nil {
		return at
	}
	var latest *time.Time
	for _, t := range []*Timestamp{s.statusModified(), s.statisticsModified(), s.bookmarkModified()} {
		if at := t.at(); at != nil && (latest == nil || at.After(*latest)) {
			latest = at
		}
	}
	return latest
}

func (s *ReadingState) statusModified() *Timestamp {
	if s.StatusInfo == nil {
		return nil
	}
	return s.StatusInfo.LastModified
}

func (s *ReadingState) statisticsModified() *Timestamp {
	if s.Statistics == nil {
		return nil
	}
	return s.Statistics.LastModified
}

func (s *ReadingState) bookmarkModified() *Timestamp {
	if s.CurrentBookmark == nil {
		return nil
	}
	return s.CurrentBookmark.LastModified
}

// bookmarkLocator sets update's position from a Kobo's bookmark in file,
// delivered as deliveredType. A span is stored as it is, in the copy the Kobo
// holds. A comic has no spans, so its page is the one the Kobo's percentage
// reaches. A bookmark of any other kind leaves the position as it was.
func bookmarkLocator(update *reading.PositionUpdate, bookmark *CurrentBookmark, file *models.File, deliveredType string) {
	percentage := 0.0
	if bookmark.ProgressPercent != nil {
		percentage = clampFraction(*bookmark.ProgressPercent / 100)
	}
	if models.IsPageBasedFileType(file.FileType) {
		if bookmark.ProgressPercent == nil || file.PageCount == nil || *file.PageCount < 1 {
			return
		}
		page := min(int(math.Floor(percentage*float64(*file.PageCount))), *file.PageCount-1)
		update.Locator = models.ReadingLocator{Page: &page}
		update.Percentage = percentage
		return
	}
	location := bookmark.Location
	if location == nil || location.Type != koboSpanLocation || location.Value == "" || location.Source == "" {
		return
	}
	span := reflowable.KoboSpan{Source: location.Source, ID: location.Value}.String()
	update.Locator = models.ReadingLocator{KoboSpan: &span}
	update.Percentage = percentage
	if deliveredType != file.FileType {
		update.LocatorFileType = &deliveredType
	}
	if bookmark.ContentSourceProgressPercent != nil {
		chapter := clampFraction(*bookmark.ContentSourceProgressPercent / 100)
		update.ChapterPercentage = &chapter
	}
}

func clampFraction(f float64) float64 {
	if math.IsNaN(f) {
		return 0
	}
	return max(0, min(1, f))
}

// updateResult is the result of a PUT of state: success for each part sent.
func updateResult(entryID string, state *ReadingState) *ReadingStateUpdateResult {
	result := &ReadingStateUpdateResult{EntitlementID: entryID}
	if state.CurrentBookmark != nil {
		result.CurrentBookmarkResult = &UpdateResult{Result: updateSucceeded}
	}
	if state.Statistics != nil {
		result.StatisticsResult = &UpdateResult{Result: updateSucceeded}
	}
	if state.StatusInfo != nil {
		result.StatusInfoResult = &UpdateResult{Result: updateSucceeded}
	}
	return result
}

// entryIDsOf returns the ids of the entries a snapshot holds that went out
// to the Kobo.
func entryIDsOf(books []*SyncPointBook) []string {
	ids := make([]string, 0, len(books))
	for _, b := range books {
		if b.EntryID != "" && b.RevisionID != "" {
			ids = append(ids, b.EntryID)
		}
	}
	return ids
}
