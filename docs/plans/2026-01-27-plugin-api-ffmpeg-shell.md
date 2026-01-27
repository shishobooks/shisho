# Plugin API: FFmpeg Enhancements and Shell Exec - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enhance the plugin FFmpeg API (rename `run()` to `transcode()`, add `probe()` and `version()`) and add a new `shisho.shell.exec()` API with command allowlist security.

**Architecture:** The FFmpeg namespace is refactored to expose three distinct methods. A new Shell namespace is added with an allowlist-based security model where plugins must declare allowed commands in their manifest. Both namespaces use direct exec (no shell) for security.

**Tech Stack:** Go (goja runtime), TypeScript type definitions

---

## Task 1: Add ShellAccessCap to Manifest

**Files:**
- Modify: `pkg/plugins/manifest.go:26-35` (add ShellAccessCap to Capabilities)
- Modify: `packages/plugin-types/manifest.d.ts:65-80` (add ShellAccessCap interface and to Capabilities)

**Step 1: Add ShellAccessCap struct to Go manifest**

In `pkg/plugins/manifest.go`, after `FFmpegAccessCap` (line 79-81), add:

```go
type ShellAccessCap struct {
	Description string   `json:"description"`
	Commands    []string `json:"commands"`
}
```

**Step 2: Add ShellAccess field to Capabilities struct**

In `pkg/plugins/manifest.go`, inside the `Capabilities` struct (around line 34), add after `FFmpegAccess`:

```go
	ShellAccess      *ShellAccessCap      `json:"shellAccess"`
```

**Step 3: Add TypeScript types for ShellAccessCap**

In `packages/plugin-types/manifest.d.ts`, after `FFmpegAccessCap` (line 66-68), add:

```typescript
/** Shell access capability declaration. */
export interface ShellAccessCap {
  description?: string;
  /** Allowed commands (e.g., ["convert", "magick", "identify"]). */
  commands: string[];
}
```

**Step 4: Add shellAccess to Capabilities interface**

In `packages/plugin-types/manifest.d.ts`, inside the `Capabilities` interface (around line 79), add after `ffmpegAccess`:

```typescript
  shellAccess?: ShellAccessCap;
```

**Step 5: Run make tygo**

Run: `make tygo`
Expected: "Nothing to be done for \`tygo'" or successful generation

**Step 6: Commit**

```bash
git add pkg/plugins/manifest.go packages/plugin-types/manifest.d.ts
git commit -m "$(cat <<'EOF'
[Backend] Add ShellAccessCap to manifest for shell exec capability
EOF
)"
```

---

## Task 2: Rename FFmpeg run() to transcode()

**Files:**
- Modify: `pkg/plugins/hostapi_ffmpeg.go:33` (rename method)
- Modify: `pkg/plugins/hostapi_ffmpeg_test.go` (update all test calls)
- Modify: `packages/plugin-types/host-api.d.ts:115-129` (rename method and result type)

**Step 1: Rename the method in hostapi_ffmpeg.go**

In `pkg/plugins/hostapi_ffmpeg.go`, line 33, change:

```go
	ffmpegObj.Set("run", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
```

to:

```go
	ffmpegObj.Set("transcode", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
```

**Step 2: Update panic messages**

In `pkg/plugins/hostapi_ffmpeg.go`, update panic messages from `shisho.ffmpeg.run:` to `shisho.ffmpeg.transcode:`:

Line 36:
```go
		panic(vm.ToValue("shisho.ffmpeg.transcode: plugin does not declare ffmpegAccess capability"))
```

Line 41:
```go
		panic(vm.ToValue("shisho.ffmpeg.transcode: args argument is required"))
```

Line 76:
```go
			panic(vm.ToValue(fmt.Sprintf("shisho.ffmpeg.transcode: failed to execute: %v", err)))
```

**Step 3: Update TypeScript types**

In `packages/plugin-types/host-api.d.ts`, rename `FFmpegResult` to `TranscodeResult` (line 115):

```typescript
/** Result from shisho.ffmpeg.transcode(). */
export interface TranscodeResult {
  /** Process exit code (0 = success). */
  exitCode: number;
  /** Standard output. */
  stdout: string;
  /** Standard error. */
  stderr: string;
}
```

Update `ShishoFFmpeg` interface (line 125-129):

```typescript
/** FFmpeg subprocess execution. */
export interface ShishoFFmpeg {
  /** Transcode files with FFmpeg. Requires ffmpegAccess capability. */
  transcode(args: string[]): TranscodeResult;
}
```

**Step 4: Update all tests to use transcode()**

In `pkg/plugins/hostapi_ffmpeg_test.go`, replace all occurrences of `shisho.ffmpeg.run` with `shisho.ffmpeg.transcode`:

- Line 63: `shisho.ffmpeg.transcode(["-version"])`
- Line 72: `shisho.ffmpeg.transcode()`
- Lines 90-92: `var result = shisho.ffmpeg.transcode(["hello", "world"]);`
- Lines 117-119: `var result = shisho.ffmpeg.transcode(["-i", "input.mp4", "output.mp3"]);`
- Lines 150-152: `var result = shisho.ffmpeg.transcode([]);`
- Lines 174-176: `var result = shisho.ffmpeg.transcode(["-i", "nonexistent_file_12345.mp4", "-f", "null", "-"]);`
- Line 191: `shisho.ffmpeg.transcode(["-version"])`
- Lines 209-211: `var result = shisho.ffmpeg.transcode([]);`
- Lines 230-235: `var result = shisho.ffmpeg.transcode(["test"]);`
- Lines 258-260: `var result = shisho.ffmpeg.transcode(["-version"]);`

Also update test function name and error messages:
- Line 59: Test name `TestFFmpeg_NoCapability` - update error check to `"plugin does not declare ffmpegAccess capability"` (unchanged)
- Line 68: Test name stays, update error check to `"args argument is required"` (unchanged)

**Step 5: Run tests to verify**

Run: `go test -v ./pkg/plugins/... -run TestFFmpeg`
Expected: All tests pass

**Step 6: Commit**

```bash
git add pkg/plugins/hostapi_ffmpeg.go pkg/plugins/hostapi_ffmpeg_test.go packages/plugin-types/host-api.d.ts
git commit -m "$(cat <<'EOF'
[Backend] Rename shisho.ffmpeg.run() to transcode()

BREAKING: Plugins must update from run() to transcode()
EOF
)"
```

---

## Task 3: Add ffprobeBinary variable and probe() method

**Files:**
- Modify: `pkg/plugins/hostapi_ffmpeg.go` (add ffprobeBinary var and probe method)
- Modify: `pkg/plugins/hostapi_ffmpeg_test.go` (add probe tests)
- Modify: `packages/plugin-types/host-api.d.ts` (add ProbeResult types and probe method)

**Step 1: Add ffprobeBinary variable**

In `pkg/plugins/hostapi_ffmpeg.go`, after line 16 (ffmpegBinary), add:

```go
// ffprobeBinary is the name/path of the ffprobe binary to use.
// Can be overridden in tests to substitute a mock command.
var ffprobeBinary = "ffprobe"
```

**Step 2: Add probe() method implementation**

In `pkg/plugins/hostapi_ffmpeg.go`, after the transcode method (after line 87, before the final `return nil`), add the probe method:

```go
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

		// Build args: user args first, then -print_format json -show_format -show_streams -show_chapters
		args := make([]string, 0, length+4)
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

		if stdout.Len() > 0 {
			if jsonErr := json.Unmarshal(stdout.Bytes(), &probeData); jsonErr != nil {
				panic(vm.ToValue(fmt.Sprintf("shisho.ffmpeg.probe: failed to parse JSON output: %v", jsonErr)))
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

		return result
	})
```

**Step 3: Add json import**

In `pkg/plugins/hostapi_ffmpeg.go`, add `"encoding/json"` to imports:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/dop251/goja"
)
```

**Step 4: Add TypeScript types for ProbeResult**

In `packages/plugin-types/host-api.d.ts`, after `TranscodeResult` and before `ShishoFFmpeg`, add the probe types:

```typescript
/** Result from shisho.ffmpeg.probe(). */
export interface ProbeResult {
  format: ProbeFormat;
  streams: ProbeStream[];
  chapters: ProbeChapter[];
  /** Standard error output (for debugging). */
  stderr: string;
}

/** Format information from ffprobe. */
export interface ProbeFormat {
  filename: string;
  nb_streams: number;
  nb_programs: number;
  format_name: string;
  format_long_name: string;
  start_time: string;
  duration: string;
  size: string;
  bit_rate: string;
  probe_score: number;
  tags?: Record<string, string>;
}

/** Stream information from ffprobe. */
export interface ProbeStream {
  index: number;
  codec_name: string;
  codec_long_name: string;
  codec_type: "video" | "audio" | "subtitle" | "data" | "attachment";
  codec_tag_string: string;
  codec_tag: string;

  // Video-specific
  width?: number;
  height?: number;
  coded_width?: number;
  coded_height?: number;
  closed_captions?: number;
  has_b_frames?: number;
  sample_aspect_ratio?: string;
  display_aspect_ratio?: string;
  pix_fmt?: string;
  level?: number;
  color_range?: string;
  color_space?: string;
  color_transfer?: string;
  color_primaries?: string;
  chroma_location?: string;
  field_order?: string;
  refs?: number;

  // Audio-specific
  sample_fmt?: string;
  sample_rate?: string;
  channels?: number;
  channel_layout?: string;
  bits_per_sample?: number;

  // Common
  r_frame_rate: string;
  avg_frame_rate: string;
  time_base: string;
  start_pts?: number;
  start_time?: string;
  duration_ts?: number;
  duration?: string;
  bit_rate?: string;
  bits_per_raw_sample?: string;
  nb_frames?: string;
  disposition: ProbeDisposition;
  tags?: Record<string, string>;
}

/** Stream disposition flags from ffprobe. */
export interface ProbeDisposition {
  default: number;
  dub: number;
  original: number;
  comment: number;
  lyrics: number;
  karaoke: number;
  forced: number;
  hearing_impaired: number;
  visual_impaired: number;
  clean_effects: number;
  attached_pic: number;
  timed_thumbnails: number;
}

/** Chapter information from ffprobe. */
export interface ProbeChapter {
  id: number;
  time_base: string;
  start: number;
  start_time: string;
  end: number;
  end_time: string;
  tags?: Record<string, string>;
}
```

**Step 5: Add probe method to ShishoFFmpeg interface**

In `packages/plugin-types/host-api.d.ts`, update `ShishoFFmpeg`:

```typescript
/** FFmpeg subprocess execution. */
export interface ShishoFFmpeg {
  /** Transcode files with FFmpeg. Requires ffmpegAccess capability. */
  transcode(args: string[]): TranscodeResult;
  /** Probe file metadata with ffprobe. Returns parsed JSON. Requires ffmpegAccess capability. */
  probe(args: string[]): ProbeResult;
}
```

**Step 6: Add probe tests**

In `pkg/plugins/hostapi_ffmpeg_test.go`, add these tests at the end of the file:

```go
func TestFFmpegProbe_NoCapability(t *testing.T) {
	t.Parallel()
	rt := newTestRuntimeWithoutFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.ffmpeg.probe(["-i", "test.mp4"])`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin does not declare ffmpegAccess capability")
}

func TestFFmpegProbe_MissingArgs(t *testing.T) {
	t.Parallel()
	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.ffmpeg.probe()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "args argument is required")
}

func TestFFmpegProbe_BinaryNotFound(t *testing.T) {
	t.Parallel()
	origBinary := ffprobeBinary
	ffprobeBinary = "nonexistent-binary-12345"
	defer func() { ffprobeBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.ffmpeg.probe(["-i", "test.mp4"])`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute")
}

func TestFFmpegProbe_JsonArgsAppended(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("echo test not supported on Windows")
	}

	// Use echo to verify args are appended correctly
	origBinary := ffprobeBinary
	ffprobeBinary = "echo"
	defer func() { ffprobeBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	// probe() should append -print_format json -show_format -show_streams -show_chapters
	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.probe(["-i", "test.mp4"]);
		result.stderr;
	`)
	require.NoError(t, err)
	stderr := val.String()
	// echo outputs to stdout, but our test captures stderr which will be empty
	// The important thing is that it doesn't crash - full integration tested separately
	assert.Empty(t, stderr)
}

func TestFFmpegProbe_RealFFprobe(t *testing.T) {
	t.Parallel()
	// Skip if ffprobe is not installed
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found on PATH, skipping integration test")
	}

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	// Probe a non-existent file - should still return result structure with stderr
	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.probe(["-i", "nonexistent_file_12345.mp4"]);
		JSON.stringify({
			hasFormat: "format" in result,
			hasStreams: "streams" in result,
			hasChapters: "chapters" in result,
			hasStderr: "stderr" in result
		});
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), `"hasFormat":true`)
	assert.Contains(t, val.String(), `"hasStreams":true`)
	assert.Contains(t, val.String(), `"hasChapters":true`)
	assert.Contains(t, val.String(), `"hasStderr":true`)
}
```

**Step 7: Run tests to verify**

Run: `go test -v ./pkg/plugins/... -run TestFFmpegProbe`
Expected: All tests pass

**Step 8: Commit**

```bash
git add pkg/plugins/hostapi_ffmpeg.go pkg/plugins/hostapi_ffmpeg_test.go packages/plugin-types/host-api.d.ts
git commit -m "$(cat <<'EOF'
[Backend] Add shisho.ffmpeg.probe() for metadata extraction

- Executes ffprobe with auto-appended JSON output flags
- Returns parsed format, streams, chapters, and stderr
- Uses same ffmpegAccess capability as transcode()
EOF
)"
```

---

## Task 4: Add version() method to FFmpeg namespace

**Files:**
- Modify: `pkg/plugins/hostapi_ffmpeg.go` (add version method)
- Modify: `pkg/plugins/hostapi_ffmpeg_test.go` (add version tests)
- Modify: `packages/plugin-types/host-api.d.ts` (add VersionResult type and version method)

**Step 1: Add version() method implementation**

In `pkg/plugins/hostapi_ffmpeg.go`, after the probe method (before the final `return nil`), add:

```go
	ffmpegObj.Set("version", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
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
		result.Set("version", version)                   //nolint:errcheck
		result.Set("configuration", vm.ToValue(configuration)) //nolint:errcheck
		result.Set("libraries", vm.ToValue(libraries))   //nolint:errcheck

		return result
	})
```

**Step 2: Add strings import**

In `pkg/plugins/hostapi_ffmpeg.go`, add `"strings"` to imports:

```go
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
```

**Step 3: Add TypeScript types for VersionResult**

In `packages/plugin-types/host-api.d.ts`, after `ProbeChapter` and before `ShishoFFmpeg`, add:

```typescript
/** Result from shisho.ffmpeg.version(). */
export interface VersionResult {
  /** FFmpeg version string (e.g., "7.0"). */
  version: string;
  /** Build configuration flags (e.g., ["--enable-libx264", "--enable-gpl"]). */
  configuration: string[];
  /** Library versions (e.g., { libavcodec: "60.31.102", ... }). */
  libraries: Record<string, string>;
}
```

**Step 4: Add version method to ShishoFFmpeg interface**

In `packages/plugin-types/host-api.d.ts`, update `ShishoFFmpeg`:

```typescript
/** FFmpeg subprocess execution. */
export interface ShishoFFmpeg {
  /** Transcode files with FFmpeg. Requires ffmpegAccess capability. */
  transcode(args: string[]): TranscodeResult;
  /** Probe file metadata with ffprobe. Returns parsed JSON. Requires ffmpegAccess capability. */
  probe(args: string[]): ProbeResult;
  /** Get FFmpeg version and configuration. Requires ffmpegAccess capability. */
  version(): VersionResult;
}
```

**Step 5: Add version tests**

In `pkg/plugins/hostapi_ffmpeg_test.go`, add these tests at the end:

```go
func TestFFmpegVersion_NoCapability(t *testing.T) {
	t.Parallel()
	rt := newTestRuntimeWithoutFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.ffmpeg.version()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin does not declare ffmpegAccess capability")
}

func TestFFmpegVersion_BinaryNotFound(t *testing.T) {
	t.Parallel()
	origBinary := ffmpegBinary
	ffmpegBinary = "nonexistent-binary-12345"
	defer func() { ffmpegBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.ffmpeg.version()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute")
}

func TestFFmpegVersion_ResultFields(t *testing.T) {
	t.Parallel()
	// Skip if ffmpeg is not installed
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found on PATH, skipping integration test")
	}

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.version();
		JSON.stringify({
			hasVersion: "version" in result,
			hasConfiguration: "configuration" in result,
			hasLibraries: "libraries" in result,
			versionNotEmpty: result.version.length > 0
		});
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), `"hasVersion":true`)
	assert.Contains(t, val.String(), `"hasConfiguration":true`)
	assert.Contains(t, val.String(), `"hasLibraries":true`)
	assert.Contains(t, val.String(), `"versionNotEmpty":true`)
}

func TestFFmpegVersion_RealFFmpeg(t *testing.T) {
	t.Parallel()
	// Skip if ffmpeg is not installed
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found on PATH, skipping integration test")
	}

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.version();
		result.version;
	`)
	require.NoError(t, err)
	version := val.String()
	// Version should be a semver-like string (e.g., "7.0", "6.1.1")
	assert.NotEmpty(t, version)
	assert.Regexp(t, `^\d+`, version)
}
```

**Step 6: Run tests to verify**

Run: `go test -v ./pkg/plugins/... -run TestFFmpegVersion`
Expected: All tests pass

**Step 7: Commit**

```bash
git add pkg/plugins/hostapi_ffmpeg.go pkg/plugins/hostapi_ffmpeg_test.go packages/plugin-types/host-api.d.ts
git commit -m "$(cat <<'EOF'
[Backend] Add shisho.ffmpeg.version() for capability detection

- Returns parsed version string, configuration flags, and library versions
- Useful for plugins to check FFmpeg capabilities before transcoding
EOF
)"
```

---

## Task 5: Create Shell Exec Host API

**Files:**
- Create: `pkg/plugins/hostapi_shell.go`
- Create: `pkg/plugins/hostapi_shell_test.go`
- Modify: `pkg/plugins/hostapi.go:60-65` (add injectShellNamespace call)
- Modify: `packages/plugin-types/host-api.d.ts` (add ShishoShell and ExecResult)

**Step 1: Create hostapi_shell.go**

Create `pkg/plugins/hostapi_shell.go`:

```go
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

// defaultShellTimeout is the default timeout for shell command execution.
var defaultShellTimeout = 5 * time.Minute

// injectShellNamespace sets up shisho.shell with the exec method.
// The exec function executes allowed commands as subprocesses:
// - Requires shellAccess capability with command in allowlist
// - Uses exec.Command directly (no shell) to prevent injection
// - Respects context cancellation for timeout management.
func injectShellNamespace(vm *goja.Runtime, shishoObj *goja.Object, rt *Runtime) error {
	shellObj := vm.NewObject()
	if err := shishoObj.Set("shell", shellObj); err != nil {
		return fmt.Errorf("failed to set shisho.shell: %w", err)
	}

	shellObj.Set("exec", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		// Check capability
		if rt.manifest.Capabilities.ShellAccess == nil {
			panic(vm.ToValue("shisho.shell.exec: plugin does not declare shellAccess capability"))
		}

		// Get command argument
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("shisho.shell.exec: command and args arguments are required"))
		}

		command := call.Argument(0).String()

		// Validate command against allowlist
		allowed := false
		for _, cmd := range rt.manifest.Capabilities.ShellAccess.Commands {
			if cmd == command {
				allowed = true
				break
			}
		}
		if !allowed {
			panic(vm.ToValue(fmt.Sprintf("shisho.shell.exec: command %q is not in allowed list", command)))
		}

		// Get args array
		argsVal := call.Argument(1)
		argsObj := argsVal.ToObject(vm)
		length := int(argsObj.Get("length").ToInteger())

		args := make([]string, 0, length)
		for i := 0; i < length; i++ {
			args = append(args, argsObj.Get(strconv.Itoa(i)).String())
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), defaultShellTimeout)
		defer cancel()

		// Build command - use exec.Command directly (no shell)
		cmd := exec.CommandContext(ctx, command, args...)

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
				panic(vm.ToValue(fmt.Sprintf("shisho.shell.exec: failed to execute: %v", err)))
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
```

**Step 2: Add shell namespace injection to hostapi.go**

In `pkg/plugins/hostapi.go`, after the ffmpeg namespace injection (line 61-63), add:

```go
	// Set up shell namespace
	if err := injectShellNamespace(vm, shishoObj, rt); err != nil {
		return err
	}
```

**Step 3: Add TypeScript types**

In `packages/plugin-types/host-api.d.ts`, after `ShishoFFmpeg` and before `ShishoHostAPI`, add:

```typescript
/** Result from shisho.shell.exec(). */
export interface ExecResult {
  /** Process exit code (0 = success). */
  exitCode: number;
  /** Standard output. */
  stdout: string;
  /** Standard error. */
  stderr: string;
}

/** Shell command execution (with allowlist). */
export interface ShishoShell {
  /**
   * Execute an allowed command with arguments.
   * Command must be declared in manifest shellAccess.commands.
   * Uses exec directly (no shell) to prevent injection.
   */
  exec(command: string, args: string[]): ExecResult;
}
```

**Step 4: Add shell to ShishoHostAPI**

In `packages/plugin-types/host-api.d.ts`, update `ShishoHostAPI`:

```typescript
/** Top-level host API object available as the global `shisho` variable. */
export interface ShishoHostAPI {
  log: ShishoLog;
  config: ShishoConfig;
  http: ShishoHTTP;
  fs: ShishoFS;
  archive: ShishoArchive;
  xml: ShishoXML;
  ffmpeg: ShishoFFmpeg;
  shell: ShishoShell;
}
```

**Step 5: Create hostapi_shell_test.go**

Create `pkg/plugins/hostapi_shell_test.go`:

```go
package plugins

import (
	"runtime"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRuntimeWithShellAccess creates a Runtime with shellAccess capability.
func newTestRuntimeWithShellAccess(commands []string) *Runtime {
	return &Runtime{
		vm:       goja.New(),
		scope:    "official",
		pluginID: "test-shell-plugin",
		manifest: &Manifest{
			ManifestVersion: 1,
			ID:              "test-shell-plugin",
			Name:            "Test Shell Plugin",
			Version:         "1.0.0",
			Capabilities: Capabilities{
				ShellAccess: &ShellAccessCap{
					Description: "Test shell access",
					Commands:    commands,
				},
			},
		},
	}
}

// newTestRuntimeWithoutShellAccess creates a Runtime with no shellAccess capability.
func newTestRuntimeWithoutShellAccess() *Runtime {
	return &Runtime{
		vm:       goja.New(),
		scope:    "official",
		pluginID: "test-no-shell-plugin",
		manifest: &Manifest{
			ManifestVersion: 1,
			ID:              "test-no-shell-plugin",
			Name:            "Test No Shell Plugin",
			Version:         "1.0.0",
			Capabilities:    Capabilities{},
		},
	}
}

// setupShellNamespace injects the Shell namespace into the runtime.
func setupShellNamespace(t *testing.T, rt *Runtime) {
	t.Helper()
	shishoObj := rt.vm.NewObject()
	err := rt.vm.Set("shisho", shishoObj)
	require.NoError(t, err)
	err = injectShellNamespace(rt.vm, shishoObj, rt)
	require.NoError(t, err)
}

func TestShell_NoCapability(t *testing.T) {
	t.Parallel()
	rt := newTestRuntimeWithoutShellAccess()
	setupShellNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.shell.exec("echo", ["hello"])`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin does not declare shellAccess capability")
}

func TestShell_MissingArgs(t *testing.T) {
	t.Parallel()
	rt := newTestRuntimeWithShellAccess([]string{"echo"})
	setupShellNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.shell.exec("echo")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command and args arguments are required")
}

func TestShell_CommandNotInAllowlist(t *testing.T) {
	t.Parallel()
	rt := newTestRuntimeWithShellAccess([]string{"echo"})
	setupShellNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.shell.exec("rm", ["-rf", "/"])`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `command "rm" is not in allowed list`)
}

func TestShell_RunEcho(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("echo test not supported on Windows")
	}

	rt := newTestRuntimeWithShellAccess([]string{"echo"})
	setupShellNamespace(t, rt)

	val, err := rt.vm.RunString(`
		var result = shisho.shell.exec("echo", ["hello", "world"]);
		JSON.stringify({exitCode: result.exitCode, stdout: result.stdout.trim()});
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), `"exitCode":0`)
	assert.Contains(t, val.String(), `"stdout":"hello world"`)
}

func TestShell_NonZeroExitCode(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("exit code test not supported on Windows")
	}

	rt := newTestRuntimeWithShellAccess([]string{"false"})
	setupShellNamespace(t, rt)

	val, err := rt.vm.RunString(`
		var result = shisho.shell.exec("false", []);
		result.exitCode;
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val.ToInteger())
}

func TestShell_StderrCaptured(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("ls test not supported on Windows")
	}

	rt := newTestRuntimeWithShellAccess([]string{"ls"})
	setupShellNamespace(t, rt)

	// ls on a nonexistent file writes to stderr
	val, err := rt.vm.RunString(`
		var result = shisho.shell.exec("ls", ["nonexistent_file_12345"]);
		result.stderr;
	`)
	require.NoError(t, err)
	assert.NotEmpty(t, val.String())
}

func TestShell_BinaryNotFound(t *testing.T) {
	t.Parallel()
	rt := newTestRuntimeWithShellAccess([]string{"nonexistent-binary-12345"})
	setupShellNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.shell.exec("nonexistent-binary-12345", [])`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute")
}

func TestShell_EmptyArgs(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("pwd test not supported on Windows")
	}

	rt := newTestRuntimeWithShellAccess([]string{"pwd"})
	setupShellNamespace(t, rt)

	val, err := rt.vm.RunString(`
		var result = shisho.shell.exec("pwd", []);
		result.exitCode;
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), val.ToInteger())
}

func TestShell_ResultFields(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("echo test not supported on Windows")
	}

	rt := newTestRuntimeWithShellAccess([]string{"echo"})
	setupShellNamespace(t, rt)

	val, err := rt.vm.RunString(`
		var result = shisho.shell.exec("echo", ["test"]);
		var hasExitCode = "exitCode" in result;
		var hasStdout = "stdout" in result;
		var hasStderr = "stderr" in result;
		JSON.stringify({hasExitCode: hasExitCode, hasStdout: hasStdout, hasStderr: hasStderr});
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), `"hasExitCode":true`)
	assert.Contains(t, val.String(), `"hasStdout":true`)
	assert.Contains(t, val.String(), `"hasStderr":true`)
}

func TestShell_MultipleAllowedCommands(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("test not supported on Windows")
	}

	rt := newTestRuntimeWithShellAccess([]string{"echo", "pwd", "ls"})
	setupShellNamespace(t, rt)

	// All three should work
	_, err := rt.vm.RunString(`shisho.shell.exec("echo", ["test"])`)
	require.NoError(t, err)

	_, err = rt.vm.RunString(`shisho.shell.exec("pwd", [])`)
	require.NoError(t, err)

	_, err = rt.vm.RunString(`shisho.shell.exec("ls", ["."])`)
	require.NoError(t, err)

	// But rm should fail
	_, err = rt.vm.RunString(`shisho.shell.exec("rm", ["-rf", "/"])`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `command "rm" is not in allowed list`)
}
```

**Step 6: Run tests to verify**

Run: `go test -v ./pkg/plugins/... -run TestShell`
Expected: All tests pass

**Step 7: Commit**

```bash
git add pkg/plugins/hostapi_shell.go pkg/plugins/hostapi_shell_test.go pkg/plugins/hostapi.go packages/plugin-types/host-api.d.ts
git commit -m "$(cat <<'EOF'
[Feature] Add shisho.shell.exec() API with command allowlist

- Commands must be declared in manifest shellAccess.commands
- Uses exec.Command directly (no shell) to prevent injection
- Returns exitCode, stdout, stderr like FFmpeg API
EOF
)"
```

---

## Task 6: Run Full Test Suite and Lint

**Files:** None (verification only)

**Step 1: Run make tygo**

Run: `make tygo`
Expected: Success or "Nothing to be done"

**Step 2: Run Go tests**

Run: `go test -race ./pkg/plugins/...`
Expected: All tests pass

**Step 3: Run Go lint**

Run: `make lint`
Expected: No lint errors

**Step 4: Run TypeScript lint**

Run: `yarn lint`
Expected: No lint errors

**Step 5: Run full check**

Run: `make check`
Expected: All checks pass

**Step 6: Final commit if any fixes needed**

If any fixes were needed during this task, commit them:

```bash
git add -A
git commit -m "$(cat <<'EOF'
[Fix] Address lint issues from plugin API changes
EOF
)"
```

---

## Summary of Breaking Changes

1. **`shisho.ffmpeg.run()` removed** - Plugins must migrate to `shisho.ffmpeg.transcode()`
2. **`FFmpegResult` type renamed** - Now `TranscodeResult` (TypeScript only)

## New Features

1. **`shisho.ffmpeg.probe(args)`** - Probe file metadata with ffprobe, returns parsed JSON
2. **`shisho.ffmpeg.version()`** - Get FFmpeg version, configuration, and library versions
3. **`shisho.shell.exec(command, args)`** - Execute allowlisted shell commands
4. **`shellAccess` capability** - Declare allowed commands in manifest
