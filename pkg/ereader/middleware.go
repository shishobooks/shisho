package ereader

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type contextKey string

const (
	// contextKeyAPIKey is the key for storing API key in context.
	contextKeyAPIKey contextKey = "ereader_api_key" //nolint:gosec
	// contextKeyUser is the key for storing the API key's owner in context.
	contextKeyUser contextKey = "ereader_user"
)

// Middleware provides authentication middleware for eReader routes.
type Middleware struct {
	apiKeyService *apikeys.Service
}

// NewMiddleware creates a new eReader middleware.
func NewMiddleware(apiKeyService *apikeys.Service) *Middleware {
	return &Middleware{apiKeyService: apiKeyService}
}

// APIKeyAuth validates the API key from the URL path, checks for the required
// permission, loads the key's owner (rejecting a deactivated owner or one
// without books:read), and stores the API key and owner in context.
func (m *Middleware) APIKeyAuth(requiredPermission string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			apiKeyValue := c.Param("apiKey")
			if apiKeyValue == "" {
				return errcodes.Unauthorized("API key required")
			}

			apiKey, err := m.apiKeyService.GetByKey(c.Request().Context(), apiKeyValue)
			if err != nil {
				return errors.WithStack(err)
			}
			if apiKey == nil {
				return errcodes.Unauthorized("Invalid API key")
			}

			if !apiKey.HasPermission(requiredPermission) {
				return errcodes.Forbidden("This API key lacks the required permission.")
			}

			user, err := m.apiKeyService.AuthenticateOwner(c.Request().Context(), apiKey)
			if err != nil {
				return errors.WithStack(err)
			}

			// Touch last accessed (fire and forget)
			go func() {
				_ = m.apiKeyService.TouchLastAccessed(context.Background(), apiKey.ID)
			}()

			// Store API key and owner in context
			ctx := context.WithValue(c.Request().Context(), contextKeyAPIKey, apiKey)
			ctx = context.WithValue(ctx, contextKeyUser, user)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

// GetAPIKeyFromContext retrieves the API key from context.
func GetAPIKeyFromContext(ctx context.Context) *apikeys.APIKey {
	if apiKey, ok := ctx.Value(contextKeyAPIKey).(*apikeys.APIKey); ok {
		return apiKey
	}
	return nil
}

// GetUserFromContext retrieves the API key's owner, loaded by APIKeyAuth with
// Role, Role.Permissions and LibraryAccess. Returns nil if not found.
func GetUserFromContext(ctx context.Context) *models.User {
	if user, ok := ctx.Value(contextKeyUser).(*models.User); ok {
		return user
	}
	return nil
}
