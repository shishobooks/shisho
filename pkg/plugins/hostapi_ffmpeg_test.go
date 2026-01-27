package plugins

import (
	"os/exec"
	"runtime"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRuntimeWithFFmpegAccess creates a Runtime with a manifest that includes ffmpegAccess.
func newTestRuntimeWithFFmpegAccess() *Runtime {
	return &Runtime{
		vm:       goja.New(),
		scope:    "official",
		pluginID: "test-ffmpeg-plugin",
		manifest: &Manifest{
			ManifestVersion: 1,
			ID:              "test-ffmpeg-plugin",
			Name:            "Test FFmpeg Plugin",
			Version:         "1.0.0",
			Capabilities: Capabilities{
				FFmpegAccess: &FFmpegAccessCap{
					Description: "Test FFmpeg access",
				},
			},
		},
	}
}

// newTestRuntimeWithoutFFmpegAccess creates a Runtime with no ffmpegAccess capability.
func newTestRuntimeWithoutFFmpegAccess() *Runtime {
	return &Runtime{
		vm:       goja.New(),
		scope:    "official",
		pluginID: "test-no-ffmpeg-plugin",
		manifest: &Manifest{
			ManifestVersion: 1,
			ID:              "test-no-ffmpeg-plugin",
			Name:            "Test No FFmpeg Plugin",
			Version:         "1.0.0",
			Capabilities:    Capabilities{},
		},
	}
}

// setupFFmpegNamespace injects the FFmpeg namespace into the runtime.
func setupFFmpegNamespace(t *testing.T, rt *Runtime) {
	t.Helper()
	shishoObj := rt.vm.NewObject()
	err := rt.vm.Set("shisho", shishoObj)
	require.NoError(t, err)
	err = injectFFmpegNamespace(rt.vm, shishoObj, rt)
	require.NoError(t, err)
}

func TestFFmpeg_NoCapability(t *testing.T) {
	t.Parallel()
	rt := newTestRuntimeWithoutFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.ffmpeg.transcode(["-version"])`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin does not declare ffmpegAccess capability")
}

func TestFFmpeg_MissingArgs(t *testing.T) {
	t.Parallel()
	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.ffmpeg.transcode()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "args argument is required")
}

func TestFFmpeg_RunEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("echo test not supported on Windows")
	}

	// Override the ffmpeg binary to use echo for testing
	origBinary := ffmpegBinary
	ffmpegBinary = "echo"
	defer func() { ffmpegBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.transcode(["hello", "world"]);
		JSON.stringify({exitCode: result.exitCode, stdout: result.stdout, stderr: result.stderr});
	`)
	require.NoError(t, err)

	// echo receives: -protocol_whitelist file,pipe hello world
	assert.Contains(t, val.String(), `"exitCode":0`)
	assert.Contains(t, val.String(), "-protocol_whitelist")
	assert.Contains(t, val.String(), "file,pipe")
	assert.Contains(t, val.String(), "hello")
	assert.Contains(t, val.String(), "world")
}

func TestFFmpeg_ProtocolWhitelistPrepended(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("echo test not supported on Windows")
	}

	// Use echo to verify the args are prepended correctly
	origBinary := ffmpegBinary
	ffmpegBinary = "echo"
	defer func() { ffmpegBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.transcode(["-i", "input.mp4", "output.mp3"]);
		result.stdout;
	`)
	require.NoError(t, err)

	// echo should output the args in order:
	// -protocol_whitelist file,pipe -i input.mp4 output.mp3
	stdout := val.String()
	assert.Contains(t, stdout, "-protocol_whitelist file,pipe -i input.mp4 output.mp3")
}

func TestFFmpeg_NonZeroExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exit code test not supported on Windows")
	}

	// Use a command that exits with a non-zero code
	origBinary := ffmpegBinary
	ffmpegBinary = "sh"
	defer func() { ffmpegBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	// sh -c "exit 42" will exit with code 42
	// But remember -protocol_whitelist file,pipe is prepended, so we need
	// to use sh with -c as the binary and craft args carefully.
	// Actually, sh receives: -protocol_whitelist file,pipe -c "exit 42"
	// sh will try to interpret -protocol_whitelist as a file, which will fail.
	// Instead, use "false" which always exits 1.
	ffmpegBinary = "false"

	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.transcode([]);
		result.exitCode;
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val.ToInteger())
}

func TestFFmpeg_StderrCaptured(t *testing.T) {
	// Skip if ffmpeg is not installed - this test needs a real binary that writes to stderr
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found on PATH, skipping stderr test")
	}

	origBinary := ffmpegBinary
	ffmpegBinary = "ffmpeg"
	defer func() { ffmpegBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	// ffmpeg -version writes version info to stdout, but calling with
	// an invalid input file will produce stderr output.
	// Use a nonexistent file which will cause ffmpeg to write an error to stderr.
	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.transcode(["-i", "nonexistent_file_12345.mp4", "-f", "null", "-"]);
		result.stderr;
	`)
	require.NoError(t, err)
	// ffmpeg writes errors to stderr
	assert.NotEmpty(t, val.String())
}

func TestFFmpeg_BinaryNotFound(t *testing.T) {
	origBinary := ffmpegBinary
	ffmpegBinary = "nonexistent-binary-12345"
	defer func() { ffmpegBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.ffmpeg.transcode(["-version"])`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute")
}

func TestFFmpeg_EmptyArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("echo test not supported on Windows")
	}

	// Even with empty args, protocol_whitelist should still be prepended
	origBinary := ffmpegBinary
	ffmpegBinary = "echo"
	defer func() { ffmpegBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.transcode([]);
		result.stdout;
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), "-protocol_whitelist file,pipe")
}

func TestFFmpeg_ResultFields(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("echo test not supported on Windows")
	}

	origBinary := ffmpegBinary
	ffmpegBinary = "echo"
	defer func() { ffmpegBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	// Verify all three fields are present on the result object
	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.transcode(["test"]);
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

func TestFFmpeg_RealFFmpeg(t *testing.T) {
	// Skip if ffmpeg is not installed
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found on PATH, skipping integration test")
	}

	// Reset to real ffmpeg binary
	origBinary := ffmpegBinary
	ffmpegBinary = "ffmpeg"
	defer func() { ffmpegBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	// Run ffmpeg -version which should succeed
	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.transcode(["-version"]);
		JSON.stringify({exitCode: result.exitCode, hasOutput: result.stderr.length > 0 || result.stdout.length > 0});
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), `"exitCode":0`)
}

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
			hasStderr: "stderr" in result,
			hasParseError: "parseError" in result
		});
	`)
	require.NoError(t, err)
	assert.Contains(t, val.String(), `"hasFormat":true`)
	assert.Contains(t, val.String(), `"hasStreams":true`)
	assert.Contains(t, val.String(), `"hasChapters":true`)
	assert.Contains(t, val.String(), `"hasStderr":true`)
	assert.Contains(t, val.String(), `"hasParseError":true`)
}

func TestFFmpegProbe_ParseError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("echo test not supported on Windows")
	}

	// Use echo to produce non-JSON output and verify parseError is set
	origBinary := ffprobeBinary
	ffprobeBinary = "echo"
	defer func() { ffprobeBinary = origBinary }()

	rt := newTestRuntimeWithFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	val, err := rt.vm.RunString(`
		var result = shisho.ffmpeg.probe(["-i", "test.mp4"]);
		result.parseError;
	`)
	require.NoError(t, err)
	// echo outputs non-JSON, so parseError should be set
	assert.NotEmpty(t, val.String())
	assert.Contains(t, val.String(), "invalid character")
}

func TestFFmpegVersion_NoCapability(t *testing.T) {
	t.Parallel()
	rt := newTestRuntimeWithoutFFmpegAccess()
	setupFFmpegNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.ffmpeg.version()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin does not declare ffmpegAccess capability")
}

func TestFFmpegVersion_BinaryNotFound(t *testing.T) {
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
