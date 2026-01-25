package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

// FSContext tracks allowed paths and temp dir for a single hook invocation.
// It controls what filesystem operations a plugin can perform.
type FSContext struct {
	pluginDir     string         // Plugin's own directory (always accessible)
	allowedPaths  []string       // Hook-provided paths (always accessible)
	fileAccessCap *FileAccessCap // From manifest (nil = no global file access)
	tempDir       string         // Lazy-created temp directory path
	tempDirOnce   sync.Once      // Ensures tempDir is created only once
}

// NewFSContext creates a new FSContext for a hook invocation.
func NewFSContext(pluginDir string, allowedPaths []string, fileAccessCap *FileAccessCap) *FSContext {
	return &FSContext{
		pluginDir:     pluginDir,
		allowedPaths:  allowedPaths,
		fileAccessCap: fileAccessCap,
	}
}

// Cleanup removes the temp directory if it was created.
func (ctx *FSContext) Cleanup() error {
	if ctx.tempDir != "" {
		return os.RemoveAll(ctx.tempDir)
	}
	return nil
}

// getOrCreateTempDir lazily creates and returns the temp directory path.
func (ctx *FSContext) getOrCreateTempDir() (string, error) {
	var createErr error
	ctx.tempDirOnce.Do(func() {
		dir, err := os.MkdirTemp("", "shisho-plugin-*")
		if err != nil {
			createErr = err
			return
		}
		ctx.tempDir = dir
	})
	if createErr != nil {
		return "", createErr
	}
	return ctx.tempDir, nil
}

// isReadAllowed checks if the given path is allowed for read operations.
func (ctx *FSContext) isReadAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Always allow plugin's own directory
	if isPathWithin(absPath, ctx.pluginDir) {
		return true
	}

	// Always allow temp directory
	if ctx.tempDir != "" && isPathWithin(absPath, ctx.tempDir) {
		return true
	}

	// Always allow hook-provided paths
	for _, allowed := range ctx.allowedPaths {
		absAllowed, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		// Exact match or prefix (for directory paths)
		if absPath == absAllowed || isPathWithin(absPath, absAllowed) {
			return true
		}
	}

	// Check fileAccess capability
	if ctx.fileAccessCap != nil {
		return ctx.fileAccessCap.Level == "read" || ctx.fileAccessCap.Level == "readwrite"
	}

	return false
}

// isWriteAllowed checks if the given path is allowed for write operations.
func (ctx *FSContext) isWriteAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Always allow plugin's own directory
	if isPathWithin(absPath, ctx.pluginDir) {
		return true
	}

	// Always allow temp directory
	if ctx.tempDir != "" && isPathWithin(absPath, ctx.tempDir) {
		return true
	}

	// Always allow hook-provided paths
	for _, allowed := range ctx.allowedPaths {
		absAllowed, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if absPath == absAllowed || isPathWithin(absPath, absAllowed) {
			return true
		}
	}

	// Check fileAccess capability - only "readwrite" allows writes
	if ctx.fileAccessCap != nil {
		return ctx.fileAccessCap.Level == "readwrite"
	}

	return false
}

// isPathWithin checks if child is within or equal to parent directory.
func isPathWithin(child, parent string) bool {
	// Clean both paths
	child = filepath.Clean(child)
	parent = filepath.Clean(parent)

	if child == parent {
		return true
	}

	// Ensure parent ends with separator for prefix matching.
	// Special-case root "/" which is already terminated.
	parentPrefix := parent
	if !strings.HasSuffix(parentPrefix, string(filepath.Separator)) {
		parentPrefix += string(filepath.Separator)
	}
	return strings.HasPrefix(child, parentPrefix)
}

// injectFSNamespace sets up shisho.fs on the given shisho object.
// Functions read from rt.fsCtx to determine access permissions.
func injectFSNamespace(vm *goja.Runtime, shishoObj *goja.Object, rt *Runtime) error {
	fsObj := vm.NewObject()
	if err := shishoObj.Set("fs", fsObj); err != nil {
		return fmt.Errorf("failed to set shisho.fs: %w", err)
	}

	fsObj.Set("readFile", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.fs.readFile: no filesystem context available"))
		}
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.fs.readFile: path argument is required"))
		}
		path := call.Argument(0).String()

		if !ctx.isReadAllowed(path) {
			panic(vm.ToValue("shisho.fs.readFile: access denied for path: " + path))
		}

		data, err := os.ReadFile(path)
		if err != nil {
			panic(vm.ToValue("shisho.fs.readFile: " + err.Error()))
		}

		// Return as ArrayBuffer
		return vm.ToValue(vm.NewArrayBuffer(data))
	})

	fsObj.Set("readTextFile", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.fs.readTextFile: no filesystem context available"))
		}
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.fs.readTextFile: path argument is required"))
		}
		path := call.Argument(0).String()

		if !ctx.isReadAllowed(path) {
			panic(vm.ToValue("shisho.fs.readTextFile: access denied for path: " + path))
		}

		data, err := os.ReadFile(path)
		if err != nil {
			panic(vm.ToValue("shisho.fs.readTextFile: " + err.Error()))
		}

		return vm.ToValue(string(data))
	})

	fsObj.Set("writeFile", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.fs.writeFile: no filesystem context available"))
		}
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.fs.writeFile: path and data arguments are required"))
		}
		path := call.Argument(0).String()

		if !ctx.isWriteAllowed(path) {
			panic(vm.ToValue("shisho.fs.writeFile: access denied for path: " + path))
		}

		// Extract data from ArrayBuffer
		dataArg := call.Argument(1)
		ab, ok := dataArg.Export().(goja.ArrayBuffer)
		if !ok {
			panic(vm.ToValue("shisho.fs.writeFile: data must be an ArrayBuffer"))
		}

		err := os.WriteFile(path, ab.Bytes(), 0600)
		if err != nil {
			panic(vm.ToValue("shisho.fs.writeFile: " + err.Error()))
		}

		return goja.Undefined()
	})

	fsObj.Set("writeTextFile", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.fs.writeTextFile: no filesystem context available"))
		}
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.fs.writeTextFile: path and content arguments are required"))
		}
		path := call.Argument(0).String()
		content := call.Argument(1).String()

		if !ctx.isWriteAllowed(path) {
			panic(vm.ToValue("shisho.fs.writeTextFile: access denied for path: " + path))
		}

		err := os.WriteFile(path, []byte(content), 0600)
		if err != nil {
			panic(vm.ToValue("shisho.fs.writeTextFile: " + err.Error()))
		}

		return goja.Undefined()
	})

	fsObj.Set("exists", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.fs.exists: no filesystem context available"))
		}
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.fs.exists: path argument is required"))
		}
		path := call.Argument(0).String()

		if !ctx.isReadAllowed(path) {
			panic(vm.ToValue("shisho.fs.exists: access denied for path: " + path))
		}

		_, err := os.Stat(path)
		return vm.ToValue(err == nil)
	})

	fsObj.Set("mkdir", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.fs.mkdir: no filesystem context available"))
		}
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.fs.mkdir: path argument is required"))
		}
		path := call.Argument(0).String()

		if !ctx.isWriteAllowed(path) {
			panic(vm.ToValue("shisho.fs.mkdir: access denied for path: " + path))
		}

		err := os.MkdirAll(path, 0755)
		if err != nil {
			panic(vm.ToValue("shisho.fs.mkdir: " + err.Error()))
		}

		return goja.Undefined()
	})

	fsObj.Set("listDir", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.fs.listDir: no filesystem context available"))
		}
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.fs.listDir: path argument is required"))
		}
		path := call.Argument(0).String()

		if !ctx.isReadAllowed(path) {
			panic(vm.ToValue("shisho.fs.listDir: access denied for path: " + path))
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			panic(vm.ToValue("shisho.fs.listDir: " + err.Error()))
		}

		names := make([]interface{}, len(entries))
		for i, entry := range entries {
			names[i] = entry.Name()
		}

		return vm.ToValue(names)
	})

	fsObj.Set("tempDir", func(_ goja.FunctionCall) goja.Value { //nolint:errcheck
		ctx := rt.fsCtx
		if ctx == nil {
			panic(vm.ToValue("shisho.fs.tempDir: no filesystem context available"))
		}

		dir, err := ctx.getOrCreateTempDir()
		if err != nil {
			panic(vm.ToValue("shisho.fs.tempDir: " + err.Error()))
		}

		return vm.ToValue(dir)
	})

	return nil
}
