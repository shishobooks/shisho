package auth

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
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

		cookie, err := c.Cookie(m.authService.cookieName)
		if err != nil || cookie.Value == "" {
			return errcodes.AuthenticationRequired()
		}

		claims, err := m.authService.ValidateToken(cookie.Value)
		if err != nil {
			return errcodes.InvalidSession()
		}

		// Verify user still exists and is active
		user, err := m.authService.GetUserByID(ctx, claims.UserID)
		if err != nil {
			return err
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
				return errcodes.PermissionDenied(resource, operation)
			}

			return next(c)
		}
	}
}

// Permission names one resource and operation for RequireAnyPermission.
type Permission = errcodes.Permission

// RequireAnyPermission returns middleware that allows the request when the
// user holds at least one of the permissions.
// Must be used after Authenticate middleware.
func (m *Middleware) RequireAnyPermission(permissions ...Permission) echo.MiddlewareFunc {
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

			return errcodes.AnyPermissionDenied(permissions...)
		}
	}
}

// RequireLibraryAccess returns middleware that checks if the user can access the library
// specified by the :libraryId or :id route parameter.
// Must be used after Authenticate middleware.
func (m *Middleware) RequireLibraryAccess(paramName string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Param(paramName) == "" {
				return next(c)
			}

			libraryID, err := httputil.ParamID(c, paramName, "Library")
			if err != nil {
				return err
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

		username, password, ok := BasicCredentials(c.Request())
		if !ok {
			return RespondBasicAuthRequired(c)
		}

		cacheKey := basicAuthCacheKey(username, password)
		user, ok := m.basicAuthCache.get(cacheKey)
		if !ok {
			authed, authErr := m.authService.Authenticate(ctx, username, password)
			if authErr != nil {
				// Only a credentials rejection is a challenge; a fault
				// loading the user is a server error.
				var codeErr *errcodes.Error
				if errors.As(authErr, &codeErr) && codeErr.HTTPCode == http.StatusUnauthorized {
					return RespondBasicAuthRequired(c)
				}
				return authErr
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
			return RespondBasicAuthRequired(c)
		}

		SetUser(c, user)

		return next(c)
	}
}

// BasicCredentials returns the username and password of a request's Basic
// Auth header, and false when it has none or it doesn't parse. The username
// never contains a ":", since the header splits at the first one.
func BasicCredentials(r *http.Request) (string, string, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Basic ") {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(header[len("Basic "):])
	if err != nil {
		return "", "", false
	}
	username, password, ok := strings.Cut(string(decoded), ":")
	return username, password, ok
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

	id, err := httputil.ParamID(c, "id", "User")
	if err != nil {
		return false
	}

	return id == userID
}

// RespondBasicAuthRequired sets the Basic challenge that prompts an OPDS
// reader for credentials and returns the 401 in the errcodes shape every
// other API error uses.
func RespondBasicAuthRequired(c echo.Context) error {
	c.Response().Header().Set("WWW-Authenticate", `Basic realm="Shisho OPDS"`)
	return errcodes.AuthenticationRequired()
}
