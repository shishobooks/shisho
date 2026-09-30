package plugins

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

func (h *handler) install(c echo.Context) error {
	ctx := c.Request().Context()

	var payload InstallPluginPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	if payload.Scope == "" || payload.ID == "" {
		return errcodes.ValidationError("Scope and ID are required.")
	}

	var plugin *models.Plugin

	if payload.DownloadURL != "" && payload.SHA256 != "" {
		// Install from provided download URL
		manifest, err := h.installer.InstallPlugin(ctx, payload.Scope, payload.ID, payload.DownloadURL, payload.SHA256)
		if err != nil {
			logger.FromContext(ctx).Warn("plugin install failed", logger.Data{"url": payload.DownloadURL, "error": err.Error()})
			return installerError(err)
		}

		plugin = &models.Plugin{
			Scope:       payload.Scope,
			ID:          manifest.ID,
			Name:        manifest.Name,
			Version:     manifest.Version,
			Status:      models.PluginStatusActive,
			InstalledAt: time.Now(),
		}
		if manifest.Description != "" {
			plugin.Description = &manifest.Description
		}
		if manifest.Homepage != "" {
			plugin.Homepage = &manifest.Homepage
		}
	} else if payload.DownloadURL == "" && payload.SHA256 == "" {
		// Look up the plugin in repositories
		downloadURL, sha256Hash, version, repoURL, imageURL, err := h.findPluginInRepos(c, payload.Scope, payload.ID, payload.Version)
		if err != nil {
			return errors.WithStack(err)
		}

		manifest, err := h.installer.InstallPlugin(ctx, payload.Scope, payload.ID, downloadURL, sha256Hash)
		if err != nil {
			logger.FromContext(ctx).Warn("plugin install failed", logger.Data{"url": downloadURL, "error": err.Error()})
			return installerError(err)
		}

		// Download plugin icon (non-fatal)
		if imageURL != "" {
			_ = h.installer.DownloadPluginImage(ctx, payload.Scope, manifest.ID, imageURL)
		}

		plugin = &models.Plugin{
			Scope:           payload.Scope,
			ID:              manifest.ID,
			Name:            manifest.Name,
			Version:         version,
			Status:          models.PluginStatusActive,
			RepositoryScope: &payload.Scope,
			RepositoryURL:   &repoURL,
			InstalledAt:     time.Now(),
		}
		if manifest.Description != "" {
			plugin.Description = &manifest.Description
		}
		if manifest.Homepage != "" {
			plugin.Homepage = &manifest.Homepage
		}
	} else {
		return errcodes.ValidationError("Both download_url and sha256 must be provided together, or neither (to install from repository).")
	}

	if err := h.service.InstallPlugin(ctx, plugin); err != nil {
		return errors.WithStack(err)
	}

	if err := h.manager.LoadPlugin(ctx, plugin.Scope, plugin.ID); err != nil {
		if !asLoadError(err) {
			// Never loaded, so remove it rather than leave it Active with
			// no runtime.
			h.removeFailedInstall(ctx, plugin)
			return errors.WithStack(err)
		}
		// Store load error but don't fail the install
		errMsg := err.Error()
		plugin.LoadError = &errMsg
		if isVersionIncompatible(err) {
			plugin.Status = models.PluginStatusNotSupported
		} else {
			plugin.Status = models.PluginStatusMalfunctioned
		}
		if err := h.service.UpdatePlugin(ctx, plugin); err != nil {
			h.removeFailedInstall(ctx, plugin)
			return errors.WithStack(err)
		}
		h.manager.emitEvent(PluginEventMalfunctioned, plugin.Scope, plugin.ID, nil)
	} else {
		var hooks []string
		if rt := h.manager.GetRuntime(plugin.Scope, plugin.ID); rt != nil {
			hooks = rt.HookTypes()
		}
		h.manager.emitEvent(PluginEventInstalled, plugin.Scope, plugin.ID, hooks)
	}

	return errors.WithStack(c.JSON(http.StatusCreated, plugin))
}

// removeFailedInstall undoes an install that failed with a server fault after
// its row and files were written: it unloads the plugin and removes its files
// and row. Each step is best effort and logged, since the request is already
// failing.
func (h *handler) removeFailedInstall(ctx context.Context, plugin *models.Plugin) {
	log := logger.FromContext(ctx)
	h.manager.UnloadPlugin(plugin.Scope, plugin.ID)
	if err := h.installer.UninstallPlugin(plugin.Scope, plugin.ID); err != nil {
		log.Warn("failed to remove files of a failed plugin install", logger.Data{"scope": plugin.Scope, "id": plugin.ID, "error": err.Error()})
	}
	if err := h.service.UninstallPlugin(ctx, plugin.Scope, plugin.ID); err != nil {
		log.Warn("failed to remove the row of a failed plugin install", logger.Data{"scope": plugin.Scope, "id": plugin.ID, "error": err.Error()})
	}
}

// installerError renders an Installer failure for the install and update
// version routes. A download URL outside the allowed hosts, a checksum
// mismatch, or a package that is not a valid plugin is a 422. A download host
// that cannot be reached or answers with an error is a 502, because the
// request was fine and the upstream failed. Anything else, such as a plugin
// directory that cannot be written, is a server fault.
func installerError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, ErrInvalidDownloadURL), errors.Is(err, ErrChecksumMismatch), errors.Is(err, ErrInvalidPackage):
		return errcodes.ValidationError(err.Error())
	case errors.Is(err, ErrDownloadFailed):
		return errcodes.UpstreamError(err.Error())
	}
	return errors.WithStack(err)
}

// findPluginInRepos searches enabled repositories for a plugin by scope and ID.
// If version is empty, it returns the latest compatible version. When no
// repository for the scope answered, it returns a 502 naming the last fetch
// failure instead of a 404, since the plugin may well be listed.
func (h *handler) findPluginInRepos(c echo.Context, scope, pluginID, version string) (downloadURL, sha256Hash, resolvedVersion, repoURL, imageURL string, err error) {
	repos, err := h.service.ListRepositories(c.Request().Context())
	if err != nil {
		return "", "", "", "", "", errors.WithStack(err)
	}

	var answered bool
	var fetchErr error
	for _, repo := range repos {
		if !repo.Enabled || repo.Scope != scope {
			continue
		}

		manifest, err := FetchRepository(repo.URL)
		if err != nil {
			logger.FromContext(c.Request().Context()).Warn("plugin repository fetch failed", logger.Data{"url": repo.URL, "error": err.Error()})
			fetchErr = err
			continue
		}
		answered = true

		for _, p := range manifest.Plugins {
			if p.ID != pluginID {
				continue
			}

			compatible := FilterVersionCompatibleVersions(FilterCompatibleVersions(p.Versions))
			if len(compatible) == 0 {
				continue
			}

			if version != "" {
				// Find specific version
				for _, v := range compatible {
					if v.Version == version {
						return v.DownloadURL, v.SHA256, v.Version, repo.URL, p.ImageURL, nil
					}
				}
			} else {
				// Return the first (latest) compatible version
				v := compatible[0]
				return v.DownloadURL, v.SHA256, v.Version, repo.URL, p.ImageURL, nil
			}
		}
	}

	if !answered && fetchErr != nil {
		return "", "", "", "", "", errcodes.UpstreamError("Could not reach the plugin repository: " + fetchErr.Error())
	}
	return "", "", "", "", "", errcodes.NotFound("Plugin in repositories")
}

func (h *handler) uninstall(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")
	id := c.Param("id")

	// Run onUninstalling lifecycle hook before unloading
	if rt := h.manager.GetRuntime(scope, id); rt != nil {
		h.manager.RunOnUninstalling(rt)
	}

	h.manager.UnloadPlugin(scope, id)

	if err := h.installer.UninstallPlugin(scope, id); err != nil {
		return errors.WithStack(err)
	}

	if err := h.service.UninstallPlugin(ctx, scope, id); err != nil {
		return errors.WithStack(err)
	}

	// Optionally delete persistent plugin data
	if c.QueryParam("delete_data") == "true" {
		h.manager.DeletePluginData(scope, id)
	}

	h.manager.emitEvent(PluginEventUninstalled, scope, id, nil)

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) scan(c echo.Context) error {
	ctx := c.Request().Context()

	// Walk the plugin directory for scope "local"
	localDir := filepath.Join(h.installer.PluginDir(), "local")

	// Check if local directory exists
	entries, err := os.ReadDir(localDir)
	if err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusOK, []*models.Plugin{})
		}
		return errors.WithStack(err)
	}

	var discovered []*models.Plugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginID := entry.Name()

		// Check if already installed
		_, err := h.service.RetrievePlugin(ctx, "local", pluginID)
		if err == nil {
			// Already exists in DB, skip
			continue
		}

		// Try to read manifest.json
		manifestPath := filepath.Join(localDir, pluginID, "manifest.json")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // Skip dirs without manifest
		}

		manifest, err := ParseManifest(manifestData)
		if err != nil {
			continue // Skip invalid manifests
		}

		// Insert as disabled
		plugin := &models.Plugin{
			Scope:       "local",
			ID:          manifest.ID,
			Name:        manifest.Name,
			Version:     manifest.Version,
			Status:      models.PluginStatusDisabled,
			InstalledAt: time.Now(),
		}
		if manifest.Description != "" {
			plugin.Description = &manifest.Description
		}

		if err := h.service.InstallPlugin(ctx, plugin); err != nil {
			continue // Skip on DB error (e.g., duplicate)
		}

		discovered = append(discovered, plugin)
	}

	if discovered == nil {
		discovered = make([]*models.Plugin, 0)
	}

	return c.JSON(http.StatusOK, discovered)
}
