package jobs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownload_SetsCacheControlNoStore(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	// Create a library and a file for the library-access check.
	lib := insertTestLibrary(t, db, "Test Lib")

	file := &models.File{
		LibraryID:     lib.ID,
		BookID:        0,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/fake.epub",
		FilesizeBytes: 100,
	}
	// Need a book for the FK. Seed one.
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Test",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/tmp",
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file.BookID = book.ID
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Set up the download cache dir and create the expected zip file.
	cacheDir := t.TempDir()
	dlCache := downloadcache.NewCache(cacheDir, 1<<30)

	fpHash := "testfingerprint123"
	bulkDir := filepath.Join(cacheDir, "bulk")
	require.NoError(t, os.MkdirAll(bulkDir, 0o755))
	zipPath := filepath.Join(bulkDir, fpHash+".zip")
	require.NoError(t, os.WriteFile(zipPath, []byte("PK\x03\x04fake zip"), 0o644))

	// Create a completed bulk-download job.
	jobData := fmt.Sprintf(`{"fingerprint_hash":"%s","file_ids":[%d],"file_count":1}`, fpHash, file.ID)
	job := &models.Job{
		Type:   models.JobTypeBulkDownload,
		Status: models.JobStatusCompleted,
		Data:   jobData,
	}
	_, err = db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)

	// Create an admin user with library access.
	user := &models.User{
		Username:     "dluser",
		PasswordHash: "hash",
		RoleID:       1, // admin
		IsActive:     true,
	}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	access := &models.UserLibraryAccess{UserID: user.ID, LibraryID: &lib.ID}
	_, err = db.NewInsert().Model(access).Exec(ctx)
	require.NoError(t, err)
	user.LibraryAccess = []*models.UserLibraryAccess{access}

	// Load role with permissions so the handler's permission check passes.
	err = db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Where("u.id = ?", user.ID).
		Scan(ctx)
	require.NoError(t, err)

	h := &handler{
		jobService:    NewService(db),
		db:            db,
		downloadCache: dlCache,
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+strconv.Itoa(job.ID)+"/download", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(job.ID))
	c.Set("user", user)

	require.NoError(t, h.download(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
}
