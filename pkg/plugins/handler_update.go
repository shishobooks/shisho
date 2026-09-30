package plugins

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")
	id := c.Param("id")
	if err := validatePluginRef(scope, id); err != nil {
		return err
	}

	var payload UpdatePluginPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	// Validate everything before changing anything.
	clearThreshold := payload.ClearConfidenceThreshold != nil && *payload.ClearConfidenceThreshold
	if !clearThreshold && payload.ConfidenceThreshold != nil &&
		(*payload.ConfidenceThreshold < 0 || *payload.ConfidenceThreshold > 1) {
		return errcodes.ValidationError("confidence_threshold must be between 0 and 1")
	}

	unlock := h.manager.lockPlugin(scope, id)
	defer unlock()

	plugin, err := h.service.RetrievePlugin(ctx, scope, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Plugin")
		}
		return errors.WithStack(err)
	}

	now := time.Now()
	plugin.UpdatedAt = &now

	var loadErr error
	if payload.Enabled != nil {
		wasActive := plugin.Status == models.PluginStatusActive

		if *payload.Enabled && !wasActive {
			loadErr, err = h.manager.enableLocked(ctx, plugin)
			if err != nil {
				// A server fault, not the plugin's: its state is unchanged.
				return errors.WithStack(err)
			}
			if loadErr != nil {
				h.manager.emitEvent(PluginEventMalfunctioned, scope, id, nil)
			} else {
				var hooks []string
				if rt := h.manager.GetRuntime(scope, id); rt != nil {
					hooks = rt.HookTypes()
				}
				h.manager.emitEvent(PluginEventEnabled, scope, id, hooks)
			}
		} else if !*payload.Enabled && wasActive {
			if err := h.manager.disableLocked(ctx, plugin); err != nil {
				return errors.WithStack(err)
			}
			h.manager.emitEvent(PluginEventDisabled, scope, id, nil)
		}
	}

	if payload.AutoUpdate != nil {
		plugin.AutoUpdate = *payload.AutoUpdate
	}
	if err := h.service.UpdatePluginColumns(ctx, plugin, "auto_update", "updated_at"); err != nil {
		return errors.WithStack(err)
	}

	if payload.Config != nil {
		for key, value := range payload.Config {
			if err := h.service.SetConfig(ctx, scope, id, key, value); err != nil {
				return errors.WithStack(err)
			}
		}
	}

	if clearThreshold {
		if err := h.service.UpdateConfidenceThreshold(ctx, scope, id, nil); err != nil {
			return errors.WithStack(err)
		}
		plugin.ConfidenceThreshold = nil
	} else if payload.ConfidenceThreshold != nil {
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

// reload loads an Active plugin again from its directory, which is how a
// local plugin under development picks up changes. A version that fails to
// load is a 422 with its error stored on the plugin, and the previous
// runtime keeps running.
func (h *handler) reload(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")
	if err := validatePluginRef(scope, id); err != nil {
		return err
	}

	plugin, loadErr, err := h.manager.reload(ctx, scope, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errcodes.NotFound("Plugin")
	case errors.Is(err, ErrNotActive):
		return errcodes.InvalidState("Plugin must be active to reload.")
	case err != nil:
		return errors.WithStack(err)
	case loadErr != nil:
		return errcodes.PluginLoadFailure(loadErr.Error())
	}

	return errors.WithStack(c.JSON(http.StatusOK, plugin))
}

// updateVersion installs the version the update check found. The package
// is staged and loaded before anything installed changes, so a package
// that is invalid or does not load is a 422 that leaves the installed
// version in place and running.
func (h *handler) updateVersion(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")
	if err := validatePluginRef(scope, id); err != nil {
		return err
	}

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
		return errcodes.InvalidState("No update available for this plugin.")
	}

	targetVersion := *plugin.UpdateAvailableVersion

	// 3. Find the download URL and SHA256 from repositories
	downloadURL, sha256Hash, _, imageURL, err := h.findPluginInRepos(c, scope, id, targetVersion)
	if err != nil {
		return errors.WithStack(err)
	}

	// 4. Download, verify, and extract the package next to the installed one
	pkg, err := h.installer.stagePackage(ctx, scope, id, downloadURL, sha256Hash)
	if err != nil {
		return installerError(err)
	}

	// Download the updated plugin icon into the package (non-fatal)
	if imageURL != "" {
		_ = h.installer.DownloadPluginImage(ctx, pkg.dir, imageURL)
	}

	// 5. Load the new version, then swap it in
	updated, err := h.manager.updateStaged(ctx, scope, id, pkg)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errcodes.NotFound("Plugin")
	case err != nil && asLoadError(err):
		return errcodes.PluginLoadFailure(err.Error())
	case err != nil:
		return errors.WithStack(err)
	}

	var hooks []string
	if rt := h.manager.GetRuntime(scope, id); rt != nil {
		hooks = rt.HookTypes()
	}
	h.manager.emitEvent(PluginEventUpdated, scope, id, hooks)

	return errors.WithStack(c.JSON(http.StatusOK, updated))
}
