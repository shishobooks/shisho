package kobo

import (
	"context"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/httputil"
)

type contextKey string

const contextKeyScope contextKey = "kobo_scope"

// ScopeParser parses the sync scope from URL params based on the given scope type.
// The scope type is passed as a parameter since each route group knows its type:
//   - "all" -> SyncScope{Type: "all"}
//   - "library" -> SyncScope{Type: "library", LibraryID: &id} (parses scopeId param)
//   - "list" -> SyncScope{Type: "list", ListID: &id} (parses scopeId param)
func ScopeParser(scopeType string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			scope := &SyncScope{Type: scopeType}

			switch scopeType {
			case "library":
				id, err := httputil.ParamID(c, "scopeId", "Library")
				if err != nil {
					return err
				}
				scope.LibraryID = &id
			case "list":
				id, err := httputil.ParamID(c, "scopeId", "List")
				if err != nil {
					return err
				}
				scope.ListID = &id
			}

			ctx := context.WithValue(c.Request().Context(), contextKeyScope, scope)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
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
