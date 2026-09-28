package worker

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withPageCaches gives the test worker its own page caches, the way
// cmd/api/main.go hands the server's caches to worker.New. It returns the
// shared cache directory.
func withPageCaches(t *testing.T, tc *testContext) (*cbzpages.Cache, string) {
	t.Helper()
	cacheDir := t.TempDir()
	cbzCache := cbzpages.NewCache(cacheDir)
	tc.worker.cbzPageCache = cbzCache
	tc.worker.pdfPageCache = pdfpages.NewCache(cacheDir, 72, 80)
	return cbzCache, cacheDir
}

// primeCachedPage writes page 0 where the page cache of the given kind
// ("pdf" or "cbz") keeps it: {cacheDir}/{kind}/{fileID}/page_0.jpg (see
// pkg/pdfpages/AGENTS.md; cbzpages matches any page_0.* extension). A real
// PDF render goes through the shared pdfium WASM pool, which is slow to start
// and is not what these tests are about.
func primeCachedPage(t *testing.T, cacheDir, kind string, fileID int) string {
	t.Helper()
	pageDir := filepath.Join(cacheDir, kind, strconv.Itoa(fileID))
	require.NoError(t, os.MkdirAll(pageDir, 0755))
	cachedPage := filepath.Join(pageDir, "page_0.jpg")
	require.NoError(t, os.WriteFile(cachedPage, []byte("old render"), 0600))
	return cachedPage
}

// assertUpdatedAtBumped checks that the scan moved the file's updated_at,
// which the frontend uses as the page URL's cache key.
func assertUpdatedAtBumped(t *testing.T, tc *testContext, before *models.File) {
	t.Helper()
	after, err := tc.bookService.RetrieveFile(tc.ctx, books.RetrieveFileOptions{ID: &before.ID})
	require.NoError(t, err)
	assert.True(t, after.UpdatedAt.After(before.UpdatedAt),
		"the scan must bump updated_at so page URLs get a new cache key")
}

// scanSingleFile scans a library holding one file and returns that file.
func scanSingleFile(t *testing.T, tc *testContext) *models.File {
	t.Helper()
	require.NoError(t, tc.runScan())
	files := tc.listFiles()
	require.Len(t, files, 1)
	return files[0]
}

// TestScan_ChangedCBZ_InvalidatesCachedPages verifies that a library scan
// that finds a CBZ replaced on disk (different size) drops the pages cached
// for its file ID, so the page endpoint cannot serve the old file's images.
func TestScan_ChangedCBZ_InvalidatesCachedPages(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	cbzCache, _ := withPageCaches(t, tc)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	cbzPath := testgen.GenerateCBZ(t, libraryPath, "comic.cbz", testgen.CBZOptions{
		Title:     "Comic",
		PageCount: 3,
	})
	file := scanSingleFile(t, tc)

	cachedPage, _, err := cbzCache.GetPage(cbzPath, file.ID, 0)
	require.NoError(t, err)
	require.FileExists(t, cachedPage)

	// Replace the file with a different one under the same path.
	testgen.GenerateCBZ(t, libraryPath, "comic.cbz", testgen.CBZOptions{
		Title:     "Comic",
		PageCount: 5,
	})
	stat, err := os.Stat(cbzPath)
	require.NoError(t, err)
	require.NotEqual(t, file.FilesizeBytes, stat.Size(),
		"test requires a different file size to trigger change detection")

	require.NoError(t, tc.runScan())

	assert.NoFileExists(t, cachedPage,
		"cached pages of a changed CBZ must be dropped by the scan")
	assertUpdatedAtBumped(t, tc, file)
}

// TestScan_ChangedPDF_InvalidatesCachedPages covers the single-file rescan
// path (scan by file ID) and the PDF page cache.
func TestScan_ChangedPDF_InvalidatesCachedPages(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	_, cacheDir := withPageCaches(t, tc)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	pdfPath := testgen.GeneratePDF(t, libraryPath, "document.pdf", testgen.PDFOptions{PageCount: 2})
	file := scanSingleFile(t, tc)

	cachedPage := primeCachedPage(t, cacheDir, "pdf", file.ID)

	testgen.GeneratePDF(t, libraryPath, "document.pdf", testgen.PDFOptions{PageCount: 4})
	stat, err := os.Stat(pdfPath)
	require.NoError(t, err)
	require.NotEqual(t, file.FilesizeBytes, stat.Size(),
		"test requires a different file size to trigger change detection")

	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID}, nil)
	require.NoError(t, err)

	assert.NoFileExists(t, cachedPage,
		"cached pages of a changed PDF must be dropped by the scan")
	assertUpdatedAtBumped(t, tc, file)
}

// TestScan_UnchangedFile_KeepsCachedPages verifies that rescanning a file
// whose size and mtime are unchanged leaves its cached pages in place.
func TestScan_UnchangedFile_KeepsCachedPages(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	cbzCache, _ := withPageCaches(t, tc)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	cbzPath := testgen.GenerateCBZ(t, libraryPath, "comic.cbz", testgen.CBZOptions{
		Title:     "Comic",
		PageCount: 3,
	})
	file := scanSingleFile(t, tc)

	cachedPage, _, err := cbzCache.GetPage(cbzPath, file.ID, 0)
	require.NoError(t, err)

	require.NoError(t, tc.runScan())
	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID}, nil)
	require.NoError(t, err)

	assert.FileExists(t, cachedPage,
		"a rescan of an unchanged file must keep its cached pages")
}

// TestScan_Refresh_InvalidatesCachedPages verifies that a forced refresh
// drops cached pages even when size and mtime match. A refresh also bumps
// the file's updated_at, which is the browser's page cache key, so it is the
// manual way to clear stale pages after a same-size, same-mtime replacement.
func TestScan_Refresh_InvalidatesCachedPages(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	cbzCache, _ := withPageCaches(t, tc)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	cbzPath := testgen.GenerateCBZ(t, libraryPath, "comic.cbz", testgen.CBZOptions{
		Title:     "Comic",
		PageCount: 3,
	})
	file := scanSingleFile(t, tc)

	cachedPage, _, err := cbzCache.GetPage(cbzPath, file.ID, 0)
	require.NoError(t, err)

	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID, ForceRefresh: true}, nil)
	require.NoError(t, err)

	assert.NoFileExists(t, cachedPage,
		"a forced refresh must drop the file's cached pages")
	assertUpdatedAtBumped(t, tc, file)
}

// TestScan_Supplement_InvalidatesCachedPagesOnlyWhenChanged covers the
// supplement branch of the library scan. Supplements are created without a
// stored mtime, so the first scan treats one as changed; it must then record
// the size and mtime so later scans keep its cached pages. A real
// replacement must drop the cached pages and bump updated_at, the page URL's
// cache key.
func TestScan_Supplement_InvalidatesCachedPagesOnlyWhenChanged(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	_, cacheDir := withPageCaches(t, tc)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Book With Extras")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:   "Book With Extras",
		Authors: []string{"Author"},
	})
	mainFile := scanSingleFile(t, tc)

	// Insert the supplement the way the scanner creates one: size, no mtime.
	extrasPath := testgen.GeneratePDF(t, bookDir, "extras.pdf", testgen.PDFOptions{PageCount: 2})
	extrasStat, err := os.Stat(extrasPath)
	require.NoError(t, err)
	supplement := &models.File{
		LibraryID:     mainFile.LibraryID,
		BookID:        mainFile.BookID,
		Filepath:      extrasPath,
		FileType:      models.FileTypePDF,
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: extrasStat.Size(),
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, supplement))

	// The first scan cannot tell whether the supplement changed, so it
	// treats it as changed and records its size and mtime.
	require.NoError(t, tc.runScan())
	recorded, err := tc.bookService.RetrieveFile(tc.ctx, books.RetrieveFileOptions{ID: &supplement.ID})
	require.NoError(t, err)
	require.NotNil(t, recorded.FileModifiedAt, "the scan must record the supplement's mtime")

	// With size and mtime recorded, a scan of the unchanged supplement keeps
	// its cached pages.
	cachedPage := primeCachedPage(t, cacheDir, "pdf", supplement.ID)
	require.NoError(t, tc.runScan())
	assert.FileExists(t, cachedPage,
		"a scan of an unchanged supplement must keep its cached pages")

	// Replacing the supplement drops its cached pages and bumps updated_at.
	testgen.GeneratePDF(t, bookDir, "extras.pdf", testgen.PDFOptions{PageCount: 4})
	require.NoError(t, tc.runScan())
	assert.NoFileExists(t, cachedPage,
		"cached pages of a changed supplement must be dropped by the scan")
	assertUpdatedAtBumped(t, tc, recorded)
}

// TestScan_UnreadableReplacement_BumpsUpdatedAt covers a file replaced by an
// unreadable one twice in a row. Both parses fail with the same message, so
// the scan error itself does not change, but the content did: the cached
// pages must go and updated_at must still move so the page URLs change.
func TestScan_UnreadableReplacement_BumpsUpdatedAt(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	_, cacheDir := withPageCaches(t, tc)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	cbzPath := testgen.GenerateCBZ(t, libraryPath, "comic.cbz", testgen.CBZOptions{
		Title:     "Comic",
		PageCount: 3,
	})
	file := scanSingleFile(t, tc)

	require.NoError(t, os.WriteFile(cbzPath, []byte("not a zip archive"), 0600))
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID}, nil)
	require.Error(t, err)
	firstFailure, err := tc.bookService.RetrieveFile(tc.ctx, books.RetrieveFileOptions{ID: &file.ID})
	require.NoError(t, err)
	require.NotNil(t, firstFailure.ScanError)

	cachedPage := primeCachedPage(t, cacheDir, "cbz", file.ID)
	require.NoError(t, os.WriteFile(cbzPath, []byte("a different, longer file that is still not a zip archive"), 0600))
	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID}, nil)
	require.Error(t, err)

	secondFailure, err := tc.bookService.RetrieveFile(tc.ctx, books.RetrieveFileOptions{ID: &file.ID})
	require.NoError(t, err)
	require.NotNil(t, secondFailure.ScanError)
	require.Equal(t, *firstFailure.ScanError, *secondFailure.ScanError,
		"test requires both replacements to fail with the same message")
	assert.NoFileExists(t, cachedPage,
		"cached pages must be dropped even when the replacement cannot be parsed")
	assertUpdatedAtBumped(t, tc, firstFailure)
}

// TestScan_FailedPageCacheInvalidation_DoesNotAbortScan verifies that a page
// cache that cannot be cleared is logged, not fatal: the scan still records
// the changed file, so its updated_at (the page URL's cache key) moves.
func TestScan_FailedPageCacheInvalidation_DoesNotAbortScan(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions, so the invalidation cannot be made to fail")
	}
	tc := newTestContext(t)
	_, cacheDir := withPageCaches(t, tc)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	testgen.GenerateCBZ(t, libraryPath, "comic.cbz", testgen.CBZOptions{
		Title:     "Comic",
		PageCount: 3,
	})
	file := scanSingleFile(t, tc)

	// A read-only page directory makes os.RemoveAll fail on the page inside.
	cachedPage := primeCachedPage(t, cacheDir, "cbz", file.ID)
	pageDir := filepath.Dir(cachedPage)
	require.NoError(t, os.Chmod(pageDir, 0500))       //nolint:gosec // test needs a read-only directory
	t.Cleanup(func() { _ = os.Chmod(pageDir, 0755) }) //nolint:gosec // restore so t.TempDir can clean up

	testgen.GenerateCBZ(t, libraryPath, "comic.cbz", testgen.CBZOptions{
		Title:     "Comic",
		PageCount: 5,
	})
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID}, nil)
	require.NoError(t, err, "a failed page cache invalidation must not abort the scan")

	require.FileExists(t, cachedPage, "test requires the invalidation to have failed")
	assertUpdatedAtBumped(t, tc, file)
}
