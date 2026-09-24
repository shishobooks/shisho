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

// An Explicit Clear leaves no tombstone (ADR 0006): the next ordinary Scan
// may repopulate the field from embedded file metadata, which ranks below
// every plugin source. These fixtures use native parsers and an enricher
// that proposes nothing, so the restored values can only come from the file.

const identifyScalarClearManifest = `{
  "manifestVersion": 1,
  "id": "silent-enricher",
  "name": "Silent Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "fileTypes": ["epub", "m4b"],
      "fields": ["description", "publisher"]
    }
  }
}`

func newIdentifyScalarClearContext(t *testing.T) (*testContext, string) {
	t.Helper()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	installTestPlugin(t, tc, pluginDir, "silent-enricher", identifyScalarClearManifest, `var plugin = {metadataEnricher: {search: function(ctx) {return {results: []};}}};`)
	require.NoError(t, tc.worker.pluginService.AppendToOrder(tc.ctx, models.PluginHookMetadataEnricher, "test", "silent-enricher"))
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	return tc, testgen.CreateSubDir(t, libraryPath, "Embedded Title")
}

func TestIdentifyApply_ScalarClear_ThenOrdinaryScan_EPUB(t *testing.T) {
	t.Parallel()

	tc, bookDir := newIdentifyScalarClearContext(t)
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:       "Embedded Title",
		Authors:     []string{"Embedded Author"},
		Description: "Embedded description",
		Publisher:   "Embedded Publisher",
		Language:    "fr",
		Date:        "2020-01-02",
	})
	require.NoError(t, tc.runScan())
	f := &identifyScanFixture{tc: tc}

	book, file := f.retrieve(t)
	require.NotNil(t, book.Description, "precondition: the Scan reads embedded metadata")
	require.Equal(t, "Embedded description", *book.Description)
	require.Equal(t, models.DataSourceEPUBMetadata, *book.DescriptionSource)
	require.NotNil(t, file.Publisher)
	require.Equal(t, "Embedded Publisher", file.Publisher.Name)
	require.Equal(t, "fr", *file.Language)
	require.NotNil(t, file.ReleaseDate)

	postIdentifyApply(t, newIdentifyApplyServer(t, tc), plugins.PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			"description":  "",
			"publisher":    "",
			"language":     "",
			"release_date": "",
		},
		PluginScope: "test",
		PluginID:    "silent-enricher",
	})

	book, file = f.retrieve(t)
	require.Nil(t, book.Description)
	require.Nil(t, book.DescriptionSource)
	require.Nil(t, file.PublisherID)
	require.Nil(t, file.PublisherSource)
	require.Nil(t, file.Language)
	require.Nil(t, file.LanguageSource)
	require.Nil(t, file.ReleaseDate)
	require.Nil(t, file.ReleaseDateSource)

	f.ordinaryScan(t, file.ID)

	book, file = f.retrieve(t)
	require.NotNil(t, book.Description, "a cleared description must be repopulated from embedded metadata")
	assert.Equal(t, "Embedded description", *book.Description)
	assert.Equal(t, models.DataSourceEPUBMetadata, *book.DescriptionSource)
	require.NotNil(t, file.Publisher, "a cleared publisher must be repopulated from embedded metadata")
	assert.Equal(t, "Embedded Publisher", file.Publisher.Name)
	assert.Equal(t, models.DataSourceEPUBMetadata, *file.PublisherSource)
	require.NotNil(t, file.Language, "a cleared language must be repopulated from embedded metadata")
	assert.Equal(t, "fr", *file.Language)
	assert.Equal(t, models.DataSourceEPUBMetadata, *file.LanguageSource)
	require.NotNil(t, file.ReleaseDate, "a cleared release date must be repopulated from embedded metadata")
	assert.Equal(t, "2020-01-02", file.ReleaseDate.UTC().Format("2006-01-02"))
	assert.Equal(t, models.DataSourceEPUBMetadata, *file.ReleaseDateSource)
}

func TestIdentifyApply_ScalarClear_ThenOrdinaryScan_M4B(t *testing.T) {
	t.Parallel()
	testgen.SkipIfNoFFmpeg(t)

	tc, bookDir := newIdentifyScalarClearContext(t)
	path := testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{Title: "Embedded Title", Artist: "Embedded Author"})
	metadata, err := mp4.ParseFull(path)
	require.NoError(t, err)
	metadata.Subtitle = "Embedded Subtitle"
	metadata.Description = "Embedded description"
	if metadata.Freeform == nil {
		metadata.Freeform = map[string]string{}
	}
	metadata.Freeform["com.shisho:url"] = "https://example.com/embedded"
	metadata.Freeform["com.pilabor.tone:LANGUAGE"] = "fr"
	metadata.Freeform["com.pilabor.tone:ABRIDGED"] = "true"
	require.NoError(t, mp4.Write(path, metadata, mp4.WriteOptions{}))
	require.NoError(t, tc.runScan())
	f := &identifyScanFixture{tc: tc}

	book, file := f.retrieve(t)
	require.NotNil(t, book.Subtitle, "precondition: the Scan reads embedded metadata")
	require.Equal(t, "Embedded Subtitle", *book.Subtitle)
	require.Equal(t, models.DataSourceM4BMetadata, *book.SubtitleSource)
	require.NotNil(t, book.Description)
	require.NotNil(t, file.URL)
	require.Equal(t, "fr", *file.Language)
	require.NotNil(t, file.Abridged)
	require.True(t, *file.Abridged)

	postIdentifyApply(t, newIdentifyApplyServer(t, tc), plugins.PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			"subtitle":    "",
			"description": "",
			"url":         "",
			"language":    "",
			"abridged":    nil,
		},
		PluginScope: "test",
		PluginID:    "silent-enricher",
	})

	book, file = f.retrieve(t)
	require.Nil(t, book.Subtitle)
	require.Nil(t, book.SubtitleSource)
	require.Nil(t, book.Description)
	require.Nil(t, book.DescriptionSource)
	require.Nil(t, file.URL)
	require.Nil(t, file.URLSource)
	require.Nil(t, file.Language)
	require.Nil(t, file.LanguageSource)
	require.Nil(t, file.Abridged)
	require.Nil(t, file.AbridgedSource)

	f.ordinaryScan(t, file.ID)

	book, file = f.retrieve(t)
	require.NotNil(t, book.Subtitle, "a cleared subtitle must be repopulated from embedded metadata")
	assert.Equal(t, "Embedded Subtitle", *book.Subtitle)
	assert.Equal(t, models.DataSourceM4BMetadata, *book.SubtitleSource)
	require.NotNil(t, book.Description, "a cleared description must be repopulated from embedded metadata")
	assert.Equal(t, "Embedded description", *book.Description)
	assert.Equal(t, models.DataSourceM4BMetadata, *book.DescriptionSource)
	require.NotNil(t, file.URL, "a cleared URL must be repopulated from embedded metadata")
	assert.Equal(t, "https://example.com/embedded", *file.URL)
	assert.Equal(t, models.DataSourceM4BMetadata, *file.URLSource)
	require.NotNil(t, file.Language, "a cleared language must be repopulated from embedded metadata")
	assert.Equal(t, "fr", *file.Language)
	assert.Equal(t, models.DataSourceM4BMetadata, *file.LanguageSource)
	require.NotNil(t, file.Abridged, "a cleared abridged flag must be repopulated from embedded metadata")
	assert.True(t, *file.Abridged)
	assert.Equal(t, models.DataSourceM4BMetadata, *file.AbridgedSource)
}
