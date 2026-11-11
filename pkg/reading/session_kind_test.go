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
