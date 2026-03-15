package plugins

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/htmlutil"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/shishobooks/shisho/pkg/sortname"
	"github.com/uptrace/bun"
)

const validRepoURLPrefix = "https://raw.githubusercontent.com/"

// enrichDeps holds dependencies for the enrich endpoint.
// Uses interfaces to avoid circular imports with the books package.
type enrichDeps struct {
	bookStore       bookStore
	relStore        relationStore
	identStore      identifierStore
	personFinder    personFinder
	genreFinder     genreFinder
	tagFinder       tagFinder
	publisherFinder publisherFinder
	imprintFinder   imprintFinder
	searchIndexer   searchIndexer
}

// bookStore provides core book and file CRUD operations.
type bookStore interface {
	UpdateBook(ctx context.Context, book *models.Book, columns []string) error
	RetrieveBook(ctx context.Context, bookID int) (*models.Book, error)
	UpdateFile(ctx context.Context, file *models.File, columns []string) error
	DeleteNarratorsForFile(ctx context.Context, fileID int) (int, error)
	CreateNarrator(ctx context.Context, narrator *models.Narrator) error
}

// relationStore provides book relationship CRUD operations.
type relationStore interface {
	DeleteAuthors(ctx context.Context, bookID int) error
	CreateAuthor(ctx context.Context, author *models.Author) error
	DeleteBookSeries(ctx context.Context, bookID int) error
	CreateBookSeries(ctx context.Context, bs *models.BookSeries) error
	FindOrCreateSeries(ctx context.Context, name string, libraryID int, nameSource string) (*models.Series, error)
	DeleteBookGenres(ctx context.Context, bookID int) error
	CreateBookGenre(ctx context.Context, bg *models.BookGenre) error
	DeleteBookTags(ctx context.Context, bookID int) error
	CreateBookTag(ctx context.Context, bt *models.BookTag) error
}

// identifierStore provides file identifier CRUD operations.
type identifierStore interface {
	DeleteIdentifiersForFile(ctx context.Context, fileID int) (int, error)
	CreateFileIdentifier(ctx context.Context, identifier *models.FileIdentifier) error
}

// personFinder finds or creates persons for author and narrator associations.
type personFinder interface {
	FindOrCreatePerson(ctx context.Context, name string, libraryID int) (*models.Person, error)
}

// genreFinder finds or creates genres.
type genreFinder interface {
	FindOrCreateGenre(ctx context.Context, name string, libraryID int) (*models.Genre, error)
}

// tagFinder finds or creates tags.
type tagFinder interface {
	FindOrCreateTag(ctx context.Context, name string, libraryID int) (*models.Tag, error)
}

// publisherFinder finds or creates publishers.
type publisherFinder interface {
	FindOrCreatePublisher(ctx context.Context, name string, libraryID int) (*models.Publisher, error)
}

// imprintFinder finds or creates imprints.
type imprintFinder interface {
	FindOrCreateImprint(ctx context.Context, name string, libraryID int) (*models.Imprint, error)
}

// searchIndexer updates the search index after metadata changes.
type searchIndexer interface {
	IndexBook(ctx context.Context, book *models.Book) error
}

type handler struct {
	service   *Service
	manager   *Manager
	installer *Installer
	db        *bun.DB
	enrich    *enrichDeps
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
	Enabled    *bool             `json:"enabled"`
	AutoUpdate *bool             `json:"auto_update"`
	Config     map[string]string `json:"config"`
}

type orderEntry struct {
	Scope string `json:"scope" validate:"required"`
	ID    string `json:"id" validate:"required"`
}

type setOrderPayload struct {
	Order []orderEntry `json:"order" validate:"required"`
}

type searchPayload struct {
	Query  string `json:"query" validate:"required"`
	BookID int    `json:"book_id" validate:"required"`
}

type enrichPayload struct {
	PluginScope  string `json:"plugin_scope" validate:"required"`
	PluginID     string `json:"plugin_id" validate:"required"`
	BookID       int    `json:"book_id" validate:"required"`
	FileID       *int   `json:"file_id"`
	ProviderData any    `json:"provider_data"`
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
			Status:      models.PluginStatusActive,
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
		downloadURL, sha256Hash, version, repoURL, imageURL, err := h.findPluginInRepos(c, payload.Scope, payload.ID, payload.Version)
		if err != nil {
			return errors.WithStack(err)
		}

		manifest, err := h.installer.InstallPlugin(ctx, payload.Scope, payload.ID, downloadURL, sha256Hash)
		if err != nil {
			return errors.WithStack(err)
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

			compatible := FilterCompatibleVersions(p.Versions)
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
		wasActive := plugin.Status == models.PluginStatusActive

		if *payload.Enabled && !wasActive {
			// Enabling: set Active and load the plugin
			plugin.Status = models.PluginStatusActive
			if err := h.manager.LoadPlugin(ctx, scope, id); err != nil {
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

	return errors.WithStack(c.JSON(http.StatusOK, plugin))
}

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
	Schema         ConfigSchema           `json:"schema"`
	Values         map[string]interface{} `json:"values"`
	DeclaredFields []string               `json:"declaredFields"`
	FieldSettings  map[string]bool        `json:"fieldSettings"`
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

	return c.JSON(http.StatusOK, pluginConfigResponse{
		Schema:         schema,
		Values:         values,
		DeclaredFields: declaredFields,
		FieldSettings:  fieldSettings,
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
			Status:      models.PluginStatusDisabled,
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

// setFieldSettingsPayload is the payload for setting field settings.
type setFieldSettingsPayload struct {
	Fields map[string]bool `json:"fields" validate:"required"`
}

// fieldSettingsResponse is the response for field settings endpoints.
type fieldSettingsResponse struct {
	Fields map[string]bool `json:"fields"`
}

// libraryFieldSettingsResponse is the response for library field settings endpoints.
type libraryFieldSettingsResponse struct {
	Fields     map[string]bool `json:"fields"`
	Customized bool            `json:"customized"`
}

func (h *handler) getFieldSettings(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")

	// Validate plugin exists and is a metadata enricher
	rt := h.manager.GetRuntime(scope, id)
	if rt == nil {
		return errcodes.NotFound("Plugin")
	}
	if rt.Manifest().Capabilities.MetadataEnricher == nil {
		// Not an enricher - return empty response
		return c.JSON(http.StatusOK, fieldSettingsResponse{Fields: nil})
	}

	settings, err := h.service.GetFieldSettings(ctx, scope, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, fieldSettingsResponse{
		Fields: settings,
	})
}

func (h *handler) setFieldSettings(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")

	// Validate plugin exists
	rt := h.manager.GetRuntime(scope, id)
	if rt == nil {
		return errcodes.NotFound("Plugin")
	}

	// Validate plugin is a metadata enricher
	enricherCap := rt.Manifest().Capabilities.MetadataEnricher
	if enricherCap == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin is not a metadata enricher")
	}

	var payload setFieldSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	if payload.Fields == nil {
		return errcodes.ValidationError("Fields are required.")
	}

	// Validate field names are declared by the plugin
	declared := make(map[string]bool, len(enricherCap.Fields))
	for _, f := range enricherCap.Fields {
		declared[f] = true
	}
	for field := range payload.Fields {
		if !declared[field] {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown field: "+field)
		}
	}

	for field, enabled := range payload.Fields {
		if err := h.service.SetFieldSetting(ctx, scope, id, field, enabled); err != nil {
			return errors.WithStack(err)
		}
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) getLibraryFieldSettings(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	scope := c.Param("scope")
	pluginID := c.Param("pluginId")

	// Validate plugin exists and is a metadata enricher
	rt := h.manager.GetRuntime(scope, pluginID)
	if rt == nil {
		return errcodes.NotFound("Plugin")
	}
	if rt.Manifest().Capabilities.MetadataEnricher == nil {
		// Not an enricher - return empty response
		return c.JSON(http.StatusOK, libraryFieldSettingsResponse{Fields: nil, Customized: false})
	}

	settings, err := h.service.GetLibraryFieldSettings(ctx, libraryID, scope, pluginID)
	if err != nil {
		return errors.WithStack(err)
	}

	customized := len(settings) > 0

	return c.JSON(http.StatusOK, libraryFieldSettingsResponse{
		Fields:     settings,
		Customized: customized,
	})
}

func (h *handler) setLibraryFieldSettings(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	scope := c.Param("scope")
	pluginID := c.Param("pluginId")

	// Validate plugin exists
	rt := h.manager.GetRuntime(scope, pluginID)
	if rt == nil {
		return errcodes.NotFound("Plugin")
	}

	// Validate plugin is a metadata enricher
	enricherCap := rt.Manifest().Capabilities.MetadataEnricher
	if enricherCap == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "plugin is not a metadata enricher")
	}

	var payload setFieldSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	if payload.Fields == nil {
		return errcodes.ValidationError("Fields are required.")
	}

	// Validate field names are declared by the plugin
	declared := make(map[string]bool, len(enricherCap.Fields))
	for _, f := range enricherCap.Fields {
		declared[f] = true
	}
	for field := range payload.Fields {
		if !declared[field] {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown field: "+field)
		}
	}

	for field, enabled := range payload.Fields {
		if err := h.service.SetLibraryFieldSetting(ctx, libraryID, scope, pluginID, field, enabled); err != nil {
			return errors.WithStack(err)
		}
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) resetLibraryFieldSettings(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	scope := c.Param("scope")
	pluginID := c.Param("pluginId")

	if err := h.service.ResetLibraryFieldSettings(ctx, libraryID, scope, pluginID); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// searchMetadata runs search() across all enabled enricher plugins and returns aggregated results.
func (h *handler) searchMetadata(c echo.Context) error {
	ctx := c.Request().Context()

	var payload searchPayload
	if err := c.Bind(&payload); err != nil {
		return errcodes.ValidationError(err.Error())
	}

	// Get all enricher runtimes first — if none exist, return empty results immediately
	runtimes, err := h.manager.GetOrderedRuntimes(ctx, models.PluginHookMetadataEnricher, 0)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(runtimes) == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"results": []SearchResult{},
		})
	}

	// Look up the book
	var book models.Book
	if err := h.db.NewSelect().Model(&book).
		Where("b.id = ?", payload.BookID).
		Relation("Authors").
		Relation("Authors.Person").
		Relation("BookSeries").
		Relation("BookSeries.Series").
		Relation("Files").
		Relation("Files.Identifiers").
		Scan(ctx); err != nil {
		return errcodes.NotFound("Book")
	}

	// Check library access
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}
	if !user.HasLibraryAccess(book.LibraryID) {
		return errcodes.Forbidden("You don't have access to this library")
	}

	// Build context objects
	bookCtx := buildSearchBookContext(&book)
	fileCtx := map[string]interface{}{} // Minimal file context for search
	if len(book.Files) > 0 {
		f := book.Files[0]
		fileCtx["fileType"] = f.FileType
		fileCtx["filePath"] = f.Filepath
	}

	var allResults []SearchResult
	for _, rt := range runtimes {
		searchCtx := map[string]interface{}{
			"query": payload.Query,
			"book":  bookCtx,
			"file":  fileCtx,
		}

		resp, sErr := h.manager.RunMetadataSearch(ctx, rt, searchCtx)
		if sErr != nil {
			continue // Skip failed plugins
		}
		if resp == nil {
			continue
		}

		// Compute disabled fields for this plugin
		var disabledFields []string
		manifest := rt.Manifest()
		if manifest.Capabilities.MetadataEnricher != nil {
			declaredFields := manifest.Capabilities.MetadataEnricher.Fields
			effectiveSettings, fErr := h.service.GetEffectiveFieldSettings(ctx, book.LibraryID, rt.Scope(), rt.PluginID(), declaredFields)
			if fErr == nil {
				for field, enabled := range effectiveSettings {
					if !enabled {
						disabledFields = append(disabledFields, field)
					}
				}
			}
		}

		for i := range resp.Results {
			resp.Results[i].DisabledFields = disabledFields
		}
		allResults = append(allResults, resp.Results...)
	}

	// Strip binary cover data from search results — covers are displayed via imageUrl
	for i := range allResults {
		if allResults[i].Metadata != nil {
			allResults[i].Metadata.CoverData = nil
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"results": allResults,
	})
}

// enrichMetadata runs enrich() on a specific plugin with a selected search result,
// then applies the returned metadata to the book.
func (h *handler) enrichMetadata(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	var payload enrichPayload
	if err := c.Bind(&payload); err != nil {
		return errcodes.ValidationError(err.Error())
	}

	// Get the specific plugin runtime
	rt := h.manager.GetRuntime(payload.PluginScope, payload.PluginID)
	if rt == nil {
		return errcodes.NotFound("Plugin")
	}

	// Look up the book with all relations needed for enrichment
	var book models.Book
	if err := h.db.NewSelect().Model(&book).
		Where("b.id = ?", payload.BookID).
		Relation("Authors").
		Relation("Authors.Person").
		Relation("BookSeries").
		Relation("BookSeries.Series").
		Relation("BookGenres").
		Relation("BookGenres.Genre").
		Relation("BookTags").
		Relation("BookTags.Tag").
		Relation("Files").
		Relation("Files.Identifiers").
		Scan(ctx); err != nil {
		return errcodes.NotFound("Book")
	}

	// Check library access
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}
	if !user.HasLibraryAccess(book.LibraryID) {
		return errcodes.Forbidden("You don't have access to this library")
	}

	// Resolve the target file for enrichment
	var targetFile *models.File
	if payload.FileID != nil {
		for _, f := range book.Files {
			if f.ID == *payload.FileID {
				targetFile = f
				break
			}
		}
		if targetFile == nil {
			return errcodes.NotFound("File")
		}
	} else if len(book.Files) > 0 {
		targetFile = book.Files[0]
	}

	bookCtx := buildSearchBookContext(&book)
	fileCtx := map[string]interface{}{}
	if targetFile != nil {
		fileCtx["fileType"] = targetFile.FileType
		fileCtx["filePath"] = targetFile.Filepath
	}

	enrichCtx := map[string]interface{}{
		"selectedResult": payload.ProviderData,
		"book":           bookCtx,
		"file":           fileCtx,
	}

	result, err := h.manager.RunMetadataEnrich(ctx, rt, enrichCtx)
	if err != nil {
		return errors.WithStack(err)
	}

	// If the plugin didn't modify anything or we don't have persistence deps, return as-is
	if !result.Modified || result.Metadata == nil || h.enrich == nil {
		return c.JSON(http.StatusOK, result)
	}

	// Apply enriched metadata to the book
	if err := h.applyEnrichment(ctx, &book, targetFile, result.Metadata, rt, log); err != nil {
		return errors.WithStack(err)
	}

	// Reload the book with all relations to return the updated state
	updatedBook, err := h.enrich.bookStore.RetrieveBook(ctx, book.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, updatedBook)
}

// DownloadCoverFromURL fetches a cover image from a URL and populates md.CoverData and md.CoverMimeType.
// Returns true if the download succeeded, false otherwise. Skips if CoverData is already set (precedence rule).
// The URL's domain (and any redirect domains) must be in the plugin's httpAccess.domains allowlist.
func DownloadCoverFromURL(ctx context.Context, md *mediafile.ParsedMetadata, allowedDomains []string, log logger.Logger) bool {
	if len(md.CoverData) > 0 || md.CoverURL == "" {
		return false
	}

	// Validate the cover URL domain against the plugin's allowed domains
	parsedURL, err := url.Parse(md.CoverURL)
	if err != nil {
		log.Warn("failed to parse cover URL", logger.Data{"url": md.CoverURL, "error": err.Error()})
		return false
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		log.Warn("cover URL uses unsupported scheme", logger.Data{"url": md.CoverURL, "scheme": parsedURL.Scheme})
		return false
	}
	if err := validateDomain(parsedURL.Host, allowedDomains); err != nil {
		log.Warn("cover URL domain not in plugin's httpAccess.domains", logger.Data{"url": md.CoverURL, "error": err.Error()})
		return false
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if err := validateDomain(req.URL.Host, allowedDomains); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, md.CoverURL, nil)
	if err != nil {
		log.Warn("failed to create cover download request", logger.Data{"url": md.CoverURL, "error": err.Error()})
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Warn("failed to download cover from URL", logger.Data{"url": md.CoverURL, "error": err.Error()})
		return false
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(contentType, "image/") {
		log.Warn("cover URL returned non-image response", logger.Data{
			"url":          md.CoverURL,
			"status":       resp.StatusCode,
			"content_type": contentType,
		})
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10 MB max
	if err != nil {
		log.Warn("failed to read cover response body", logger.Data{"url": md.CoverURL, "error": err.Error()})
		return false
	}

	md.CoverData = body
	md.CoverMimeType = contentType
	return true
}

// applyEnrichment applies enriched metadata to a book, respecting field filtering.
// targetFile is the specific file to apply file-level metadata (identifiers, cover) to; may be nil.
func (h *handler) applyEnrichment(ctx context.Context, book *models.Book, targetFile *models.File, md *mediafile.ParsedMetadata, rt *Runtime, log logger.Logger) error {
	manifest := rt.Manifest()
	if manifest.Capabilities.MetadataEnricher == nil {
		return nil
	}

	// Get declared and effective field settings
	declaredFields := manifest.Capabilities.MetadataEnricher.Fields
	enabledFields, err := h.service.GetEffectiveFieldSettings(ctx, book.LibraryID, rt.Scope(), rt.PluginID(), declaredFields)
	if err != nil {
		log.Warn("failed to get field settings, using all enabled", logger.Data{"error": err.Error()})
		enabledFields = make(map[string]bool, len(declaredFields))
		for _, f := range declaredFields {
			enabledFields[f] = true
		}
	}

	// Build a set of declared fields
	declared := make(map[string]bool, len(declaredFields))
	for _, f := range declaredFields {
		declared[f] = true
	}

	// Helper: check if a field is both declared and enabled
	isAllowed := func(field string) bool {
		if !declared[field] {
			return false
		}
		if enabled, ok := enabledFields[field]; ok {
			return enabled
		}
		return true // Default: enabled
	}

	pluginSource := models.PluginDataSource(rt.Scope(), rt.PluginID())
	var columns []string

	// Title
	title := strings.TrimSpace(md.Title)
	if title != "" && isAllowed("title") {
		book.Title = title
		book.TitleSource = pluginSource
		book.SortTitle = sortname.ForTitle(title)
		book.SortTitleSource = pluginSource
		columns = append(columns, "title", "title_source", "sort_title", "sort_title_source")
	}

	// Subtitle
	if md.Subtitle != "" && isAllowed("subtitle") {
		subtitle := strings.TrimSpace(md.Subtitle)
		book.Subtitle = &subtitle
		book.SubtitleSource = &pluginSource
		columns = append(columns, "subtitle", "subtitle_source")
	}

	// Description
	if md.Description != "" && isAllowed("description") {
		desc := htmlutil.StripTags(strings.TrimSpace(md.Description))
		if desc != "" {
			book.Description = &desc
			book.DescriptionSource = &pluginSource
			columns = append(columns, "description", "description_source")
		}
	}

	// Apply scalar column updates
	if len(columns) > 0 {
		if err := h.enrich.bookStore.UpdateBook(ctx, book, columns); err != nil {
			return errors.Wrap(err, "failed to update book")
		}
	}

	// Authors
	if len(md.Authors) > 0 && isAllowed("authors") && h.enrich.personFinder != nil {
		if err := h.enrich.relStore.DeleteAuthors(ctx, book.ID); err != nil {
			return errors.Wrap(err, "failed to delete authors")
		}
		for i, pa := range md.Authors {
			if pa.Name == "" {
				continue
			}
			person, pErr := h.enrich.personFinder.FindOrCreatePerson(ctx, pa.Name, book.LibraryID)
			if pErr != nil {
				log.Warn("failed to find/create person", logger.Data{"name": pa.Name, "error": pErr.Error()})
				continue
			}
			var role *string
			if pa.Role != "" {
				role = &pa.Role
			}
			if err := h.enrich.relStore.CreateAuthor(ctx, &models.Author{
				BookID:    book.ID,
				PersonID:  person.ID,
				Role:      role,
				SortOrder: i + 1,
			}); err != nil {
				log.Warn("failed to create author", logger.Data{"error": err.Error()})
			}
		}
		book.AuthorSource = pluginSource
		if err := h.enrich.bookStore.UpdateBook(ctx, book, []string{"author_source"}); err != nil {
			return errors.Wrap(err, "failed to update author source")
		}
	}

	// Series
	seriesAllowed := isAllowed("series") || isAllowed("seriesNumber")
	if md.Series != "" && seriesAllowed {
		if err := h.enrich.relStore.DeleteBookSeries(ctx, book.ID); err != nil {
			return errors.Wrap(err, "failed to delete series")
		}
		seriesRecord, sErr := h.enrich.relStore.FindOrCreateSeries(ctx, md.Series, book.LibraryID, pluginSource)
		if sErr != nil {
			log.Warn("failed to find/create series", logger.Data{"name": md.Series, "error": sErr.Error()})
		} else {
			if err := h.enrich.relStore.CreateBookSeries(ctx, &models.BookSeries{
				BookID:       book.ID,
				SeriesID:     seriesRecord.ID,
				SeriesNumber: md.SeriesNumber,
				SortOrder:    1,
			}); err != nil {
				log.Warn("failed to create book series", logger.Data{"error": err.Error()})
			}
		}
	}

	// Genres
	if len(md.Genres) > 0 && isAllowed("genres") && h.enrich.genreFinder != nil {
		if err := h.enrich.relStore.DeleteBookGenres(ctx, book.ID); err != nil {
			return errors.Wrap(err, "failed to delete genres")
		}
		for _, genreName := range md.Genres {
			if genreName == "" {
				continue
			}
			genre, gErr := h.enrich.genreFinder.FindOrCreateGenre(ctx, genreName, book.LibraryID)
			if gErr != nil {
				log.Warn("failed to find/create genre", logger.Data{"genre": genreName, "error": gErr.Error()})
				continue
			}
			if err := h.enrich.relStore.CreateBookGenre(ctx, &models.BookGenre{
				BookID:  book.ID,
				GenreID: genre.ID,
			}); err != nil {
				log.Warn("failed to create book genre", logger.Data{"error": err.Error()})
			}
		}
		book.GenreSource = &pluginSource
		if err := h.enrich.bookStore.UpdateBook(ctx, book, []string{"genre_source"}); err != nil {
			return errors.Wrap(err, "failed to update genre source")
		}
	}

	// Tags
	if len(md.Tags) > 0 && isAllowed("tags") && h.enrich.tagFinder != nil {
		if err := h.enrich.relStore.DeleteBookTags(ctx, book.ID); err != nil {
			return errors.Wrap(err, "failed to delete tags")
		}
		for _, tagName := range md.Tags {
			if tagName == "" {
				continue
			}
			tag, tErr := h.enrich.tagFinder.FindOrCreateTag(ctx, tagName, book.LibraryID)
			if tErr != nil {
				log.Warn("failed to find/create tag", logger.Data{"tag": tagName, "error": tErr.Error()})
				continue
			}
			if err := h.enrich.relStore.CreateBookTag(ctx, &models.BookTag{
				BookID: book.ID,
				TagID:  tag.ID,
			}); err != nil {
				log.Warn("failed to create book tag", logger.Data{"error": err.Error()})
			}
		}
		book.TagSource = &pluginSource
		if err := h.enrich.bookStore.UpdateBook(ctx, book, []string{"tag_source"}); err != nil {
			return errors.Wrap(err, "failed to update tag source")
		}
	}

	// File-level metadata: accumulate column updates and flush once at the end
	var fileColumns []string

	// Narrators (file-level, applied to target file)
	if len(md.Narrators) > 0 && isAllowed("narrators") && targetFile != nil && h.enrich.personFinder != nil {
		if _, err := h.enrich.bookStore.DeleteNarratorsForFile(ctx, targetFile.ID); err != nil {
			return errors.Wrap(err, "failed to delete narrators")
		}
		for i, narratorName := range md.Narrators {
			if narratorName == "" {
				continue
			}
			person, pErr := h.enrich.personFinder.FindOrCreatePerson(ctx, narratorName, book.LibraryID)
			if pErr != nil {
				log.Warn("failed to find/create person for narrator", logger.Data{"name": narratorName, "error": pErr.Error()})
				continue
			}
			if err := h.enrich.bookStore.CreateNarrator(ctx, &models.Narrator{
				FileID:    targetFile.ID,
				PersonID:  person.ID,
				SortOrder: i + 1,
			}); err != nil {
				log.Warn("failed to create narrator", logger.Data{"error": err.Error()})
			}
		}
		targetFile.NarratorSource = &pluginSource
		fileColumns = append(fileColumns, "narrator_source")
	}

	// Publisher (file-level, applied to target file)
	if md.Publisher != "" && isAllowed("publisher") && targetFile != nil && h.enrich.publisherFinder != nil {
		publisher, pErr := h.enrich.publisherFinder.FindOrCreatePublisher(ctx, md.Publisher, book.LibraryID)
		if pErr != nil {
			log.Warn("failed to find/create publisher", logger.Data{"name": md.Publisher, "error": pErr.Error()})
		} else {
			targetFile.PublisherID = &publisher.ID
			targetFile.PublisherSource = &pluginSource
			fileColumns = append(fileColumns, "publisher_id", "publisher_source")
		}
	}

	// Imprint (file-level, applied to target file)
	if md.Imprint != "" && isAllowed("imprint") && targetFile != nil && h.enrich.imprintFinder != nil {
		imprint, iErr := h.enrich.imprintFinder.FindOrCreateImprint(ctx, md.Imprint, book.LibraryID)
		if iErr != nil {
			log.Warn("failed to find/create imprint", logger.Data{"name": md.Imprint, "error": iErr.Error()})
		} else {
			targetFile.ImprintID = &imprint.ID
			targetFile.ImprintSource = &pluginSource
			fileColumns = append(fileColumns, "imprint_id", "imprint_source")
		}
	}

	// URL (file-level, applied to target file)
	if md.URL != "" && isAllowed("url") && targetFile != nil {
		targetFile.URL = &md.URL
		targetFile.URLSource = &pluginSource
		fileColumns = append(fileColumns, "url", "url_source")
	}

	// Release date (file-level, applied to target file)
	if md.ReleaseDate != nil && isAllowed("releaseDate") && targetFile != nil {
		targetFile.ReleaseDate = md.ReleaseDate
		targetFile.ReleaseDateSource = &pluginSource
		fileColumns = append(fileColumns, "release_date", "release_date_source")
	}

	// Identifiers (file-level, applied to target file)
	if len(md.Identifiers) > 0 && isAllowed("identifiers") && targetFile != nil {
		if _, err := h.enrich.identStore.DeleteIdentifiersForFile(ctx, targetFile.ID); err != nil {
			return errors.Wrap(err, "failed to delete identifiers")
		}
		for _, ident := range md.Identifiers {
			if ident.Type == "" || ident.Value == "" {
				continue
			}
			if err := h.enrich.identStore.CreateFileIdentifier(ctx, &models.FileIdentifier{
				FileID: targetFile.ID,
				Type:   ident.Type,
				Value:  ident.Value,
				Source: pluginSource,
			}); err != nil {
				log.Warn("failed to create identifier", logger.Data{"error": err.Error()})
			}
		}
	}

	// Cover image: download from URL if coverData is empty and coverUrl is set
	if md.CoverURL != "" && isAllowed("cover") && targetFile != nil {
		var allowedDomains []string
		if manifest.Capabilities.HTTPAccess != nil {
			allowedDomains = manifest.Capabilities.HTTPAccess.Domains
		}
		DownloadCoverFromURL(ctx, md, allowedDomains, log)
	}

	// Apply cover data (from coverData or downloaded from coverUrl)
	if len(md.CoverData) > 0 && isAllowed("cover") && targetFile != nil {
		coverDir := book.Filepath
		coverBaseName := filepath.Base(targetFile.Filepath) + ".cover"

		// Normalize the cover image
		normalizedData, normalizedMime, _ := fileutils.NormalizeImage(md.CoverData, md.CoverMimeType)
		coverExt := ".png"
		if normalizedMime == md.CoverMimeType {
			coverExt = md.CoverExtension()
		}

		coverFilename := coverBaseName + coverExt
		coverFilepath := filepath.Join(coverDir, coverFilename)

		if err := os.WriteFile(coverFilepath, normalizedData, 0600); err != nil {
			log.Warn("failed to write cover file", logger.Data{"error": err.Error()})
		} else {
			targetFile.CoverImageFilename = &coverFilename
			fileColumns = append(fileColumns, "cover_image_filename")
		}
	}

	// Flush all file-level column updates in a single DB call
	if len(fileColumns) > 0 && targetFile != nil {
		if err := h.enrich.bookStore.UpdateFile(ctx, targetFile, fileColumns); err != nil {
			return errors.Wrap(err, "failed to update file metadata")
		}
	}

	// Write sidecars to keep them in sync
	updatedBook, err := h.enrich.bookStore.RetrieveBook(ctx, book.ID)
	if err == nil {
		if sErr := sidecar.WriteBookSidecarFromModel(updatedBook); sErr != nil {
			log.Warn("failed to write book sidecar", logger.Data{"error": sErr.Error()})
		}
		for _, file := range updatedBook.Files {
			if sErr := sidecar.WriteFileSidecarFromModel(file); sErr != nil {
				log.Warn("failed to write file sidecar", logger.Data{"file_id": file.ID, "error": sErr.Error()})
			}
		}
	}

	// Update FTS index
	if h.enrich.searchIndexer != nil && updatedBook != nil {
		if err := h.enrich.searchIndexer.IndexBook(ctx, updatedBook); err != nil {
			log.Warn("failed to update search index", logger.Data{"error": err.Error()})
		}
	}

	return nil
}

// buildSearchBookContext builds a context map for search/enrich from a book model.
func buildSearchBookContext(book *models.Book) map[string]interface{} {
	ctx := map[string]interface{}{
		"id":    book.ID,
		"title": book.Title,
	}

	if book.Subtitle != nil {
		ctx["subtitle"] = *book.Subtitle
	}
	if book.Description != nil {
		ctx["description"] = *book.Description
	}

	if len(book.Authors) > 0 {
		authors := make([]map[string]interface{}, 0, len(book.Authors))
		for _, a := range book.Authors {
			author := map[string]interface{}{}
			if a.Person != nil {
				author["name"] = a.Person.Name
			}
			if a.Role != nil {
				author["role"] = *a.Role
			}
			authors = append(authors, author)
		}
		ctx["authors"] = authors
	}

	if len(book.BookSeries) > 0 {
		series := make([]map[string]interface{}, 0, len(book.BookSeries))
		for _, bs := range book.BookSeries {
			if bs.Series == nil {
				continue
			}
			s := map[string]interface{}{
				"name": bs.Series.Name,
			}
			if bs.SeriesNumber != nil {
				s["number"] = *bs.SeriesNumber
			}
			series = append(series, s)
		}
		if len(series) > 0 {
			ctx["series"] = series
		}
	}

	// Collect identifiers from all files
	var identifiers []map[string]interface{}
	for _, f := range book.Files {
		for _, id := range f.Identifiers {
			identifiers = append(identifiers, map[string]interface{}{
				"type":  id.Type,
				"value": id.Value,
			})
		}
	}
	if len(identifiers) > 0 {
		ctx["identifiers"] = identifiers
	}

	return ctx
}

// NewHandler creates a handler for testing and external route registration.
func NewHandler(service *Service, manager *Manager, installer *Installer) *handler { //nolint:revive // unexported return is intentional for same-package tests
	return &handler{service: service, manager: manager, installer: installer}
}

// Exported handler methods for testing.
func (h *handler) GetImage(c echo.Context) error              { return h.getImage(c) }
func (h *handler) GetLibraryOrder(c echo.Context) error       { return h.getLibraryOrder(c) }
func (h *handler) SetLibraryOrder(c echo.Context) error       { return h.setLibraryOrder(c) }
func (h *handler) ResetLibraryOrder(c echo.Context) error     { return h.resetLibraryOrder(c) }
func (h *handler) ResetAllLibraryOrders(c echo.Context) error { return h.resetAllLibraryOrders(c) }
