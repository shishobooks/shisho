package plugins

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/dop251/goja"
)

// ffmpegBinary is the name/path of the ffmpeg binary to use.
// Can be overridden in tests to substitute a mock command.
var ffmpegBinary = "ffmpeg"

// defaultFFmpegTimeout is the default timeout for individual ffmpeg calls
// when no context timeout is set by hook execution.
var defaultFFmpegTimeout = 5 * time.Minute

// injectFFmpegNamespace sets up shisho.ffmpeg with the run method.
// The run function executes FFmpeg as a subprocess with security restrictions:
// - Requires ffmpegAccess capability in the manifest
// - Prepends -protocol_whitelist file,pipe to disable network protocols
// - Respects context cancellation for timeout management.
func injectFFmpegNamespace(vm *goja.Runtime, shishoObj *goja.Object, rt *Runtime) error {
	ffmpegObj := vm.NewObject()
	if err := shishoObj.Set("ffmpeg", ffmpegObj); err != nil {
		return fmt.Errorf("failed to set shisho.ffmpeg: %w", err)
	}

	ffmpegObj.Set("run", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		// Check capability
		if rt.manifest.Capabilities.FFmpegAccess == nil {
			panic(vm.ToValue("shisho.ffmpeg.run: plugin does not declare ffmpegAccess capability"))
		}

		// Get args array from the JS call
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.ffmpeg.run: args argument is required"))
		}

		argsVal := call.Argument(0)
		argsObj := argsVal.ToObject(vm)
		length := int(argsObj.Get("length").ToInteger())

		// Build args with protocol_whitelist prepended
		args := make([]string, 0, length+2)
		args = append(args, "-protocol_whitelist", "file,pipe")
		for i := 0; i < length; i++ {
			args = append(args, argsObj.Get(strconv.Itoa(i)).String())
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), defaultFFmpegTimeout)
		defer cancel()

		// Build command
		cmd := exec.CommandContext(ctx, ffmpegBinary, args...)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Run the command
		err := cmd.Run()

		// Determine exit code
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				// Command failed to start or was killed
				panic(vm.ToValue(fmt.Sprintf("shisho.ffmpeg.run: failed to execute: %v", err)))
			}
		}

		// Build result object
		result := vm.NewObject()
		result.Set("exitCode", exitCode)      //nolint:errcheck
		result.Set("stdout", stdout.String()) //nolint:errcheck
		result.Set("stderr", stderr.String()) //nolint:errcheck

		return result
	})

	return nil
}
