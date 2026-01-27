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
