package kobo

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCover_SetsCacheControlPrivateNoCache(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	bookService := books.NewService(db)

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book with a directory
	bookDir := filepath.Join(t.TempDir(), "Test Book")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	filePath := filepath.Join(bookDir, "test.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0o644))

	coverFilename := "test.epub.cover.jpg"
	coverPath := filepath.Join(bookDir, coverFilename)
	coverFile, err := os.Create(coverPath)
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(coverFile, image.NewRGBA(image.Rect(0, 0, 100, 150)), nil))
	require.NoError(t, coverFile.Close())

	mimeType := "image/jpeg"
	file := &models.File{
		LibraryID:          library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           filePath,
		FilesizeBytes:      1000,
		CoverImageFilename: &coverFilename,
		CoverMimeType:      &mimeType,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	h := &handler{bookService: bookService}

	// Use a resized request (w=100 h=150)
	imageID := fmt.Sprintf("shisho-%d", file.ID)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("imageId", "w", "h")
	c.SetParamValues(imageID, "100", "150")

	require.NoError(t, h.handleCover(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
	assert.NotEmpty(t, rec.Header().Get("Last-Modified"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestHandleCover_Returns304WhenIfModifiedSinceMatches(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	bookService := books.NewService(db)

	library := &models.Library{
		Name:                     "Test Library 304",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	bookDir := filepath.Join(t.TempDir(), "Test Book 304")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book 304",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book 304",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	filePath := filepath.Join(bookDir, "test.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0o644))

	coverFilename := "test.epub.cover.jpg"
	coverPath := filepath.Join(bookDir, coverFilename)
	coverFile, err := os.Create(coverPath)
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(coverFile, image.NewRGBA(image.Rect(0, 0, 100, 150)), nil))
	require.NoError(t, coverFile.Close())
	_ = coverPath

	mimeType := "image/jpeg"
	file := &models.File{
		LibraryID:          library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           filePath,
		FilesizeBytes:      1000,
		CoverImageFilename: &coverFilename,
		CoverMimeType:      &mimeType,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	h := &handler{bookService: bookService}
	imageID := fmt.Sprintf("shisho-%d", file.ID)

	// First GET.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("imageId", "w", "h")
	c1.SetParamValues(imageID, "100", "150")
	require.NoError(t, h.handleCover(c1))
	lastModified := rec1.Header().Get("Last-Modified")
	require.NotEmpty(t, lastModified)

	// Second GET with If-Modified-Since.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-Modified-Since", lastModified)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("imageId", "w", "h")
	c2.SetParamValues(imageID, "100", "150")
	require.NoError(t, h.handleCover(c2))

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())
}

func TestHandleCover_304SkipsResizeWork(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	bookService := books.NewService(db)

	library := &models.Library{
		Name:                     "Test Library Skip",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	bookDir := filepath.Join(t.TempDir(), "Test Book Skip")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book Skip",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book Skip",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	filePath := filepath.Join(bookDir, "test.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0o644))

	coverFilename := "test.epub.cover.jpg"
	coverPath := filepath.Join(bookDir, coverFilename)
	coverFile, err := os.Create(coverPath)
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(coverFile, image.NewRGBA(image.Rect(0, 0, 100, 150)), nil))
	require.NoError(t, coverFile.Close())

	mimeType := "image/jpeg"
	file := &models.File{
		LibraryID:          library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           filePath,
		FilesizeBytes:      1000,
		CoverImageFilename: &coverFilename,
		CoverMimeType:      &mimeType,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	h := &handler{bookService: bookService}
	imageID := fmt.Sprintf("shisho-%d", file.ID)

	// First GET to get Last-Modified from the valid cover.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("imageId", "w", "h")
	c1.SetParamValues(imageID, "100", "150")
	require.NoError(t, h.handleCover(c1))
	lastModified := rec1.Header().Get("Last-Modified")
	require.NotEmpty(t, lastModified)

	// Capture the cover file's original mtime.
	info, err := os.Stat(coverPath)
	require.NoError(t, err)
	originalMtime := info.ModTime()

	// Overwrite cover with garbage bytes (invalid image), but restore the original
	// mtime so the If-Modified-Since short-circuit still fires.
	require.NoError(t, os.WriteFile(coverPath, []byte("this is not a valid jpeg"), 0o644))
	require.NoError(t, os.Chtimes(coverPath, originalMtime, originalMtime))

	// Second GET with If-Modified-Since matching the original mtime.
	// If the handler tried to decode/resize the garbage bytes it would return
	// an error. Getting 304 proves the decode path was skipped.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-Modified-Since", lastModified)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("imageId", "w", "h")
	c2.SetParamValues(imageID, "100", "150")
	require.NoError(t, h.handleCover(c2))

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())

	// Sanity-check: confirm the garbage file would indeed cause an error if the
	// resize path were taken (i.e. without If-Modified-Since).
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec3 := httptest.NewRecorder()
	c3 := e.NewContext(req3, rec3)
	c3.SetParamNames("imageId", "w", "h")
	c3.SetParamValues(imageID, "100", "150")
	err3 := h.handleCover(c3)
	// Without IMS the resize path is taken and the garbage bytes cause an error,
	// confirming that the 304 above genuinely skipped image decoding.
	require.Error(t, err3, "expected decode error on garbage cover to confirm 304 skipped resize")
}
