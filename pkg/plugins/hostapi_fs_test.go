package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFSTestRuntime creates a Runtime with host APIs injected and a given FSContext set.
func newFSTestRuntime(t *testing.T, fsCtx *FSContext) *Runtime {
	t.Helper()
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)
	rt.SetFSContext(fsCtx)
	return rt
}

func TestFS_ReadTextFile_PluginDir(t *testing.T) {
	// Create a temp dir to act as the plugin directory
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "test.txt")
	err := os.WriteFile(testFile, []byte("hello world"), 0644)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.NoError(t, err)
	assert.Equal(t, "hello world", val.String())
}

func TestFS_ReadFile_PluginDir(t *testing.T) {
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "binary.bin")
	data := []byte{0x00, 0x01, 0x02, 0xFF}
	err := os.WriteFile(testFile, data, 0644)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	// Read the file and verify it returns an ArrayBuffer with correct data
	val, err := rt.vm.RunString(`
		var buf = shisho.fs.readFile("` + testFile + `");
		var view = new Uint8Array(buf);
		var result = [];
		for (var i = 0; i < view.length; i++) {
			result.push(view[i]);
		}
		JSON.stringify(result);
	`)
	require.NoError(t, err)
	assert.Equal(t, "[0,1,2,255]", val.String())
}

func TestFS_WriteTextFile_PluginDir(t *testing.T) {
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "output.txt")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.writeTextFile("` + testFile + `", "written content")`)
	require.NoError(t, err)

	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "written content", string(content))
}

func TestFS_WriteFile_PluginDir(t *testing.T) {
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "output.bin")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`
		var data = new Uint8Array([10, 20, 30, 40]);
		shisho.fs.writeFile("` + testFile + `", data.buffer);
	`)
	require.NoError(t, err)

	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, []byte{10, 20, 30, 40}, content)
}

func TestFS_WriteTextFile_ReadwriteAccess(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()
	testFile := filepath.Join(externalDir, "external.txt")

	fsCtx := NewFSContext(pluginDir, nil, &FileAccessCap{Level: "readwrite"})
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.writeTextFile("` + testFile + `", "readwrite content")`)
	require.NoError(t, err)

	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "readwrite content", string(content))
}

func TestFS_Write_DeniedWithoutFileAccess(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()
	testFile := filepath.Join(externalDir, "denied.txt")

	// No fileAccess capability
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.writeTextFile("` + testFile + `", "should fail")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestFS_Write_DeniedWithReadOnlyAccess(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()
	testFile := filepath.Join(externalDir, "denied.txt")

	// Read-only fileAccess
	fsCtx := NewFSContext(pluginDir, nil, &FileAccessCap{Level: "read"})
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.writeTextFile("` + testFile + `", "should fail")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestFS_Read_AllowedWithReadAccess(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()
	testFile := filepath.Join(externalDir, "readable.txt")
	err := os.WriteFile(testFile, []byte("external content"), 0644)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, &FileAccessCap{Level: "read"})
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.NoError(t, err)
	assert.Equal(t, "external content", val.String())
}

func TestFS_Exists_True(t *testing.T) {
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "exists.txt")
	err := os.WriteFile(testFile, []byte("hi"), 0644)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`shisho.fs.exists("` + testFile + `")`)
	require.NoError(t, err)
	assert.True(t, val.ToBoolean())
}

func TestFS_Exists_False(t *testing.T) {
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "nonexistent.txt")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`shisho.fs.exists("` + testFile + `")`)
	require.NoError(t, err)
	assert.False(t, val.ToBoolean())
}

func TestFS_Mkdir_PluginDir(t *testing.T) {
	pluginDir := t.TempDir()
	newDir := filepath.Join(pluginDir, "sub", "dir")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.mkdir("` + newDir + `")`)
	require.NoError(t, err)

	info, err := os.Stat(newDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestFS_Mkdir_DeniedOutsidePluginDir(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()
	newDir := filepath.Join(externalDir, "denied-dir")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.mkdir("` + newDir + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestFS_ListDir(t *testing.T) {
	pluginDir := t.TempDir()
	// Create some files
	err := os.WriteFile(filepath.Join(pluginDir, "a.txt"), []byte("a"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(pluginDir, "b.txt"), []byte("b"), 0644)
	require.NoError(t, err)
	err = os.Mkdir(filepath.Join(pluginDir, "subdir"), 0755)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`
		var entries = shisho.fs.listDir("` + pluginDir + `");
		entries.sort();
		JSON.stringify(entries);
	`)
	require.NoError(t, err)
	assert.Equal(t, `["a.txt","b.txt","subdir"]`, val.String())
}

func TestFS_TempDir_ConsistentPerInvocation(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	// Call tempDir twice - should return the same path
	val1, err := rt.vm.RunString(`shisho.fs.tempDir()`)
	require.NoError(t, err)

	val2, err := rt.vm.RunString(`shisho.fs.tempDir()`)
	require.NoError(t, err)

	assert.Equal(t, val1.String(), val2.String())
	assert.NotEmpty(t, val1.String())

	// Verify the directory exists
	info, err := os.Stat(val1.String())
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Cleanup
	err = fsCtx.Cleanup()
	require.NoError(t, err)

	// Verify directory is removed
	_, err = os.Stat(val1.String())
	assert.True(t, os.IsNotExist(err))
}

func TestFS_TempDir_WriteAllowed(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)
	defer fsCtx.Cleanup() //nolint:errcheck

	// Get the temp dir and write to it
	_, err := rt.vm.RunString(`
		var tmp = shisho.fs.tempDir();
		shisho.fs.writeTextFile(tmp + "/test.txt", "temp content");
	`)
	require.NoError(t, err)

	// Verify the file exists
	tmpPath := fsCtx.tempDir
	content, err := os.ReadFile(filepath.Join(tmpPath, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, "temp content", string(content))
}

func TestFS_AllowedPaths_ReadAccess(t *testing.T) {
	pluginDir := t.TempDir()
	allowedDir := t.TempDir()
	testFile := filepath.Join(allowedDir, "allowed.txt")
	err := os.WriteFile(testFile, []byte("allowed content"), 0644)
	require.NoError(t, err)

	// No fileAccess capability, but allowedDir is in allowedPaths
	fsCtx := NewFSContext(pluginDir, []string{allowedDir}, nil)
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.NoError(t, err)
	assert.Equal(t, "allowed content", val.String())
}

func TestFS_AllowedPaths_WriteAccess(t *testing.T) {
	pluginDir := t.TempDir()
	allowedDir := t.TempDir()
	testFile := filepath.Join(allowedDir, "allowed-write.txt")

	// No fileAccess capability, but allowedDir is in allowedPaths
	fsCtx := NewFSContext(pluginDir, []string{allowedDir}, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.writeTextFile("` + testFile + `", "allowed write")`)
	require.NoError(t, err)

	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "allowed write", string(content))
}

func TestFS_AllowedPaths_ExactFileMatch(t *testing.T) {
	pluginDir := t.TempDir()
	allowedDir := t.TempDir()
	allowedFile := filepath.Join(allowedDir, "specific.txt")
	err := os.WriteFile(allowedFile, []byte("specific content"), 0644)
	require.NoError(t, err)

	// Allow only the specific file
	fsCtx := NewFSContext(pluginDir, []string{allowedFile}, nil)
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`shisho.fs.readTextFile("` + allowedFile + `")`)
	require.NoError(t, err)
	assert.Equal(t, "specific content", val.String())
}

func TestFS_DeniedOutsideAllPaths(t *testing.T) {
	pluginDir := t.TempDir()
	deniedDir := t.TempDir()
	testFile := filepath.Join(deniedDir, "secret.txt")
	err := os.WriteFile(testFile, []byte("secret"), 0644)
	require.NoError(t, err)

	// No fileAccess, no allowedPaths matching deniedDir
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err = rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestFS_Read_DeniedOutsidePluginDir_NoFileAccess(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()
	testFile := filepath.Join(externalDir, "external.txt")
	err := os.WriteFile(testFile, []byte("external"), 0644)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err = rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestFS_NoFSContext_DeniesAllAccess(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	// Don't set FSContext - it should deny all access
	_, err = rt.vm.RunString(`shisho.fs.readTextFile("/tmp/test.txt")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no filesystem context available")
}

func TestFS_Exists_DeniedOutsidePluginDir(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()
	testFile := filepath.Join(externalDir, "secret.txt")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.exists("` + testFile + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestFS_ListDir_DeniedOutsidePluginDir(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.listDir("` + externalDir + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestFS_ReadFile_NonexistentFile(t *testing.T) {
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "nonexistent.txt")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such file")
}

func TestFS_WriteFile_InvalidData(t *testing.T) {
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "output.bin")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	// Pass a string instead of ArrayBuffer
	_, err := rt.vm.RunString(`shisho.fs.writeFile("` + testFile + `", "not an arraybuffer")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an ArrayBuffer")
}

func TestFS_ReadTextFile_Subdirectory(t *testing.T) {
	pluginDir := t.TempDir()
	subDir := filepath.Join(pluginDir, "sub", "nested")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)
	testFile := filepath.Join(subDir, "deep.txt")
	err = os.WriteFile(testFile, []byte("deep content"), 0644)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.NoError(t, err)
	assert.Equal(t, "deep content", val.String())
}

func TestFS_IsPathWithin(t *testing.T) {
	tests := []struct {
		name     string
		child    string
		parent   string
		expected bool
	}{
		{"exact match", "/foo/bar", "/foo/bar", true},
		{"child within", "/foo/bar/baz", "/foo/bar", true},
		{"not within", "/foo/baz", "/foo/bar", false},
		{"prefix overlap not within", "/foo/barBaz", "/foo/bar", false},
		{"parent is root", "/foo", "/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isPathWithin(tt.child, tt.parent))
		})
	}
}

func TestFS_SetFSContext_CanBeCleared(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)

	// Set context
	rt.SetFSContext(fsCtx)

	testFile := filepath.Join(pluginDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Should work
	_, err = rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.NoError(t, err)

	// Clear context
	rt.SetFSContext(nil)

	// Should fail now
	_, err = rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no filesystem context available")
}

func TestFS_Cleanup_NoTempDir(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)

	// Cleanup when no temp dir was created should be a no-op
	err := fsCtx.Cleanup()
	require.NoError(t, err)
}

func TestFS_WriteFile_Mkdir_WithReadwriteAccess(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()
	newDir := filepath.Join(externalDir, "new-dir")

	fsCtx := NewFSContext(pluginDir, nil, &FileAccessCap{Level: "readwrite"})
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.mkdir("` + newDir + `")`)
	require.NoError(t, err)

	info, err := os.Stat(newDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestFS_Mkdir_DeniedWithReadOnlyAccess(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()
	newDir := filepath.Join(externalDir, "new-dir")

	fsCtx := NewFSContext(pluginDir, nil, &FileAccessCap{Level: "read"})
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.mkdir("` + newDir + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestFS_TempDir_DifferentPerFSContext(t *testing.T) {
	pluginDir := t.TempDir()

	fsCtx1 := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx1)

	val1, err := rt.vm.RunString(`shisho.fs.tempDir()`)
	require.NoError(t, err)
	defer fsCtx1.Cleanup() //nolint:errcheck

	// Create a new FSContext (simulating a new hook invocation)
	fsCtx2 := NewFSContext(pluginDir, nil, nil)
	rt.SetFSContext(fsCtx2)

	val2, err := rt.vm.RunString(`shisho.fs.tempDir()`)
	require.NoError(t, err)
	defer fsCtx2.Cleanup() //nolint:errcheck

	// Different invocations should get different temp dirs
	assert.NotEqual(t, val1.String(), val2.String())
}

func TestFS_ReadFile_ReadwriteAccess(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()
	testFile := filepath.Join(externalDir, "readable.txt")
	err := os.WriteFile(testFile, []byte("readwrite content"), 0644)
	require.NoError(t, err)

	// readwrite also grants read access
	fsCtx := NewFSContext(pluginDir, nil, &FileAccessCap{Level: "readwrite"})
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.NoError(t, err)
	assert.Equal(t, "readwrite content", val.String())
}

func TestFS_NewFSContext_NilAllowedPaths(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	assert.NotNil(t, fsCtx)
	assert.Equal(t, pluginDir, fsCtx.pluginDir)
	assert.Nil(t, fsCtx.allowedPaths)
	assert.Nil(t, fsCtx.fileAccessCap)
}

func TestFS_ReadFile_NoArgument(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.readFile()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path argument is required")
}

func TestFS_ReadTextFile_NoArgument(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.readTextFile()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path argument is required")
}

func TestFS_WriteTextFile_NoArguments(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.writeTextFile()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path and content arguments are required")
}

func TestFS_WriteFile_NoArguments(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.writeFile()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path and data arguments are required")
}

func TestFS_Exists_NoArgument(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	// goja converts missing args to "undefined" string, which will fail path check
	_, err := rt.vm.RunString(`shisho.fs.exists()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path argument is required")
}

func TestFS_Mkdir_NoArgument(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.mkdir()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path argument is required")
}

func TestFS_ListDir_NoArgument(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.listDir()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path argument is required")
}

// Verify that the fs namespace functions properly check argument count
// by using the same pattern as existing host APIs.
func TestFS_FunctionsExist(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	// Verify all expected functions exist on shisho.fs
	funcs := []string{"readFile", "readTextFile", "writeFile", "writeTextFile", "exists", "mkdir", "listDir", "tempDir"}
	for _, fn := range funcs {
		val, err := rt.vm.RunString(`typeof shisho.fs.` + fn)
		require.NoError(t, err, "checking typeof shisho.fs.%s", fn)
		assert.Equal(t, "function", val.String(), "shisho.fs.%s should be a function", fn)
	}
}

func TestFS_AllowedPaths_SubdirectoryAccess(t *testing.T) {
	pluginDir := t.TempDir()
	allowedDir := t.TempDir()
	subDir := filepath.Join(allowedDir, "sub")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)
	testFile := filepath.Join(subDir, "nested.txt")
	err = os.WriteFile(testFile, []byte("nested"), 0644)
	require.NoError(t, err)

	// Allow the parent directory - subdirectory files should also be accessible
	fsCtx := NewFSContext(pluginDir, []string{allowedDir}, nil)
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`shisho.fs.readTextFile("` + testFile + `")`)
	require.NoError(t, err)
	assert.Equal(t, "nested", val.String())
}

func TestFS_WriteFile_Uint8Array(t *testing.T) {
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "uint8.bin")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	// Create a Uint8Array and pass its buffer
	_, err := rt.vm.RunString(`
		var arr = new Uint8Array([65, 66, 67]); // "ABC"
		shisho.fs.writeFile("` + testFile + `", arr.buffer);
	`)
	require.NoError(t, err)

	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "ABC", string(content))
}

func TestFS_ReadFile_RoundTrip(t *testing.T) {
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "roundtrip.bin")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	// Write binary data and read it back, verify equality
	val, err := rt.vm.RunString(`
		(function() {
			var original = new Uint8Array([0, 128, 255, 1, 2, 3]);
			shisho.fs.writeFile("` + testFile + `", original.buffer);
			var readBack = shisho.fs.readFile("` + testFile + `");
			var view = new Uint8Array(readBack);
			if (view.length !== original.length) return false;
			for (var i = 0; i < view.length; i++) {
				if (view[i] !== original[i]) return false;
			}
			return true;
		})();
	`)
	require.NoError(t, err)
	assert.True(t, val.ToBoolean())
}
