package kobo

import (
	"context"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type contextKey string

const (
	contextKeyAPIKey contextKey = "kobo_api_key" //nolint:gosec
	contextKeyScope  contextKey = "kobo_scope"
	contextKeyUser   contextKey = "kobo_user"
)

// Middleware provides authentication and scope middleware for Kobo routes.
type Middleware struct {
	apiKeyService *apikeys.Service
}

// NewMiddleware creates a new Kobo middleware.
func NewMiddleware(apiKeyService *apikeys.Service) *Middleware {
	return &Middleware{apiKeyService: apiKeyService}
}

// APIKeyAuth validates the API key from c.Param("apiKey"), checks kobo_sync permission,
// loads the key's owner (rejecting a deactivated owner or one without books:read),
// touches last accessed (fire and forget goroutine), and stores the API key and
// owner in context. Handlers read the owner with GetUserFromContext and never
// re-query it.
func (m *Middleware) APIKeyAuth() echo.MiddlewareFunc {
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

			if !apiKey.HasPermission(apikeys.PermissionKoboSync) {
				return errcodes.Forbidden("This API key does not allow Kobo sync access.")
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

// ScopeParser parses the sync scope from URL params based on the given scope type.
// The scope type is passed as a parameter since each route group knows its type:
//   - "all" -> SyncScope{Type: "all"}
//   - "library" -> SyncScope{Type: "library", LibraryID: &id} (parses scopeId param)
//   - "list" -> SyncScope{Type: "list", ListID: &id} (parses scopeId param)
func (m *Middleware) ScopeParser(scopeType string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			scope := &SyncScope{Type: scopeType}

			switch scopeType {
			case "library":
				id, err := strconv.Atoi(c.Param("scopeId"))
				if err != nil {
					return errcodes.NotFound("Library")
				}
				scope.LibraryID = &id
			case "list":
				id, err := strconv.Atoi(c.Param("scopeId"))
				if err != nil {
					return errcodes.NotFound("List")
				}
				scope.ListID = &id
			}

			ctx := context.WithValue(c.Request().Context(), contextKeyScope, scope)
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

// GetScopeFromContext retrieves the sync scope from context.
// Returns a default scope of {Type: "all"} if not found.
func GetScopeFromContext(ctx context.Context) *SyncScope {
	if scope, ok := ctx.Value(contextKeyScope).(*SyncScope); ok {
		return scope
	}
	return &SyncScope{Type: "all"}
}

// StripKoboPrefix strips everything before /v1/ to get the Kobo API path.
// For example, "/kobo/ak_123/all/v1/library/sync" becomes "/v1/library/sync".
func StripKoboPrefix(path string) string {
	if idx := strings.Index(path, "/v1/"); idx != -1 {
		return path[idx:]
	}
	return path
}
