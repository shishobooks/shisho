package plugins

import (
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
)

func (h *handler) getImage(c echo.Context) error {
	scope := c.Param("scope")
	id := c.Param("id")

	if strings.Contains(scope, "..") || strings.Contains(id, "..") ||
		strings.ContainsAny(scope, "/\\") || strings.ContainsAny(id, "/\\") {
		return errcodes.ValidationError("Invalid scope or plugin ID")
	}

	iconPath := filepath.Join(h.installer.PluginDir(), scope, id, "icon.png")

	// The icon URL has no cache-busting version and the icon changes when the
	// plugin is updated, so clients revalidate through Last-Modified.
	c.Response().Header().Set("Cache-Control", covers.CacheControlNoCache)
	return httputil.ServeFile(c, iconPath, errcodes.NotFound("Plugin icon"))
}
