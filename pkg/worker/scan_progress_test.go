package worker

import (
	"testing"
	"time"

	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drainScanProgress returns the scan progress events already published to ch.
func drainScanProgress(t *testing.T, ch chan events.Event) []models.JobScanProgress {
	t.Helper()
	var got []models.JobScanProgress
	for {
		select {
		case evt := <-ch:
			if evt.Type != events.EventTypeScanProgress {
				continue
			}
			var p models.JobScanProgress
			require.NoError(t, json.Unmarshal([]byte(evt.Data), &p))
			got = append(got, p)
		default:
			return got
		}
	}
}

func TestScanProgressReporter_Throttles(t *testing.T) {
	t.Parallel()
	broker := events.NewBroker()
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)

	now := time.Unix(1_000, 0)
	p := newScanProgressReporter(broker, 7, 3, true)
	p.now = func() time.Time { return now }

	for i := 0; i < 50; i++ {
		p.fileFound()
	}
	got := drainScanProgress(t, ch)
	require.Len(t, got, 1, "only the first count publishes until the interval passes")
	assert.Equal(t, models.JobScanProgress{
		JobID:              7,
		LibraryID:          3,
		ScanForNewMetadata: true,
		FilesFound:         1,
	}, got[0])

	now = now.Add(scanProgressInterval - time.Millisecond)
	p.fileScanned(true)
	assert.Empty(t, drainScanProgress(t, ch), "nothing publishes inside the interval")

	now = now.Add(time.Millisecond)
	p.fileScanned(false)
	got = drainScanProgress(t, ch)
	require.Len(t, got, 1)
	assert.Equal(t, 50, got[0].FilesFound)
	assert.Equal(t, 2, got[0].FilesScanned)
	assert.Equal(t, 1, got[0].BooksImported)
	assert.False(t, got[0].Done)

	// Flushing publishes the latest counts at once, without marking them
	// done, so a library that goes on to organize files shows its last
	// imports.
	p.fileScanned(true)
	assert.Empty(t, drainScanProgress(t, ch))
	p.flush()
	got = drainScanProgress(t, ch)
	require.Len(t, got, 1)
	assert.Equal(t, 3, got[0].FilesScanned)
	assert.Equal(t, 2, got[0].BooksImported)
	assert.False(t, got[0].Done)

	// Finishing publishes the final counts at once, whatever the interval.
	p.finish()
	got = drainScanProgress(t, ch)
	require.Len(t, got, 1)
	assert.Equal(t, 50, got[0].FilesFound)
	assert.Equal(t, 3, got[0].FilesScanned)
	assert.True(t, got[0].Done)
}

func TestProcessScanJob_PublishesProgress(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	broker := events.NewBroker()
	tc.worker.broker = broker
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	for _, title := range []string{"One", "Two", "Three"} {
		dir := testgen.CreateSubDir(t, libraryPath, title)
		testgen.GenerateEPUB(t, dir, "book.epub", testgen.EPUBOptions{Title: title, Authors: []string{"An Author"}})
	}
	libraryID := tc.listLibraryIDs()[0]

	require.NoError(t, tc.runScanJob(&models.Job{
		ID:         42,
		Type:       models.JobTypeScan,
		DataParsed: &models.JobScanData{},
	}))

	got := drainScanProgress(t, ch)
	require.NotEmpty(t, got)
	last := got[len(got)-1]
	assert.Equal(t, models.JobScanProgress{
		JobID:         42,
		LibraryID:     libraryID,
		FilesFound:    3,
		FilesScanned:  3,
		BooksImported: 3,
		Done:          true,
	}, last)

	// A scan that finds nothing new imports no books.
	require.NoError(t, tc.runScanJob(&models.Job{
		ID:         43,
		Type:       models.JobTypeScan,
		DataParsed: &models.JobScanData{},
	}))
	got = drainScanProgress(t, ch)
	require.NotEmpty(t, got)
	last = got[len(got)-1]
	assert.Equal(t, 3, last.FilesFound)
	assert.Equal(t, 0, last.BooksImported)
	assert.True(t, last.Done)
}

func TestProcessScanJob_PublishesProgressPerLibrary(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	broker := events.NewBroker()
	tc.worker.broker = broker
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)

	// A global scan job covers both libraries, each with its own counts.
	first := testgen.TempLibraryDir(t)
	second := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{first})
	tc.createLibrary([]string{second})
	testgen.GenerateEPUB(t, testgen.CreateSubDir(t, first, "One"), "book.epub", testgen.EPUBOptions{Title: "One", Authors: []string{"A"}})
	for _, title := range []string{"Two", "Three"} {
		testgen.GenerateEPUB(t, testgen.CreateSubDir(t, second, title), "book.epub", testgen.EPUBOptions{Title: title, Authors: []string{"A"}})
	}
	ids := tc.listLibraryIDs()
	require.Len(t, ids, 2)

	require.NoError(t, tc.runScanJob(&models.Job{ID: 5, Type: models.JobTypeScan, DataParsed: &models.JobScanData{}}))

	final := map[int]models.JobScanProgress{}
	for _, p := range drainScanProgress(t, ch) {
		if p.Done {
			final[p.LibraryID] = p
		}
	}
	require.Len(t, final, 2, "each library ends with its own done event")
	assert.Equal(t, 1, final[ids[0]].BooksImported)
	assert.Equal(t, 2, final[ids[1]].BooksImported)
	assert.Equal(t, 2, final[ids[1]].FilesFound)
}
