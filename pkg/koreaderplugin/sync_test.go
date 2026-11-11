package koreaderplugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	syncHash  = "dcc8fb3ab2c17a8bc3e90312f91c0823"
	pageHash  = "a07ac168355942aaab99a3883774b0f3"
	xpointerA = "/body/DocFragment[2]/body/p[20]/text().371"
)

// syncFixture is a reader whose KOReader Device has existed for a while,
// with one Book holding an EPUB the Device's copy hashes to syncHash.
type syncFixture struct {
	*pluginFixture
	reading *reading.Service
	hashes  *fingerprints.Service
	lib     *models.Library
	file    *models.File
	// day1 is 10:00 UTC three days ago.
	day1 time.Time
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	f := &syncFixture{pluginFixture: newPluginFixture(t)}
	f.reading = reading.NewService(f.db)
	f.hashes = fingerprints.NewService(f.db)
	f.lib = &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := f.db.NewInsert().Model(f.lib).Exec(f.ctx)
	require.NoError(t, err)
	f.grantAll(f.user)
	f.backdate(f.device, time.Now().AddDate(0, -1, 0))
	f.file = f.addFile(models.FileTypeEPUB, syncHash)
	f.day1 = time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -3).Add(10 * time.Hour)
	return f
}

func (f *syncFixture) grantAll(user *models.User) {
	f.t.Helper()
	_, err := f.db.NewInsert().Model(&models.UserLibraryAccess{UserID: user.ID}).Exec(f.ctx)
	require.NoError(f.t, err)
}

// backdate makes device created at at, as one set up a while ago.
func (f *syncFixture) backdate(device *devices.Device, at time.Time) {
	f.t.Helper()
	_, err := f.db.NewUpdate().Model(device).Set("created_at = ?", at).WherePK().Exec(f.ctx)
	require.NoError(f.t, err)
}

func (f *syncFixture) addFile(fileType, hash string) *models.File {
	f.t.Helper()
	title := fmt.Sprintf("%s-%s", fileType, hash)
	book := &models.Book{
		LibraryID: f.lib.ID, Title: title, TitleSource: models.DataSourceFilepath, SortTitle: title,
		SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/books/" + title,
	}
	_, err := f.db.NewInsert().Model(book).Exec(f.ctx)
	require.NoError(f.t, err)
	file := &models.File{
		LibraryID: f.lib.ID, BookID: book.ID, FileType: fileType, FileRole: models.FileRoleMain,
		Filepath: fmt.Sprintf("/books/%s/book.%s", title, fileType), FilesizeBytes: 1,
	}
	_, err = f.db.NewInsert().Model(file).Exec(f.ctx)
	require.NoError(f.t, err)
	require.NoError(f.t, f.hashes.RecordDocumentHash(f.ctx, file.ID, fileType, hash))
	return file
}

// pages is a minute on each of pages from..to of 100, starting at start.
func pages(document string, from, to int, start time.Time) []PageRowPayload {
	var rows []PageRowPayload
	for p := from; p <= to; p++ {
		rows = append(rows, PageRowPayload{
			Document: document, Page: p, TotalPages: 100,
			StartTime: start.Add(time.Duration(p-from) * time.Minute).Unix(), Duration: 60,
		})
	}
	return rows
}

func (f *syncFixture) uploadAs(device *devices.Device, payload UploadPayload) *httptest.ResponseRecorder {
	f.t.Helper()
	if payload.Clock == 0 {
		payload.Clock = time.Now().Unix()
	}
	body, err := json.Marshal(payload)
	require.NoError(f.t, err)
	user := f.user
	if device.UserID != f.user.ID {
		user = &models.User{}
		require.NoError(f.t, f.db.NewSelect().Model(user).Where("id = ?", device.UserID).Scan(f.ctx))
	}
	return f.do(request{path: "/koreader/plugin/upload", body: string(body), username: user.Username, credential: device.Credential})
}

func (f *syncFixture) upload(payload UploadPayload) UploadResponse {
	f.t.Helper()
	rec := f.uploadAs(f.device, payload)
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp UploadResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func (f *syncFixture) pullAs(device *devices.Device, clock int64, documents ...string) PullResponse {
	f.t.Helper()
	body, err := json.Marshal(PullPayload{Clock: clock, Documents: documents})
	require.NoError(f.t, err)
	user := &models.User{}
	require.NoError(f.t, f.db.NewSelect().Model(user).Where("id = ?", device.UserID).Scan(f.ctx))
	rec := f.do(request{path: "/koreader/plugin/pull", body: string(body), username: user.Username, credential: device.Credential})
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp PullResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func (f *syncFixture) sessions(user *models.User) []*models.ReadingSession {
	f.t.Helper()
	var sessions []*models.ReadingSession
	require.NoError(f.t, f.db.NewSelect().Model(&sessions).Where("rs.user_id = ?", user.ID).OrderExpr("rs.started_at").Scan(f.ctx))
	return sessions
}

func (f *syncFixture) readThroughs(file *models.File) []*models.ReadThrough {
	f.t.Helper()
	var rts []*models.ReadThrough
	require.NoError(f.t, f.db.NewSelect().Model(&rts).Where("rt.user_id = ? AND rt.book_id = ?", f.user.ID, file.BookID).OrderExpr("rt.id").Scan(f.ctx))
	return rts
}

// position is a position event at xpointerA, percentage through the book.
func position(id string, at time.Time, percentage float64) EventPayload {
	return EventPayload{ID: id, Type: PluginEventPosition, Document: syncHash, At: at.Unix(), Progress: xpointerA, Percentage: &percentage}
}

func TestUpload_GroupsPageRowsIntoMeasuredSessions(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	day2 := f.day1.Add(24 * time.Hour)

	var rows []PageRowPayload
	// Ten minutes, then a gap of twenty, then five more: two sessions.
	rows = append(rows, pages(syncHash, 2, 11, f.day1)...)
	rows = append(rows, pages(syncHash, 12, 16, f.day1.Add(30*time.Minute))...)
	// A gap of exactly ten minutes doesn't split a session.
	rows = append(rows, pages(syncHash, 30, 32, day2)...)
	rows = append(rows, pages(syncHash, 33, 34, day2.Add(13*time.Minute))...)
	// A row read for longer than a minute.
	rows[0].Duration = 300
	resp := f.upload(UploadPayload{Pages: rows})
	assert.Equal(t, len(rows), resp.Pages)

	sessions := f.sessions(f.user)
	require.Len(t, sessions, 3)
	first := sessions[0]
	assert.True(t, first.Measured)
	assert.Equal(t, models.ReadingSourceKOReader, first.Source)
	assert.Equal(t, f.device.ID, *first.SourceDeviceID)
	assert.Equal(t, f.file.ID, *first.FileID)
	assert.True(t, first.StartedAt.Equal(f.day1))
	// The last row starts nine minutes in and lasts a minute.
	assert.True(t, first.EndedAt.Equal(f.day1.Add(10*time.Minute)), "ended %s", first.EndedAt)
	assert.Equal(t, 300+9*60, first.Duration, "the sum of its rows, not its span")
	assert.InDelta(t, 0.02, first.StartPercentage, 1e-9)
	assert.InDelta(t, 0.11, first.EndPercentage, 1e-9)

	assert.True(t, sessions[1].StartedAt.Equal(f.day1.Add(30*time.Minute)))
	assert.Equal(t, 5*60, sessions[1].Duration)
	assert.True(t, sessions[2].StartedAt.Equal(day2))
	assert.Equal(t, 5*60, sessions[2].Duration)
	assert.InDelta(t, 0.34, sessions[2].EndPercentage, 1e-9)

	// The Book was never read, so the first session started a Read-through
	// on its own day, and every session belongs to it.
	rts := f.readThroughs(f.file)
	require.Len(t, rts, 1)
	assert.Equal(t, f.day1.Format(time.DateOnly), *rts[0].BeginDay)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, rts[0].Outcome)
	for _, s := range sessions {
		require.NotNil(t, s.ReadThroughID)
		assert.Equal(t, rts[0].ID, *s.ReadThroughID)
	}
}

func TestUpload_RowsNextToAnEarlierUploadExtendItsSession(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.upload(UploadPayload{Pages: pages(syncHash, 2, 6, f.day1)})
	f.upload(UploadPayload{Pages: pages(syncHash, 7, 9, f.day1.Add(8*time.Minute))})

	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	assert.Equal(t, 8*60, sessions[0].Duration)
	assert.True(t, sessions[0].EndedAt.Equal(f.day1.Add(11*time.Minute)))
	assert.InDelta(t, 0.09, sessions[0].EndPercentage, 1e-9)

	// Rows filling the gap between two sessions join them.
	f.upload(UploadPayload{Pages: pages(syncHash, 20, 21, f.day1.Add(30*time.Minute))})
	require.Len(t, f.sessions(f.user), 2)
	f.upload(UploadPayload{Pages: pages(syncHash, 10, 19, f.day1.Add(19*time.Minute))})
	sessions = f.sessions(f.user)
	require.Len(t, sessions, 1)
	assert.Equal(t, (8+2+10)*60, sessions[0].Duration)
	assert.InDelta(t, 0.21, sessions[0].EndPercentage, 1e-9)
}

func TestUpload_RepeatsAreIgnored(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	batch := UploadPayload{Pages: pages(syncHash, 2, 6, f.day1)}
	f.upload(batch)
	f.upload(batch)
	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	assert.Equal(t, 5*60, sessions[0].Duration)
}

func TestUpload_RepeatsStayIgnoredAfterTheUserDeletesTheSession(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	batch := UploadPayload{Pages: pages(syncHash, 2, 6, f.day1)}
	f.upload(batch)
	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	require.NoError(t, f.reading.DeleteSession(f.ctx, f.user.ID, sessions[0].ID))

	f.upload(batch)

	assert.Empty(t, f.sessions(f.user), "a deleted session stays deleted")
}

func TestUpload_RepeatsStayIgnoredAfterTheUserDeletesTheReadThrough(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	rows := pages(syncHash, 2, 6, f.day1)
	place := position("pos-1", f.day1.Add(5*time.Minute), 0.06)
	batch := UploadPayload{Pages: rows, Events: []EventPayload{place}}
	f.upload(batch)
	rts := f.readThroughs(f.file)
	require.Len(t, rts, 1)
	require.Equal(t, models.ReadThroughOutcomeUnderway, rts[0].Outcome)
	// Deleting the read underway takes the Book back to before it started.
	require.NoError(t, f.reading.DeleteReadThrough(f.ctx, rts[0]))
	require.Empty(t, f.sessions(f.user))

	resp := f.upload(batch)

	assert.Equal(t, []string{"pos-1"}, resp.Events, "acknowledged")
	assert.Empty(t, f.sessions(f.user))
	assert.Empty(t, f.readThroughs(f.file), "neither its rows nor its position start it again")
	stored, err := f.reading.RetrievePosition(f.ctx, f.user.ID, f.file.ID)
	require.NoError(t, err)
	assert.Nil(t, stored)
}

func TestUpload_ShiftsABatchFromAClockThatIsOff(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	// The device's clock is an hour slow: it read at day1 by its own clock,
	// an hour later in fact.
	slow := time.Now().Add(-time.Hour).Unix()
	resp := f.upload(UploadPayload{Clock: slow, Pages: pages(syncHash, 2, 3, f.day1)})
	assert.InDelta(t, 3600, resp.ClockOffset, 5)

	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	assert.WithinDuration(t, f.day1.Add(time.Hour), sessions[0].StartedAt, 5*time.Second)

	// A few minutes off is left alone.
	other := f.addFile(models.FileTypeEPUB, "b2c8fb3ab2c17a8bc3e90312f91c0824")
	f.upload(UploadPayload{Clock: time.Now().Add(-2 * time.Minute).Unix(), Pages: pages("b2c8fb3ab2c17a8bc3e90312f91c0824", 2, 3, f.day1)})
	var unshifted models.ReadingSession
	require.NoError(t, f.db.NewSelect().Model(&unshifted).Where("rs.file_id = ?", other.ID).Scan(f.ctx))
	assert.True(t, unshifted.StartedAt.Equal(f.day1))
}

func TestUpload_RefusesImpossibleTimesAndClampsFutureOnes(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	created := time.Now().AddDate(0, 0, -10)
	f.backdate(f.device, created)

	var rows []PageRowPayload
	// A clock reset to 1970, and a time before the Device existed.
	rows = append(rows, pages(syncHash, 2, 3, time.Unix(3600, 0))...)
	rows = append(rows, pages(syncHash, 2, 3, created.Add(-24*time.Hour))...)
	// A fast clock can't date reading after it arrived.
	rows = append(rows, pages(syncHash, 50, 50, time.Now().Add(30*time.Minute+time.Hour*24))...)
	resp := f.upload(UploadPayload{Pages: rows, Clock: time.Now().Unix()})
	assert.Equal(t, len(rows), resp.Pages, "refused rows are acknowledged")

	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	assert.WithinDuration(t, time.Now(), sessions[0].StartedAt, time.Minute)
}

func TestUpload_PlacesASessionByItsTime(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	// A Read-through started today on the web: reading from three days ago
	// arriving now didn't happen during it.
	today := time.Now().UTC().Format(time.DateOnly)
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, f.file.BookID, today))

	f.upload(UploadPayload{Pages: pages(syncHash, 2, 6, f.day1)})

	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	assert.Nil(t, sessions[0].ReadThroughID, "it counts toward time on no Read-through")
	require.Len(t, f.readThroughs(f.file), 1, "nor does it start one")

	// Reading today belongs to the one underway.
	f.upload(UploadPayload{Pages: pages(syncHash, 7, 8, time.Now().Add(-5*time.Minute))})
	sessions = f.sessions(f.user)
	require.Len(t, sessions, 2)
	require.NotNil(t, sessions[1].ReadThroughID)
	assert.Equal(t, f.readThroughs(f.file)[0].ID, *sessions[1].ReadThroughID)
}

func TestUpload_SessionsLandOnTheReadThroughUnderwayThen(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	// Read and finished a week ago, and being reread since yesterday.
	weekAgo := time.Now().UTC().AddDate(0, 0, -7)
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, f.file.BookID, weekAgo.AddDate(0, 0, -3).Format(time.DateOnly)))
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, f.file.BookID, models.ReadThroughOutcomeFinished, weekAgo.Format(time.DateOnly)))
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, f.file.BookID, yesterday))
	rts := f.readThroughs(f.file)
	require.Len(t, rts, 2)

	f.upload(UploadPayload{Pages: pages(syncHash, 2, 4, weekAgo.Add(-24*time.Hour))})

	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	require.NotNil(t, sessions[0].ReadThroughID)
	assert.Equal(t, rts[0].ID, *sessions[0].ReadThroughID, "the read underway then, not the reread")
}

func TestUpload_PositionsTakeTheirOwnTime(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	at := f.day1.Add(10 * time.Minute)
	resp := f.upload(UploadPayload{
		Pages:  pages(syncHash, 2, 11, f.day1),
		Events: []EventPayload{position("pos-1", at, 0.11)},
	})
	assert.Equal(t, []string{"pos-1"}, resp.Events)

	stored, err := f.reading.RetrievePosition(f.ctx, f.user.ID, f.file.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, xpointerA, *stored.XPointer)
	assert.InDelta(t, 0.11, stored.Percentage, 1e-9)
	assert.True(t, stored.OccurredAt.Equal(at), "occurred %s", stored.OccurredAt)
	assert.Equal(t, f.device.ID, *stored.SourceDeviceID)
	require.Len(t, f.readThroughs(f.file), 1, "the session started it; the position doesn't start another")
}

func TestUpload_AnOlderOfflinePositionKeepsItsSessionButNotThePlace(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	// The web reader was further on yesterday.
	webAt := time.Now().Add(-24 * time.Hour)
	_, err := f.reading.UpdatePosition(f.ctx, reading.PositionUpdate{
		UserID: f.user.ID, FileID: f.file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: ptr("epubcfi(/6/8!/4/2)")}, Percentage: 0.5, OccurredAt: &webAt,
	})
	require.NoError(t, err)

	f.upload(UploadPayload{
		Pages:  pages(syncHash, 2, 11, f.day1),
		Events: []EventPayload{position("pos-old", f.day1.Add(10*time.Minute), 0.11)},
	})

	stored, err := f.reading.RetrievePosition(f.ctx, f.user.ID, f.file.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ReadingSourceWebReader, stored.Source, "the newer place stands")
	assert.InDelta(t, 0.5, stored.Percentage, 1e-9)
	assert.Len(t, f.sessions(f.user), 1, "the reading still counts")
}

func TestUpload_UnrecognizedDocumentsAreAcknowledged(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	unknown := "ffffffffffffffffffffffffffffffff"
	resp := f.upload(UploadPayload{
		Pages:  pages(unknown, 2, 3, f.day1),
		Events: []EventPayload{{ID: "x", Type: PluginEventPosition, Document: unknown, At: f.day1.Unix(), Progress: xpointerA, Percentage: ptr(0.1)}},
	})
	assert.Equal(t, 2, resp.Pages)
	assert.Equal(t, []string{"x"}, resp.Events)
	assert.Empty(t, f.sessions(f.user))
	held, err := f.db.NewSelect().Model((*models.KOReaderPageRow)(nil)).Where("document = ? AND file_id IS NULL AND NOT grouped", unknown).Count(f.ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 2, held, "kept until the copy is linked")
}

func TestUpload_RecordsThePluginSync(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.upload(UploadPayload{})
	device, err := f.devices.Retrieve(f.ctx, f.user.ID, f.device.ID)
	require.NoError(t, err)
	require.NotNil(t, device.PluginLastSyncedAt)
	assert.WithinDuration(t, time.Now(), *device.PluginLastSyncedAt, time.Minute)
}

func TestUpload_NeedsTheDevicesSignIn(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	rec := f.do(request{path: "/koreader/plugin/upload", body: `{"clock":1}`, username: f.user.Username, credential: "wrong"})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUpload_AnotherUsersReadingIsNeverReachable(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	other := f.addUser("other", true)
	f.grantAll(other)
	otherDevice := f.addDevice(other, devices.DeviceTypeKOReader)
	f.backdate(otherDevice, time.Now().AddDate(0, -1, 0))
	f.upload(UploadPayload{
		Pages:  pages(syncHash, 2, 6, f.day1),
		Events: []EventPayload{position("mine", f.day1.Add(5*time.Minute), 0.06)},
	})

	// The other reader's Device sees none of it.
	pulled := f.pullAs(otherDevice, time.Now().Unix(), syncHash)
	assert.Empty(t, pulled.Positions)
	assert.Empty(t, f.sessions(other))

	// And the same rows from their Device are their own reading.
	rec := f.uploadAs(otherDevice, UploadPayload{Pages: pages(syncHash, 2, 6, f.day1)})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Len(t, f.sessions(other), 1)
	assert.Len(t, f.sessions(f.user), 1)
}

func TestPull_OffersAnotherReadersPositionNamed(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	cbz := f.addFile(models.FileTypeCBZ, pageHash)
	at := time.Now().Add(-time.Hour)
	_, err := f.reading.UpdatePosition(f.ctx, reading.PositionUpdate{
		UserID: f.user.ID, FileID: cbz.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{Page: ptr(4)}, Percentage: 0.25, OccurredAt: &at,
	})
	require.NoError(t, err)

	// The device's clock is two hours fast.
	clock := time.Now().Add(2 * time.Hour).Unix()
	pulled := f.pullAs(f.device, clock, pageHash, syncHash, "ffffffffffffffffffffffffffffffff")

	require.Len(t, pulled.Positions, 1, "books with no position are left out")
	p := pulled.Positions[0]
	assert.Equal(t, pageHash, p.Document)
	assert.Equal(t, "5", p.Progress, "KOReader's 1-based page")
	assert.InDelta(t, 0.25, p.Percentage, 1e-9)
	assert.Equal(t, "Shisho web reader", p.Source)
	assert.False(t, p.Own)
	assert.InDelta(t, at.Add(2*time.Hour).Unix(), p.Timestamp, 5, "on the device's clock")
}

func TestPull_HandsOffBetweenTwoKOReaderDevices(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	second := f.addDevice(f.user, devices.DeviceTypeKOReader)
	f.backdate(second, time.Now().AddDate(0, -1, 0))
	at := time.Now().Add(-10 * time.Minute)
	f.upload(UploadPayload{Events: []EventPayload{position("p", at, 0.4)}})

	own := f.pullAs(f.device, time.Now().Unix(), syncHash)
	require.Len(t, own.Positions, 1)
	assert.True(t, own.Positions[0].Own)

	pulled := f.pullAs(second, time.Now().Unix(), syncHash)
	require.Len(t, pulled.Positions, 1)
	assert.False(t, pulled.Positions[0].Own)
	assert.Equal(t, f.device.Name, pulled.Positions[0].Source)
	assert.Equal(t, xpointerA, pulled.Positions[0].Progress)
	assert.Equal(t, at.Unix(), pulled.Positions[0].Timestamp)
}

func ptr[T any](v T) *T { return &v }

func TestUpload_RowsNextToADeletedSessionStayDeleted(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.upload(UploadPayload{Pages: pages(syncHash, 2, 6, f.day1)})
	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	require.NoError(t, f.reading.DeleteSession(f.ctx, f.user.ID, sessions[0].ID))

	// The rest of the same sitting, flushed after the first sync.
	f.upload(UploadPayload{Pages: pages(syncHash, 7, 9, f.day1.Add(8*time.Minute))})

	assert.Empty(t, f.sessions(f.user), "the sitting the user deleted doesn't come back in part")
}

func TestUpload_ARowCantEndAfterItArrived(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	row := pages(syncHash, 5, 5, time.Now().Add(-time.Minute))
	row[0].Duration = 7200
	f.upload(UploadPayload{Pages: row})

	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	assert.False(t, sessions[0].EndedAt.After(time.Now()), "ended %s", sessions[0].EndedAt)
	assert.LessOrEqual(t, sessions[0].Duration, 65)
}

func TestUpload_AFirstPageSittingBeforeTheReadStartsBelongsToIt(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	var rows []PageRowPayload
	// Opening the book on its first page, then reading in earnest later
	// the same day.
	rows = append(rows, pages(syncHash, 1, 1, f.day1)...)
	rows = append(rows, pages(syncHash, 2, 5, f.day1.Add(time.Hour))...)
	f.upload(UploadPayload{Pages: rows})

	rts := f.readThroughs(f.file)
	require.Len(t, rts, 1)
	sessions := f.sessions(f.user)
	require.Len(t, sessions, 2)
	for _, s := range sessions {
		require.NotNil(t, s.ReadThroughID)
		assert.Equal(t, rts[0].ID, *s.ReadThroughID)
	}
}
