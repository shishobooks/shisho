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
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveBulkZip builds a completed bulk download whose zip is written with the
// given mode, and serves it through the error handler the way the server does.
func serveBulkZip(t *testing.T, mode os.FileMode) *httptest.ResponseRecorder {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()

	lib := insertTestLibrary(t, db, "Zip Lib")
	book := &models.Book{LibraryID: lib.ID, Title: "Test", TitleSource: models.DataSourceFilepath, SortTitle: "Test", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/tmp"}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{LibraryID: lib.ID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: "/tmp/fake.epub", FilesizeBytes: 100}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)
	job := &models.Job{
		Type:   models.JobTypeBulkDownload,
		Status: models.JobStatusCompleted,
		Data:   fmt.Sprintf(`{"fingerprint_hash":"zipped","file_ids":[%d],"file_count":1}`, file.ID),
	}
	_, err = db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)

	user := &models.User{Username: "zipuser", PasswordHash: "hash", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.UserLibraryAccess{UserID: user.ID, LibraryID: &lib.ID}).Exec(ctx)
	require.NoError(t, err)
	require.NoError(t, db.NewSelect().Model(user).Relation("Role").Relation("Role.Permissions").Relation("LibraryAccess").Where("u.id = ?", user.ID).Scan(ctx))

	cache := downloadcache.NewCache(t.TempDir(), 1<<30)
	zipPath := cache.BulkZipPath("zipped")
	require.NoError(t, os.MkdirAll(filepath.Dir(zipPath), 0o755))
	require.NoError(t, os.WriteFile(zipPath, []byte("PK zip bytes"), mode))
	t.Cleanup(func() { _ = os.Chmod(zipPath, 0o644) })

	h := &handler{jobService: NewService(db), db: db, downloadCache: cache}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(job.ID))
	auth.SetUser(c, user)
	if err := h.download(c); err != nil {
		errcodes.NewHandler().Handle(err, c)
	}
	return rec
}

func TestDownload_ServesZip(t *testing.T) {
	t.Parallel()
	rec := serveBulkZip(t, 0o644)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "PK zip bytes", rec.Body.String())
	assert.Equal(t, "application/zip", rec.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="shisho-download-1-books.zip"`, rec.Header().Get("Content-Disposition"))
	assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
}

// A zip that exists but cannot be opened is a 500 with no attachment header.
func TestDownload_UnreadableZipIsServerError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}
	rec := serveBulkZip(t, 0o000)
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Empty(t, rec.Header().Get("Content-Disposition"))
}
