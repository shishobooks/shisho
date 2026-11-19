package fileutils

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// organizeExamplePath is the example Setup's organize animation shows. The
// frontend reads its organized names, so they come from the naming functions
// here rather than being typed in by hand.
const organizeExamplePath = "../../app/components/setup/organizeExample.json"

// updateOrganizeExampleEnv rewrites the example's organized names from the
// naming functions instead of checking them.
const updateOrganizeExampleEnv = "UPDATE_ORGANIZE_EXAMPLE"

type organizeExampleFile struct {
	// Type is the icon the animation shows: ebook, audiobook, or comic.
	Type          string `json:"type"`
	MessyName     string `json:"messy_name"`
	OrganizedName string `json:"organized_name"`
}

type organizeExampleBook struct {
	Author       string   `json:"author"`
	Title        string   `json:"title"`
	SeriesNumber *float64 `json:"series_number,omitempty"`
	// MessyFolder holds the book's files on the messy side; files in one
	// folder are one book, so it names a single book. Nil leaves the files
	// loose in the root, where each is a book of its own.
	MessyFolder     *string               `json:"messy_folder"`
	OrganizedFolder string                `json:"organized_folder"`
	Files           []organizeExampleFile `json:"files"`
}

type organizeExample struct {
	Root  string                `json:"root"`
	Books []organizeExampleBook `json:"books"`
}

// exampleFileType is the icon a file type shows in the animation.
func exampleFileType(fileType string) string {
	switch {
	case fileType == models.FileTypeCBZ:
		return "comic"
	case models.IsAudiobookFileType(fileType):
		return "audiobook"
	default:
		return "ebook"
	}
}

// organizedNames runs a book through the naming functions a scan uses: the
// folder takes CBZ naming when the book has a CBZ, as books.Service names
// it, and each file is named for itself.
func organizedNames(t *testing.T, book organizeExampleBook) (string, []string) {
	t.Helper()
	require.NotEmpty(t, book.Files, "%s has no files", book.Title)
	opts := func(fileType string) OrganizedNameOptions {
		return OrganizedNameOptions{
			AuthorNames:  []string{book.Author},
			Title:        book.Title,
			SeriesNumber: book.SeriesNumber,
			FileType:     fileType,
		}
	}
	folderType := models.FileTypeForPath(book.Files[0].MessyName)
	for _, file := range book.Files {
		fileType := models.FileTypeForPath(file.MessyName)
		require.True(t, models.IsBuiltInFileType(fileType), "%s isn't a file Shisho scans", file.MessyName)
		require.Equal(t, exampleFileType(fileType), file.Type, "%s's icon type", file.MessyName)
		if fileType == models.FileTypeCBZ {
			folderType = models.FileTypeCBZ
		}
	}
	folder := GenerateOrganizedFolderName(opts(folderType))
	files := make([]string, len(book.Files))
	for i, file := range book.Files {
		files[i] = GenerateOrganizedFileName(opts(models.FileTypeForPath(file.MessyName)), file.MessyName)
	}
	return folder, files
}

// Setup's organize animation shows example books moving into Shisho's
// layout. Its organized names must be what the naming functions give, so a
// change to the naming fails here until the example is rewritten with
// UPDATE_ORGANIZE_EXAMPLE=1.
func TestOrganizeExampleMatchesNaming(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(organizeExamplePath)
	require.NoError(t, err)
	var example organizeExample
	dec := json.NewDecoder(bytes.NewReader(raw))
	// A field the struct doesn't know would be dropped by an update.
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(&example))

	if os.Getenv(updateOrganizeExampleEnv) != "" {
		for i, book := range example.Books {
			folder, files := organizedNames(t, book)
			example.Books[i].OrganizedFolder = folder
			for j := range book.Files {
				example.Books[i].Files[j].OrganizedName = files[j]
			}
		}
		var out bytes.Buffer
		enc := json.NewEncoder(&out)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		require.NoError(t, enc.Encode(example))
		require.NoError(t, os.WriteFile(organizeExamplePath, out.Bytes(), 0o644)) //nolint:gosec // a checked-in source file
	}

	for _, book := range example.Books {
		folder, files := organizedNames(t, book)
		assert.Equal(t, folder, book.OrganizedFolder,
			"%s's folder; rerun with %s=1 to update the example", book.Title, updateOrganizeExampleEnv)
		for i, file := range book.Files {
			assert.Equal(t, files[i], file.OrganizedName,
				"%s's file %s; rerun with %s=1 to update the example", book.Title, file.MessyName, updateOrganizeExampleEnv)
		}
	}
}
