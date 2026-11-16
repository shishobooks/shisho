package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/cbz"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScan_CBZReadingDirectionFromComicInfo(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Manga")
	testgen.GenerateCBZ(t, bookDir, "manga.cbz", testgen.CBZOptions{
		Title:        "Manga",
		HasComicInfo: true,
		Manga:        cbz.MangaYesAndRightToLeft,
	})
	plainDir := testgen.CreateSubDir(t, libraryPath, "[Author] Comic")
	testgen.GenerateCBZ(t, plainDir, "comic.cbz", testgen.CBZOptions{Title: "Comic", HasComicInfo: true})

	require.NoError(t, tc.runScan())

	byName := map[string]*models.File{}
	for _, f := range tc.listFiles() {
		byName[filepath.Base(f.Filepath)] = f
	}
	manga := byName["manga.cbz"]
	require.NotNil(t, manga)
	require.NotNil(t, manga.ReadingDirection)
	assert.Equal(t, models.ReadingDirectionRTL, *manga.ReadingDirection)
	require.NotNil(t, manga.ReadingDirectionSource)
	assert.Equal(t, models.DataSourceCBZMetadata, *manga.ReadingDirectionSource)

	comic := byName["comic.cbz"]
	require.NotNil(t, comic)
	assert.Nil(t, comic.ReadingDirection, "a ComicInfo without Manga says nothing, so the File runs left to right")
}

func TestScan_ManualReadingDirectionSurvivesRescan(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Manga")
	testgen.GenerateCBZ(t, bookDir, "manga.cbz", testgen.CBZOptions{
		Title:        "Manga",
		HasComicInfo: true,
		Manga:        cbz.MangaYesAndRightToLeft,
	})
	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 1)
	file := files[0]
	ltr := models.ReadingDirectionLTR
	manual := models.DataSourceManual
	file.ReadingDirection = &ltr
	file.ReadingDirectionSource = &manual
	require.NoError(t, tc.bookService.UpdateFile(tc.ctx, file, books.UpdateFileOptions{
		Columns: []string{"reading_direction", "reading_direction_source"},
	}))

	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID}, nil)
	require.NoError(t, err)

	files = tc.listFiles()
	require.Len(t, files, 1)
	require.NotNil(t, files[0].ReadingDirection)
	assert.Equal(t, models.ReadingDirectionLTR, *files[0].ReadingDirection)
	assert.Equal(t, models.DataSourceManual, *files[0].ReadingDirectionSource)
}

func TestScan_ReadingDirectionFromSidecar(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Manga")
	testgen.GeneratePDF(t, bookDir, "manga.pdf", testgen.PDFOptions{})
	sidecar := `{"version":1,"reading_direction":"rtl"}`
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "manga.pdf.metadata.json"), []byte(sidecar), 0o644))

	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 1)
	require.NotNil(t, files[0].ReadingDirection)
	assert.Equal(t, models.ReadingDirectionRTL, *files[0].ReadingDirection)
	assert.Equal(t, models.DataSourceSidecar, *files[0].ReadingDirectionSource)
}
