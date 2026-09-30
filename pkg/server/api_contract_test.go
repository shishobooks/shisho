package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorResponse is the body the errcodes handler renders for a failed request.
type errorResponse struct {
	Error struct {
		Code       string `json:"code"`
		Message    string `json:"message"`
		StatusCode int    `json:"status_code"`
	} `json:"error"`
}

// requestError sends an authenticated admin request and returns the status
// code and the wire error code and message of the response. It takes the
// calling test's t, so a subtest's failure stays in that subtest.
func (f *resourceDeleteFixture) requestError(t *testing.T, method, path, body string) (int, string, string) {
	t.Helper()
	token, err := f.authSvc.GenerateToken(f.admin)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	var resp errorResponse
	if rec.Code >= http.StatusBadRequest {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), "response body: %s", rec.Body.String())
	}
	return rec.Code, resp.Error.Code, resp.Error.Message
}

// Every rejected request follows one rule through the real routes: payload
// validation is 422, a missing related entity is 404, and the binder's own
// error (400 for a malformed body, 422 for an unknown parameter) reaches the
// client unchanged.
func TestAPIContract_StatusCodes(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	parent := &models.Publisher{LibraryID: f.lib.ID, Name: "Parent House"}
	f.insert(parent)
	child := &models.Publisher{LibraryID: f.lib.ID, Name: "Child Imprint", ParentID: &parent.ID}
	f.insert(child)
	f.insert(&models.Plugin{Scope: "contract", ID: "installed", Name: "Installed", Version: "1.0.0", Status: models.PluginStatusActive, InstalledAt: time.Now()})
	// Files on disk for the download and page rejections: a main file of a
	// type with no generator, and a CBZ with no stored page count.
	fileBook := &models.Book{
		LibraryID: f.lib.ID, Title: "Served Files", TitleSource: models.DataSourceManual,
		SortTitle: "Served Files", SortTitleSource: models.DataSourceFilepath,
		AuthorSource: models.DataSourceFilepath, Filepath: t.TempDir(),
	}
	f.insert(fileBook)
	pluginPath := filepath.Join(fileBook.Filepath, "book.fb2")
	require.NoError(t, os.WriteFile(pluginPath, []byte("fb2"), 0o644))
	pluginFile := &models.File{LibraryID: f.lib.ID, BookID: fileBook.ID, FileType: "fb2", FileRole: models.FileRoleMain, Filepath: pluginPath, FilesizeBytes: 3}
	f.insert(pluginFile)
	cbzPath := testgen.GenerateCBZ(t, fileBook.Filepath, "comic.cbz", testgen.CBZOptions{Title: "Comic"})
	cbzFile := &models.File{LibraryID: f.lib.ID, BookID: fileBook.ID, FileType: models.FileTypeCBZ, FileRole: models.FileRoleMain, Filepath: cbzPath, FilesizeBytes: 1}
	f.insert(cbzFile)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
		// message, when set, is the expected error message.
		message string
	}{
		{
			name:   "book update with an invalid series range",
			method: http.MethodPost, path: fmt.Sprintf("/api/books/%d", seeded.bookID),
			body:   `{"series":[{"name":"Saga","number":3,"number_end":2}]}`,
			status: http.StatusUnprocessableEntity, code: "validation_error",
		},
		{
			name:   "file update with an invalid language tag",
			method: http.MethodPost, path: fmt.Sprintf("/api/books/files/%d", seeded.fileID),
			body:   `{"language":"!!not a language!!"}`,
			status: http.StatusUnprocessableEntity, code: "validation_error",
		},
		{
			name:   "review criteria with a field the section does not allow",
			method: http.MethodPut, path: "/api/settings/review-criteria",
			body:   `{"book_fields":["narrators"],"audio_fields":[]}`,
			status: http.StatusUnprocessableEntity, code: "validation_error",
		},
		{
			name:   "bulk download with no file IDs",
			method: http.MethodPost, path: "/api/jobs",
			body:   `{"type":"bulk_download","data":{"file_ids":[]}}`,
			status: http.StatusUnprocessableEntity, code: "validation_error",
		},
		{
			name:   "bulk download with file IDs of the wrong type",
			method: http.MethodPost, path: "/api/jobs",
			body:   `{"type":"bulk_download","data":{"file_ids":"one"}}`,
			status: http.StatusUnprocessableEntity, code: "validation_error",
		},
		{
			name:   "publisher set-child with a child that does not exist",
			method: http.MethodPost, path: fmt.Sprintf("/api/publishers/%d/set-child", parent.ID),
			body:   `{"child_id":999999}`,
			status: http.StatusNotFound, code: "not_found", message: "Child publisher not found.",
		},
		{
			name:   "publisher update with a parent that does not exist",
			method: http.MethodPatch, path: fmt.Sprintf("/api/publishers/%d", child.ID),
			body:   `{"parent_id":999999}`,
			status: http.StatusNotFound, code: "not_found", message: "Parent publisher not found.",
		},
		{
			name:   "publisher set-child that would form a cycle",
			method: http.MethodPost, path: fmt.Sprintf("/api/publishers/%d/set-child", child.ID),
			body:   fmt.Sprintf(`{"child_id":%d}`, parent.ID),
			status: http.StatusUnprocessableEntity, code: "validation_error",
		},
		{
			name:   "move files to a target book that does not exist",
			method: http.MethodPost, path: fmt.Sprintf("/api/books/%d/move-files", seeded.bookID),
			body:   fmt.Sprintf(`{"file_ids":[%d],"target_book_id":999999}`, seeded.fileID),
			status: http.StatusNotFound, code: "not_found",
		},
		{
			name:   "generated download of a type with no generator",
			method: http.MethodGet, path: fmt.Sprintf("/api/books/files/%d/download", pluginFile.ID),
			status: http.StatusUnprocessableEntity, code: "invalid_state",
			message: "Generated downloads are not supported for fb2 files",
		},
		{
			name:   "page past the end of a file with no stored page count",
			method: http.MethodGet, path: fmt.Sprintf("/api/books/files/%d/page/99", cbzFile.ID),
			status: http.StatusNotFound, code: "not_found", message: "Page not found.",
		},
		{
			name:   "bulk book delete with a malformed body",
			method: http.MethodPost, path: "/api/books/delete",
			body:   `{`,
			status: http.StatusBadRequest, code: "malformed_payload",
		},
		{
			name:   "bulk book delete with an unknown parameter",
			method: http.MethodPost, path: "/api/books/delete",
			body:   `{"bogus":1}`,
			status: http.StatusUnprocessableEntity, code: "unknown_parameter",
		},
		{
			name:   "file cover page with a malformed body",
			method: http.MethodPut, path: fmt.Sprintf("/api/books/files/%d/cover-page", seeded.fileID),
			body:   `{`,
			status: http.StatusBadRequest, code: "malformed_payload",
		},
		{
			name:   "plugin search with a malformed body",
			method: http.MethodPost, path: "/api/plugins/search",
			body:   `{`,
			status: http.StatusBadRequest, code: "malformed_payload",
		},
		{
			name:   "plugin apply with a malformed body",
			method: http.MethodPost, path: "/api/plugins/apply",
			body:   `{`,
			status: http.StatusBadRequest, code: "malformed_payload",
		},
		{
			name:   "plugin install of an installed plugin",
			method: http.MethodPost, path: "/api/plugins/installed",
			body:   `{"scope":"contract","id":"installed"}`,
			status: http.StatusUnprocessableEntity, code: "invalid_state", message: "Plugin is already installed.",
		},
		{
			name:   "plugin install with an unsafe id",
			method: http.MethodPost, path: "/api/plugins/installed",
			body:   `{"scope":"contract","id":"a/b"}`,
			status: http.StatusUnprocessableEntity, code: "validation_error", message: "Invalid scope or plugin ID",
		},
		{
			name:   "plugin uninstall with an unsafe scope",
			method: http.MethodDelete, path: "/api/plugins/installed/.hidden/installed",
			status: http.StatusUnprocessableEntity, code: "validation_error", message: "Invalid scope or plugin ID",
		},
		{
			name:   "plugin reload with an unsafe scope",
			method: http.MethodPost, path: "/api/plugins/installed/.hidden/installed/reload",
			status: http.StatusUnprocessableEntity, code: "validation_error", message: "Invalid scope or plugin ID",
		},
		{
			name:   "users list with a limit above the bound",
			method: http.MethodGet, path: "/api/users?limit=1000",
			status: http.StatusUnprocessableEntity, code: "validation_error",
		},
		{
			name:   "roles list with a limit above the bound",
			method: http.MethodGet, path: "/api/roles?limit=1000",
			status: http.StatusUnprocessableEntity, code: "validation_error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, message := f.requestError(t, tt.method, tt.path, tt.body)
			assert.Equal(t, tt.status, status)
			assert.Equal(t, tt.code, code)
			if tt.message != "" {
				assert.Equal(t, tt.message, message)
			}
		})
	}
}

// A plugin reload whose files no longer load is a 422 plugin_load_failure
// through the real route. The shared fixture mounts the plugin routes with
// no manager, so this builds its own server with one.
func TestAPIContract_PluginReloadLoadFailure(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	cfg := newPermissionTestConfig(t)
	cfg.PluginDir = t.TempDir()
	pluginService := plugins.NewService(db)
	pm := plugins.NewManager(pluginService, cfg.PluginDir, t.TempDir())
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil, nil, nil, nil), pluginService, pm, nil, downloadcache.NewCache(t.TempDir(), 1<<30), nil, nil, nil)
	require.NoError(t, err)
	f := &resourceDeleteFixture{t: t, ctx: context.Background(), db: db, handler: srv.Handler, authSvc: auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration())}
	f.admin = insertPermissionTestUser(f.ctx, t, db, "admin", models.RoleAdmin, nil)

	dir := filepath.Join(cfg.PluginDir, "contract", "rl")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"manifestVersion":1,"id":"rl","name":"Reload","version":"1.0.0","capabilities":{"fileParser":{"types":["rlx"]}}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.js"), []byte(`var plugin={fileParser:{parse:function(){return{}}}};`), 0o644))
	f.insert(&models.Plugin{Scope: "contract", ID: "rl", Name: "Reload", Version: "1.0.0", Status: models.PluginStatusActive, InstalledAt: time.Now()})
	require.NoError(t, pm.LoadPlugin(f.ctx, "contract", "rl"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.js"), []byte(`var plugin = (;`), 0o644))

	status, code, _ := f.requestError(t, http.MethodPost, "/api/plugins/installed/contract/rl/reload", "")
	assert.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Equal(t, "plugin_load_failure", code)
}

// The unpaginated chapters and caches lists are bare arrays, and a library
// is the bare model, per the two-tier collection and bare-model rules.
func TestAPIContract_ResponseShapes(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)

	chapters := f.request(http.MethodGet, fmt.Sprintf("/api/books/files/%d/chapters", seeded.fileID), "", http.StatusOK)
	var chapterList []json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(chapters), &chapterList), "chapters list must be a bare array: %s", chapters)

	replaced := f.request(http.MethodPut, fmt.Sprintf("/api/books/files/%d/chapters", seeded.fileID), `{"chapters":[{"title":"One","href":"one.xhtml","children":[]}]}`, http.StatusOK)
	require.NoError(t, json.Unmarshal([]byte(replaced), &chapterList), "chapters replace must return a bare array: %s", replaced)
	assert.Len(t, chapterList, 1)

	caches := f.request(http.MethodGet, "/api/cache", "", http.StatusOK)
	var cacheList []map[string]any
	require.NoError(t, json.Unmarshal([]byte(caches), &cacheList), "cache list must be a bare array: %s", caches)
	assert.Len(t, cacheList, 3)

	library := f.request(http.MethodGet, fmt.Sprintf("/api/libraries/%d", f.lib.ID), "", http.StatusOK)
	var lib map[string]any
	require.NoError(t, json.Unmarshal([]byte(library), &lib))
	assert.InDelta(t, float64(f.lib.ID), lib["id"], 0)
	assert.Equal(t, "Library", lib["name"])
}

// API keys serialize with snake_case keys like every other /api payload.
func TestAPIContract_APIKeyJSONIsSnakeCase(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)

	created := f.request(http.MethodPost, "/api/user/api-keys", `{"name":"Reader"}`, http.StatusCreated)
	var key map[string]any
	require.NoError(t, json.Unmarshal([]byte(created), &key))
	for _, k := range []string{"id", "user_id", "name", "key", "created_at", "updated_at", "last_accessed_at", "permissions"} {
		assert.Contains(t, key, k)
	}
	for _, k := range []string{"userId", "createdAt", "updatedAt", "lastAccessedAt"} {
		assert.NotContains(t, key, k)
	}

	id, ok := key["id"].(string)
	require.True(t, ok)
	withPermission := f.request(http.MethodPost, fmt.Sprintf("/api/user/api-keys/%s/permissions/%s", id, "ereader_browser"), "", http.StatusOK)
	var keyWithPermission struct {
		Permissions []map[string]any `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal([]byte(withPermission), &keyWithPermission))
	require.Len(t, keyWithPermission.Permissions, 1)
	assert.Contains(t, keyWithPermission.Permissions[0], "api_key_id")
	assert.Contains(t, keyWithPermission.Permissions[0], "created_at")

	shortURL := f.request(http.MethodPost, fmt.Sprintf("/api/user/api-keys/%s/short-url", id), "", http.StatusCreated)
	var short map[string]any
	require.NoError(t, json.Unmarshal([]byte(shortURL), &short))
	for _, k := range []string{"id", "api_key_id", "short_code", "expires_at", "created_at"} {
		assert.Contains(t, short, k)
	}
}

// A failed count or alias lookup fails the request with a 500 rather than
// rendering a zero count or an empty alias list. Each case drops the table
// the lookup reads, after seeding, on a fixture of its own.
func TestAPIContract_CountAndAliasFailuresSurface(t *testing.T) {
	t.Parallel()

	type ids struct {
		genre, tag, series, person, publisher, list int
	}
	tests := []struct {
		table string
		paths func(ids) []string
	}{
		{"book_genres", func(i ids) []string {
			return []string{fmt.Sprintf("/api/genres/%d", i.genre), "/api/genres", fmt.Sprintf("PATCH /api/genres/%d", i.genre)}
		}},
		{"genre_aliases", func(i ids) []string {
			return []string{fmt.Sprintf("/api/genres/%d", i.genre), "/api/genres", fmt.Sprintf("PATCH /api/genres/%d", i.genre)}
		}},
		{"book_tags", func(i ids) []string {
			return []string{fmt.Sprintf("/api/tags/%d", i.tag), "/api/tags", fmt.Sprintf("PATCH /api/tags/%d", i.tag)}
		}},
		{"tag_aliases", func(i ids) []string {
			return []string{fmt.Sprintf("/api/tags/%d", i.tag), "/api/tags", fmt.Sprintf("PATCH /api/tags/%d", i.tag)}
		}},
		{"book_series", func(i ids) []string {
			return []string{fmt.Sprintf("/api/series/%d", i.series), "/api/series", fmt.Sprintf("PATCH /api/series/%d", i.series)}
		}},
		{"series_aliases", func(i ids) []string {
			return []string{fmt.Sprintf("/api/series/%d", i.series), "/api/series", fmt.Sprintf("PATCH /api/series/%d", i.series)}
		}},
		{"person_aliases", func(i ids) []string {
			return []string{fmt.Sprintf("/api/people/%d", i.person), "/api/people", fmt.Sprintf("PATCH /api/people/%d", i.person)}
		}},
		{"authors", func(i ids) []string {
			return []string{fmt.Sprintf("/api/people/%d", i.person), "/api/people", fmt.Sprintf("PATCH /api/people/%d", i.person)}
		}},
		{"narrators", func(i ids) []string {
			return []string{fmt.Sprintf("/api/people/%d", i.person), "/api/people", fmt.Sprintf("PATCH /api/people/%d", i.person)}
		}},
		{"publisher_aliases", func(i ids) []string {
			return []string{fmt.Sprintf("/api/publishers/%d", i.publisher), "/api/publishers"}
		}},
		{"list_books", func(i ids) []string {
			return []string{fmt.Sprintf("/api/lists/%d", i.list), "/api/lists"}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.table, func(t *testing.T) {
			t.Parallel()
			f := newResourceDeleteFixture(t)
			seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
			series := f.seriesWithBooks("Tidewater", seeded.bookID)
			publisher := &models.Publisher{LibraryID: f.lib.ID, Name: "Harbor Press"}
			f.insert(publisher)
			list := &models.List{CreatedAt: time.Now(), UpdatedAt: time.Now(), UserID: f.admin.ID, Name: "Favorites", DefaultSort: models.ListSortAddedAtDesc}
			f.insert(list)
			paths := tt.paths(ids{
				genre: seeded.genre.ID, tag: seeded.tag.ID, series: series.ID,
				person: seeded.author.ID, publisher: publisher.ID, list: list.ID,
			})

			_, err := f.db.ExecContext(f.ctx, "DROP TABLE "+tt.table)
			require.NoError(t, err)

			for _, path := range paths {
				method, body := http.MethodGet, ""
				if rest, ok := strings.CutPrefix(path, "PATCH "); ok {
					method, path, body = http.MethodPatch, rest, `{}`
				}
				status, code, _ := f.requestError(t, method, path, body)
				assert.Equal(t, http.StatusInternalServerError, status, "%s %s", method, path)
				assert.Equal(t, "internal_server_error", code, "%s %s", method, path)
			}
		})
	}
}

// Deleting a Genre, Tag, or Publisher drops its own FTS row through the
// deferred ReindexAffected, like every other mutation.
func TestAPIContract_DeleteDropsResourceFTSRow(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	publisher := &models.Publisher{LibraryID: f.lib.ID, Name: "Lighthouse Press"}
	f.insert(publisher)
	require.NoError(t, f.searchSvc.IndexPublisher(f.ctx, publisher))
	require.NoError(t, f.searchSvc.IndexGenre(f.ctx, seeded.genre))
	require.NoError(t, f.searchSvc.IndexTag(f.ctx, seeded.tag))

	tests := []struct {
		table, path string
		id          int
	}{
		{"genres_fts", fmt.Sprintf("/api/genres/%d", seeded.genre.ID), seeded.genre.ID},
		{"tags_fts", fmt.Sprintf("/api/tags/%d", seeded.tag.ID), seeded.tag.ID},
		{"publishers_fts", fmt.Sprintf("/api/publishers/%d", publisher.ID), publisher.ID},
	}
	for _, tt := range tests {
		var before int
		require.NoError(t, f.db.NewRaw("SELECT COUNT(*) FROM "+tt.table+" WHERE rowid = ?", tt.id).Scan(f.ctx, &before))
		require.Equal(t, 1, before, "precondition: %s holds the row", tt.table)

		f.delete(tt.path)

		var after int
		require.NoError(t, f.db.NewRaw("SELECT COUNT(*) FROM "+tt.table+" WHERE rowid = ?", tt.id).Scan(f.ctx, &after))
		assert.Zero(t, after, "%s drops the deleted row", tt.table)
	}
}

// A target book lookup that fails for any reason but a missing row is a
// server fault, not a 404. The target's created_at holds a value that cannot
// scan into a time, so only the target's lookup fails.
func TestAPIContract_MoveFilesTargetLookupFaultIsServerError(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	source := f.seedReviewedBook(models.FileTypeEPUB, nil)
	target := f.seedReviewedBook(models.FileTypeEPUB, &source)
	_, err := f.db.ExecContext(f.ctx, "UPDATE books SET created_at = 'not a time' WHERE id = ?", target.bookID)
	require.NoError(t, err)

	status, code, _ := f.requestError(t, http.MethodPost, fmt.Sprintf("/api/books/%d/move-files", source.bookID),
		fmt.Sprintf(`{"file_ids":[%d],"target_book_id":%d}`, source.fileID, target.bookID))
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, "internal_server_error", code)
}

// A publisher whose parent_id names no row still lists, with no parent name,
// instead of failing the whole page with a 404.
func TestAPIContract_PublisherListToleratesDanglingParent(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	missing := 999999
	_, err := f.db.ExecContext(f.ctx, "PRAGMA foreign_keys = OFF")
	require.NoError(t, err)
	f.insert(&models.Publisher{LibraryID: f.lib.ID, Name: "Orphaned Imprint", ParentID: &missing})
	_, err = f.db.ExecContext(f.ctx, "PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	body := f.request(http.MethodGet, "/api/publishers", "", http.StatusOK)
	var resp struct {
		Items []struct {
			Name       string  `json:"name"`
			ParentName *string `json:"parent_name"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "Orphaned Imprint", resp.Items[0].Name)
	assert.Nil(t, resp.Items[0].ParentName)
}

// MoveFilesToBook's own validation (a file outside the library) is a 422,
// and any other failure, here a missing library_paths table it reads after
// validating the files, is a 500 for both the move and the merge routes.
func TestAPIContract_MoveFilesServiceErrors(t *testing.T) {
	t.Parallel()

	t.Run("file outside the library", func(t *testing.T) {
		t.Parallel()
		f := newResourceDeleteFixture(t)
		source := f.seedReviewedBook(models.FileTypeEPUB, nil)
		target := f.seedReviewedBook(models.FileTypeEPUB, &source)
		status, code, _ := f.requestError(t, http.MethodPost, fmt.Sprintf("/api/books/%d/move-files", source.bookID),
			fmt.Sprintf(`{"file_ids":[%d,999999],"target_book_id":%d}`, source.fileID, target.bookID))
		assert.Equal(t, http.StatusUnprocessableEntity, status)
		assert.Equal(t, "validation_error", code)
	})

	for _, route := range []string{"move", "merge"} {
		t.Run(route+" with a database fault", func(t *testing.T) {
			t.Parallel()
			f := newResourceDeleteFixture(t)
			source := f.seedReviewedBook(models.FileTypeEPUB, nil)
			target := f.seedReviewedBook(models.FileTypeEPUB, &source)
			_, err := f.db.ExecContext(f.ctx, "DROP TABLE library_paths")
			require.NoError(t, err)

			path := fmt.Sprintf("/api/books/%d/move-files", source.bookID)
			body := fmt.Sprintf(`{"file_ids":[%d],"target_book_id":%d}`, source.fileID, target.bookID)
			if route == "merge" {
				path = "/api/books/merge"
				body = fmt.Sprintf(`{"target_book_id":%d,"source_book_ids":[%d]}`, target.bookID, source.bookID)
			}
			status, code, _ := f.requestError(t, http.MethodPost, path, body)
			assert.Equal(t, http.StatusInternalServerError, status)
			assert.Equal(t, "internal_server_error", code)
		})
	}
}

// A user lookup that fails for any reason but a missing or deactivated user
// is a server fault on every authenticating path, not a 401 that signs the
// user out or prompts an OPDS reader for credentials again. The admin's
// created_at holds a value that cannot scan into a time.
func TestAPIContract_UserLookupFaultIsServerError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  func(f *resourceDeleteFixture) *http.Request
	}{
		{"/api/auth/me", func(f *resourceDeleteFixture) *http.Request {
			return f.sessionRequest("/api/auth/me")
		}},
		{"a session route", func(f *resourceDeleteFixture) *http.Request {
			return f.sessionRequest("/api/libraries")
		}},
		{"OPDS Basic Auth", func(f *resourceDeleteFixture) *http.Request {
			req := httptest.NewRequest(http.MethodGet, "/opds/v1/epub/catalog", nil)
			req.SetBasicAuth(f.admin.Username, "any password")
			return req
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newResourceDeleteFixture(t)
			req := tt.req(f)
			_, err := f.db.ExecContext(f.ctx, "UPDATE users SET created_at = 'not a time' WHERE id = ?", f.admin.ID)
			require.NoError(t, err)

			rec := httptest.NewRecorder()
			f.handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusInternalServerError, rec.Code, "response body: %s", rec.Body.String())
			assert.Empty(t, rec.Header().Get("WWW-Authenticate"))
		})
	}
}

// A session whose user has been deactivated gets the shared 401 on
// /api/auth/me, the same code and message as every other session route.
func TestAPIContract_MeWithDeactivatedUser(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	req := f.sessionRequest("/api/auth/me")
	_, err := f.db.ExecContext(f.ctx, "UPDATE users SET is_active = 0 WHERE id = ?", f.admin.ID)
	require.NoError(t, err)

	for _, r := range []*http.Request{req, f.sessionRequest("/api/libraries")} {
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, r)
		var resp errorResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), "response body: %s", rec.Body.String())
		assert.Equal(t, http.StatusUnauthorized, rec.Code, r.URL.Path)
		assert.Equal(t, "unauthorized", resp.Error.Code, r.URL.Path)
		assert.Equal(t, "User not found or inactive", resp.Error.Message, r.URL.Path)
	}
}

// A merge source whose lookup fails for any reason but a missing row is a
// server fault, not a 404. The source's created_at cannot scan into a time.
func TestAPIContract_MergeSourceLookupFaultIsServerError(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	target := f.seedReviewedBook(models.FileTypeEPUB, nil)
	source := f.seedReviewedBook(models.FileTypeEPUB, &target)
	_, err := f.db.ExecContext(f.ctx, "UPDATE books SET created_at = 'not a time' WHERE id = ?", source.bookID)
	require.NoError(t, err)

	status, code, _ := f.requestError(t, http.MethodPost, "/api/books/merge",
		fmt.Sprintf(`{"target_book_id":%d,"source_book_ids":[%d]}`, target.bookID, source.bookID))
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, "internal_server_error", code)

	status, code, message := f.requestError(t, http.MethodPost, "/api/books/merge",
		fmt.Sprintf(`{"target_book_id":%d,"source_book_ids":[999999]}`, target.bookID))
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, "not_found", code)
	assert.Equal(t, "Book not found.", message)
}

// sessionRequest builds a GET request carrying the admin's session cookie.
func (f *resourceDeleteFixture) sessionRequest(path string) *http.Request {
	f.t.Helper()
	token, err := f.authSvc.GenerateToken(f.admin)
	require.NoError(f.t, err)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	return req
}
