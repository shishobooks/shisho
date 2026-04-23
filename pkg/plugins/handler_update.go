package plugins

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type updatePayload struct {
	Enabled                  *bool             `json:"enabled"`
	AutoUpdate               *bool             `json:"auto_update"`
	Config                   map[string]string `json:"config"`
	ConfidenceThreshold      *float64          `json:"confidence_threshold"`
	ClearConfidenceThreshold *bool             `json:"clear_confidence_threshold"`
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")
	id := c.Param("id")

	var payload updatePayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	plugin, err := h.service.RetrievePlugin(ctx, scope, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Plugin")
		}
		return errors.WithStack(err)
	}

	var loadErr error
	if payload.Enabled != nil {
		wasActive := plugin.Status == models.PluginStatusActive

		if *payload.Enabled && !wasActive {
			// Enabling: set Active and load the plugin
			plugin.Status = models.PluginStatusActive
			if err := h.manager.LoadPlugin(ctx, scope, id); err != nil {
				loadErr = err
				errMsg := err.Error()
				plugin.LoadError = &errMsg
				if isVersionIncompatible(err) {
					plugin.Status = models.PluginStatusNotSupported
				} else {
					plugin.Status = models.PluginStatusMalfunctioned
				}
				h.manager.emitEvent(PluginEventMalfunctioned, scope, id, nil)
			} else {
				plugin.LoadError = nil
				var hooks []string
				if rt := h.manager.GetRuntime(scope, id); rt != nil {
					hooks = rt.HookTypes()
				}
				h.manager.emitEvent(PluginEventEnabled, scope, id, hooks)
			}
		} else if !*payload.Enabled && wasActive {
			// Disabling: unload the plugin
			h.manager.UnloadPlugin(scope, id)
			plugin.Status = models.PluginStatusDisabled
			plugin.LoadError = nil
			h.manager.emitEvent(PluginEventDisabled, scope, id, nil)
		}
	}

	if payload.AutoUpdate != nil {
		plugin.AutoUpdate = *payload.AutoUpdate
	}

	now := time.Now()
	plugin.UpdatedAt = &now

	if err := h.service.UpdatePlugin(ctx, plugin); err != nil {
		return errors.WithStack(err)
	}

	if payload.Config != nil {
		for key, value := range payload.Config {
			if err := h.service.SetConfig(ctx, scope, id, key, value); err != nil {
				return errors.WithStack(err)
			}
		}
	}

	if payload.ClearConfidenceThreshold != nil && *payload.ClearConfidenceThreshold {
		if err := h.service.UpdateConfidenceThreshold(ctx, scope, id, nil); err != nil {
			return errors.WithStack(err)
		}
		plugin.ConfidenceThreshold = nil
	} else if payload.ConfidenceThreshold != nil {
		if *payload.ConfidenceThreshold < 0 || *payload.ConfidenceThreshold > 1 {
			return errcodes.ValidationError("confidence_threshold must be between 0 and 1")
		}
		if err := h.service.UpdateConfidenceThreshold(ctx, scope, id, payload.ConfidenceThreshold); err != nil {
			return errors.WithStack(err)
		}
		plugin.ConfidenceThreshold = payload.ConfidenceThreshold
	}

	// Surface enable-time load failures as a 422 after applying config/threshold
	// writes, so a caller mixing enable+config in one payload still gets their
	// config persisted and the Malfunctioned state + error message reported.
	if loadErr != nil {
		return errcodes.PluginLoadFailure(loadErr.Error())
	}

	return errors.WithStack(c.JSON(http.StatusOK, plugin))
}

func (h *handler) reload(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")

	plugin, err := h.service.RetrievePlugin(ctx, scope, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Plugin")
		}
		return errors.WithStack(err)
	}

	if plugin.Status != models.PluginStatusActive {
		return errcodes.ValidationError("Plugin must be active to reload.")
	}

	if err := h.manager.ReloadPlugin(ctx, scope, id); err != nil {
		errMsg := err.Error()
		plugin.LoadError = &errMsg
	} else {
		plugin.LoadError = nil
	}

	// Re-read the manifest to pick up any name/version changes
	manifestPath := filepath.Join(h.installer.PluginDir(), scope, id, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err == nil {
		if manifest, err := ParseManifest(manifestData); err == nil {
			plugin.Name = manifest.Name
			plugin.Version = manifest.Version
			if manifest.Description != "" {
				plugin.Description = &manifest.Description
			}
		}
	}

	now := time.Now()
	plugin.UpdatedAt = &now

	if err := h.service.UpdatePlugin(ctx, plugin); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, plugin))
}

func (h *handler) updateVersion(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")

	// 1. Get the installed plugin
	plugin, err := h.service.RetrievePlugin(ctx, scope, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Plugin")
		}
		return errors.WithStack(err)
	}

	// 2. Check if an update is available
	if plugin.UpdateAvailableVersion == nil || *plugin.UpdateAvailableVersion == "" {
		return errcodes.ValidationError("No update available for this plugin.")
	}

	targetVersion := *plugin.UpdateAvailableVersion

	// 3. Find the download URL and SHA256 from repositories
	downloadURL, sha256Hash, _, _, imageURL, err := h.findPluginInRepos(c, scope, id, targetVersion)
	if err != nil {
		return errors.WithStack(err)
	}

	// 4. Install (overwrite) the plugin files
	manifest, err := h.installer.InstallPlugin(ctx, scope, id, downloadURL, sha256Hash)
	if err != nil {
		return errors.WithStack(err)
	}

	// Download updated plugin icon (non-fatal)
	if imageURL != "" {
		_ = h.installer.DownloadPluginImage(ctx, scope, manifest.ID, imageURL)
	}

	// 5. Hot-reload the plugin
	if err := h.manager.ReloadPlugin(ctx, scope, id); err != nil {
		errMsg := err.Error()
		plugin.LoadError = &errMsg
		h.manager.emitEvent(PluginEventMalfunctioned, scope, id, nil)
	} else {
		plugin.LoadError = nil
		var hooks []string
		if rt := h.manager.GetRuntime(scope, id); rt != nil {
			hooks = rt.HookTypes()
		}
		h.manager.emitEvent(PluginEventUpdated, scope, id, hooks)
	}

	// 6. Update DB record
	plugin.Version = manifest.Version
	plugin.UpdateAvailableVersion = nil
	plugin.Name = manifest.Name
	now := time.Now()
	plugin.UpdatedAt = &now

	if err := h.service.UpdatePlugin(ctx, plugin); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, plugin))
}
