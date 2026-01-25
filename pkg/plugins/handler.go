package plugins

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

const validRepoURLPrefix = "https://raw.githubusercontent.com/"

type handler struct {
	service   *Service
	manager   *Manager
	installer *Installer
}

type installPayload struct {
	Scope       string `json:"scope" validate:"required"`
	ID          string `json:"id" validate:"required"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
}

type updatePayload struct {
	Enabled *bool             `json:"enabled"`
	Config  map[string]string `json:"config"`
}

type orderEntry struct {
	Scope string `json:"scope" validate:"required"`
	ID    string `json:"id" validate:"required"`
}

type setOrderPayload struct {
	Order []orderEntry `json:"order" validate:"required"`
}

func (h *handler) listIdentifierTypes(c echo.Context) error {
	ctx := c.Request().Context()

	types, err := h.service.ListIdentifierTypes(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, types))
}

func (h *handler) listInstalled(c echo.Context) error {
	ctx := c.Request().Context()

	plugins, err := h.service.ListPlugins(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, plugins))
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
			return errors.WithStack(err)
		}

		plugin = &models.Plugin{
			Scope:       payload.Scope,
			ID:          manifest.ID,
			Name:        manifest.Name,
			Version:     manifest.Version,
			Enabled:     true,
			InstalledAt: time.Now(),
		}
		if manifest.Description != "" {
			plugin.Description = &manifest.Description
		}
		if manifest.Author != "" {
			plugin.Author = &manifest.Author
		}
		if manifest.Homepage != "" {
			plugin.Homepage = &manifest.Homepage
		}
	} else if payload.DownloadURL == "" && payload.SHA256 == "" {
		// Look up the plugin in repositories
		downloadURL, sha256Hash, version, err := h.findPluginInRepos(c, payload.Scope, payload.ID, payload.Version)
		if err != nil {
			return errors.WithStack(err)
		}

		manifest, err := h.installer.InstallPlugin(ctx, payload.Scope, payload.ID, downloadURL, sha256Hash)
		if err != nil {
			return errors.WithStack(err)
		}

		plugin = &models.Plugin{
			Scope:       payload.Scope,
			ID:          manifest.ID,
			Name:        manifest.Name,
			Version:     version,
			Enabled:     true,
			InstalledAt: time.Now(),
		}
		if manifest.Description != "" {
			plugin.Description = &manifest.Description
		}
		if manifest.Author != "" {
			plugin.Author = &manifest.Author
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
		_ = h.service.UpdatePlugin(ctx, plugin)
	}

	return errors.WithStack(c.JSON(http.StatusCreated, plugin))
}

// findPluginInRepos searches enabled repositories for a plugin by scope and ID.
// If version is empty, it returns the latest compatible version.
func (h *handler) findPluginInRepos(c echo.Context, scope, pluginID, version string) (downloadURL, sha256Hash, resolvedVersion string, err error) {
	repos, err := h.service.ListRepositories(c.Request().Context())
	if err != nil {
		return "", "", "", errors.WithStack(err)
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

			compatible := FilterCompatibleVersions(p.Versions)
			if len(compatible) == 0 {
				continue
			}

			if version != "" {
				// Find specific version
				for _, v := range compatible {
					if v.Version == version {
						return v.DownloadURL, v.SHA256, v.Version, nil
					}
				}
			} else {
				// Return the first (latest) compatible version
				v := compatible[0]
				return v.DownloadURL, v.SHA256, v.Version, nil
			}
		}
	}

	return "", "", "", errcodes.NotFound("Plugin in repositories")
}

func (h *handler) uninstall(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")
	id := c.Param("id")

	h.manager.UnloadPlugin(scope, id)

	if err := h.installer.UninstallPlugin(scope, id); err != nil {
		return errors.WithStack(err)
	}

	if err := h.service.UninstallPlugin(ctx, scope, id); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
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

	if payload.Enabled != nil {
		wasEnabled := plugin.Enabled
		plugin.Enabled = *payload.Enabled

		if *payload.Enabled && !wasEnabled {
			// Enabling: load the plugin
			if err := h.manager.LoadPlugin(ctx, scope, id); err != nil {
				errMsg := err.Error()
				plugin.LoadError = &errMsg
			} else {
				plugin.LoadError = nil
			}
		} else if !*payload.Enabled && wasEnabled {
			// Disabling: unload the plugin
			h.manager.UnloadPlugin(scope, id)
			plugin.LoadError = nil
		}
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

	if !plugin.Enabled {
		return errcodes.ValidationError("Plugin must be enabled to reload.")
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

func (h *handler) getOrder(c echo.Context) error {
	ctx := c.Request().Context()

	hookType := c.Param("hookType")

	orders, err := h.service.GetOrder(ctx, hookType)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, orders))
}

func (h *handler) setOrder(c echo.Context) error {
	ctx := c.Request().Context()

	hookType := c.Param("hookType")

	var payload setOrderPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	orderEntries := make([]models.PluginOrder, len(payload.Order))
	for i, entry := range payload.Order {
		orderEntries[i] = models.PluginOrder{
			Scope:    entry.Scope,
			PluginID: entry.ID,
		}
	}

	if err := h.service.SetOrder(ctx, hookType, orderEntries); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

type addRepoPayload struct {
	URL   string `json:"url" validate:"required,url"`
	Scope string `json:"scope" validate:"required"`
}

func isValidRepoURL(url string) bool {
	return strings.HasPrefix(url, validRepoURLPrefix)
}

func (h *handler) listRepositories(c echo.Context) error {
	ctx := c.Request().Context()

	repos, err := h.service.ListRepositories(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, repos))
}

func (h *handler) addRepository(c echo.Context) error {
	ctx := c.Request().Context()

	var payload addRepoPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	if payload.URL == "" || payload.Scope == "" {
		return errcodes.ValidationError("URL and scope are required.")
	}

	if !isValidRepoURL(payload.URL) {
		return &errcodes.Error{
			HTTPCode: http.StatusBadRequest,
			Message:  "Invalid repository URL. Only GitHub raw content URLs are allowed (https://raw.githubusercontent.com/...).",
			Code:     "invalid_repo_url",
		}
	}

	repo := &models.PluginRepository{
		URL:        payload.URL,
		Scope:      payload.Scope,
		IsOfficial: false,
		Enabled:    true,
	}

	if err := h.service.AddRepository(ctx, repo); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusCreated, repo))
}

func (h *handler) removeRepository(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")

	if err := h.service.RemoveRepository(ctx, scope); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) syncRepository(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")

	repo, err := h.service.GetRepository(ctx, scope)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Repository")
		}
		return errors.WithStack(err)
	}

	manifest, fetchErr := FetchRepository(repo.URL)

	now := time.Now()
	repo.LastFetchedAt = &now

	if fetchErr != nil {
		errMsg := fetchErr.Error()
		repo.FetchError = &errMsg
		if err := h.service.UpdateRepository(ctx, repo); err != nil {
			return errors.WithStack(err)
		}
		return errors.WithStack(c.JSON(http.StatusOK, repo))
	}

	// Update repository metadata from manifest
	repo.Name = &manifest.Name
	repo.FetchError = nil

	if err := h.service.UpdateRepository(ctx, repo); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(c.JSON(http.StatusOK, repo))
}

// availablePluginResponse is the response format for available plugins.
type availablePluginResponse struct {
	Scope       string          `json:"scope"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Author      string          `json:"author"`
	Homepage    string          `json:"homepage"`
	Versions    []PluginVersion `json:"versions"`
}

// listAvailable aggregates plugins from all enabled repositories.
func (h *handler) listAvailable(c echo.Context) error {
	ctx := c.Request().Context()

	repos, err := h.service.ListRepositories(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	var result []availablePluginResponse

	for _, repo := range repos {
		if !repo.Enabled {
			continue
		}

		manifest, fetchErr := FetchRepository(repo.URL)
		if fetchErr != nil {
			continue
		}

		for _, p := range manifest.Plugins {
			compatible := FilterCompatibleVersions(p.Versions)
			if len(compatible) == 0 {
				continue
			}

			result = append(result, availablePluginResponse{
				Scope:       manifest.Scope,
				ID:          p.ID,
				Name:        p.Name,
				Description: p.Description,
				Author:      p.Author,
				Homepage:    p.Homepage,
				Versions:    compatible,
			})
		}
	}

	if result == nil {
		result = []availablePluginResponse{}
	}

	return errors.WithStack(c.JSON(http.StatusOK, result))
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
	downloadURL, sha256Hash, _, err := h.findPluginInRepos(c, scope, id, targetVersion)
	if err != nil {
		return errors.WithStack(err)
	}

	// 4. Install (overwrite) the plugin files
	manifest, err := h.installer.InstallPlugin(ctx, scope, id, downloadURL, sha256Hash)
	if err != nil {
		return errors.WithStack(err)
	}

	// 5. Hot-reload the plugin
	if err := h.manager.ReloadPlugin(ctx, scope, id); err != nil {
		errMsg := err.Error()
		plugin.LoadError = &errMsg
	} else {
		plugin.LoadError = nil
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

// retrieveAvailable returns details for a specific available plugin.
func (h *handler) retrieveAvailable(c echo.Context) error {
	ctx := c.Request().Context()

	scope := c.Param("scope")
	id := c.Param("id")

	repos, err := h.service.ListRepositories(ctx)
	if err != nil {
		return errors.WithStack(err)
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
			if p.ID != id {
				continue
			}

			compatible := FilterCompatibleVersions(p.Versions)
			if len(compatible) == 0 {
				continue
			}

			return errors.WithStack(c.JSON(http.StatusOK, availablePluginResponse{
				Scope:       manifest.Scope,
				ID:          p.ID,
				Name:        p.Name,
				Description: p.Description,
				Author:      p.Author,
				Homepage:    p.Homepage,
				Versions:    compatible,
			}))
		}
	}

	return errcodes.NotFound("Plugin")
}

type pluginConfigResponse struct {
	Schema ConfigSchema           `json:"schema"`
	Values map[string]interface{} `json:"values"`
}

func (h *handler) getConfig(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")

	// Get the plugin runtime to access manifest
	rt := h.manager.GetRuntime(scope, id)
	var schema ConfigSchema
	if rt != nil {
		schema = rt.Manifest().ConfigSchema
	}
	if schema == nil {
		schema = ConfigSchema{}
	}

	// Get config values (masked secrets)
	values, err := h.service.GetConfig(ctx, scope, id, schema, false)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, pluginConfigResponse{
		Schema: schema,
		Values: values,
	})
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
			Enabled:     false,
			InstalledAt: time.Now(),
		}
		if manifest.Description != "" {
			plugin.Description = &manifest.Description
		}
		if manifest.Author != "" {
			plugin.Author = &manifest.Author
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

type libraryOrderEntry struct {
	Scope   string `json:"scope" validate:"required"`
	ID      string `json:"id" validate:"required"`
	Enabled bool   `json:"enabled"`
}

type setLibraryOrderPayload struct {
	Plugins []libraryOrderEntry `json:"plugins" validate:"required"`
}

type libraryOrderResponse struct {
	Customized bool                 `json:"customized"`
	Plugins    []libraryOrderPlugin `json:"plugins"`
}

type libraryOrderPlugin struct {
	Scope   string `json:"scope"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func (h *handler) getLibraryOrder(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	hookType := c.Param("hookType")

	customized, err := h.service.IsLibraryCustomized(ctx, libraryID, hookType)
	if err != nil {
		return errors.WithStack(err)
	}

	var plugins []libraryOrderPlugin

	if customized {
		entries, err := h.service.GetLibraryOrder(ctx, libraryID, hookType)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, entry := range entries {
			name := entry.Scope + "/" + entry.PluginID
			if p, _ := h.service.GetPlugin(ctx, entry.Scope, entry.PluginID); p != nil {
				name = p.Name
			}
			plugins = append(plugins, libraryOrderPlugin{
				Scope:   entry.Scope,
				ID:      entry.PluginID,
				Name:    name,
				Enabled: entry.Enabled,
			})
		}
	} else {
		orders, err := h.service.GetOrder(ctx, hookType)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, order := range orders {
			name := order.Scope + "/" + order.PluginID
			if p, _ := h.service.GetPlugin(ctx, order.Scope, order.PluginID); p != nil {
				name = p.Name
			}
			plugins = append(plugins, libraryOrderPlugin{
				Scope:   order.Scope,
				ID:      order.PluginID,
				Name:    name,
				Enabled: true,
			})
		}
	}

	return c.JSON(http.StatusOK, libraryOrderResponse{
		Customized: customized,
		Plugins:    plugins,
	})
}

func (h *handler) setLibraryOrder(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	hookType := c.Param("hookType")

	var payload setLibraryOrderPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	entries := make([]models.LibraryPlugin, len(payload.Plugins))
	for i, p := range payload.Plugins {
		entries[i] = models.LibraryPlugin{
			Scope:    p.Scope,
			PluginID: p.ID,
			Enabled:  p.Enabled,
		}
	}

	if err := h.service.SetLibraryOrder(ctx, libraryID, hookType, entries); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) resetLibraryOrder(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	hookType := c.Param("hookType")

	if err := h.service.ResetLibraryOrder(ctx, libraryID, hookType); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) resetAllLibraryOrders(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}

	if err := h.service.ResetAllLibraryOrders(ctx, libraryID); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// NewHandler creates a handler for testing and external route registration.
func NewHandler(service *Service, manager *Manager, installer *Installer) *handler { //nolint:revive // unexported return is intentional for same-package tests
	return &handler{service: service, manager: manager, installer: installer}
}

// Exported handler methods for testing.
func (h *handler) GetLibraryOrder(c echo.Context) error       { return h.getLibraryOrder(c) }
func (h *handler) SetLibraryOrder(c echo.Context) error       { return h.setLibraryOrder(c) }
func (h *handler) ResetLibraryOrder(c echo.Context) error     { return h.resetLibraryOrder(c) }
func (h *handler) ResetAllLibraryOrders(c echo.Context) error { return h.resetAllLibraryOrders(c) }
