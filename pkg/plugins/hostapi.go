package plugins

import (
	"context"
	"fmt"

	"github.com/dop251/goja"
	"github.com/robinjoseph08/golib/logger"
)

// ConfigGetter provides config values for a plugin at runtime.
type ConfigGetter interface {
	GetConfigRaw(ctx context.Context, scope, pluginID, key string) (*string, error)
	GetAllConfigRaw(ctx context.Context, scope, pluginID string) (map[string]*string, error)
}

// InjectHostAPIs sets up the shisho.* APIs in a plugin's goja runtime.
// The logPrefix is derived from the runtime's scope and pluginID.
// The configGetter provides config values for the plugin.
func InjectHostAPIs(rt *Runtime, configGetter ConfigGetter) error {
	vm := rt.vm
	pluginTag := rt.scope + "/" + rt.pluginID

	// Create the top-level shisho object
	shishoObj := vm.NewObject()
	if err := vm.Set("shisho", shishoObj); err != nil {
		return fmt.Errorf("failed to set shisho global: %w", err)
	}

	// Set up log namespace
	if err := injectLogNamespace(vm, shishoObj, pluginTag); err != nil {
		return err
	}

	// Set up config namespace
	if err := injectConfigNamespace(vm, shishoObj, rt.scope, rt.pluginID, configGetter); err != nil {
		return err
	}

	// Set up http namespace
	if err := injectHTTPNamespace(vm, shishoObj, rt); err != nil {
		return err
	}

	// Set up fs namespace
	if err := injectFSNamespace(vm, shishoObj, rt); err != nil {
		return err
	}

	// Set up archive namespace
	if err := injectArchiveNamespace(vm, shishoObj, rt); err != nil {
		return err
	}

	// Set up xml namespace
	if err := injectXMLNamespace(vm, shishoObj); err != nil {
		return err
	}

	// Set up ffmpeg namespace
	if err := injectFFmpegNamespace(vm, shishoObj, rt); err != nil {
		return err
	}

	// Set up shell namespace
	if err := injectShellNamespace(vm, shishoObj, rt); err != nil {
		return err
	}

	// Set up url namespace
	if err := injectURLNamespace(vm, shishoObj); err != nil {
		return err
	}

	return nil
}

// injectLogNamespace sets up shisho.log with debug/info/warn/error methods.
func injectLogNamespace(vm *goja.Runtime, shishoObj *goja.Object, pluginTag string) error {
	log := logger.New()
	logObj := vm.NewObject()
	if err := shishoObj.Set("log", logObj); err != nil {
		return fmt.Errorf("failed to set shisho.log: %w", err)
	}

	logObj.Set("debug", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		msg := call.Argument(0).String()
		log.Debug(msg, logger.Data{"plugin": pluginTag})
		return goja.Undefined()
	})
	logObj.Set("info", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		msg := call.Argument(0).String()
		log.Info(msg, logger.Data{"plugin": pluginTag})
		return goja.Undefined()
	})
	logObj.Set("warn", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		msg := call.Argument(0).String()
		log.Warn(msg, logger.Data{"plugin": pluginTag})
		return goja.Undefined()
	})
	logObj.Set("error", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		msg := call.Argument(0).String()
		log.Error(msg, logger.Data{"plugin": pluginTag})
		return goja.Undefined()
	})

	return nil
}

// injectConfigNamespace sets up shisho.config with get/getAll methods.
func injectConfigNamespace(vm *goja.Runtime, shishoObj *goja.Object, scope, pluginID string, configGetter ConfigGetter) error {
	configObj := vm.NewObject()
	if err := shishoObj.Set("config", configObj); err != nil {
		return fmt.Errorf("failed to set shisho.config: %w", err)
	}

	configObj.Set("get", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		key := call.Argument(0).String()
		ctx := context.Background()
		val, err := configGetter.GetConfigRaw(ctx, scope, pluginID, key)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.config.get failed: %v", err)))
		}
		if val == nil {
			return goja.Undefined()
		}
		return vm.ToValue(*val)
	})

	configObj.Set("getAll", func(_ goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := context.Background()
		all, err := configGetter.GetAllConfigRaw(ctx, scope, pluginID)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.config.getAll failed: %v", err)))
		}
		result := vm.NewObject()
		for k, v := range all {
			if v != nil {
				result.Set(k, *v) //nolint:errcheck
			}
		}
		return result
	})

	return nil
}
