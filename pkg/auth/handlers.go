package auth

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
)

const (
	// CookieName is the name of the session cookie.
	CookieName = "shisho_session"
	// CookieMaxAge is how long the cookie is valid.
	CookieMaxAge = 7 * 24 * time.Hour // 7 days
)

type handler struct {
	authService *Service
}

// buildMeResponse builds a MeResponse from a user model.
func buildMeResponse(user *models.User) MeResponse {
	permissions := make([]string, 0)
	if user.Role != nil {
		for _, p := range user.Role.Permissions {
			permissions = append(permissions, p.Resource+":"+p.Operation)
		}
	}

	var libraryAccess *[]int
	if accessibleIDs := user.GetAccessibleLibraryIDs(); accessibleIDs != nil {
		libraryAccess = &accessibleIDs
	}

	return MeResponse{
		ID:            user.ID,
		Username:      user.Username,
		Email:         user.Email,
		RoleID:        user.RoleID,
		RoleName:      user.Role.Name,
		Permissions:   permissions,
		LibraryAccess: libraryAccess,
	}
}

// login handles user login.
func (h *handler) login(c echo.Context) error {
	ctx := c.Request().Context()

	params := LoginPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := h.authService.Authenticate(ctx, params.Username, params.Password)
	if err != nil {
		return err
	}

	token, err := h.authService.GenerateToken(user)
	if err != nil {
		return errors.WithStack(err)
	}

	// Set HTTP-only cookie
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(CookieMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   c.Request().TLS != nil || c.Request().Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	}
	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, buildMeResponse(user))
}

// logout handles user logout.
func (h *handler) logout(c echo.Context) error {
	// Clear cookie by setting MaxAge to -1
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.Request().TLS != nil || c.Request().Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	}
	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, map[string]string{"message": "Logged out successfully"})
}

// me returns the current authenticated user's info.
func (h *handler) me(c echo.Context) error {
	ctx := c.Request().Context()

	// Read the session cookie directly
	cookie, err := c.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
	}

	// Validate the token
	claims, err := h.authService.ValidateToken(cookie.Value)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid or expired token"})
	}

	user, err := h.authService.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "User not found"})
	}

	return c.JSON(http.StatusOK, buildMeResponse(user))
}

// status returns whether the app needs initial setup.
func (h *handler) status(c echo.Context) error {
	ctx := c.Request().Context()

	count, err := h.authService.CountUsers(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, StatusResponse{
		NeedsSetup: count == 0,
	})
}

// setup creates the first admin user.
func (h *handler) setup(c echo.Context) error {
	ctx := c.Request().Context()

	params := SetupPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, err := h.authService.CreateFirstAdmin(ctx, params.Username, params.Email, params.Password)
	if err != nil {
		return err
	}

	token, err := h.authService.GenerateToken(user)
	if err != nil {
		return errors.WithStack(err)
	}

	// Set HTTP-only cookie
	cookie := &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(CookieMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   c.Request().TLS != nil || c.Request().Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	}
	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, buildMeResponse(user))
}
