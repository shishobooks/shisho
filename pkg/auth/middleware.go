package auth

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// Middleware provides authentication middleware.
type Middleware struct {
	authService    *Service
	basicAuthCache *basicAuthCache
}

// NewMiddleware creates a new auth middleware.
func NewMiddleware(authService *Service) *Middleware {
	return &Middleware{
		authService:    authService,
		basicAuthCache: newBasicAuthCache(defaultBasicAuthCacheTTL),
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
			return errcodes.AuthenticationRequired()
		}

		claims, err := m.authService.ValidateToken(cookie.Value)
		if err != nil {
			return errcodes.Unauthorized("Invalid or expired token")
		}

		// Verify user still exists and is active
		user, err := m.authService.GetUserByID(ctx, claims.UserID)
		if err != nil {
			return errcodes.UserInactive()
		}

		if user.MustChangePassword && !isSelfPasswordResetRequest(c, user.ID) {
			return errcodes.PasswordResetRequired()
		}

		SetUser(c, user)

		return next(c)
	}
}

// RequirePermission returns middleware that checks if the user has the required permission.
// Must be used after Authenticate middleware.
func (m *Middleware) RequirePermission(resource, operation string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, err := RequireUser(c)
			if err != nil {
				return err
			}

			if !user.HasPermission(resource, operation) {
				return errcodes.Forbidden("You don't have permission to " + operation + " " + resource)
			}

			return next(c)
		}
	}
}

// Permission names one resource and operation for RequireAnyPermission.
type Permission struct {
	Resource  string
	Operation string
}

// RequireAnyPermission returns middleware that allows the request when the
// user holds at least one of the permissions.
// Must be used after Authenticate middleware.
func (m *Middleware) RequireAnyPermission(permissions ...Permission) echo.MiddlewareFunc {
	names := make([]string, len(permissions))
	for i, p := range permissions {
		names[i] = p.Operation + " " + p.Resource
	}
	var listed string
	switch len(names) {
	case 0:
	case 1, 2:
		listed = strings.Join(names, " or ")
	default:
		listed = strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, err := RequireUser(c)
			if err != nil {
				return err
			}

			for _, p := range permissions {
				if user.HasPermission(p.Resource, p.Operation) {
					return next(c)
				}
			}

			return errcodes.Forbidden("You don't have permission to " + listed)
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

			if err := RequireLibraryAccessFor(c, libraryID); err != nil {
				return err
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

		cacheKey := basicAuthCacheKey(username, password)
		user, ok := m.basicAuthCache.get(cacheKey)
		if !ok {
			authed, authErr := m.authService.Authenticate(ctx, username, password)
			if authErr != nil {
				return respondBasicAuthRequired(c)
			}
			user = authed
			// Skip caching while a password reset is required so that
			// completing the reset takes effect on the very next OPDS hit
			// without waiting for the TTL.
			if !user.MustChangePassword {
				m.basicAuthCache.put(cacheKey, user)
			}
		}
		if user.MustChangePassword {
			return respondBasicAuthRequired(c)
		}

		SetUser(c, user)

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
