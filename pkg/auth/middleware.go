package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// Context keys for storing user data.
type contextKey string

const (
	ContextKeyUserID   contextKey = "user_id"
	ContextKeyUsername contextKey = "username"
	ContextKeyUser     contextKey = "user"
)

// Middleware provides authentication middleware.
type Middleware struct {
	authService *Service
}

// NewMiddleware creates a new auth middleware.
func NewMiddleware(authService *Service) *Middleware {
	return &Middleware{
		authService: authService,
	}
}

// Authenticate extracts and validates the JWT from the cookie.
// If valid, it verifies the user is still active and adds user info to the context.
// If not authenticated, it returns 401.
func (m *Middleware) Authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		cookie, err := c.Cookie(CookieName)
		if err != nil || cookie.Value == "" {
			return errcodes.Unauthorized("Authentication required")
		}

		claims, err := m.authService.ValidateToken(cookie.Value)
		if err != nil {
			return errcodes.Unauthorized("Invalid or expired token")
		}

		// Verify user still exists and is active
		user, err := m.authService.GetUserByID(ctx, claims.UserID)
		if err != nil {
			return errcodes.Unauthorized("User not found or inactive")
		}

		if user.MustChangePassword && !isSelfPasswordResetRequest(c, user.ID) {
			return errcodes.PasswordResetRequired()
		}

		// Store user info in context
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("user", user)

		return next(c)
	}
}

// AuthenticateOptional extracts user info if available but doesn't require authentication.
// If a valid token is present, it verifies the user is still active.
func (m *Middleware) AuthenticateOptional(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		cookie, err := c.Cookie(CookieName)
		if err == nil && cookie.Value != "" {
			claims, err := m.authService.ValidateToken(cookie.Value)
			if err == nil {
				// Verify user still exists and is active
				user, err := m.authService.GetUserByID(ctx, claims.UserID)
				if err == nil {
					c.Set("user_id", user.ID)
					c.Set("username", user.Username)
					c.Set("user", user)
				}
			}
		}
		return next(c)
	}
}

// RequirePermission returns middleware that checks if the user has the required permission.
// Must be used after Authenticate middleware.
func (m *Middleware) RequirePermission(resource, operation string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, ok := c.Get("user").(*models.User)
			if !ok {
				return errcodes.Unauthorized("Authentication required")
			}

			if !user.HasPermission(resource, operation) {
				return errcodes.Forbidden("You don't have permission to " + operation + " " + resource)
			}

			return next(c)
		}
	}
}

// RequireLibraryAccess returns middleware that checks if the user can access the library
// specified by the :libraryId or :id route parameter.
// Must be used after Authenticate middleware.
func (m *Middleware) RequireLibraryAccess(paramName string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			libraryIDStr := c.Param(paramName)
			if libraryIDStr == "" {
				return next(c)
			}

			libraryID, err := strconv.Atoi(libraryIDStr)
			if err != nil {
				return errcodes.NotFound("Library")
			}

			user, ok := c.Get("user").(*models.User)
			if !ok {
				return errcodes.Unauthorized("Authentication required")
			}

			if !user.HasLibraryAccess(libraryID) {
				return errcodes.Forbidden("You don't have access to this library")
			}

			return next(c)
		}
	}
}

// BasicAuth provides HTTP Basic Auth for OPDS endpoints.
func (m *Middleware) BasicAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()

		auth := c.Request().Header.Get("Authorization")
		if auth == "" {
			return respondBasicAuthRequired(c)
		}

		if !strings.HasPrefix(auth, "Basic ") {
			return respondBasicAuthRequired(c)
		}

		decoded, err := base64.StdEncoding.DecodeString(auth[6:])
		if err != nil {
			return respondBasicAuthRequired(c)
		}

		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return respondBasicAuthRequired(c)
		}

		username := parts[0]
		password := parts[1]

		user, err := m.authService.Authenticate(ctx, username, password)
		if err != nil {
			return respondBasicAuthRequired(c)
		}
		if user.MustChangePassword {
			return respondBasicAuthRequired(c)
		}

		// Store user info in context
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("user", user)

		return next(c)
	}
}

func isSelfPasswordResetRequest(c echo.Context, userID int) bool {
	if c.Request().Method != http.MethodPost {
		return false
	}

	path := c.Path()
	if path == "" {
		path = c.Request().URL.Path
	}
	if path != "/users/:id/reset-password" && path != "/api/users/:id/reset-password" {
		return false
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return false
	}

	return id == userID
}

func respondBasicAuthRequired(c echo.Context) error {
	c.Response().Header().Set("WWW-Authenticate", `Basic realm="Shisho OPDS"`)
	return c.String(http.StatusUnauthorized, "Unauthorized")
}

// GetUserFromContext retrieves the user from the context.
func GetUserFromContext(ctx context.Context) *models.User {
	user, _ := ctx.Value(ContextKeyUser).(*models.User)
	return user
}

// GetUserIDFromContext retrieves the user ID from the Echo context.
func GetUserIDFromContext(c echo.Context) (int, bool) {
	userID, ok := c.Get("user_id").(int)
	return userID, ok
}
