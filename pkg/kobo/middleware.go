package kobo

import (
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/httputil"
)

// legacyURLScope serves the sync URLs API keys used, whose path carries a
// scope segment ("all", "library/:scopeId", or "list/:scopeId"), so a Kobo
// configured before Devices keeps syncing. A migrated Kobo takes its scope
// from the segment on its first request after upgrading; after that, and
// for every Device created since, the segment is ignored and the Device's
// own scope applies.
func legacyURLScope(service *devices.Service, scopeType string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			device, err := devices.RequireDevice(c)
			if err != nil {
				return err
			}
			if !device.URLScopePending {
				return next(c)
			}

			scope := devices.Scope{Type: scopeType}
			switch scopeType {
			case devices.ScopeLibrary:
				id, err := httputil.ParamID(c, "scopeId", "Library")
				if err != nil {
					return err
				}
				// A library the owner cannot access synced nothing, and
				// keeps syncing nothing.
				user, err := auth.RequireUser(c)
				if err != nil {
					return err
				}
				if user.HasLibraryAccess(id) {
					scope.LibraryID = &id
				}
			case devices.ScopeList:
				id, err := httputil.ParamID(c, "scopeId", "List")
				if err != nil {
					return err
				}
				scope.ListID = &id
			}

			device, err = service.ApplyURLScope(c.Request().Context(), device, scope)
			if err != nil {
				return errors.WithStack(err)
			}
			devices.SetDevice(c, device)
			return next(c)
		}
	}
}

// basePath returns the part of a Kobo request's path before the Kobo API
// path, such as "/kobo/<credential>" or a legacy "/kobo/<credential>/all",
// so the URLs Shisho hands the Kobo come back through the same route.
func basePath(path string) string {
	for _, marker := range []string{"/v1/", "/download/"} {
		if idx := strings.Index(path, marker); idx != -1 {
			return path[:idx]
		}
	}
	return path
}

// StripKoboPrefix strips everything before /v1/ to get the Kobo API path.
// For example, "/kobo/abc/v1/library/sync" becomes "/v1/library/sync".
func StripKoboPrefix(path string) string {
	if idx := strings.Index(path, "/v1/"); idx != -1 {
		return path[idx:]
	}
	return path
}
