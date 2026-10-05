//go:build unix

package mp4_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/shishobooks/shisho/pkg/testutils/testumask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file change the process umask, so none of them may call
// t.Parallel().

func testM4B() []byte {
	source := append(testBox("ftyp", []byte("M4B ")), testBox("moov", testBox("free", []byte{0}))...)
	return append(source, testBox("mdat", []byte("audio payload"))...)
}

func TestWrite_CreatesRewriteWithSourceFileMode(t *testing.T) {
	for _, tc := range []struct {
		umask int
		perm  os.FileMode
		want  os.FileMode
	}{
		{0o022, 0o640, 0o640},
		{0o022, 0o644, 0o644},
		{0o022, 0o600, 0o600},
		{0o077, 0o644, 0o600},
	} {
		testumask.Set(t, tc.umask)
		path := filepath.Join(t.TempDir(), "source.m4b")
		require.NoError(t, os.WriteFile(path, testM4B(), 0o600))
		require.NoError(t, os.Chmod(path, tc.perm))

		require.NoError(t, mp4.Write(path, &mp4.Metadata{}, mp4.WriteOptions{}))

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, tc.want, info.Mode().Perm(), "umask %#o, source %#o", tc.umask, tc.perm)
	}
}

// WriteToFile builds download-cache output, which is created 0644 like the
// other generated formats whatever the source file's mode.
func TestWriteToFile_CreatesDestinationWithGeneratedFileMode(t *testing.T) {
	for _, tc := range []struct {
		umask int
		want  os.FileMode
	}{
		{0o022, 0o644},
		{0o077, 0o600},
	} {
		testumask.Set(t, tc.umask)
		dir := t.TempDir()
		srcPath := filepath.Join(dir, "source.m4b")
		destPath := filepath.Join(dir, "dest.m4b")
		require.NoError(t, os.WriteFile(srcPath, testM4B(), 0o600))

		require.NoError(t, mp4.WriteToFile(srcPath, destPath, &mp4.Metadata{}))

		info, err := os.Stat(destPath)
		require.NoError(t, err)
		assert.Equal(t, tc.want, info.Mode().Perm(), "umask %#o", tc.umask)
	}
}
