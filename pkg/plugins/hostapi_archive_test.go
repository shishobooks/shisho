package plugins

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestZip creates a ZIP file at the given path with the specified file entries.
func createTestZip(t *testing.T, zipPath string, entries map[string]string) {
	t.Helper()
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range entries {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
}

// createTestZipWithDirs creates a ZIP file that includes directory entries.
func createTestZipWithDirs(t *testing.T, zipPath string, dirs []string, files map[string]string) {
	t.Helper()
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	for _, dir := range dirs {
		_, err := w.Create(dir)
		require.NoError(t, err)
	}
	for name, content := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
}

func TestArchive_ExtractZip_ExtractsFiles(t *testing.T) {
	pluginDir := t.TempDir()

	// Create a zip file in the plugin dir
	zipPath := filepath.Join(pluginDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"hello.txt":      "Hello, World!",
		"sub/nested.txt": "Nested content",
	})

	destDir := filepath.Join(pluginDir, "output")
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err = rt.vm.RunString(`shisho.archive.extractZip("` + zipPath + `", "` + destDir + `")`)
	require.NoError(t, err)

	// Verify extracted files
	content, err := os.ReadFile(filepath.Join(destDir, "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", string(content))

	content, err = os.ReadFile(filepath.Join(destDir, "sub", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Nested content", string(content))
}

func TestArchive_ExtractZip_ZipSlipBlocked(t *testing.T) {
	pluginDir := t.TempDir()

	// Create a malicious zip with a path traversal entry
	zipPath := filepath.Join(pluginDir, "malicious.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	// Create entry with path traversal
	fw, err := w.Create("../../../etc/passwd")
	require.NoError(t, err)
	_, err = fw.Write([]byte("malicious content"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	destDir := filepath.Join(pluginDir, "output")
	err = os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err = rt.vm.RunString(`shisho.archive.extractZip("` + zipPath + `", "` + destDir + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zip slip detected")
}

func TestArchive_CreateZip_CreatesValidZip(t *testing.T) {
	pluginDir := t.TempDir()

	// Create source directory with files
	srcDir := filepath.Join(pluginDir, "source")
	err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644)
	require.NoError(t, err)

	destPath := filepath.Join(pluginDir, "output.zip")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err = rt.vm.RunString(`shisho.archive.createZip("` + srcDir + `", "` + destPath + `")`)
	require.NoError(t, err)

	// Verify the zip is valid by reading it back
	r, err := zip.OpenReader(destPath)
	require.NoError(t, err)
	defer r.Close()

	entries := make(map[string]bool)
	for _, f := range r.File {
		entries[f.Name] = true
	}

	assert.True(t, entries["file1.txt"], "should contain file1.txt")
	assert.True(t, entries["subdir/file2.txt"], "should contain subdir/file2.txt")
}

func TestArchive_ReadZipEntry_ReadsSpecificEntry(t *testing.T) {
	pluginDir := t.TempDir()

	zipPath := filepath.Join(pluginDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"first.txt":  "first content",
		"second.txt": "second content",
	})

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`
		var buf = shisho.archive.readZipEntry("` + zipPath + `", "second.txt");
		var view = new Uint8Array(buf);
		var result = "";
		for (var i = 0; i < view.length; i++) {
			result += String.fromCharCode(view[i]);
		}
		result;
	`)
	require.NoError(t, err)
	assert.Equal(t, "second content", val.String())
}

func TestArchive_ReadZipEntry_NonExistentEntry(t *testing.T) {
	pluginDir := t.TempDir()

	zipPath := filepath.Join(pluginDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"exists.txt": "content",
	})

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.archive.readZipEntry("` + zipPath + `", "nonexistent.txt")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in archive")
}

func TestArchive_ListZipEntries_ReturnsAllNames(t *testing.T) {
	pluginDir := t.TempDir()

	zipPath := filepath.Join(pluginDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"a.txt":     "a",
		"b.txt":     "b",
		"sub/c.txt": "c",
	})

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	val, err := rt.vm.RunString(`
		var entries = shisho.archive.listZipEntries("` + zipPath + `");
		entries.sort();
		JSON.stringify(entries);
	`)
	require.NoError(t, err)
	assert.Equal(t, `["a.txt","b.txt","sub/c.txt"]`, val.String())
}

func TestArchive_ExtractZip_ReadAccessDenied(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()

	// Create zip in external dir (not accessible)
	zipPath := filepath.Join(externalDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{"hello.txt": "hi"})

	destDir := filepath.Join(pluginDir, "output")

	// No fileAccess capability - external dir is not accessible
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.archive.extractZip("` + zipPath + `", "` + destDir + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read access denied")
}

func TestArchive_ExtractZip_WriteAccessDenied(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()

	// Create zip in plugin dir (readable)
	zipPath := filepath.Join(pluginDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{"hello.txt": "hi"})

	// Write to external dir (not writable without capability)
	destDir := filepath.Join(externalDir, "output")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.archive.extractZip("` + zipPath + `", "` + destDir + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write access denied")
}

func TestArchive_CreateZip_ReadAccessDenied(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()

	// Source in external dir (not readable)
	srcDir := filepath.Join(externalDir, "source")
	err := os.MkdirAll(srcDir, 0755)
	require.NoError(t, err)

	destPath := filepath.Join(pluginDir, "output.zip")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err = rt.vm.RunString(`shisho.archive.createZip("` + srcDir + `", "` + destPath + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read access denied")
}

func TestArchive_CreateZip_WriteAccessDenied(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()

	// Source in plugin dir (readable)
	srcDir := filepath.Join(pluginDir, "source")
	err := os.MkdirAll(srcDir, 0755)
	require.NoError(t, err)

	// Write to external dir (not writable)
	destPath := filepath.Join(externalDir, "output.zip")

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err = rt.vm.RunString(`shisho.archive.createZip("` + srcDir + `", "` + destPath + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write access denied")
}

func TestArchive_NoFSContext_DeniesAccess(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	// Don't set FSContext - all archive operations should deny access
	tests := []struct {
		name string
		call string
	}{
		{"extractZip", `shisho.archive.extractZip("/tmp/a.zip", "/tmp/out")`},
		{"createZip", `shisho.archive.createZip("/tmp/src", "/tmp/out.zip")`},
		{"readZipEntry", `shisho.archive.readZipEntry("/tmp/a.zip", "file.txt")`},
		{"listZipEntries", `shisho.archive.listZipEntries("/tmp/a.zip")`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := rt.vm.RunString(tt.call)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no filesystem context available")
		})
	}
}

func TestArchive_ExtractZip_WithDirectoryEntries(t *testing.T) {
	pluginDir := t.TempDir()

	zipPath := filepath.Join(pluginDir, "test.zip")
	createTestZipWithDirs(t, zipPath,
		[]string{"mydir/"},
		map[string]string{"mydir/file.txt": "in directory"},
	)

	destDir := filepath.Join(pluginDir, "output")
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err = rt.vm.RunString(`shisho.archive.extractZip("` + zipPath + `", "` + destDir + `")`)
	require.NoError(t, err)

	// Verify directory was created and file extracted
	info, err := os.Stat(filepath.Join(destDir, "mydir"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	content, err := os.ReadFile(filepath.Join(destDir, "mydir", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "in directory", string(content))
}

func TestArchive_ReadZipEntry_ReadAccessDenied(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()

	zipPath := filepath.Join(externalDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{"file.txt": "content"})

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.archive.readZipEntry("` + zipPath + `", "file.txt")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read access denied")
}

func TestArchive_ListZipEntries_ReadAccessDenied(t *testing.T) {
	pluginDir := t.TempDir()
	externalDir := t.TempDir()

	zipPath := filepath.Join(externalDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{"file.txt": "content"})

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.archive.listZipEntries("` + zipPath + `")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read access denied")
}

func TestArchive_FunctionsExist(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}
	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	funcs := []string{"extractZip", "createZip", "readZipEntry", "listZipEntries"}
	for _, fn := range funcs {
		val, err := rt.vm.RunString(`typeof shisho.archive.` + fn)
		require.NoError(t, err, "checking typeof shisho.archive.%s", fn)
		assert.Equal(t, "function", val.String(), "shisho.archive.%s should be a function", fn)
	}
}

func TestArchive_ExtractZip_WithAllowedPaths(t *testing.T) {
	pluginDir := t.TempDir()
	srcDir := t.TempDir()
	destDir := t.TempDir()

	// Create zip in srcDir
	zipPath := filepath.Join(srcDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{"hello.txt": "world"})

	// Both srcDir and destDir are in allowedPaths
	fsCtx := NewFSContext(pluginDir, []string{srcDir, destDir}, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.archive.extractZip("` + zipPath + `", "` + destDir + `")`)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(destDir, "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "world", string(content))
}

func TestArchive_CreateZip_RoundTrip(t *testing.T) {
	pluginDir := t.TempDir()

	// Create source files
	srcDir := filepath.Join(pluginDir, "src")
	err := os.MkdirAll(srcDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "data.txt"), []byte("round trip data"), 0644)
	require.NoError(t, err)

	zipPath := filepath.Join(pluginDir, "rt.zip")
	extractDir := filepath.Join(pluginDir, "extracted")
	err = os.MkdirAll(extractDir, 0755)
	require.NoError(t, err)

	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	// Create zip then extract it
	_, err = rt.vm.RunString(`
		shisho.archive.createZip("` + srcDir + `", "` + zipPath + `");
		shisho.archive.extractZip("` + zipPath + `", "` + extractDir + `");
	`)
	require.NoError(t, err)

	// Verify round-tripped content
	content, err := os.ReadFile(filepath.Join(extractDir, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "round trip data", string(content))
}

func TestArchive_ExtractZip_MissingArgs(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.archive.extractZip("only-one-arg")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "archivePath and destDir arguments are required")
}

func TestArchive_CreateZip_MissingArgs(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.archive.createZip("only-one-arg")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "srcDir and destPath arguments are required")
}

func TestArchive_ReadZipEntry_MissingArgs(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.archive.readZipEntry("only-one-arg")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "archivePath and entryPath arguments are required")
}

func TestArchive_ListZipEntries_MissingArgs(t *testing.T) {
	pluginDir := t.TempDir()
	fsCtx := NewFSContext(pluginDir, nil, nil)
	rt := newFSTestRuntime(t, fsCtx)

	_, err := rt.vm.RunString(`shisho.archive.listZipEntries()`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "archivePath argument is required")
}
