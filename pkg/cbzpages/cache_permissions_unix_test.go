//go:build unix

package cbzpages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/testutils/testumask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test changes the process umask, so it must not call t.Parallel().
func TestCache_GetPage_CachedPageModeComesFromCreation(t *testing.T) {
	for umask, want := range map[int]os.FileMode{0o022: 0o644, 0o077: 0o600} {
		testumask.Set(t, umask)
		dir := t.TempDir()
		cbzPath := writeCBZ(t, dir, []byte("page"))
		c := NewCache(filepath.Join(dir, "cache"))

		path, _, err := c.GetPage(cbzPath, 1, 0)
		require.NoError(t, err)

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, want, info.Mode().Perm(), "umask %#o", umask)
	}
}
