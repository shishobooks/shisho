package fileutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoverExistsWithBaseName(t *testing.T) {
	t.Parallel()
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "cover-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	tests := []struct {
		name           string
		existingFiles  []string
		baseName       string
		expectedResult string // empty string means no cover exists
	}{
		{
			name:           "no cover exists",
			existingFiles:  []string{},
			baseName:       "cover",
			expectedResult: "",
		},
		{
			name:           "exact png cover exists",
			existingFiles:  []string{"cover.png"},
			baseName:       "cover",
			expectedResult: "cover.png",
		},
		{
			name:           "exact jpg cover exists",
			existingFiles:  []string{"cover.jpg"},
			baseName:       "cover",
			expectedResult: "cover.jpg",
		},
		{
			name:           "jpeg extension cover exists",
			existingFiles:  []string{"cover.jpeg"},
			baseName:       "cover",
			expectedResult: "cover.jpeg",
		},
		{
			name:           "webp cover exists",
			existingFiles:  []string{"cover.webp"},
			baseName:       "cover",
			expectedResult: "cover.webp",
		},
		{
			name:           "individual cover with different extension",
			existingFiles:  []string{"book.epub.cover.png"},
			baseName:       "book.epub.cover",
			expectedResult: "book.epub.cover.png",
		},
		{
			name:           "custom cover exists",
			existingFiles:  []string{"custom_cover.jpg"},
			baseName:       "custom_cover",
			expectedResult: "custom_cover.jpg",
		},
		{
			name:           "returns first match (jpg before png in extension list)",
			existingFiles:  []string{"cover.png", "cover.jpg"},
			baseName:       "cover",
			expectedResult: "cover.jpg", // jpg comes before png in extension list
		},
		{
			name:           "different base name - no match",
			existingFiles:  []string{"other_cover.png"},
			baseName:       "cover",
			expectedResult: "",
		},
		{
			name:           "only non-cover files exist",
			existingFiles:  []string{"book.epub", "notes.txt"},
			baseName:       "book.epub.cover",
			expectedResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a subdirectory for this test case
			testDir := filepath.Join(tempDir, tt.name)
			err := os.MkdirAll(testDir, 0755)
			if err != nil {
				t.Fatalf("failed to create test dir: %v", err)
			}

			// Create the test files
			for _, filename := range tt.existingFiles {
				filePath := filepath.Join(testDir, filename)
				err := os.WriteFile(filePath, []byte("test content"), 0600)
				if err != nil {
					t.Fatalf("failed to create test file %s: %v", filename, err)
				}
			}

			// Run the function
			result := CoverExistsWithBaseName(testDir, tt.baseName)

			// Check the result
			if tt.expectedResult == "" {
				assert.Empty(t, result, "expected no cover to be found")
			} else {
				expectedPath := filepath.Join(testDir, tt.expectedResult)
				assert.Equal(t, expectedPath, result, "unexpected cover path")
			}
		})
	}
}

func TestComputeNewCoverPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		oldCoverPath string
		newFilePath  string
		want         string
	}{
		{
			name:         "computes new cover path with jpg extension",
			oldCoverPath: "/path/to/Old Title.epub.cover.jpg",
			newFilePath:  "/path/to/New Title.epub",
			want:         "/path/to/New Title.epub.cover.jpg",
		},
		{
			name:         "computes new cover path with png extension",
			oldCoverPath: "/path/to/Old Book.cbz.cover.png",
			newFilePath:  "/path/to/New Book.cbz",
			want:         "/path/to/New Book.cbz.cover.png",
		},
		{
			name:         "preserves jpeg extension",
			oldCoverPath: "/lib/book.m4b.cover.jpeg",
			newFilePath:  "/lib/audiobook.m4b",
			want:         "/lib/audiobook.m4b.cover.jpeg",
		},
		{
			name:         "handles webp extension",
			oldCoverPath: "/lib/old.epub.cover.webp",
			newFilePath:  "/lib/new.epub",
			want:         "/lib/new.epub.cover.webp",
		},
		{
			name:         "returns empty string for empty cover path",
			oldCoverPath: "",
			newFilePath:  "/path/to/file.epub",
			want:         "",
		},
		{
			name:         "handles path with spaces",
			oldCoverPath: "/path/to/My Book.epub.cover.jpg",
			newFilePath:  "/path/to/My New Book.epub",
			want:         "/path/to/My New Book.epub.cover.jpg",
		},
		{
			name:         "handles path with brackets",
			oldCoverPath: "/lib/[Author] Old Title.epub.cover.png",
			newFilePath:  "/lib/[Author] New Title.epub",
			want:         "/lib/[Author] New Title.epub.cover.png",
		},
		{
			name:         "handles audiobook with narrator braces",
			oldCoverPath: "/lib/Book {Old Narrator}.m4b.cover.jpg",
			newFilePath:  "/lib/Book {New Narrator}.m4b",
			want:         "/lib/Book {New Narrator}.m4b.cover.jpg",
		},
		{
			// Real-world usage: CoverImagePath stores just the filename, not the full path.
			// The function returns a full path, so callers must use filepath.Base() if they
			// need just the filename (e.g., for database storage).
			name:         "filename-only input returns full path (caller must use Base)",
			oldCoverPath: "OldBook.cbz.cover.jpg",
			newFilePath:  "/path/to/NewBook.cbz",
			want:         "/path/to/NewBook.cbz.cover.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeNewCoverPath(tt.oldCoverPath, tt.newFilePath)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRenameOrganizedFile_RenamesAssociatedFiles(t *testing.T) {
	t.Parallel()
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "rename-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	tests := []struct {
		name            string
		originalFile    string
		associatedFiles []string // files to create alongside the main file
		opts            OrganizedNameOptions
		wantNewFile     string
		wantRenamed     []string // associated files that should be renamed
		wantGone        []string // original associated files that should no longer exist
	}{
		{
			name:         "renames individual cover with file",
			originalFile: "Old Title.epub",
			associatedFiles: []string{
				"Old Title.epub.cover.jpg",
			},
			opts: OrganizedNameOptions{
				Title:    "New Title",
				FileType: "epub",
			},
			wantNewFile: "New Title.epub",
			wantRenamed: []string{
				"New Title.epub.cover.jpg",
			},
			wantGone: []string{
				"Old Title.epub.cover.jpg",
			},
		},
		{
			name:         "renames file sidecar with file",
			originalFile: "Old Title.epub",
			associatedFiles: []string{
				"Old Title.epub.metadata.json",
			},
			opts: OrganizedNameOptions{
				Title:    "New Title",
				FileType: "epub",
			},
			wantNewFile: "New Title.epub",
			wantRenamed: []string{
				"New Title.epub.metadata.json",
			},
			wantGone: []string{
				"Old Title.epub.metadata.json",
			},
		},
		{
			name:         "renames book sidecar when basename changes",
			originalFile: "Old Title.epub",
			associatedFiles: []string{
				"Old Title.metadata.json",
			},
			opts: OrganizedNameOptions{
				Title:    "New Title",
				FileType: "epub",
			},
			wantNewFile: "New Title.epub",
			wantRenamed: []string{
				"New Title.metadata.json",
			},
			wantGone: []string{
				"Old Title.metadata.json",
			},
		},
		{
			name:         "renames all associated files together",
			originalFile: "My Book.epub",
			associatedFiles: []string{
				"My Book.epub.cover.jpg",
				"My Book.epub.cover.png", // multiple cover formats
				"My Book.epub.metadata.json",
				"My Book.metadata.json",
			},
			opts: OrganizedNameOptions{
				Title:    "Renamed Book",
				FileType: "epub",
			},
			wantNewFile: "Renamed Book.epub",
			wantRenamed: []string{
				"Renamed Book.epub.cover.jpg",
				"Renamed Book.epub.cover.png",
				"Renamed Book.epub.metadata.json",
				"Renamed Book.metadata.json",
			},
			wantGone: []string{
				"My Book.epub.cover.jpg",
				"My Book.epub.cover.png",
				"My Book.epub.metadata.json",
				"My Book.metadata.json",
			},
		},
		{
			name:         "handles audiobook with narrator in filename",
			originalFile: "Book {Old Narrator}.m4b",
			associatedFiles: []string{
				"Book {Old Narrator}.m4b.cover.jpg",
				"Book {Old Narrator}.m4b.metadata.json",
			},
			opts: OrganizedNameOptions{
				Title:         "Book",
				NarratorNames: []string{"New Narrator"},
				FileType:      "m4b",
			},
			wantNewFile: "Book {New Narrator}.m4b",
			wantRenamed: []string{
				"Book {New Narrator}.m4b.cover.jpg",
				"Book {New Narrator}.m4b.metadata.json",
			},
			wantGone: []string{
				"Book {Old Narrator}.m4b.cover.jpg",
				"Book {Old Narrator}.m4b.metadata.json",
			},
		},
		{
			name:         "no rename needed when name unchanged",
			originalFile: "Same Title.epub",
			associatedFiles: []string{
				"Same Title.epub.cover.jpg",
			},
			opts: OrganizedNameOptions{
				Title:    "Same Title",
				FileType: "epub",
			},
			wantNewFile: "Same Title.epub",
			wantRenamed: []string{
				"Same Title.epub.cover.jpg", // should still exist
			},
			wantGone: []string{}, // nothing should be gone
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a subdirectory for this test case
			testDir := filepath.Join(tempDir, tt.name)
			err := os.MkdirAll(testDir, 0755)
			if err != nil {
				t.Fatalf("failed to create test dir: %v", err)
			}

			// Create the main file
			originalPath := filepath.Join(testDir, tt.originalFile)
			err = os.WriteFile(originalPath, []byte("main file content"), 0600)
			if err != nil {
				t.Fatalf("failed to create main file: %v", err)
			}

			// Create associated files
			for _, filename := range tt.associatedFiles {
				filePath := filepath.Join(testDir, filename)
				err = os.WriteFile(filePath, []byte("associated content"), 0600)
				if err != nil {
					t.Fatalf("failed to create associated file %s: %v", filename, err)
				}
			}

			// Run the function
			newPath, err := RenameOrganizedFile(originalPath, tt.opts)
			if err != nil {
				t.Fatalf("RenameOrganizedFile failed: %v", err)
			}

			// Check main file was renamed
			expectedNewPath := filepath.Join(testDir, tt.wantNewFile)
			assert.Equal(t, expectedNewPath, newPath, "main file path mismatch")
			assert.FileExists(t, newPath, "new main file should exist")

			// Check associated files were renamed
			for _, filename := range tt.wantRenamed {
				filePath := filepath.Join(testDir, filename)
				assert.FileExists(t, filePath, "renamed file should exist: %s", filename)
			}

			// Check old associated files are gone (unless name didn't change)
			for _, filename := range tt.wantGone {
				filePath := filepath.Join(testDir, filename)
				_, err := os.Stat(filePath)
				assert.True(t, os.IsNotExist(err), "old file should not exist: %s", filename)
			}
		})
	}
}

// TestRenameOrganizedFile_SupplementDoesNotRenameBookSidecar tests that when a supplement
// file is renamed, it should NOT rename the book sidecar file, even if they share the same
// basename. This is a regression test for a bug where renaming a supplement file with the
// same basename as the main file would incorrectly rename the book's metadata sidecar.
func TestRenameOrganizedFile_SupplementDoesNotRenameBookSidecar(t *testing.T) {
	t.Parallel()
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "supplement-rename-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	// Scenario: A book directory with:
	// - Main file: [Author] Book Title.epub
	// - Book sidecar: [Author] Book Title.metadata.json (named after main file's basename)
	// - Supplement file: [Author] Book Title.pdf (same basename as main file!)
	//
	// When the supplement file is renamed to "[Author] Supplement Name.pdf",
	// the book sidecar should NOT be renamed.

	// Create the main file (just to establish context, we won't rename this)
	mainFile := filepath.Join(tempDir, "[Author] Book Title.epub")
	err = os.WriteFile(mainFile, []byte("main epub content"), 0600)
	if err != nil {
		t.Fatalf("failed to create main file: %v", err)
	}

	// Create the book sidecar (named after main file's basename)
	bookSidecar := filepath.Join(tempDir, "[Author] Book Title.metadata.json")
	err = os.WriteFile(bookSidecar, []byte(`{"version":1,"title":"Book Title"}`), 0600)
	if err != nil {
		t.Fatalf("failed to create book sidecar: %v", err)
	}

	// Create the supplement file (same basename as main file, different extension)
	supplementFile := filepath.Join(tempDir, "[Author] Book Title.pdf")
	err = os.WriteFile(supplementFile, []byte("pdf content"), 0600)
	if err != nil {
		t.Fatalf("failed to create supplement file: %v", err)
	}

	// Rename the supplement file using RenameOrganizedFileOnly
	// (which should NOT rename the book sidecar)
	newPath, err := RenameOrganizedFileOnly(supplementFile, OrganizedNameOptions{
		Title:    "Supplement Name",
		FileType: "pdf",
	})
	if err != nil {
		t.Fatalf("RenameOrganizedFileOnly failed: %v", err)
	}

	// Verify the supplement was renamed
	expectedNewSupplementPath := filepath.Join(tempDir, "Supplement Name.pdf")
	assert.Equal(t, expectedNewSupplementPath, newPath, "supplement file path mismatch")
	assert.FileExists(t, newPath, "renamed supplement file should exist")

	// Verify the old supplement file is gone
	_, err = os.Stat(supplementFile)
	assert.True(t, os.IsNotExist(err), "old supplement file should not exist")

	// THIS IS THE KEY ASSERTION:
	// The book sidecar should NOT have been renamed!
	assert.FileExists(t, bookSidecar, "book sidecar should NOT be renamed when supplement is renamed")

	// The wrongly-renamed sidecar path should NOT exist
	wrongSidecar := filepath.Join(tempDir, "Supplement Name.metadata.json")
	_, err = os.Stat(wrongSidecar)
	assert.True(t, os.IsNotExist(err), "book sidecar should NOT have been renamed to supplement's basename")
}

func TestCleanupEmptyDirectory(t *testing.T) {
	t.Parallel()
	tempDir, err := os.MkdirTemp("", "cleanup-empty-dir-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	tests := []struct {
		name          string
		setup         func(dir string) string // returns the dir to test
		expectRemoved bool
		expectError   bool
		expectDirGone bool
	}{
		{
			name: "removes empty directory",
			setup: func(dir string) string {
				emptyDir := filepath.Join(dir, "empty")
				os.MkdirAll(emptyDir, 0755)
				return emptyDir
			},
			expectRemoved: true,
			expectDirGone: true,
		},
		{
			name: "does not remove non-empty directory",
			setup: func(dir string) string {
				nonEmptyDir := filepath.Join(dir, "nonempty")
				os.MkdirAll(nonEmptyDir, 0755)
				os.WriteFile(filepath.Join(nonEmptyDir, "file.txt"), []byte("content"), 0600)
				return nonEmptyDir
			},
			expectRemoved: false,
			expectDirGone: false,
		},
		{
			name: "returns false for non-existent directory",
			setup: func(dir string) string {
				return filepath.Join(dir, "nonexistent")
			},
			expectRemoved: false,
			expectError:   false,
		},
		{
			name: "does not remove directory with subdirectory",
			setup: func(dir string) string {
				parentDir := filepath.Join(dir, "parent")
				os.MkdirAll(filepath.Join(parentDir, "child"), 0755)
				return parentDir
			},
			expectRemoved: false,
			expectDirGone: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := filepath.Join(tempDir, tt.name)
			os.MkdirAll(testDir, 0755)

			targetDir := tt.setup(testDir)
			removed, err := CleanupEmptyDirectory(targetDir)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			assert.Equal(t, tt.expectRemoved, removed, "removed mismatch")

			if tt.expectDirGone {
				_, statErr := os.Stat(targetDir)
				assert.True(t, os.IsNotExist(statErr), "directory should be gone")
			} else if removed {
				_, statErr := os.Stat(targetDir)
				assert.True(t, os.IsNotExist(statErr), "directory should be gone after removal")
			}
		})
	}
}

func TestCleanupEmptyParentDirectories(t *testing.T) {
	t.Parallel()
	tempDir, err := os.MkdirTemp("", "cleanup-parents-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	tests := []struct {
		name            string
		setup           func(dir string) (startPath, stopAt string)
		expectDirsGone  []string // relative to tempDir
		expectDirsExist []string // relative to tempDir
	}{
		{
			name: "removes chain of empty directories",
			setup: func(dir string) (string, string) {
				// Create: dir/a/b/c (all empty)
				os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0755)
				return filepath.Join(dir, "a", "b", "c"), dir
			},
			expectDirsGone:  []string{"a/b/c", "a/b", "a"},
			expectDirsExist: []string{},
		},
		{
			name: "stops at non-empty directory",
			setup: func(dir string) (string, string) {
				// Create: dir/a/b/c with file in a
				os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0755)
				os.WriteFile(filepath.Join(dir, "a", "file.txt"), []byte("content"), 0600)
				return filepath.Join(dir, "a", "b", "c"), dir
			},
			expectDirsGone:  []string{"a/b/c", "a/b"},
			expectDirsExist: []string{"a"},
		},
		{
			name: "stops at stopAt directory",
			setup: func(dir string) (string, string) {
				// Create: dir/a/b/c, stopAt = dir/a
				os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0755)
				return filepath.Join(dir, "a", "b", "c"), filepath.Join(dir, "a")
			},
			expectDirsGone:  []string{"a/b/c", "a/b"},
			expectDirsExist: []string{"a"},
		},
		{
			name: "handles non-existent start path",
			setup: func(dir string) (string, string) {
				return filepath.Join(dir, "nonexistent"), dir
			},
			expectDirsGone:  []string{},
			expectDirsExist: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := filepath.Join(tempDir, tt.name)
			os.MkdirAll(testDir, 0755)

			startPath, stopAt := tt.setup(testDir)
			err := CleanupEmptyParentDirectories(startPath, stopAt)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			for _, relPath := range tt.expectDirsGone {
				fullPath := filepath.Join(testDir, relPath)
				_, statErr := os.Stat(fullPath)
				assert.True(t, os.IsNotExist(statErr), "directory should be gone: %s", relPath)
			}

			for _, relPath := range tt.expectDirsExist {
				fullPath := filepath.Join(testDir, relPath)
				_, statErr := os.Stat(fullPath)
				assert.NoError(t, statErr, "directory should exist: %s", relPath)
			}
		})
	}
}
