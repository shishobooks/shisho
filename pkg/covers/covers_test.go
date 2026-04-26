package covers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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

	epub := withCover(1, models.FileTypeEPUB, "epub.cover.jpg")
	cbz := withCover(2, models.FileTypeCBZ, "cbz.cover.jpg")
	pdf := withCover(3, models.FileTypePDF, "pdf.cover.jpg")
	m4b := withCover(4, models.FileTypeM4B, "m4b.cover.jpg")
	bookNoCover := withCover(5, models.FileTypeEPUB, "")
	audioNoCover := withCover(6, models.FileTypeM4B, "")

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

	err := ServeBookCover(c, files, "book")
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

	require.NoError(t, ServeBookCover(c, files, "book"))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
	assert.Equal(t, coverBytes, rec.Body.Bytes())
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

	err := ServeBookCover(c, files, "book")
	require.Error(t, err)
	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr)
	assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
}
