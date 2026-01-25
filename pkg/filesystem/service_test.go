package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowse_EmptyDirectory(t *testing.T) {
	t.Parallel()
	// Create a temporary empty directory.
	tempDir := t.TempDir()
	emptyDir := filepath.Join(tempDir, "empty")
	err := os.Mkdir(emptyDir, 0755)
	require.NoError(t, err)

	// Resolve symlinks for comparison (macOS /var -> /private/var).
	resolvedEmptyDir, err := filepath.EvalSymlinks(emptyDir)
	require.NoError(t, err)
	resolvedTempDir, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)

	// Verify the directory is actually empty.
	entries, err := os.ReadDir(emptyDir)
	require.NoError(t, err)
	require.Empty(t, entries, "test directory should be empty")

	// Browse the empty directory.
	svc := NewService()
	resp, err := svc.Browse(BrowseOptions{
		Path:  emptyDir,
		Limit: 50,
	})
	require.NoError(t, err)

	// Verify the response.
	assert.Equal(t, resolvedEmptyDir, resp.CurrentPath)
	assert.Equal(t, resolvedTempDir, resp.ParentPath)
	assert.Equal(t, 0, resp.Total)
	assert.False(t, resp.HasMore)

	// Critical: Entries should be an empty slice, not nil.
	// This is important for JSON serialization - nil becomes null, [] becomes [].
	assert.NotNil(t, resp.Entries, "Entries should not be nil for empty directories")
	assert.Empty(t, resp.Entries, "Entries should be empty for empty directories")
}

func TestBrowse_NonEmptyDirectory(t *testing.T) {
	t.Parallel()
	// Create a temporary directory with some files.
	tempDir := t.TempDir()

	// Resolve symlinks for comparison (macOS /var -> /private/var).
	resolvedTempDir, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)

	// Create a subdirectory and a file.
	subDir := filepath.Join(tempDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	file := filepath.Join(tempDir, "file.txt")
	err = os.WriteFile(file, []byte("test"), 0644)
	require.NoError(t, err)

	// Browse the directory.
	svc := NewService()
	resp, err := svc.Browse(BrowseOptions{
		Path:  tempDir,
		Limit: 50,
	})
	require.NoError(t, err)

	// Verify the response.
	assert.Equal(t, resolvedTempDir, resp.CurrentPath)
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Entries, 2)

	// Directories should come first.
	assert.Equal(t, "subdir", resp.Entries[0].Name)
	assert.True(t, resp.Entries[0].IsDir)
	assert.Equal(t, "file.txt", resp.Entries[1].Name)
	assert.False(t, resp.Entries[1].IsDir)
}
