package plugins

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

func (h *handler) getImage(c echo.Context) error {
	scope := c.Param("scope")
	id := c.Param("id")

	if strings.Contains(scope, "..") || strings.Contains(id, "..") ||
		strings.ContainsAny(scope, "/\\") || strings.ContainsAny(id, "/\\") {
		return errcodes.ValidationError("Invalid scope or plugin ID")
	}

	iconPath := filepath.Join(h.installer.PluginDir(), scope, id, "icon.png")
	if _, err := os.Stat(iconPath); err != nil {
		return errcodes.NotFound("Plugin icon not found")
	}

	return c.File(iconPath)
}
