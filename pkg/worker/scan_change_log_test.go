package worker

import (
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
	for _, force := range []bool{false, true} {
		for _, book := range books {
			_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{BookID: book.ID, ForceRefresh: force}, nil)
			require.NoError(t, err)
		}
		assert.Equal(t, before, tc.changeLog(), "force refresh %v", force)
	}
	assert.Equal(t, chaptersBefore, chapterIDs())
}
