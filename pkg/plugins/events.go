package plugins

// PluginEventType describes a plugin lifecycle event.
type PluginEventType int

const (
	// PluginEventInstalled fires after a plugin is installed and loaded.
	PluginEventInstalled PluginEventType = iota
	// PluginEventUninstalled fires after a plugin is uninstalled.
	PluginEventUninstalled
	// PluginEventUpdated fires after a plugin version is updated (hot-reloaded).
	PluginEventUpdated
	// PluginEventEnabled fires after a disabled plugin is re-enabled and loaded.
	PluginEventEnabled
	// PluginEventDisabled fires after an active plugin is disabled and unloaded.
	PluginEventDisabled
	// PluginEventMalfunctioned fires when a plugin fails to load.
	PluginEventMalfunctioned
)

// PluginEvent contains information about a plugin lifecycle change.
type PluginEvent struct {
	Type  PluginEventType
	Scope string
	ID    string
	// Hooks lists the hook types this plugin provides (e.g., "fileParser", "metadataEnricher").
	// Empty for uninstall/disable events.
	Hooks []string
}

// PluginEventCallback is the signature for event listeners.
type PluginEventCallback func(event PluginEvent)
