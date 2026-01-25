package plugins

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
)

// reservedExtensions are built-in file types that plugins cannot claim.
var reservedExtensions = map[string]struct{}{
	"epub": {},
	"cbz":  {},
	"m4b":  {},
}

// Manager holds loaded Runtime instances indexed by "scope/id".
// It coordinates loading at startup, unloading, and hot-reloading on install/update/enable.
type Manager struct {
	mu        sync.RWMutex
	plugins   map[string]*Runtime // key: "scope/id"
	service   *Service
	pluginDir string

	// fetchRepo is the function used to fetch repository manifests.
	// Defaults to FetchRepository; tests can override this.
	fetchRepo func(url string) (*RepositoryManifest, error)
}

// NewManager creates a new Manager.
func NewManager(service *Service, pluginDir string) *Manager {
	return &Manager{
		plugins:   make(map[string]*Runtime),
		service:   service,
		pluginDir: pluginDir,
		fetchRepo: FetchRepository,
	}
}

// pluginKey returns the map key for a plugin.
func pluginKey(scope, id string) string {
	return scope + "/" + id
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
		if !p.Enabled {
			continue
		}

		if err := m.loadPlugin(ctx, p.Scope, p.ID); err != nil {
			// Store error in the plugin record
			errMsg := err.Error()
			p.LoadError = &errMsg
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

		// Clear any previous LoadError on success
		if p.LoadError != nil {
			p.LoadError = nil
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

// ReloadPlugin performs a hot-reload (called during update).
// Acquires write lock on old runtime (waits for in-progress hooks), then swaps.
func (m *Manager) ReloadPlugin(ctx context.Context, scope, id string) error {
	dir := filepath.Join(m.pluginDir, scope, id)

	// Load new runtime from disk
	newRT, err := LoadPlugin(dir, scope, id)
	if err != nil {
		return errors.Wrapf(err, "failed to reload plugin %s/%s", scope, id)
	}

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
				if !entry.Enabled {
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
// (excluding reserved built-in extensions: epub, cbz, m4b).
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
		var latestVersion string

		for _, sm := range manifests {
			if sm.manifest.Scope != plugin.Scope {
				continue
			}
			for _, available := range sm.manifest.Plugins {
				if available.ID != plugin.ID {
					continue
				}
				compatible := FilterCompatibleVersions(available.Versions)
				if len(compatible) == 0 {
					continue
				}
				// The last compatible version is the latest
				candidate := compatible[len(compatible)-1].Version
				if candidate != plugin.Version && (latestVersion == "" || candidate > latestVersion) {
					latestVersion = candidate
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
