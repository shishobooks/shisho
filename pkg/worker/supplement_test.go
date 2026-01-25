package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessScanJob_SupplementsInDirectory(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with main file + supplements
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{
		Title:    "My Book",
		HasCover: true,
	})

	// Create supplement files
	supplementPDF := filepath.Join(bookDir, "companion.pdf")
	require.NoError(t, os.WriteFile(supplementPDF, []byte("PDF content"), 0644))

	supplementTXT := filepath.Join(bookDir, "notes.txt")
	require.NoError(t, os.WriteFile(supplementTXT, []byte("Notes content"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	// Verify book was created
	books := tc.listBooks()
	require.Len(t, books, 1)

	// Verify files: 1 main + 2 supplements
	files := tc.listFiles()
	require.Len(t, files, 3)

	mainFiles := 0
	supplementFiles := 0
	for _, f := range files {
		switch f.FileRole {
		case models.FileRoleMain:
			mainFiles++
			assert.Equal(t, "m4b", f.FileType)
		case models.FileRoleSupplement:
			supplementFiles++
			assert.Contains(t, []string{"pdf", "txt"}, f.FileType)
		}
	}
	assert.Equal(t, 1, mainFiles)
	assert.Equal(t, 2, supplementFiles)
}

func TestProcessScanJob_SupplementsExcludeHiddenFiles(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{})

	// Create hidden file (should be excluded)
	hiddenFile := filepath.Join(bookDir, ".hidden")
	require.NoError(t, os.WriteFile(hiddenFile, []byte("hidden"), 0644))

	// Create .DS_Store (should be excluded)
	dsStore := filepath.Join(bookDir, ".DS_Store")
	require.NoError(t, os.WriteFile(dsStore, []byte("dsstore"), 0644))

	// Create normal supplement (should be included)
	supplement := filepath.Join(bookDir, "guide.pdf")
	require.NoError(t, os.WriteFile(supplement, []byte("guide"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	// Should have 2: main epub + guide.pdf supplement
	// Hidden files and .DS_Store should be excluded
	require.Len(t, files, 2)

	for _, f := range files {
		assert.NotContains(t, f.Filepath, ".hidden")
		assert.NotContains(t, f.Filepath, ".DS_Store")
	}
}

func TestProcessScanJob_SupplementsExcludeShishoFiles(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{})

	// Create shisho special files (should be excluded)
	coverFile := filepath.Join(bookDir, "book.cover.jpg")
	require.NoError(t, os.WriteFile(coverFile, []byte("cover"), 0644))

	metadataFile := filepath.Join(bookDir, "book.metadata.json")
	require.NoError(t, os.WriteFile(metadataFile, []byte("{}"), 0644))

	// Create normal supplement
	supplement := filepath.Join(bookDir, "appendix.pdf")
	require.NoError(t, os.WriteFile(supplement, []byte("appendix"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	// Should have 2: main epub + appendix.pdf
	require.Len(t, files, 2)

	for _, f := range files {
		assert.NotContains(t, f.Filepath, ".cover.")
		assert.NotContains(t, f.Filepath, ".metadata.json")
	}
}

func TestProcessScanJob_SupplementsInSubdirectory(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{})

	// Create subdirectory with supplements
	subDir := testgen.CreateSubDir(t, bookDir, "extras")
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "bonus.pdf"), []byte("bonus"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "artwork.jpg"), []byte("art"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	// Should have 3: main m4b + 2 supplements in subdirectory
	require.Len(t, files, 3)

	supplementCount := 0
	for _, f := range files {
		if f.FileRole == models.FileRoleSupplement {
			supplementCount++
		}
	}
	assert.Equal(t, 2, supplementCount)
}

func TestProcessScanJob_RootLevelSupplements(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create root-level main file
	testgen.GenerateM4B(t, libraryPath, "My Book.m4b", testgen.M4BOptions{})

	// Create supplement with matching basename
	supplement := filepath.Join(libraryPath, "My Book.pdf")
	require.NoError(t, os.WriteFile(supplement, []byte("supplement"), 0644))

	// Create unrelated file (different basename - should NOT be picked up)
	unrelated := filepath.Join(libraryPath, "Other Book.pdf")
	require.NoError(t, os.WriteFile(unrelated, []byte("other"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	// Should have 1 book: "My Book" (Other Book.pdf has no main file so is ignored)
	books := tc.listBooks()
	require.Len(t, books, 1, "Only My Book should exist, Other Book.pdf doesn't have a main file")

	files := tc.listFiles()
	// My Book.m4b (main) + My Book.pdf (supplement)
	require.Len(t, files, 2)

	mainCount := 0
	suppCount := 0
	for _, f := range files {
		if f.FileRole == models.FileRoleMain {
			mainCount++
		} else {
			suppCount++
		}
	}
	assert.Equal(t, 1, mainCount)
	assert.Equal(t, 1, suppCount)
}
