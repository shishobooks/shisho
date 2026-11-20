package worker

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// changeLog lists the change log as "kind:record_id:seq", so a write that
// moved any entry changes it.
func (tc *testContext) changeLog() []string {
	tc.t.Helper()
	var entries []string
	require.NoError(tc.t, tc.db.NewRaw(`SELECT kind || ':' || record_id || ':' || seq FROM change_log_entries ORDER BY seq`).Scan(tc.ctx, &entries))
	return entries
}

// A Resync of a Book whose files haven't changed, forced or not, rewrites
// rows with the values they hold, and records nothing (ADR 0023). Chapters
// and relationships are updated in place, so they keep their rows.
func TestResync_UnchangedBookRecordsNothing(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)
	tc := newTestContext(t)
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})

	one := 1.0
	epubDir := testgen.CreateSubDir(t, libraryPath, "Dune")
	testgen.GenerateEPUB(t, epubDir, "dune.epub", testgen.EPUBOptions{
		Title: "Dune", Authors: []string{"Frank Herbert", "Brian Herbert"}, Series: "Dune Chronicles", SeriesNumber: &one, HasCover: true,
	})
	cbzDir := testgen.CreateSubDir(t, libraryPath, "Comic")
	testgen.GenerateCBZ(t, cbzDir, "comic.cbz", testgen.CBZOptions{
		Title: "Comic", Writer: "Wanda Writer", Penciller: "Pat Penciller", HasComicInfo: true, PageCount: 3,
	})
	m4bDir := testgen.CreateSubDir(t, libraryPath, "Listen")
	testgen.GenerateM4B(t, m4bDir, "listen.m4b", testgen.M4BOptions{
		Title: "Listen", Artist: "Ann Author", Composer: "Ned Narrator", Duration: 6, HasCover: true,
		Chapters: []testgen.M4BChapter{{Title: "One", Start: 0}, {Title: "Two", Start: 2}, {Title: "Three", Start: 4}},
	})
	require.NoError(t, tc.runScan())

	books := tc.listBooks()
	require.Len(t, books, 3)
	var m4bFileID int
	for _, file := range tc.listFiles() {
		if file.FileType == "m4b" {
			m4bFileID = file.ID
		}
	}
	require.NotZero(t, m4bFileID)
	chapterIDs := func() []int {
		var ids []int
		for _, chapter := range tc.listChapters(m4bFileID) {
			ids = append(ids, chapter.ID)
		}
		return ids
	}
	chaptersBefore := chapterIDs()
	require.Len(t, chaptersBefore, 3)

	before := tc.changeLog()
	for _, book := range books {
		_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{BookID: book.ID}, nil)
		require.NoError(t, err)
	}
	assert.Equal(t, before, tc.changeLog())

	// A forced refresh records nothing either, except for a page-based File:
	// its page URLs are keyed by updated_at, which a refresh moves so
	// browsers drop pages it re-read (see TestForceRefresh_MovesThePageKey).
	var comicBookID int
	for _, book := range books {
		if book.Title == "Comic" {
			comicBookID = book.ID
		}
		_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{BookID: book.ID, ForceRefresh: true}, nil)
		require.NoError(t, err)
	}
	require.NotZero(t, comicBookID)
	after := tc.changeLog()
	assert.Len(t, after, len(before))
	moved := []string{}
	for _, entry := range after {
		if !slices.Contains(before, entry) {
			moved = append(moved, entry[:strings.LastIndex(entry, ":")])
		}
	}
	assert.Equal(t, []string{fmt.Sprintf("book:%d", comicBookID)}, moved)
	assert.Equal(t, chaptersBefore, chapterIDs())
}

// A file's bytes can change while its size and modified time don't (a copy
// that keeps them). A forced refresh re-reads it and drops the cached pages,
// so it must also move updated_at, the key on the immutable page URLs, or
// browsers keep showing the old pages.
func TestForceRefresh_MovesThePageKey(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	cbzDir := testgen.CreateSubDir(t, libraryPath, "Comic")
	testgen.GenerateCBZ(t, cbzDir, "comic.cbz", testgen.CBZOptions{Title: "Comic", HasComicInfo: true, PageCount: 2})
	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 1)
	before := files[0].UpdatedAt
	books := tc.listBooks()
	require.Len(t, books, 1)

	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{BookID: books[0].ID}, nil)
	require.NoError(t, err)
	assert.Equal(t, before, tc.listFiles()[0].UpdatedAt, "an unforced Resync of an unchanged file")

	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{BookID: books[0].ID, ForceRefresh: true}, nil)
	require.NoError(t, err)
	assert.True(t, tc.listFiles()[0].UpdatedAt.After(before), "a forced refresh")
}
