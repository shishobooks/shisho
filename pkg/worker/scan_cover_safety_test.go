package worker

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scanner cover writes must never destroy a working cover: a replacement is
// installed before the previous cover is removed, and bytes that do not fully
// decode are rejected before anything is written.

func makePNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// obstruct places a nonempty directory at path so a write there fails
// deterministically: it can be neither created over nor removed.
func obstruct(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.Mkdir(path, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "keep"), []byte("x"), 0600))
}

func newEnricherCoverFixture(t *testing.T) (file *models.File, dir, prevPath string, prevBytes []byte) {
	t.Helper()
	dir = t.TempDir()
	filePath := filepath.Join(dir, "book.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0644))
	prevPath = filepath.Join(dir, "book.epub.cover.jpg")
	prevBytes = makeJPEG(200, 300)
	require.NoError(t, os.WriteFile(prevPath, prevBytes, 0644))
	prevFilename, prevMime, prevSource := "book.epub.cover.jpg", "image/jpeg", models.DataSourceEPUBMetadata
	file = &models.File{Filepath: filePath, FileType: models.FileTypeEPUB, CoverImageFilename: &prevFilename, CoverMimeType: &prevMime, CoverSource: &prevSource}
	return file, dir, prevPath, prevBytes
}

func enricherCoverMetadata(data []byte, mime string) *mediafile.ParsedMetadata {
	return &mediafile.ParsedMetadata{
		CoverData:        data,
		CoverMimeType:    mime,
		FieldDataSources: map[string]string{"cover": models.PluginDataSource("test", "enricher")},
	}
}

func assertEnricherCoverUntouched(t *testing.T, file *models.File, prevPath string, prevBytes []byte) {
	t.Helper()
	got, err := os.ReadFile(prevPath)
	require.NoError(t, err, "the previous cover must still exist")
	assert.Equal(t, prevBytes, got, "the previous cover bytes must be untouched")
	assert.Equal(t, "book.epub.cover.jpg", *file.CoverImageFilename)
	assert.Equal(t, "image/jpeg", *file.CoverMimeType)
	assert.Equal(t, models.DataSourceEPUBMetadata, *file.CoverSource)
}

// A truncated enricher cover passes the header-only resolution gate (its
// header claims a larger image) but cannot be decoded, so it must be skipped.
func TestUpgradeEnricherCover_TruncatedImageIsSkipped(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	file, dir, prevPath, prevBytes := newEnricherCoverFixture(t)

	truncated := makePNG(800, 1200)[:33]
	tc.worker.upgradeEnricherCover(tc.ctx, enricherCoverMetadata(truncated, "image/png"), file, dir, nil)

	assertEnricherCoverUntouched(t, file, prevPath, prevBytes)
	entries, err := filepath.Glob(filepath.Join(dir, "book.epub.cover.*"))
	require.NoError(t, err)
	assert.Equal(t, []string{prevPath}, entries, "no new cover file may be written")
}

func TestUpgradeEnricherCover_FailedWriteKeepsPreviousCover(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	file, dir, prevPath, prevBytes := newEnricherCoverFixture(t)
	obstruct(t, filepath.Join(dir, "book.epub.cover.png"))

	tc.worker.upgradeEnricherCover(tc.ctx, enricherCoverMetadata(makePNG(800, 1200), "image/png"), file, dir, nil)

	assertEnricherCoverUntouched(t, file, prevPath, prevBytes)
}

func TestExtractCBZPageCover_FailedWriteKeepsPreviousCover(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cbzPath := testgen.GenerateCBZ(t, dir, "comic.cbz", testgen.CBZOptions{PageCount: 3, ImageFormat: "jpeg"})
	prevPath := filepath.Join(dir, "comic.cbz.cover.png")
	prevBytes := makePNG(4, 4)
	require.NoError(t, os.WriteFile(prevPath, prevBytes, 0644))
	obstruct(t, filepath.Join(dir, "comic.cbz.cover.jpg"))

	filename, _, stale, err := extractCBZPageCover(cbzPath, dir, "comic.cbz.cover", 1)
	require.Error(t, err)
	assert.Empty(t, filename)
	assert.Nil(t, stale, "a failed install must not report anything to remove")

	got, readErr := os.ReadFile(prevPath)
	require.NoError(t, readErr, "the previous cover must still exist after a failed install")
	assert.Equal(t, prevBytes, got)
}

func TestExtractCBZPageCover_ReportsPreviousCoverAsStale(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cbzPath := testgen.GenerateCBZ(t, dir, "comic.cbz", testgen.CBZOptions{PageCount: 3, ImageFormat: "jpeg"})
	prevPath := filepath.Join(dir, "comic.cbz.cover.png")
	require.NoError(t, os.WriteFile(prevPath, makePNG(4, 4), 0644))

	filename, mime, stale, err := extractCBZPageCover(cbzPath, dir, "comic.cbz.cover", 1)
	require.NoError(t, err)
	assert.Equal(t, "comic.cbz.cover.jpg", filename)
	assert.Equal(t, "image/jpeg", mime)
	assert.Equal(t, []string{prevPath}, stale)
	_, statErr := os.Stat(prevPath)
	require.NoError(t, statErr, "the previous cover is the caller's to remove after its database write")
}

// newScannedCBZFixture scans a CBZ with JPEG pages and swaps its cover for a
// PNG, so a page apply installs at a different extension than it replaces.
func newScannedCBZFixture(t *testing.T) (tc *testContext, file *models.File, book *models.Book, prevPath string, prevBytes []byte) {
	t.Helper()
	tc = newTestContext(t)
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Comic")
	testgen.GenerateCBZ(t, bookDir, "comic.cbz", testgen.CBZOptions{Title: "Comic", HasComicInfo: true, PageCount: 3, ImageFormat: "jpeg"})
	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 1)
	file = files[0]
	require.NotNil(t, file.CoverImageFilename)
	require.NoError(t, os.Remove(filepath.Join(bookDir, *file.CoverImageFilename)))
	prevPath = filepath.Join(bookDir, "comic.cbz.cover.png")
	prevBytes = makePNG(4, 4)
	require.NoError(t, os.WriteFile(prevPath, prevBytes, 0644))
	prevFilename, prevMime := "comic.cbz.cover.png", "image/png"
	file.CoverImageFilename = &prevFilename
	file.CoverMimeType = &prevMime
	require.NoError(t, tc.bookService.UpdateFile(tc.ctx, file, books.UpdateFileOptions{Columns: []string{"cover_image_filename", "cover_mime_type"}}))
	book = &models.Book{ID: file.BookID, Filepath: bookDir}
	return tc, file, book, prevPath, prevBytes
}

// The previous page cover is removed only after the database write succeeds.
// A cancelled context makes UpdateFile fail after the replacement is on disk.
func TestApplyPageCover_RemovesPreviousCoverOnlyAfterUpdate(t *testing.T) {
	t.Parallel()

	t.Run("update fails", func(t *testing.T) {
		t.Parallel()
		tc, file, book, prevPath, prevBytes := newScannedCBZFixture(t)
		cancelled, cancel := context.WithCancel(tc.ctx)
		cancel()

		extractErr, updateErr := tc.worker.applyPageCover(cancelled, file, book, 1, models.DataSourceManual)
		require.NoError(t, extractErr)
		require.Error(t, updateErr)

		got, err := os.ReadFile(prevPath)
		require.NoError(t, err, "the previous cover must survive a failed database write")
		assert.Equal(t, prevBytes, got)
	})

	t.Run("update succeeds", func(t *testing.T) {
		t.Parallel()
		tc, file, book, prevPath, _ := newScannedCBZFixture(t)

		extractErr, updateErr := tc.worker.applyPageCover(tc.ctx, file, book, 1, models.DataSourceManual)
		require.NoError(t, extractErr)
		require.NoError(t, updateErr)

		_, statErr := os.Stat(prevPath)
		assert.True(t, os.IsNotExist(statErr), "the previous cover must be removed after a successful database write")
	})
}

func TestUpgradeEnricherCover_KeepsPreviousCoverWhenUpdateFails(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	file, dir, prevPath, prevBytes := newEnricherCoverFixture(t)
	cancelled, cancel := context.WithCancel(tc.ctx)
	cancel()

	tc.worker.upgradeEnricherCover(cancelled, enricherCoverMetadata(makePNG(800, 1200), "image/png"), file, dir, nil)

	got, err := os.ReadFile(prevPath)
	require.NoError(t, err, "the previous cover must survive a failed database write")
	assert.Equal(t, prevBytes, got)
}

func TestExtractPDFPageCover_FailedWriteKeepsPreviousCover(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pdfPath := testgen.GeneratePDF(t, dir, "book.pdf", testgen.PDFOptions{PageCount: 2})
	prevPath := filepath.Join(dir, "book.pdf.cover.png")
	prevBytes := makePNG(4, 4)
	require.NoError(t, os.WriteFile(prevPath, prevBytes, 0644))
	obstruct(t, filepath.Join(dir, "book.pdf.cover.jpg"))

	filename, _, stale, err := extractPDFPageCover(pdfPath, dir, "book.pdf.cover", 1)
	require.Error(t, err)
	assert.Empty(t, filename)
	assert.Nil(t, stale, "a failed install must not report anything to remove")

	got, readErr := os.ReadFile(prevPath)
	require.NoError(t, readErr, "the previous cover must still exist after a failed install")
	assert.Equal(t, prevBytes, got)
}
