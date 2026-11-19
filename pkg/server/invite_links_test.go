package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/invitelinks"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// inviteLinksFixture serves the real route table, so the Users Write guard,
// the public routes, and the session cookie accept sets are exercised
// together.
type inviteLinksFixture struct {
	t       *testing.T
	db      *bun.DB
	handler http.Handler
	authSvc *auth.Service
	libA    *models.Library
	libB    *models.Library
	admin   *models.User
	viewer  *models.User
}

func newInviteLinksFixture(t *testing.T, demo bool) *inviteLinksFixture {
	t.Helper()
	ctx := context.Background()
	db := testdb.New(t)
	cfg := newPermissionTestConfig(t)
	cfg.DemoMode = demo
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil, nil, nil, nil), nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	f := &inviteLinksFixture{
		t:       t,
		db:      db,
		handler: srv.Handler,
		authSvc: auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration()),
	}
	f.libA = &models.Library{Name: "Library A", CoverAspectRatio: "book"}
	f.libB = &models.Library{Name: "Library B", CoverAspectRatio: "book"}
	for _, lib := range []*models.Library{f.libA, f.libB} {
		_, err := db.NewInsert().Model(lib).Exec(ctx)
		require.NoError(t, err)
	}
	f.admin = insertPermissionTestUser(ctx, t, db, "admin", models.RoleAdmin, nil)
	f.viewer = insertPermissionTestUser(ctx, t, db, "viewer", models.RoleViewer, &f.libA.ID)
	return f
}

func (f *inviteLinksFixture) do(user *models.User, method, path, body string) *httptest.ResponseRecorder {
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

func (f *inviteLinksFixture) roleID(name string) int {
	f.t.Helper()
	var id int
	require.NoError(f.t, f.db.NewRaw("SELECT id FROM roles WHERE name = ?", name).Scan(context.Background(), &id))
	return id
}

// mustCreate creates an invite as user from body and returns it.
func (f *inviteLinksFixture) mustCreate(user *models.User, body string) invitelinks.InviteLinkResponse {
	f.t.Helper()
	rec := f.do(user, http.MethodPost, "/api/invite-links", body)
	require.Equal(f.t, http.StatusCreated, rec.Code, rec.Body.String())
	var link invitelinks.InviteLinkResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &link))
	return link
}

func (f *inviteLinksFixture) viewerInvite(user *models.User) invitelinks.InviteLinkResponse {
	f.t.Helper()
	return f.mustCreate(user, fmt.Sprintf(`{"role_id":%d,"library_ids":[%d]}`, f.roleID(models.RoleViewer), f.libA.ID))
}

func (f *inviteLinksFixture) pending() []invitelinks.InviteLinkResponse {
	f.t.Helper()
	rec := f.do(f.admin, http.MethodGet, "/api/invite-links", "")
	require.Equal(f.t, http.StatusOK, rec.Code, rec.Body.String())
	var links []invitelinks.InviteLinkResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &links))
	return links
}

func (f *inviteLinksFixture) lookup(token string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(nil, http.MethodGet, "/api/invites/"+token, "")
}

func (f *inviteLinksFixture) accept(token, username string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(nil, http.MethodPost, "/api/invites/"+token+"/accept",
		fmt.Sprintf(`{"username":%q,"password":"password123","email":""}`, username))
}

func (f *inviteLinksFixture) setColumn(link invitelinks.InviteLinkResponse, column string, value any) {
	f.t.Helper()
	_, err := f.db.NewUpdate().Model((*models.InviteLink)(nil)).
		Set("? = ?", bun.Ident(column), value).
		Where("id = ?", link.ID).
		Exec(context.Background())
	require.NoError(f.t, err)
}

func TestInviteLinks_CreateListAndRevoke(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)

	before := time.Now()
	link := f.viewerInvite(f.admin)
	assert.Len(t, link.Token, 43)
	assert.Equal(t, f.roleID(models.RoleViewer), link.RoleID)
	require.NotNil(t, link.Role)
	assert.Equal(t, models.RoleViewer, link.Role.Name)
	assert.Equal(t, []invitelinks.InviteLinkLibraryRef{{ID: f.libA.ID, Name: "Library A"}}, link.Libraries)
	assert.False(t, link.AllLibraryAccess)
	assert.Equal(t, "admin", link.CreatedByUsername)
	// Seven days unless the admin picks another expiry.
	assert.WithinDuration(t, before.AddDate(0, 0, 7), link.ExpiresAt, time.Minute)

	everything := f.mustCreate(f.admin, fmt.Sprintf(`{"role_id":%d,"all_library_access":true,"library_ids":[%d],"expires_in_days":1}`, f.roleID(models.RoleEditor), f.libA.ID))
	assert.True(t, everything.AllLibraryAccess)
	assert.Empty(t, everything.Libraries)
	assert.WithinDuration(t, before.AddDate(0, 0, 1), everything.ExpiresAt, time.Minute)

	listed := f.pending()
	require.Len(t, listed, 2)
	assert.Equal(t, everything.ID, listed[0].ID, "newest first")
	assert.Equal(t, link.Token, listed[1].Token, "a pending link can be copied again")
	assert.Equal(t, []invitelinks.InviteLinkLibraryRef{{ID: f.libA.ID, Name: "Library A"}}, listed[1].Libraries)

	rec := f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/invite-links/%d/revoke", link.ID), "")
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	// Revoking twice is harmless.
	rec = f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/invite-links/%d/revoke", link.ID), "")
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	listed = f.pending()
	require.Len(t, listed, 1)
	assert.Equal(t, everything.ID, listed[0].ID)

	assert.Equal(t, http.StatusNotFound, f.do(f.admin, http.MethodPost, "/api/invite-links/9999/revoke", "").Code)
}

// The Users page names each Library an invite grants, however many
// Libraries the server has, so the names come with the invite.
func TestInviteLinks_ListNamesEveryLibrary(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	ids := []string{}
	want := []invitelinks.InviteLinkLibraryRef{}
	for i := range 12 {
		lib := &models.Library{Name: fmt.Sprintf("Shelf %02d", 12-i), CoverAspectRatio: "book"}
		_, err := f.db.NewInsert().Model(lib).Exec(context.Background())
		require.NoError(t, err)
		ids = append(ids, strconv.Itoa(lib.ID))
		want = append([]invitelinks.InviteLinkLibraryRef{{ID: lib.ID, Name: lib.Name}}, want...)
	}
	f.mustCreate(f.admin, fmt.Sprintf(`{"role_id":%d,"library_ids":[%s]}`, f.roleID(models.RoleViewer), strings.Join(ids, ",")))

	listed := f.pending()
	require.Len(t, listed, 1)
	assert.Equal(t, want, listed[0].Libraries, "every Library, by name")
}

func TestInviteLinks_CreateValidation(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	viewer := f.roleID(models.RoleViewer)
	for name, body := range map[string]string{
		"missing role":    `{}`,
		"unknown role":    `{"role_id":9999}`,
		"unknown library": fmt.Sprintf(`{"role_id":%d,"library_ids":[9999]}`, viewer),
		"expiry too long": fmt.Sprintf(`{"role_id":%d,"expires_in_days":31}`, viewer),
		"negative expiry": fmt.Sprintf(`{"role_id":%d,"expires_in_days":-1}`, viewer),
	} {
		rec := f.do(f.admin, http.MethodPost, "/api/invite-links", body)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "%s: %s", name, rec.Body.String())
	}
	assert.Empty(t, f.pending())
}

func TestInviteLinks_ManagementNeedsUsersWrite(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	link := f.viewerInvite(f.admin)
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/invite-links", ""},
		{http.MethodPost, "/api/invite-links", fmt.Sprintf(`{"role_id":%d}`, f.roleID(models.RoleViewer))},
		{http.MethodPost, fmt.Sprintf("/api/invite-links/%d/revoke", link.ID), ""},
	} {
		rec := f.do(f.viewer, request.method, request.path, request.body)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s", request.method, request.path)
		assert.Equal(t, http.StatusUnauthorized, f.do(nil, request.method, request.path, request.body).Code)
	}
	require.Len(t, f.pending(), 1)
}

func TestInviteLinks_AcceptCreatesTheUserAndSignsThemIn(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	link := f.viewerInvite(f.admin)

	rec := f.lookup(link.Token)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"valid":true,"invited_by":"admin"}`, rec.Body.String())

	rec = f.do(nil, http.MethodPost, "/api/invites/"+link.Token+"/accept",
		`{"username":"  newreader ","password":"password123","email":"reader@example.com"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var me auth.MeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &me))
	assert.Equal(t, "newreader", me.Username)
	assert.Equal(t, models.RoleViewer, me.RoleName)
	require.NotNil(t, me.LibraryAccess)
	assert.Equal(t, []int{f.libA.ID}, *me.LibraryAccess)
	assert.False(t, me.MustChangePassword, "they chose their own password")

	// The response signs them in.
	var session *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == auth.CookieName {
			session = cookie
		}
	}
	require.NotNil(t, session)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(session)
	meRec := httptest.NewRecorder()
	f.handler.ServeHTTP(meRec, req)
	require.Equal(t, http.StatusOK, meRec.Code, meRec.Body.String())
	assert.Contains(t, meRec.Body.String(), `"username":"newreader"`)

	// They get Getting Started, opening on its own.
	user := &models.User{}
	require.NoError(t, f.db.NewSelect().Model(user).Where("u.id = ?", me.ID).Scan(context.Background()))
	assert.True(t, user.GettingStartedEnabled)
	assert.True(t, user.GettingStartedAutoOpen)
	require.NotNil(t, user.Email)
	assert.Equal(t, "reader@example.com", *user.Email)

	// The invite is used and no longer listed.
	stored := &models.InviteLink{}
	require.NoError(t, f.db.NewSelect().Model(stored).Where("il.id = ?", link.ID).Scan(context.Background()))
	require.NotNil(t, stored.UsedAt)
	require.NotNil(t, stored.UsedByUserID)
	assert.Equal(t, me.ID, *stored.UsedByUserID)
	assert.Empty(t, f.pending())
}

func TestInviteLinks_AllLibrariesAndNoEmail(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	link := f.mustCreate(f.admin, fmt.Sprintf(`{"role_id":%d,"all_library_access":true}`, f.roleID(models.RoleEditor)))
	rec := f.accept(link.Token, "editor2")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var me auth.MeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &me))
	assert.Equal(t, models.RoleEditor, me.RoleName)
	assert.Nil(t, me.LibraryAccess, "every Library")
	assert.Nil(t, me.Email, "a blank email is no email")
}

func TestInviteLinks_AcceptingTwiceFails(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	link := f.viewerInvite(f.admin)
	require.Equal(t, http.StatusCreated, f.accept(link.Token, "first").Code)

	rec := f.accept(link.Token, "second")
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	exists, err := f.db.NewSelect().Model((*models.User)(nil)).Where("username = ?", "second").Exists(context.Background())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestInviteLinks_TakenUsernameKeepsTheInvitePending(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	link := f.viewerInvite(f.admin)

	rec := f.accept(link.Token, "VIEWER")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"code":"username_taken"`)

	rec = f.do(nil, http.MethodPost, "/api/invites/"+link.Token+"/accept", `{"username":"fresh","password":"password123"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

func TestInviteLinks_TakenEmailKeepsTheInvitePending(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	_, err := f.db.NewUpdate().Model((*models.User)(nil)).Set("email = ?", "taken@example.com").Where("id = ?", f.viewer.ID).Exec(context.Background())
	require.NoError(t, err)
	link := f.viewerInvite(f.admin)

	rec := f.do(nil, http.MethodPost, "/api/invites/"+link.Token+"/accept",
		`{"username":"fresh","password":"password123","email":"TAKEN@example.com"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"code":"email_taken"`)

	rec = f.do(nil, http.MethodPost, "/api/invites/"+link.Token+"/accept",
		`{"username":"fresh","password":"password123","email":"other@example.com"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

// An invite that can't be accepted gets one answer whatever the reason, so
// whoever holds it can't tell an expired link from a revoked or used one.
func TestInviteLinks_UnavailableInvitesShareOneResponse(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	ctx := context.Background()

	expired := f.viewerInvite(f.admin)
	f.setColumn(expired, "expires_at", time.Now().Add(-time.Minute))
	revoked := f.viewerInvite(f.admin)
	require.Equal(t, http.StatusNoContent, f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/invite-links/%d/revoke", revoked.ID), "").Code)
	used := f.viewerInvite(f.admin)
	require.Equal(t, http.StatusCreated, f.accept(used.Token, "usedit").Code)

	// A creator who is deactivated, or can no longer add users, stops their
	// invites from working.
	deactivatedCreator := insertPermissionTestUser(ctx, t, f.db, "gone", models.RoleAdmin, nil)
	byDeactivated := f.viewerInvite(deactivatedCreator)
	require.Equal(t, http.StatusNoContent, f.do(f.admin, http.MethodDelete, fmt.Sprintf("/api/users/%d", deactivatedCreator.ID), "").Code)
	demotedCreator := insertPermissionTestUser(ctx, t, f.db, "demoted", models.RoleAdmin, nil)
	byDemoted := f.viewerInvite(demotedCreator)
	rec := f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/users/%d", demotedCreator.ID), fmt.Sprintf(`{"role_id":%d}`, f.roleID(models.RoleEditor)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	// A deleted creator takes their invites with them.
	deletedCreator := insertPermissionTestUser(ctx, t, f.db, "deleted", models.RoleAdmin, nil)
	byDeleted := f.viewerInvite(deletedCreator)
	_, err := f.db.NewDelete().Model((*models.User)(nil)).Where("id = ?", deletedCreator.ID).Exec(ctx)
	require.NoError(t, err)

	assert.Empty(t, f.pending(), "none of them works, so none is listed")

	var acceptBody string
	for name, tc := range map[string]struct{ token, invitedBy string }{
		"expired": {expired.Token, "admin"},
		"revoked": {revoked.Token, "admin"},
		"used":    {used.Token, "admin"},
		// Asking a creator who can't add users for a new invite wouldn't
		// help, so they aren't named.
		"deactivated creator": {byDeactivated.Token, ""},
		"demoted creator":     {byDemoted.Token, ""},
	} {
		token := tc.token
		rec := f.lookup(token)
		require.Equal(t, http.StatusOK, rec.Code, name)
		assert.JSONEq(t, fmt.Sprintf(`{"valid":false,"invited_by":%q}`, tc.invitedBy), rec.Body.String(), name)

		rec = f.accept(token, "taker")
		require.Equal(t, http.StatusNotFound, rec.Code, name)
		if acceptBody == "" {
			acceptBody = rec.Body.String()
			assert.Contains(t, acceptBody, `"code":"not_found"`)
		}
		assert.JSONEq(t, acceptBody, rec.Body.String(), name)
	}

	// A token that matches no invite at all is a 404 like a failed accept.
	unknown := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	for _, token := range []string{unknown, "not-a-token", byDeleted.Token} {
		rec := f.lookup(token)
		assert.Equal(t, http.StatusNotFound, rec.Code, token)
		assert.JSONEq(t, acceptBody, rec.Body.String(), token)
		assert.JSONEq(t, acceptBody, f.accept(token, "taker").Body.String(), token)
	}

	exists, err := f.db.NewSelect().Model((*models.User)(nil)).Where("username = ?", "taker").Exists(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestInviteLinks_RevokingAUsedInviteIsRefused(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	link := f.viewerInvite(f.admin)
	require.Equal(t, http.StatusCreated, f.accept(link.Token, "taken").Code)
	rec := f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/invite-links/%d/revoke", link.ID), "")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"code":"invalid_state"`)
}

func TestInviteLinks_RejectedInDemoMode(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, true)
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	for _, request := range []struct {
		user         *models.User
		method, path string
	}{
		{nil, http.MethodGet, "/api/invites/" + token},
		// The demo's admin password is public, so its pending invites,
		// tokens included, aren't listed either.
		{f.admin, http.MethodGet, "/api/invite-links"},
		{nil, http.MethodPost, "/api/invites/" + token + "/accept"},
		{f.admin, http.MethodPost, "/api/invite-links"},
		{f.admin, http.MethodPost, "/api/invite-links/1/revoke"},
	} {
		rec := f.do(request.user, request.method, request.path, `{}`)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s", request.method, request.path)
		assert.Contains(t, rec.Body.String(), `"code":"demo_mode"`)
	}
}

// A failed lookup is a server fault, not an unavailable invite.
func TestInviteLinks_LookupFaultIsServerError(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	link := f.viewerInvite(f.admin)
	f.setColumn(link, "expires_at", "not a time")

	assert.Equal(t, http.StatusInternalServerError, f.lookup(link.Token).Code)
	assert.Equal(t, http.StatusInternalServerError, f.accept(link.Token, "someone").Code)
	assert.Equal(t, http.StatusInternalServerError, f.do(f.admin, http.MethodGet, "/api/invite-links", "").Code)
	assert.Equal(t, http.StatusInternalServerError, f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/invite-links/%d/revoke", link.ID), "").Code)
}

// Usernames are stored composed, so "José" typed with a combining accent and
// "José" typed with a precomposed é are one username.
func TestInviteLinks_UsernamesThatLookAlikeAreOne(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	first := f.viewerInvite(f.admin)
	require.Equal(t, http.StatusCreated, f.accept(first.Token, "Jose\u0301").Code)

	second := f.viewerInvite(f.admin)
	rec := f.accept(second.Token, "Jos\u00e9")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"code":"username_taken"`)

	// Scripts written with marks are letters too.
	require.Equal(t, http.StatusCreated, f.accept(second.Token, "\u0930\u093e\u092e").Code)
}

// Add User and renaming store the composed spelling too, so a look-alike
// can't come in through either.
func TestUsers_CreateAndRenameStoreComposedUsernames(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	viewer := f.roleID(models.RoleViewer)

	rec := f.do(f.admin, http.MethodPost, "/api/users", fmt.Sprintf(`{"username":"Jose\u0301","password":"password123","role_id":%d}`, viewer))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = f.do(f.admin, http.MethodPost, "/api/users", fmt.Sprintf(`{"username":"Jos\u00e9","password":"password123","role_id":%d}`, viewer))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"code":"username_taken"`)

	rec = f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/users/%d", f.viewer.ID), `{"username":"Rene\u0301e"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var renamed models.User
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &renamed))
	assert.Equal(t, "Ren\u00e9e", renamed.Username)
}
