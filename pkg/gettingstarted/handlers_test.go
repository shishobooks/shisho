package gettingstarted

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

var allPermissions = []string{
	"libraries:read", "libraries:write", "books:read", "books:write",
	"users:read", "users:write", "config:read", "config:write",
}

var roleCounter atomic.Int64

// newUser creates an active user whose role holds exactly perms and loads it
// the way authentication does, with its role's permissions.
func newUser(t *testing.T, db *bun.DB, perms ...string) *models.User {
	t.Helper()
	ctx := context.Background()
	n := roleCounter.Add(1)
	role := &models.Role{Name: fmt.Sprintf("role-%d", n), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err := db.NewInsert().Model(role).Exec(ctx)
	require.NoError(t, err)
	for _, perm := range perms {
		resource, operation, _ := strings.Cut(perm, ":")
		_, err := db.NewInsert().Model(&models.Permission{RoleID: role.ID, Resource: resource, Operation: operation}).Exec(ctx)
		require.NoError(t, err)
	}
	user := &models.User{
		Username: fmt.Sprintf("user-%d", n), PasswordHash: "x", RoleID: role.ID, IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(), GettingStartedEnabled: true,
	}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	return loadUser(t, db, user.ID)
}

func loadUser(t *testing.T, db *bun.DB, id int) *models.User {
	t.Helper()
	user, err := auth.LoadUser(context.Background(), db, auth.LoadUserOptions{ID: &id, IncludeInactive: true})
	require.NoError(t, err)
	return user
}

func do(t *testing.T, fn echo.HandlerFunc, user *models.User, params map[string]string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if user != nil {
		auth.SetUser(c, user)
	}
	names := make([]string, 0, len(params))
	values := make([]string, 0, len(params))
	for name, value := range params {
		names = append(names, name)
		values = append(values, value)
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	return rec, fn(c)
}

func status(t *testing.T, h *handler, user *models.User) GettingStartedResponse {
	t.Helper()
	rec, err := do(t, h.retrieve, user, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	return decode(t, rec)
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) GettingStartedResponse {
	t.Helper()
	var resp GettingStartedResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func step(t *testing.T, resp GettingStartedResponse, key string) GettingStartedStep {
	t.Helper()
	for _, s := range resp.Steps {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("step %q missing from %+v", key, resp.Steps)
	return GettingStartedStep{}
}

func insert(t *testing.T, db *bun.DB, model any) {
	t.Helper()
	_, err := db.NewInsert().Model(model).Exec(context.Background())
	require.NoError(t, err)
}

func insertLibrary(t *testing.T, db *bun.DB) *models.Library {
	t.Helper()
	library := &models.Library{Name: "Books", CoverAspectRatio: models.CoverAspectRatioBook}
	insert(t, db, library)
	return library
}

func insertBook(t *testing.T, db *bun.DB, library *models.Library, name string) *models.Book {
	t.Helper()
	book := &models.Book{
		LibraryID: library.ID, Title: name, TitleSource: models.DataSourceFilepath, SortTitle: name,
		SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/books/" + name,
	}
	insert(t, db, book)
	return book
}

func insertEnricher(t *testing.T, db *bun.DB, id, hookType string) {
	t.Helper()
	insert(t, db, &models.Plugin{Scope: "shisho", ID: id, Name: id, Version: "1.0.0", InstalledAt: time.Now()})
	insert(t, db, &models.PluginHookConfig{HookType: hookType, Scope: "shisho", PluginID: id, Mode: models.PluginModeEnabled})
}

func TestRetrieve_StepsApplyByPermission(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		perms   []string
		applies []string
	}{
		{"admin", allPermissions, []string{StepLibrary, StepInvite, StepDevice, StepToRead, StepStartReading}},
		{"viewer", []string{"books:read"}, []string{StepDevice, StepToRead, StepStartReading}},
		{"library manager without Books Read", []string{"libraries:read", "libraries:write"}, []string{StepLibrary}},
		{"user manager", []string{"users:read", "users:write"}, []string{StepInvite}},
		{"no permissions", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			resp := status(t, newHandler(db, false), newUser(t, db, tc.perms...))
			require.True(t, resp.Enabled)
			keys := make([]string, 0, len(resp.Steps))
			var applies []string
			for _, s := range resp.Steps {
				keys = append(keys, s.Key)
				if s.Applies {
					applies = append(applies, s.Key)
				}
				assert.False(t, s.Done, s.Key)
			}
			assert.Equal(t, []string{StepLibrary, StepPlugins, StepInvite, StepDevice, StepToRead, StepStartReading}, keys)
			assert.Equal(t, tc.applies, applies)
		})
	}
}

// "Add metadata plugins" needs Config Write, and applies only once a Library
// exists: before that, setting up the first Library covers plugins.
func TestRetrieve_PluginsStepAppliesOnceALibraryExists(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	admin := newUser(t, db, allPermissions...)
	pluginManager := newUser(t, db, "config:read", "config:write")
	libraryOnly := newUser(t, db, "libraries:read", "libraries:write")

	assert.False(t, step(t, status(t, h, admin), StepPlugins).Applies)
	insertLibrary(t, db)
	assert.True(t, step(t, status(t, h, admin), StepPlugins).Applies)
	assert.True(t, step(t, status(t, h, pluginManager), StepPlugins).Applies)
	assert.False(t, step(t, status(t, h, libraryOnly), StepPlugins).Applies)
}

func TestRetrieve_AdminStepsAreDoneWhenTheirDataExists(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	admin := newUser(t, db, allPermissions...)

	resp := status(t, h, admin)
	assert.False(t, step(t, resp, StepLibrary).Done)
	assert.False(t, step(t, resp, StepInvite).Done)

	insertLibrary(t, db)
	assert.True(t, step(t, status(t, h, admin), StepLibrary).Done)

	// A plugin with only other hooks doesn't count; one declaring a
	// metadata enricher does, whatever its mode.
	insertEnricher(t, db, "converter", models.PluginHookInputConverter)
	assert.False(t, step(t, status(t, h, admin), StepPlugins).Done)
	insertEnricher(t, db, "goodreads", models.PluginHookMetadataEnricher)
	assert.True(t, step(t, status(t, h, admin), StepPlugins).Done)

	// A deactivated user isn't someone the admin invited in.
	other := newUser(t, db, "books:read")
	_, err := db.NewUpdate().Model((*models.User)(nil)).Set("is_active = FALSE").Where("id = ?", other.ID).Exec(context.Background())
	require.NoError(t, err)
	assert.False(t, step(t, status(t, h, admin), StepInvite).Done)
	newUser(t, db, "books:read")
	assert.True(t, step(t, status(t, h, admin), StepInvite).Done)
}

func TestRetrieve_PersonalStepsAreDoneFromTheUsersOwnData(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	h := newHandler(db, false)
	reader := newUser(t, db, "books:read")
	other := newUser(t, db, "books:read")
	library := insertLibrary(t, db)
	book := insertBook(t, db, library, "one")

	// Another user's Device, To Read, and reading don't count.
	_, err := devices.NewService(db).Create(ctx, other.ID, "Kobo", devices.DeviceTypeKobo)
	require.NoError(t, err)
	position := 1
	insert(t, db, &models.UserBook{UserID: other.ID, BookID: book.ID, ToReadPosition: &position, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	insert(t, db, &models.ReadThrough{UserID: other.ID, BookID: &book.ID, Outcome: models.ReadThroughOutcomeUnderway, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	file := &models.File{LibraryID: library.ID, BookID: book.ID, Filepath: "/books/one/one.epub", FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain}
	insert(t, db, file)
	insert(t, db, &models.ReadingPosition{
		UserID: other.ID, FileID: file.ID, Percentage: 0.1, Source: models.ReadingSourceWebReader,
		OccurredAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	resp := status(t, h, reader)
	assert.False(t, step(t, resp, StepDevice).Done)
	assert.False(t, step(t, resp, StepToRead).Done)
	assert.False(t, step(t, resp, StepStartReading).Done)

	_, err = devices.NewService(db).Create(ctx, reader.ID, "Kobo", devices.DeviceTypeKobo)
	require.NoError(t, err)
	assert.True(t, step(t, status(t, h, reader), StepDevice).Done)

	// A Rating alone puts nothing on To Read.
	rating := 10
	insert(t, db, &models.UserBook{UserID: reader.ID, BookID: book.ID, Rating: &rating, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	assert.False(t, step(t, status(t, h, reader), StepToRead).Done)
	_, err = db.NewUpdate().Model((*models.UserBook)(nil)).Set("to_read_position = 1").
		Where("user_id = ? AND book_id = ?", reader.ID, book.ID).Exec(ctx)
	require.NoError(t, err)
	assert.True(t, step(t, status(t, h, reader), StepToRead).Done)
}

func TestRetrieve_StartReadingIsDoneByAPositionOrARead(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	library := insertLibrary(t, db)
	book := insertBook(t, db, library, "one")
	file := &models.File{LibraryID: library.ID, BookID: book.ID, Filepath: "/books/one/one.epub", FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain}
	insert(t, db, file)

	positioned := newUser(t, db, "books:read")
	insert(t, db, &models.ReadingPosition{
		UserID: positioned.ID, FileID: file.ID, Percentage: 0.1, Source: models.ReadingSourceWebReader,
		OccurredAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	assert.True(t, step(t, status(t, h, positioned), StepStartReading).Done)

	// A Read-through counts even after its Book is gone.
	finished := newUser(t, db, "books:read")
	title := "Gone"
	insert(t, db, &models.ReadThrough{UserID: finished.ID, BookTitle: &title, Outcome: models.ReadThroughOutcomeFinished, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	assert.True(t, step(t, status(t, h, finished), StepStartReading).Done)
}

func TestRetrieve_ReportsTheUsersState(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	user := newUser(t, db, "books:read")
	_, err := db.NewUpdate().Model((*models.User)(nil)).
		Set("getting_started_enabled = FALSE").
		Set("getting_started_auto_open = TRUE").
		Set("getting_started_dismissed_at = ?", time.Now()).
		Where("id = ?", user.ID).Exec(context.Background())
	require.NoError(t, err)

	resp := status(t, h, user)
	assert.False(t, resp.Enabled)
	assert.True(t, resp.AutoOpen)
	assert.True(t, resp.Dismissed)
	assert.Empty(t, resp.Steps)
}

// While the list is off or dismissed nothing shows it, so no step is
// checked.
func TestRetrieve_HiddenListChecksNoSteps(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	off := newUser(t, db, allPermissions...)
	_, err := db.NewUpdate().Model((*models.User)(nil)).Set("getting_started_enabled = FALSE").Where("id = ?", off.ID).Exec(context.Background())
	require.NoError(t, err)
	dismissed := newUser(t, db, allPermissions...)
	_, err = do(t, h.dismiss, dismissed, nil)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE reading_positions`)
	require.NoError(t, err)

	for _, user := range []*models.User{off, dismissed} {
		rec, err := do(t, h.retrieve, user, nil)
		require.NoError(t, err)
		assert.Equal(t, []GettingStartedStep{}, decode(t, rec).Steps)
	}
}

func TestNotForMe_MarksAndUndoesAStep(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	user := newUser(t, db, allPermissions...)
	params := map[string]string{"step": StepInvite}

	for range 2 {
		rec, err := do(t, h.markNotForMe, user, params)
		require.NoError(t, err)
		assert.True(t, step(t, decode(t, rec), StepInvite).NotForMe)
	}
	_, err := do(t, h.markNotForMe, user, map[string]string{"step": StepDevice})
	require.NoError(t, err)
	resp := status(t, h, user)
	assert.True(t, step(t, resp, StepInvite).NotForMe)
	assert.True(t, step(t, resp, StepDevice).NotForMe)
	assert.False(t, step(t, resp, StepLibrary).NotForMe)
	assert.Equal(t, models.StringList{StepInvite, StepDevice}, loadUser(t, db, user.ID).GettingStartedNotForMe)

	rec, err := do(t, h.unmarkNotForMe, user, params)
	require.NoError(t, err)
	assert.False(t, step(t, decode(t, rec), StepInvite).NotForMe)
	assert.True(t, step(t, decode(t, rec), StepDevice).NotForMe)

	// Another user's choices are their own.
	assert.False(t, step(t, status(t, h, newUser(t, db, allPermissions...)), StepDevice).NotForMe)
}

func TestNotForMe_RefusesAnUnknownStep(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	user := newUser(t, db, "books:read")

	for _, fn := range []echo.HandlerFunc{h.markNotForMe, h.unmarkNotForMe} {
		_, err := do(t, fn, user, map[string]string{"step": "nope"})
		var codeErr *errcodes.Error
		require.ErrorAs(t, err, &codeErr)
		assert.Equal(t, http.StatusUnprocessableEntity, codeErr.HTTPCode)
	}
}

func TestDismissAndRestore(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	user := newUser(t, db, "books:read")
	_, err := do(t, h.markNotForMe, user, map[string]string{"step": StepDevice})
	require.NoError(t, err)

	rec, err := do(t, h.dismiss, user, nil)
	require.NoError(t, err)
	assert.True(t, decode(t, rec).Dismissed)
	assert.True(t, status(t, h, user).Dismissed)

	// Restoring keeps "Not for me" choices; only an admin's Show clears them.
	rec, err = do(t, h.restore, user, nil)
	require.NoError(t, err)
	resp := decode(t, rec)
	assert.False(t, resp.Dismissed)
	assert.True(t, resp.Enabled)
	assert.True(t, step(t, resp, StepDevice).NotForMe)
}

// A user it was never turned on for, such as one from before the upgrade,
// gets it by restoring it from the user menu.
func TestRestore_EnablesGettingStarted(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	user := newUser(t, db, "books:read")
	_, err := db.NewUpdate().Model((*models.User)(nil)).Set("getting_started_enabled = FALSE").Where("id = ?", user.ID).Exec(context.Background())
	require.NoError(t, err)
	require.False(t, status(t, h, user).Enabled)

	rec, err := do(t, h.restore, user, nil)
	require.NoError(t, err)
	assert.True(t, decode(t, rec).Enabled)
	assert.False(t, decode(t, rec).AutoOpen)
}

func TestClearAutoOpen(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	user := newUser(t, db, "books:read")
	_, err := db.NewUpdate().Model((*models.User)(nil)).Set("getting_started_auto_open = TRUE").Where("id = ?", user.ID).Exec(context.Background())
	require.NoError(t, err)
	require.True(t, status(t, h, user).AutoOpen)

	rec, err := do(t, h.clearAutoOpen, user, nil)
	require.NoError(t, err)
	assert.False(t, decode(t, rec).AutoOpen)
	assert.False(t, status(t, h, user).AutoOpen)
}

func TestShow_ResetsAnotherUsersListWithoutRevealingIt(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	h := newHandler(db, false)
	admin := newUser(t, db, allPermissions...)
	target := newUser(t, db, "books:read")
	_, err := devices.NewService(db).Create(ctx, target.ID, "Kobo", devices.DeviceTypeKobo)
	require.NoError(t, err)
	_, err = db.NewUpdate().Model((*models.User)(nil)).
		Set("getting_started_enabled = FALSE").
		Set("getting_started_dismissed_at = ?", time.Now()).
		Set("getting_started_not_for_me = ?", `["to_read"]`).
		Where("id = ?", target.ID).Exec(ctx)
	require.NoError(t, err)

	rec, err := do(t, h.show, admin, map[string]string{"id": strconv.Itoa(target.ID)})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())

	resp := status(t, h, loadUser(t, db, target.ID))
	assert.True(t, resp.Enabled)
	assert.True(t, resp.AutoOpen)
	assert.False(t, resp.Dismissed)
	assert.False(t, step(t, resp, StepToRead).NotForMe)
	assert.True(t, step(t, resp, StepDevice).Done)

	// The admin's own list is untouched.
	assert.False(t, status(t, h, admin).AutoOpen)
}

func TestShow_UnknownUserIsNotFound(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	admin := newUser(t, db, allPermissions...)

	for _, id := range []string{"999", "abc"} {
		_, err := do(t, h.show, admin, map[string]string{"id": id})
		var codeErr *errcodes.Error
		require.ErrorAs(t, err, &codeErr, id)
		assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode, id)
	}
}

func TestShow_StorageFailureIsAServerError(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	admin := newUser(t, db, allPermissions...)
	target := newUser(t, db, "books:read")
	_, err := db.Exec(`CREATE TRIGGER fail_user_update BEFORE UPDATE ON users BEGIN SELECT RAISE(ABORT, 'boom'); END`)
	require.NoError(t, err)

	_, err = do(t, h.show, admin, map[string]string{"id": strconv.Itoa(target.ID)})
	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr)
}

func TestRetrieve_StorageFailureIsAServerError(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)
	user := newUser(t, db, "books:read")
	_, err := db.Exec(`DROP TABLE reading_positions`)
	require.NoError(t, err)

	_, err = do(t, h.retrieve, user, nil)
	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr)
}

// Demo Mode reports Getting Started off and reads nothing, so the demo's
// shared accounts never see first steps.
func TestRetrieve_DemoModeReportsItDisabled(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, true)
	user := newUser(t, db, allPermissions...)
	_, err := db.NewUpdate().Model((*models.User)(nil)).Set("getting_started_auto_open = TRUE").Where("id = ?", user.ID).Exec(context.Background())
	require.NoError(t, err)

	resp := status(t, h, loadUser(t, db, user.ID))
	assert.Equal(t, GettingStartedResponse{Steps: []GettingStartedStep{}}, resp)
}

func TestHandlers_RequireASignedInUser(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	h := newHandler(db, false)

	for _, fn := range []echo.HandlerFunc{h.retrieve, h.dismiss, h.restore, h.clearAutoOpen} {
		_, err := do(t, fn, nil, nil)
		var codeErr *errcodes.Error
		require.ErrorAs(t, err, &codeErr)
		assert.Equal(t, http.StatusUnauthorized, codeErr.HTTPCode)
	}
}
