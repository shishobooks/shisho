package plugins

import (
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

type installPayload struct {
	Scope       string `json:"scope" validate:"required"`
	ID          string `json:"id" validate:"required"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
}

func (h *handler) install(c echo.Context) error {
	ctx := c.Request().Context()

	var payload installPayload
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
			return errcodes.ValidationError(err.Error())
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
			return errcodes.ValidationError(err.Error())
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
		// Store load error but don't fail the install
		errMsg := err.Error()
		plugin.LoadError = &errMsg
		if isVersionIncompatible(err) {
			plugin.Status = models.PluginStatusNotSupported
		} else {
			plugin.Status = models.PluginStatusMalfunctioned
		}
		_ = h.service.UpdatePlugin(ctx, plugin)
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

// findPluginInRepos searches enabled repositories for a plugin by scope and ID.
// If version is empty, it returns the latest compatible version.
func (h *handler) findPluginInRepos(c echo.Context, scope, pluginID, version string) (downloadURL, sha256Hash, resolvedVersion, repoURL, imageURL string, err error) {
	repos, err := h.service.ListRepositories(c.Request().Context())
	if err != nil {
		return "", "", "", "", "", errors.WithStack(err)
	}

	for _, repo := range repos {
		if !repo.Enabled || repo.Scope != scope {
			continue
		}

		manifest, fetchErr := FetchRepository(repo.URL)
		if fetchErr != nil {
			continue
		}

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
