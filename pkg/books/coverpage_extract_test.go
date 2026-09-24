package books

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newExtractCoverFixture creates a scanned-looking CBZ with JPEG pages and an
// existing PNG cover next to it, so a replacement lands at a different
// extension than the previous cover.
func newExtractCoverFixture(t *testing.T) (file *models.File, bookDir, prevPath string, prevBytes []byte) {
	t.Helper()
	bookDir = t.TempDir()
	cbzPath := filepath.Join(bookDir, "book.cbz")
	createTestCBZWithPages(t, cbzPath, 5)
	prevPath = filepath.Join(bookDir, "book.cbz.cover.png")
	prevBytes = []byte("previous png cover")
	require.NoError(t, os.WriteFile(prevPath, prevBytes, 0600))
	file = &models.File{ID: 1, BookID: 1, Filepath: cbzPath, FileType: models.FileTypeCBZ}
	return file, bookDir, prevPath, prevBytes
}

// The previous cover may only be removed once the replacement is installed.
// A write that fails part way through must leave the old bytes readable.
func TestExtractCoverPageToFile_FailedInstallKeepsPreviousCover(t *testing.T) {
	t.Parallel()

	file, bookDir, prevPath, prevBytes := newExtractCoverFixture(t)
	// A nonempty directory at the destination makes the install fail
	// deterministically: it can be neither created over nor removed.
	obstruction := filepath.Join(bookDir, "book.cbz.cover.jpg")
	require.NoError(t, os.Mkdir(obstruction, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(obstruction, "keep"), []byte("x"), 0600))

	_, _, err := ExtractCoverPageToFile(file, bookDir, 2, cbzpages.NewCache(t.TempDir()), nil, logger.New())
	require.Error(t, err)

	got, readErr := os.ReadFile(prevPath)
	require.NoError(t, readErr, "the previous cover must still exist after a failed install")
	assert.Equal(t, prevBytes, got, "the previous cover bytes must be untouched")
	leftovers, globErr := filepath.Glob(filepath.Join(bookDir, "book.cbz.cover.*"))
	require.NoError(t, globErr)
	assert.ElementsMatch(t, []string{prevPath, obstruction}, leftovers, "no temporary file may be left behind")
}

func TestExtractCoverPageToFile_ReplacesPreviousCoverAfterInstall(t *testing.T) {
	t.Parallel()

	file, bookDir, prevPath, _ := newExtractCoverFixture(t)

	filename, mimeType, err := ExtractCoverPageToFile(file, bookDir, 2, cbzpages.NewCache(t.TempDir()), nil, logger.New())
	require.NoError(t, err)
	assert.Equal(t, "book.cbz.cover.jpg", filename)
	assert.Equal(t, "image/jpeg", mimeType)

	_, statErr := os.Stat(prevPath)
	assert.True(t, os.IsNotExist(statErr), "the previous cover must be removed after a successful install")
	entries, globErr := filepath.Glob(filepath.Join(bookDir, "book.cbz.cover.*"))
	require.NoError(t, globErr)
	assert.Equal(t, []string{filepath.Join(bookDir, "book.cbz.cover.jpg")}, entries)
}
