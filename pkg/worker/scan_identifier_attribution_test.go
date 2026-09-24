package worker

import (
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const scanIdentifierEnricherManifest = `{
  "manifestVersion": 1,
  "id": "id-enricher",
  "name": "Identifier Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "fileTypes": ["epub"],
      "fields": ["identifiers"]
    }
  }
}`

const scanIdentifierEnricherJS = `var plugin = {
  metadataEnricher: {
    search: function(ctx) {
      return { results: [{
        identifiers: [{ type: "isbn_13", value: "9780316769488" }, { type: "asin", value: "B01ABC1234" }]
      }] };
    }
  }
};`

const scanIdentifierEmbeddedUUID = "2d049387-8f5a-4c3e-9b1a-6f2e1d0c9a7b"

// newScanIdentifierFixture scans an EPUB that embeds an ISBN and a UUID while
// an enricher proposes a different ISBN plus an ASIN.
func newScanIdentifierFixture(t *testing.T) *identifyScanFixture {
	t.Helper()

	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	installTestPlugin(t, tc, pluginDir, "id-enricher", scanIdentifierEnricherManifest, scanIdentifierEnricherJS)
	require.NoError(t, tc.worker.pluginService.AppendToOrder(tc.ctx, models.PluginHookMetadataEnricher, "test", "id-enricher"))
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))

	libraryPath := t.TempDir()
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Good Omens")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:   "Good Omens",
		Authors: []string{"Terry Pratchett"},
		Identifiers: []testgen.EPUBIdentifier{
			{Scheme: "ISBN", Value: "9780060853983"},
			{Scheme: "UUID", Value: scanIdentifierEmbeddedUUID},
		},
	})

	require.NoError(t, tc.runScan())
	return &identifyScanFixture{tc: tc}
}

func identifierSources(file *models.File) map[string]string {
	out := make(map[string]string, len(file.Identifiers))
	for _, id := range file.Identifiers {
		out[id.Type+"|"+id.Value] = id.Source
	}
	return out
}

func TestScan_MixedIdentifierCollection_AttributesEachEntry(t *testing.T) {
	t.Parallel()
	f := newScanIdentifierFixture(t)
	_, file := f.retrieve(t)
	pluginSource := models.PluginDataSource("test", "id-enricher")

	// The enricher's ISBN wins the isbn_13 type conflict (plugin priority beats
	// embedded file metadata), so the embedded ISBN is not stored.
	assert.Equal(t, map[string]string{
		"isbn_13|9780316769488":              pluginSource,
		"asin|B01ABC1234":                    pluginSource,
		"uuid|" + scanIdentifierEmbeddedUUID: models.DataSourceEPUBMetadata,
	}, identifierSources(file))
	require.NotNil(t, file.IdentifierSource)
	assert.Equal(t, pluginSource, *file.IdentifierSource, "the aggregate names the highest-priority contributor")
}

// setIdentifierSources overwrites stored identifier sources the way a Scan
// before #492 left them.
func (f *identifyScanFixture) setIdentifierSources(t *testing.T, fileID int, aggregate string, entries map[string]string) {
	t.Helper()
	for typ, source := range entries {
		_, err := f.tc.db.NewRaw("UPDATE file_identifiers SET source = ? WHERE file_id = ? AND type = ?", source, fileID, typ).Exec(f.tc.ctx)
		require.NoError(t, err)
	}
	_, err := f.tc.db.NewRaw("UPDATE files SET identifier_source = ? WHERE id = ?", aggregate, fileID).Exec(f.tc.ctx)
	require.NoError(t, err)
}

func TestScan_MixedIdentifierCollection_OrdinaryScanRepairsLowerPriorityAttribution(t *testing.T) {
	t.Parallel()
	f := newScanIdentifierFixture(t)
	_, file := f.retrieve(t)
	pluginSource := models.PluginDataSource("test", "id-enricher")

	// Every entry and the aggregate mislabeled as the EPUB, as before the fix.
	f.setIdentifierSources(t, file.ID, models.DataSourceEPUBMetadata, map[string]string{
		"isbn_13": models.DataSourceEPUBMetadata,
		"asin":    models.DataSourceEPUBMetadata,
	})

	f.ordinaryScan(t, file.ID)

	_, file = f.retrieve(t)
	assert.Equal(t, map[string]string{
		"isbn_13|9780316769488":              pluginSource,
		"asin|B01ABC1234":                    pluginSource,
		"uuid|" + scanIdentifierEmbeddedUUID: models.DataSourceEPUBMetadata,
	}, identifierSources(file), "unchanged values with a lower-priority aggregate are re-attributed")
	require.NotNil(t, file.IdentifierSource)
	assert.Equal(t, pluginSource, *file.IdentifierSource)
}

func TestScan_MixedIdentifierCollection_OrdinaryScanKeepsManualAttribution(t *testing.T) {
	t.Parallel()
	f := newScanIdentifierFixture(t)
	_, file := f.retrieve(t)

	f.setIdentifierSources(t, file.ID, models.DataSourceManual, map[string]string{
		"isbn_13": models.DataSourceManual,
		"asin":    models.DataSourceManual,
		"uuid":    models.DataSourceManual,
	})

	f.ordinaryScan(t, file.ID)

	_, file = f.retrieve(t)
	assert.Equal(t, map[string]string{
		"isbn_13|9780316769488":              models.DataSourceManual,
		"asin|B01ABC1234":                    models.DataSourceManual,
		"uuid|" + scanIdentifierEmbeddedUUID: models.DataSourceManual,
	}, identifierSources(file))
	require.NotNil(t, file.IdentifierSource)
	assert.Equal(t, models.DataSourceManual, *file.IdentifierSource)
}

func TestScan_MixedIdentifierCollection_ForceRefreshRewritesChangedEntrySource(t *testing.T) {
	t.Parallel()
	f := newScanIdentifierFixture(t)
	_, file := f.retrieve(t)
	pluginSource := models.PluginDataSource("test", "id-enricher")

	// The aggregate already matches; only one entry's origin is stale.
	f.setIdentifierSources(t, file.ID, pluginSource, map[string]string{"uuid": pluginSource})

	_, err := f.tc.worker.scanInternal(f.tc.ctx, ScanOptions{FileID: file.ID, ForceRefresh: true}, nil)
	require.NoError(t, err)

	_, file = f.retrieve(t)
	assert.Equal(t, map[string]string{
		"isbn_13|9780316769488":              pluginSource,
		"asin|B01ABC1234":                    pluginSource,
		"uuid|" + scanIdentifierEmbeddedUUID: models.DataSourceEPUBMetadata,
	}, identifierSources(file))
	require.NotNil(t, file.IdentifierSource)
	assert.Equal(t, pluginSource, *file.IdentifierSource)
}
