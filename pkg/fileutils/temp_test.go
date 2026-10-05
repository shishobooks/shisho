package fileutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sequence returns a name source that yields names in order.
func sequence(names ...string) func() string {
	i := 0
	return func() string {
		name := names[i%len(names)]
		i++
		return name
	}
}

func TestCreateTemp_ReplacesLastStarWithRandomName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	f, err := createTemp(dir, "a*b.jpg.*.tmp", 0o600, sequence("42"))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	assert.Equal(t, filepath.Join(dir, "a*b.jpg.42.tmp"), f.Name())

	f, err = createTemp(dir, ".partial-", 0o600, sequence("7"))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	assert.Equal(t, filepath.Join(dir, ".partial-7"), f.Name())
}

func TestCreateTemp_RetriesOnCollisionWithoutOpeningTheOtherFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	taken := filepath.Join(dir, "cover.jpg.1.tmp")
	require.NoError(t, os.WriteFile(taken, []byte("another writer"), 0o600))

	f, err := createTemp(dir, "cover.jpg.*.tmp", 0o600, sequence("1", "2"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "cover.jpg.2.tmp"), f.Name())
	_, err = f.WriteString("mine")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := os.ReadFile(taken)
	require.NoError(t, err)
	assert.Equal(t, "another writer", string(got))
}

func TestCreateTemp_GivesUpWhenEveryNameIsTaken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.1"), nil, 0o600))

	_, err := createTemp(dir, "x.*", 0o600, sequence("1"))
	require.ErrorIs(t, err, os.ErrExist)
}

func TestCreateTemp_RejectsPathSeparatorInPattern(t *testing.T) {
	t.Parallel()

	_, err := CreateTemp(t.TempDir(), "sub/x-*", 0o600)
	require.Error(t, err)
}

func TestMkdirTemp_RetriesOnCollision(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	taken := filepath.Join(dir, "package-1")
	require.NoError(t, os.Mkdir(taken, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(taken, "keep"), nil, 0o600))

	got, err := mkdirTemp(dir, "package-", 0o700, sequence("1", "2"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "package-2"), got)
	assert.FileExists(t, filepath.Join(taken, "keep"))
}
