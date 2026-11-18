package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover how a scan settles a field between the value it found,
// the value stored, and the sidecar Shisho writes after every scan and edit:
//
//   - A sidecar value equal to what was stored before the scan is only
//     Shisho's own copy of the database, so it does not outrank a plugin.
//   - A sidecar value that differs (carried over from another library, or
//     edited on disk) keeps its priority.
//   - A plugin value never replaces another plugin value in an ordinary scan
//     of an unchanged file, so a fix made through Identify stays put.

// epubEnricher returns a metadata enricher for EPUBs that sets description
// and authors.
func epubEnricher(id, description, author string) (string, string) {
	manifest := `{
  "manifestVersion": 1,
  "id": "` + id + `",
  "name": "` + id + `",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Enriches EPUBs",
      "fileTypes": ["epub"],
      "fields": ["description", "authors", "publisher"]
    }
  }
}`
	mainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      search: function(ctx) {
        return { results: [{ title: ctx.query, description: "` + description + `", authors: [{name: "` + author + `", role: ""}], publisher: "` + id + ` Publisher" }] };
      }
    }
  };
})();`
	return manifest, mainJS
}

// sidecarRulesFixture is a library with one EPUB book and a plugin manager.
type sidecarRulesFixture struct {
	tc        *testContext
	pluginDir string
	libraryID int
	bookDir   string
	epubPath  string
}

func newSidecarRulesFixture(t *testing.T) *sidecarRulesFixture {
	t.Helper()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "A Book")
	return &sidecarRulesFixture{
		tc:        tc,
		pluginDir: pluginDir,
		libraryID: tc.listLibraryIDs()[0],
		bookDir:   bookDir,
		epubPath:  filepath.Join(bookDir, "book.epub"),
	}
}

func (f *sidecarRulesFixture) writeEPUB(t *testing.T, description string) {
	t.Helper()
	testgen.GenerateEPUB(t, f.bookDir, "book.epub", testgen.EPUBOptions{
		Title:       "A Book",
		Authors:     []string{"Embedded Author"},
		Description: description,
		Publisher:   "Embedded Publisher",
	})
}

// installEnricher installs an EPUB enricher, ordered before any installed so
// far, and loads it.
func (f *sidecarRulesFixture) installEnricher(t *testing.T, id, description, author string) {
	t.Helper()
	manifest, mainJS := epubEnricher(id, description, author)
	installTestPlugin(t, f.tc, f.pluginDir, id, manifest, mainJS)
	svc := plugins.NewService(f.tc.db)
	order, err := svc.GetOrder(f.tc.ctx, models.PluginHookMetadataEnricher)
	require.NoError(t, err)
	entries := []models.PluginHookConfig{{Scope: "test", PluginID: id}}
	for _, o := range order {
		entries = append(entries, models.PluginHookConfig{Scope: o.Scope, PluginID: o.PluginID})
	}
	require.NoError(t, svc.SetOrder(f.tc.ctx, models.PluginHookMetadataEnricher, entries))
	require.NoError(t, f.tc.worker.pluginManager.LoadAll(f.tc.ctx))
}

func (f *sidecarRulesFixture) scanForNewMetadata(t *testing.T) {
	t.Helper()
	require.NoError(t, f.tc.runScanJob(&models.Job{
		Type:       models.JobTypeScan,
		LibraryID:  &f.libraryID,
		DataParsed: &models.JobScanData{ScanForNewMetadata: true},
	}))
}

func (f *sidecarRulesFixture) book(t *testing.T) *models.Book {
	t.Helper()
	all := f.tc.listBooks()
	require.Len(t, all, 1)
	return all[0]
}

func bookAuthorNames(book *models.Book) []string {
	names := make([]string, 0, len(book.Authors))
	for _, a := range book.Authors {
		names = append(names, a.Person.Name)
	}
	return names
}

func sourceOf(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// An enricher installed after the import replaces values that came from the
// file, even though the sidecar Shisho wrote at import holds them.
func TestScanRules_LateEnricherReplacesFileValues(t *testing.T) {
	t.Parallel()
	f := newSidecarRulesFixture(t)
	f.writeEPUB(t, "Embedded description")
	require.NoError(t, f.tc.runScan())
	require.Equal(t, "Embedded description", *f.book(t).Description)

	f.installEnricher(t, "late", "Enriched description", "Enriched Author")
	f.scanForNewMetadata(t)

	book := f.book(t)
	require.NotNil(t, book.Description)
	assert.Equal(t, "Enriched description", *book.Description)
	assert.Equal(t, "plugin:test/late", sourceOf(book.DescriptionSource))
	assert.Equal(t, []string{"Enriched Author"}, bookAuthorNames(book))
	assert.Equal(t, "plugin:test/late", book.AuthorSource)
	// A File-level field follows the same rule.
	file := f.tc.listFiles()[0]
	file, err := f.tc.bookService.RetrieveFileWithRelations(f.tc.ctx, file.ID)
	require.NoError(t, err)
	require.NotNil(t, file.Publisher)
	assert.Equal(t, "late Publisher", file.Publisher.Name)
	assert.Equal(t, "plugin:test/late", sourceOf(file.PublisherSource))
}

// Metadata carried over from another library in a sidecar keeps outranking
// plugins.
func TestScanRules_CarriedOverSidecarStaysProtected(t *testing.T) {
	t.Parallel()
	f := newSidecarRulesFixture(t)
	f.writeEPUB(t, "Embedded description")
	carried := "Carried description"
	require.NoError(t, sidecar.WriteBookSidecar(f.bookDir, &sidecar.BookSidecar{Description: &carried}))
	require.NoError(t, f.tc.runScan())
	book := f.book(t)
	require.Equal(t, carried, *book.Description)
	require.Equal(t, models.DataSourceSidecar, sourceOf(book.DescriptionSource))

	f.installEnricher(t, "late", "Enriched description", "Enriched Author")
	f.scanForNewMetadata(t)

	book = f.book(t)
	assert.Equal(t, carried, *book.Description)
	assert.Equal(t, models.DataSourceSidecar, sourceOf(book.DescriptionSource))
}

// A sidecar edited by hand on disk outranks plugins.
func TestScanRules_HandEditedSidecarWins(t *testing.T) {
	t.Parallel()
	f := newSidecarRulesFixture(t)
	f.writeEPUB(t, "Embedded description")
	require.NoError(t, f.tc.runScan())

	path := sidecar.BookSidecarPath(f.bookDir)
	s, err := sidecar.ReadBookSidecar(f.bookDir)
	require.NoError(t, err)
	require.NotNil(t, s, "the import writes a sidecar at %s", path)
	edited := "Edited on disk"
	s.Description = &edited
	require.NoError(t, sidecar.WriteBookSidecar(f.bookDir, s))

	f.installEnricher(t, "late", "Enriched description", "Enriched Author")
	f.scanForNewMetadata(t)

	book := f.book(t)
	assert.Equal(t, edited, *book.Description)
	assert.Equal(t, models.DataSourceSidecar, sourceOf(book.DescriptionSource))
	// Fields the hand edit left alone still take the enricher's values.
	assert.Equal(t, []string{"Enriched Author"}, bookAuthorNames(book))
}

// A plugin value stays when another plugin, or the same plugin matching a
// different book, offers a different one. This is what keeps a fix made
// through Identify from being undone by the next scan.
func TestScanRules_PluginValueIsNotReplacedByAnotherPluginValue(t *testing.T) {
	t.Parallel()
	f := newSidecarRulesFixture(t)
	f.writeEPUB(t, "Embedded description")
	f.installEnricher(t, "first", "First description", "First Author")
	require.NoError(t, f.tc.runScan())
	book := f.book(t)
	require.Equal(t, "First description", *book.Description)

	// Identify picks the right book: its values carry the plugin's source.
	right := "Identified description"
	rightSource := "plugin:test/first"
	book.Description = &right
	book.DescriptionSource = &rightSource
	require.NoError(t, f.tc.bookService.UpdateBook(f.tc.ctx, book, books.UpdateBookOptions{
		Columns: []string{"description", "description_source"},
	}))
	book = f.book(t)
	require.NoError(t, sidecar.WriteBookSidecarFromModel(book))

	// A second enricher, first in order, offers something else.
	f.installEnricher(t, "second", "Second description", "Second Author")
	f.scanForNewMetadata(t)

	book = f.book(t)
	assert.Equal(t, right, *book.Description)
	assert.Equal(t, rightSource, sourceOf(book.DescriptionSource), "the value keeps its plugin source")
	assert.Equal(t, []string{"First Author"}, bookAuthorNames(book))
	assert.Equal(t, "plugin:test/first", book.AuthorSource)
}

// A file whose contents changed on disk keeps values a plugin set. Its new
// embedded values never outrank them, and neither does another plugin's.
func TestScanRules_ChangedFileKeepsPluginValues(t *testing.T) {
	t.Parallel()
	f := newSidecarRulesFixture(t)
	f.writeEPUB(t, "Embedded description")
	f.installEnricher(t, "first", "First description", "First Author")
	require.NoError(t, f.tc.runScan())
	require.Equal(t, "First description", *f.book(t).Description)

	f.installEnricher(t, "second", "Second description", "Second Author")
	require.NoError(t, os.Remove(f.epubPath))
	f.writeEPUB(t, "A longer embedded description that changes the file's size")
	require.NoError(t, f.tc.runScan())

	book := f.book(t)
	assert.Equal(t, "First description", *book.Description)
	assert.Equal(t, "plugin:test/first", sourceOf(book.DescriptionSource))
}

// A new File joining a Book already in the library treats the Book's sidecar
// as Shisho's copy, and its enrichment neither swaps the Book's plugin
// values nor relabels them sidecar.
func TestScanRules_NewFileInExistingBook(t *testing.T) {
	t.Parallel()
	f := newSidecarRulesFixture(t)
	f.writeEPUB(t, "Embedded description")
	require.NoError(t, f.tc.runScan())

	f.installEnricher(t, "late", "Enriched description", "Enriched Author")
	testgen.GenerateEPUB(t, f.bookDir, "second.epub", testgen.EPUBOptions{
		Title:       "A Book",
		Authors:     []string{"Embedded Author"},
		Description: "Second file description",
	})
	require.NoError(t, f.tc.runScan())
	require.Len(t, f.tc.listFiles(), 2)

	book := f.book(t)
	assert.Equal(t, "Enriched description", *book.Description)
	assert.Equal(t, "plugin:test/late", sourceOf(book.DescriptionSource))
}

// A plugin file parser's values are the file's own metadata, so a late
// enricher replaces them as it replaces embedded metadata.
func TestScanRules_LateEnricherReplacesPluginParserValues(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	installTestPlugin(t, tc, pluginDir, "newmeta-parser", newMetadataParserManifest, newMetadataParserMainJS)
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))
	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := filepath.Join(libraryPath, "A Book")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.newmeta"), []byte("content"), 0644))
	require.NoError(t, tc.runScan())
	books := tc.listBooks()
	require.Len(t, books, 1)
	require.Equal(t, "Parsed Title", books[0].Title)

	installTestPlugin(t, tc, pluginDir, "newmeta-enricher", newMetadataEnricherManifest, newMetadataEnricherMainJS)
	require.NoError(t, plugins.NewService(tc.db).AppendToOrder(tc.ctx, models.PluginHookMetadataEnricher, "test", "newmeta-enricher"))
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))
	libraryID := books[0].LibraryID
	require.NoError(t, tc.runScanJob(&models.Job{
		Type:       models.JobTypeScan,
		LibraryID:  &libraryID,
		DataParsed: &models.JobScanData{ScanForNewMetadata: true},
	}))

	books = tc.listBooks()
	require.Len(t, books, 1)
	assert.Equal(t, "Enriched Title", books[0].Title)
	assert.Equal(t, "plugin:test/newmeta-enricher", books[0].TitleSource)
}

// A manual value outranks everything in an ordinary scan.
func TestScanRules_ManualValueStays(t *testing.T) {
	t.Parallel()
	f := newSidecarRulesFixture(t)
	f.writeEPUB(t, "Embedded description")
	require.NoError(t, f.tc.runScan())
	book := f.book(t)
	mine := "My description"
	manual := models.DataSourceManual
	book.Description = &mine
	book.DescriptionSource = &manual
	require.NoError(t, f.tc.bookService.UpdateBook(f.tc.ctx, book, books.UpdateBookOptions{
		Columns: []string{"description", "description_source"},
	}))
	require.NoError(t, sidecar.WriteBookSidecarFromModel(f.book(t)))

	f.installEnricher(t, "late", "Enriched description", "Enriched Author")
	f.scanForNewMetadata(t)

	book = f.book(t)
	assert.Equal(t, mine, *book.Description)
	assert.Equal(t, models.DataSourceManual, sourceOf(book.DescriptionSource))
}
