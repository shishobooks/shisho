package plugins

import (
	"context"
	"database/sql"
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
	if err := validatePluginRef(payload.Scope, payload.ID); err != nil {
		return err
	}

	// Refuse an installed id before downloading anything. installStaged
	// checks again under the plugin's lock.
	if _, err := h.service.RetrievePlugin(ctx, payload.Scope, payload.ID); err == nil {
		return alreadyInstalledError()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return errors.WithStack(err)
	}

	plugin := &models.Plugin{
		Scope:       payload.Scope,
		ID:          payload.ID,
		InstalledAt: time.Now(),
	}

	var pkg *stagedPackage
	if payload.DownloadURL != "" && payload.SHA256 != "" {
		// Install from provided download URL
		staged, err := h.installer.stagePackage(ctx, payload.Scope, payload.ID, payload.DownloadURL, payload.SHA256)
		if err != nil {
			logger.FromContext(ctx).Warn("plugin install failed", logger.Data{"url": payload.DownloadURL, "error": err.Error()})
			return installerError(err)
		}
		pkg = staged
	} else if payload.DownloadURL == "" && payload.SHA256 == "" {
		// Look up the plugin in repositories
		downloadURL, sha256Hash, repoURL, imageURL, err := h.findPluginInRepos(c, payload.Scope, payload.ID, payload.Version)
		if err != nil {
			return errors.WithStack(err)
		}

		staged, err := h.installer.stagePackage(ctx, payload.Scope, payload.ID, downloadURL, sha256Hash)
		if err != nil {
			logger.FromContext(ctx).Warn("plugin install failed", logger.Data{"url": downloadURL, "error": err.Error()})
			return installerError(err)
		}
		pkg = staged

		// Download plugin icon into the package (non-fatal)
		if imageURL != "" {
			_ = h.installer.DownloadPluginImage(ctx, pkg.dir, imageURL)
		}

		plugin.RepositoryScope = &payload.Scope
		plugin.RepositoryURL = &repoURL
	} else {
		return errcodes.ValidationError("Both download_url and sha256 must be provided together, or neither (to install from repository).")
	}

	if err := h.manager.installStaged(ctx, plugin, pkg); err != nil {
		switch {
		case errors.Is(err, ErrAlreadyInstalled):
			return alreadyInstalledError()
		case errors.Is(err, ErrDirectoryExists):
			return errcodes.InvalidState("A plugin directory for this scope and ID already exists. Remove it, or use Scan for Local Plugins to add it.")
		}
		return errors.WithStack(err)
	}

	if plugin.Status != models.PluginStatusActive {
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

// alreadyInstalledError is the 422 for installing a scope and id that is
// already installed. Updating is how an installed plugin changes version.
func alreadyInstalledError() error {
	return errcodes.InvalidState("Plugin is already installed.")
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
func (h *handler) findPluginInRepos(c echo.Context, scope, pluginID, version string) (downloadURL, sha256Hash, repoURL, imageURL string, err error) {
	repos, err := h.service.ListRepositories(c.Request().Context())
	if err != nil {
		return "", "", "", "", errors.WithStack(err)
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
						return v.DownloadURL, v.SHA256, repo.URL, p.ImageURL, nil
					}
				}
			} else {
				// Return the first (latest) compatible version
				v := compatible[0]
				return v.DownloadURL, v.SHA256, repo.URL, p.ImageURL, nil
			}
		}
	}

	if !answered && fetchErr != nil {
		return "", "", "", "", errcodes.UpstreamError("Could not reach the plugin repository: " + fetchErr.Error())
	}
	return "", "", "", "", errcodes.NotFound("Plugin in repositories")
}

func (h *handler) uninstall(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")
	id := c.Param("id")
	if err := validatePluginRef(scope, id); err != nil {
		return err
	}

	if err := h.manager.uninstall(ctx, scope, id, c.QueryParam("delete_data") == "true"); err != nil {
		return errors.WithStack(err)
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
		if !validPathSegment(pluginID) {
			continue // Skip hidden directories
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
		if manifest.ID != pluginID {
			continue // The directory must be named after the manifest id
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

		if h.addScannedPlugin(ctx, plugin) {
			discovered = append(discovered, plugin)
		}
	}

	if discovered == nil {
		discovered = make([]*models.Plugin, 0)
	}

	return c.JSON(http.StatusOK, discovered)
}

// addScannedPlugin inserts a plugin the local scan found, under the plugin's
// lock so it cannot race an install of the same id. It reports whether the
// plugin was added; one that is already installed, or cannot be inserted, is
// skipped.
func (h *handler) addScannedPlugin(ctx context.Context, plugin *models.Plugin) bool {
	unlock := h.manager.lockPlugin(plugin.Scope, plugin.ID)
	defer unlock()
	if _, err := h.service.RetrievePlugin(ctx, plugin.Scope, plugin.ID); err == nil {
		return false
	}
	return h.service.InstallPlugin(ctx, plugin) == nil
}
