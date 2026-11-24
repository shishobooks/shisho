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
	_, err := tc.db.NewUpdate().Table("files").Set("word_count = NULL").Where("1 = 1").Exec(tc.ctx)
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

	require.NoError(t, tc.runScan(), "a File it can't count doesn't fail the scan")

	assert.Nil(t, tc.fileByPath(locked).WordCount)
	counted := tc.fileByPath(open)
	require.NotNil(t, counted.WordCount, "one File it can't count doesn't stop the rest")
	assert.Equal(t, generatedChapterWords, *counted.WordCount)
}
