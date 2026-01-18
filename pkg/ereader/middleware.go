package ereader

import (
	"context"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

type contextKey string

const (
	// contextKeyAPIKey is the key for storing API key in context.
	contextKeyAPIKey contextKey = "ereader_api_key" //nolint:gosec
)

// Middleware provides authentication middleware for eReader routes.
type Middleware struct {
	apiKeyService *apikeys.Service
}

// NewMiddleware creates a new eReader middleware.
func NewMiddleware(apiKeyService *apikeys.Service) *Middleware {
	return &Middleware{apiKeyService: apiKeyService}
}

// APIKeyAuth validates the API key from the URL path and checks for required permission.
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
				return errcodes.Forbidden("API key lacks required permission")
			}

			// Touch last accessed (fire and forget)
			go func() {
				_ = m.apiKeyService.TouchLastAccessed(context.Background(), apiKey.ID)
			}()

			// Store API key in context
			ctx := context.WithValue(c.Request().Context(), contextKeyAPIKey, apiKey)
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
