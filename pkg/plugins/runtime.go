package plugins

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/dop251/goja"
	"github.com/pkg/errors"
)

// Runtime wraps a goja VM for a single plugin. It loads manifest.json,
// executes main.js (which defines a `plugin` global via IIFE), and extracts
// hook function references.
type Runtime struct {
	vm       *goja.Runtime
	mu       sync.RWMutex //nolint:unused // Read lock for hook invocation, write lock for reload (future use)
	manifest *Manifest
	scope    string
	pluginID string

	// Hook function references (nil if not provided by the plugin)
	inputConverter   goja.Value
	fileParser       goja.Value
	outputGenerator  goja.Value
	metadataEnricher goja.Value

	// fsCtx is the filesystem context for the current hook invocation.
	// Set by hook execution code before invoking hooks, cleared after.
	fsCtx *FSContext

	// loadWarning holds a non-fatal warning from plugin load (e.g., missing fields declaration).
	// The Manager can check this after successful load and store it in Plugin.LoadError.
	loadWarning string
}

// LoadPlugin creates a new Runtime by reading manifest.json and executing main.js
// from the given plugin directory.
func LoadPlugin(dir, scope, pluginID string) (*Runtime, error) {
	// 1. Read and parse manifest.json
	manifestPath := filepath.Join(dir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read manifest.json")
	}

	manifest, err := ParseManifest(manifestData)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse manifest")
	}

	// 2. Read main.js
	mainJSPath := filepath.Join(dir, "main.js")
	mainJS, err := os.ReadFile(mainJSPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read main.js")
	}

	// 3. Create new goja runtime
	vm := goja.New()

	// 4. Execute main.js
	_, err = vm.RunString(string(mainJS))
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute main.js")
	}

	// 5. Read `plugin` global from VM
	pluginVal := vm.Get("plugin")

	// 6. Verify it's a non-null object
	if pluginVal == nil || goja.IsUndefined(pluginVal) || goja.IsNull(pluginVal) {
		return nil, errors.New("main.js did not define a 'plugin' global")
	}

	pluginObj := pluginVal.ToObject(vm)
	if pluginObj == nil {
		return nil, errors.New("'plugin' global is not an object")
	}

	// 7. Extract hook references
	rt := &Runtime{
		vm:       vm,
		manifest: manifest,
		scope:    scope,
		pluginID: pluginID,
	}

	rt.inputConverter = extractHook(pluginObj, "inputConverter")
	rt.fileParser = extractHook(pluginObj, "fileParser")
	rt.outputGenerator = extractHook(pluginObj, "outputGenerator")
	rt.metadataEnricher = extractHook(pluginObj, "metadataEnricher")

	// 8. Validate: If manifest declares a capability but JS doesn't export it -> warning (don't fail)
	// (Warnings are silent for now; could be logged in the future)

	// 9. Validate: If JS exports a hook but manifest doesn't declare it -> error (fail load)
	if rt.inputConverter != nil && manifest.Capabilities.InputConverter == nil {
		return nil, errors.New("plugin exports 'inputConverter' but manifest does not declare it in capabilities")
	}
	if rt.fileParser != nil && manifest.Capabilities.FileParser == nil {
		return nil, errors.New("plugin exports 'fileParser' but manifest does not declare it in capabilities")
	}
	if rt.outputGenerator != nil && manifest.Capabilities.OutputGenerator == nil {
		return nil, errors.New("plugin exports 'outputGenerator' but manifest does not declare it in capabilities")
	}
	if rt.metadataEnricher != nil && manifest.Capabilities.MetadataEnricher == nil {
		return nil, errors.New("plugin exports 'metadataEnricher' but manifest does not declare it in capabilities")
	}

	// 10. Validate metadataEnricher fields (if declared)
	if manifest.Capabilities.MetadataEnricher != nil {
		enricherCap := manifest.Capabilities.MetadataEnricher
		if len(enricherCap.Fields) == 0 {
			// Missing or empty fields: disable the enricher hook but don't fail load
			rt.metadataEnricher = nil
			rt.loadWarning = "metadataEnricher requires fields declaration"
		} else {
			// Validate that all field names are valid
			for _, f := range enricherCap.Fields {
				if !IsValidMetadataField(f) {
					return nil, errors.Errorf("invalid metadata field %q in metadataEnricher.fields", f)
				}
			}
		}
	}

	return rt, nil
}

// extractHook reads a property from the plugin object and returns the value
// if it is defined and non-null, otherwise returns nil.
func extractHook(obj *goja.Object, name string) goja.Value {
	val := obj.Get(name)
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	return val
}

// Manifest returns the parsed manifest.
func (rt *Runtime) Manifest() *Manifest {
	return rt.manifest
}

// Scope returns the plugin's scope (e.g., "shisho").
func (rt *Runtime) Scope() string {
	return rt.scope
}

// PluginID returns the plugin's ID (e.g., "goodreads-metadata").
func (rt *Runtime) PluginID() string {
	return rt.pluginID
}

// LoadWarning returns any non-fatal warning from plugin load.
// Empty string means no warning.
func (rt *Runtime) LoadWarning() string {
	return rt.loadWarning
}

// HookTypes returns the list of hook type strings this plugin provides,
// sorted alphabetically for deterministic output.
func (rt *Runtime) HookTypes() []string {
	var hooks []string
	if rt.inputConverter != nil {
		hooks = append(hooks, "inputConverter")
	}
	if rt.fileParser != nil {
		hooks = append(hooks, "fileParser")
	}
	if rt.outputGenerator != nil {
		hooks = append(hooks, "outputGenerator")
	}
	if rt.metadataEnricher != nil {
		hooks = append(hooks, "metadataEnricher")
	}
	sort.Strings(hooks)
	return hooks
}

// SetFSContext sets the filesystem context for the current hook invocation.
// Called by hook execution code before invoking hooks. Pass nil to clear.
func (rt *Runtime) SetFSContext(ctx *FSContext) {
	rt.fsCtx = ctx
}
