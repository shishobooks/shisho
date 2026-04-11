package plugins

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/dop251/goja"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
)

var errDefinePropertyNotFound = errors.New("failed to get Object.defineProperty")

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

	// Set dataDir property (lazily creates the directory on first access)
	if err := injectDataDirProperty(vm, shishoObj, rt); err != nil {
		return err
	}

	// Set up log namespace
	if err := injectLogNamespace(vm, shishoObj, pluginTag); err != nil {
		return err
	}

	// Set up top-level sleep function
	if err := injectSleepFunction(vm, shishoObj, rt); err != nil {
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

	// Set up html namespace
	if err := injectHTMLNamespace(vm, shishoObj); err != nil {
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

// injectDataDirProperty sets up shisho.dataDir as a string property.
// The directory is lazily created on first access via a getter.
func injectDataDirProperty(vm *goja.Runtime, shishoObj *goja.Object, rt *Runtime) error {
	if rt.dataDir == "" {
		return nil
	}

	// Define a getter that lazily creates the directory
	getter := vm.ToValue(func(_ goja.FunctionCall) goja.Value {
		if err := os.MkdirAll(rt.dataDir, 0755); err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.dataDir: failed to create data directory: %v", err)))
		}
		return vm.ToValue(rt.dataDir)
	})

	// Use Object.defineProperty to set up a getter
	objectVal := vm.GlobalObject().Get("Object")
	defineProperty, ok := goja.AssertFunction(objectVal.ToObject(vm).Get("defineProperty"))
	if !ok {
		return errDefinePropertyNotFound
	}

	descriptor := vm.NewObject()
	descriptor.Set("get", getter)         //nolint:errcheck
	descriptor.Set("enumerable", true)    //nolint:errcheck
	descriptor.Set("configurable", false) //nolint:errcheck

	_, err := defineProperty(objectVal, shishoObj, vm.ToValue("dataDir"), descriptor)
	return err
}

var errSleepNotFinite = errors.New("shisho.sleep: ms must be a finite non-negative number")

// validateSleepMs enforces the rules for shisho.sleep's ms argument.
// Extracted so boundary cases can be unit-tested without actually sleeping.
func validateSleepMs(ms float64) error {
	if math.IsNaN(ms) || math.IsInf(ms, 0) || ms < 0 {
		return errSleepNotFinite
	}
	return nil
}

// injectSleepFunction sets up shisho.sleep(ms) as a blocking delay primitive.
// Plugins use this to implement exponential backoff between retries, since
// Goja has no Promise/setTimeout support.
//
// The sleep honors the current hook's context (set by invokeHook in hooks.go)
// so a long sleep inside a hook that has exceeded its deadline unblocks
// immediately instead of sitting in time.Sleep. vm.Interrupt alone cannot do
// this — interrupts only fire between JS statements, not inside native calls.
func injectSleepFunction(vm *goja.Runtime, shishoObj *goja.Object, rt *Runtime) error {
	sleepFn := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.sleep: ms argument is required"))
		}
		ms := call.Argument(0).ToFloat()
		if err := validateSleepMs(ms); err != nil {
			panic(vm.ToValue(err.Error()))
		}
		duration := time.Duration(ms * float64(time.Millisecond))
		ctx := rt.hookCtx
		if ctx == nil {
			// No hook ctx (e.g., direct unit test of shisho.sleep outside a
			// hook runner). Fall back to a plain time.Sleep.
			time.Sleep(duration)
			return goja.Undefined()
		}
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			panic(vm.NewGoError(ctx.Err()))
		}
		return goja.Undefined()
	}
	if err := shishoObj.Set("sleep", sleepFn); err != nil {
		return fmt.Errorf("failed to set shisho.sleep: %w", err)
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
