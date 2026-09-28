package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sharelinks"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// shareLinksFixture serves the real route table so the books group
// middleware, the shares permission checks, the sharing settings, and the
// public share family are exercised together.
type shareLinksFixture struct {
	t        *testing.T
	db       *bun.DB
	handler  http.Handler
	authSvc  *auth.Service
	rootDir  string // parent of every file on disk; must never reach a recipient
	libA     *models.Library
	libB     *models.Library
	bookA    *models.Book // library A: main EPUB with a cover, plus a supplement
	epubA    *models.File
	suppA    *models.File
	bookB    *models.Book // library B: main EPUB
	epubB    *models.File
	admin    *models.User // all libraries
	editor   *models.User // Books Write, no shares
	viewer   *models.User // library A only, no shares
	sharer   *models.User // Books Read and Shares Read+Write, library A only
	auditor  *models.User // Books Read and Shares Read, library A only
	writer   *models.User // Books Read and Shares Write without Shares Read, library A only
	outsider *models.User // Books Read and Shares Read+Write, library B only
}

func newShareLinksFixture(t *testing.T) *shareLinksFixture {
	t.Helper()
	ctx := context.Background()

	db := newPermissionTestDB(t)
	cfg := newPermissionTestConfig(t)
	dlCache := downloadcache.NewCache(t.TempDir(), 1<<30)
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil), nil, nil, dlCache, nil, nil, nil)
	require.NoError(t, err)

	f := &shareLinksFixture{
		t:       t,
		db:      db,
		handler: srv.Handler,
		authSvc: auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration()),
		rootDir: t.TempDir(),
	}

	f.libA = f.insertLibrary(ctx, "Library A")
	f.libB = f.insertLibrary(ctx, "Library B")
	f.bookA, f.epubA = f.insertBook(ctx, f.libA, "Alpha")
	f.suppA = f.insertSupplement(ctx, f.bookA, "notes.pdf")
	f.insertSeries(ctx, f.bookA, "Saga")
	scanError := "open " + f.epubA.Filepath + ": zip: not a valid zip file"
	_, err = db.NewUpdate().Model(f.epubA).Set("scan_error = ?", scanError).Set("url = ?", "https://example.com/internal-catalog").WherePK().Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.FileIdentifier{FileID: f.epubA.ID, Type: "uuid", Value: "urn:uuid:internal-1234", Source: models.DataSourceFileMetadata}).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewUpdate().Model(f.bookA).Set("sort_title = ?", "Alpha, The").WherePK().Exec(ctx)
	require.NoError(t, err)
	f.bookB, f.epubB = f.insertBook(ctx, f.libB, "Bravo")

	f.admin = insertPermissionTestUser(ctx, t, db, "admin", models.RoleAdmin, nil)
	f.editor = insertPermissionTestUser(ctx, t, db, "editor", models.RoleEditor, nil)
	f.viewer = insertPermissionTestUser(ctx, t, db, "viewer", models.RoleViewer, &f.libA.ID)
	f.sharer = f.insertCustomUser(ctx, "sharer", &f.libA.ID, models.OperationRead, models.OperationWrite)
	f.auditor = f.insertCustomUser(ctx, "auditor", &f.libA.ID, models.OperationRead)
	f.writer = f.insertCustomUser(ctx, "writer", &f.libA.ID, models.OperationWrite)
	f.outsider = f.insertCustomUser(ctx, "outsider", &f.libB.ID, models.OperationRead, models.OperationWrite)
	return f
}

func (f *shareLinksFixture) insertLibrary(ctx context.Context, name string) *models.Library {
	f.t.Helper()
	lib := &models.Library{Name: name, CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err := f.db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(f.t, err)
	return lib
}

// insertBook creates a book directory holding a real EPUB and its cover.
func (f *shareLinksFixture) insertBook(ctx context.Context, lib *models.Library, title string) (*models.Book, *models.File) {
	f.t.Helper()
	dir := filepath.Join(f.rootDir, title)
	require.NoError(f.t, os.MkdirAll(dir, 0o755))
	epubPath := testgen.GenerateEPUB(f.t, dir, title+".epub", testgen.EPUBOptions{Title: title, Authors: []string{"Author " + title}})
	coverName := title + ".epub.cover.png"
	require.NoError(f.t, os.WriteFile(filepath.Join(dir, coverName), []byte("cover-"+title), 0o600))

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           title,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        dir,
	}
	_, err := f.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(f.t, err)
	file := &models.File{
		LibraryID:          lib.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           epubPath,
		FilesizeBytes:      100,
		CoverImageFilename: &coverName,
	}
	_, err = f.db.NewInsert().Model(file).Exec(ctx)
	require.NoError(f.t, err)
	return book, file
}

// insertSeries puts book in a series whose cover filename must not reach a
// recipient.
func (f *shareLinksFixture) insertSeries(ctx context.Context, book *models.Book, name string) {
	f.t.Helper()
	cover := name + ".series.cover.png"
	series := &models.Series{
		LibraryID:          book.LibraryID,
		Name:               name,
		NameSource:         models.DataSourceManual,
		SortName:           name,
		SortNameSource:     models.DataSourceManual,
		CoverImageFilename: &cover,
	}
	_, err := f.db.NewInsert().Model(series).Exec(ctx)
	require.NoError(f.t, err)
	_, err = f.db.NewInsert().Model(&models.BookSeries{BookID: book.ID, SeriesID: series.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(f.t, err)
}

// insertPluginFormatFile adds a main file whose type only a plugin parses, so
// no built-in generator exists for it.
func (f *shareLinksFixture) insertPluginFormatFile(ctx context.Context, book *models.Book, name string) *models.File {
	f.t.Helper()
	path := filepath.Join(book.Filepath, name)
	require.NoError(f.t, os.WriteFile(path, []byte("plugin format bytes"), 0o600))
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      "fb2",
		FileRole:      models.FileRoleMain,
		Filepath:      path,
		FilesizeBytes: 19,
	}
	_, err := f.db.NewInsert().Model(file).Exec(ctx)
	require.NoError(f.t, err)
	return file
}

func (f *shareLinksFixture) insertSupplement(ctx context.Context, book *models.Book, name string) *models.File {
	f.t.Helper()
	path := filepath.Join(book.Filepath, name)
	require.NoError(f.t, os.WriteFile(path, []byte("supplement bytes"), 0o600))
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypePDF,
		FileRole:      models.FileRoleSupplement,
		Filepath:      path,
		FilesizeBytes: 16,
	}
	_, err := f.db.NewInsert().Model(file).Exec(ctx)
	require.NoError(f.t, err)
	return file
}

// insertCustomUser creates a role with Books Read plus the given shares
// operations, and a user holding it with access to one library.
func (f *shareLinksFixture) insertCustomUser(ctx context.Context, name string, libraryID *int, shareOps ...string) *models.User {
	f.t.Helper()
	role := &models.Role{Name: name}
	_, err := f.db.NewInsert().Model(role).Exec(ctx)
	require.NoError(f.t, err)
	perms := []*models.Permission{{RoleID: role.ID, Resource: models.ResourceBooks, Operation: models.OperationRead}}
	for _, op := range shareOps {
		perms = append(perms, &models.Permission{RoleID: role.ID, Resource: models.ResourceShares, Operation: op})
	}
	for _, p := range perms {
		_, err = f.db.NewInsert().Model(p).Exec(ctx)
		require.NoError(f.t, err)
	}
	return insertPermissionTestUser(ctx, f.t, f.db, name, role.Name, libraryID)
}

func (f *shareLinksFixture) setSharing(enabled, requireExpiration bool) {
	f.t.Helper()
	rec := f.do(f.admin, http.MethodPut, "/api/settings/sharing", fmt.Sprintf(`{"enabled":%t,"require_expiration":%t}`, enabled, requireExpiration))
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
}

// do sends a request as user, or anonymously when user is nil.
func (f *shareLinksFixture) do(user *models.User, method, path, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if user != nil {
		token, err := f.authSvc.GenerateToken(user)
		require.NoError(f.t, err)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f *shareLinksFixture) create(user *models.User, book *models.Book, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(user, http.MethodPost, fmt.Sprintf("/api/books/%d/share-links", book.ID), body)
}

// mustCreate creates a link from the given payload and returns it.
func (f *shareLinksFixture) mustCreate(user *models.User, book *models.Book, body string) sharelinks.ShareLinkResponse {
	f.t.Helper()
	rec := f.create(user, book, body)
	require.Equal(f.t, http.StatusCreated, rec.Code, rec.Body.String())
	var link sharelinks.ShareLinkResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &link))
	return link
}

func (f *shareLinksFixture) list(user *models.User, book *models.Book) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(user, http.MethodGet, fmt.Sprintf("/api/books/%d/share-links", book.ID), "")
}

func (f *shareLinksFixture) expire(link sharelinks.ShareLinkResponse) {
	f.t.Helper()
	_, err := f.db.NewUpdate().Table("share_links").
		Set("expires_at = ?", time.Now().Add(-time.Minute)).
		Where("id = ?", link.ID).Exec(context.Background())
	require.NoError(f.t, err)
}

func futureJSON(d time.Duration) string {
	return time.Now().Add(d).UTC().Format(time.RFC3339)
}

func TestShareLinks_CreateRefusedWhileSharingDisabled(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)

	rec := f.create(f.admin, f.bookA, `{}`)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "Sharing is turned off")

	var count int
	require.NoError(t, f.db.NewRaw("SELECT COUNT(*) FROM share_links").Scan(context.Background(), &count))
	assert.Zero(t, count)
}

func TestShareLinks_CreatePermissions(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)

	tests := []struct {
		name   string
		user   *models.User
		book   *models.Book
		status int
	}{
		{"admin", f.admin, f.bookA, http.StatusCreated},
		{"custom role with shares write and no books write", f.sharer, f.bookA, http.StatusCreated},
		{"shares write without shares read", f.writer, f.bookA, http.StatusCreated},
		{"editor without shares", f.editor, f.bookA, http.StatusForbidden},
		{"viewer without shares", f.viewer, f.bookA, http.StatusForbidden},
		{"shares read only", f.auditor, f.bookA, http.StatusForbidden},
		{"shares write without library access", f.sharer, f.bookB, http.StatusForbidden},
		{"shares write in another library", f.outsider, f.bookA, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := f.create(tt.user, tt.book, `{}`)
			assert.Equal(t, tt.status, rec.Code, rec.Body.String())
		})
	}

	rec := f.create(f.admin, &models.Book{ID: 999999}, `{}`)
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

func TestShareLinks_CreateExpirationRules(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)

	rec := f.create(f.admin, f.bookA, fmt.Sprintf(`{"expires_at":%q}`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())

	never := f.mustCreate(f.admin, f.bookA, `{}`)
	assert.Nil(t, never.ExpiresAt)

	f.setSharing(true, true)
	rec = f.create(f.admin, f.bookA, `{}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "expiration")

	expiring := f.mustCreate(f.admin, f.bookA, fmt.Sprintf(`{"expires_at":%q}`, futureJSON(7*24*time.Hour)))
	require.NotNil(t, expiring.ExpiresAt)
	assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), *expiring.ExpiresAt, time.Minute)
}

func TestShareLinks_CreateResponse(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)

	link := f.mustCreate(f.sharer, f.bookA, `{"label":"  for Alice  "}`)
	raw, err := base64.RawURLEncoding.DecodeString(link.Token)
	require.NoError(t, err, "token must be unpadded base64url")
	assert.Len(t, raw, 32)
	assert.Equal(t, models.ShareLinkStateActive, link.State)
	assert.Equal(t, "sharer", link.CreatedByUsername)
	require.NotNil(t, link.Label)
	assert.Equal(t, "for Alice", *link.Label)
	assert.Equal(t, f.bookA.ID, link.BookID)

	blank := f.mustCreate(f.sharer, f.bookA, `{"label":"   "}`)
	assert.Nil(t, blank.Label, "a blank label is stored as no label")
	assert.NotEqual(t, link.Token, blank.Token)
}

func TestShareLinks_List(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)

	first := f.mustCreate(f.admin, f.bookA, `{"label":"first"}`)
	second := f.mustCreate(f.sharer, f.bookA, fmt.Sprintf(`{"label":"second","expires_at":%q}`, futureJSON(24*time.Hour)))
	f.mustCreate(f.admin, f.bookB, `{"label":"other book"}`)
	f.expire(second)

	rec := f.list(f.auditor, f.bookA)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var links []sharelinks.ShareLinkResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &links), "the list is a bare array")
	require.Len(t, links, 2)
	byID := map[int]sharelinks.ShareLinkResponse{}
	for _, l := range links {
		byID[l.ID] = l
	}
	assert.Equal(t, models.ShareLinkStateActive, byID[first.ID].State)
	assert.Equal(t, "admin", byID[first.ID].CreatedByUsername)
	assert.Equal(t, models.ShareLinkStateExpired, byID[second.ID].State)
	assert.Equal(t, "sharer", byID[second.ID].CreatedByUsername)

	// Shares Write also lists, so a sharer can copy the links they make.
	assert.Equal(t, http.StatusOK, f.list(f.writer, f.bookA).Code)

	// Reading needs a shares permission and library access, not Books Write.
	assert.Equal(t, http.StatusForbidden, f.list(f.editor, f.bookA).Code)
	assert.Equal(t, http.StatusForbidden, f.list(f.viewer, f.bookA).Code)
	assert.Equal(t, http.StatusForbidden, f.list(f.auditor, f.bookB).Code)

	// Listing stays available while sharing is off.
	f.setSharing(false, false)
	assert.Equal(t, http.StatusOK, f.list(f.auditor, f.bookA).Code)
}

func TestShareLinks_RecipientFetchesBookWithoutPaths(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	expiresAt := futureJSON(30 * 24 * time.Hour)
	link := f.mustCreate(f.sharer, f.bookA, fmt.Sprintf(`{"expires_at":%q}`, expiresAt))

	rec := f.do(nil, http.MethodGet, "/api/share/"+link.Token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	assert.NotContains(t, body, f.rootDir, "no filesystem path may reach a recipient")
	assert.NotContains(t, body, ".cover.png", "no cover filename may reach a recipient")
	assert.NotContains(t, body, "not a valid zip file", "scan errors can quote paths")
	assert.NotContains(t, body, "Alpha, The", "the sort title is the library's, not the reader's")
	assert.NotContains(t, body, "internal-catalog", "file URLs are not shown to recipients")
	assert.NotContains(t, body, "internal-1234", "file identifiers are not shown to recipients")
	assert.Contains(t, body, `"name":"Saga"`, "the series still shows")
	assert.NotContains(t, body, "Library A", "the library is not part of the payload")

	var shared sharelinks.SharedBookResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &shared))
	assert.Equal(t, "Alpha", shared.Title)
	assert.Equal(t, "sharer", shared.SharedBy)
	require.NotNil(t, shared.ExpiresAt)
	assert.Equal(t, expiresAt, shared.ExpiresAt.UTC().Format(time.RFC3339))
	assert.NotEmpty(t, shared.CoverCacheKey)
	assert.Equal(t, "book", shared.CoverAspectRatio)
	require.Len(t, shared.Files, 2)
	for _, file := range shared.Files {
		assert.Empty(t, file.Filepath)
		assert.Nil(t, file.CoverImageFilename)
		if file.FileRole == models.FileRoleSupplement {
			assert.Equal(t, "notes.pdf", file.DisplayName, "supplements show their filename without the path")
		} else {
			// A main file without a name must not fall back to its on-disk
			// filename for a recipient; the page shows the file type instead.
			assert.Empty(t, file.DisplayName)
		}
	}
	assert.Empty(t, shared.Filepath)
	assert.Nil(t, shared.Library)
}

func TestShareLinks_RecipientCovers(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	link := f.mustCreate(f.admin, f.bookA, `{}`)

	rec := f.do(nil, http.MethodGet, "/api/share/"+link.Token+"/cover?v=1", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "cover-Alpha", rec.Body.String())
	assert.Equal(t, "private, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))

	rec = f.do(nil, http.MethodGet, fmt.Sprintf("/api/share/%s/files/%d/cover", link.Token, f.epubA.ID), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "cover-Alpha", rec.Body.String())
	assert.Equal(t, "private, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))

	rec = f.do(nil, http.MethodGet, fmt.Sprintf("/api/share/%s/files/%d/cover", link.Token, f.epubB.ID), "")
	assert.Equal(t, http.StatusNotFound, rec.Code, "a file outside the link's book is not served")
}

func TestShareLinks_RecipientDownloads(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	link := f.mustCreate(f.admin, f.bookA, `{}`)
	downloadPath := func(file *models.File) string {
		return fmt.Sprintf("/api/share/%s/files/%d/download", link.Token, file.ID)
	}

	rec := f.do(nil, http.MethodHead, downloadPath(f.epubA), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
	assert.Empty(t, rec.Body.Bytes())

	rec = f.do(nil, http.MethodGet, downloadPath(f.epubA), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
	assert.Contains(t, rec.Header().Get("Content-Disposition"), ".epub")
	original, err := os.ReadFile(f.epubA.Filepath)
	require.NoError(t, err)
	assert.NotEqual(t, original, rec.Body.Bytes(), "recipients get the generated file, not the original")
	assert.Equal(t, "PK", string(rec.Body.Bytes()[:2]))

	rec = f.do(nil, http.MethodGet, downloadPath(f.suppA), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "supplement bytes", rec.Body.String(), "supplements download as-is")
	assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))

	pluginFile := f.insertPluginFormatFile(context.Background(), f.bookA, "Alpha.fb2")
	rec = f.do(nil, http.MethodGet, downloadPath(pluginFile), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "plugin format bytes", rec.Body.String(), "a format with no generator downloads as-is")

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		rec = f.do(nil, method, downloadPath(f.epubB), "")
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s of a file outside the link's book", method)
	}
}

func TestShareLinks_UnavailableLinksShareOneResponse(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	active := f.mustCreate(f.admin, f.bookA, `{}`)
	expired := f.mustCreate(f.admin, f.bookA, fmt.Sprintf(`{"expires_at":%q}`, futureJSON(time.Hour)))
	f.expire(expired)
	revoked := f.mustCreate(f.admin, f.bookA, `{}`)
	require.Equal(t, http.StatusOK, f.revoke(f.admin, f.bookA, revoked).Code)
	byDeactivated := f.mustCreate(f.sharer, f.bookA, `{}`)
	require.Equal(t, http.StatusNoContent, f.do(f.admin, http.MethodDelete, fmt.Sprintf("/api/users/%d", f.sharer.ID), "").Code)
	byOutsider := f.mustCreate(f.writer, f.bookA, `{}`)
	rec := f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/users/%d", f.writer.ID), fmt.Sprintf(`{"library_ids":[%d]}`, f.libB.ID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	paths := func(token string) []string {
		return []string{
			"/api/share/" + token,
			"/api/share/" + token + "/cover",
			fmt.Sprintf("/api/share/%s/files/%d/cover", token, f.epubA.ID),
			fmt.Sprintf("/api/share/%s/files/%d/download", token, f.epubA.ID),
		}
	}
	unknown := base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	var want string
	check := func(name, path string) {
		t.Helper()
		rec := f.do(nil, http.MethodGet, path, "")
		require.Equal(t, http.StatusNotFound, rec.Code, "%s: %s", name, path)
		if want == "" {
			want = rec.Body.String()
			assert.Contains(t, want, `"code":"not_found"`)
		}
		assert.JSONEq(t, want, rec.Body.String(), "%s: %s", name, path)
	}
	for _, path := range paths(unknown) {
		check("unknown", path)
	}
	for _, path := range paths("not-a-token") {
		check("malformed", path)
	}
	for _, path := range paths(expired.Token) {
		check("expired", path)
	}
	for _, path := range paths(revoked.Token) {
		check("revoked", path)
	}
	for _, path := range paths(byDeactivated.Token) {
		check("deactivated creator", path)
	}
	for _, path := range paths(byOutsider.Token) {
		check("creator without library access", path)
	}

	// Turning sharing off stops every link without deleting it.
	f.setSharing(false, false)
	for _, path := range paths(active.Token) {
		check("disabled", path)
	}
	// Turning it back on restores the link.
	f.setSharing(true, false)
	assert.Equal(t, http.StatusOK, f.do(nil, http.MethodGet, "/api/share/"+active.Token, "").Code)
}

func TestShareLinks_ManagementRejectedInDemoMode(t *testing.T) {
	t.Parallel()
	db := newPermissionTestDB(t)
	cfg := newPermissionTestConfig(t)
	cfg.DemoMode = true
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil), nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	authSvc := auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration())
	admin := insertPermissionTestUser(context.Background(), t, db, "admin", models.RoleAdmin, nil)
	token, err := authSvc.GenerateToken(admin)
	require.NoError(t, err)

	for _, request := range []struct{ method, path string }{
		{http.MethodPost, "/api/books/1/share-links"},
		{http.MethodPost, "/api/books/1/share-links/1/revoke"},
		{http.MethodDelete, "/api/books/1/share-links/1"},
	} {
		req := httptest.NewRequest(request.method, request.path, strings.NewReader(`{}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s", request.method, request.path)
		assert.Contains(t, rec.Body.String(), `"code":"demo_mode"`)
	}
}

func TestShareLinks_SharesWriteReadsSharingSettings(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)

	rec := f.do(f.writer, http.MethodGet, "/api/settings/sharing", "")
	assert.Equal(t, http.StatusOK, rec.Code, "a sharer without Shares Read needs the policy to render the form")
	rec = f.do(f.editor, http.MethodGet, "/api/settings/sharing", "")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func (f *shareLinksFixture) revoke(user *models.User, book *models.Book, link sharelinks.ShareLinkResponse) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(user, http.MethodPost, fmt.Sprintf("/api/books/%d/share-links/%d/revoke", book.ID, link.ID), "")
}

func (f *shareLinksFixture) deleteLink(user *models.User, book *models.Book, link sharelinks.ShareLinkResponse) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(user, http.MethodDelete, fmt.Sprintf("/api/books/%d/share-links/%d", book.ID, link.ID), "")
}

// listed returns the book's links as the admin sees them, keyed by id.
func (f *shareLinksFixture) listed(book *models.Book) map[int]sharelinks.ShareLinkResponse {
	f.t.Helper()
	rec := f.list(f.admin, book)
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var links []sharelinks.ShareLinkResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &links))
	byID := map[int]sharelinks.ShareLinkResponse{}
	for _, l := range links {
		byID[l.ID] = l
	}
	return byID
}

func TestShareLinks_RevokeStopsTheLinkAndKeepsItListed(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	link := f.mustCreate(f.sharer, f.bookA, `{"label":"for Alice"}`)
	other := f.mustCreate(f.sharer, f.bookA, `{"label":"for Bob"}`)
	require.Equal(t, http.StatusOK, f.do(nil, http.MethodGet, "/api/share/"+link.Token, "").Code)

	rec := f.revoke(f.sharer, f.bookA, link)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var revoked sharelinks.ShareLinkResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &revoked))
	assert.Equal(t, models.ShareLinkStateRevoked, revoked.State)
	require.NotNil(t, revoked.RevokedAt)

	assert.Equal(t, http.StatusNotFound, f.do(nil, http.MethodGet, "/api/share/"+link.Token, "").Code)
	assert.Equal(t, http.StatusOK, f.do(nil, http.MethodGet, "/api/share/"+other.Token, "").Code, "other links on the book keep working")

	listed := f.listed(f.bookA)
	require.Contains(t, listed, link.ID, "a revoked link stays listed")
	assert.Equal(t, models.ShareLinkStateRevoked, listed[link.ID].State)
	assert.Equal(t, models.ShareLinkStateActive, listed[other.ID].State)

	// Revoking again changes nothing; there is no way back to active.
	rec = f.revoke(f.admin, f.bookA, link)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var again sharelinks.ShareLinkResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &again))
	assert.Equal(t, models.ShareLinkStateRevoked, again.State)
	assert.True(t, revoked.RevokedAt.Equal(*again.RevokedAt), "the first revocation time is kept")
	assert.Equal(t, http.StatusNotFound, f.do(nil, http.MethodGet, "/api/share/"+link.Token, "").Code)
}

func TestShareLinks_DeleteRemovesLinksInEveryState(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	active := f.mustCreate(f.sharer, f.bookA, `{"label":"active"}`)
	expired := f.mustCreate(f.admin, f.bookA, fmt.Sprintf(`{"label":"expired","expires_at":%q}`, futureJSON(time.Hour)))
	f.expire(expired)
	revoked := f.mustCreate(f.sharer, f.bookA, `{"label":"revoked"}`)
	require.Equal(t, http.StatusOK, f.revoke(f.sharer, f.bookA, revoked).Code)
	kept := f.mustCreate(f.sharer, f.bookA, `{"label":"kept"}`)

	for _, link := range []sharelinks.ShareLinkResponse{active, expired, revoked} {
		rec := f.deleteLink(f.sharer, f.bookA, link)
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
		assert.Empty(t, rec.Body.String())
		assert.Equal(t, http.StatusNotFound, f.deleteLink(f.sharer, f.bookA, link).Code, "a deleted link is gone")
	}

	listed := f.listed(f.bookA)
	assert.Len(t, listed, 1)
	assert.Contains(t, listed, kept.ID)
	var count int
	require.NoError(t, f.db.NewRaw("SELECT COUNT(*) FROM share_links").Scan(context.Background(), &count))
	assert.Equal(t, 1, count)
	assert.Equal(t, http.StatusNotFound, f.do(nil, http.MethodGet, "/api/share/"+active.Token, "").Code)
}

func TestShareLinks_RevokeAndDeletePermissions(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	onA := f.mustCreate(f.admin, f.bookA, `{}`)
	onB := f.mustCreate(f.admin, f.bookB, `{}`)

	tests := []struct {
		name   string
		user   *models.User
		book   *models.Book
		link   sharelinks.ShareLinkResponse
		status int
	}{
		{"editor without shares", f.editor, f.bookA, onA, http.StatusForbidden},
		{"viewer without shares", f.viewer, f.bookA, onA, http.StatusForbidden},
		{"shares read only", f.auditor, f.bookA, onA, http.StatusForbidden},
		{"shares write without library access", f.sharer, f.bookB, onB, http.StatusForbidden},
		{"shares write in another library", f.outsider, f.bookA, onA, http.StatusForbidden},
		{"a link on another book", f.admin, f.bookA, onB, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.status, f.revoke(tt.user, tt.book, tt.link).Code, "revoke")
			assert.Equal(t, tt.status, f.deleteLink(tt.user, tt.book, tt.link).Code, "delete")
		})
	}
	listed := f.listed(f.bookA)
	require.Contains(t, listed, onA.ID)
	assert.Equal(t, models.ShareLinkStateActive, listed[onA.ID].State, "a refused revoke changes nothing")
	assert.Contains(t, f.listed(f.bookB), onB.ID, "a refused delete changes nothing")

	// Revoking and deleting are writes, refused while sharing is off.
	f.setSharing(false, false)
	rec := f.revoke(f.admin, f.bookA, onA)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "Sharing is turned off")
	assert.Equal(t, http.StatusForbidden, f.deleteLink(f.admin, f.bookA, onA).Code)
	f.setSharing(true, false)

	// Shares Write without Shares Read can revoke and delete.
	require.Equal(t, http.StatusOK, f.revoke(f.writer, f.bookA, onA).Code)
	require.Equal(t, http.StatusNoContent, f.deleteLink(f.writer, f.bookA, onA).Code)
}

func TestShareLinks_Counters(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	link := f.mustCreate(f.sharer, f.bookA, `{}`)
	assert.Zero(t, link.OpenCount)
	assert.Zero(t, link.DownloadCount)
	assert.Nil(t, link.LastAccessedAt)
	download := fmt.Sprintf("/api/share/%s/files/%d/download", link.Token, f.epubA.ID)
	counts := func() sharelinks.ShareLinkResponse {
		t.Helper()
		return f.listed(f.bookA)[link.ID]
	}

	// Covers, HEAD, and a refused download change nothing.
	require.Equal(t, http.StatusOK, f.do(nil, http.MethodGet, "/api/share/"+link.Token+"/cover", "").Code)
	require.Equal(t, http.StatusOK, f.do(nil, http.MethodGet, fmt.Sprintf("/api/share/%s/files/%d/cover", link.Token, f.epubA.ID), "").Code)
	require.Equal(t, http.StatusOK, f.do(nil, http.MethodHead, download, "").Code)
	require.Equal(t, http.StatusNotFound, f.do(nil, http.MethodGet, fmt.Sprintf("/api/share/%s/files/%d/download", link.Token, f.epubB.ID), "").Code)
	got := counts()
	assert.Zero(t, got.OpenCount)
	assert.Zero(t, got.DownloadCount)
	assert.Nil(t, got.LastAccessedAt)

	// Opening the page counts an open and sets last used.
	before := time.Now().Add(-time.Second)
	require.Equal(t, http.StatusOK, f.do(nil, http.MethodGet, "/api/share/"+link.Token, "").Code)
	require.Equal(t, http.StatusOK, f.do(nil, http.MethodGet, "/api/share/"+link.Token, "").Code)
	got = counts()
	assert.Equal(t, 2, got.OpenCount)
	assert.Zero(t, got.DownloadCount)
	require.NotNil(t, got.LastAccessedAt)
	assert.True(t, got.LastAccessedAt.After(before))
	opened := *got.LastAccessedAt

	// A download GET counts a download, including a supplement.
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, http.StatusOK, f.do(nil, http.MethodGet, download, "").Code)
	require.Equal(t, http.StatusOK, f.do(nil, http.MethodGet, fmt.Sprintf("/api/share/%s/files/%d/download", link.Token, f.suppA.ID), "").Code)
	got = counts()
	assert.Equal(t, 2, got.OpenCount)
	assert.Equal(t, 2, got.DownloadCount)
	require.NotNil(t, got.LastAccessedAt)
	assert.True(t, got.LastAccessedAt.After(opened), "a download moves last used")

	// A revoked link keeps its counts, and requests to it count nothing.
	require.Equal(t, http.StatusOK, f.revoke(f.sharer, f.bookA, link).Code)
	require.Equal(t, http.StatusNotFound, f.do(nil, http.MethodGet, "/api/share/"+link.Token, "").Code)
	require.Equal(t, http.StatusNotFound, f.do(nil, http.MethodGet, download, "").Code)
	revoked := counts()
	assert.Equal(t, models.ShareLinkStateRevoked, revoked.State)
	assert.Equal(t, 2, revoked.OpenCount)
	assert.Equal(t, 2, revoked.DownloadCount)
	assert.True(t, got.LastAccessedAt.Equal(*revoked.LastAccessedAt))
}

// linkRowExists reports whether the share_links row is still there. Deleting
// a creator or a book removes the row, which no endpoint can show once the
// book is gone.
func (f *shareLinksFixture) linkRowExists(link sharelinks.ShareLinkResponse) bool {
	f.t.Helper()
	var count int
	require.NoError(f.t, f.db.NewRaw("SELECT COUNT(*) FROM share_links WHERE id = ?", link.ID).Scan(context.Background(), &count))
	return count == 1
}

// assertUnavailable checks every public path for token returns the same 404
// body as a token that never existed.
func (f *shareLinksFixture) assertUnavailable(token string, fileID int) {
	f.t.Helper()
	unknown := f.do(nil, http.MethodGet, "/api/share/"+base64.RawURLEncoding.EncodeToString(make([]byte, 32)), "")
	require.Equal(f.t, http.StatusNotFound, unknown.Code)
	for _, path := range []string{
		"/api/share/" + token,
		"/api/share/" + token + "/cover",
		fmt.Sprintf("/api/share/%s/files/%d/cover", token, fileID),
		fmt.Sprintf("/api/share/%s/files/%d/download", token, fileID),
	} {
		rec := f.do(nil, http.MethodGet, path, "")
		require.Equal(f.t, http.StatusNotFound, rec.Code, path)
		assert.JSONEq(f.t, unknown.Body.String(), rec.Body.String(), path)
	}
}

func TestShareLinks_CreatorAndBookLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("deactivating the creator", func(t *testing.T) {
		t.Parallel()
		f := newShareLinksFixture(t)
		f.setSharing(true, false)
		link := f.mustCreate(f.sharer, f.bookA, `{}`)
		require.Equal(t, http.StatusNoContent, f.do(f.admin, http.MethodDelete, fmt.Sprintf("/api/users/%d", f.sharer.ID), "").Code)

		f.assertUnavailable(link.Token, f.epubA.ID)
		assert.True(t, f.linkRowExists(link), "the link stays listed")
		assert.Contains(t, f.listed(f.bookA), link.ID)
	})

	t.Run("removing the creator's access to the library", func(t *testing.T) {
		t.Parallel()
		f := newShareLinksFixture(t)
		f.setSharing(true, false)
		link := f.mustCreate(f.sharer, f.bookA, `{}`)
		rec := f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/users/%d", f.sharer.ID), fmt.Sprintf(`{"library_ids":[%d]}`, f.libB.ID))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		f.assertUnavailable(link.Token, f.epubA.ID)
		assert.True(t, f.linkRowExists(link))

		// Giving access back restores the link, as with the sharing switch.
		rec = f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/users/%d", f.sharer.ID), `{"all_library_access":true}`)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, http.StatusOK, f.do(nil, http.MethodGet, "/api/share/"+link.Token, "").Code)
	})

	t.Run("deleting the creator", func(t *testing.T) {
		t.Parallel()
		f := newShareLinksFixture(t)
		f.setSharing(true, false)
		link := f.mustCreate(f.sharer, f.bookA, `{}`)
		_, err := f.db.NewDelete().Model((*models.User)(nil)).Where("id = ?", f.sharer.ID).Exec(context.Background())
		require.NoError(t, err)

		f.assertUnavailable(link.Token, f.epubA.ID)
		assert.False(t, f.linkRowExists(link))
	})

	t.Run("deleting the book", func(t *testing.T) {
		t.Parallel()
		f := newShareLinksFixture(t)
		f.setSharing(true, false)
		link := f.mustCreate(f.sharer, f.bookA, `{}`)
		rec := f.do(f.admin, http.MethodDelete, fmt.Sprintf("/api/books/%d", f.bookA.ID), "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		f.assertUnavailable(link.Token, f.epubA.ID)
		assert.False(t, f.linkRowExists(link))
	})
}

func TestShareLinks_PartialDownloadsCountOnce(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	f.setSharing(true, false)
	link := f.mustCreate(f.admin, f.bookA, `{}`)
	download := func(rangeHeader string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/share/%s/files/%d/download", link.Token, f.suppA.ID), nil)
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// A download manager resuming or splitting a file sends ranges past the
	// start; only the request that starts at byte 0 is a new download.
	require.Equal(t, http.StatusPartialContent, download("bytes=0-3"))
	require.Equal(t, http.StatusPartialContent, download("bytes=4-"))
	require.Equal(t, http.StatusPartialContent, download("bytes=8-11"))
	assert.Equal(t, 1, f.listed(f.bookA)[link.ID].DownloadCount)
}
