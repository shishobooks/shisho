package plugins

import (
	"context"
	"database/sql"
	stderrors "errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/models"
	pkgversion "github.com/shishobooks/shisho/pkg/version"
)

// Manager holds loaded Runtime instances indexed by "scope/id".
// It coordinates loading at startup, unloading, and hot-reloading on install/update/enable.
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]*Runtime // key: "scope/id"
	// locks holds one *sync.Mutex per plugin key, taken by every lifecycle
	// transition (see lockPlugin). The zero value is ready to use. Entries
	// are never removed: one small mutex per plugin id ever seen is cheaper
	// than coordinating deletion with a waiter.
	locks sync.Map
	// onLockWait, when set, is called by lockPlugin just before it blocks
	// on a lock another transition holds. Tests use it to know a call is
	// waiting.
	onLockWait    func(key string)
	service       *Service
	pluginDir     string
	pluginDataDir string // Base directory for persistent plugin data

	// fetchRepo is the function used to fetch repository manifests.
	// Defaults to FetchRepository; tests can override this.
	fetchRepo func(url string) (*RepositoryManifest, error)

	// onEvent is called when a plugin lifecycle event occurs.
	// Set via SetEventCallback. May be nil.
	onEvent PluginEventCallback
}

// NewManager creates a new Manager.
func NewManager(service *Service, pluginDir, pluginDataDir string) *Manager {
	return &Manager{
		plugins:       make(map[string]*Runtime),
		service:       service,
		pluginDir:     pluginDir,
		pluginDataDir: pluginDataDir,
		fetchRepo:     FetchRepository,
	}
}

// SetEventCallback registers a callback for plugin lifecycle events.
// Only one callback is supported; subsequent calls replace the previous one.
func (m *Manager) SetEventCallback(cb PluginEventCallback) {
	m.mu.Lock()
	m.onEvent = cb
	m.mu.Unlock()
}

// emitEvent fires a plugin lifecycle event if a callback is registered.
func (m *Manager) emitEvent(eventType PluginEventType, scope, id string, hooks []string) {
	m.mu.RLock()
	cb := m.onEvent
	m.mu.RUnlock()

	if cb != nil {
		cb(PluginEvent{
			Type:  eventType,
			Scope: scope,
			ID:    id,
			Hooks: hooks,
		})
	}
}

// pluginKey returns the map key for a plugin.
func pluginKey(scope, id string) string {
	return scope + "/" + id
}

// LoadError is a load failure caused by the plugin itself: its manifest, its
// script, or its host version requirement. Injecting the host APIs is the
// server's side of loading, so its failure is not a LoadError. Install, enable, reload, and
// startup record it on the plugin and mark the plugin Malfunctioned or Not
// Supported. Any other error from LoadPlugin or ReloadPlugin, such as a
// database fault, is a server fault that leaves the plugin's state alone.
type LoadError struct {
	Err error
}

func (e *LoadError) Error() string { return e.Err.Error() }

func (e *LoadError) Unwrap() error { return e.Err }

// asLoadError reports whether err is a LoadError.
func asLoadError(err error) bool {
	var loadErr *LoadError
	return stderrors.As(err, &loadErr)
}

// isVersionIncompatible checks if the error (or any wrapped error) is ErrVersionIncompatible.
func isVersionIncompatible(err error) bool {
	var vErr *ErrVersionIncompatible
	return stderrors.As(err, &vErr)
}

// LoadAll loads all enabled plugins from the database at startup, after
// removing the staging and trash directories an interrupted transition
// left behind. A plugin that fails to load is marked Malfunctioned or Not
// Supported and doesn't prevent other plugins from loading. Each loaded
// plugin is reconciled through applyVersion, which repairs its derived
// rows.
func (m *Manager) LoadAll(ctx context.Context) error {
	sweepTransitDirs(m.pluginDir)

	plugins, err := m.service.ListPlugins(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list plugins")
	}

	for _, p := range plugins {
		if p.Status != models.PluginStatusActive {
			continue
		}
		m.loadAtStartup(ctx, p.Scope, p.ID)
	}

	return nil
}

// loadAtStartup loads one plugin for LoadAll, logging instead of failing.
func (m *Manager) loadAtStartup(ctx context.Context, scope, id string) {
	unlock := m.lockPlugin(scope, id)
	defer unlock()

	row, err := m.service.RetrievePlugin(ctx, scope, id)
	if err != nil || row.Status != models.PluginStatusActive {
		return
	}
	loadErr, err := m.enableLocked(ctx, row)
	if err == nil {
		// A load failure the plugin caused is already stored on its row.
		err = loadErr
	}
	// A server fault says nothing about the plugin, so enableLocked left its
	// stored state as it is.
	if err != nil {
		logger.New().Warn("failed to load plugin", logger.Data{"plugin": pluginKey(scope, id), "error": err.Error()})
	}
}

// LoadPlugin loads a plugin from its directory, marks it Active, reconciles
// its derived rows, and registers it. It is enable without the handler: a
// *LoadError is returned without changing the row, so the caller can record
// it. Any other error is a server fault and leaves the plugin unloaded.
func (m *Manager) LoadPlugin(ctx context.Context, scope, id string) error {
	unlock := m.lockPlugin(scope, id)
	defer unlock()

	row, err := m.service.RetrievePlugin(ctx, scope, id)
	if err != nil {
		return err
	}
	rt, err := m.loadRuntime(m.liveDir(scope, id), scope, id)
	if err != nil {
		return err
	}
	row.Status = models.PluginStatusActive
	row.LoadError = nil
	return m.applyVersion(ctx, row, rt, false)
}

// UnloadPlugin unregisters a plugin's runtime once hooks in progress on it
// finish. The plugin's row is not changed.
func (m *Manager) UnloadPlugin(scope, id string) {
	unlock := m.lockPlugin(scope, id)
	defer unlock()
	m.setRuntime(scope, id, nil)
}

// DeletePluginData removes the persistent data directory for a plugin.
// Errors are logged but not returned since this is a best-effort cleanup.
func (m *Manager) DeletePluginData(scope, id string) {
	dataDir := filepath.Join(m.pluginDataDir, scope, id)
	if err := os.RemoveAll(dataDir); err != nil {
		log := logger.New()
		log.Warn("failed to delete plugin data directory", logger.Data{
			"plugin": pluginKey(scope, id),
			"path":   dataDir,
			"error":  err.Error(),
		})
	}
}

// ReloadPlugin loads an Active plugin again from its directory and swaps
// the new runtime in once hooks in progress on the old one finish. A
// *LoadError leaves the row and the old runtime as they were.
func (m *Manager) ReloadPlugin(ctx context.Context, scope, id string) error {
	unlock := m.lockPlugin(scope, id)
	defer unlock()

	row, err := m.service.RetrievePlugin(ctx, scope, id)
	if err != nil {
		return err
	}
	rt, err := m.loadRuntime(m.liveDir(scope, id), scope, id)
	if err != nil {
		return err
	}
	row.LoadError = nil
	return m.applyVersion(ctx, row, rt, false)
}

// GetRuntime returns the runtime for a plugin (nil if not loaded).
func (m *Manager) GetRuntime(scope, id string) *Runtime {
	key := pluginKey(scope, id)
	m.mu.RLock()
	rt := m.plugins[key]
	m.mu.RUnlock()
	return rt
}

// GetOrderedRuntimes returns runtimes for a hook type in user-defined order.
// If libraryID > 0 and the library has customized the order for this hook type,
// uses the per-library order (only enabled entries). Otherwise falls back to global order.
// In both paths, only runtimes that are actually loaded (globally enabled) are returned.
func (m *Manager) GetOrderedRuntimes(ctx context.Context, hookType string, libraryID int) ([]*Runtime, error) {
	if libraryID > 0 {
		customized, err := m.service.IsLibraryCustomized(ctx, libraryID, hookType)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check library customization for hook type %s", hookType)
		}
		if customized {
			entries, err := m.service.GetLibraryOrder(ctx, libraryID, hookType)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get library order for hook type %s", hookType)
			}
			var runtimes []*Runtime
			m.mu.RLock()
			for _, entry := range entries {
				if entry.Mode != models.PluginModeEnabled {
					continue
				}
				key := pluginKey(entry.Scope, entry.PluginID)
				if rt, ok := m.plugins[key]; ok {
					runtimes = append(runtimes, rt)
				}
			}
			m.mu.RUnlock()
			return runtimes, nil
		}
	}

	// Fall back to global order
	orders, err := m.service.GetOrder(ctx, hookType)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get order for hook type %s", hookType)
	}

	var runtimes []*Runtime
	m.mu.RLock()
	for _, order := range orders {
		if order.Mode != models.PluginModeEnabled {
			continue
		}
		key := pluginKey(order.Scope, order.PluginID)
		if rt, ok := m.plugins[key]; ok {
			runtimes = append(runtimes, rt)
		}
	}
	m.mu.RUnlock()

	return runtimes, nil
}

// GetManualRuntimes returns runtimes for a hook type that are available for manual invocation.
// Both "enabled" and "manual_only" plugins are returned; only "disabled" are excluded.
// If libraryID > 0 and the library has customized the order for this hook type,
// uses the per-library order. Otherwise falls back to global order.
func (m *Manager) GetManualRuntimes(ctx context.Context, hookType string, libraryID int) ([]*Runtime, error) {
	if libraryID > 0 {
		customized, err := m.service.IsLibraryCustomized(ctx, libraryID, hookType)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check library customization for hook type %s", hookType)
		}
		if customized {
			entries, err := m.service.GetLibraryOrder(ctx, libraryID, hookType)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get library order for hook type %s", hookType)
			}
			var runtimes []*Runtime
			m.mu.RLock()
			for _, entry := range entries {
				if entry.Mode == models.PluginModeDisabled {
					continue
				}
				key := pluginKey(entry.Scope, entry.PluginID)
				if rt, ok := m.plugins[key]; ok {
					runtimes = append(runtimes, rt)
				}
			}
			m.mu.RUnlock()
			return runtimes, nil
		}
	}

	// Fall back to global order
	orders, err := m.service.GetOrder(ctx, hookType)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get order for hook type %s", hookType)
	}

	var runtimes []*Runtime
	m.mu.RLock()
	for _, order := range orders {
		if order.Mode == models.PluginModeDisabled {
			continue
		}
		key := pluginKey(order.Scope, order.PluginID)
		if rt, ok := m.plugins[key]; ok {
			runtimes = append(runtimes, rt)
		}
	}
	m.mu.RUnlock()

	return runtimes, nil
}

// GetParserForType returns the first loaded runtime that has a fileParser for the given type.
func (m *Manager) GetParserForType(fileType string) *Runtime {
	// Built-in file types are reserved
	if models.IsBuiltInFileExtension(fileType) {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rt := range m.plugins {
		if rt.manifest.Capabilities.FileParser == nil {
			continue
		}
		for _, t := range rt.manifest.Capabilities.FileParser.Types {
			if t == fileType {
				return rt
			}
		}
	}
	return nil
}

// RegisteredFileExtensions returns all file extensions registered by plugin fileParsers,
// excluding the extensions of built-in file types, which plugins cannot claim.
func (m *Manager) RegisteredFileExtensions() map[string]struct{} {
	result := make(map[string]struct{})

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rt := range m.plugins {
		if rt.manifest.Capabilities.FileParser == nil {
			continue
		}
		for _, t := range rt.manifest.Capabilities.FileParser.Types {
			if !models.IsBuiltInFileExtension(t) {
				result[t] = struct{}{}
			}
		}
	}
	return result
}

// GetOutputGenerator returns the PluginGenerator for a given format ID.
// Returns nil if no plugin provides that format.
func (m *Manager) GetOutputGenerator(formatID string) *PluginGenerator {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rt := range m.plugins {
		if rt.manifest.Capabilities.OutputGenerator == nil {
			continue
		}
		if rt.manifest.Capabilities.OutputGenerator.ID == formatID {
			return NewPluginGenerator(m, rt.scope, rt.pluginID, formatID)
		}
	}
	return nil
}

// RegisteredOutputFormats returns all format IDs registered by plugin output generators,
// along with their source type restrictions.
func (m *Manager) RegisteredOutputFormats() []OutputFormatInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var formats []OutputFormatInfo
	for _, rt := range m.plugins {
		if rt.manifest.Capabilities.OutputGenerator == nil {
			continue
		}
		outGen := rt.manifest.Capabilities.OutputGenerator
		formats = append(formats, OutputFormatInfo{
			ID:          outGen.ID,
			Name:        outGen.Name,
			SourceTypes: outGen.SourceTypes,
			Scope:       rt.scope,
			PluginID:    rt.pluginID,
		})
	}
	return formats
}

// RegisteredConverterExtensions returns source extensions that have input converters.
func (m *Manager) RegisteredConverterExtensions() map[string]struct{} {
	result := make(map[string]struct{})

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rt := range m.plugins {
		if rt.manifest.Capabilities.InputConverter == nil {
			continue
		}
		for _, t := range rt.manifest.Capabilities.InputConverter.SourceTypes {
			result[t] = struct{}{}
		}
	}
	return result
}

// scopedManifest pairs a repository manifest with the scope it was fetched
// for, so refreshPluginUpdateVersion can skip manifests whose scope doesn't
// match the plugin being evaluated.
type scopedManifest struct {
	scope    string
	manifest *RepositoryManifest
}

// CheckForUpdates checks all installed plugins against enabled repositories
// for available updates. It sets or clears UpdateAvailableVersion on each plugin.
func (m *Manager) CheckForUpdates(ctx context.Context) error {
	log := logger.New()

	installedPlugins, err := m.service.ListPlugins(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list plugins for update check")
	}

	if len(installedPlugins) == 0 {
		return nil
	}

	repos, err := m.service.ListRepositories(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list repositories for update check")
	}

	var manifests []scopedManifest
	for _, repo := range repos {
		if !repo.Enabled {
			continue
		}
		manifest, err := m.fetchRepo(repo.URL)
		if err != nil {
			log.Warn("failed to fetch repository for update check", logger.Data{
				"url":   repo.URL,
				"scope": repo.Scope,
				"error": err.Error(),
			})
			continue
		}
		manifests = append(manifests, scopedManifest{scope: repo.Scope, manifest: manifest})
	}

	for _, plugin := range installedPlugins {
		if !plugin.AutoUpdate {
			continue
		}
		if err := m.refreshPluginUpdateVersion(ctx, plugin, manifests); err != nil {
			log.Warn("failed to update plugin update_available_version", logger.Data{
				"plugin": pluginKey(plugin.Scope, plugin.ID),
				"error":  err.Error(),
			})
		}
	}

	return nil
}

// CheckForUpdatesForRepo re-evaluates UpdateAvailableVersion for installed
// plugins belonging to the given scope using an already-fetched manifest. Used
// by the sync-repo handler to avoid re-fetching every enabled repository when
// a single repo is synced. Plugins in other scopes are left untouched — they
// can't have changed since we only synced one repo.
func (m *Manager) CheckForUpdatesForRepo(ctx context.Context, scope string, manifest *RepositoryManifest) error {
	log := logger.New()

	installedPlugins, err := m.service.ListPlugins(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list plugins for update check")
	}

	manifests := []scopedManifest{{scope: scope, manifest: manifest}}
	for _, plugin := range installedPlugins {
		if plugin.Scope != scope {
			continue
		}
		if !plugin.AutoUpdate {
			continue
		}
		if err := m.refreshPluginUpdateVersion(ctx, plugin, manifests); err != nil {
			log.Warn("failed to update plugin update_available_version", logger.Data{
				"plugin": pluginKey(plugin.Scope, plugin.ID),
				"error":  err.Error(),
			})
		}
	}

	return nil
}

// refreshPluginUpdateVersion finds the newest compatible version of the plugin
// across the provided manifests and persists UpdateAvailableVersion if the
// computed value differs from the current one. If no newer version is found,
// the field is cleared. It holds the plugin's lock and re-reads the row, so
// the version it compares is current, and it writes only
// update_available_version, so a transition that landed while the
// repositories were being fetched is kept.
func (m *Manager) refreshPluginUpdateVersion(ctx context.Context, listed *models.Plugin, manifests []scopedManifest) error {
	unlock := m.lockPlugin(listed.Scope, listed.ID)
	defer unlock()

	plugin, err := m.service.RetrievePlugin(ctx, listed.Scope, listed.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !plugin.AutoUpdate {
		// Turned off while the repositories were being fetched.
		return nil
	}

	latestVersion := latestAvailableVersion(plugin, manifests)

	if latestVersion != "" {
		if plugin.UpdateAvailableVersion != nil && *plugin.UpdateAvailableVersion == latestVersion {
			return nil
		}
		plugin.UpdateAvailableVersion = &latestVersion
	} else {
		if plugin.UpdateAvailableVersion == nil {
			return nil
		}
		plugin.UpdateAvailableVersion = nil
	}

	return m.service.UpdatePluginColumns(ctx, plugin, "update_available_version")
}

// latestAvailableVersion returns the newest compatible version of plugin
// listed in manifests that is newer than its installed version, or "".
func latestAvailableVersion(plugin *models.Plugin, manifests []scopedManifest) string {
	var latestVersion string
	for _, sm := range manifests {
		if sm.manifest.Scope != plugin.Scope {
			continue
		}
		for _, available := range sm.manifest.Plugins {
			if available.ID != plugin.ID {
				continue
			}
			compatible := FilterVersionCompatibleVersions(FilterCompatibleVersions(available.Versions))
			for _, v := range compatible {
				if pkgversion.Compare(v.Version, plugin.Version) <= 0 {
					continue
				}
				if latestVersion == "" || pkgversion.Compare(v.Version, latestVersion) > 0 {
					latestVersion = v.Version
				}
			}
		}
	}
	return latestVersion
}
