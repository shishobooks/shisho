package ereader

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
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCover_SetsCacheControlPrivateNoCache(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	e := echo.New()

	// Create admin role-based user
	var roleID int
	err := db.QueryRow("SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?)",
		"ereader_cover_user", "hash", roleID, true)
	require.NoError(t, err)

	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE username = 'ereader_cover_user'").Scan(&userID)
	require.NoError(t, err)

	// Grant all-library access (null library_id = all libraries).
	_, err = db.ExecContext(ctx, "INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)", userID)
	require.NoError(t, err)

	apiKeyService := apikeys.NewService(db)
	apiKey, err := apiKeyService.Create(ctx, userID, "Test Key")
	require.NoError(t, err)

	libraryService := libraries.NewService(db)
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	require.NoError(t, libraryService.CreateLibrary(ctx, library))

	bookDir := filepath.Join(t.TempDir(), "Test Book")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	bookService := books.NewService(db)
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

	h := &handler{
		db:             db,
		bookService:    bookService,
		libraryService: libraryService,
	}

	// Inject API key into context (as middleware would do).
	apiKeyCtx := context.WithValue(ctx, contextKeyAPIKey, apiKey)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(apiKeyCtx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("bookId")
	c.SetParamValues(strconv.Itoa(book.ID))

	require.NoError(t, h.Cover(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
	assert.NotEmpty(t, rec.Header().Get("Last-Modified"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestCover_Returns304WhenIfModifiedSinceMatches(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	e := echo.New()

	var roleID int
	err := db.QueryRow("SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?)",
		"ereader_304_user", "hash", roleID, true)
	require.NoError(t, err)

	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE username = 'ereader_304_user'").Scan(&userID)
	require.NoError(t, err)

	// Grant all-library access (null library_id = all libraries).
	_, err = db.ExecContext(ctx, "INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)", userID)
	require.NoError(t, err)

	apiKeyService := apikeys.NewService(db)
	apiKey, err := apiKeyService.Create(ctx, userID, "Test Key 304")
	require.NoError(t, err)

	libraryService := libraries.NewService(db)
	library := &models.Library{
		Name:                     "Test Library 304",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	require.NoError(t, libraryService.CreateLibrary(ctx, library))

	bookDir := filepath.Join(t.TempDir(), "Test Book 304")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	bookService := books.NewService(db)
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
	require.NoError(t, jpeg.Encode(coverFile, image.NewRGBA(image.Rect(0, 0, 10, 10)), nil))
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

	h := &handler{
		db:             db,
		bookService:    bookService,
		libraryService: libraryService,
	}

	apiKeyCtx := context.WithValue(ctx, contextKeyAPIKey, apiKey)

	// First GET.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1 = req1.WithContext(apiKeyCtx)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("bookId")
	c1.SetParamValues(strconv.Itoa(book.ID))
	require.NoError(t, h.Cover(c1))
	lastModified := rec1.Header().Get("Last-Modified")
	require.NotEmpty(t, lastModified)

	// Second GET with If-Modified-Since.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2 = req2.WithContext(apiKeyCtx)
	req2.Header.Set("If-Modified-Since", lastModified)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("bookId")
	c2.SetParamValues(strconv.Itoa(book.ID))
	require.NoError(t, h.Cover(c2))

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())
}
