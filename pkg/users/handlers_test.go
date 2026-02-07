package users

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUsersTestContext(t *testing.T, payload, path string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()

	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rr := httptest.NewRecorder()
	return e.NewContext(req, rr), rr
}

func TestHandlerResetPassword_SelfForcedReset_DoesNotRequireCurrentPassword(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	h := &handler{userService: NewService(db)}
	ctx := context.Background()

	user, err := h.userService.Create(ctx, CreateUserOptions{
		Username:             "forcedreset",
		Password:             "password123",
		RoleID:               getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess:     true,
		RequirePasswordReset: true,
	})
	require.NoError(t, err)

	c, rr := newUsersTestContext(t, `{"new_password":"newpassword123"}`, "/users/"+strconv.Itoa(user.ID)+"/reset-password")
	c.SetPath("/users/:id/reset-password")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(user.ID))
	c.Set("user_id", user.ID)
	c.Set("user", &models.User{ID: user.ID, MustChangePassword: true})

	err = h.resetPassword(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rr.Code)

	updatedUser, err := h.userService.Retrieve(ctx, user.ID)
	require.NoError(t, err)
	assert.False(t, updatedUser.MustChangePassword)

	valid, err := h.userService.VerifyPassword(ctx, user.ID, "newpassword123")
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestHandlerResetPassword_SelfNormal_RequiresCurrentPassword(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	h := &handler{userService: NewService(db)}
	ctx := context.Background()

	user, err := h.userService.Create(ctx, CreateUserOptions{
		Username:         "normalselfreset",
		Password:         "password123",
		RoleID:           getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess: true,
	})
	require.NoError(t, err)

	c, _ := newUsersTestContext(t, `{"new_password":"newpassword123"}`, "/users/"+strconv.Itoa(user.ID)+"/reset-password")
	c.SetPath("/users/:id/reset-password")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(user.ID))
	c.Set("user_id", user.ID)
	c.Set("user", &models.User{ID: user.ID, MustChangePassword: false})

	err = h.resetPassword(c)
	require.Error(t, err)

	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, "validation_error", codeErr.Code)
	assert.Equal(t, "Current password is required when resetting your own password", codeErr.Message)
}

func TestHandlerResetPassword_AdminResetsOtherUser_WithRequirePasswordReset(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	h := &handler{userService: NewService(db)}
	ctx := context.Background()

	// Create the target user
	targetUser, err := h.userService.Create(ctx, CreateUserOptions{
		Username:         "targetuser",
		Password:         "password123",
		RoleID:           getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess: true,
	})
	require.NoError(t, err)
	assert.False(t, targetUser.MustChangePassword)

	// Create an admin user
	adminUser, err := h.userService.Create(ctx, CreateUserOptions{
		Username:         "adminuser",
		Password:         "adminpass123",
		RoleID:           getRoleIDByName(ctx, t, db, models.RoleAdmin),
		AllLibraryAccess: true,
	})
	require.NoError(t, err)

	// Admin resets target user's password with require_password_reset: true
	c, rr := newUsersTestContext(t,
		`{"new_password":"temppass123","require_password_reset":true}`,
		"/users/"+strconv.Itoa(targetUser.ID)+"/reset-password",
	)
	c.SetPath("/users/:id/reset-password")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(targetUser.ID))
	c.Set("user_id", adminUser.ID)

	// Retrieve the full admin user so HasPermission works (needs Role.Permissions loaded)
	fullAdminUser, err := h.userService.Retrieve(ctx, adminUser.ID)
	require.NoError(t, err)
	c.Set("user", fullAdminUser)

	err = h.resetPassword(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify the flag was set on the target user
	updatedUser, err := h.userService.Retrieve(ctx, targetUser.ID)
	require.NoError(t, err)
	assert.True(t, updatedUser.MustChangePassword)

	// Verify password was actually changed
	valid, err := h.userService.VerifyPassword(ctx, targetUser.ID, "temppass123")
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestHandlerResetPassword_NonAdminCannotResetOtherUser(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	h := &handler{userService: NewService(db)}
	ctx := context.Background()

	// Create two viewer users
	targetUser, err := h.userService.Create(ctx, CreateUserOptions{
		Username:         "target",
		Password:         "password123",
		RoleID:           getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess: true,
	})
	require.NoError(t, err)

	attackerUser, err := h.userService.Create(ctx, CreateUserOptions{
		Username:         "attacker",
		Password:         "password123",
		RoleID:           getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess: true,
	})
	require.NoError(t, err)

	c, _ := newUsersTestContext(t,
		`{"new_password":"hacked123"}`,
		"/users/"+strconv.Itoa(targetUser.ID)+"/reset-password",
	)
	c.SetPath("/users/:id/reset-password")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(targetUser.ID))
	c.Set("user_id", attackerUser.ID)

	fullAttackerUser, err := h.userService.Retrieve(ctx, attackerUser.ID)
	require.NoError(t, err)
	c.Set("user", fullAttackerUser)

	err = h.resetPassword(c)
	require.Error(t, err)

	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, "forbidden", codeErr.Code)
}

func TestHandlerResetPassword_SelfResetIgnoresRequirePasswordResetParam(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	h := &handler{userService: NewService(db)}
	ctx := context.Background()

	user, err := h.userService.Create(ctx, CreateUserOptions{
		Username:             "selfreset",
		Password:             "password123",
		RoleID:               getRoleIDByName(ctx, t, db, models.RoleViewer),
		AllLibraryAccess:     true,
		RequirePasswordReset: true,
	})
	require.NoError(t, err)

	// Self-reset sending require_password_reset: true — should be ignored
	c, rr := newUsersTestContext(t,
		`{"new_password":"newpassword123","require_password_reset":true}`,
		"/users/"+strconv.Itoa(user.ID)+"/reset-password",
	)
	c.SetPath("/users/:id/reset-password")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(user.ID))
	c.Set("user_id", user.ID)
	c.Set("user", &models.User{ID: user.ID, MustChangePassword: true})

	err = h.resetPassword(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Flag should be cleared despite the param being true
	updatedUser, err := h.userService.Retrieve(ctx, user.ID)
	require.NoError(t, err)
	assert.False(t, updatedUser.MustChangePassword)
}
