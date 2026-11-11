package kobo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/reflowable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// koboTimeLayout is how a Kobo writes times: UTC, to the second.
const koboTimeLayout = "2006-01-02T15:04:05Z"

// koboState is one book's reading state as a Kobo PUTs it to
// /v1/library/{id}/state, in the shape a Kobo Libra Colour (firmware 4.46)
// sent during device testing (testdata/kobo-state-*.json): every part dated
// with the update's own time, the status included even when unchanged, and
// whole-number percentages. Empty fields leave their part out.
type koboState struct {
	status   string
	at       time.Time
	minutes  *int
	source   string
	span     string
	progress int
	chapter  int
}

func (s koboState) body(entryID string) string {
	at := s.at.UTC().Format(koboTimeLayout)
	var parts []string
	if s.span != "" {
		parts = append(parts, fmt.Sprintf(`"CurrentBookmark":{"ContentSourceProgressPercent":%d,"LastModified":%q,"Location":{"Source":%q,"Type":"KoboSpan","Value":%q},"ProgressPercent":%d}`,
			s.chapter, at, s.source, s.span, s.progress))
	}
	parts = append(parts, fmt.Sprintf(`"EntitlementId":%q`, entryID), fmt.Sprintf(`"LastModified":%q`, at))
	if s.minutes != nil {
		parts = append(parts, fmt.Sprintf(`"Statistics":{"LastModified":%q,"RemainingTimeMinutes":143,"SpentReadingMinutes":%d}`, at, *s.minutes))
	}
	if s.status != "" {
		parts = append(parts, fmt.Sprintf(`"StatusInfo":{"LastModified":%q,"Status":%q}`, at, s.status))
	}
	return `{"ReadingStates":[{` + strings.Join(parts, ",") + `}]}`
}

func minutes(n int) *int { return &n }

// readingStateFixture is the scope fixture with Alpha's EPUB replaced by the
// handoff EPUB, synced once so the Kobo holds it as a KePub.
type readingStateFixture struct {
	*koboScopeFixture
	t       *testing.T
	ctx     context.Context
	reading *reading.Service
	entry   string
	token   string
	// lastToken is the token the latest sync was sent with.
	lastToken string
}

func newReadingStateFixture(t *testing.T) *readingStateFixture {
	t.Helper()
	f := &readingStateFixture{koboScopeFixture: newKoboScopeFixture(t), t: t, ctx: context.Background()}
	f.reading = reading.NewService(f.db)
	path := testgen.GenerateHandoffEPUB(t, filepath.Dir(f.fileA.Filepath), "handoff.epub")
	_, err := f.db.NewUpdate().Model((*models.File)(nil)).Set("filepath = ?", path).Where("id = ?", f.fileA.ID).Exec(f.ctx)
	require.NoError(t, err)
	f.fileA.Filepath = path
	_, f.token = f.syncRaw(t, "")
	f.entry = f.entryID(t, f.fileA.ID)
	return f
}

func (f *readingStateFixture) statePath(entryID string) string {
	return f.prefix() + "/v1/library/" + entryID + "/state"
}

// put sends state as the Kobo does, which expects to hear that it succeeded.
func (f *readingStateFixture) put(state koboState) {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPut, f.statePath(f.entry), strings.NewReader(state.body(f.entry)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var response UpdateReadingStatesResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(f.t, updateSucceeded, response.RequestResult)
	require.Len(f.t, response.UpdateResults, 1)
	assert.Equal(f.t, f.entry, response.UpdateResults[0].EntitlementID)
}

// setTimeZone sets the reader's time zone, returning it.
func (f *readingStateFixture) setTimeZone(zone string) *time.Location {
	f.t.Helper()
	_, err := f.db.NewInsert().Model(&models.UserSettings{
		UserID: f.user.ID, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ViewerFitMode: models.FitModeHeight, ReflowableTheme: models.ReflowableThemeLight,
		ReflowableFlow: models.ReflowableFlowPaginated, GallerySize: models.GallerySizeMedium,
		PlaybackSpeed: 1, ViewerPreloadCount: 3, ReflowableFontSize: 100, TimeZone: &zone,
	}).Exec(f.ctx)
	require.NoError(f.t, err)
	location, err := time.LoadLocation(zone)
	require.NoError(f.t, err)
	return location
}

func (f *readingStateFixture) bookReading() *reading.BookReading {
	f.t.Helper()
	r, err := f.reading.RetrieveBookReading(f.ctx, f.user.ID, f.bookA.ID)
	require.NoError(f.t, err)
	return r
}

func (f *readingStateFixture) status() string {
	f.t.Helper()
	if s := f.bookReading().Status; s != nil {
		return *s
	}
	return ""
}

func (f *readingStateFixture) sessions() []*models.ReadingSession {
	f.t.Helper()
	var sessions []*models.ReadingSession
	require.NoError(f.t, f.db.NewSelect().Model(&sessions).Where("rs.user_id = ?", f.user.ID).Order("rs.id").Scan(f.ctx))
	return sessions
}

// syncRaw runs one sync and returns its items undecoded and the next token.
func (f *koboScopeFixture) syncRaw(t *testing.T, token string) ([]map[string]json.RawMessage, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, f.prefix()+"/v1/library/sync", nil)
	if token != "" {
		req.Header.Set("X-Kobo-SyncToken", token)
	}
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Empty(t, rec.Header().Get("X-Kobo-Sync"), "fixture syncs fit in one page")
	var items []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &items))
	return items, rec.Header().Get("X-Kobo-SyncToken")
}

// changedStates syncs and returns the reading states the sync sent as
// ChangedReadingState.
func (f *readingStateFixture) changedStates() []ReadingState {
	f.t.Helper()
	items, token := f.syncRaw(f.t, f.token)
	f.lastToken, f.token = f.token, token
	var states []ReadingState
	for _, item := range items {
		raw, ok := item["ChangedReadingState"]
		if !ok {
			continue
		}
		var wrapper struct{ ReadingState ReadingState }
		require.NoError(f.t, json.Unmarshal(raw, &wrapper))
		states = append(states, wrapper.ReadingState)
	}
	return states
}

// Opening a synced book on the Kobo reports Reading, which starts the
// Read-through on the day the Kobo says, at the Kobo's first screen.
func TestKoboReadingState_ReadingStartsTheReadThrough(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	at := time.Now().Add(-26 * time.Hour)

	f.put(koboState{status: "Reading", at: at, source: "OEBPS/text/one.xhtml", span: "kobo.1.1"})

	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
	rts := f.bookReading().ReadThroughs
	require.Len(t, rts, 1)
	assert.Equal(t, at.UTC().Format(time.DateOnly), *rts[0].BeginDay)
}

// Finished ends the Read-through, and the Kobo going back right after, as
// when the reader flips back from the last page, doesn't undo it.
func TestKoboReadingState_FinishedEndsWithoutTheRevertWindow(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	start := time.Now().Add(-time.Hour)
	f.put(koboState{status: "Reading", at: start, source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 20, chapter: 30})

	f.put(koboState{status: "Finished", at: start.Add(30 * time.Minute), source: "OEBPS/text/two.xhtml", span: "kobo.40.1", progress: 100, chapter: 100})
	assert.Equal(t, models.ReadingStatusRead, f.status())

	f.put(koboState{status: "Reading", at: start.Add(32 * time.Minute), source: "OEBPS/text/two.xhtml", span: "kobo.2.1", progress: 60, chapter: 10})
	assert.Equal(t, models.ReadingStatusRead, f.status())
	require.Len(t, f.bookReading().ReadThroughs, 1)
}

// "ready to read" says nothing about reading: it starts nothing.
func TestKoboReadingState_ReadyToReadIsIgnored(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)

	f.put(koboState{status: "ReadyToRead", at: time.Now().Add(-time.Minute), source: "OEBPS/text/one.xhtml", span: "kobo.1.1"})

	assert.Empty(t, f.status())
}

// The Kobo's bookmark is stored as its place, and the web reader opens in
// the same paragraph, as a CFI in the EPUB the Kobo's KePub was made from.
func TestKoboReadingState_StoresTheBookmark(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	kepubPath := filepath.Join(t.TempDir(), "handoff.kepub.epub")
	require.NoError(t, kepub.NewConverter().ConvertEPUB(f.ctx, f.fileA.Filepath, kepubPath))
	delivered, err := reflowable.Open(f.ctx, kepubPath, true)
	require.NoError(t, err)
	span := reflowable.KoboSpan{Source: "OEBPS/text/two.xhtml", ID: "kobo.6.2"}
	want, err := delivered.FromKoboSpan(span)
	require.NoError(t, err)

	f.put(koboState{status: "Reading", at: time.Now().Add(-time.Minute), source: span.Source, span: span.ID, progress: 47, chapter: 12})

	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, f.fileA.ID)
	require.NoError(t, err)
	require.NotNil(t, position)
	assert.InDelta(t, 0.47, position.Percentage, 0.0001)
	assert.Equal(t, models.ReadingSourceKobo, position.Source)

	web, err := f.reading.RetrieveWebReaderPosition(f.ctx, f.user.ID, f.fileA.ID)
	require.NoError(t, err)
	require.NotNil(t, web.CFI)
	plain, err := reflowable.Open(f.ctx, f.fileA.Filepath, false)
	require.NoError(t, err)
	landed, err := plain.FromCFI(*web.CFI)
	require.NoError(t, err)
	assert.Equal(t, want.Spine, landed.Spine)
	assert.InDelta(t, want.Offset, landed.Offset, 100)
}

// Each increase in the Kobo's reading minutes is a Reading Session of that
// long, ending when the Kobo says. A report with no increase records none.
func TestKoboReadingState_MinutesBecomeSessions(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	first := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	second := first.Add(2 * time.Hour)

	f.put(koboState{status: "Reading", at: first, minutes: minutes(10), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 10})
	f.put(koboState{status: "Reading", at: second, minutes: minutes(25), source: "OEBPS/text/two.xhtml", span: "kobo.3.1", progress: 40})
	f.put(koboState{status: "Reading", at: second.Add(time.Minute), minutes: minutes(25), source: "OEBPS/text/two.xhtml", span: "kobo.3.1", progress: 40})

	sessions := f.sessions()
	require.Len(t, sessions, 2)
	assert.Equal(t, models.ReadingSourceKobo, sessions[0].Source)
	assert.Equal(t, &f.device.ID, sessions[0].SourceDeviceID, "the session names the Kobo")
	assert.False(t, sessions[0].Measured)
	assert.True(t, first.Equal(sessions[0].EndedAt), sessions[0].EndedAt)
	assert.Equal(t, 10*time.Minute, sessions[0].EndedAt.Sub(sessions[0].StartedAt))
	assert.True(t, second.Equal(sessions[1].EndedAt), sessions[1].EndedAt)
	assert.Equal(t, 15*time.Minute, sessions[1].EndedAt.Sub(sessions[1].StartedAt))
	assert.InDelta(t, 0.1, sessions[1].StartPercentage, 0.0001)
	assert.InDelta(t, 0.4, sessions[1].EndPercentage, 0.0001)
}

// An entry on the Kobo from before reading state synced counted minutes
// Shisho never saw, so its first report only sets where counting starts.
func TestKoboReadingState_FirstMinutesOfAnOlderEntryAreTheBaseline(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	_, err := f.db.NewUpdate().Model((*SyncEntry)(nil)).Set("reading_minutes = NULL").Where("entry_id = ?", f.entry).Exec(f.ctx)
	require.NoError(t, err)
	at := time.Now().Add(-time.Hour)

	f.put(koboState{status: "Reading", at: at, minutes: minutes(300), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 10})
	assert.Empty(t, f.sessions())

	f.put(koboState{status: "Reading", at: at.Add(20 * time.Minute), minutes: minutes(312), source: "OEBPS/text/one.xhtml", span: "kobo.3.1", progress: 12})
	sessions := f.sessions()
	require.Len(t, sessions, 1)
	assert.Equal(t, 12*time.Minute, sessions[0].EndedAt.Sub(sessions[0].StartedAt))
}

// A book finished with Wi-Fi off and synced days later is Read on the day
// the Kobo finished it, in the reader's time zone.
func TestKoboReadingState_KoboTimesDateAFinish(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	location := f.setTimeZone("America/Los_Angeles")
	// Late in the evening in Los Angeles, which is already the next day in UTC.
	day := time.Now().In(location).AddDate(0, 0, -3)
	finishedAt := time.Date(day.Year(), day.Month(), day.Day(), 22, 30, 0, 0, location)

	f.put(koboState{status: "Reading", at: finishedAt.Add(-2 * time.Hour), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 50})
	f.put(koboState{status: "Finished", at: finishedAt, source: "OEBPS/text/two.xhtml", span: "kobo.40.1", progress: 100})

	assert.Equal(t, models.ReadingStatusRead, f.status())
	rts := f.bookReading().ReadThroughs
	require.Len(t, rts, 1)
	assert.Equal(t, finishedAt.Format(time.DateOnly), *rts[0].EndDay)
}

// The Kobo asking for a Shisho book's state gets what Shisho has.
func TestKoboReadingState_GetReturnsTheState(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	f.put(koboState{status: "Reading", at: time.Now().Add(-time.Minute), minutes: minutes(4), source: "OEBPS/text/one.xhtml", span: "kobo.3.1", progress: 9, chapter: 40})

	rec := f.serve(http.MethodGet, f.statePath(f.entry))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var states []ReadingState
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &states))
	require.Len(t, states, 1)
	state := states[0]
	assert.Equal(t, f.entry, state.EntitlementID)
	require.NotNil(t, state.StatusInfo)
	assert.Equal(t, koboStatusReading, state.StatusInfo.Status)
	require.NotNil(t, state.CurrentBookmark)
	require.NotNil(t, state.CurrentBookmark.Location)
	assert.Equal(t, "kobo.3.1", state.CurrentBookmark.Location.Value)
	assert.Equal(t, "OEBPS/text/one.xhtml", state.CurrentBookmark.Location.Source)
	require.NotNil(t, state.Statistics)
	assert.Equal(t, 4, *state.Statistics.SpentReadingMinutes)
}

// storeCall is a request that reached the fake Kobo store.
type storeCall struct{ method, path, body string }

// receive returns the next request the fake store got, failing rather than
// hanging when the request never reached it.
func receive(t *testing.T, calls <-chan storeCall) storeCall {
	t.Helper()
	select {
	case got := <-calls:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached the store")
		return storeCall{}
	}
}

// Reading state for a book from Kobo's store still goes to the store, both
// ways. It swaps the store's address, so it can't run alongside other tests
// that proxy.
//
//nolint:paralleltest // Replaces koboStoreBaseURL.
func TestKoboReadingState_StoreBooksGoToTheStore(t *testing.T) {
	calls := make(chan storeCall, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls <- storeCall{r.Method, r.URL.Path, string(body)}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"RequestResult":"Success"}`))
	}))
	defer upstream.Close()
	original := koboStoreBaseURL
	koboStoreBaseURL = upstream.URL
	defer func() { koboStoreBaseURL = original }()

	f := newReadingStateFixture(t)
	storeBook := "4d0b5a3e-6c2f-4f8e-9d3a-1b2c3d4e5f60"
	body := koboState{status: "Reading", at: time.Now(), source: "OEBPS/ch1.xhtml", span: "kobo.1.1"}.body(storeBook)

	req := httptest.NewRequest(http.MethodPut, f.statePath(storeBook), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	got := receive(t, calls)
	assert.Equal(t, http.MethodPut, got.method)
	assert.Equal(t, "/v1/library/"+storeBook+"/state", got.path)
	assert.Equal(t, body, got.body)

	rec = f.serve(http.MethodGet, f.statePath(storeBook))
	assert.Equal(t, http.StatusOK, rec.Code)
	got = receive(t, calls)
	assert.Equal(t, http.MethodGet, got.method)
	assert.Equal(t, "/v1/library/"+storeBook+"/state", got.path)

	assert.Empty(t, f.status(), "a store book's state changes nothing in Shisho")
}

// After reading on in the web reader, the next sync sends the Kobo the
// nearest span in the KePub it holds, in the chapter the web reader is in,
// once.
func TestKoboReadingState_SyncSendsAWebReaderUpdate(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	plain, err := reflowable.Open(f.ctx, f.fileA.Filepath, false)
	require.NoError(t, err)
	start, err := plain.FromXPointer("/body/DocFragment[4]/body/section/p[3]/text().20")
	require.NoError(t, err)
	cfi := plain.CFI(start)
	_, err = f.reading.UpdatePosition(f.ctx, reading.PositionUpdate{
		UserID: f.user.ID, FileID: f.fileA.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: 0.62,
	})
	require.NoError(t, err)

	states := f.changedStates()
	require.Len(t, states, 1)
	state := states[0]
	assert.Equal(t, f.entry, state.EntitlementID)
	require.NotNil(t, state.StatusInfo)
	assert.Equal(t, koboStatusReading, state.StatusInfo.Status)
	require.NotNil(t, state.CurrentBookmark)
	assert.InDelta(t, 62, *state.CurrentBookmark.ProgressPercent, 0.001)
	location := state.CurrentBookmark.Location
	require.NotNil(t, location)
	assert.Equal(t, "KoboSpan", location.Type)
	assert.Equal(t, "OEBPS/text/two.xhtml", location.Source)

	kepubPath := filepath.Join(t.TempDir(), "handoff.kepub.epub")
	require.NoError(t, kepub.NewConverter().ConvertEPUB(f.ctx, f.fileA.Filepath, kepubPath))
	delivered, err := reflowable.Open(f.ctx, kepubPath, true)
	require.NoError(t, err)
	landed, err := delivered.FromKoboSpan(reflowable.KoboSpan{Source: location.Source, ID: location.Value})
	require.NoError(t, err)
	assert.Equal(t, start.Spine, landed.Spine)
	assert.InDelta(t, start.Offset, landed.Offset, 100)

	assert.Empty(t, f.changedStates(), "an unchanged state isn't sent again")
}

// A state the Kobo sent itself isn't sent back to it.
func TestKoboReadingState_SyncDoesNotEchoTheKobo(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	f.put(koboState{status: "Reading", at: time.Now().Add(-time.Minute), minutes: minutes(3), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 8})

	assert.Empty(t, f.changedStates())
}

// Marking a book read in Shisho shows it finished on the Kobo after a sync.
func TestKoboReadingState_SyncSendsAFinishFromShisho(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	f.put(koboState{status: "Reading", at: time.Now().Add(-time.Hour), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 8})
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, f.bookA.ID, models.ReadThroughOutcomeFinished, time.Now().Format(time.DateOnly)))

	states := f.changedStates()
	require.Len(t, states, 1)
	require.NotNil(t, states[0].StatusInfo)
	assert.Equal(t, koboStatusFinished, states[0].StatusInfo.Status)
	require.NotNil(t, states[0].CurrentBookmark.Location)
	assert.Equal(t, "kobo.2.1", states[0].CurrentBookmark.Location.Value, "the Kobo's own place comes back as it was")
}

// A book first synced to a Kobo after reading it elsewhere arrives with its
// reading state.
func TestKoboReadingState_NewBooksArriveWithTheirState(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	readingService := reading.NewService(f.db)
	cfi := "epubcfi(/6/4!/4/2/1:0)"
	_, err := readingService.UpdatePosition(context.Background(), reading.PositionUpdate{
		UserID: f.user.ID, FileID: f.fileA.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: 0.3,
	})
	require.NoError(t, err)

	items, _ := f.syncRaw(t, "")
	entryA, entryB := f.entryID(t, f.fileA.ID), f.entryID(t, f.fileB.ID)
	found := map[string]*ReadingState{}
	for _, item := range items {
		var wrapper struct {
			BookEntitlement struct {
				ID string `json:"Id"`
			}
			ReadingState *ReadingState
		}
		require.NoError(t, json.Unmarshal(item["NewEntitlement"], &wrapper))
		found[wrapper.BookEntitlement.ID] = wrapper.ReadingState
	}
	require.Contains(t, found, entryA)
	require.NotNil(t, found[entryA])
	assert.Equal(t, koboStatusReading, found[entryA].StatusInfo.Status)
	assert.InDelta(t, 30, *found[entryA].CurrentBookmark.ProgressPercent, 0.001)
	require.Contains(t, found, entryB)
	assert.Nil(t, found[entryB], "a book never read goes out without a state")
}

// Minutes reported with neither a place nor a status still make a session,
// and it names the Kobo too.
func TestKoboReadingState_ATimeOnlySessionNamesTheKobo(t *testing.T) {
	t.Parallel()
	f := newReadingStateFixture(t)
	at := time.Now().Add(-time.Hour).Truncate(time.Second)
	f.put(koboState{status: "Reading", at: at, minutes: minutes(10), source: "OEBPS/text/one.xhtml", span: "kobo.2.1", progress: 10})
	f.put(koboState{at: at.Add(30 * time.Minute), minutes: minutes(20)})

	sessions := f.sessions()
	require.Len(t, sessions, 2, "one session per report that added minutes")
	last := sessions[1]
	assert.Equal(t, 10*time.Minute, last.EndedAt.Sub(last.StartedAt))
	assert.Equal(t, &f.device.ID, last.SourceDeviceID)
}
