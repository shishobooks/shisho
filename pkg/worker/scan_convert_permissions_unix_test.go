//go:build unix

package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testumask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests change the process umask, so they must not call t.Parallel().

// An input converter writes into the hook's targetDir, and the scanner moves
// the result into the library with fileutils.MoveFile, so the converted book
// keeps the mode the plugin's write created it with (a cross-device copy uses
// the source's permission bits, see fileutils' copyFile tests).
func TestScanWithPluginInputConverter_ConvertedBookIsWorldReadable(t *testing.T) {
	testumask.Set(t, 0o022)
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	manifest := `{
  "manifestVersion": 1,
  "id": "mode-converter",
  "name": "Mode Converter",
  "version": "1.0.0",
  "capabilities": {
    "inputConverter": {
      "description": "Converts modeconv to epub",
      "sourceTypes": ["modeconv"],
      "targetType": "epub"
    }
  }
}`
	mainJS := `var plugin = (function() {
  return {
    inputConverter: {
      convert: function(ctx) {
        var targetPath = ctx.targetDir + "/converted.epub";
        shisho.fs.writeTextFile(targetPath, shisho.fs.readTextFile(ctx.sourcePath));
        return { success: true, targetPath: targetPath };
      }
    }
  };
})();`
	installTestPlugin(t, tc, pluginDir, "mode-converter", manifest, mainJS)
	require.NoError(t, tc.worker.pluginService.AppendToOrder(context.Background(), models.PluginHookInputConverter, "test", "mode-converter"))
	require.NoError(t, tc.worker.pluginManager.LoadAll(context.Background()))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := filepath.Join(libraryPath, "Converted Book")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "myfile.modeconv"), []byte("content"), 0644))

	require.NoError(t, tc.runScan())

	info, err := os.Stat(filepath.Join(bookDir, "converted.epub"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}
