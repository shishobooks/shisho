package books

import (
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/shishobooks/shisho/pkg/models"
)

// seedBookWithFileCover creates a library, book, and file with a real cover
// image on disk. Returns the file ID and the on-disk cover path.
func seedBookWithFileCover(t *testing.T, ctx context.Context, db *bun.DB) (fileID int, coverPath string) {
	t.Helper()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

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
	coverPath = filepath.Join(bookDir, coverFilename)
	coverFile, err := os.Create(coverPath)
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(coverFile, image.NewRGBA(image.Rect(0, 0, 10, 10)), nil))
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

	return file.ID, coverPath
}

func TestFileCover_SetsCacheControlPrivateNoCache(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	h := &handler{bookService: NewService(db)}

	fileID, _ := seedBookWithFileCover(t, ctx, db)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(fileID))

	require.NoError(t, h.fileCover(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
	assert.NotEmpty(t, rec.Header().Get("Last-Modified"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestFileCover_Returns304WhenIfModifiedSinceMatches(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()
	e := echo.New()
	h := &handler{bookService: NewService(db)}

	fileID, _ := seedBookWithFileCover(t, ctx, db)

	// First GET to capture Last-Modified.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("id")
	c1.SetParamValues(strconv.Itoa(fileID))
	require.NoError(t, h.fileCover(c1))
	lastModified := rec1.Header().Get("Last-Modified")
	require.NotEmpty(t, lastModified)

	// Second GET with If-Modified-Since.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-Modified-Since", lastModified)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(strconv.Itoa(fileID))
	require.NoError(t, h.fileCover(c2))

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())
}
