package apikeys

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// apiKeyContextKey is the echo context key APIKeyAuth stores the key under.
const apiKeyContextKey = "api_key"

// permissionDeniedMessages holds the 403 message for a key that lacks a
// permission, where a route family words it for its own clients.
var permissionDeniedMessages = map[string]string{
	PermissionKoboSync: "This API key does not allow Kobo sync access.",
}

// Middleware authenticates the device routes (Kobo and eReader) that carry an
// API key in the URL.
type Middleware struct {
	service *Service
}

// NewMiddleware creates API key middleware backed by service.
func NewMiddleware(service *Service) *Middleware {
	return &Middleware{service: service}
}

// APIKeyAuth validates the key in c.Param("apiKey"), checks that it holds
// permission, and loads its owner with AuthenticateOwner, which rejects a
// missing or deactivated owner (401) and one without books:read (403). It
// touches the key's last accessed time without waiting, stores the key for
// RequireKey, and stores the owner with auth.SetUser, so handlers read
// it with auth.RequireUser and auth.RequireLibraryAccessFor like every other
// route.
func (m *Middleware) APIKeyAuth(permission string) echo.MiddlewareFunc {
	deniedMessage, ok := permissionDeniedMessages[permission]
	if !ok {
		deniedMessage = "This API key lacks the required permission."
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			apiKeyValue := c.Param("apiKey")
			if apiKeyValue == "" {
				return errcodes.Unauthorized("API key required")
			}

			apiKey, err := m.service.GetByKey(c.Request().Context(), apiKeyValue)
			if err != nil {
				return errors.WithStack(err)
			}
			if apiKey == nil {
				return errcodes.Unauthorized("Invalid API key")
			}

			if !apiKey.HasPermission(permission) {
				return errcodes.Forbidden(deniedMessage)
			}

			user, err := m.service.AuthenticateOwner(c.Request().Context(), apiKey)
			if err != nil {
				return errors.WithStack(err)
			}

			go func() {
				_ = m.service.TouchLastAccessed(context.Background(), apiKey.ID)
			}()

			SetKey(c, apiKey)
			auth.SetUser(c, user)

			return next(c)
		}
	}
}

// SetKey stores apiKey for RequireKey. Only APIKeyAuth, and tests
// standing in for it, call it.
func SetKey(c echo.Context, apiKey *APIKey) {
	c.Set(apiKeyContextKey, apiKey)
}

// RequireKey returns the API key APIKeyAuth stored, or a 401 when there is
// none, so a route registered without APIKeyAuth fails closed.
func RequireKey(c echo.Context) (*APIKey, error) {
	apiKey, ok := c.Get(apiKeyContextKey).(*APIKey)
	if !ok || apiKey == nil {
		return nil, errcodes.Unauthorized("API key not found")
	}
	return apiKey, nil
}
