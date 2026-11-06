package koreader

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Places in the handoff EPUB (internal/testgen), as each reader writes them:
// the start of "Beta" in chapter one, taken from foliate and from KOReader
// through the harness (see pkg/reflowable's tests).
const (
	kepubHash        = "4f0c1a7e9d2b3c5a6e8f0a1b2c3d4e5f"
	betaCFI          = "epubcfi(/6/4!/4/2/4/1:67)"
	betaXPointer     = "/body/DocFragment[2]/body/section/p[1]/text().58"
	betaKePubPointer = "/body/DocFragment[2]/body/div/div/section/p[1]/span[3]/text().0"
)

// addHandoffEPUB adds a Book whose File is the handoff EPUB on disk, mapped
// from epubHash as itself and kepubHash as its KePub.
func (f *syncFixture) addHandoffEPUB() *models.File {
	f.t.Helper()
	file := f.addFile(f.lib, models.FileTypeEPUB, epubHash)
	path := testgen.GenerateHandoffEPUB(f.t, f.t.TempDir(), "handoff.epub")
	_, err := f.db.NewUpdate().Model(file).Set("filepath = ?", path).WherePK().Exec(f.ctx)
	require.NoError(f.t, err)
	file.Filepath = path
	require.NoError(f.t, f.hashes.RecordDocumentHash(f.ctx, file.ID, models.FileTypeKePub, kepubHash))
	return file
}

func (f *syncFixture) progress(document string) ProgressResponse {
	f.t.Helper()
	rec := f.pull(document)
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var out ProgressResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

// webReaderAtBeta saves the web reader's place at the start of "Beta".
func (f *syncFixture) webReaderAtBeta(file *models.File, percentage float64) {
	f.t.Helper()
	cfi := betaCFI
	_, err := f.reading.UpdatePosition(f.ctx, reading.PositionUpdate{
		UserID: f.user.ID, FileID: file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: percentage,
	})
	require.NoError(f.t, err)
}

func (f *syncFixture) sessions(file *models.File) []*models.ReadingSession {
	f.t.Helper()
	var sessions []*models.ReadingSession
	err := f.db.NewSelect().Model(&sessions).Where("rs.user_id = ? AND rs.file_id = ?", f.user.ID, file.ID).OrderExpr("rs.id").Scan(f.ctx)
	require.NoError(f.t, err)
	return sessions
}

// backdatePull moves the Device's latest pull of document back by ago, as if
// it came that long before the push that follows.
func (f *syncFixture) backdatePull(device *devices.Device, document string, ago time.Duration) {
	f.t.Helper()
	res, err := f.db.NewUpdate().Model((*models.KOReaderPull)(nil)).
		Set("pulled_at = ?", time.Now().Add(-ago)).
		Where("device_id = ? AND document = ?", device.ID, document).Exec(f.ctx)
	require.NoError(f.t, err)
	n, err := res.RowsAffected()
	require.NoError(f.t, err)
	require.Equal(f.t, int64(1), n, "no pull to backdate")
}

// agePushes makes the pushes so far an hour older, so the next one isn't
// taken for part of a replay burst with them.
func (f *syncFixture) agePushes() {
	f.t.Helper()
	_, err := f.db.NewUpdate().Model((*models.ReadingPositionUpdate)(nil)).
		Set("created_at = ?", time.Now().Add(-time.Hour)).
		Where("user_id = ?", f.user.ID).Exec(f.ctx)
	require.NoError(f.t, err)
}

// A pull after the web reader moved gets the web reader's place, translated
// for the copy KOReader holds and labelled with the web reader's identity, so
// KOReader offers the jump.
func TestHandoff_PullTranslatesTheWebReadersPosition(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addHandoffEPUB()
	f.webReaderAtBeta(file, 0.12)

	for document, want := range map[string]string{epubHash: betaXPointer, kepubHash: betaKePubPointer} {
		got := f.progress(document)
		assert.Equal(t, want, got.Progress, document)
		assert.Equal(t, document, got.Document)
		require.NotNil(t, got.Percentage)
		assert.InDelta(t, 0.12, *got.Percentage, 1e-9)
		assert.Equal(t, "Shisho web reader", got.Device)
		assert.Equal(t, models.ReadingSourceWebReader, got.DeviceID)
		assert.NotNil(t, got.Timestamp)
	}
}

// A position another KOReader Device pushed from its KePub reaches this
// Device's EPUB in the EPUB's form, under the other Device's name.
func TestHandoff_PullTranslatesAnotherDevicesKePubPosition(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addHandoffEPUB()
	phone, err := f.devices.Create(f.ctx, f.user.ID, "Phone KOReader", devices.DeviceTypeKOReader)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, f.pushAs(phone, kepubHash, betaKePubPointer, 0.12).Code)
	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	require.NotNil(t, position.LocatorFileType)
	assert.Equal(t, models.FileTypeKePub, *position.LocatorFileType)

	got := f.progress(epubHash)
	assert.Equal(t, betaXPointer, got.Progress)
	assert.Equal(t, "Phone KOReader", got.Device)
	assert.Equal(t, phone.ID, got.DeviceID)

	// The Device that pushed still gets exactly what it sent, as its own.
	own := f.pullAs(phone, kepubHash)
	assert.Contains(t, own.Body.String(), betaKePubPointer)
	assert.Contains(t, own.Body.String(), koreaderDeviceID)
}

// A Device's position in its other copy of the File comes back in this
// copy's form, still as its own, so KOReader doesn't offer a jump to itself.
func TestHandoff_SameDeviceOtherCopyIsTranslated(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	f.addHandoffEPUB()
	require.Equal(t, http.StatusOK, f.push(kepubHash, betaKePubPointer, 0.12).Code)
	got := f.progress(epubHash)
	assert.Equal(t, betaXPointer, got.Progress)
	assert.Equal(t, koreaderDeviceID, got.DeviceID)
}

// A pull, and the pushes of the same document from the same Device within six
// hours, are a Reading Session: estimated, from the pull to the last push, as
// KOReader also pushes every few pages while you read.
func TestHandoff_PullThenPushIsASession(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addHandoffEPUB()
	f.webReaderAtBeta(file, 0.12)

	f.pull(epubHash)
	f.backdatePull(f.device, epubHash, 20*time.Minute)
	pulledAt := time.Now().Add(-20 * time.Minute)
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.3).Code)

	sessions := f.sessions(file)
	require.Len(t, sessions, 1)
	s := sessions[0]
	assert.False(t, s.Measured, "KOReader's sessions are estimates")
	assert.Equal(t, models.ReadingSourceKOReader, s.Source)
	assert.WithinDuration(t, pulledAt, s.StartedAt, 5*time.Second)
	assert.WithinDuration(t, time.Now(), s.EndedAt, 5*time.Second)
	assert.InDelta(t, 0.12, s.StartPercentage, 1e-9)
	assert.InDelta(t, 0.3, s.EndPercentage, 1e-9)
	assert.NotNil(t, s.ReadThroughID)

	// A later push, such as the one on closing the book, moves its end.
	f.agePushes()
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.35).Code)
	sessions = f.sessions(file)
	require.Len(t, sessions, 1)
	assert.InDelta(t, 0.35, sessions[0].EndPercentage, 1e-9)
	assert.WithinDuration(t, pulledAt, sessions[0].StartedAt, 5*time.Second)

	// The next pull starts another.
	f.pull(epubHash)
	f.backdatePull(f.device, epubHash, 5*time.Minute)
	f.agePushes()
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.4).Code)
	assert.Len(t, f.sessions(file), 2)
}

func TestHandoff_NoSessionWithoutAPairWithinSixHours(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addHandoffEPUB()
	phone, err := f.devices.Create(f.ctx, f.user.ID, "Phone KOReader", devices.DeviceTypeKOReader)
	require.NoError(t, err)

	// A push with no pull before it.
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.3).Code)
	// A pull too long before the push.
	f.pull(epubHash)
	f.backdatePull(f.device, epubHash, 7*time.Hour)
	f.agePushes()
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.4).Code)
	// Another Device's pull, or a pull of another copy.
	f.agePushes()
	f.pullAs(phone, epubHash)
	f.backdatePull(phone, epubHash, time.Hour)
	f.pull(kepubHash)
	f.backdatePull(f.device, kepubHash, time.Hour)
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.5).Code)

	assert.Empty(t, f.sessions(file))
}

// Pushes KOReader queued offline arrive together when it reconnects. They
// apply in order, but they're a replay of reading done earlier, not reading
// now, so none of them is a session, even one that follows a pull.
func TestHandoff_ReplayBurstCreatesNoSessions(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addHandoffEPUB()
	other := f.addFile(f.lib, models.FileTypeCBZ, cbzHash)

	f.pull(epubHash)
	f.backdatePull(f.device, epubHash, time.Hour)
	f.pull(cbzHash)
	f.backdatePull(f.device, cbzHash, time.Hour)
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.3).Code)
	require.Equal(t, http.StatusOK, f.push(cbzHash, "4", 0.4).Code)
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.35).Code)

	assert.Empty(t, f.sessions(file))
	assert.Empty(t, f.sessions(other))
	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	assert.InDelta(t, 0.35, position.Percentage, 1e-9, "the last of the burst is the position")
	comic, err := f.reading.RetrievePosition(f.ctx, f.user.ID, other.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, *comic.Page)
}

// A push queued offline may be older than it looks, so a push from another
// reader can't undo a finish, even on its own, as a queue of one drains.
func TestHandoff_AnotherReaderCannotUndoAFinish(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addHandoffEPUB()
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.3).Code)
	f.agePushes()
	end := "epubcfi(/6/8!/4/2/1:0)"
	_, err := f.reading.UpdatePosition(f.ctx, reading.PositionUpdate{
		UserID: f.user.ID, FileID: file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &end}, Percentage: 1, AtEnd: true,
	})
	require.NoError(t, err)
	require.Equal(t, models.ReadThroughOutcomeFinished, f.status(file))

	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.5).Code)
	assert.Equal(t, models.ReadThroughOutcomeFinished, f.status(file))
}

// A pull a replay used up makes no session with a push hours later, which
// would count the time spent offline.
func TestHandoff_ReplayUsesUpItsPulls(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addHandoffEPUB()
	f.addFile(f.lib, models.FileTypeCBZ, cbzHash)
	f.pull(epubHash)
	f.backdatePull(f.device, epubHash, 2*time.Hour)
	require.Equal(t, http.StatusOK, f.push(cbzHash, "4", 0.4).Code)
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.3).Code)
	f.agePushes()
	require.Equal(t, http.StatusOK, f.push(epubHash, betaXPointer, 0.4).Code)
	assert.Empty(t, f.sessions(file))
}

// A push carrying a timestamp is dated by it, so a queued update can't pass
// for newer than what happened since.
func TestHandoff_TimestampDatesThePush(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addHandoffEPUB()
	at := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	body := fmt.Sprintf(`{"device_id":%q,"progress":%q,"document":%q,"percentage":0.3,"device":%q,"timestamp":%d}`,
		koreaderDeviceID, betaXPointer, epubHash, koreaderDevice, at.Unix())
	require.Equal(t, http.StatusOK, f.serve(http.MethodPut, "/koreader/syncs/progress", f.user.Username, f.device.Credential, body).Code)
	position, err := f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	assert.True(t, at.Equal(position.OccurredAt), "dated %v, not %v", position.OccurredAt, at)

	// The web reader has moved on since, so an older queued push doesn't
	// take the place back.
	f.webReaderAtBeta(file, 0.5)
	body = fmt.Sprintf(`{"device_id":%q,"progress":%q,"document":%q,"percentage":0.4,"device":%q,"timestamp":%d}`,
		koreaderDeviceID, betaXPointer, epubHash, koreaderDevice, time.Now().Add(-time.Hour).Unix())
	require.Equal(t, http.StatusOK, f.serve(http.MethodPut, "/koreader/syncs/progress", f.user.Username, f.device.Credential, body).Code)
	position, err = f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	assert.InDelta(t, 0.5, position.Percentage, 1e-9)
	assert.Equal(t, models.ReadingSourceWebReader, position.Source)

	// A clock running fast can't date a push, or end its session, later
	// than it arrived.
	f.pull(epubHash)
	f.backdatePull(f.device, epubHash, 10*time.Minute)
	f.agePushes()
	body = fmt.Sprintf(`{"device_id":%q,"progress":%q,"document":%q,"percentage":0.6,"device":%q,"timestamp":%d}`,
		koreaderDeviceID, betaXPointer, epubHash, koreaderDevice, time.Now().Add(3*time.Hour).Unix())
	require.Equal(t, http.StatusOK, f.serve(http.MethodPut, "/koreader/syncs/progress", f.user.Username, f.device.Credential, body).Code)
	position, err = f.reading.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(t, err)
	assert.False(t, position.OccurredAt.After(time.Now()))
	sessions := f.sessions(file)
	require.Len(t, sessions, 1)
	assert.False(t, sessions[0].EndedAt.After(time.Now()))

	// A timestamp of nothing is refused, rather than dating a push to 1970.
	body = fmt.Sprintf(`{"progress":%q,"document":%q,"percentage":0.4,"timestamp":0}`, betaXPointer, epubHash)
	assert.Equal(t, http.StatusUnprocessableEntity, f.serve(http.MethodPut, "/koreader/syncs/progress", f.user.Username, f.device.Credential, body).Code)
}

// A File that can't be read, such as one moved away since the scan, still
// syncs, with no translated position to give.
func TestHandoff_UnreadableFileHasNoTranslation(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	file := f.addHandoffEPUB()
	f.webReaderAtBeta(file, 0.12)
	_, err := f.db.NewUpdate().Model(file).Set("filepath = ?", filepath.Join(t.TempDir(), "gone.epub")).WherePK().Exec(f.ctx)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, f.pull(epubHash).Body.String())
}
