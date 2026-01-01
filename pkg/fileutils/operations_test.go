package fileutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoverExistsWithBaseName(t *testing.T) {
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
			name:           "audiobook cover exists",
			existingFiles:  []string{"audiobook_cover.jpg"},
			baseName:       "audiobook_cover",
			expectedResult: "audiobook_cover.jpg",
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
