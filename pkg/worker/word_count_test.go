package worker

import (
	"os"
	"testing"
	"time"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generatedChapterWords is the word count of GenerateEPUB's default chapter,
// "Chapter 1" and "This is a test chapter.".
const generatedChapterWords = 7

func (tc *testContext) fileByType(fileType string) *models.File {
	tc.t.Helper()
	for _, f := range tc.listFiles() {
		if f.FileType == fileType {
			return f
		}
	}
	tc.t.Fatalf("no %s file", fileType)
	return nil
}

func (tc *testContext) fileByPath(path string) *models.File {
	tc.t.Helper()
	for _, f := range tc.listFiles() {
		if f.Filepath == path {
			return f
		}
	}
	tc.t.Fatalf("no file at %s", path)
	return nil
}

// clearWordCounts leaves every File as a scan before word counts left it.
func (tc *testContext) clearWordCounts() {
	tc.t.Helper()
	_, err := tc.db.NewUpdate().Table("files").Set("word_count = NULL").Set("word_count_failed = FALSE").Where("1 = 1").Exec(tc.ctx)
	require.NoError(tc.t, err)
}

func TestScan_CountsWordsOfReflowableFiles(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	testgen.GenerateEPUB(t, testgen.CreateSubDir(t, libraryPath, "Prose"), "prose.epub", testgen.EPUBOptions{Title: "Prose"})
	testgen.GenerateCBZ(t, testgen.CreateSubDir(t, libraryPath, "Comic"), "comic.cbz", testgen.CBZOptions{Title: "Comic", PageCount: 3})

	require.NoError(t, tc.runScan())

	epub := tc.fileByType(models.FileTypeEPUB)
	require.NotNil(t, epub.WordCount)
	assert.Equal(t, generatedChapterWords, *epub.WordCount)
	assert.Nil(t, tc.fileByType(models.FileTypeCBZ).WordCount, "a Page-based File has no word count")
}

func TestScan_RecountsWordsOfChangedFile(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Prose")
	path := testgen.GenerateEPUB(t, bookDir, "prose.epub", testgen.EPUBOptions{Title: "Prose"})
	require.NoError(t, tc.runScan())

	testgen.GenerateEPUB(t, bookDir, "prose.epub", testgen.EPUBOptions{
		Title:   "Prose",
		Chapter: []byte(`<html><body><p>Only four words here.</p></body></html>`),
	})
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(path, later, later))
	require.NoError(t, tc.runScan())

	epub := tc.fileByType(models.FileTypeEPUB)
	require.NotNil(t, epub.WordCount)
	assert.Equal(t, 4, *epub.WordCount)
}

func TestScan_CountsWordsOfUnchangedFilesWithoutACount(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	testgen.GenerateEPUB(t, testgen.CreateSubDir(t, libraryPath, "Prose"), "prose.epub", testgen.EPUBOptions{Title: "Prose"})
	testgen.GenerateMOBI(t, testgen.CreateSubDir(t, libraryPath, "Kindle"), "kindle.azw3", testgen.KF8Sample(testgen.MOBIKindKF8))
	testgen.GenerateCBZ(t, testgen.CreateSubDir(t, libraryPath, "Comic"), "comic.cbz", testgen.CBZOptions{Title: "Comic", PageCount: 3})
	require.NoError(t, tc.runScan())
	scanned := tc.fileByType(models.FileTypeAZW3)
	require.NotNil(t, scanned.WordCount)
	azw3Words := *scanned.WordCount

	// As a library scanned before word counts were kept.
	tc.clearWordCounts()
	before := tc.fileByType(models.FileTypeEPUB)
	require.NoError(t, tc.runScan())

	epub := tc.fileByType(models.FileTypeEPUB)
	require.NotNil(t, epub.WordCount)
	assert.Equal(t, generatedChapterWords, *epub.WordCount)
	assert.Equal(t, before.UpdatedAt, epub.UpdatedAt, "a count isn't an edit, and updated_at keys the cover URLs")
	azw3 := tc.fileByType(models.FileTypeAZW3)
	require.NotNil(t, azw3.WordCount)
	assert.Equal(t, azw3Words, *azw3.WordCount)
	assert.Nil(t, tc.fileByType(models.FileTypeCBZ).WordCount)
}

func TestScan_LeavesFilesItCannotCountWithoutACount(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	// An EPUB with Adobe DRM is imported, but its text can't be read.
	locked := testgen.GenerateEPUB(t, testgen.CreateSubDir(t, libraryPath, "Locked"), "locked.epub", testgen.EPUBOptions{
		Title:        "Locked",
		ExtraEntries: map[string]string{"META-INF/rights.xml": "<rights/>"},
	})
	open := testgen.GenerateEPUB(t, testgen.CreateSubDir(t, libraryPath, "Open"), "open.epub", testgen.EPUBOptions{Title: "Open"})
	require.NoError(t, tc.runScan())
	tc.clearWordCounts()

	logs := tc.runScanWithJob()
	assert.Len(t, warningsFor(logs, locked), 1, "a File it can't count is named, and doesn't fail the scan")
	assert.Nil(t, tc.fileByPath(locked).WordCount)
	counted := tc.fileByPath(open)
	require.NotNil(t, counted.WordCount, "one File it can't count doesn't stop the rest")
	assert.Equal(t, generatedChapterWords, *counted.WordCount)

	logs = tc.runScanWithJob()
	assert.Empty(t, warningsFor(logs, locked), "an unchanged File that couldn't be counted isn't tried again")
}

func TestScan_KeepsACountedFilesCount(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	path := testgen.GenerateEPUB(t, testgen.CreateSubDir(t, libraryPath, "Prose"), "prose.epub", testgen.EPUBOptions{Title: "Prose"})
	require.NoError(t, tc.runScan())
	_, err := tc.db.NewUpdate().Table("files").Set("word_count = 12345").Where("1 = 1").Exec(tc.ctx)
	require.NoError(t, err)

	require.NoError(t, tc.runScan())

	got := tc.fileByPath(path).WordCount
	require.NotNil(t, got)
	assert.Equal(t, 12345, *got, "a scan that skips an unchanged File doesn't count it again")
}

func TestScan_DropsTheCountOfAReplacementItCannotCount(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Prose")
	path := testgen.GenerateEPUB(t, bookDir, "prose.epub", testgen.EPUBOptions{Title: "Prose"})
	require.NoError(t, tc.runScan())
	require.NotNil(t, tc.fileByPath(path).WordCount)

	testgen.GenerateEPUB(t, bookDir, "prose.epub", testgen.EPUBOptions{
		Title:        "Prose",
		ExtraEntries: map[string]string{"META-INF/rights.xml": "<rights/>"},
	})
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(path, later, later))
	require.NoError(t, tc.runScan())

	assert.Nil(t, tc.fileByPath(path).WordCount, "the old edition's count isn't the new one's")
}

func TestScan_ScanForNewMetadataCountsWithoutChangingCoverURLs(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	path := testgen.GenerateEPUB(t, testgen.CreateSubDir(t, libraryPath, "Prose"), "prose.epub", testgen.EPUBOptions{Title: "Prose"})
	require.NoError(t, tc.runScan())
	tc.clearWordCounts()
	before := tc.fileByPath(path)

	libraryID := before.LibraryID
	require.NoError(t, tc.runScanJob(&models.Job{
		Type:       models.JobTypeScan,
		LibraryID:  &libraryID,
		DataParsed: &models.JobScanData{ScanForNewMetadata: true},
	}))

	after := tc.fileByPath(path)
	require.NotNil(t, after.WordCount)
	assert.Equal(t, generatedChapterWords, *after.WordCount)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt, "a count isn't an edit, and updated_at keys the cover URLs")
}

func TestCountUnchangedFileWords_LeavesAFileDemotedMeanwhile(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	path := testgen.GenerateEPUB(t, testgen.CreateSubDir(t, libraryPath, "Prose"), "prose.epub", testgen.EPUBOptions{Title: "Prose"})
	require.NoError(t, tc.runScan())
	tc.clearWordCounts()
	// The scan loaded the File as a main File with no count...
	stale := tc.fileByPath(path)
	// ...and it was made a supplement before the count was written.
	_, err := tc.db.NewUpdate().Table("files").Set("file_role = ?", models.FileRoleSupplement).Where("id = ?", stale.ID).Exec(tc.ctx)
	require.NoError(t, err)

	tc.worker.countUnchangedFileWords(tc.ctx, stale, ScanOptions{})

	assert.Nil(t, tc.fileByPath(path).WordCount, "supplements have no word count")
}
