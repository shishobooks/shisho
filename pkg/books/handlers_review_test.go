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
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestSetFileReview_SetsOverride(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	// Seed library, book, and file
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		Filepath:        "/tmp",
		TitleSource:     "file",
		SortTitle:       "T",
		SortTitleSource: "file",
		AuthorSource:    "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/test.epub",
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Build handler
	svc := NewService(db).WithAppSettings(appsettings.NewService(db))
	h := &handler{
		bookService:        svc,
		appSettingsService: appsettings.NewService(db),
	}

	// Build Echo context with PATCH body {"override":"reviewed"}
	e := newTestEchoBooks(t)
	payload := map[string]string{"override": models.ReviewOverrideReviewed}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(file.ID))

	// Create user with library access
	user := setupTestUser(t, db, library.ID, true)
	c.Set("user", user)

	err = h.setFileReview(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify the file's review_override and review_overridden_at are set
	var updated models.File
	err = db.NewSelect().Model(&updated).Where("f.id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.ReviewOverride)
	assert.Equal(t, models.ReviewOverrideReviewed, *updated.ReviewOverride)
	assert.NotNil(t, updated.ReviewOverriddenAt)
}

func TestSetFileReview_ClearsOverride(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	// Seed library, book, and file
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		Filepath:        "/tmp",
		TitleSource:     "file",
		SortTitle:       "T",
		SortTitleSource: "file",
		AuthorSource:    "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/test.epub",
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Seed an existing override so we can verify it gets cleared
	overrideVal := models.ReviewOverrideReviewed
	_, err = db.NewUpdate().
		Model((*models.File)(nil)).
		Set("review_override = ?", overrideVal).
		Where("id = ?", file.ID).
		Exec(ctx)
	require.NoError(t, err)

	// Build handler
	svc := NewService(db).WithAppSettings(appsettings.NewService(db))
	h := &handler{
		bookService:        svc,
		appSettingsService: appsettings.NewService(db),
	}

	// Build Echo context with PATCH body {"override":null}
	e := newTestEchoBooks(t)
	body := []byte(`{"override":null}`)
	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(file.ID))

	// Create user with library access
	user := setupTestUser(t, db, library.ID, true)
	c.Set("user", user)

	err = h.setFileReview(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify the file's review_override and review_overridden_at are cleared
	var updated models.File
	err = db.NewSelect().Model(&updated).Where("f.id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Nil(t, updated.ReviewOverride)
	assert.Nil(t, updated.ReviewOverriddenAt)
}

func TestSetBookReview_CascadesToAllFiles(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		Filepath:        "/tmp",
		TitleSource:     "file",
		SortTitle:       "T",
		SortTitleSource: "file",
		AuthorSource:    "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	for _, ft := range []string{models.FileTypeEPUB, models.FileTypeM4B} {
		f := &models.File{
			LibraryID:     library.ID,
			BookID:        book.ID,
			Filepath:      "/tmp/" + ft,
			FileType:      ft,
			FileRole:      models.FileRoleMain,
			FilesizeBytes: 1,
		}
		_, err = db.NewInsert().Model(f).Exec(ctx)
		require.NoError(t, err)
	}

	svc := NewService(db).WithAppSettings(appsettings.NewService(db))
	h := &handler{
		bookService:        svc,
		appSettingsService: appsettings.NewService(db),
	}

	body := []byte(`{"override":"reviewed"}`)
	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := newTestEchoBooks(t)
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(book.ID))

	user := setupTestUser(t, db, library.ID, true)
	c.Set("user", user)

	require.NoError(t, h.setBookReview(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var rows []*models.File
	require.NoError(t, db.NewSelect().Model(&rows).Where("f.book_id = ?", book.ID).Scan(ctx))
	for _, f := range rows {
		require.NotNil(t, f.ReviewOverride)
		assert.Equal(t, "reviewed", *f.ReviewOverride)
	}
}

func TestBulkSetReview_AppliesToAllSpecifiedBooks(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	bookIDs := make([]int, 0, 2)
	for i := 0; i < 2; i++ {
		book := &models.Book{
			LibraryID:       library.ID,
			Title:           "T" + strconv.Itoa(i),
			Filepath:        "/tmp/book" + strconv.Itoa(i),
			TitleSource:     "file",
			SortTitle:       "T",
			SortTitleSource: "file",
			AuthorSource:    "file",
		}
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)
		bookIDs = append(bookIDs, book.ID)

		f := &models.File{
			LibraryID:     library.ID,
			BookID:        book.ID,
			Filepath:      "/tmp/book" + strconv.Itoa(i) + "/file.epub",
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			FilesizeBytes: 1,
		}
		_, err = db.NewInsert().Model(f).Exec(ctx)
		require.NoError(t, err)
	}

	svc := NewService(db).WithAppSettings(appsettings.NewService(db))
	h := &handler{
		bookService:        svc,
		appSettingsService: appsettings.NewService(db),
	}

	bodyStr := `{"book_ids":[` + strconv.Itoa(bookIDs[0]) + `,` + strconv.Itoa(bookIDs[1]) + `],"override":"reviewed"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(bodyStr)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := newTestEchoBooks(t)
	c := e.NewContext(req, rec)

	user := setupTestUser(t, db, library.ID, true)
	c.Set("user", user)

	require.NoError(t, h.bulkSetReview(c))
	assert.Equal(t, http.StatusNoContent, rec.Code)

	var files []*models.File
	require.NoError(t, db.NewSelect().Model(&files).Where("f.book_id IN (?)", bun.List(bookIDs)).Scan(ctx))
	for _, f := range files {
		require.NotNil(t, f.ReviewOverride)
		assert.Equal(t, "reviewed", *f.ReviewOverride)
	}
}
