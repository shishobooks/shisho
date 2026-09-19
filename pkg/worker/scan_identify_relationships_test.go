package worker

import (
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/shishobooks/shisho/pkg/plugins"
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
// parser. This fixture catches a source left behind after a complete clear.
func newIdentifyRelationshipScanFixture(t *testing.T, autoEnrich bool) *identifyScanFixture {
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
	bookDir := testgen.CreateSubDir(t, libraryPath, "Embedded Title")
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

	f := newIdentifyRelationshipScanFixture(t, false)
	book, file := f.retrieve(t)
	assertIdentifyRelationships(t, book, file, "Embedded", true, models.DataSourceM4BMetadata)
	require.False(t, t.Failed(), "precondition: all four relationships come from native M4B metadata")
	e := newIdentifyApplyServer(t, f.tc)

	// Establish plugin provenance before clearing, so stale provenance would
	// outrank the embedded metadata on the next ordinary Scan.
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
	book, file = f.retrieve(t)
	assert.Empty(t, book.Authors)
	assert.Empty(t, book.BookGenres)
	assert.Empty(t, book.BookTags)
	assert.Empty(t, file.Narrators)
	assert.Empty(t, book.AuthorSource)
	assert.Nil(t, book.GenreSource)
	assert.Nil(t, book.TagSource)
	assert.Nil(t, file.NarratorSource)
	var authorSourceIsNull bool
	require.NoError(t, f.tc.db.NewSelect().Table("books").ColumnExpr("author_source IS NULL").Where("id = ?", book.ID).Scan(f.tc.ctx, &authorSourceIsNull))
	assert.True(t, authorSourceIsNull, "complete clear must remove author provenance from the database")

	f.ordinaryScan(t, file.ID)
	book, file = f.retrieve(t)
	assertIdentifyRelationships(t, book, file, "Embedded", true, models.DataSourceM4BMetadata)
}

func TestIdentifyApply_RelationshipPartialRemoval_ThenOrdinaryScan(t *testing.T) {
	t.Parallel()

	f := newIdentifyRelationshipScanFixture(t, true)
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
