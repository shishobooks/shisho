package chapters

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func newTestEcho(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	return e
}

// TestReplaceChapters_TriggersReviewRecompute verifies that a successful PUT
// chapters request calls RecomputeReviewedForFile so that files.reviewed is
// refreshed when "chapters" is in the active audio_fields criteria.
func TestReplaceChapters_TriggersReviewRecompute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)

	// Seed library, book, and an M4B file (chapters only apply to audio_fields)
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Audiobook",
		Filepath:        t.TempDir(),
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Audiobook",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/test.m4b",
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Configure criteria: only "chapters" required so adding one chapter makes the file complete
	appSettingsSvc := appsettings.NewService(db)
	err = review.Save(ctx, appSettingsSvc, review.Criteria{
		BookFields:  []string{},
		AudioFields: []string{review.FieldChapters},
	})
	require.NoError(t, err)

	// Build handler with appSettings wired so recompute is active
	bookSvc := books.NewService(db).WithAppSettings(appSettingsSvc)
	h := &handler{
		chapterService: NewService(db),
		bookService:    bookSvc,
	}

	// Build PUT request with one chapter
	dur := int64(0)
	payload := ReplaceChaptersPayload{
		Chapters: []ChapterInput{
			{Title: "Chapter 1", StartTimestampMs: &dur},
		},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e := newTestEcho(t)
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(file.ID))

	// Inject a user with library access
	user := &models.User{
		Username:      "testuser",
		PasswordHash:  "hash",
		RoleID:        1,
		IsActive:      true,
		LibraryAccess: []*models.UserLibraryAccess{{UserID: 0, LibraryID: &library.ID}},
	}
	c.Set("user", user)

	require.NoError(t, h.replace(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify reviewed was recomputed to true (chapters criterion is now satisfied)
	var after models.File
	require.NoError(t, db.NewSelect().Model(&after).Where("f.id = ?", file.ID).Scan(ctx))
	require.NotNil(t, after.Reviewed, "reviewed should not be nil after recompute")
	assert.True(t, *after.Reviewed, "file should be reviewed=true after chapters added with chapters-only criteria")
}
