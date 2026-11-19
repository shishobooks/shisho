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
	Type          string `json:"type"`
	MessyName     string `json:"messyName"`
	OrganizedName string `json:"organizedName"`
}

type organizeExampleBook struct {
	Author          string                `json:"author"`
	Title           string                `json:"title"`
	SeriesNumber    *float64              `json:"seriesNumber,omitempty"`
	MessyFolder     *string               `json:"messyFolder"`
	OrganizedFolder string                `json:"organizedFolder"`
	Files           []organizeExampleFile `json:"files"`
}

type organizeExample struct {
	Root  string                `json:"root"`
	Books []organizeExampleBook `json:"books"`
}

// organizedNames runs a book through the naming functions a scan uses: the
// folder is named for the book's first file, and each file for itself.
func organizedNames(t *testing.T, book organizeExampleBook) (string, []string) {
	t.Helper()
	require.NotEmpty(t, book.Files, "%s has no files", book.Title)
	opts := func(path string) OrganizedNameOptions {
		return OrganizedNameOptions{
			AuthorNames:  []string{book.Author},
			Title:        book.Title,
			SeriesNumber: book.SeriesNumber,
			FileType:     models.FileTypeForPath(path),
		}
	}
	folder := GenerateOrganizedFolderName(opts(book.Files[0].MessyName))
	files := make([]string, len(book.Files))
	for i, file := range book.Files {
		files[i] = GenerateOrganizedFileName(opts(file.MessyName), file.MessyName)
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
	require.NoError(t, json.Unmarshal(raw, &example))

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
		return
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
