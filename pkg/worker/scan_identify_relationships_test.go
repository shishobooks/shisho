package worker

import (
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const identifyRelationshipEnricherManifest = `{
  "manifestVersion": 1,
  "id": "relationship-enricher",
  "name": "Relationship Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "fileTypes": ["m4b"],
      "fields": ["authors", "genres", "tags", "narrators"]
    }
  }
}`

const identifyRelationshipEnricherJS = `var plugin = {
  metadataEnricher: {
    search: function(ctx) {
      return { results: [{
        authors: [{name: "Plugin Author One"}, {name: "Plugin Author Two"}],
        genres: ["Plugin Genre One", "Plugin Genre Two"],
        tags: ["Plugin Tag One", "Plugin Tag Two"],
        narrators: ["Plugin Narrator One", "Plugin Narrator Two"]
      }] };
    }
  }
};`

// Native M4B metadata has lower priority than plugin metadata, unlike a plugin
// parser. The embedded relationships are what a Scan would refill a cleared
// collection from.
func newIdentifyRelationshipScanFixture(t *testing.T, autoEnrich, rootLevel bool) *identifyScanFixture {
	t.Helper()
	testgen.SkipIfNoFFmpeg(t)

	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	js := `var plugin = {metadataEnricher: {search: function(ctx) {return {results: []};}}};`
	if autoEnrich {
		js = identifyRelationshipEnricherJS
	}
	installTestPlugin(t, tc, pluginDir, "relationship-enricher", identifyRelationshipEnricherManifest, js)
	require.NoError(t, tc.worker.pluginService.AppendToOrder(tc.ctx, models.PluginHookMetadataEnricher, "test", "relationship-enricher"))
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := libraryPath
	if !rootLevel {
		bookDir = testgen.CreateSubDir(t, libraryPath, "Embedded Title")
	}
	path := testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{
		Title:    "Embedded Title",
		Artist:   "Embedded Author One, Embedded Author Two",
		Composer: "Embedded Narrator One, Embedded Narrator Two",
		Genre:    "Embedded Genre One, Embedded Genre Two",
	})
	metadata, err := mp4.ParseFull(path)
	require.NoError(t, err)
	metadata.Tags = []string{"Embedded Tag One", "Embedded Tag Two"}
	require.NoError(t, mp4.Write(path, metadata, mp4.WriteOptions{}))

	require.NoError(t, tc.runScan())
	return &identifyScanFixture{tc: tc}
}

func identifyRelationshipFields(prefix string, keepBoth bool) map[string]any {
	authors := []map[string]string{{"name": prefix + " Author One"}}
	genres := []string{prefix + " Genre One"}
	tags := []string{prefix + " Tag One"}
	narrators := []string{prefix + " Narrator One"}
	if keepBoth {
		authors = append(authors, map[string]string{"name": prefix + " Author Two"})
		genres = append(genres, prefix+" Genre Two")
		tags = append(tags, prefix+" Tag Two")
		narrators = append(narrators, prefix+" Narrator Two")
	}
	return map[string]any{"authors": authors, "genres": genres, "tags": tags, "narrators": narrators}
}

func assertIdentifyRelationships(t *testing.T, book *models.Book, file *models.File, prefix string, keepBoth bool, source string) {
	t.Helper()
	var authors, genres, tags, narrators []string
	for _, author := range book.Authors {
		require.NotNil(t, author.Person)
		authors = append(authors, author.Person.Name)
	}
	for _, genre := range book.BookGenres {
		require.NotNil(t, genre.Genre)
		genres = append(genres, genre.Genre.Name)
	}
	for _, tag := range book.BookTags {
		require.NotNil(t, tag.Tag)
		tags = append(tags, tag.Tag.Name)
	}
	for _, narrator := range file.Narrators {
		require.NotNil(t, narrator.Person)
		narrators = append(narrators, narrator.Person.Name)
	}
	wantAuthors := []string{prefix + " Author One"}
	if keepBoth {
		wantAuthors = append(wantAuthors, prefix+" Author Two")
	}
	want := identifyRelationshipFields(prefix, keepBoth)
	assert.Equal(t, wantAuthors, authors, "authors")
	assert.ElementsMatch(t, want["genres"], genres, "genres")
	assert.ElementsMatch(t, want["tags"], tags, "tags")
	assert.Equal(t, want["narrators"], narrators, "narrators")
	assert.Equal(t, source, book.AuthorSource, "author source")
	assert.Equal(t, &source, book.GenreSource, "genre source")
	assert.Equal(t, &source, book.TagSource, "tag source")
	assert.Equal(t, &source, file.NarratorSource, "narrator source")
}

func TestIdentifyApply_RelationshipClear_ThenOrdinaryScan(t *testing.T) {
	t.Parallel()

	f := newIdentifyRelationshipScanFixture(t, false, false)
	book, file := f.retrieve(t)
	assertIdentifyRelationships(t, book, file, "Embedded", true, models.DataSourceM4BMetadata)
	require.False(t, t.Failed(), "precondition: all four relationships come from native M4B metadata")
	e := newIdentifyApplyServer(t, f.tc)

	// Establish plugin provenance before clearing, so the clear has to replace
	// it.
	postIdentifyApply(t, e, plugins.PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: identifyRelationshipFields("Plugin", true),
		Sources: map[string]string{
			"authors": plugins.SourceIntentPlugin, "genres": plugins.SourceIntentPlugin,
			"tags": plugins.SourceIntentPlugin, "narrators": plugins.SourceIntentPlugin,
		},
		PluginScope: "test",
		PluginID:    "relationship-enricher",
	})
	book, file = f.retrieve(t)
	assertIdentifyRelationships(t, book, file, "Plugin", true, models.PluginDataSource("test", "relationship-enricher"))
	require.False(t, t.Failed(), "precondition: Identify accepted all four plugin relationships")

	postIdentifyApply(t, e, plugins.PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			"authors": []any{}, "genres": []string{}, "tags": []string{}, "narrators": []string{},
		},
		Sources: map[string]string{
			"authors": plugins.SourceIntentUser, "genres": plugins.SourceIntentUser,
			"tags": plugins.SourceIntentUser, "narrators": plugins.SourceIntentUser,
		},
		PluginScope: "test",
		PluginID:    "relationship-enricher",
	})
	// A complete clear leaves protected empty collections.
	book, file = f.retrieve(t)
	assertClearedIdentifyRelationships(t, book, file)

	f.ordinaryScan(t, file.ID)
	book, file = f.retrieve(t)
	assertClearedIdentifyRelationships(t, book, file)
}

// assertClearedIdentifyRelationships checks that all four relationships are
// protected empty collections: no entries, with a manual source.
func assertClearedIdentifyRelationships(t *testing.T, book *models.Book, file *models.File) {
	t.Helper()
	manual := models.DataSourceManual
	assert.Empty(t, book.Authors, "authors")
	assert.Empty(t, book.BookGenres, "genres")
	assert.Empty(t, book.BookTags, "tags")
	assert.Empty(t, file.Narrators, "narrators")
	assert.Equal(t, manual, book.AuthorSource, "author source")
	assert.Equal(t, &manual, book.GenreSource, "genre source")
	assert.Equal(t, &manual, book.TagSource, "tag source")
	assert.Equal(t, &manual, file.NarratorSource, "narrator source")
}

func TestIdentifyApply_OrganizedRelationshipClear_ThenOrdinaryScan(t *testing.T) {
	t.Parallel()

	f := newIdentifyRelationshipScanFixture(t, false, true)
	book, file := f.retrieve(t)
	libraryPath := filepath.Dir(file.Filepath)
	_, err := f.tc.db.NewUpdate().Model((*models.Library)(nil)).
		Set("organize_file_structure = ?", true).Where("id = ?", book.LibraryID).Exec(f.tc.ctx)
	require.NoError(t, err)
	e := newIdentifyApplyServer(t, f.tc)

	postIdentifyApply(t, e, plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields: map[string]any{
			"authors":   []map[string]string{{"name": "Org Author"}},
			"narrators": []string{"Org Narrator"},
		},
		PluginScope: "test", PluginID: "relationship-enricher",
	})
	book, file = f.retrieve(t)
	require.Equal(t, filepath.Join(libraryPath, "[Org Author] Embedded Title"), book.Filepath)
	require.Equal(t, filepath.Join(book.Filepath, "Embedded Title {Org Narrator}.m4b"), file.Filepath)
	require.FileExists(t, file.Filepath)

	postIdentifyApply(t, e, plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields:      map[string]any{"authors": []any{}, "narrators": []string{}},
		PluginScope: "test", PluginID: "relationship-enricher",
	})
	book, file = f.retrieve(t)
	require.Empty(t, book.Authors)
	require.Equal(t, models.DataSourceManual, book.AuthorSource)
	require.Empty(t, file.Narrators)
	require.Equal(t, models.DataSourceManual, *file.NarratorSource)
	require.Equal(t, filepath.Join(libraryPath, "Embedded Title"), book.Filepath)
	require.Equal(t, filepath.Join(book.Filepath, "Embedded Title.m4b"), file.Filepath)
	require.FileExists(t, file.Filepath)
	bookSidecar, err := sidecar.ReadBookSidecarFromModel(book, file)
	require.NoError(t, err)
	require.NotNil(t, bookSidecar)
	assert.Empty(t, bookSidecar.Authors, "organization must not restore the old author in the book sidecar")
	fileSidecar, err := sidecar.ReadFileSidecar(file.Filepath)
	require.NoError(t, err)
	require.NotNil(t, fileSidecar)
	assert.Empty(t, fileSidecar.Narrators)

	f.ordinaryScan(t, file.ID)
	book, file = f.retrieve(t)
	assert.Empty(t, book.Authors, "cleared authors must stay empty")
	assert.Equal(t, models.DataSourceManual, book.AuthorSource)
	assert.Empty(t, file.Narrators, "cleared narrators must stay empty")
	assert.Equal(t, models.DataSourceManual, *file.NarratorSource)
	assert.Equal(t, filepath.Join(libraryPath, "Embedded Title"), book.Filepath)
	assert.Equal(t, filepath.Join(book.Filepath, "Embedded Title.m4b"), file.Filepath)
	require.FileExists(t, file.Filepath)
}

func TestIdentifyApply_RelationshipPartialRemoval_ThenOrdinaryScan(t *testing.T) {
	t.Parallel()

	f := newIdentifyRelationshipScanFixture(t, true, false)
	book, file := f.retrieve(t)
	assertIdentifyRelationships(t, book, file, "Plugin", true, models.PluginDataSource("test", "relationship-enricher"))
	require.False(t, t.Failed(), "precondition: auto-enrichment supplies two entries for every relationship")

	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: identifyRelationshipFields("Plugin", false),
		Sources: map[string]string{
			"authors": plugins.SourceIntentUser, "genres": plugins.SourceIntentUser,
			"tags": plugins.SourceIntentUser, "narrators": plugins.SourceIntentUser,
		},
		PluginScope: "test",
		PluginID:    "relationship-enricher",
	})
	book, file = f.retrieve(t)
	assertIdentifyRelationships(t, book, file, "Plugin", false, models.DataSourceManual)

	// The enricher still offers both entries. A partial user removal protects
	// the remaining collection instead of accepting that proposal again.
	f.ordinaryScan(t, file.ID)
	book, file = f.retrieve(t)
	assertIdentifyRelationships(t, book, file, "Plugin", false, models.DataSourceManual)
}
