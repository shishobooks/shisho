package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// routePermissionsFixture serves the real route table with two libraries and
// lets each test build roles holding exactly the permissions it needs, so a
// route's middleware is checked against the pages that call it.
type routePermissionsFixture struct {
	t         *testing.T
	ctx       context.Context
	db        *bun.DB
	handler   http.Handler
	authSvc   *auth.Service
	searchSvc *search.Service
	libA      *models.Library
	libB      *models.Library
	admin     *models.User
}

func newRoutePermissionsFixture(t *testing.T) *routePermissionsFixture {
	t.Helper()
	db := testdb.New(t)
	cfg := newPermissionTestConfig(t)
	cfg.PluginDir = t.TempDir()
	pluginSvc := plugins.NewService(db)
	pm := plugins.NewManager(pluginSvc, cfg.PluginDir, t.TempDir())
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil, nil, nil, nil), pluginSvc, pm, nil,
		downloadcache.NewCache(t.TempDir(), 1<<30), nil, nil, nil)
	require.NoError(t, err)
	f := &routePermissionsFixture{
		t:         t,
		ctx:       t.Context(),
		db:        db,
		handler:   srv.Handler,
		authSvc:   auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration()),
		searchSvc: search.NewService(db),
	}
	f.libA = &models.Library{Name: "Alpha Library", CoverAspectRatio: models.CoverAspectRatioBook, DownloadFormatPreference: models.DownloadFormatKepub, OrganizeFileStructure: true}
	f.insert(f.libA)
	f.libB = &models.Library{Name: "Beta Library", CoverAspectRatio: models.CoverAspectRatioAudiobook, DownloadFormatPreference: models.DownloadFormatOriginal}
	f.insert(f.libB)
	f.insert(&models.LibraryPath{LibraryID: f.libA.ID, Filepath: "/secret/alpha"})
	f.admin = insertPermissionTestUser(f.ctx, t, db, "admin", models.RoleAdmin, nil)
	return f
}

func (f *routePermissionsFixture) insert(model any) {
	f.t.Helper()
	_, err := f.db.NewInsert().Model(model).Exec(f.ctx)
	require.NoError(f.t, err)
}

// role creates a role holding exactly the given "resource:operation"
// permissions and returns its name.
func (f *routePermissionsFixture) role(name string, perms ...string) string {
	f.t.Helper()
	role := &models.Role{Name: name}
	f.insert(role)
	for _, p := range perms {
		resource, operation, ok := strings.Cut(p, ":")
		require.True(f.t, ok, "permission %q must be resource:operation", p)
		f.insert(&models.Permission{RoleID: role.ID, Resource: resource, Operation: operation})
	}
	return name
}

// user creates an active user with the named role and access to the given
// libraries. No library IDs grants access to every library.
func (f *routePermissionsFixture) user(username, roleName string, libraryIDs ...int) *models.User {
	f.t.Helper()
	if len(libraryIDs) == 0 {
		return insertPermissionTestUser(f.ctx, f.t, f.db, username, roleName, nil)
	}
	user := insertPermissionTestUser(f.ctx, f.t, f.db, username, roleName, &libraryIDs[0])
	for _, id := range libraryIDs[1:] {
		f.insert(&models.UserLibraryAccess{UserID: user.ID, LibraryID: &id})
	}
	return user
}

// do sends a request as user, or anonymously when user is nil.
func (f *routePermissionsFixture) do(user *models.User, method, path, body string) *httptest.ResponseRecorder {
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

// expect sends a request and requires the status, returning the body.
func (f *routePermissionsFixture) expect(user *models.User, method, path, body string, wantStatus int) string {
	f.t.Helper()
	rec := f.do(user, method, path, body)
	require.Equal(f.t, wantStatus, rec.Code, "%s %s: %s", method, path, rec.Body.String())
	return rec.Body.String()
}

func (f *routePermissionsFixture) book(lib *models.Library, title string) *models.Book {
	f.t.Helper()
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           title,
		TitleSource:     models.DataSourceManual,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceManual,
		Filepath:        f.t.TempDir(),
	}
	f.insert(book)
	return book
}

// A Books Read role without Libraries Read lists exactly the libraries it can
// reach, with only the fields the reader pages need.
func TestUserLibraries_ListsAccessibleLibrariesWithoutLibrariesRead(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	reader := f.role("reader", "books:read")

	limited := f.user("limited", reader, f.libA.ID)
	body := f.expect(limited, http.MethodGet, "/api/user/libraries", "", http.StatusOK)
	assert.JSONEq(t, fmt.Sprintf(
		`[{"id":%d,"name":"Alpha Library","cover_aspect_ratio":"book","download_format_preference":"kepub","organize_file_structure":true}]`,
		f.libA.ID), body, "only the accessible library, without paths or timestamps")

	everywhere := f.user("everywhere", reader)
	body = f.expect(everywhere, http.MethodGet, "/api/user/libraries", "", http.StatusOK)
	var all []map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &all))
	require.Len(t, all, 2, "all-library access lists every library")
	assert.Equal(t, "Alpha Library", all[0]["name"], "sorted by name")
	assert.Equal(t, "Beta Library", all[1]["name"], "sorted by name")

	// A user with no library access rows reaches no library.
	nowhere := &models.User{Username: "nowhere", PasswordHash: "unused", RoleID: limited.RoleID, IsActive: true}
	f.insert(nowhere)
	assert.JSONEq(t, `[]`, f.expect(nowhere, http.MethodGet, "/api/user/libraries", "", http.StatusOK))

	f.expect(nil, http.MethodGet, "/api/user/libraries", "", http.StatusUnauthorized)
}

// A user with no library access rows must not see every library from the
// admin listing either. An empty access list used to disable the filter.
func TestListLibraries_NoLibraryAccessListsNothing(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	librarian := f.role("librarian", "libraries:read")
	var roleID int
	require.NoError(t, f.db.NewRaw("SELECT id FROM roles WHERE name = ?", librarian).Scan(f.ctx, &roleID))
	nowhere := &models.User{Username: "nowhere", PasswordHash: "unused", RoleID: roleID, IsActive: true}
	f.insert(nowhere)

	assert.JSONEq(t, `{"items":[],"total":0}`, f.expect(nowhere, http.MethodGet, "/api/libraries", "", http.StatusOK))
}

// Library languages are book data: Books Read and library access reach them
// without Libraries Read.
func TestLibraryLanguages_RequireBooksReadNotLibrariesRead(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	reader := f.user("reader", f.role("reader", "books:read"), f.libA.ID)
	librarian := f.user("librarian", f.role("librarian", "libraries:read"), f.libA.ID)

	f.expect(reader, http.MethodGet, fmt.Sprintf("/api/libraries/%d/languages", f.libA.ID), "", http.StatusOK)
	f.expect(reader, http.MethodGet, fmt.Sprintf("/api/libraries/%d/languages", f.libB.ID), "", http.StatusForbidden)
	f.expect(librarian, http.MethodGet, fmt.Sprintf("/api/libraries/%d/languages", f.libA.ID), "", http.StatusForbidden)
	// The rest of the libraries family still requires Libraries Read.
	f.expect(reader, http.MethodGet, fmt.Sprintf("/api/libraries/%d", f.libA.ID), "", http.StatusForbidden)
	f.expect(reader, http.MethodGet, fmt.Sprintf("/api/libraries/%d/plugins/order/%s", f.libA.ID, models.PluginHookMetadataEnricher), "", http.StatusForbidden)
}

// A Users Write role lists libraries to assign access, and nothing else in
// the libraries family opens up to it.
func TestListLibraries_UsersWriteWithoutLibrariesRead(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	userAdmin := f.user("user-admin", f.role("user-admin", "users:read", "users:write"))
	usersReadOnly := f.user("users-read", f.role("users-read", "users:read"))

	body := f.expect(userAdmin, http.MethodGet, "/api/libraries?limit=100", "", http.StatusOK)
	var resp struct {
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	assert.Equal(t, 2, resp.Total)

	f.expect(usersReadOnly, http.MethodGet, "/api/libraries", "", http.StatusForbidden)
	f.expect(userAdmin, http.MethodGet, fmt.Sprintf("/api/libraries/%d", f.libA.ID), "", http.StatusForbidden)
	f.expect(userAdmin, http.MethodPost, fmt.Sprintf("/api/libraries/%d", f.libA.ID), `{"name":"Renamed"}`, http.StatusForbidden)
}

// A Config Read role can load every plugin read route the plugins page calls,
// while every mutation stays on Config Write.
func TestPluginManagementReads_ConfigRead(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	auditor := f.user("auditor", f.role("auditor", "config:read"))
	// Keep listAvailable off the network.
	_, err := f.db.NewDelete().Model((*models.PluginRepository)(nil)).Where("1 = 1").Exec(f.ctx)
	require.NoError(t, err)
	f.insert(&models.Plugin{Scope: "community", ID: "example", Name: "Example", Version: "1.0.0", InstalledAt: time.Now()})

	reads := []string{
		"/api/plugins/installed",
		"/api/plugins/available",
		"/api/plugins/available/community/example",
		"/api/plugins/repositories",
		"/api/plugins/installed/community/example/config",
		"/api/plugins/installed/community/example/fields",
		"/api/plugins/installed/community/example/manifest",
		"/api/plugins/installed/community/example/image",
	}
	for _, path := range reads {
		adminRec := f.do(f.admin, http.MethodGet, path, "")
		require.NotEqual(t, http.StatusForbidden, adminRec.Code, path)
		rec := f.do(auditor, http.MethodGet, path, "")
		assert.Equal(t, adminRec.Code, rec.Code, "config:read gets the admin's status from %s: %s", path, rec.Body.String())
		assert.Equal(t, adminRec.Body.String(), rec.Body.String(), path)
	}
	assert.Contains(t, f.expect(auditor, http.MethodGet, "/api/plugins/installed", "", http.StatusOK), `"id":"example"`)

	mutations := []struct{ method, path, body string }{
		{http.MethodPost, "/api/plugins/installed", `{}`},
		{http.MethodPost, "/api/plugins/scan", ""},
		{http.MethodDelete, "/api/plugins/installed/community/example", ""},
		{http.MethodPatch, "/api/plugins/installed/community/example", `{}`},
		{http.MethodPut, "/api/plugins/installed/community/example/fields", `{"fields":{}}`},
		{http.MethodPost, "/api/plugins/installed/community/example/reload", ""},
		{http.MethodPost, "/api/plugins/installed/community/example/update", ""},
		{http.MethodPut, "/api/plugins/order/" + models.PluginHookMetadataEnricher, `{"order":[]}`},
		{http.MethodPost, "/api/plugins/repositories", `{}`},
		{http.MethodDelete, "/api/plugins/repositories/community", ""},
		{http.MethodPost, "/api/plugins/repositories/community/sync", ""},
	}
	for _, m := range mutations {
		f.expect(auditor, m.method, m.path, m.body, http.StatusForbidden)
	}
}

// The review criteria page needs Config Read, the review panel Books Read.
func TestReviewCriteria_BooksReadOrConfigRead(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	auditor := f.user("auditor", f.role("auditor", "config:read"))
	reader := f.user("reader", f.role("reader", "books:read"))
	neither := f.user("neither", f.role("neither", "people:read"))

	f.expect(auditor, http.MethodGet, "/api/settings/review-criteria", "", http.StatusOK)
	f.expect(reader, http.MethodGet, "/api/settings/review-criteria", "", http.StatusOK)
	f.expect(neither, http.MethodGet, "/api/settings/review-criteria", "", http.StatusForbidden)
	f.expect(auditor, http.MethodPut, "/api/settings/review-criteria", `{"book_fields":[],"audio_fields":[]}`, http.StatusForbidden)
}

// Any signed-in user can list who they can share a list with, and the
// directory carries only ids and usernames of active users.
func TestUsersDirectory_AuthenticatedIDsAndUsernamesOnly(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	viewer := f.user("viewer", models.RoleViewer)
	email := "hidden@example.com"
	_, err := f.db.NewUpdate().Model(viewer).Set("email = ?", email).WherePK().Exec(f.ctx)
	require.NoError(t, err)
	gone := f.user("gone", models.RoleViewer)
	_, err = f.db.NewUpdate().Model(gone).Set("is_active = ?", false).WherePK().Exec(f.ctx)
	require.NoError(t, err)

	body := f.expect(viewer, http.MethodGet, "/api/users/directory", "", http.StatusOK)
	assert.JSONEq(t, fmt.Sprintf(`[{"id":%d,"username":"admin"},{"id":%d,"username":"viewer"}]`, f.admin.ID, viewer.ID), body)
	assert.NotContains(t, body, email)

	f.expect(nil, http.MethodGet, "/api/users/directory", "", http.StatusUnauthorized)
	// The full users listing still needs Users Read.
	f.expect(viewer, http.MethodGet, "/api/users", "", http.StatusForbidden)
}

// A list owner shares the list without Users Read. Managing shares depends
// on the caller's permission on the list, and the payload does not expose
// the recipients' email addresses.
func TestListShares_OwnerWithoutUsersRead(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	owner := f.user("owner", models.RoleViewer)
	friend := f.user("friend", models.RoleViewer)
	stranger := f.user("stranger", models.RoleViewer)
	email := "friend@example.com"
	_, err := f.db.NewUpdate().Model(friend).Set("email = ?", email).WherePK().Exec(f.ctx)
	require.NoError(t, err)
	list, err := lists.NewService(f.db).CreateList(f.ctx, lists.CreateListOptions{UserID: owner.ID, Name: "Shared"})
	require.NoError(t, err)
	base := fmt.Sprintf("/api/lists/%d/shares", list.ID)

	body := f.expect(owner, http.MethodPost, base, fmt.Sprintf(`{"user_id":%d,"permission":"viewer"}`, friend.ID), http.StatusCreated)
	assertOnlyUserIDAndUsername(t, body, email)
	var share models.ListShare
	require.NoError(t, json.Unmarshal([]byte(body), &share))

	body = f.expect(owner, http.MethodGet, base, "", http.StatusOK)
	assert.Contains(t, body, `"username":"friend"`)
	assertOnlyUserIDAndUsername(t, body, email)
	f.expect(owner, http.MethodGet, base+"/check?user_id="+strconv.Itoa(friend.ID), "", http.StatusOK)
	f.expect(owner, http.MethodPatch, fmt.Sprintf("%s/%d", base, share.ID), `{"permission":"editor"}`, http.StatusNoContent)

	// Only active users can receive a share, and an unknown id gets the same
	// answer, so the share route cannot probe for accounts.
	gone := f.user("gone", models.RoleViewer)
	_, err = f.db.NewUpdate().Model(gone).Set("is_active = ?", false).WherePK().Exec(f.ctx)
	require.NoError(t, err)
	inactiveBody := f.expect(owner, http.MethodPost, base, fmt.Sprintf(`{"user_id":%d,"permission":"viewer"}`, gone.ID), http.StatusUnprocessableEntity)
	unknownBody := f.expect(owner, http.MethodPost, base, `{"user_id":999999,"permission":"viewer"}`, http.StatusUnprocessableEntity)
	assert.Equal(t, unknownBody, inactiveBody, "an inactive and an unknown user get the same response")

	// Someone without manage permission on the list is still refused.
	f.expect(stranger, http.MethodGet, base, "", http.StatusForbidden)
	f.expect(friend, http.MethodGet, base, "", http.StatusForbidden)

	f.expect(owner, http.MethodDelete, fmt.Sprintf("%s/%d", base, share.ID), "", http.StatusNoContent)
}

// assertOnlyUserIDAndUsername requires that the users embedded in a list
// payload carry no email address, role, or account state. Any role can share
// a list, so a recipient must learn nothing about the owner beyond the name.
func assertOnlyUserIDAndUsername(t *testing.T, body string, emails ...string) {
	t.Helper()
	for _, email := range emails {
		assert.NotContains(t, body, email, "no email addresses")
	}
	for _, key := range []string{`"email"`, `"role_id"`, `"is_active"`, `"must_change_password"`} {
		assert.NotContains(t, body, key, "embedded users carry only id and username")
	}
}

// A list's owner and whoever added each book appear to every recipient of a
// shared list with only their id and username.
func TestSharedList_RecipientSeesOnlyOwnerUsername(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	owner := f.user("owner", models.RoleViewer)
	friend := f.user("friend", models.RoleViewer)
	email := "owner@example.com"
	_, err := f.db.NewUpdate().Model(owner).Set("email = ?", email).WherePK().Exec(f.ctx)
	require.NoError(t, err)
	list, err := lists.NewService(f.db).CreateList(f.ctx, lists.CreateListOptions{UserID: owner.ID, Name: "Shared"})
	require.NoError(t, err)
	book := f.book(f.libA, "Listed Book")
	f.expect(owner, http.MethodPost, fmt.Sprintf("/api/lists/%d/books", list.ID), fmt.Sprintf(`{"book_ids":[%d]}`, book.ID), http.StatusNoContent)
	f.expect(owner, http.MethodPost, fmt.Sprintf("/api/lists/%d/shares", list.ID), fmt.Sprintf(`{"user_id":%d,"permission":"viewer"}`, friend.ID), http.StatusCreated)

	for _, path := range []string{"/api/lists", fmt.Sprintf("/api/lists/%d", list.ID), fmt.Sprintf("/api/lists/%d/books", list.ID)} {
		body := f.expect(friend, http.MethodGet, path, "", http.StatusOK)
		assert.Contains(t, body, `"username":"owner"`, path)
		assertOnlyUserIDAndUsername(t, body, email)
	}
}

// A list's books are book data, so reading them requires Books Read even for
// the list's owner.
func TestListBooks_RequireBooksRead(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	noBooks := f.user("no-books", f.role("no-books", "people:read"))
	viewer := f.user("viewer", models.RoleViewer)
	svc := lists.NewService(f.db)
	noBooksList, err := svc.CreateList(f.ctx, lists.CreateListOptions{UserID: noBooks.ID, Name: "Mine"})
	require.NoError(t, err)
	viewerList, err := svc.CreateList(f.ctx, lists.CreateListOptions{UserID: viewer.ID, Name: "Mine"})
	require.NoError(t, err)

	f.expect(noBooks, http.MethodGet, fmt.Sprintf("/api/lists/%d/books", noBooksList.ID), "", http.StatusForbidden)
	f.expect(viewer, http.MethodGet, fmt.Sprintf("/api/lists/%d/books", viewerList.ID), "", http.StatusOK)
	// The list itself stays readable without Books Read.
	f.expect(noBooks, http.MethodGet, fmt.Sprintf("/api/lists/%d", noBooksList.ID), "", http.StatusOK)
}

// Global search only returns the series and people sections to roles that
// can read those resources.
func TestGlobalSearch_FiltersSectionsByPermission(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	book := f.book(f.libA, "Marrowind Chronicle")
	series := &models.Series{LibraryID: f.libA.ID, Name: "Marrowind Saga", NameSource: models.DataSourceManual, SortName: "Marrowind Saga", SortNameSource: models.DataSourceFilepath}
	f.insert(series)
	f.insert(&models.BookSeries{BookID: book.ID, SeriesID: series.ID, SortOrder: 1})
	person := &models.Person{LibraryID: f.libA.ID, Name: "Marrowind Pell", SortName: "Pell, Marrowind", SortNameSource: models.DataSourceFilepath}
	f.insert(person)
	f.insert(&models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: 1})
	require.NoError(t, f.searchSvc.ReindexBookByID(f.ctx, book.ID))
	require.NoError(t, f.searchSvc.IndexSeries(f.ctx, series))
	require.NoError(t, f.searchSvc.IndexPerson(f.ctx, person))

	path := fmt.Sprintf("/api/search?q=Marrowind&library_id=%d", f.libA.ID)
	sections := func(user *models.User) (int, int, int) {
		t.Helper()
		var resp struct {
			Books  []any `json:"books"`
			Series []any `json:"series"`
			People []any `json:"people"`
		}
		body := f.expect(user, http.MethodGet, path, "", http.StatusOK)
		require.NoError(t, json.Unmarshal([]byte(body), &resp))
		require.NotNil(t, resp.Series, "the series key is always present")
		require.NotNil(t, resp.People, "the people key is always present")
		return len(resp.Books), len(resp.Series), len(resp.People)
	}

	b, s, p := sections(f.user("viewer", models.RoleViewer))
	assert.Equal(t, []int{1, 1, 1}, []int{b, s, p}, "a viewer sees every section")

	b, s, p = sections(f.user("books-only", f.role("books-only", "books:read")))
	assert.Equal(t, []int{1, 0, 0}, []int{b, s, p}, "books:read alone sees only books")

	b, s, p = sections(f.user("with-series", f.role("with-series", "books:read", "series:read")))
	assert.Equal(t, []int{1, 1, 0}, []int{b, s, p}, "series:read adds the series section")

	b, s, p = sections(f.user("with-people", f.role("with-people", "books:read", "people:read")))
	assert.Equal(t, []int{1, 0, 1}, []int{b, s, p}, "people:read adds the people section")
}

// A role holding only shares operations manages a book's Share Links without
// Books Read; the handlers still check the book's library access.
func TestShareLinkManagement_SharesOnlyRole(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	f.expect(f.admin, http.MethodPut, "/api/settings/sharing", `{"enabled":true,"require_expiration":false}`, http.StatusOK)
	sharesRole := f.role("sharer", "shares:read", "shares:write")
	sharer := f.user("sharer", sharesRole, f.libA.ID)
	outsider := f.user("outsider", sharesRole, f.libB.ID)
	noShares := f.user("no-shares", f.role("no-shares", "people:read"), f.libA.ID)
	book := f.book(f.libA, "Shared Book")
	base := fmt.Sprintf("/api/books/%d/share-links", book.ID)

	body := f.expect(sharer, http.MethodPost, base, `{}`, http.StatusCreated)
	var link struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &link))
	f.expect(sharer, http.MethodGet, base, "", http.StatusOK)
	f.expect(sharer, http.MethodPost, fmt.Sprintf("%s/%d/revoke", base, link.ID), "", http.StatusOK)

	f.expect(outsider, http.MethodGet, base, "", http.StatusForbidden)
	f.expect(noShares, http.MethodGet, base, "", http.StatusForbidden)
	f.expect(noShares, http.MethodPost, base, `{}`, http.StatusForbidden)
	f.expect(nil, http.MethodGet, base, "", http.StatusUnauthorized)

	f.expect(sharer, http.MethodDelete, fmt.Sprintf("%s/%d", base, link.ID), "", http.StatusNoContent)
	// The rest of the books family still requires Books Read.
	f.expect(sharer, http.MethodGet, fmt.Sprintf("/api/books/%d", book.ID), "", http.StatusForbidden)
}
