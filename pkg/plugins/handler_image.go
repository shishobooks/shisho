package plugins

import (
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
)

func (h *handler) getImage(c echo.Context) error {
	scope := c.Param("scope")
	id := c.Param("id")

	if err := validatePluginRef(scope, id); err != nil {
		return err
	}

	iconPath := filepath.Join(h.installer.PluginDir(), scope, id, "icon.png")

	// The icon URL has no cache-busting version and the icon changes when the
	// plugin is updated, so clients revalidate through Last-Modified.
	return httputil.ServeFile(c, iconPath, errcodes.NotFound("Plugin icon"),
		httputil.WithCacheControl(covers.CacheControlNoCache))
}
