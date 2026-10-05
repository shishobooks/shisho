//go:build unix

package plugins

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/pkg/testutils/testumask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file change the process umask, so none of them may call
// t.Parallel().

// modeTestWriteBoth writes dir/binary.bin with shisho.fs.writeFile and
// dir/text.txt with shisho.fs.writeTextFile, and returns the mode of each.
func modeTestWriteBoth(t *testing.T, fsCtx *FSContext, dir string) (binaryMode, textMode os.FileMode) {
	t.Helper()
	rt := newFSTestRuntime(t, fsCtx)
	binaryPath := filepath.Join(dir, "binary.bin")
	textPath := filepath.Join(dir, "text.txt")

	_, err := rt.vm.RunString(`
		shisho.fs.writeFile(` + strconv.Quote(binaryPath) + `, new Uint8Array([1, 2, 3]).buffer);
		shisho.fs.writeTextFile(` + strconv.Quote(textPath) + `, "content");
	`)
	require.NoError(t, err)

	return modeTestPerm(t, binaryPath), modeTestPerm(t, textPath)
}

func modeTestPerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

func TestFS_Write_HookProvidedDirIsWorldReadable(t *testing.T) {
	testumask.Set(t, 0o022)
	targetDir := t.TempDir()
	fsCtx := NewFSContext(t.TempDir(), t.TempDir(), []string{targetDir}, nil)

	binaryMode, textMode := modeTestWriteBoth(t, fsCtx, targetDir)
	assert.Equal(t, os.FileMode(0o644), binaryMode)
	assert.Equal(t, os.FileMode(0o644), textMode)
}

func TestFS_Write_HookProvidedFileIsWorldReadable(t *testing.T) {
	testumask.Set(t, 0o022)
	destPath := filepath.Join(t.TempDir(), "output.epub")
	fsCtx := NewFSContext(t.TempDir(), t.TempDir(), []string{destPath}, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.fs.writeTextFile(` + strconv.Quote(destPath) + `, "content")`)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), modeTestPerm(t, destPath))
}

func TestFS_Write_ReadwriteCapabilityPathIsWorldReadable(t *testing.T) {
	testumask.Set(t, 0o022)
	targetDir := t.TempDir()
	fsCtx := NewFSContext(t.TempDir(), t.TempDir(), nil, &FileAccessCap{Level: "readwrite"})

	binaryMode, textMode := modeTestWriteBoth(t, fsCtx, targetDir)
	assert.Equal(t, os.FileMode(0o644), binaryMode)
	assert.Equal(t, os.FileMode(0o644), textMode)
}

func TestFS_Write_PluginDirIsPrivate(t *testing.T) {
	testumask.Set(t, 0o022)
	pluginDir := t.TempDir()
	// The readwrite capability and a hook path covering the directory must not
	// widen the mode of the plugin's own files.
	fsCtx := NewFSContext(pluginDir, t.TempDir(), []string{pluginDir}, &FileAccessCap{Level: "readwrite"})

	binaryMode, textMode := modeTestWriteBoth(t, fsCtx, pluginDir)
	assert.Equal(t, os.FileMode(0o600), binaryMode)
	assert.Equal(t, os.FileMode(0o600), textMode)
}

func TestFS_Write_DataDirIsPrivate(t *testing.T) {
	testumask.Set(t, 0o022)
	dataDir := t.TempDir()
	fsCtx := NewFSContext(t.TempDir(), dataDir, nil, &FileAccessCap{Level: "readwrite"})

	binaryMode, textMode := modeTestWriteBoth(t, fsCtx, dataDir)
	assert.Equal(t, os.FileMode(0o600), binaryMode)
	assert.Equal(t, os.FileMode(0o600), textMode)
}

// The dev config sets relative plugin and data directories.
func TestFS_Write_RelativeDataDirIsPrivate(t *testing.T) {
	testumask.Set(t, 0o022)
	dataDir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	relDataDir, err := filepath.Rel(wd, dataDir)
	require.NoError(t, err)
	fsCtx := NewFSContext(t.TempDir(), relDataDir, nil, &FileAccessCap{Level: "readwrite"})

	binaryMode, textMode := modeTestWriteBoth(t, fsCtx, dataDir)
	assert.Equal(t, os.FileMode(0o600), binaryMode)
	assert.Equal(t, os.FileMode(0o600), textMode)
}

func TestFS_Write_TempDirIsPrivate(t *testing.T) {
	testumask.Set(t, 0o022)
	fsCtx := NewFSContext(t.TempDir(), t.TempDir(), nil, &FileAccessCap{Level: "readwrite"})
	t.Cleanup(func() { _ = fsCtx.Cleanup() })
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`shisho.fs.tempDir()`)
	require.NoError(t, err)
	tempDir := val.String()

	binaryMode, textMode := modeTestWriteBoth(t, fsCtx, tempDir)
	assert.Equal(t, os.FileMode(0o600), binaryMode)
	assert.Equal(t, os.FileMode(0o600), textMode)
}

func TestFS_Write_OverwriteKeepsExistingMode(t *testing.T) {
	testumask.Set(t, 0o022)
	targetDir := t.TempDir()
	for _, name := range []string{"binary.bin", "text.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(targetDir, name), []byte("old"), 0o600))
	}
	fsCtx := NewFSContext(t.TempDir(), t.TempDir(), []string{targetDir}, nil)

	binaryMode, textMode := modeTestWriteBoth(t, fsCtx, targetDir)
	assert.Equal(t, os.FileMode(0o600), binaryMode)
	assert.Equal(t, os.FileMode(0o600), textMode)
	content, err := os.ReadFile(filepath.Join(targetDir, "text.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestFS_Write_WorldReadableRequestHonorsUmask(t *testing.T) {
	testumask.Set(t, 0o077)
	targetDir := t.TempDir()
	fsCtx := NewFSContext(t.TempDir(), t.TempDir(), []string{targetDir}, nil)

	binaryMode, textMode := modeTestWriteBoth(t, fsCtx, targetDir)
	assert.Equal(t, os.FileMode(0o600), binaryMode)
	assert.Equal(t, os.FileMode(0o600), textMode)
}
