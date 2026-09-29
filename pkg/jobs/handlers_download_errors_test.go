package jobs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// A completed bulk download whose zip has left the cache returns a 404 that
// says "not found" once.
func TestDownload_ExpiredZipReturnsDownloadFileNotFound(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	lib := insertTestLibrary(t, db, "Expired Lib")
	book := &models.Book{LibraryID: lib.ID, Title: "Test", TitleSource: models.DataSourceFilepath, SortTitle: "Test", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/tmp"}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{LibraryID: lib.ID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: "/tmp/fake.epub", FilesizeBytes: 100}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	job := &models.Job{
		Type:   models.JobTypeBulkDownload,
		Status: models.JobStatusCompleted,
		Data:   fmt.Sprintf(`{"fingerprint_hash":"gone","file_ids":[%d],"file_count":1}`, file.ID),
	}
	_, err = db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)

	user := &models.User{Username: "expireduser", PasswordHash: "hash", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.UserLibraryAccess{UserID: user.ID, LibraryID: &lib.ID}).Exec(ctx)
	require.NoError(t, err)
	require.NoError(t, db.NewSelect().Model(user).Relation("Role").Relation("Role.Permissions").Relation("LibraryAccess").Where("u.id = ?", user.ID).Scan(ctx))

	h := &handler{jobService: NewService(db), db: db, downloadCache: downloadcache.NewCache(t.TempDir(), 1<<30)}
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(job.ID))
	auth.SetUser(c, user)

	err = h.download(c)

	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr, "want an errcodes error, got %T: %v", err, err)
	assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
	assert.Equal(t, "not_found", ecErr.Code)
	assert.Equal(t, "Download file not found.", ecErr.Message)
}
