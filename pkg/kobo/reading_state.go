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
	book     *reading.StatusSummary
}

// tracked reports whether the user has read or tracked the entry's book at
// all, so there's a state worth sending.
func (in *stateInputs) tracked() bool {
	return in.position != nil || in.book != nil && in.book.Status != nil
}

// untracked is the version of every book nobody read or tracked, whichever
// File its entry holds: there's nothing to send.
const untracked = "untracked"

// version is what the Kobo would have to have heard for its state to be
// current: the File the entry holds, the status and when it changed, and the
// position and when it was stored. It changes whenever the state Shisho
// would send does, and building the state, which can mean translating the
// position, waits until it has.
func (in *stateInputs) version() string {
	if !in.tracked() {
		return untracked
	}
	status, statusAt := "", int64(0)
	if in.book != nil {
		if in.book.Status != nil {
			status = *in.book.Status
		}
		if in.book.ChangedAt != nil {
			statusAt = in.book.ChangedAt.UnixNano()
		}
	}
	position := "none"
	if in.position != nil {
		position = fmt.Sprintf("%s@%d", in.position.Source, in.position.UpdatedAt.UnixNano())
	}
	return fmt.Sprintf("%d|%s|%d|%s", in.file.ID, status, statusAt, position)
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
		Column("f.id", "f.book_id", "f.file_type", "f.filepath").
		Where("f.id IN (?)", bun.List(fileIDs)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	filesByID := make(map[int]*models.File, len(files))
	bookIDs := make([]int, 0, len(files))
	for _, f := range files {
		filesByID[f.ID] = f
		bookIDs = append(bookIDs, f.BookID)
	}
	positions, err := r.reading.RetrievePositions(ctx, userID, fileIDs)
	if err != nil {
		return nil, err
	}
	summaries, err := r.reading.RetrieveStatusSummaries(ctx, userID, bookIDs)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if e.FileID == nil {
			continue
		}
		file := filesByID[*e.FileID]
		if file == nil {
			continue
		}
		inputs[e.EntryID] = &stateInputs{entry: e, file: file, position: positions[file.ID], book: summaries[file.BookID]}
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
	if in.book != nil && in.book.ChangedAt != nil {
		statusAt = *in.book.ChangedAt
	} else if in.position != nil {
		statusAt = in.position.OccurredAt
	}
	later(statusAt)
	info := &StatusInfo{LastModified: NewTimestamp(statusAt), Status: koboStatusReadyToRead}
	if in.book != nil {
		info.Status = koboStatus(in.book.Status)
		times := in.book.ReadThroughs
		info.TimesStartedReading = &times
		if in.book.LastBeganAt != nil {
			info.LastTimeStartedReading = NewTimestamp(*in.book.LastBeganAt)
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
// span can stand for, as in an EPUB delivered as itself or a comic, goes with
// its percentage only.
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

// syncPage is the page of a sync a reading state goes out on: its sync point,
// and where the page starts. The Kobo has heard the page once it asks for a
// later one, or syncs again from the point.
type syncPage struct {
	pointID string
	cursor  int
}

// heard reports whether the Kobo has heard version of e's state: the version
// it last confirmed, or the one an earlier page of this sync sent it.
func heard(e *SyncEntry, version string, page syncPage) bool {
	if e.ReadingStateVersion != nil && *e.ReadingStateVersion == version {
		return true
	}
	return e.ReadingStateSentVersion != nil && *e.ReadingStateSentVersion == version &&
		e.ReadingStateSentIn != nil && *e.ReadingStateSentIn == page.pointID &&
		e.ReadingStateSentCursor != nil && *e.ReadingStateSentCursor < page.cursor
}

// confirmSync records that the Kobo has heard every reading state sent in
// the sync point pointID, which it's syncing from.
func (r *readingStates) confirmSync(ctx context.Context, deviceID, pointID string) error {
	if pointID == "" {
		return nil
	}
	_, err := r.db.NewUpdate().
		Model((*SyncEntry)(nil)).
		Set("reading_state_version = reading_state_sent_version").
		Set("reading_state_sent_version = NULL").
		Set("reading_state_sent_in = NULL").
		Set("reading_state_sent_cursor = NULL").
		Where("device_id = ? AND reading_state_sent_in = ?", deviceID, pointID).
		Exec(ctx)
	return errors.WithStack(err)
}

// markSent records that page sends version of entryID's state.
func (r *readingStates) markSent(ctx context.Context, deviceID, entryID, version string, page syncPage) error {
	_, err := r.db.NewUpdate().
		Model((*SyncEntry)(nil)).
		Set("reading_state_sent_version = ?", version).
		Set("reading_state_sent_in = ?", page.pointID).
		Set("reading_state_sent_cursor = ?", page.cursor).
		Where("device_id = ? AND entry_id = ?", deviceID, entryID).
		Exec(ctx)
	return errors.WithStack(err)
}

// setHeard records version as what the Kobo has heard of entryID's state,
// or that it's heard nothing current when version is nil, so the next sync
// sends it.
func (r *readingStates) setHeard(ctx context.Context, deviceID, entryID string, version *string) error {
	_, err := r.db.NewUpdate().
		Model((*SyncEntry)(nil)).
		Set("reading_state_version = ?", version).
		Set("reading_state_sent_version = NULL").
		Set("reading_state_sent_in = NULL").
		Set("reading_state_sent_cursor = NULL").
		Where("device_id = ? AND entry_id = ?", deviceID, entryID).
		Exec(ctx)
	return errors.WithStack(err)
}

// forNewEntries returns the reading state each of entryIDs, reaching the
// Kobo on page, goes with: one for every book the user has read or tracked,
// by entry id.
func (r *readingStates) forNewEntries(ctx context.Context, userID int, deviceID string, entryIDs []string, page syncPage) (map[string]*ReadingState, error) {
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
		version := in.version()
		if !in.tracked() {
			// Nothing to send, and nothing to lose if the page is.
			if err := r.setHeard(ctx, deviceID, e.EntryID, &version); err != nil {
				return nil, err
			}
			continue
		}
		states[e.EntryID] = r.build(ctx, in)
		if err := r.markSent(ctx, deviceID, e.EntryID, version, page); err != nil {
			return nil, err
		}
	}
	return states, nil
}

// changed returns the reading states of entryIDs the Kobo hasn't heard,
// leaving out those in sent, which this page already carries: at most limit
// of them, in entry id order, and whether more remain. A book nobody read
// or tracked, that the Kobo hasn't heard of either, has nothing to send.
func (r *readingStates) changed(ctx context.Context, userID int, deviceID string, entryIDs []string, sent map[string]*ReadingState, page syncPage, limit int) ([]*ReadingState, bool, error) {
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
		if in == nil || sent[e.EntryID] != nil {
			continue
		}
		version := in.version()
		if heard(e, version, page) {
			continue
		}
		if e.ReadingStateVersion == nil && version == untracked {
			if err := r.setHeard(ctx, deviceID, e.EntryID, &version); err != nil {
				return nil, false, err
			}
			continue
		}
		if len(states) == limit {
			return states, true, nil
		}
		states = append(states, r.build(ctx, in))
		if err := r.markSent(ctx, deviceID, e.EntryID, version, page); err != nil {
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

// apply takes in a Kobo's reading state for entry. Kobo's own times date
// everything, so reading done offline lands on the day it happened:
//
//   - Reading starts or resumes the Read-through, and Finished ends it, for
//     good. ReadyToRead says nothing about reading, so it changes nothing.
//   - The bookmark, a span with how far through the chapter and the book it
//     is, is stored as the position.
//   - The increase in the Kobo's reading minutes since its last report is an
//     estimated Reading Session ending when the Kobo reported it.
//
// A Kobo that had heard Shisho's state before the update has it still,
// with its own update on top. One that hadn't is sent the merged state with
// the next sync, as is one whose update was older than Shisho's.
func (r *readingStates) apply(ctx context.Context, user *models.User, device *devices.Device, entry *SyncEntry, state *ReadingState) error {
	now := time.Now()
	file := &models.File{}
	err := r.db.NewSelect().
		Model(file).
		Column("f.id", "f.file_type").
		Where("f.id = ?", *entry.FileID).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	before, err := r.load(ctx, user.ID, []*SyncEntry{entry})
	if err != nil {
		return err
	}
	// wasCurrent is the Kobo having confirmed the state before the update,
	// and wasSent a sync having sent it, which the Kobo may not have got.
	wasCurrent, wasSent := false, false
	var stored *models.ReadingPosition
	if in := before[entry.EntryID]; in != nil {
		version := in.version()
		wasCurrent = entry.ReadingStateVersion != nil && *entry.ReadingStateVersion == version
		wasSent = entry.ReadingStateSentVersion != nil && *entry.ReadingStateSentVersion == version
		stored = in.position
	}

	update := reading.PositionUpdate{
		UserID:     user.ID,
		FileID:     file.ID,
		Source:     models.ReadingSourceKobo,
		DeviceID:   &device.ID,
		OccurredAt: state.occurredAt(),
	}
	// A Kobo stamps its status with the time of every update, whether or
	// not it changed, so every Reading it reports is reading: it resumes a
	// Book paused in Shisho.
	if info := state.StatusInfo; info != nil {
		switch info.Status {
		case koboStatusReading:
			update.ReportedStatus = reading.ReportedReading
		case koboStatusFinished:
			update.ReportedStatus = reading.ReportedFinished
		}
	}
	if state.CurrentBookmark != nil {
		bookmarkLocator(&update, state.CurrentBookmark, file, entry.DeliveredFileType)
	}

	if stats := state.Statistics; stats != nil && stats.SpentReadingMinutes != nil {
		reported := *stats.SpentReadingMinutes
		at := now
		if when := stats.LastModified.at(); when != nil {
			at = *when
		} else if update.OccurredAt != nil {
			at = *update.OccurredAt
		}
		// A Kobo whose clock runs fast can't have read in the future.
		if at.After(now) {
			at = now
		}
		if entry.ReadingMinutes != nil && reported > *entry.ReadingMinutes {
			endPercentage := update.Percentage
			startPercentage := endPercentage
			if stored != nil {
				startPercentage = stored.Percentage
			}
			if update.Locator.IsZero() {
				endPercentage = startPercentage
			}
			started := at.Add(-time.Duration(reported-*entry.ReadingMinutes) * time.Minute)
			update.Session = &reading.SessionInput{
				StartedAt:       started,
				EndedAt:         at,
				StartPercentage: startPercentage,
				EndPercentage:   endPercentage,
			}
			update.ListeningSince = &started
		}
		// The new count is saved before the session it makes, so a failure
		// between the two loses that session rather than recording it twice
		// when the Kobo sends the same count again.
		_, err := r.db.NewUpdate().
			Model((*SyncEntry)(nil)).
			Set("reading_minutes = ?", reported).
			Set("reading_minutes_at = ?", at).
			Where("device_id = ? AND entry_id = ?", entry.DeviceID, entry.EntryID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		entry.ReadingMinutes, entry.ReadingMinutesAt = &reported, &at
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

	if !accepted {
		return r.setHeard(ctx, entry.DeviceID, entry.EntryID, nil)
	}
	if !wasCurrent && !wasSent {
		return nil
	}
	after, err := r.load(ctx, user.ID, []*SyncEntry{entry})
	if err != nil {
		return err
	}
	in := after[entry.EntryID]
	if in == nil {
		return nil
	}
	version := in.version()
	if wasCurrent {
		return r.setHeard(ctx, entry.DeviceID, entry.EntryID, &version)
	}
	// The Kobo has what was sent once it confirms the sync that sent it, and
	// its own update on top; until then a retry sends both.
	_, err = r.db.NewUpdate().
		Model((*SyncEntry)(nil)).
		Set("reading_state_sent_version = ?", version).
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
// delivered as deliveredType: its span, stored as it is, in the copy the Kobo
// holds. A bookmark of any other kind, such as in a comic, which has no
// spans, leaves the position as it was.
func bookmarkLocator(update *reading.PositionUpdate, bookmark *CurrentBookmark, file *models.File, deliveredType string) {
	location := bookmark.Location
	if models.IsPageBasedFileType(file.FileType) || location == nil || location.Type != koboSpanLocation || location.Value == "" || location.Source == "" {
		return
	}
	span := reflowable.KoboSpan{Source: location.Source, ID: location.Value}.String()
	update.Locator = models.ReadingLocator{KoboSpan: &span}
	if bookmark.ProgressPercent != nil {
		update.Percentage = clampFraction(*bookmark.ProgressPercent / 100)
	}
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
