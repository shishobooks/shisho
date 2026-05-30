package covers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectFile(t *testing.T) {
	t.Parallel()

	withCover := func(id int, fileType, cover string) *models.File {
		var ptr *string
		if cover != "" {
			ptr = &cover
		}
		return &models.File{ID: id, FileType: fileType, CoverImageFilename: ptr}
	}
	supplementWithCover := func(id int, fileType, cover string) *models.File {
		f := withCover(id, fileType, cover)
		f.FileRole = models.FileRoleSupplement
		return f
	}
	preferredWithCover := func(id int, fileType, cover string) *models.File {
		f := withCover(id, fileType, cover)
		f.IsPreferredCover = true
		return f
	}

	epub := withCover(1, models.FileTypeEPUB, "epub.cover.jpg")
	cbz := withCover(2, models.FileTypeCBZ, "cbz.cover.jpg")
	pdf := withCover(3, models.FileTypePDF, "pdf.cover.jpg")
	m4b := withCover(4, models.FileTypeM4B, "m4b.cover.jpg")
	bookNoCover := withCover(5, models.FileTypeEPUB, "")
	audioNoCover := withCover(6, models.FileTypeM4B, "")
	pdfSupplement := supplementWithCover(7, models.FileTypePDF, "supp.cover.jpg")
	epubSupplement := supplementWithCover(8, models.FileTypeEPUB, "supp.cover.jpg")
	preferredCBZ := preferredWithCover(9, models.FileTypeCBZ, "cbz2.cover.jpg")
	preferredM4B := preferredWithCover(10, models.FileTypeM4B, "m4b2.cover.jpg")

	tests := []struct {
		name           string
		files          []*models.File
		aspectRatio    string
		expectedFileID int // 0 means nil
	}{
		{
			name:           "prefers book file in default mode",
			files:          []*models.File{m4b, epub},
			aspectRatio:    "book",
			expectedFileID: epub.ID,
		},
		{
			name:           "falls back to audiobook in default mode",
			files:          []*models.File{m4b},
			aspectRatio:    "book",
			expectedFileID: m4b.ID,
		},
		{
			name:           "prefers audiobook file in audiobook mode",
			files:          []*models.File{epub, m4b},
			aspectRatio:    "audiobook",
			expectedFileID: m4b.ID,
		},
		{
			name:           "falls back to book file in audiobook mode",
			files:          []*models.File{epub},
			aspectRatio:    "audiobook",
			expectedFileID: epub.ID,
		},
		{
			name:           "audiobook_fallback_book behaves like audiobook",
			files:          []*models.File{epub, m4b},
			aspectRatio:    "audiobook_fallback_book",
			expectedFileID: m4b.ID,
		},
		{
			name:           "book_fallback_audiobook behaves like default",
			files:          []*models.File{m4b, epub},
			aspectRatio:    "book_fallback_audiobook",
			expectedFileID: epub.ID,
		},
		{
			name:           "skips files with no cover",
			files:          []*models.File{bookNoCover, m4b},
			aspectRatio:    "book",
			expectedFileID: m4b.ID,
		},
		{
			name:           "returns nil when no covers exist",
			files:          []*models.File{bookNoCover, audioNoCover},
			aspectRatio:    "book",
			expectedFileID: 0,
		},
		{
			name:           "treats CBZ as a book file",
			files:          []*models.File{cbz},
			aspectRatio:    "book",
			expectedFileID: cbz.ID,
		},
		{
			name:           "treats PDF as a book file",
			files:          []*models.File{pdf},
			aspectRatio:    "book",
			expectedFileID: pdf.ID,
		},
		{
			name:           "returns nil for empty files",
			files:          nil,
			aspectRatio:    "book",
			expectedFileID: 0,
		},
		{
			name:           "unknown aspect ratio falls through to default",
			files:          []*models.File{m4b, epub},
			aspectRatio:    "weird",
			expectedFileID: epub.ID,
		},
		{
			name:           "skips PDF supplement when M4B main has cover",
			files:          []*models.File{pdfSupplement, m4b},
			aspectRatio:    "book",
			expectedFileID: m4b.ID,
		},
		{
			name:           "skips EPUB supplement when M4B main has cover",
			files:          []*models.File{epubSupplement, m4b},
			aspectRatio:    "book",
			expectedFileID: m4b.ID,
		},
		{
			name:           "skips supplement even when no main has cover",
			files:          []*models.File{pdfSupplement, bookNoCover},
			aspectRatio:    "book",
			expectedFileID: 0,
		},
		{
			name:           "supplement also skipped in audiobook mode",
			files:          []*models.File{pdfSupplement, m4b},
			aspectRatio:    "audiobook",
			expectedFileID: m4b.ID,
		},
		// Preferred cover tests
		{
			name:           "preferred ebook file wins over first in bucket",
			files:          []*models.File{epub, preferredCBZ},
			aspectRatio:    "book",
			expectedFileID: preferredCBZ.ID,
		},
		{
			name:           "preferred audiobook file wins over first in bucket",
			files:          []*models.File{m4b, preferredM4B},
			aspectRatio:    "audiobook",
			expectedFileID: preferredM4B.ID,
		},
		{
			name:           "preferred ebook in non-selected bucket does not affect audiobook selection",
			files:          []*models.File{epub, preferredCBZ, m4b},
			aspectRatio:    "audiobook",
			expectedFileID: m4b.ID,
		},
		{
			name:           "preferred audiobook in non-selected bucket does not affect ebook selection",
			files:          []*models.File{epub, cbz, preferredM4B},
			aspectRatio:    "book",
			expectedFileID: epub.ID,
		},
		{
			name:           "no preferred falls back to first file (existing behavior)",
			files:          []*models.File{epub, cbz},
			aspectRatio:    "book",
			expectedFileID: epub.ID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SelectFile(tt.files, tt.aspectRatio)
			if tt.expectedFileID == 0 {
				assert.Nil(t, got)
				return
			}
			if assert.NotNil(t, got) {
				assert.Equal(t, tt.expectedFileID, got.ID)
			}
		})
	}
}

func TestServeBookCover_NotFoundWhenNoCover(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	files := []*models.File{
		{ID: 1, FileType: models.FileTypeEPUB, CoverImageFilename: nil},
	}

	err := ServeBookCover(c, files, "book", CacheControlNoCache)
	require.Error(t, err)
	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr)
	assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
}

func TestServeBookCover_ServesCoverFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bookPath := filepath.Join(dir, "book.epub")
	require.NoError(t, os.WriteFile(bookPath, []byte("epub-bytes"), 0o644))
	coverName := "book.epub.cover.jpg"
	coverBytes := []byte("\xff\xd8\xff\xe0jpeg-bytes")
	require.NoError(t, os.WriteFile(filepath.Join(dir, coverName), coverBytes, 0o644))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	files := []*models.File{
		{ID: 1, FileType: models.FileTypeEPUB, Filepath: bookPath, CoverImageFilename: &coverName},
	}

	require.NoError(t, ServeBookCover(c, files, "book", CacheControlNoCache))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
	assert.NotEmpty(t, rec.Header().Get("ETag"))
	// Last-Modified is intentionally NOT emitted: it would re-enable IMS-based
	// revalidation inside http.ServeContent, which can return stale 304s when
	// the selected cover file changes (hybrid book + aspect-ratio change, or a
	// file removed from the book) to one whose cover has an older mtime.
	assert.Empty(t, rec.Header().Get("Last-Modified"))
	assert.Equal(t, coverBytes, rec.Body.Bytes())
}

func TestServeBookCover_ETagIncludesFileIDAndMtime(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bookPath := filepath.Join(dir, "book.epub")
	require.NoError(t, os.WriteFile(bookPath, []byte("epub-bytes"), 0o644))
	coverName := "book.epub.cover.jpg"
	coverPath := filepath.Join(dir, coverName)
	require.NoError(t, os.WriteFile(coverPath, []byte("jpeg"), 0o644))

	// Pin the mtime so the assertion is stable.
	pinned := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(coverPath, pinned, pinned))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	files := []*models.File{
		{ID: 42, FileType: models.FileTypeEPUB, Filepath: bookPath, CoverImageFilename: &coverName},
	}

	require.NoError(t, ServeBookCover(c, files, "book", CacheControlNoCache))
	assert.Equal(t, fmt.Sprintf(`"%d-%d"`, 42, pinned.Unix()), rec.Header().Get("ETag"))
}

func TestServeBookCover_Returns304WhenIfNoneMatchMatches(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bookPath := filepath.Join(dir, "book.epub")
	require.NoError(t, os.WriteFile(bookPath, []byte("epub-bytes"), 0o644))
	coverName := "book.epub.cover.jpg"
	coverBytes := []byte("\xff\xd8\xff\xe0jpeg-bytes")
	require.NoError(t, os.WriteFile(filepath.Join(dir, coverName), coverBytes, 0o644))

	e := echo.New()
	files := []*models.File{
		{ID: 1, FileType: models.FileTypeEPUB, Filepath: bookPath, CoverImageFilename: &coverName},
	}

	// First GET to capture the ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	require.NoError(t, ServeBookCover(c1, files, "book", CacheControlNoCache))
	etag := rec1.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// Revalidate with If-None-Match.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)

	require.NoError(t, ServeBookCover(c2, files, "book", CacheControlNoCache))
	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())
	assert.Equal(t, etag, rec2.Header().Get("ETag"))
	assert.Equal(t, "private, no-cache", rec2.Header().Get("Cache-Control"))
}

// Regression: when the library admin flips CoverAspectRatio on a hybrid book
// (EPUB + M4B, both with covers), selectCoverFile picks a different file. If
// the newly-selected cover's mtime happens to be older than the previously-
// served cover's mtime, an ETag computed from mtime alone could let a client
// holding the old validator serve the previous cover indefinitely. The ETag
// must bake in file identity so it bumps even when mtime goes backwards.
func TestServeBookCover_AspectRatioChangeInvalidatesEtagEvenWhenNewCoverMtimeIsOlder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	epubPath := filepath.Join(dir, "book.epub")
	require.NoError(t, os.WriteFile(epubPath, []byte("epub"), 0o644))
	epubCover := "book.epub.cover.jpg"
	epubCoverPath := filepath.Join(dir, epubCover)
	require.NoError(t, os.WriteFile(epubCoverPath, []byte("epub-cover"), 0o644))

	m4bPath := filepath.Join(dir, "book.m4b")
	require.NoError(t, os.WriteFile(m4bPath, []byte("m4b"), 0o644))
	m4bCover := "book.m4b.cover.jpg"
	m4bCoverPath := filepath.Join(dir, m4bCover)
	require.NoError(t, os.WriteFile(m4bCoverPath, []byte("m4b-cover"), 0o644))

	// M4B cover is OLDER than EPUB cover — exactly the case that breaks
	// mtime-only revalidation.
	older := time.Now().Add(-72 * time.Hour)
	require.NoError(t, os.Chtimes(m4bCoverPath, older, older))
	newer := time.Now().Add(-1 * time.Hour)
	require.NoError(t, os.Chtimes(epubCoverPath, newer, newer))

	files := []*models.File{
		{ID: 11, FileType: models.FileTypeEPUB, Filepath: epubPath, CoverImageFilename: &epubCover},
		{ID: 22, FileType: models.FileTypeM4B, Filepath: m4bPath, CoverImageFilename: &m4bCover},
	}

	e := echo.New()

	// Aspect ratio "book" → EPUB cover served, capture its ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	require.NoError(t, ServeBookCover(c1, files, "book", CacheControlNoCache))
	require.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, []byte("epub-cover"), rec1.Body.Bytes())
	etagEPUB := rec1.Header().Get("ETag")
	require.NotEmpty(t, etagEPUB)
	assert.True(t, strings.HasPrefix(etagEPUB, `"11-`),
		"ETag should encode the EPUB file ID (11) in <id-mtime> format, got %q", etagEPUB)

	// Aspect ratio "audiobook" → M4B cover should be served. Client revalidates
	// with the EPUB ETag — must NOT 304, because the served file is different
	// even though the M4B cover's mtime is older.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-None-Match", etagEPUB)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	require.NoError(t, ServeBookCover(c2, files, "audiobook", CacheControlNoCache))
	assert.Equal(t, http.StatusOK, rec2.Code,
		"expected 200 after aspect-ratio change (ETag must change with file identity, not just mtime)")
	etagM4B := rec2.Header().Get("ETag")
	assert.NotEmpty(t, etagM4B)
	assert.NotEqual(t, etagEPUB, etagM4B, "ETag must change when the selected file changes")
	assert.True(t, strings.HasPrefix(etagM4B, `"22-`),
		"ETag should encode the M4B file ID (22) in <id-mtime> format, got %q", etagM4B)
	assert.Equal(t, []byte("m4b-cover"), rec2.Body.Bytes())
}

func TestCacheKey_ReturnsFileIDAndUpdatedAt(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	cover := "book.epub.cover.jpg"
	files := []*models.File{
		{ID: 42, FileType: models.FileTypeEPUB, CoverImageFilename: &cover, UpdatedAt: updatedAt},
	}

	got := CacheKey(files, "book")
	assert.Equal(t, fmt.Sprintf("%d-%d", 42, updatedAt.Unix()), got)
}

func TestCacheKey_ReturnsEmptyWhenNoCover(t *testing.T) {
	t.Parallel()

	files := []*models.File{
		{ID: 1, FileType: models.FileTypeEPUB, CoverImageFilename: nil},
	}

	assert.Empty(t, CacheKey(files, "book"))
}

func TestCacheKey_ReturnsEmptyForNilFiles(t *testing.T) {
	t.Parallel()
	assert.Empty(t, CacheKey(nil, "book"))
}

func TestCacheKey_RespectsAspectRatio(t *testing.T) {
	t.Parallel()

	epubCover := "epub.cover.jpg"
	m4bCover := "m4b.cover.jpg"
	epubTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	m4bTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	files := []*models.File{
		{ID: 10, FileType: models.FileTypeEPUB, CoverImageFilename: &epubCover, UpdatedAt: epubTime},
		{ID: 20, FileType: models.FileTypeM4B, CoverImageFilename: &m4bCover, UpdatedAt: m4bTime},
	}

	bookKey := CacheKey(files, "book")
	audioKey := CacheKey(files, "audiobook")

	assert.Equal(t, fmt.Sprintf("%d-%d", 10, epubTime.Unix()), bookKey)
	assert.Equal(t, fmt.Sprintf("%d-%d", 20, m4bTime.Unix()), audioKey)
	assert.NotEqual(t, bookKey, audioKey)
}

func TestServeBookCover_UsesCacheControlParam(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bookPath := filepath.Join(dir, "book.epub")
	require.NoError(t, os.WriteFile(bookPath, []byte("epub"), 0o644))
	coverName := "book.epub.cover.jpg"
	require.NoError(t, os.WriteFile(filepath.Join(dir, coverName), []byte("jpeg"), 0o644))

	files := []*models.File{
		{ID: 1, FileType: models.FileTypeEPUB, Filepath: bookPath, CoverImageFilename: &coverName},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, ServeBookCover(c, files, "book", "private, max-age=31536000, immutable"))
	assert.Equal(t, "private, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
}

func TestServeBookCover_NotFoundWhenCoverFileMissingOnDisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bookPath := filepath.Join(dir, "book.epub")
	require.NoError(t, os.WriteFile(bookPath, []byte("epub-bytes"), 0o644))
	// CoverImageFilename is set, but the cover file itself isn't on disk —
	// e.g. it was deleted out from under the DB. Should 404, not 500.
	coverName := "book.epub.cover.jpg"

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	files := []*models.File{
		{ID: 1, FileType: models.FileTypeEPUB, Filepath: bookPath, CoverImageFilename: &coverName},
	}

	err := ServeBookCover(c, files, "book", CacheControlNoCache)
	require.Error(t, err)
	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr)
	assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
}
