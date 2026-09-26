package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// jobsPermissionFixture serves the real route table so the jobs group
// middleware and the handler checks are exercised together.
type jobsPermissionFixture struct {
	t       *testing.T
	db      *bun.DB
	handler http.Handler
	authSvc *auth.Service
	dlCache *downloadcache.Cache
	libA    *models.Library
	libB    *models.Library
	fileA   *models.File // in library A
	fileB   *models.File // in library B
	admin   *models.User // all libraries
	editor  *models.User // all libraries
	viewer  *models.User // library A only
	writer  *models.User // custom role with Books Read and Jobs Write only, all libraries
}

func newJobsPermissionFixture(t *testing.T) *jobsPermissionFixture {
	t.Helper()
	ctx := context.Background()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	// Every pooled connection to a bare :memory: DSN is its own database, so
	// pin the pool before migrating.
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = migrations.BringUpToDate(ctx, db)
	require.NoError(t, err)

	cfg := config.NewForTest()
	// Test-mode routes mutate shared plugin globals; this test runs in parallel.
	cfg.Environment = ""
	cfg.CacheDir = t.TempDir()
	dlCache := downloadcache.NewCache(t.TempDir(), 1<<30)
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil), nil, nil, dlCache, nil, nil, nil)
	require.NoError(t, err)

	f := &jobsPermissionFixture{
		t:       t,
		db:      db,
		handler: srv.Handler,
		authSvc: auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration()),
		dlCache: dlCache,
	}

	f.libA = f.insertLibrary(ctx, "Library A")
	f.libB = f.insertLibrary(ctx, "Library B")
	f.fileA = f.insertFile(ctx, f.libA, "a")
	f.fileB = f.insertFile(ctx, f.libB, "b")
	f.admin = f.insertUser(ctx, "admin", models.RoleAdmin, nil)
	f.editor = f.insertUser(ctx, "editor", models.RoleEditor, nil)
	f.viewer = f.insertUser(ctx, "viewer", models.RoleViewer, &f.libA.ID)

	writerRole := &models.Role{Name: "jobs-writer"}
	_, err = db.NewInsert().Model(writerRole).Exec(ctx)
	require.NoError(t, err)
	for _, p := range []*models.Permission{
		{RoleID: writerRole.ID, Resource: models.ResourceBooks, Operation: models.OperationRead},
		{RoleID: writerRole.ID, Resource: models.ResourceJobs, Operation: models.OperationWrite},
	} {
		_, err = db.NewInsert().Model(p).Exec(ctx)
		require.NoError(t, err)
	}
	f.writer = f.insertUser(ctx, "writer", writerRole.Name, nil)
	return f
}

func (f *jobsPermissionFixture) insertLibrary(ctx context.Context, name string) *models.Library {
	f.t.Helper()
	lib := &models.Library{Name: name, CoverAspectRatio: "portrait", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err := f.db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(f.t, err)
	return lib
}

func (f *jobsPermissionFixture) insertFile(ctx context.Context, lib *models.Library, name string) *models.File {
	f.t.Helper()
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           name,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       name,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/tmp/" + name,
	}
	_, err := f.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(f.t, err)
	file := &models.File{
		LibraryID:     lib.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/" + name + "/" + name + ".epub",
		FilesizeBytes: 100,
	}
	_, err = f.db.NewInsert().Model(file).Exec(ctx)
	require.NoError(f.t, err)
	return file
}

// insertUser creates a user with the named built-in role. A nil libraryID
// grants access to all libraries.
func (f *jobsPermissionFixture) insertUser(ctx context.Context, username, roleName string, libraryID *int) *models.User {
	f.t.Helper()
	var roleID int
	require.NoError(f.t, f.db.NewRaw("SELECT id FROM roles WHERE name = ?", roleName).Scan(ctx, &roleID))
	user := &models.User{Username: username, PasswordHash: "unused", RoleID: roleID, IsActive: true}
	_, err := f.db.NewInsert().Model(user).Exec(ctx)
	require.NoError(f.t, err)
	_, err = f.db.NewInsert().Model(&models.UserLibraryAccess{UserID: user.ID, LibraryID: libraryID}).Exec(ctx)
	require.NoError(f.t, err)
	return user
}

func (f *jobsPermissionFixture) do(user *models.User, method, path, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	token, err := f.authSvc.GenerateToken(user)
	require.NoError(f.t, err)
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

// createBulkDownload posts a bulk_download job and returns the response.
func (f *jobsPermissionFixture) createBulkDownload(user *models.User, fileIDs ...int) *httptest.ResponseRecorder {
	f.t.Helper()
	ids, err := json.Marshal(fileIDs)
	require.NoError(f.t, err)
	return f.do(user, http.MethodPost, "/api/jobs", fmt.Sprintf(`{"type":"bulk_download","data":{"file_ids":%s,"estimated_size_bytes":100}}`, ids))
}

func (f *jobsPermissionFixture) jobID(rec *httptest.ResponseRecorder) int {
	f.t.Helper()
	var job models.Job
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &job))
	require.NotZero(f.t, job.ID)
	return job.ID
}

// completeBulkDownload marks a job completed the way the worker does and
// writes the archive into the download cache.
func (f *jobsPermissionFixture) completeBulkDownload(jobID int, fileIDs ...int) {
	f.t.Helper()
	hash := fmt.Sprintf("fingerprint-%d", jobID)
	data, err := json.Marshal(models.JobBulkDownloadData{FileIDs: fileIDs, FileCount: len(fileIDs), FingerprintHash: hash})
	require.NoError(f.t, err)
	_, err = f.db.NewUpdate().Table("jobs").
		Set("status = ?", models.JobStatusCompleted).
		Set("data = ?", string(data)).
		Where("id = ?", jobID).
		Exec(context.Background())
	require.NoError(f.t, err)
	zipPath := f.dlCache.BulkZipPath(hash)
	require.NoError(f.t, os.MkdirAll(filepath.Dir(zipPath), 0o755))
	require.NoError(f.t, os.WriteFile(zipPath, []byte("PK\x03\x04fake zip"), 0o644))
}

func TestJobsPermissions_BulkDownloadWithBooksRead(t *testing.T) {
	t.Parallel()
	f := newJobsPermissionFixture(t)

	for _, user := range []*models.User{f.editor, f.viewer} {
		rec := f.createBulkDownload(user, f.fileA.ID)
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", user.Username, rec.Body.String())
		id := f.jobID(rec)

		var createdBy *int
		require.NoError(t, f.db.NewRaw("SELECT created_by_user_id FROM jobs WHERE id = ?", id).Scan(context.Background(), &createdBy))
		require.NotNil(t, createdBy, "%s: the job must record its creator", user.Username)
		assert.Equal(t, user.ID, *createdBy, user.Username)

		rec = f.do(user, http.MethodGet, fmt.Sprintf("/api/jobs/%d", id), "")
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", user.Username, rec.Body.String())

		f.completeBulkDownload(id, f.fileA.ID)
		rec = f.do(user, http.MethodGet, fmt.Sprintf("/api/jobs/%d/download", id), "")
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", user.Username, rec.Body.String())
		assert.Equal(t, "PK\x03\x04fake zip", rec.Body.String(), user.Username)
	}
}

func TestJobsPermissions_BulkDownloadRejectsFilesOutsideLibraryAccess(t *testing.T) {
	t.Parallel()
	f := newJobsPermissionFixture(t)

	rec := f.createBulkDownload(f.viewer, f.fileA.ID, f.fileB.ID)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

	// Missing files are skipped, so the inaccessible file lands in a later
	// batch of the library lookup.
	ids := make([]int, 0, 1002)
	for i := range 1001 {
		ids = append(ids, 100000+i)
	}
	ids = append(ids, f.fileB.ID)
	rec = f.createBulkDownload(f.viewer, ids...)
	assert.Equal(t, http.StatusForbidden, rec.Code, "inaccessible file after the first batch: %s", rec.Body.String())

	count, err := f.db.NewSelect().Table("jobs").Where("type = ?", models.JobTypeBulkDownload).Count(context.Background())
	require.NoError(t, err)
	assert.Zero(t, count, "a rejected request must not enqueue a job")
}

func TestJobsPermissions_OtherJobTypesStillNeedJobsWrite(t *testing.T) {
	t.Parallel()
	f := newJobsPermissionFixture(t)

	// The writer role has Jobs Write without Jobs Read. Other job types keep
	// requiring both, as before bulk download was opened up.
	for _, user := range []*models.User{f.editor, f.viewer, f.writer} {
		for _, body := range []string{
			`{"type":"scan","data":{}}`,
			`{"type":"export","data":{}}`,
			`{"type":"recompute_review","data":{"clear_overrides":false}}`,
		} {
			rec := f.do(user, http.MethodPost, "/api/jobs", body)
			assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s", user.Username, body)
		}
	}

	rec := f.do(f.admin, http.MethodPost, "/api/jobs", `{"type":"scan","data":{}}`)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestJobsPermissions_OnlyCreatorOrJobsReadCanReadJob(t *testing.T) {
	t.Parallel()
	f := newJobsPermissionFixture(t)

	rec := f.createBulkDownload(f.editor, f.fileA.ID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	editorJob := f.jobID(rec)
	f.completeBulkDownload(editorJob, f.fileA.ID)

	rec = f.do(f.admin, http.MethodPost, "/api/jobs", `{"type":"scan","data":{}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	scanJob := f.jobID(rec)

	// The viewer has Books Read and access to library A but did not create
	// these jobs and lacks Jobs Read. A job it cannot read looks the same as a
	// missing one, so job IDs cannot be probed.
	for _, path := range []string{
		fmt.Sprintf("/api/jobs/%d", editorJob),
		fmt.Sprintf("/api/jobs/%d/download", editorJob),
		fmt.Sprintf("/api/jobs/%d", scanJob),
		fmt.Sprintf("/api/jobs/%d/download", scanJob),
		"/api/jobs/999999",
		"/api/jobs/999999/download",
	} {
		rec = f.do(f.viewer, http.MethodGet, path, "")
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
	}
	// Listing and logs are gated by Jobs Read on the route.
	for _, path := range []string{
		fmt.Sprintf("/api/jobs/%d/logs", editorJob),
		fmt.Sprintf("/api/jobs/%d/logs", scanJob),
		"/api/jobs",
	} {
		rec = f.do(f.viewer, http.MethodGet, path, "")
		assert.Equal(t, http.StatusForbidden, rec.Code, path)
	}

	// Only a bulk download is readable by its creator. Record the writer as
	// the creator of a scan directly, since the API no longer lets it start one.
	_, err := f.db.NewUpdate().Table("jobs").Set("created_by_user_id = ?", f.writer.ID).Where("id = ?", scanJob).Exec(context.Background())
	require.NoError(t, err)
	rec = f.do(f.writer, http.MethodGet, fmt.Sprintf("/api/jobs/%d", scanJob), "")
	assert.Equal(t, http.StatusNotFound, rec.Code, "creator of a non-bulk job without Jobs Read")

	// The creator still cannot list jobs or read logs without Jobs Read.
	for _, path := range []string{"/api/jobs", fmt.Sprintf("/api/jobs/%d/logs", editorJob)} {
		rec = f.do(f.editor, http.MethodGet, path, "")
		assert.Equal(t, http.StatusForbidden, rec.Code, path)
	}

	// Jobs Read keeps access to every job.
	for _, path := range []string{
		fmt.Sprintf("/api/jobs/%d", editorJob),
		fmt.Sprintf("/api/jobs/%d/download", editorJob),
		fmt.Sprintf("/api/jobs/%d/logs", editorJob),
		fmt.Sprintf("/api/jobs/%d", scanJob),
		"/api/jobs",
	} {
		rec = f.do(f.admin, http.MethodGet, path, "")
		assert.Equal(t, http.StatusOK, rec.Code, path)
	}
}

func TestJobsPermissions_BulkDownloadRechecksLibraryAccessOnDownload(t *testing.T) {
	t.Parallel()
	f := newJobsPermissionFixture(t)
	ctx := context.Background()

	rec := f.createBulkDownload(f.viewer, f.fileA.ID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	id := f.jobID(rec)
	f.completeBulkDownload(id, f.fileA.ID)

	// Move the viewer from library A to library B after the job completes.
	_, err := f.db.NewUpdate().Model((*models.UserLibraryAccess)(nil)).
		Set("library_id = ?", f.libB.ID).
		Where("user_id = ?", f.viewer.ID).
		Exec(ctx)
	require.NoError(t, err)

	rec = f.do(f.viewer, http.MethodGet, fmt.Sprintf("/api/jobs/%d/download", id), "")
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

func TestJobsPermissions_BulkDownloadStoresOnlyValidatedInput(t *testing.T) {
	t.Parallel()
	f := newJobsPermissionFixture(t)
	ctx := context.Background()

	body := fmt.Sprintf(`{"type":"bulk_download","library_id":%d,"data":{"file_ids":[%d],"estimated_size_bytes":100,"fingerprint_hash":"forged","extra":"junk"}}`, f.libB.ID, f.fileA.ID)
	rec := f.do(f.viewer, http.MethodPost, "/api/jobs", body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	id := f.jobID(rec)

	var stored struct {
		Data      string `bun:"data"`
		LibraryID *int   `bun:"library_id"`
	}
	require.NoError(t, f.db.NewRaw("SELECT data, library_id FROM jobs WHERE id = ?", id).Scan(ctx, &stored))
	assert.Nil(t, stored.LibraryID, "a bulk download must not attach itself to a library")
	assert.JSONEq(t, fmt.Sprintf(`{"file_ids":[%d],"estimated_size_bytes":100}`, f.fileA.ID), stored.Data)

	// A library_id that does not exist is ignored rather than failing the
	// jobs.library_id foreign key.
	rec = f.do(f.viewer, http.MethodPost, "/api/jobs", fmt.Sprintf(`{"type":"bulk_download","library_id":999999,"data":{"file_ids":[%d]}}`, f.fileA.ID))
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
