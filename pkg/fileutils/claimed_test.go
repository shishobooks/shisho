package fileutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func claimedPaths(paths ...string) func(string) bool {
	return func(path string) bool {
		for _, p := range paths {
			if p == path {
				return true
			}
		}
		return false
	}
}

// A path another record claims counts as taken even when nothing is on disk
// there, so organizing never moves a file onto a path the database already
// assigns to a different file.
func TestOrganizeRootLevelFile_SkipsClaimedPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	original := filepath.Join(root, "download.epub")
	writeTestFile(t, original, "book")

	claimed := filepath.Join(root, "[Emily Henry] Beach Read", "Beach Read.epub")
	result, err := OrganizeRootLevelFile(original, OrganizedNameOptions{
		AuthorNames: []string{"Emily Henry"},
		Title:       "Beach Read",
		FileType:    "epub",
		Claimed:     claimedPaths(claimed),
	})
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(root, "[Emily Henry] Beach Read", "Beach Read (1).epub"), result.NewPath)
	assert.FileExists(t, result.NewPath)
	assert.NoFileExists(t, claimed)
}

func TestRenameOrganizedFile_SkipsClaimedPath(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "[Emily Henry] Beach Read")
	current := filepath.Join(dir, "old.epub")
	writeTestFile(t, current, "book")

	claimed := filepath.Join(dir, "Beach Read.epub")
	opts := OrganizedNameOptions{Title: "Beach Read", FileType: "epub", Claimed: claimedPaths(claimed)}

	newPath, err := RenameOrganizedFile(current, opts)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "Beach Read (1).epub"), newPath)

	writeTestFile(t, current, "book")
	newPath, err = RenameOrganizedFileOnly(current, opts)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "Beach Read (2).epub"), newPath, "the on-disk (1) and the claimed path are both skipped")
}

func TestRenameOrganizedFolder_SkipsClaimedPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	current := filepath.Join(root, "old folder")
	require.NoError(t, os.Mkdir(current, 0755))

	claimed := filepath.Join(root, "[Emily Henry] Beach Read")
	newPath, err := RenameOrganizedFolder(current, OrganizedNameOptions{
		AuthorNames: []string{"Emily Henry"},
		Title:       "Beach Read",
		FileType:    "epub",
		Claimed:     claimedPaths(claimed),
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "[Emily Henry] Beach Read (1)"), newPath)
	assert.DirExists(t, newPath)
}

func TestUndoOrganizedMove_RestoresFileAndAssociatedFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	original := filepath.Join(root, "download.epub")
	writeTestFile(t, original, "book")
	writeTestFile(t, original+".cover.jpg", "cover")
	writeTestFile(t, original+".metadata.json", "{}")

	result, err := OrganizeRootLevelFile(original, OrganizedNameOptions{
		AuthorNames: []string{"Emily Henry"},
		Title:       "Beach Read",
		FileType:    "epub",
	})
	require.NoError(t, err)
	require.True(t, result.Moved)
	require.NoFileExists(t, original)

	require.NoError(t, UndoOrganizedMove(result.OriginalPath, result.NewPath, true))

	assert.FileExists(t, original)
	assert.FileExists(t, original+".cover.jpg")
	assert.FileExists(t, original+".metadata.json")
	assert.NoDirExists(t, filepath.Dir(result.NewPath), "the folder organize created is removed once empty")
}

func TestUndoOrganizedMove_RecreatesRemovedSourceDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	original := filepath.Join(root, "nested", "old.epub")
	writeTestFile(t, original, "book")
	moved := filepath.Join(root, "organized", "new.epub")
	require.NoError(t, os.MkdirAll(filepath.Dir(moved), 0755))
	require.NoError(t, os.Rename(original, moved))
	require.NoError(t, os.Remove(filepath.Dir(original)))

	require.NoError(t, UndoOrganizedMove(original, moved, false))
	assert.FileExists(t, original)
}

// A file already at a numbered name, because its organized name is claimed,
// stays put on the next organize instead of moving to the next number.
func TestRenameOrganizedFile_KeepsNumberedNameWhenBaseIsClaimed(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "[Emily Henry] Beach Read")
	current := filepath.Join(dir, "Beach Read (1).epub")
	writeTestFile(t, current, "book")

	opts := OrganizedNameOptions{Title: "Beach Read", FileType: "epub", Claimed: claimedPaths(filepath.Join(dir, "Beach Read.epub"))}
	newPath, err := RenameOrganizedFile(current, opts)
	require.NoError(t, err)
	assert.Equal(t, current, newPath)
	newPath, err = RenameOrganizedFileOnly(current, opts)
	require.NoError(t, err)
	assert.Equal(t, current, newPath)
}

func TestRenameOrganizedFolder_KeepsNumberedNameWhenBaseIsClaimed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	current := filepath.Join(root, "[Emily Henry] Beach Read (1)")
	require.NoError(t, os.Mkdir(current, 0755))

	newPath, err := RenameOrganizedFolder(current, OrganizedNameOptions{
		AuthorNames: []string{"Emily Henry"},
		Title:       "Beach Read",
		FileType:    "epub",
		Claimed:     claimedPaths(filepath.Join(root, "[Emily Henry] Beach Read")),
	})
	require.NoError(t, err)
	assert.Equal(t, current, newPath)
}
