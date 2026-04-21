package plugins

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/models"
	pkgversion "github.com/shishobooks/shisho/pkg/version"
)

// reservedExtensions are built-in file types that plugins cannot claim.
var reservedExtensions = map[string]struct{}{
	"epub": {},
	"cbz":  {},
	"m4b":  {},
	"pdf":  {},
}

// Manager holds loaded Runtime instances indexed by "scope/id".
// It coordinates loading at startup, unloading, and hot-reloading on install/update/enable.
type Manager struct {
	mu            sync.RWMutex
	plugins       map[string]*Runtime // key: "scope/id"
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

// isVersionIncompatible checks if the error (or any wrapped error) is ErrVersionIncompatible.
func isVersionIncompatible(err error) bool {
	var vErr *ErrVersionIncompatible
	return stderrors.As(err, &vErr)
}

// LoadAll loads all enabled plugins from the database at startup.
// Errors are stored in plugins.LoadError and don't prevent other plugins from loading.
func (m *Manager) LoadAll(ctx context.Context) error {
	log := logger.New()

	plugins, err := m.service.ListPlugins(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list plugins")
	}

	for _, p := range plugins {
		if p.Status != models.PluginStatusActive {
			continue
		}

		if err := m.loadPlugin(ctx, p.Scope, p.ID); err != nil {
			errMsg := err.Error()
			p.LoadError = &errMsg

			// Distinguish version incompatibility from other load errors
			if isVersionIncompatible(err) {
				p.Status = models.PluginStatusNotSupported
			} else {
				p.Status = models.PluginStatusMalfunctioned
			}

			if updateErr := m.service.UpdatePlugin(ctx, p); updateErr != nil {
				log.Warn("failed to store load error", logger.Data{
					"plugin": pluginKey(p.Scope, p.ID),
					"error":  updateErr.Error(),
				})
			}
			log.Warn("failed to load plugin", logger.Data{
				"plugin": pluginKey(p.Scope, p.ID),
				"error":  err.Error(),
			})
			continue
		}

		// Clear any previous LoadError and ensure Active status on success
		if p.LoadError != nil || p.Status != models.PluginStatusActive {
			p.LoadError = nil
			p.Status = models.PluginStatusActive
			if updateErr := m.service.UpdatePlugin(ctx, p); updateErr != nil {
				log.Warn("failed to clear load error", logger.Data{
					"plugin": pluginKey(p.Scope, p.ID),
					"error":  updateErr.Error(),
				})
			}
		}
	}

	return nil
}

// LoadPlugin loads a single plugin (called during install/enable).
func (m *Manager) LoadPlugin(ctx context.Context, scope, id string) error {
	return m.loadPlugin(ctx, scope, id)
}

// loadPlugin is the internal loading logic shared by LoadAll and LoadPlugin.
func (m *Manager) loadPlugin(ctx context.Context, scope, id string) error {
	dir := filepath.Join(m.pluginDir, scope, id)

	rt, err := LoadPlugin(dir, scope, id)
	if err != nil {
		return errors.Wrapf(err, "failed to load plugin %s/%s", scope, id)
	}

	// Set the persistent data directory for this plugin
	rt.dataDir = filepath.Join(m.pluginDataDir, scope, id)

	if err := InjectHostAPIs(rt, m.service); err != nil {
		return errors.Wrapf(err, "failed to inject host APIs for %s/%s", scope, id)
	}

	key := pluginKey(scope, id)
	m.mu.Lock()
	m.plugins[key] = rt
	m.mu.Unlock()

	// Register identifier types from manifest
	if rt.manifest.Capabilities.IdentifierTypes != nil {
		if err := m.service.UpsertIdentifierTypes(ctx, scope, id, rt.manifest.Capabilities.IdentifierTypes); err != nil {
			return errors.Wrapf(err, "failed to upsert identifier types for %s/%s", scope, id)
		}
	}

	// Append hook types to order table (ignore duplicate key errors)
	for _, hookType := range rt.HookTypes() {
		if err := m.service.AppendToOrder(ctx, hookType, scope, id); err != nil {
			// Ignore duplicate key errors — the plugin may already be in the order table
			_ = err
		}
	}

	return nil
}

// UnloadPlugin removes a plugin from memory (called during uninstall/disable).
func (m *Manager) UnloadPlugin(scope, id string) {
	key := pluginKey(scope, id)
	m.mu.Lock()
	delete(m.plugins, key)
	m.mu.Unlock()
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

// ReloadPlugin performs a hot-reload (called during update).
// Acquires write lock on old runtime (waits for in-progress hooks), then swaps.
func (m *Manager) ReloadPlugin(ctx context.Context, scope, id string) error {
	dir := filepath.Join(m.pluginDir, scope, id)

	// Load new runtime from disk
	newRT, err := LoadPlugin(dir, scope, id)
	if err != nil {
		return errors.Wrapf(err, "failed to reload plugin %s/%s", scope, id)
	}

	// Set the persistent data directory
	newRT.dataDir = filepath.Join(m.pluginDataDir, scope, id)

	// Inject host APIs into new runtime
	if err := InjectHostAPIs(newRT, m.service); err != nil {
		return errors.Wrapf(err, "failed to inject host APIs for %s/%s during reload", scope, id)
	}

	key := pluginKey(scope, id)

	// Acquire write lock on old runtime to wait for in-progress hooks
	m.mu.RLock()
	oldRT := m.plugins[key]
	m.mu.RUnlock()

	if oldRT != nil {
		oldRT.mu.Lock()
		// Swap new runtime into the map while holding the old runtime's lock
		m.mu.Lock()
		m.plugins[key] = newRT
		m.mu.Unlock()
		oldRT.mu.Unlock()
	} else {
		// No existing runtime, just store the new one
		m.mu.Lock()
		m.plugins[key] = newRT
		m.mu.Unlock()
	}

	// Update identifier types
	if newRT.manifest.Capabilities.IdentifierTypes != nil {
		if err := m.service.UpsertIdentifierTypes(ctx, scope, id, newRT.manifest.Capabilities.IdentifierTypes); err != nil {
			return errors.Wrapf(err, "failed to upsert identifier types for %s/%s during reload", scope, id)
		}
	}

	return nil
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
	// Skip reserved extensions
	if _, reserved := reservedExtensions[fileType]; reserved {
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

// RegisteredFileExtensions returns all file extensions registered by plugin fileParsers
// (excluding reserved built-in extensions: epub, cbz, m4b, pdf).
func (m *Manager) RegisteredFileExtensions() map[string]struct{} {
	result := make(map[string]struct{})

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rt := range m.plugins {
		if rt.manifest.Capabilities.FileParser == nil {
			continue
		}
		for _, t := range rt.manifest.Capabilities.FileParser.Types {
			if _, reserved := reservedExtensions[t]; !reserved {
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

	// Fetch manifests from all enabled repositories
	type scopedManifest struct {
		scope    string
		manifest *RepositoryManifest
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

	// For each installed plugin, find latest compatible version from matching repos
	for _, plugin := range installedPlugins {
		// Skip plugins with auto-update disabled
		if !plugin.AutoUpdate {
			continue
		}

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
				if len(compatible) == 0 {
					continue
				}
				for _, v := range compatible {
					if v.Version == plugin.Version {
						continue
					}
					if pkgversion.Compare(v.Version, plugin.Version) <= 0 {
						continue
					}
					if latestVersion == "" || pkgversion.Compare(v.Version, latestVersion) > 0 {
						latestVersion = v.Version
					}
				}
			}
		}

		// Determine if we need to update the record
		var changed bool
		if latestVersion != "" {
			if plugin.UpdateAvailableVersion == nil || *plugin.UpdateAvailableVersion != latestVersion {
				plugin.UpdateAvailableVersion = &latestVersion
				changed = true
			}
		} else {
			if plugin.UpdateAvailableVersion != nil {
				plugin.UpdateAvailableVersion = nil
				changed = true
			}
		}

		if changed {
			if err := m.service.UpdatePlugin(ctx, plugin); err != nil {
				log.Warn("failed to update plugin update_available_version", logger.Data{
					"plugin": pluginKey(plugin.Scope, plugin.ID),
					"error":  err.Error(),
				})
			}
		}
	}

	return nil
}
