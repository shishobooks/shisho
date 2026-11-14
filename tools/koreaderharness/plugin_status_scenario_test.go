//go:build koreaderharness

package main

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (s *scenario) readThroughs(file *models.File) []*models.ReadThrough {
	s.t.Helper()
	var rts []*models.ReadThrough
	err := s.db.NewSelect().Model(&rts).Where("rt.user_id = ? AND rt.book_id = ?", s.user.ID, file.BookID).
		OrderExpr("rt.id").Scan(s.ctx)
	require.NoError(s.t, err)
	return rts
}

func (s *scenario) ratingUnits(file *models.File) *int {
	s.t.Helper()
	reading, err := s.reading.RetrieveBookReading(s.ctx, s.user.ID, file.BookID)
	require.NoError(s.t, err)
	return reading.RatingUnits
}

// Finishing a book at KOReader's end-of-book prompt marks it Read in Shisho
// at the next sync, ended the day it was finished, and its stars become its
// Rating. Neither is sent back to KOReader as a change.
func TestPluginSyncsAFinishFromKOReader(t *testing.T) {
	s := newScenario(t)
	s.downloadOPDS("Comic", "comic.cbz")
	cbz := s.file(models.FileTypeCBZ)
	s.signInPlugin(time.Now().Add(-time.Hour))

	read := s.run(Args{Command: "plugin-read", File: "comic.cbz", Turns: 3, MarkFinished: true, Stars: 4, Close: true})
	require.Contains(t, read.Plugin.Sidecars, "comic.cbz")
	require.Equal(t, "complete", read.Plugin.Sidecars["comic.cbz"].Status)
	assert.Empty(t, s.readThroughs(cbz), "nothing goes with Wi-Fi off")

	synced := s.run(Args{Command: "plugin-sync", Online: true})
	rts := s.readThroughs(cbz)
	require.Len(t, rts, 1, "shown: %v", synced.Shown)
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
	require.NotNil(t, rts[0].EndDay)
	assert.Equal(t, time.Now().UTC().Format(time.DateOnly), *rts[0].EndDay)
	require.NotNil(t, s.ratingUnits(cbz))
	assert.Equal(t, 16, *s.ratingUnits(cbz), "4 stars")

	again := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.Empty(t, again.Plugin.Problems)
	assert.Len(t, s.readThroughs(cbz), 1, "nothing is sent twice")
	assert.Equal(t, "complete", again.Plugin.Sidecars["comic.cbz"].Status)
}

// Marking a book Read in Shisho shows it finished in KOReader after the next
// sync.
func TestPluginWritesAReadFromShisho(t *testing.T) {
	s := newScenario(t)
	s.downloadOPDS("Comic", "comic.cbz")
	cbz := s.file(models.FileTypeCBZ)
	s.signInPlugin(time.Now().Add(-time.Hour))

	opened := s.run(Args{Command: "plugin-read", File: "comic.cbz", Turns: 3, Close: true})
	require.Equal(t, "reading", opened.Plugin.Sidecars["comic.cbz"].Status, "opening a book marks it reading")
	s.run(Args{Command: "plugin-sync", Online: true})
	rts := s.readThroughs(cbz)
	require.Len(t, rts, 1)
	require.Equal(t, models.ReadThroughOutcomeUnderway, rts[0].Outcome, "the pages read started it")

	require.NoError(t, s.reading.End(s.ctx, s.user.ID, cbz.BookID, models.ReadThroughOutcomeFinished, time.Now().UTC().Format(time.DateOnly)))
	synced := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.Equal(t, "complete", synced.Plugin.Sidecars["comic.cbz"].Status, "shown: %v", synced.Shown)

	s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.Len(t, s.readThroughs(cbz), 1, "the finish written to the device isn't sent back")
}

// A status and Rating set in Shisho reach a book this KOReader has never
// opened, found through its index of the home folder, and the Rating's
// rounded copy never comes back to overwrite its quarter star.
func TestPluginWritesToABookNeverOpened(t *testing.T) {
	s := newScenario(t)
	s.downloadOPDS("Long Book", "long.epub")
	epub := s.file(models.FileTypeEPUB)
	s.signInPlugin(time.Now().Add(-time.Hour))
	require.NoError(t, s.reading.End(s.ctx, s.user.ID, epub.BookID, models.ReadThroughOutcomeFinished, time.Now().UTC().Format(time.DateOnly)))
	units := 15
	require.NoError(t, s.reading.SetRating(s.ctx, s.user.ID, epub.BookID, &units))

	synced := s.run(Args{Command: "plugin-sync", Online: true})
	require.Contains(t, synced.Plugin.Sidecars, "long.epub", "shown: %v", synced.Shown)
	sidecar := synced.Plugin.Sidecars["long.epub"]
	assert.Equal(t, "complete", sidecar.Status)
	assert.Equal(t, 4, sidecar.Rating, "3.75 stars round to 4")

	s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	require.NotNil(t, s.ratingUnits(epub))
	assert.Equal(t, 15, *s.ratingUnits(epub), "the rounded copy isn't sent back")
	assert.Len(t, s.readThroughs(epub), 1)
}

// A book saved outside KOReader's home folder, as KOReader's OPDS catalog
// saves to its own download folder, still gets its status from Shisho.
func TestPluginWritesToABookOutsideTheHomeFolder(t *testing.T) {
	s := newScenario(t)
	s.downloadOPDS("Long Book", "long.epub")
	epub := s.file(models.FileTypeEPUB)
	s.signInPlugin(time.Now().Add(-time.Hour))
	require.NoError(t, s.reading.End(s.ctx, s.user.ID, epub.BookID, models.ReadThroughOutcomeFinished, time.Now().UTC().Format(time.DateOnly)))

	// The book is in /books, and the home folder is a folder inside it.
	synced := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true, HomeDir: "/books/main", DownloadDir: "/books"})
	require.Contains(t, synced.Plugin.Sidecars, "long.epub", "shown: %v", synced.Shown)
	assert.Equal(t, "complete", synced.Plugin.Sidecars["long.epub"].Status)
}

// Reading a finished book again in Shisho sets it back to reading in
// KOReader, taking the finished mark off.
func TestPluginWritesARereadFromShisho(t *testing.T) {
	s := newScenario(t)
	s.downloadOPDS("Long Book", "long.epub")
	epub := s.file(models.FileTypeEPUB)
	s.signInPlugin(time.Now().Add(-time.Hour))
	today := time.Now().UTC().Format(time.DateOnly)
	require.NoError(t, s.reading.End(s.ctx, s.user.ID, epub.BookID, models.ReadThroughOutcomeFinished, today))
	finished := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	require.Equal(t, "complete", finished.Plugin.Sidecars["long.epub"].Status)

	require.NoError(t, s.reading.Start(s.ctx, s.user.ID, epub.BookID, today))
	reread := s.run(Args{Command: "plugin-sync", Online: true, SyncNow: true})
	assert.Equal(t, "reading", reread.Plugin.Sidecars["long.epub"].Status, "shown: %v", reread.Shown)
	assert.Positive(t, reread.Plugin.Redrawn, "the file browser shows the status it now has")
	rts := s.readThroughs(epub)
	require.Len(t, rts, 2)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, rts[1].Outcome, "the reread stays underway")
}
