package shishoapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/bookmarks"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
)

const (
	testPassword   = "correct horse"
	testInstanceID = "instance-1"
)

type fixture struct {
	t        *testing.T
	ctx      context.Context
	db       *bun.DB
	e        *echo.Echo
	h        *handler
	password string
}

// newFixture serves the Device surface. opts configure its download cache,
// whose generators are the real ones without them.
func newFixture(t *testing.T, opts ...downloadcache.Option) *fixture {
	t.Helper()
	return newFixtureWith(t, newContract(ProtocolMin, Protocol), opts...)
}

func newFixtureWith(t *testing.T, contract *Contract, opts ...downloadcache.Option) *fixture {
	t.Helper()
	return newFixtureFull(t, contract, downloadcache.NewCache(t.TempDir(), 1<<30, opts...))
}

// newFixtureWithCache serves the Device surface with cache as its download
// cache.
func newFixtureWithCache(t *testing.T, cache *downloadcache.Cache) *fixture {
	t.Helper()
	return newFixtureFull(t, newContract(ProtocolMin, Protocol), cache)
}

func newFixtureFull(t *testing.T, contract *Contract, cache *downloadcache.Cache) *fixture {
	t.Helper()
	db := testdb.New(t)
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	e.Use(contract.CapabilitiesHeader)

	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	require.NoError(t, err)
	deviceService := devices.NewService(db)
	// Builds run outside requests; they end before the directories go.
	t.Cleanup(cache.Wait)
	h := &handler{
		db:        db,
		books:     books.NewService(db, appsettings.NewService(db)),
		downloads: cache,
		auth:      auth.NewService(db, "secret", time.Hour),
		devices:   deviceService,
		feed:      changefeed.NewService(db),
		reading:   reading.NewService(db),
		bookmarks: bookmarks.NewService(db),
		contract:  contract,
		opts:      Options{Contract: contract, ServerVersion: "v0.10.0", InstanceID: testInstanceID},
	}
	registerRoutes(e, h, devices.NewMiddleware(deviceService, nil))
	return &fixture{t: t, ctx: context.Background(), db: db, e: e, h: h, password: string(hash)}
}

// exec runs SQL as a test step, the way any write path would.
func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	_, err := f.db.ExecContext(f.ctx, query, args...)
	require.NoError(f.t, err, query)
}

// addUser adds an active user, "reader", with the admin role (every permission) and
// access to libraryIDs, or to every Library when there are none.
func (f *fixture) addUser(libraryIDs ...int) int {
	f.t.Helper()
	var id int
	require.NoError(f.t, f.db.QueryRowContext(f.ctx,
		`INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, 1, TRUE) RETURNING id`,
		"reader", f.password).Scan(&id))
	if len(libraryIDs) == 0 {
		f.exec(`INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)`, id)
	}
	for _, libraryID := range libraryIDs {
		f.exec(`INSERT INTO user_library_access (user_id, library_id) VALUES (?, ?)`, id, libraryID)
	}
	return id
}

func (f *fixture) addLibrary(id int, name string) {
	f.t.Helper()
	f.exec(`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (?, ?, 'book')`, id, name)
}

func (f *fixture) addBook(id, libraryID int, title string) {
	f.t.Helper()
	f.exec(`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
		VALUES (?, ?, ?, ?, 'file', ?, 'file')`, id, libraryID, fmt.Sprintf("/library/%d", id), title, title)
}

type call struct {
	method     string
	path       string
	body       string
	credential string
	protocol   string
	version    string
	instance   string
}

func (f *fixture) do(r call) *httptest.ResponseRecorder {
	f.t.Helper()
	method := r.method
	if method == "" {
		method = http.MethodGet
	}
	req := httptest.NewRequest(method, r.path, strings.NewReader(r.body))
	if r.body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	switch r.protocol {
	case "":
		req.Header.Set(HeaderAppProtocol, strconv.Itoa(Protocol))
	case "none":
	default:
		req.Header.Set(HeaderAppProtocol, r.protocol)
	}
	if r.version != "" {
		req.Header.Set(HeaderAppVersion, r.version)
	}
	if r.credential != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+r.credential)
	}
	if r.instance != "" {
		req.Header.Set(HeaderInstance, r.instance)
	}
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	return rec
}

func signInBody(username, password string) string {
	b, _ := json.Marshal(SignInPayload{
		Username: username, Password: password, InstallationID: "install-1", DeviceName: "iPhone", AppVersion: "1.0.0 (1)",
	})
	return string(b)
}

// signIn signs the user in and returns the credential.
func (f *fixture) signIn() string {
	f.t.Helper()
	rec := f.do(call{method: http.MethodPost, path: "/mobile/sign-in", body: signInBody("reader", testPassword)})
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp SignInResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Credential
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body errcodes.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	return body.Error.Code
}

// changes reads one page of the feed after cursor.
func (f *fixture) changes(credential string, cursor int64, limit int) FeedResponse {
	f.t.Helper()
	return f.changesSeen(credential, cursor, 0, limit)
}

// changesSeen asks for the changes after cursor from an installation that
// last saw deletions expired through expiredThrough.
func (f *fixture) changesSeen(credential string, cursor, expiredThrough int64, limit int) FeedResponse {
	f.t.Helper()
	path := fmt.Sprintf("/mobile/changes?cursor=%d&expired_through=%d", cursor, expiredThrough)
	if limit > 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}
	rec := f.do(call{path: path, credential: credential})
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp FeedResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// summary names each entry, as "kind:id", with "-" for a deletion.
func summary(resp FeedResponse) []string {
	out := []string{}
	for _, entry := range resp.Entries {
		s := entry.Kind + ":" + strconv.Itoa(entry.ID)
		if entry.Deleted {
			s += "-"
		}
		out = append(out, s)
	}
	return out
}

func TestSignIn_CreatesAnAppDevice(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	userID := f.addUser()

	rec := f.do(call{method: http.MethodPost, path: "/mobile/sign-in", body: signInBody("Reader", testPassword)})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp SignInResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Credential)
	assert.Equal(t, "iPhone", resp.DeviceName)
	assert.Equal(t, AppUser{ID: userID, Username: "reader"}, resp.User)
	assert.Equal(t, "v0.10.0", resp.ServerVersion)
	assert.Equal(t, 1, resp.Server.ProtocolMin)
	assert.Equal(t, 1, resp.Server.ProtocolMax)
	assert.Equal(t, testInstanceID, resp.Server.InstanceID)

	device, err := f.h.devices.Retrieve(f.ctx, userID, resp.DeviceID)
	require.NoError(t, err)
	assert.Equal(t, devices.DeviceTypeShishoApp, device.DeviceType)
	assert.Equal(t, "install-1", *device.InstallationID)
	assert.Equal(t, "1.0.0 (1)", *device.AppVersion)

	// The credential signs in, and reports the app's version as it changes.
	rec = f.do(call{path: "/mobile/changes?cursor=0", credential: resp.Credential, version: "1.1.0 (2)"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	device, err = f.h.devices.Retrieve(f.ctx, userID, resp.DeviceID)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0 (2)", *device.AppVersion)
}

func TestSignIn_WrongPasswordIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addUser()

	rec := f.do(call{method: http.MethodPost, path: "/mobile/sign-in", body: signInBody("reader", "wrong")})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	count, err := f.db.NewSelect().Model((*devices.Device)(nil)).Count(f.ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestSignIn_ForcedPasswordResetIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addUser()
	f.exec(`UPDATE users SET must_change_password = TRUE`)

	rec := f.do(call{method: http.MethodPost, path: "/mobile/sign-in", body: signInBody("reader", testPassword)})
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

func TestCredential_RevokedOrDeactivatedIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	userID := f.addUser()

	revoked := f.signIn()
	require.NoError(t, f.h.devices.Revoke(f.ctx, deviceIDFor(t, f, revoked)))
	rec := f.do(call{path: "/mobile/changes?cursor=0", credential: revoked})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	deactivated := f.signIn()
	f.exec(`UPDATE users SET is_active = FALSE WHERE id = ?`, userID)
	rec = f.do(call{path: "/mobile/changes?cursor=0", credential: deactivated})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestCredential_SurvivesAPasswordChange(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addUser()
	credential := f.signIn()

	f.exec(`UPDATE users SET password_hash = 'changed'`)
	rec := f.do(call{path: "/mobile/changes?cursor=0", credential: credential})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSignOut_EndsTheCredential(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addUser()
	credential := f.signIn()

	// An app too old for the server can still sign out.
	rec := f.do(call{method: http.MethodDelete, path: "/mobile/device", credential: credential, protocol: "none"})
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = f.do(call{path: "/mobile/changes?cursor=0", credential: credential})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func deviceIDFor(t *testing.T, f *fixture, credential string) string {
	t.Helper()
	device, err := f.h.devices.GetByCredential(f.ctx, credential)
	require.NoError(t, err)
	require.NotNil(t, device)
	return device.ID
}

func TestProtocol_OutsideTheRangeIs426(t *testing.T) {
	t.Parallel()

	t.Run("app too old", func(t *testing.T) {
		t.Parallel()
		f := newFixtureWith(t, newContract(2, 3))
		f.addUser()
		rec := f.do(call{method: http.MethodPost, path: "/mobile/sign-in", body: signInBody("reader", testPassword), protocol: "1"})
		require.Equal(t, http.StatusUpgradeRequired, rec.Code)
		assert.Equal(t, "app_update_required", errorCode(t, rec))
		assert.Equal(t, "2", rec.Header().Get(HeaderProtocolMin))
		assert.Equal(t, "3", rec.Header().Get(HeaderProtocolMax))
		assert.Equal(t, "v0.10.0", rec.Header().Get(HeaderServerVersion))
	})

	t.Run("server too old", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.addUser()
		credential := f.signIn()
		rec := f.do(call{path: "/mobile/changes?cursor=0", credential: credential, protocol: "2"})
		require.Equal(t, http.StatusUpgradeRequired, rec.Code)
		assert.Equal(t, "server_update_required", errorCode(t, rec))
		assert.Equal(t, "v0.10.0", rec.Header().Get(HeaderServerVersion))
	})

	t.Run("a wrong password learns nothing", func(t *testing.T) {
		t.Parallel()
		f := newFixtureWith(t, newContract(2, 2))
		f.addUser()
		rec := f.do(call{method: http.MethodPost, path: "/mobile/sign-in", body: signInBody("reader", "wrong"), protocol: "1"})
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Empty(t, rec.Header().Get(HeaderServerVersion))
	})

	t.Run("server info answers any protocol", func(t *testing.T) {
		t.Parallel()
		f := newFixtureWith(t, newContract(2, 2))
		rec := f.do(call{path: "/mobile/server", protocol: "1"})
		require.Equal(t, http.StatusOK, rec.Code)
		var info ServerInfo
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
		assert.Equal(t, 2, info.ProtocolMin)
		assert.Equal(t, 2, info.ProtocolMax)
		assert.Empty(t, info.Capabilities)
		assert.Equal(t, testInstanceID, info.InstanceID)
		assert.Empty(t, rec.Header().Get(HeaderServerVersion))
	})
}

func TestNewContract_ReadsTheDevRange(t *testing.T) {
	t.Parallel()
	contract, err := NewContract("")
	require.NoError(t, err)
	assert.Equal(t, []int{ProtocolMin, Protocol}, []int{contract.Min, contract.Max})
	contract, err = NewContract("0-0")
	require.NoError(t, err)
	assert.False(t, contract.accepts(Protocol), "a server too old for the app")
	for _, bad := range []string{"2", "3-2", "-1-2", "a-b"} {
		_, err := NewContract(bad)
		assert.Error(t, err, bad)
	}
}

func TestCapabilities_DependOnTheRequestsProtocol(t *testing.T) {
	t.Parallel()
	contract := newContract(1, 2)
	contract.capabilities = map[int][]string{1: {"old_feature", "folded_in"}, 2: {"new_feature"}}
	f := newFixtureWith(t, contract)
	f.addUser()
	credential := f.signIn()

	for _, tc := range []struct {
		protocol, want string
	}{
		{"1", "old_feature,folded_in"},
		{"2", "new_feature"},
		{"3", ""},
		{"none", ""},
	} {
		// Every response carries the list: a success, a refusal, and an
		// /api route this test doesn't even register.
		rec := f.do(call{path: "/mobile/changes?cursor=0", credential: credential, protocol: tc.protocol})
		assert.Equal(t, tc.want, rec.Header().Get(HeaderCapabilities), "changes, protocol %s", tc.protocol)
		assert.Contains(t, rec.Header(), HeaderCapabilities)
		rec = f.do(call{path: "/mobile/changes?cursor=0", credential: "wrong", protocol: tc.protocol})
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, tc.want, rec.Header().Get(HeaderCapabilities), "refusal, protocol %s", tc.protocol)
		rec = f.do(call{path: "/api/books", protocol: tc.protocol})
		assert.Contains(t, rec.Header(), HeaderCapabilities)
		assert.Equal(t, tc.want, rec.Header().Get(HeaderCapabilities), "/api, protocol %s", tc.protocol)
	}

	// Other paths don't carry it.
	rec := f.do(call{path: "/opds/v1"})
	assert.NotContains(t, rec.Header(), HeaderCapabilities)
}

func TestFeed_CreatesUpdatesAndDeletesAfterACursor(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	f.addBook(1, 1, "Dune")
	f.addUser()
	credential := f.signIn()

	first := f.changes(credential, 0, 0)
	assert.Equal(t, []string{"library:1", "book:1"}, summary(first))
	assert.False(t, first.HasMore)
	require.NotNil(t, first.Entries[0].Library)
	assert.Equal(t, "Fiction", first.Entries[0].Library.Name)
	require.NotNil(t, first.Entries[1].Book)
	assert.Equal(t, "Dune", first.Entries[1].Book.Title)

	// Nothing new.
	assert.Empty(t, f.changes(credential, first.Cursor, 0).Entries)

	f.addBook(2, 1, "Emma")
	f.exec(`UPDATE books SET title = 'Dune Messiah', sort_title = 'Dune Messiah' WHERE id = 1`)
	second := f.changes(credential, first.Cursor, 0)
	assert.Equal(t, []string{"book:2", "book:1"}, summary(second))
	assert.Equal(t, "Dune Messiah", second.Entries[1].Book.Title)

	f.exec(`DELETE FROM books WHERE id = 2`)
	third := f.changes(credential, second.Cursor, 0)
	assert.Equal(t, []string{"book:2-"}, summary(third))
	assert.Nil(t, third.Entries[0].Book)
	assert.Equal(t, 1, third.Entries[0].LibraryID)

	// From the start, a record appears once, as it is now.
	assert.Equal(t, []string{"library:1", "book:1", "book:2-"}, summary(f.changes(credential, 0, 0)))
}

func TestFeed_BookDocument(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	f.addBook(1, 1, "Dune")
	f.exec(`UPDATE books SET subtitle = 'A Novel', created_at = '2025-03-04 05:06:07' WHERE id = 1`)
	f.exec(`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES (1, 1, 'Frank Herbert', 'Herbert, Frank', 'file'), (2, 1, 'Ann Artist', 'Artist, Ann', 'file')`)
	f.exec(`INSERT INTO authors (book_id, person_id, sort_order, role) VALUES (1, 2, 2, 'penciller'), (1, 1, 1, NULL)`)
	f.exec(`INSERT INTO series (id, library_id, name, name_source, sort_name, sort_name_source) VALUES (1, 1, 'Dune Chronicles', 'file', 'Dune Chronicles', 'file')`)
	f.exec(`INSERT INTO book_series (book_id, series_id, series_number, sort_order) VALUES (1, 1, 1, 1)`)
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, cover_image_filename, updated_at,
		release_date, page_count, audiobook_duration_seconds, created_at)
		VALUES (7, 1, 1, '/library/1/dune.epub', 'epub', 12345, 'dune.epub.cover.jpg', '2026-01-02 03:04:05',
		'1965-08-01 00:00:00', 412, 1.5, '2025-01-01 00:00:00')`)
	f.addUser()

	resp := f.changes(f.signIn(), 0, 0)
	book := resp.Entries[len(resp.Entries)-1].Book
	require.NotNil(t, book)
	subtitle := "A Novel"
	one := 1.0
	penciller := "penciller"
	released := time.Date(1965, 8, 1, 0, 0, 0, 0, time.UTC)
	pages := 412
	duration := 1.5
	assert.Equal(t, &FeedBook{
		ID: 1, LibraryID: 1, Title: "Dune", Subtitle: &subtitle, SortTitle: "Dune",
		CreatedAt: time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC),
		Authors: []FeedAuthor{
			{PersonID: 1, Name: "Frank Herbert", SortName: "Herbert, Frank"},
			{PersonID: 2, Name: "Ann Artist", SortName: "Artist, Ann", Role: &penciller},
		},
		Series: []FeedSeriesMembership{{SeriesID: 1, Name: "Dune Chronicles", SortName: "Dune Chronicles", Number: &one}},
		Genres: []FeedGenreRef{},
		Tags:   []FeedTagRef{},
		Files: []FeedFile{{
			ID: 7, FileType: "epub", FileRole: "main", SizeBytes: 12345, DisplayName: "dune.epub",
			ReleaseDate: &released, PageCount: &pages, AudiobookDurationSeconds: &duration,
			CoverVersion: fmt.Sprintf("7-%d", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC).Unix()),
			Narrators:    []FeedNarrator{},
			Chapters:     []FeedChapter{},
			Identifiers:  []FeedIdentifier{},
			Pages:        []string{},
			Filepath:     "/library/1/dune.epub",
			CreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		}},
		// covers.CacheKey: the File's id and its updated_at.
		CoverVersion: fmt.Sprintf("7-%d", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC).Unix()),
	}, book)
}

// A change to a join table, or to a row a Book names, moves the Book.
func TestFeed_JoinTableChangesMoveTheBook(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	f.addBook(1, 1, "Dune")
	f.addBook(2, 1, "Emma")
	f.exec(`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES (1, 1, 'Frank Herbert', 'Herbert, Frank', 'file'), (2, 1, 'Jane Austen', 'Austen, Jane', 'file')`)
	f.exec(`INSERT INTO series (id, library_id, name, name_source, sort_name, sort_name_source) VALUES (1, 1, 'Dune Chronicles', 'file', 'Dune Chronicles', 'file')`)
	userID := f.addUser()
	credential := f.signIn()
	saveSort := fmt.Sprintf(`INSERT INTO user_library_settings (user_id, library_id, sort_spec) VALUES (%d, 1, 'title:asc')`, userID)

	steps := []struct {
		name  string
		write string
		want  []string
	}{
		{"date added", `UPDATE books SET created_at = '2020-01-01 00:00:00' WHERE id = 1`, []string{"book:1"}},
		{"author added", `INSERT INTO authors (id, book_id, person_id, sort_order) VALUES (1, 1, 1, 1)`, []string{"book:1"}},
		{"author reordered", `UPDATE authors SET sort_order = 2 WHERE id = 1`, []string{"book:1"}},
		{"person renamed", `UPDATE persons SET name = 'F. Herbert' WHERE id = 1`, []string{"book:1", "person:1"}},
		{"person deleted", `DELETE FROM persons WHERE id = 1`, []string{"book:1", "person:1-"}},
		{"series added", `INSERT INTO book_series (id, book_id, series_id, series_number, sort_order) VALUES (1, 2, 1, 1, 1)`, []string{"book:2"}},
		{"series number", `UPDATE book_series SET series_number = 2 WHERE id = 1`, []string{"book:2"}},
		{"series renamed", `UPDATE series SET name = 'Dune Saga' WHERE id = 1`, []string{"book:2", "series:1"}},
		{"membership moved", `UPDATE book_series SET book_id = 1 WHERE id = 1`, []string{"book:1", "book:2"}},
		{"series removed", `DELETE FROM book_series WHERE id = 1`, []string{"book:1"}},
		{"file added", `INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (1, 1, 1, '/f.epub', 'epub', 1)`, []string{"book:1"}},
		{"file resized", `UPDATE files SET filesize_bytes = 2 WHERE id = 1`, []string{"book:1"}},
		{"file named", `UPDATE files SET name = 'Special Edition', name_source = 'manual' WHERE id = 1`, []string{"book:1"}},
		{"file moved on disk", `UPDATE files SET filepath = '/g.epub' WHERE id = 1`, []string{"book:1"}},
		{"release date", `UPDATE files SET release_date = '2001-01-01 00:00:00' WHERE id = 1`, []string{"book:1"}},
		{"page count", `UPDATE files SET page_count = 10 WHERE id = 1`, []string{"book:1"}},
		{"duration", `UPDATE files SET audiobook_duration_seconds = 60 WHERE id = 1`, []string{"book:1"}},
		{"cover changed", `UPDATE files SET updated_at = '2030-01-01 00:00:00' WHERE id = 1`, []string{"book:1"}},
		{"file moved", `UPDATE files SET book_id = 2 WHERE id = 1`, []string{"book:1", "book:2"}},
		{"file deleted", `DELETE FROM files WHERE id = 1`, []string{"book:2"}},
		{"cover aspect", `UPDATE libraries SET cover_aspect_ratio = 'audiobook' WHERE id = 1`, []string{"library:1", "book:1", "book:2"}},
		{"sort saved", saveSort, []string{"library:1"}},
		{"sort changed", `UPDATE user_library_settings SET sort_spec = 'author:desc'`, []string{"library:1"}},
		{"sort cleared", `DELETE FROM user_library_settings`, []string{"library:1"}},
	}
	cursor := f.changes(credential, 0, 0).Cursor
	for _, step := range steps {
		f.exec(step.write)
		resp := f.changes(credential, cursor, 0)
		assert.ElementsMatch(t, step.want, summary(resp), step.name)
		cursor = resp.Cursor
	}
}

func TestFeed_WritesThatChangeNothingRecordNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES (1, 1, 'Frank Herbert', 'Herbert, Frank', 'file')`)
	f.exec(`INSERT INTO authors (id, book_id, person_id, sort_order) VALUES (1, 1, 1, 1)`)
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (1, 1, 1, '/f.epub', 'epub', 1)`)
	f.addUser()
	credential := f.signIn()
	cursor := f.changes(credential, 0, 0).Cursor

	for _, write := range []string{
		`UPDATE books SET title = title, sort_title = sort_title, subtitle = subtitle`,
		// Columns the app doesn't see.
		`UPDATE books SET description_source = 'manual', updated_at = '2030-01-01 00:00:00'`,
		`UPDATE files SET filesize_bytes = 1, scan_error = 'x', reviewed = TRUE`,
		`UPDATE authors SET sort_order = 1`,
		`UPDATE persons SET name = 'Frank Herbert', updated_at = '2030-01-01 00:00:00'`,
		`UPDATE libraries SET name = 'Fiction', organize_file_structure = FALSE`,
		`UPDATE files SET release_date = release_date, page_count = page_count, audiobook_duration_seconds = audiobook_duration_seconds`,
		`UPDATE files SET name = name, name_source = name_source, filepath = filepath`,
	} {
		f.exec(write)
		assert.Empty(t, f.changes(credential, cursor, 0).Entries, write)
	}
}

// A Library's document carries the reading user's own saved sort for it,
// so the app's Library is in the order the web app shows them.
func TestFeed_LibraryDocumentCarriesTheUsersSort(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	f.addLibrary(2, "Comics")
	f.addLibrary(3, "Audiobooks")
	userID := f.addUser()
	var otherID int
	require.NoError(t, f.db.QueryRowContext(f.ctx,
		`INSERT INTO users (username, password_hash, role_id, is_active) VALUES ('other', 'x', 1, TRUE) RETURNING id`).Scan(&otherID))
	f.exec(`INSERT INTO user_library_settings (user_id, library_id, sort_spec) VALUES (?, 1, 'author:asc,date_added:desc'), (?, 1, 'title:desc'), (?, 2, 'not a spec')`,
		userID, otherID, userID)

	sorts := map[int]string{}
	for _, entry := range f.changes(f.signIn(), 0, 0).Entries {
		require.NotNil(t, entry.Library)
		sorts[entry.ID] = entry.Library.SortSpec
	}
	// Library 2's spec doesn't parse, and Library 3 has none: both are the
	// default, which is empty.
	assert.Equal(t, map[int]string{1: "author:asc,date_added:desc", 2: "", 3: ""}, sorts)
}

// Deleting a Library deletes its saved sorts too, and that mustn't hide
// the Library's deletion.
func TestFeed_DeletedLibraryWithASavedSortIsDeleted(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	f.addLibrary(2, "Comics")
	userID := f.addUser()
	f.exec(`INSERT INTO user_library_settings (user_id, library_id, sort_spec) VALUES (?, 1, 'title:asc')`, userID)
	credential := f.signIn()
	cursor := f.changes(credential, 0, 0).Cursor

	f.exec(`DELETE FROM libraries WHERE id = 1`)
	assert.Equal(t, []string{"library:1-"}, summary(f.changes(credential, cursor, 0)))
}

func TestFeed_FiltersByLibraryAccess(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Mine")
	f.addLibrary(2, "Theirs")
	f.addBook(1, 1, "Dune")
	f.addBook(2, 2, "Emma")
	f.addUser(1)
	credential := f.signIn()

	first := f.changes(credential, 0, 0)
	assert.Equal(t, []string{"library:1", "book:1"}, summary(first))

	// Deletions in a Library the user can't access stay hidden.
	f.exec(`DELETE FROM books WHERE id = 2`)
	assert.Empty(t, f.changes(credential, first.Cursor, 0).Entries)

	// A Library deleted outright takes its access with it, so its
	// deletions reach everyone, carrying only ids.
	f.addBook(3, 1, "Persuasion")
	f.exec(`DELETE FROM libraries WHERE id = 1`)
	resp := f.changes(credential, first.Cursor, 0)
	assert.ElementsMatch(t, []string{"book:1-", "book:3-", "library:1-"}, summary(resp))
}

func TestFeed_PagesAndSkipsHiddenEntries(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Mine")
	f.addLibrary(2, "Theirs")
	for id := 1; id <= 5; id++ {
		f.addBook(id, 1, fmt.Sprintf("Book %d", id))
	}
	f.addBook(6, 2, "Hidden")
	f.addUser(1)
	credential := f.signIn()

	var seen []string
	cursor := int64(0)
	for {
		page := f.changes(credential, cursor, 2)
		seen = append(seen, summary(page)...)
		require.Greater(t, page.Cursor, cursor)
		cursor = page.Cursor
		if !page.HasMore {
			break
		}
	}
	assert.Equal(t, []string{"library:1", "book:1", "book:2", "book:3", "book:4", "book:5"}, seen)

	// The last page moved the cursor past the hidden entries at the end.
	var last int64
	require.NoError(t, f.db.QueryRowContext(f.ctx, `SELECT MAX(seq) FROM change_log_entries`).Scan(&last))
	assert.Equal(t, last, cursor)
}

func TestFeed_ExpiredCursorStartsOver(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	f.addBook(1, 1, "Dune")
	f.addBook(2, 1, "Emma")
	f.addUser()
	credential := f.signIn()
	cursor := f.changes(credential, 0, 0).Cursor

	f.exec(`DELETE FROM books WHERE id = 1`)
	// The deletion is past its lifetime, and the last expiry was over an
	// hour ago.
	f.exec(`UPDATE change_log_entries SET changed_at = '2000-01-01 00:00:00' WHERE deleted`)
	f.exec(`UPDATE change_log_expiries SET expired_at = '2000-01-01 00:00:00'`)

	resp := f.changes(credential, cursor, 0)
	assert.True(t, resp.StartOver)
	assert.Empty(t, resp.Entries)

	fresh := f.changes(credential, 0, 0)
	assert.False(t, fresh.StartOver)
	assert.Equal(t, []string{"library:1", "book:2"}, summary(fresh))
}

// A download from the start finishes even when its cursors fall below
// deletions that had already expired when it began: it never had those
// records, so it missed nothing. The cursor it ends on syncs on later.
func TestFeed_DownloadAcrossExpiredDeletionsFinishes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	for id := 1; id <= 4; id++ {
		f.addBook(id, 1, fmt.Sprintf("Book %d", id))
	}
	f.addBook(5, 1, "Gone")
	f.exec(`DELETE FROM books WHERE id = 5`)
	f.exec(`UPDATE change_log_entries SET changed_at = '2000-01-01 00:00:00' WHERE deleted`)
	f.addUser()
	credential := f.signIn()

	var got []string
	resp := f.changesSeen(credential, 0, 0, 2)
	for {
		require.False(t, resp.StartOver, "after %v", got)
		got = append(got, summary(resp)...)
		if !resp.HasMore {
			break
		}
		resp = f.changesSeen(credential, resp.Cursor, resp.ExpiredThrough, 2)
	}
	assert.Equal(t, []string{"library:1", "book:1", "book:2", "book:3", "book:4"}, got)

	next := f.changesSeen(credential, resp.Cursor, resp.ExpiredThrough, 0)
	assert.False(t, next.StartOver)
	assert.Empty(t, next.Entries)
}

// A cursor past anything this server's log handed out came from another
// database, such as the one before a restore from backup, so the app's copy
// can't be trusted and starts over.
func TestFeed_CursorAheadOfTheLogStartsOver(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Fiction")
	f.addBook(1, 1, "Dune")
	f.addUser()
	credential := f.signIn()
	cursor := f.changes(credential, 0, 0).Cursor

	assert.False(t, f.changes(credential, cursor, 0).StartOver, "the newest cursor is fine")
	// Expiry deleting the newest entries doesn't make a cursor look ahead.
	f.exec(`DELETE FROM books WHERE id = 1`)
	cursor = f.changes(credential, cursor, 0).Cursor
	f.exec(`DELETE FROM change_log_entries`)
	assert.False(t, f.changes(credential, cursor, 0).StartOver)

	resp := f.changes(credential, cursor+100, 0)
	assert.True(t, resp.StartOver)
	assert.Empty(t, resp.Entries)
}

func TestCover_ServesAccessibleBooksOnly(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.epub.cover.png"), tinyPNG(t), 0o600))
	f.addLibrary(1, "Mine")
	f.addLibrary(2, "Theirs")
	f.addBook(1, 1, "Dune")
	f.addBook(2, 2, "Emma")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, cover_image_filename) VALUES
		(1, 1, 1, ?, 'epub', 1, 'a.epub.cover.png'), (2, 2, 2, ?, 'epub', 1, 'a.epub.cover.png')`,
		filepath.Join(dir, "a.epub"), filepath.Join(dir, "a.epub"))
	f.addUser(1)
	credential := f.signIn()

	rec := f.do(call{path: "/mobile/books/1/cover?v=1-1", credential: credential})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "private, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))

	rec = f.do(call{path: "/mobile/books/2/cover?v=1-1", credential: credential})
	assert.Equal(t, http.StatusNotFound, rec.Code)
	rec = f.do(call{path: "/mobile/books/9/cover", credential: credential})
	assert.Equal(t, http.StatusNotFound, rec.Code)
	rec = f.do(call{path: "/mobile/books/1/cover?v=1-1"})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// The feed sends the kinds the triggers write.
func TestFeedKinds_MatchTheChangeLog(t *testing.T) {
	t.Parallel()
	assert.Equal(t, changefeed.KindBook, FeedKindBook)
	assert.Equal(t, changefeed.KindLibrary, FeedKindLibrary)
	assert.Equal(t, changefeed.KindReadingPosition, FeedKindReadingPosition)
	assert.Equal(t, changefeed.KindReadThrough, FeedKindReadThrough)
	assert.Equal(t, changefeed.KindSeries, FeedKindSeries)
	assert.Equal(t, changefeed.KindPerson, FeedKindPerson)
	assert.Equal(t, changefeed.KindGenre, FeedKindGenre)
	assert.Equal(t, changefeed.KindTag, FeedKindTag)
	assert.Equal(t, changefeed.KindPublisher, FeedKindPublisher)
	assert.Equal(t, changefeed.KindBookLeft, FeedKindBookLeft)
	assert.Equal(t, changefeed.KindUserBook, FeedKindUserBook)
	assert.Equal(t, changefeed.KindList, FeedKindList)
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 3))))
	return buf.Bytes()
}
