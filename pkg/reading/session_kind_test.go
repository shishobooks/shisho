package reading

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdatePosition_SessionsRecordWhetherTheyWereReadingOrListening(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.recordSession(f.file, 0.4, 0.5)

	audiobook := createBook(t, f.db, "Dune Audio")
	audio := f.addFile(audiobook, "dune.m4b")
	_, err := f.db.NewUpdate().Model(audio).Set("file_type = ?", models.FileTypeM4B).WherePK().Exec(f.ctx)
	require.NoError(t, err)
	seconds := 900.0
	_, err = f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: audio.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{Seconds: &seconds}, Percentage: 0.1,
		Session: &SessionInput{
			StartedAt: f.clock.now.Add(-15 * time.Minute), EndedAt: f.clock.now,
			StartPercentage: 0, EndPercentage: 0.1, Measured: true,
		},
	})
	require.NoError(t, err)

	// The kind outlives the File.
	_, err = f.db.NewDelete().Model((*models.File)(nil)).Where("id = ?", audio.ID).Exec(f.ctx)
	require.NoError(t, err)

	var sessions []*models.ReadingSession
	require.NoError(t, f.db.NewSelect().Model(&sessions).Order("rs.id").Scan(f.ctx))
	require.Len(t, sessions, 2)
	assert.Equal(t, models.ReadingKindReading, sessions[0].Kind)
	assert.Equal(t, 600, sessions[0].Duration, "a timed Session lasts its span")
	assert.Equal(t, models.ReadingKindListening, sessions[1].Kind)
	assert.Nil(t, sessions[1].FileID)
	assert.Equal(t, 900, sessions[1].Duration)
}

// An estimated Session's end moves as its source learns more, as KOReader's
// pulls do, and its duration moves with it, both ways.
func TestSetEstimatedSessionEnd_KeepsTheDurationInStep(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	start := f.clock.now
	id, err := f.svc.RecordEstimatedSession(f.ctx, f.user.ID, f.file.ID, models.ReadingSourceKOReader, SessionInput{
		StartedAt: start, EndedAt: start, StartPercentage: 0.1, EndPercentage: 0.1,
	})
	require.NoError(t, err)

	duration := func() int {
		t.Helper()
		var session models.ReadingSession
		require.NoError(t, f.db.NewSelect().Model(&session).Where("rs.id = ?", id).Scan(f.ctx))
		return session.Duration
	}
	assert.Equal(t, 0, duration())

	require.NoError(t, f.svc.SetEstimatedSessionEnd(f.ctx, f.user.ID, id, start.Add(30*time.Minute), 0.3))
	assert.Equal(t, 30*60, duration())

	require.NoError(t, f.svc.SetEstimatedSessionEnd(f.ctx, f.user.ID, id, start.Add(5*time.Minute), 0.15))
	assert.Equal(t, 5*60, duration(), "a replay rolling the end back shortens it")
}

func (f *positionsFixture) addDevice(id, deviceType string) string {
	f.t.Helper()
	_, err := f.db.ExecContext(f.ctx, `INSERT INTO devices (id, user_id, name, device_type, credential, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, f.user.ID, id, deviceType, "cred-"+id, f.clock.now, f.clock.now)
	require.NoError(f.t, err)
	return id
}

func (f *positionsFixture) sessionDevices() []*string {
	f.t.Helper()
	var sessions []*models.ReadingSession
	require.NoError(f.t, f.db.NewSelect().Model(&sessions).Order("rs.id").Scan(f.ctx))
	devices := make([]*string, len(sessions))
	for i, s := range sessions {
		devices[i] = s.SourceDeviceID
	}
	return devices
}

// Each Reading Session records the Device that reported it, so a Device's
// sessions can be told apart; a browser isn't a Device. Deleting the Device
// keeps its sessions.
func TestSessions_RecordTheDeviceThatReportedThem(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	kobo := f.addDevice("kobo-1", "kobo")
	koreader := f.addDevice("koreader-1", "koreader")

	f.recordSession(f.file, 0.1, 0.2)

	span := "OEBPS/ch1.xhtml#kobo.4.1"
	f.clock.advance(time.Hour)
	_, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: f.file.ID, Source: models.ReadingSourceKobo, DeviceID: &kobo,
		Locator: models.ReadingLocator{KoboSpan: &span}, Percentage: 0.3,
		Session: &SessionInput{StartedAt: f.clock.now.Add(-5 * time.Minute), EndedAt: f.clock.now, StartPercentage: 0.2, EndPercentage: 0.3},
	})
	require.NoError(t, err)

	f.clock.advance(time.Hour)
	_, err = f.svc.RecordEstimatedSession(f.ctx, f.user.ID, f.file.ID, models.ReadingSourceKOReader, SessionInput{
		StartedAt: f.clock.now.Add(-5 * time.Minute), EndedAt: f.clock.now, StartPercentage: 0.3, EndPercentage: 0.4, DeviceID: &koreader,
	})
	require.NoError(t, err)

	assert.Equal(t, []*string{nil, &kobo, &koreader}, f.sessionDevices())

	_, err = f.db.ExecContext(f.ctx, `DELETE FROM devices WHERE id = ?`, kobo)
	require.NoError(t, err)
	assert.Equal(t, []*string{nil, nil, &koreader}, f.sessionDevices())
}
