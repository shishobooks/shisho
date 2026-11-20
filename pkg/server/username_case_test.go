package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signIn posts username and password to the login route and returns the
// signed-in user's username, or "" when sign-in failed.
func (f *inviteLinksFixture) signIn(username, password string) string {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return ""
	}
	var me auth.MeResponse
	require.NoError(f.t, json.Unmarshal(rec.Body.Bytes(), &me))
	return me.Username
}

func (f *inviteLinksFixture) addUser(username string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(f.admin, http.MethodPost, "/api/users",
		fmt.Sprintf(`{"username":%q,"password":"password123","role_id":%d}`, username, f.roleID(models.RoleViewer)))
}

func (f *inviteLinksFixture) rename(user *models.User, username string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/users/%d", user.ID), fmt.Sprintf(`{"username":%q}`, username))
}

// Usernames compare without case for every letter, not just English ones:
// Add User, accepting an invite, and renaming all refuse a username that
// differs from another user's only in case, and sign-in takes any case.
func TestUsernames_IgnoreCaseForEveryLetter(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)

	rec := f.addUser("Émile")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var emile models.User
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &emile))
	require.Equal(t, http.StatusCreated, f.addUser("Straße").Code)
	require.Equal(t, http.StatusCreated, f.addUser("ΟΔΟΣ").Code)

	for _, taken := range []string{"émile", "ÉMILE", "STRASSE", "strasse", "οδος"} {
		rec := f.addUser(taken)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, taken)
		assert.Contains(t, rec.Body.String(), `"code":"username_taken"`, taken)

		rec = f.rename(f.viewer, taken)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, taken)
		assert.Contains(t, rec.Body.String(), `"code":"username_taken"`, taken)
	}

	// The invite form flags the field from the same code.
	link := f.viewerInvite(f.admin)
	rec = f.accept(link.Token, "éMILE")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"code":"username_taken"`)

	// The stored username keeps the case it was typed in, and sign-in takes
	// any case of it.
	assert.Equal(t, "Émile", emile.Username)
	for _, typed := range []string{"Émile", "émile", "ÉMILE", "E\u0301mile", "e\u0301MILE"} {
		assert.Empty(t, f.signIn(typed, "wrong password"), typed)
		assert.Equal(t, "Émile", f.signIn(typed, "password123"), typed)
	}
	assert.Equal(t, "Straße", f.signIn("STRASSE", "password123"))

	// A user can change the case of their own username, and it then signs
	// in as the new spelling too.
	rec = f.rename(&emile, "émile")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "émile", f.signIn("ÉMILE", "password123"))
	require.Equal(t, http.StatusCreated, f.addUser("Émile2").Code)
}

// A server upgraded with two users whose usernames differ only in the case
// of a non-English letter keeps both (the upgrade leaves their usernames
// unkeyed). Each still signs in as their exact username, and no third user
// can take another case of it.
func TestUsernames_UsersThatDifferOnlyInCaseFromBeforeTheUpgrade(t *testing.T) {
	t.Parallel()
	f := newInviteLinksFixture(t, false)
	ctx := context.Background()
	hash, err := auth.HashPassword("first-password")
	require.NoError(t, err)
	otherHash, err := auth.HashPassword("second-password")
	require.NoError(t, err)
	roleID := f.roleID(models.RoleViewer)
	_, err = f.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, role_id, is_active)
		VALUES ('Émile', ?, ?, TRUE), ('émile', ?, ?, TRUE)
	`, hash, roleID, otherHash, roleID)
	require.NoError(t, err)

	assert.Equal(t, "Émile", f.signIn("Émile", "first-password"))
	assert.Equal(t, "émile", f.signIn("émile", "second-password"))
	assert.Empty(t, f.signIn("Émile", "second-password"))
	assert.Empty(t, f.signIn("émile", "first-password"))
	// Before the upgrade, a username matched in any case of its English
	// letters; that still works.
	assert.Equal(t, "Émile", f.signIn("ÉMILE", "first-password"))
	assert.Equal(t, "émile", f.signIn("éMILE", "second-password"))

	rec := f.addUser("ÉMILE")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"code":"username_taken"`)
	rec = f.rename(f.viewer, "éMILE")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"code":"username_taken"`)
}
