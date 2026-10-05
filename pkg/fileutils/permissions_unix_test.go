//go:build unix

package fileutils

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/testutils/testumask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file change the process umask, so none of them may call
// t.Parallel().

func TestWriteFileAtomic_ModeUnderDefaultUmask(t *testing.T) {
	testumask.Set(t, 0o022)
	dir := t.TempDir()

	public := filepath.Join(dir, "book.epub.cover.jpg")
	require.NoError(t, os.WriteFile(public, []byte("old"), 0o600))
	require.NoError(t, WriteFileAtomic(public, []byte("new"), 0o644))
	assertPerm(t, public, 0o644)

	private := filepath.Join(dir, "metadata.json")
	require.NoError(t, WriteFileAtomic(private, []byte("{}"), 0o600))
	assertPerm(t, private, 0o600)
}

func TestWriteFileAtomic_RestrictiveUmaskIsRespected(t *testing.T) {
	testumask.Set(t, 0o077)
	dir := t.TempDir()

	public := filepath.Join(dir, "book.epub.cover.jpg")
	require.NoError(t, WriteFileAtomic(public, []byte("new"), 0o644))
	assertPerm(t, public, 0o600)

	private := filepath.Join(dir, "metadata.json")
	require.NoError(t, WriteFileAtomic(private, []byte("{}"), 0o600))
	assertPerm(t, private, 0o600)
}

func TestCreateTemp_ModeComesFromCreation(t *testing.T) {
	for _, tc := range []struct {
		umask int
		perm  os.FileMode
		want  os.FileMode
	}{
		{0o022, 0o644, 0o644},
		{0o022, 0o640, 0o640},
		{0o022, 0o600, 0o600},
		{0o077, 0o644, 0o600},
	} {
		testumask.Set(t, tc.umask)
		f, err := CreateTemp(t.TempDir(), "page-*.tmp", tc.perm)
		require.NoError(t, err)
		require.NoError(t, f.Close())
		assertPerm(t, f.Name(), tc.want)
	}
}

func TestMkdirTemp_ModeComesFromCreation(t *testing.T) {
	testumask.Set(t, 0o022)
	dir, err := MkdirTemp(t.TempDir(), "package-", 0o755)
	require.NoError(t, err)
	assertPerm(t, dir, 0o755)

	testumask.Set(t, 0o077)
	dir, err = MkdirTemp(t.TempDir(), "package-", 0o755)
	require.NoError(t, err)
	assertPerm(t, dir, 0o700)
}

func TestCopyFile_CreatesDestinationWithSourcePermissions(t *testing.T) {
	for _, tc := range []struct {
		umask int
		perm  os.FileMode
		want  os.FileMode
	}{
		{0o022, 0o644, 0o644},
		{0o022, 0o640, 0o640},
		{0o022, 0o600, 0o600},
		{0o077, 0o644, 0o600},
	} {
		testumask.Set(t, tc.umask)
		dir := t.TempDir()
		src := filepath.Join(dir, "src")
		dst := filepath.Join(dir, "dst")
		require.NoError(t, os.WriteFile(src, []byte("data"), 0o600))
		require.NoError(t, os.Chmod(src, tc.perm))

		require.NoError(t, copyFile(src, dst))

		got, err := os.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, []byte("data"), got)
		assertPerm(t, dst, tc.want)
	}
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%#o", want), fmt.Sprintf("%#o", info.Mode().Perm()), "permissions of %s", filepath.Base(path))
}
