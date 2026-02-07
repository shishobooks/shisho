package books

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetPrimaryFile_SetsPrimaryFileSuccessfully(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two files
	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file1.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file2.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Create admin user with library access
	user := setupTestUser(t, db, library.ID, true)

	// Setup handler
	h := &handler{bookService: svc}
	e := echo.New()

	payload := map[string]int{"file_id": file2.ID}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(book.ID))
	c.Set("user", user)

	err = h.setPrimaryFile(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify the book was updated
	var updatedBook models.Book
	err = db.NewSelect().Model(&updatedBook).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updatedBook.PrimaryFileID)
	assert.Equal(t, file2.ID, *updatedBook.PrimaryFileID)
}

func TestSetPrimaryFile_RejectsFileFromDifferentBook(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create a file for the first book
	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file1.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	// Create another book with a file
	otherBook := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/other",
		Title:           "Other Book",
		SortTitle:       "Other Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(otherBook).Exec(ctx)
	require.NoError(t, err)

	otherFile := &models.File{
		LibraryID:     library.ID,
		BookID:        otherBook.ID,
		Filepath:      "/tmp/test/other/file.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, otherFile)
	require.NoError(t, err)

	// Create admin user with library access
	user := setupTestUser(t, db, library.ID, true)

	// Setup handler
	h := &handler{bookService: svc}
	e := echo.New()

	// Try to set other book's file as primary for our book
	payload := map[string]int{"file_id": otherFile.ID}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(book.ID))
	c.Set("user", user)

	err = h.setPrimaryFile(c)
	require.Error(t, err)

	// Should be a 400 Bad Request
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok, "Expected echo.HTTPError, got %T", err)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func TestSetPrimaryFile_RejectsZeroFileID(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create a file so the book exists with data
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, file)
	require.NoError(t, err)

	// Create admin user with library access
	user := setupTestUser(t, db, library.ID, true)

	// Setup handler
	h := &handler{bookService: svc}
	e := echo.New()

	// Send file_id: 0 which should be rejected by validation
	payload := map[string]int{"file_id": 0}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(book.ID))
	c.Set("user", user)

	err = h.setPrimaryFile(c)
	require.Error(t, err)

	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok, "Expected echo.HTTPError, got %T", err)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}
