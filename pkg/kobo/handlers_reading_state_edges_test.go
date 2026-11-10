package kobo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// webReaderAt saves a web reader position in Alpha at a CFI in its second
// chapter.
func (f *readingStateFixture) webReaderAt(percentage float64) {
	f.t.Helper()
	cfi := "epubcfi(/6/4!/4/2/4/1:67)"
	_, err := f.reading.UpdatePosition(f.ctx, reading.PositionUpdate{
		UserID: f.user.ID, FileID: f.fileA.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: percentage,
	})
	require.NoError(f.t, err)
}

func (f *readingStateFixture) putBody(body string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPut, f.statePath(f.entry), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	return rec
}

// A Kobo update with no bookmark, such as one reporting only its reading
// time, doesn't stand in for a place the web reader saved since, which the
// next sync still sends.
func TestKoboReadingState_PartialUpdateKeepsAnotherReadersPlaceUnsent(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	f.put(koboState{status: "Reading", at: time.Now().Add(-time.Hour), minutes: minutes(2), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 8})
	f.webReaderAt(0.35)

	f.put(koboState{status: "Reading", at: time.Now(), minutes: minutes(2)})

	states := f.changedStates()
	require.Len(t, states, 1)
	require.NotNil(t, states[0].CurrentBookmark)
	assert.InDelta(t, 35, *states[0].CurrentBookmark.ProgressPercent, 0.001)
}

// A book marked read in Shisho while the Kobo read on is sent to the Kobo as
// finished, even after the Kobo reports Reading again.
func TestKoboReadingState_AMarkAsReadSurvivesTheKobosOlderReading(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	started := time.Now().Add(-3 * time.Hour)
	f.put(koboState{status: "Reading", at: started, source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 8})
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, f.bookA.ID, models.ReadThroughOutcomeFinished, time.Now().Format(time.DateOnly)))

	f.put(koboState{status: "Reading", at: time.Now().Add(-time.Minute), source: "OEBPS/text/one.xhtml", span: "kobo.4.1", progress: 12})

	assert.Equal(t, models.ReadingStatusRead, f.status())
	states := f.changedStates()
	require.Len(t, states, 1)
	assert.Equal(t, koboStatusFinished, states[0].StatusInfo.Status)
}

// A sync response that never reached the Kobo is sent again when the Kobo
// retries with the token it last had.
func TestKoboReadingState_ALostSyncIsSentAgain(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	f.webReaderAt(0.35)

	first := f.changedStates()
	require.Len(t, first, 1)
	// The Kobo never got the response, so it syncs again with its old token.
	items, _ := f.syncRaw(t, f.lastToken)
	var resent []ReadingState
	for _, item := range items {
		if raw, ok := item["ChangedReadingState"]; ok {
			var wrapper struct{ ReadingState ReadingState }
			require.NoError(t, json.Unmarshal(raw, &wrapper))
			resent = append(resent, wrapper.ReadingState)
		}
		// The sync it retries from may be gone, which sends the books again
		// as new, each with its state.
		if raw, ok := item["NewEntitlement"]; ok {
			var wrapper struct{ ReadingState *ReadingState }
			require.NoError(t, json.Unmarshal(raw, &wrapper))
			if wrapper.ReadingState != nil && wrapper.ReadingState.EntitlementID == f.entry {
				resent = append(resent, *wrapper.ReadingState)
			}
		}
	}
	require.Len(t, resent, 1)
	assert.InDelta(t, 35, *resent[0].CurrentBookmark.ProgressPercent, 0.001)
}

// A Kobo still holding a book that has left its scope, until its next sync
// removes it, gets an empty success for that book's state, not an error that
// would stop the sync.
func TestKoboReadingState_OutOfScopeBooksAreAccepted(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	f.setScope(t, devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.libB.ID})

	rec := f.putBody(koboState{status: "Finished", at: time.Now(), source: "OEBPS/text/two.xhtml", span: "kobo.4.1", progress: 100}.body(f.entry))
	var response UpdateReadingStatesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, updateSucceeded, response.RequestResult)
	assert.Empty(t, response.UpdateResults)
	assert.Empty(t, f.status(), "a book outside the scope isn't changed")

	get := f.serve(http.MethodGet, f.statePath(f.entry))
	assert.Equal(t, http.StatusOK, get.Code)
}

// A Kobo whose clock runs ahead can't record reading that ends in the future.
func TestKoboReadingState_AFastClockIsHeldToNow(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	ahead := time.Now().Add(3 * time.Hour)

	f.put(koboState{status: "Reading", at: ahead, minutes: minutes(20), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 8})

	sessions := f.sessions()
	require.Len(t, sessions, 1)
	assert.False(t, sessions[0].EndedAt.After(time.Now()), sessions[0].EndedAt)
}

// A book read offline from start to finish, reported only once finished,
// gets its reading time on the finished read.
func TestKoboReadingState_AFinishFirstHeardOfKeepsItsTime(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)

	f.put(koboState{status: "Finished", at: time.Now().Add(-time.Hour), minutes: minutes(300), source: "OEBPS/text/two.xhtml", span: "kobo.40.1", progress: 100})

	rts := f.bookReading().ReadThroughs
	require.Len(t, rts, 1)
	sessions := f.sessions()
	require.Len(t, sessions, 1)
	require.NotNil(t, sessions[0].ReadThroughID)
	assert.Equal(t, rts[0].ID, *sessions[0].ReadThroughID)
}

// A Kobo queues its updates while offline and sends them in order on
// reconnecting, each with its own time, so a read started offline begins on
// the day the Kobo first opened the book.
func TestKoboReadingState_AnOfflineQueueIsDatedByTheKobo(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	opened := time.Now().Add(-72 * time.Hour)

	f.put(koboState{status: "Reading", at: opened, minutes: minutes(0), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 2})
	f.put(koboState{status: "Reading", at: opened.Add(40 * time.Minute), minutes: minutes(30), source: "OEBPS/text/one.xhtml", span: "kobo.5.1", progress: 20})

	rts := f.bookReading().ReadThroughs
	require.Len(t, rts, 1)
	assert.Equal(t, opened.UTC().Format(time.DateOnly), *rts[0].BeginDay)
	sessions := f.sessions()
	require.Len(t, sessions, 1)
	assert.Equal(t, 30*time.Minute, sessions[0].EndedAt.Sub(sessions[0].StartedAt))
}

// Some firmware writes times without a zone, and whole-number percentages.
func TestKoboReadingState_ZonelessTimesAndWholePercents(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	at := time.Now().Add(-48 * time.Hour).UTC().Format("2006-01-02T15:04:05")
	body := `{"ReadingStates":[{"EntitlementId":"` + f.entry + `","LastModified":"` + at + `",` +
		`"StatusInfo":{"LastModified":"` + at + `","Status":"Reading"},` +
		`"CurrentBookmark":{"LastModified":"` + at + `","ProgressPercent":23,"ContentSourceProgressPercent":50,` +
		`"Location":{"Value":"kobo.3.1","Type":"KoboSpan","Source":"OEBPS/text/one.xhtml"}}}]}`

	f.putBody(body)

	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, f.fileA.ID)
	require.NoError(t, err)
	require.NotNil(t, position)
	assert.InDelta(t, 0.23, position.Percentage, 0.0001)
	assert.Equal(t, at[:10], position.OccurredAt.UTC().Format(time.DateOnly))
}

// A state for another book in the body isn't applied to this one.
func TestKoboReadingState_OnlyThisBooksStateApplies(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)

	f.putBody(koboState{status: "Finished", at: time.Now(), source: "OEBPS/text/two.xhtml", span: "kobo.4.1", progress: 100}.body("shisho-eotherbook"))

	assert.Empty(t, f.status())
}

// A comic has no spans, so a Kobo shares its status and time, not its place.
func TestKoboReadingState_AComicSharesItsStatusNotItsPlace(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	_, cbz := insertScopeBookWithFile(t.Context(), t, f.db, f.libA.ID, "Comic", models.FileTypeCBZ)
	pages := 20
	_, err := f.db.NewUpdate().Model((*models.File)(nil)).Set("page_count = ?", pages).Where("id = ?", cbz.ID).Exec(t.Context())
	require.NoError(t, err)
	f.syncRaw(t, "")
	r := &readingStateFixture{koboScopeFixture: f, t: t, ctx: t.Context(), reading: reading.NewService(f.db), entry: f.entryID(t, cbz.ID)}

	r.put(koboState{status: "Reading", at: time.Now().Add(-time.Minute), source: "OEBPS/page_0005.xhtml", span: "kobo.1.1", progress: 25})

	position, err := r.reading.RetrievePosition(r.ctx, f.user.ID, cbz.ID)
	require.NoError(t, err)
	assert.Nil(t, position)
	status, err := r.reading.RetrieveBookReading(r.ctx, f.user.ID, cbz.BookID)
	require.NoError(t, err)
	require.NotNil(t, status.Status)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, *status.Status)
}

// Reading states past what fits on a page go on later pages. A page asked for
// again, as when its response was lost, sends its states again; one the Kobo
// has moved past doesn't, and states sent in a sync the Kobo then syncs from
// are heard for good.
func TestKoboReadingState_StatesPageAndRetry(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	f.syncRaw(t, "")
	readingService := reading.NewService(f.db)
	for _, file := range []*models.File{f.fileA, f.fileB} {
		cfi := "epubcfi(/6/4!/4/2/1:0)"
		_, err := readingService.UpdatePosition(t.Context(), reading.PositionUpdate{
			UserID: f.user.ID, FileID: file.ID, Source: models.ReadingSourceWebReader,
			Locator: models.ReadingLocator{CFI: &cfi}, Percentage: 0.3,
		})
		require.NoError(t, err)
	}
	states := &readingStates{db: f.db, reading: readingService}
	ids := []string{f.entryID(t, f.fileA.ID), f.entryID(t, f.fileB.ID)}
	changed := func(page syncPage) ([]*ReadingState, bool) {
		t.Helper()
		got, more, err := states.changed(t.Context(), f.user.ID, f.device.ID, ids, nil, page, 1)
		require.NoError(t, err)
		return got, more
	}

	first, more := changed(syncPage{pointID: "point-2", cursor: 0})
	require.Len(t, first, 1)
	assert.True(t, more)
	again, _ := changed(syncPage{pointID: "point-2", cursor: 0})
	require.Len(t, again, 1, "a retried page sends its state again")
	assert.Equal(t, first[0].EntitlementID, again[0].EntitlementID)

	second, more := changed(syncPage{pointID: "point-2", cursor: 1})
	require.Len(t, second, 1)
	assert.False(t, more)
	assert.NotEqual(t, first[0].EntitlementID, second[0].EntitlementID)
	last, _ := changed(syncPage{pointID: "point-2", cursor: 2})
	assert.Empty(t, last)

	// A new sync that isn't from point-2 never got them.
	retried, _ := changed(syncPage{pointID: "point-3", cursor: 0})
	assert.Len(t, retried, 1)

	// Syncing from point-3 confirms its state; the other, last sent in
	// point-2, which the Kobo never synced from, goes out again.
	require.NoError(t, states.confirmSync(t.Context(), f.device.ID, "point-3"))
	after, more := changed(syncPage{pointID: "point-4", cursor: 0})
	require.Len(t, after, 1)
	assert.False(t, more)
	assert.NotEqual(t, retried[0].EntitlementID, after[0].EntitlementID)
}

// A Mark as read whose sync never reached the Kobo is sent again, even after
// the Kobo reports more reading in between.
func TestKoboReadingState_ALostFinishIsSentAgainAfterAKoboUpdate(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	started := time.Now().Add(-3 * time.Hour)
	f.put(koboState{status: "Reading", at: started, source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 8})
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, f.bookA.ID, models.ReadThroughOutcomeFinished, time.Now().Format(time.DateOnly)))
	require.Len(t, f.changedStates(), 1)
	lost := f.lastToken

	f.put(koboState{status: "Reading", at: time.Now(), source: "OEBPS/text/one.xhtml", span: "kobo.6.1", progress: 15})

	items, _ := f.syncRaw(t, lost)
	var finished bool
	for _, item := range items {
		for _, key := range []string{"ChangedReadingState", "NewEntitlement"} {
			raw, ok := item[key]
			if !ok {
				continue
			}
			var wrapper struct{ ReadingState *ReadingState }
			require.NoError(t, json.Unmarshal(raw, &wrapper))
			if wrapper.ReadingState != nil && wrapper.ReadingState.EntitlementID == f.entry {
				finished = wrapper.ReadingState.StatusInfo.Status == koboStatusFinished
			}
		}
	}
	assert.True(t, finished)
}

// The day a read began goes to the Kobo as the start of that day where the
// reader is, not in UTC, which west of UTC is still the day before.
func TestKoboReadingState_StartDayIsInTheReadersTimeZone(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	zone := "America/Los_Angeles"
	_, err := f.db.NewInsert().Model(&models.UserSettings{
		UserID: f.user.ID, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ViewerFitMode: models.FitModeHeight, ReflowableTheme: models.ReflowableThemeLight,
		ReflowableFlow: models.ReflowableFlowPaginated, GallerySize: models.GallerySizeMedium,
		PlaybackSpeed: 1, ViewerPreloadCount: 3, ReflowableFontSize: 100, TimeZone: &zone,
	}).Exec(f.ctx)
	require.NoError(t, err)
	location, err := time.LoadLocation(zone)
	require.NoError(t, err)
	f.put(koboState{status: "Reading", at: time.Now().Add(-time.Minute), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 8})
	rts := f.bookReading().ReadThroughs
	require.Len(t, rts, 1)
	day, err := time.ParseInLocation(time.DateOnly, *rts[0].BeginDay, location)
	require.NoError(t, err)

	rec := f.serve(http.MethodGet, f.statePath(f.entry))
	require.Equal(t, http.StatusOK, rec.Code)
	var states []ReadingState
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &states))
	require.Len(t, states, 1)
	require.NotNil(t, states[0].StatusInfo.LastTimeStartedReading)
	assert.True(t, day.Equal(states[0].StatusInfo.LastTimeStartedReading.Time),
		"got %s, want %s", states[0].StatusInfo.LastTimeStartedReading.Time, day.UTC())
}

// capturedBody is a reading state a Kobo Libra Colour (firmware 4.46) sent
// during device testing, from testdata, addressed to entryID. Only its ids
// and its span's chapter and id were changed, onto the test book.
func capturedBody(t *testing.T, name, entryID string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return strings.ReplaceAll(string(data), "{{ENTRY_ID}}", entryID)
}

// A Kobo's own updates: one it queued while offline weeks ago, arriving on
// reconnecting, then one as it reads.
func TestKoboReadingState_CapturedKoboUpdates(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)

	f.putBody(capturedBody(t, "kobo-state-queued-offline.json", f.entry))

	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
	rts := f.bookReading().ReadThroughs
	require.Len(t, rts, 1)
	assert.Equal(t, "2026-10-27", *rts[0].BeginDay, "dated when the Kobo was reading, not when it reconnected")
	sessions := f.sessions()
	require.Len(t, sessions, 1)
	assert.Equal(t, time.Minute, sessions[0].EndedAt.Sub(sessions[0].StartedAt))

	f.putBody(capturedBody(t, "kobo-state-reading.json", f.entry))

	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, f.fileA.ID)
	require.NoError(t, err)
	require.NotNil(t, position)
	require.NotNil(t, position.KoboSpan)
	assert.Equal(t, "OEBPS/text/two.xhtml#kobo.6.2", *position.KoboSpan)
	assert.InDelta(t, 0.16, position.Percentage, 0.0001)
	require.NotNil(t, position.ChapterPercentage)
	assert.InDelta(t, 0.75, *position.ChapterPercentage, 0.0001)
	assert.Len(t, f.sessions(), 1, "the minutes didn't go up")
}
