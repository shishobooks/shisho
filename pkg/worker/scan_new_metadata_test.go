package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const newMetadataParserManifest = `{
  "manifestVersion": 1,
  "id": "newmeta-parser",
  "name": "NewMeta Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Parses newmeta files",
      "types": ["newmeta"]
    }
  }
}`

const newMetadataParserMainJS = `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "Parsed Title", authors: [{name: "Parsed Author", role: ""}] };
      }
    }
  };
})();`

const newMetadataEnricherManifest = `{
  "manifestVersion": 1,
  "id": "newmeta-enricher",
  "name": "NewMeta Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Enriches newmeta files",
      "fileTypes": ["newmeta"],
      "fields": ["title", "description"]
    }
  }
}`

const newMetadataEnricherMainJS = `var plugin = (function() {
  return {
    metadataEnricher: {
      search: function(ctx) {
        return { results: [{ title: "Enriched Title", description: "Enriched description" }] };
      }
    }
  };
})();`

// TestScanForNewMetadata_EnrichesUnchangedFilesAndKeepsManualValues covers
// the library-wide Scan for new metadata: a normal scan skips a File whose
// size and modified time haven't changed, so an enricher installed after the
// import never reaches it, while the new metadata scan runs the enricher on
// it and still keeps the manual title.
func TestScanForNewMetadata_EnrichesUnchangedFilesAndKeepsManualValues(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	ctx := tc.ctx

	installTestPlugin(t, tc, pluginDir, "newmeta-parser", newMetadataParserManifest, newMetadataParserMainJS)
	require.NoError(t, tc.worker.pluginManager.LoadAll(ctx))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := filepath.Join(libraryPath, "A Book")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.newmeta"), []byte("content"), 0644))

	require.NoError(t, tc.runScan())
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	book := allBooks[0]
	require.Nil(t, book.Description)

	// The user edits the title by hand.
	book.Title = "Manual Title"
	book.TitleSource = models.DataSourceManual
	require.NoError(t, tc.bookService.UpdateBook(tc.ctx, book, books.UpdateBookOptions{
		Columns: []string{"title", "title_source"},
	}))

	// An enricher is installed after the import.
	installTestPlugin(t, tc, pluginDir, "newmeta-enricher", newMetadataEnricherManifest, newMetadataEnricherMainJS)
	require.NoError(t, plugins.NewService(tc.db).AppendToOrder(ctx, models.PluginHookMetadataEnricher, "test", "newmeta-enricher"))
	require.NoError(t, tc.worker.pluginManager.LoadAll(ctx))

	libraryID := book.LibraryID

	// A normal scan skips the unchanged File.
	require.NoError(t, tc.runScanJob(&models.Job{
		Type:       models.JobTypeScan,
		LibraryID:  &libraryID,
		DataParsed: &models.JobScanData{},
	}))
	allBooks = tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Nil(t, allBooks[0].Description, "a normal scan must skip a File that hasn't changed")

	// The library-wide Scan for new metadata runs the enricher on it.
	require.NoError(t, tc.runScanJob(&models.Job{
		Type:       models.JobTypeScan,
		LibraryID:  &libraryID,
		DataParsed: &models.JobScanData{ScanForNewMetadata: true},
	}))
	allBooks = tc.listBooks()
	require.Len(t, allBooks, 1)
	enriched := allBooks[0]
	require.NotNil(t, enriched.Description)
	assert.Equal(t, "Enriched description", *enriched.Description)
	require.NotNil(t, enriched.DescriptionSource)
	assert.Equal(t, "plugin:test/newmeta-enricher", *enriched.DescriptionSource)
	assert.Equal(t, "Manual Title", enriched.Title, "the manual title must win over the enricher")
	assert.Equal(t, models.DataSourceManual, enriched.TitleSource)
}

// TestScanForNewMetadata_LeavesOrganizingToThePostScanPass checks that a
// File the new metadata scan rescans is not moved while the scan is still
// working through the library: a sibling File queued at the old path would
// then look changed. The scan organizes the Book once every File is done.
func TestScanForNewMetadata_LeavesOrganizingToThePostScanPass(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)

	// The parser gives no title, so the Book's title comes from its folder
	// and an enricher's title outranks it.
	installTestPlugin(t, tc, pluginDir, "newmeta-parser", newMetadataParserManifest, `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { authors: [{name: "Parsed Author", role: ""}] };
      }
    }
  };
})();`)
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))

	libraryPath := t.TempDir()
	tc.createLibraryWithOptions([]string{libraryPath}, true)
	bookDir := filepath.Join(libraryPath, "Folder Title")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.newmeta"), []byte("content"), 0644))
	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 1)
	organizedPath := files[0].Filepath
	require.NotContains(t, organizedPath, "Enriched Title")

	installTestPlugin(t, tc, pluginDir, "newmeta-enricher", newMetadataEnricherManifest, newMetadataEnricherMainJS)
	require.NoError(t, plugins.NewService(tc.db).AppendToOrder(tc.ctx, models.PluginHookMetadataEnricher, "test", "newmeta-enricher"))
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))

	// Without sidecars, which outrank plugins, the enricher's title wins.
	require.NoError(t, filepath.WalkDir(libraryPath, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(path) == ".json" {
			return os.Remove(path)
		}
		return err
	}))

	allFiles, err := tc.bookService.ListAllFilesForLibrary(tc.ctx, files[0].LibraryID)
	require.NoError(t, err)
	cache := NewScanCache()
	cache.LoadKnownFiles(allFiles)
	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:      organizedPath,
		LibraryID:     files[0].LibraryID,
		ScanUnchanged: true,
	}, cache)
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "Enriched Title", allBooks[0].Title)
	assert.FileExists(t, organizedPath, "the File must stay put until the scan's organize pass")
	assert.Equal(t, organizedPath, tc.listFiles()[0].Filepath)

	// The full scan organizes the Book under its new title afterwards.
	libraryID := files[0].LibraryID
	require.NoError(t, tc.runScanJob(&models.Job{
		Type:       models.JobTypeScan,
		LibraryID:  &libraryID,
		DataParsed: &models.JobScanData{ScanForNewMetadata: true},
	}))
	assert.Contains(t, tc.listFiles()[0].Filepath, "Enriched Title")
}
