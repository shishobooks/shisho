package plugins

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

type pluginConfigResponse struct {
	Schema              ConfigSchema           `json:"schema"`
	Values              map[string]interface{} `json:"values"`
	DeclaredFields      []string               `json:"declaredFields"`
	FieldSettings       map[string]bool        `json:"fieldSettings"`
	ConfidenceThreshold *float64               `json:"confidence_threshold"`
}

// getManifest returns the raw manifest.json for an installed plugin.
func (h *handler) getManifest(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")

	if strings.Contains(scope, "..") || strings.Contains(id, "..") ||
		strings.ContainsAny(scope, "/\\") || strings.ContainsAny(id, "/\\") {
		return errcodes.ValidationError("Invalid scope or plugin ID")
	}

	if _, err := h.service.RetrievePlugin(ctx, scope, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Plugin")
		}
		return errors.WithStack(err)
	}

	manifestPath := filepath.Join(h.installer.PluginDir(), scope, id, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errcodes.NotFound("Manifest")
		}
		return errors.WithStack(err)
	}

	return c.Blob(http.StatusOK, "application/json", data)
}

func (h *handler) getConfig(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")

	// Get the plugin runtime to access manifest
	rt := h.manager.GetRuntime(scope, id)
	var schema ConfigSchema
	var declaredFields []string
	if rt != nil {
		schema = rt.Manifest().ConfigSchema
		if rt.Manifest().Capabilities.MetadataEnricher != nil {
			declaredFields = rt.Manifest().Capabilities.MetadataEnricher.Fields
		}
	}
	if schema == nil {
		schema = ConfigSchema{}
	}
	if declaredFields == nil {
		declaredFields = []string{}
	}

	// Get config values (masked secrets)
	values, err := h.service.GetConfig(ctx, scope, id, schema, false)
	if err != nil {
		return errors.WithStack(err)
	}

	// Get field settings
	fieldSettings, err := h.service.GetFieldSettings(ctx, scope, id)
	if err != nil {
		return errors.WithStack(err)
	}

	// Get plugin for confidence threshold
	plugin, err := h.service.GetPlugin(ctx, scope, id)
	if err != nil {
		return errors.WithStack(err)
	}

	var confidenceThreshold *float64
	if plugin != nil {
		confidenceThreshold = plugin.ConfidenceThreshold
	}

	return c.JSON(http.StatusOK, pluginConfigResponse{
		Schema:              schema,
		Values:              values,
		DeclaredFields:      declaredFields,
		FieldSettings:       fieldSettings,
		ConfidenceThreshold: confidenceThreshold,
	})
}
