package koreaderplugin

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReference_PullSendsNoStatus(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, f.file.BookID, models.ReadThroughOutcomeFinished, day(f.day1)))
	require.NoError(t, f.reading.MarkReference(f.ctx, f.user.ID, f.file.BookID, nil))

	state := f.state(0, syncHash)

	require.NotNil(t, state)
	assert.Empty(t, state.Status, "KOReader keeps whatever status it had")
}

func TestReference_IncomingStatusesDontChangeIt(t *testing.T) {
	t.Parallel()
	for _, status := range []string{KOReaderStatusFinished, KOReaderStatusOnHold, KOReaderStatusReading} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			f := newSyncFixture(t)
			require.NoError(t, f.reading.MarkReference(f.ctx, f.user.ID, f.file.BookID, nil))
			now := time.Now()

			f.upload(UploadPayload{
				Pages:  pages(syncHash, 1, 5, now.Add(-time.Hour)),
				Events: []EventPayload{statusEvent("s1", status, KOReaderStatusReading, now.Add(-30*time.Minute))},
			})

			assert.Equal(t, models.ReadingStatusReference, f.status(f.file))
			assert.Empty(t, f.readThroughs(f.file))
			assert.NotEmpty(t, f.sessionsOf(f.file), "the time still comes through")
		})
	}
}

func TestReference_BackfillDoesntChangeIt(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	require.NoError(t, f.reading.MarkReference(f.ctx, f.user.ID, f.file.BookID, nil))
	start := f.day1.AddDate(-1, 0, 0)

	f.backfill(UploadPayload{Pages: pages(syncHash, 1, 10, start)})
	f.backfill(UploadPayload{Events: []EventPayload{
		historyStatus("a", syncHash, KOReaderStatusFinished, day(start.AddDate(0, 0, 3))),
	}})

	assert.Equal(t, models.ReadingStatusReference, f.status(f.file))
	assert.Empty(t, f.readThroughs(f.file))
	assert.NotEmpty(t, f.sessionsOf(f.file))
}
