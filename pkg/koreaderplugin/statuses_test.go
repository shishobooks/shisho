package koreaderplugin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pullStates pulls the states of documents, and of what changed after since.
func (f *syncFixture) pullStates(since int64, documents ...string) PullResponse {
	f.t.Helper()
	body, err := json.Marshal(PullPayload{Clock: time.Now().Unix(), Statuses: documents, Since: since})
	require.NoError(f.t, err)
	rec := f.do(request{path: "/koreader/plugin/pull", body: string(body), username: f.user.Username, credential: f.device.Credential})
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp PullResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// state is the device state the pull gives document, or nil for none.
func (f *syncFixture) state(since int64, document string) *PulledState {
	f.t.Helper()
	for _, s := range f.pullStates(since, document).States {
		if s.Document == document {
			return &s
		}
	}
	return nil
}

// status is the Book's Reading Status in Shisho, "" for none.
func (f *syncFixture) status(file *models.File) string {
	f.t.Helper()
	reading, err := f.reading.RetrieveBookReading(f.ctx, f.user.ID, file.BookID)
	require.NoError(f.t, err)
	if reading.Status == nil {
		return ""
	}
	return *reading.Status
}

func (f *syncFixture) ratingUnits(file *models.File) *int {
	f.t.Helper()
	reading, err := f.reading.RetrieveBookReading(f.ctx, f.user.ID, file.BookID)
	require.NoError(f.t, err)
	return reading.RatingUnits
}

// changedOn makes every change to the Book's Read-throughs and Rating look
// made at at, as one made then would.
func (f *syncFixture) changedOn(file *models.File, at time.Time) {
	f.t.Helper()
	_, err := f.db.NewUpdate().Model((*models.ReadThrough)(nil)).Set("updated_at = ?", at).
		Where("user_id = ? AND book_id = ?", f.user.ID, file.BookID).Exec(f.ctx)
	require.NoError(f.t, err)
	_, err = f.db.NewUpdate().Model((*models.UserBook)(nil)).Set("rating_updated_at = ?", at).
		Where("user_id = ? AND book_id = ? AND rating_updated_at IS NOT NULL", f.user.ID, file.BookID).Exec(f.ctx)
	require.NoError(f.t, err)
}

func statusEvent(id, status, previous string, at time.Time) EventPayload {
	return EventPayload{ID: id, Type: PluginEventStatus, Document: syncHash, At: at.Unix(), Status: status, PreviousStatus: previous}
}

func ratingEvent(id string, stars, previous int, at time.Time) EventPayload {
	return EventPayload{ID: id, Type: PluginEventRating, Document: syncHash, At: at.Unix(), Rating: stars, PreviousRating: previous}
}

func day(t time.Time) string { return t.Format(time.DateOnly) }

func TestStatus_FinishedOnTheDeviceIsAReportedFinish(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, f.file.BookID, day(f.day1)))
	finishedAt := f.day1.Add(48 * time.Hour)

	f.upload(UploadPayload{Events: []EventPayload{statusEvent("s1", KOReaderStatusFinished, KOReaderStatusReading, finishedAt)}})

	assert.Equal(t, models.ReadingStatusRead, f.status(f.file))
	rts := f.readThroughs(f.file)
	require.Len(t, rts, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
	assert.Equal(t, day(finishedAt), *rts[0].EndDay, "ended the day it was finished on the device")
}

func TestStatus_AFinishNeverSyncedFinishesTheReadItsPagesStarted(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	now := time.Now()

	// Read and finished offline, then synced for the first time: the pages
	// start the read, and the finish, with nothing synced before it, ends it.
	f.upload(UploadPayload{
		Pages:  pages(syncHash, 1, 5, now.Add(-time.Hour)),
		Events: []EventPayload{statusEvent("s1", KOReaderStatusFinished, "", now.Add(-30*time.Minute))},
	})

	assert.Equal(t, models.ReadingStatusRead, f.status(f.file))
	assert.Len(t, f.readThroughs(f.file), 1)
}

func TestStatus_TheDevicesOwnReadingInTheSameSyncIsntAChangeInShisho(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, f.file.BookID, day(f.day1)))
	require.NoError(t, f.reading.SetPaused(f.ctx, f.user.ID, f.file.BookID, true))
	f.changedOn(f.file, f.day1)
	now := time.Now()

	// Synced On hold, then read on and marked finished short of the end: the
	// pages resume the read before the finish arrives.
	f.upload(UploadPayload{Events: []EventPayload{
		position("p1", now.Add(-time.Minute), 0.95),
		statusEvent("s1", KOReaderStatusFinished, KOReaderStatusOnHold, now),
	}})

	assert.Equal(t, models.ReadingStatusRead, f.status(f.file))
}

func TestRating_OnASupplementLeavesTheBookAlone(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	supplement := f.addFile(models.FileTypePDF, "supplement")
	_, err := f.db.NewUpdate().Model(supplement).Set("book_id = ?, file_role = ?", f.file.BookID, models.FileRoleSupplement).WherePK().Exec(f.ctx)
	require.NoError(t, err)

	f.upload(UploadPayload{Events: []EventPayload{{ID: "r1", Type: PluginEventRating, Document: "supplement", At: time.Now().Unix(), Rating: 2}}})

	assert.Nil(t, f.ratingUnits(f.file))
}

func TestStatus_AFinishMarkedInKOReaderStaysAfterPagingBack(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, f.file.BookID, day(f.day1)))
	now := time.Now()

	// The last page finishes the book on its own, and KOReader's end-of-book
	// prompt then marks it finished.
	f.upload(UploadPayload{Events: []EventPayload{
		position("p1", now.Add(-5*time.Minute), 1),
		statusEvent("s1", KOReaderStatusFinished, KOReaderStatusReading, now.Add(-4*time.Minute)),
	}})
	require.Equal(t, models.ReadingStatusRead, f.status(f.file))

	// Paging back soon after would undo a finish the reader didn't choose,
	// but this one they did.
	back := position("p2", now.Add(-time.Minute), 0.5)
	back.Progress = "/body/DocFragment[1]/body/p[3]/text().0"
	f.upload(UploadPayload{Events: []EventPayload{back}})
	assert.Equal(t, models.ReadingStatusRead, f.status(f.file))
}

func TestStatus_FinishedWithOnlyADayRecordsAFinishedRead(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	finished := EventPayload{ID: "s1", Type: PluginEventStatus, Document: syncHash, Day: day(f.day1), Status: KOReaderStatusFinished}

	f.upload(UploadPayload{Events: []EventPayload{finished}})

	rts := f.readThroughs(f.file)
	require.Len(t, rts, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
	assert.Nil(t, rts[0].BeginDay, "nothing says when it began")
	assert.Equal(t, day(f.day1), *rts[0].EndDay)
}

func TestStatus_OnHoldPausesAndReadingResumes(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, f.file.BookID, day(f.day1)))
	f.changedOn(f.file, f.day1)

	f.upload(UploadPayload{Events: []EventPayload{statusEvent("s1", KOReaderStatusOnHold, KOReaderStatusReading, f.day1.Add(24*time.Hour))}})
	assert.Equal(t, models.ReadingStatusPaused, f.status(f.file))

	f.upload(UploadPayload{Events: []EventPayload{statusEvent("s2", KOReaderStatusReading, KOReaderStatusOnHold, f.day1.Add(48*time.Hour))}})
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status(f.file))
	assert.Len(t, f.readThroughs(f.file), 1, "resumed, not a new read")
}

func TestStatus_OnHoldWithNothingUnderwayStartsAPausedRead(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)

	f.upload(UploadPayload{Events: []EventPayload{statusEvent("s1", KOReaderStatusOnHold, "", f.day1)}})

	assert.Equal(t, models.ReadingStatusPaused, f.status(f.file))
	rts := f.readThroughs(f.file)
	require.Len(t, rts, 1)
	assert.Equal(t, day(f.day1), *rts[0].BeginDay)
}

func TestStatus_ReadingAFinishedBookStartsAReread(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, f.file.BookID, models.ReadThroughOutcomeFinished, day(f.day1)))
	f.changedOn(f.file, f.day1)
	rereadAt := f.day1.Add(24 * time.Hour)

	// The reread's first pages arrive in the same batch, before the status.
	f.upload(UploadPayload{
		Pages:  pages(syncHash, 1, 5, rereadAt.Add(time.Hour)),
		Events: []EventPayload{statusEvent("s1", KOReaderStatusReading, KOReaderStatusFinished, rereadAt)},
	})

	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status(f.file))
	rts := f.readThroughs(f.file)
	require.Len(t, rts, 2)
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
	assert.Equal(t, day(rereadAt), *rts[1].BeginDay)
	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	require.NotNil(t, sessions[0].ReadThroughID, "the reread's pages are part of it")
	assert.Equal(t, rts[1].ID, *sessions[0].ReadThroughID)
}

func TestStatus_OpeningAFinishedBookChangesNothing(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, f.file.BookID, models.ReadThroughOutcomeFinished, day(f.day1)))

	// KOReader leaves a finished book finished when it's opened, so only
	// pages and a position arrive.
	f.upload(UploadPayload{
		Pages:  pages(syncHash, 40, 45, f.day1.Add(48*time.Hour)),
		Events: []EventPayload{position("p1", f.day1.Add(48*time.Hour+6*time.Minute), 0.45)},
	})

	assert.Equal(t, models.ReadingStatusRead, f.status(f.file))
	assert.Len(t, f.readThroughs(f.file), 1)
	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1, "the minutes still count as reading time")
	assert.Equal(t, KOReaderStatusFinished, f.state(0, syncHash).Status)
}

func TestPull_StatusesMapToKOReaders(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	today := day(time.Now())
	read := f.addFile(models.FileTypeEPUB, "read")
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, read.BookID, models.ReadThroughOutcomeFinished, today))
	paused := f.addFile(models.FileTypeEPUB, "paused")
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, paused.BookID, today))
	require.NoError(t, f.reading.SetPaused(f.ctx, f.user.ID, paused.BookID, true))
	dnf := f.addFile(models.FileTypeEPUB, "dnf")
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, dnf.BookID, today))
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, dnf.BookID, models.ReadThroughOutcomeAbandoned, today))
	current := f.addFile(models.FileTypeEPUB, "current")
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, current.BookID, today))
	toRead := f.addFile(models.FileTypeEPUB, "toread")
	require.NoError(t, f.reading.AddToRead(f.ctx, f.user.ID, toRead.BookID))

	resp := f.pullStates(0, "read", "paused", "dnf", "current", "toread", syncHash, "unknown")

	got := map[string]string{}
	for _, s := range resp.States {
		got[s.Document] = s.Status
		assert.Nil(t, s.Rating, "%s has no Rating", s.Document)
	}
	assert.Equal(t, map[string]string{
		"read":    KOReaderStatusFinished,
		"paused":  KOReaderStatusOnHold,
		"dnf":     KOReaderStatusOnHold,
		"current": KOReaderStatusReading,
		// To Read and no status leave the device's alone.
		"toread": "",
		syncHash: "",
	}, got, "a book Shisho doesn't recognize is left out")
	assert.NotZero(t, resp.Cursor)
}

func TestPull_ReturnsWhatChangedSinceTheLastPull(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	other := f.addFile(models.FileTypeEPUB, "other")
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, other.BookID, day(f.day1)))
	f.changedOn(other, f.day1)

	cursor := f.pullStates(0).Cursor
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, f.file.BookID, models.ReadThroughOutcomeFinished, day(time.Now())))

	resp := f.pullStates(cursor)
	require.Len(t, resp.States, 1, "only the book that changed")
	assert.Equal(t, syncHash, resp.States[0].Document)
	assert.Equal(t, KOReaderStatusFinished, resp.States[0].Status)
}

func TestStatus_DidNotFinishNeverReturnsAsPaused(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, f.file.BookID, day(f.day1)))
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, f.file.BookID, models.ReadThroughOutcomeAbandoned, day(f.day1)))
	f.changedOn(f.file, f.day1)

	state := f.state(0, syncHash)
	require.NotNil(t, state)
	assert.Equal(t, KOReaderStatusOnHold, state.Status)

	// Even a plugin that sent the On hold it was given back changes nothing.
	f.upload(UploadPayload{Events: []EventPayload{statusEvent("s1", KOReaderStatusOnHold, KOReaderStatusReading, time.Now())}})
	assert.Equal(t, models.ReadingStatusDidNotFinish, f.status(f.file))
	assert.Len(t, f.readThroughs(f.file), 1)
}

func TestStatus_WhenBothChangedTheLaterDayWins(t *testing.T) {
	t.Parallel()
	day2 := func(f *syncFixture) time.Time { return f.day1.Add(24 * time.Hour) }
	for _, tc := range []struct {
		name string
		// shishoAt and deviceAt are when each side changed.
		shishoAt, deviceAt func(f *syncFixture) time.Time
		want               string
	}{
		{"Shisho later", day2, func(f *syncFixture) time.Time { return f.day1 }, models.ReadingStatusPaused},
		{"device later", func(f *syncFixture) time.Time { return f.day1 }, day2, models.ReadingStatusRead},
		{"same day goes to Shisho", day2, func(f *syncFixture) time.Time { return day2(f).Add(5 * time.Hour) }, models.ReadingStatusPaused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newSyncFixture(t)
			// Both last synced reading. Then Shisho paused it and the device
			// finished it.
			require.NoError(t, f.reading.Start(f.ctx, f.user.ID, f.file.BookID, day(f.day1.Add(-24*time.Hour))))
			require.NoError(t, f.reading.SetPaused(f.ctx, f.user.ID, f.file.BookID, true))
			f.changedOn(f.file, tc.shishoAt(f))

			f.upload(UploadPayload{Events: []EventPayload{statusEvent("s1", KOReaderStatusFinished, KOReaderStatusReading, tc.deviceAt(f))}})

			assert.Equal(t, tc.want, f.status(f.file))
		})
	}
}

func TestStatus_ADeviceChangeAppliesWhenShishoDidntChange(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, f.file.BookID, day(f.day1)))
	// Shisho's read began today, after the device's change, but Shisho's
	// status is still the one the device last synced.
	f.upload(UploadPayload{Events: []EventPayload{statusEvent("s1", KOReaderStatusOnHold, KOReaderStatusReading, f.day1)}})
	assert.Equal(t, models.ReadingStatusPaused, f.status(f.file))
}

func TestStatus_ADayOnlyChangeIsBoundedByTheDevice(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	created := time.Now().UTC().Add(-time.Minute)
	f.backdate(f.device, created)
	other := f.addFile(models.FileTypeEPUB, "other")

	f.upload(UploadPayload{Events: []EventPayload{
		// The day the Device was made counts, though its noon may be earlier.
		{ID: "today", Type: PluginEventStatus, Document: syncHash, Day: day(created), Status: KOReaderStatusFinished},
		// A day before it existed can't be right.
		{ID: "before", Type: PluginEventStatus, Document: "other", Day: day(created.Add(-48 * time.Hour)), Status: KOReaderStatusFinished},
	}})

	assert.Equal(t, models.ReadingStatusRead, f.status(f.file))
	assert.Empty(t, f.status(other))
}

func TestRating_FromTheDeviceSetsWholeStars(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)

	f.upload(UploadPayload{Events: []EventPayload{ratingEvent("r1", 4, 0, time.Now())}})

	require.NotNil(t, f.ratingUnits(f.file))
	assert.Equal(t, 16, *f.ratingUnits(f.file))
}

func TestRating_RoundsToWholeStarsForTheDevice(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	for units, stars := range map[int]int{1: 1, 2: 1, 5: 1, 6: 2, 13: 3, 14: 4, 15: 4, 20: 5} {
		require.NoError(t, f.reading.SetRating(f.ctx, f.user.ID, f.file.BookID, &units))
		state := f.state(0, syncHash)
		require.NotNil(t, state)
		require.NotNil(t, state.Rating)
		assert.Equal(t, stars, *state.Rating, "%d quarter stars", units)
	}
}

func TestRating_ARoundedCopyNeverOverwritesAQuarterStarRating(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	units := 15
	require.NoError(t, f.reading.SetRating(f.ctx, f.user.ID, f.file.BookID, &units))
	require.Equal(t, 4, *f.state(0, syncHash).Rating)
	f.changedOn(f.file, f.day1)

	// The device shows 4. Even sent back later, as from a device that synced
	// 3 before Shisho's change, it leaves 3.75 alone.
	f.upload(UploadPayload{Events: []EventPayload{ratingEvent("r1", 4, 3, time.Now())}})

	assert.Equal(t, 15, *f.ratingUnits(f.file))
}

func TestRating_ADeliberateWholeStarChangeWins(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	units := 15
	require.NoError(t, f.reading.SetRating(f.ctx, f.user.ID, f.file.BookID, &units))
	f.changedOn(f.file, f.day1)

	// The device synced 4 for 3.75, then the reader changed it to 3.
	f.upload(UploadPayload{Events: []EventPayload{ratingEvent("r1", 3, 4, time.Now())}})

	assert.Equal(t, 12, *f.ratingUnits(f.file))
}

func TestRating_RemovedOnEitherSide(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	units := 16
	require.NoError(t, f.reading.SetRating(f.ctx, f.user.ID, f.file.BookID, &units))
	f.changedOn(f.file, f.day1)

	// Removed on the device.
	f.upload(UploadPayload{Events: []EventPayload{ratingEvent("r1", 0, 4, time.Now())}})
	assert.Nil(t, f.ratingUnits(f.file))

	// Removed in Shisho: the next pull tells the device.
	require.NoError(t, f.reading.SetRating(f.ctx, f.user.ID, f.file.BookID, &units))
	cursor := f.pullStates(0).Cursor
	require.NoError(t, f.reading.SetRating(f.ctx, f.user.ID, f.file.BookID, nil))
	state := f.state(cursor, syncHash)
	require.NotNil(t, state)
	require.NotNil(t, state.Rating)
	assert.Equal(t, 0, *state.Rating)

	// A book never rated leaves the device's Rating alone.
	other := f.addFile(models.FileTypeEPUB, "other")
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, other.BookID, day(time.Now())))
	state = f.state(cursor, "other")
	require.NotNil(t, state)
	assert.Nil(t, state.Rating)
}

func TestRating_WhenBothChangedTheLaterDayWins(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	units := 8
	require.NoError(t, f.reading.SetRating(f.ctx, f.user.ID, f.file.BookID, &units))
	f.changedOn(f.file, f.day1.Add(24*time.Hour))

	// The device last synced 4, and changed to 5 a day before Shisho's 2.
	f.upload(UploadPayload{Events: []EventPayload{ratingEvent("r1", 5, 4, f.day1)}})
	assert.Equal(t, 8, *f.ratingUnits(f.file))

	// A day after, the device's wins.
	f.upload(UploadPayload{Events: []EventPayload{ratingEvent("r2", 5, 4, f.day1.Add(48*time.Hour))}})
	assert.Equal(t, 20, *f.ratingUnits(f.file))
}

func TestStatus_AnotherUsersBooksAreNeverReachable(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	stranger := f.addUser("stranger", true)
	device := f.addDevice(stranger, devices.DeviceTypeKOReader)

	rec := f.uploadAs(device, UploadPayload{Events: []EventPayload{statusEvent("s1", KOReaderStatusFinished, "", time.Now())}})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Empty(t, f.status(f.file), "the stranger can't reach the library")
}
