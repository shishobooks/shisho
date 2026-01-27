package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// ffmpegBinary is the name/path of the ffmpeg binary to use.
// Can be overridden in tests to substitute a mock command.
var ffmpegBinary = "ffmpeg"

// ffprobeBinary is the name/path of the ffprobe binary to use.
// Can be overridden in tests to substitute a mock command.
var ffprobeBinary = "ffprobe"

// defaultFFmpegTimeout is the default timeout for individual ffmpeg calls
// when no context timeout is set by hook execution.
var defaultFFmpegTimeout = 5 * time.Minute

// injectFFmpegNamespace sets up shisho.ffmpeg with the transcode method.
// The transcode function executes FFmpeg as a subprocess with security restrictions:
// - Requires ffmpegAccess capability in the manifest
// - Prepends -protocol_whitelist file,pipe to disable network protocols
// - Respects context cancellation for timeout management.
func injectFFmpegNamespace(vm *goja.Runtime, shishoObj *goja.Object, rt *Runtime) error {
	ffmpegObj := vm.NewObject()
	if err := shishoObj.Set("ffmpeg", ffmpegObj); err != nil {
		return fmt.Errorf("failed to set shisho.ffmpeg: %w", err)
	}

	ffmpegObj.Set("transcode", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		// Check capability
		if rt.manifest.Capabilities.FFmpegAccess == nil {
			panic(vm.ToValue("shisho.ffmpeg.transcode: plugin does not declare ffmpegAccess capability"))
		}

		// Get args array from the JS call
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.ffmpeg.transcode: args argument is required"))
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
				panic(vm.ToValue(fmt.Sprintf("shisho.ffmpeg.transcode: failed to execute: %v", err)))
			}
		}

		// Build result object
		result := vm.NewObject()
		result.Set("exitCode", exitCode)      //nolint:errcheck
		result.Set("stdout", stdout.String()) //nolint:errcheck
		result.Set("stderr", stderr.String()) //nolint:errcheck

		return result
	})

	ffmpegObj.Set("probe", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		// Check capability
		if rt.manifest.Capabilities.FFmpegAccess == nil {
			panic(vm.ToValue("shisho.ffmpeg.probe: plugin does not declare ffmpegAccess capability"))
		}

		// Get args array from the JS call
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.ffmpeg.probe: args argument is required"))
		}

		argsVal := call.Argument(0)
		argsObj := argsVal.ToObject(vm)
		length := int(argsObj.Get("length").ToInteger())

		// Build args: user args first, then 5 args: -print_format json -show_format -show_streams -show_chapters
		args := make([]string, 0, length+5)
		for i := 0; i < length; i++ {
			args = append(args, argsObj.Get(strconv.Itoa(i)).String())
		}
		args = append(args, "-print_format", "json", "-show_format", "-show_streams", "-show_chapters")

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), defaultFFmpegTimeout)
		defer cancel()

		// Build command
		cmd := exec.CommandContext(ctx, ffprobeBinary, args...)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Run the command
		err := cmd.Run()
		if err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				// Command failed to start or was killed
				panic(vm.ToValue(fmt.Sprintf("shisho.ffmpeg.probe: failed to execute: %v", err)))
			}
		}

		// Parse JSON output
		var probeData struct {
			Format   map[string]interface{}   `json:"format"`
			Streams  []map[string]interface{} `json:"streams"`
			Chapters []map[string]interface{} `json:"chapters"`
		}

		var parseError string
		if stdout.Len() > 0 {
			if err := json.Unmarshal(stdout.Bytes(), &probeData); err != nil {
				// Capture parse error for debugging - this can happen if the file doesn't
				// exist or if a non-ffprobe binary is used in tests.
				parseError = err.Error()
			}
		}

		// Build result object with parsed data
		result := vm.NewObject()

		// Format
		if probeData.Format != nil {
			formatObj := vm.ToValue(probeData.Format)
			result.Set("format", formatObj) //nolint:errcheck
		} else {
			result.Set("format", vm.NewObject()) //nolint:errcheck
		}

		// Streams
		if probeData.Streams != nil {
			streamsVal := vm.ToValue(probeData.Streams)
			result.Set("streams", streamsVal) //nolint:errcheck
		} else {
			emptyArr, _ := vm.RunString("[]")
			result.Set("streams", emptyArr) //nolint:errcheck
		}

		// Chapters
		if probeData.Chapters != nil {
			chaptersVal := vm.ToValue(probeData.Chapters)
			result.Set("chapters", chaptersVal) //nolint:errcheck
		} else {
			emptyArr, _ := vm.RunString("[]")
			result.Set("chapters", emptyArr) //nolint:errcheck
		}

		result.Set("stderr", stderr.String()) //nolint:errcheck
		result.Set("parseError", parseError)  //nolint:errcheck

		return result
	})

	ffmpegObj.Set("version", func(_ goja.FunctionCall) goja.Value { //nolint:errcheck
		// Check capability
		if rt.manifest.Capabilities.FFmpegAccess == nil {
			panic(vm.ToValue("shisho.ffmpeg.version: plugin does not declare ffmpegAccess capability"))
		}

		// Create context with timeout (shorter for version check)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Run ffmpeg -version
		cmd := exec.CommandContext(ctx, ffmpegBinary, "-version")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				panic(vm.ToValue(fmt.Sprintf("shisho.ffmpeg.version: failed to execute: %v", err)))
			}
		}

		output := stdout.String()

		// Parse version string (first line: "ffmpeg version X.X.X ...")
		version := ""
		if idx := strings.Index(output, "ffmpeg version "); idx != -1 {
			rest := output[idx+15:]
			if spaceIdx := strings.IndexAny(rest, " \n"); spaceIdx != -1 {
				version = rest[:spaceIdx]
			}
		}

		// Parse configuration (line starting with "configuration:")
		configuration := make([]string, 0)
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, "configuration:") {
				configStr := strings.TrimPrefix(line, "configuration:")
				parts := strings.Fields(configStr)
				configuration = append(configuration, parts...)
				break
			}
		}

		// Parse library versions (lines like "libavutil      58. 29.100 / 58. 29.100")
		libraries := make(map[string]string)
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "lib") {
				// Parse: "libavutil      58. 29.100 / 58. 29.100"
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					libName := fields[0]
					// Version is typically in format "58. 29.100" - normalize to "58.29.100"
					verParts := []string{}
					for i := 1; i < len(fields) && fields[i] != "/"; i++ {
						verParts = append(verParts, strings.TrimSuffix(fields[i], "."))
					}
					if len(verParts) > 0 {
						libraries[libName] = strings.Join(verParts, ".")
					}
				}
			}
		}

		// Build result object
		result := vm.NewObject()
		result.Set("version", version)                         //nolint:errcheck
		result.Set("configuration", vm.ToValue(configuration)) //nolint:errcheck
		result.Set("libraries", vm.ToValue(libraries))         //nolint:errcheck

		return result
	})

	return nil
}
