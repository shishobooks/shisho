package plugins

import (
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Identifier attribution (ADR 0006): aggregate provenance gates Scan
// replacement and per-entry provenance records each entry's origin.

func identifierEntry(idType, value, intent string) map[string]any {
	entry := map[string]any{"type": idType, "value": value}
	if intent != "" {
		entry["source"] = intent
	}
	return entry
}

func newIdentifierFixture(t *testing.T, aggregateSource string, existing ...*models.FileIdentifier) (*models.Book, *models.File, *stubIdentStoreForPersist, *handler) {
	t.Helper()
	book, file, _, h := newAttributionFixture(t, "epub_metadata")
	identStore := &stubIdentStoreForPersist{}
	h.enrich.identStore = identStore
	for _, id := range existing {
		id.FileID = file.ID
	}
	file.Identifiers = existing
	if aggregateSource != "" {
		file.IdentifierSource = &aggregateSource
	}
	return book, file, identStore, h
}

func bulkSourcesByType(t *testing.T, identStore *stubIdentStoreForPersist) map[string]string {
	t.Helper()
	require.Len(t, identStore.bulkCalls, 1)
	sources := make(map[string]string, len(identStore.bulkCalls[0]))
	for _, id := range identStore.bulkCalls[0] {
		sources[id.Type] = id.Source
	}
	return sources
}

// Defect: duplicate types were not rejected, so the delete ran and the
// store's dedupe left the File with a single (or zero) Identifier.
func TestApplyMetadata_Identifiers_RejectsDuplicateTypesBeforeDelete(t *testing.T) {
	t.Parallel()

	book, file, identStore, h := newIdentifierFixture(t, models.DataSourceManual,
		&models.FileIdentifier{Type: "asin", Value: "B01ORIGINAL", Source: models.DataSourceManual})

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{
			"identifiers": []any{identifierEntry("asin", "B01AAA", ""), identifierEntry(" asin ", "B02BBB", "")},
		},
	})

	var ec *errcodes.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, http.StatusUnprocessableEntity, ec.HTTPCode)
	assert.Contains(t, ec.Error(), "duplicate identifier type: asin")
	assert.Empty(t, identStore.deleteCalls, "the existing collection must be untouched")
	assert.Empty(t, identStore.bulkCalls)
	require.NotNil(t, file.IdentifierSource)
	assert.Equal(t, models.DataSourceManual, *file.IdentifierSource)
}

// Defect: every apply deleted and reinserted the collection with the plugin
// source, so retained entries lost their origin and a manual aggregate was
// downgraded even when nothing changed.
func TestApplyMetadata_Identifiers_NoOpPreservesBothLayers(t *testing.T) {
	t.Parallel()

	for _, aggregate := range []string{models.DataSourceManual, applyTestPluginSource} {
		t.Run(aggregate, func(t *testing.T) {
			t.Parallel()
			book, file, identStore, h := newIdentifierFixture(t, aggregate,
				&models.FileIdentifier{Type: "isbn_13", Value: "9780316769488", Source: models.DataSourceEPUBMetadata},
				&models.FileIdentifier{Type: "asin", Value: "B01ABC1234", Source: models.DataSourceManual})

			err := applyForAttribution(t, h, PluginApplyPayload{
				BookID: book.ID,
				FileID: &file.ID,
				Fields: map[string]any{
					// Reordered and unnormalized, but the same (type, normalized value) set.
					"identifiers": []any{
						identifierEntry("asin", "b01abc1234", SourceIntentPlugin),
						identifierEntry("isbn_13", "978-0-316-76948-8", SourceIntentPlugin),
					},
				},
				Sources: map[string]string{"identifiers": SourceIntentPlugin},
			})
			require.NoError(t, err)

			assert.Empty(t, identStore.deleteCalls, "a no-op must not delete the collection")
			assert.Empty(t, identStore.bulkCalls, "a no-op must not reinsert the collection")
			require.NotNil(t, file.IdentifierSource)
			assert.Equal(t, aggregate, *file.IdentifierSource)
			assert.Equal(t, models.DataSourceEPUBMetadata, file.Identifiers[0].Source)
			assert.Equal(t, models.DataSourceManual, file.Identifiers[1].Source)
		})
	}
}

func TestApplyMetadata_Identifiers_PerEntrySourcesFollowIntent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		fieldIntent   string
		wantAggregate string
	}{
		{name: "plugin field intent", fieldIntent: SourceIntentPlugin, wantAggregate: applyTestPluginSource},
		{name: "user field intent", fieldIntent: SourceIntentUser, wantAggregate: models.DataSourceManual},
		{name: "missing field intent", fieldIntent: "", wantAggregate: models.DataSourceManual},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			book, file, identStore, h := newIdentifierFixture(t, models.DataSourceEPUBMetadata,
				&models.FileIdentifier{Type: "asin", Value: "B01ABC1234", Source: models.DataSourceEPUBMetadata})

			var sources map[string]string
			if tc.fieldIntent != "" {
				sources = map[string]string{"identifiers": tc.fieldIntent}
			}
			err := applyForAttribution(t, h, PluginApplyPayload{
				BookID: book.ID,
				FileID: &file.ID,
				Fields: map[string]any{
					"identifiers": []any{
						// Retained: plugin intent must not override the stored origin.
						identifierEntry("asin", "B01ABC1234", SourceIntentPlugin),
						// Accepted from the proposal.
						identifierEntry("isbn_13", "9780316769488", SourceIntentPlugin),
						// Added by hand, explicit and implicit.
						identifierEntry("goodreads", "12345", SourceIntentUser),
						identifierEntry("google", "abc", ""),
					},
				},
				Sources: sources,
			})
			require.NoError(t, err)

			assert.Equal(t, []int{file.ID}, identStore.deleteCalls)
			assert.Equal(t, map[string]string{
				"asin":      models.DataSourceEPUBMetadata,
				"isbn_13":   applyTestPluginSource,
				"goodreads": models.DataSourceManual,
				"google":    models.DataSourceManual,
			}, bulkSourcesByType(t, identStore))
			require.NotNil(t, file.IdentifierSource)
			assert.Equal(t, tc.wantAggregate, *file.IdentifierSource)
		})
	}
}

func TestApplyMetadata_Identifiers_ReplacedValueUsesEntryIntent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		entryIntent string
		wantSource  string
	}{
		{entryIntent: SourceIntentPlugin, wantSource: applyTestPluginSource},
		{entryIntent: SourceIntentUser, wantSource: models.DataSourceManual},
		{entryIntent: "", wantSource: models.DataSourceManual},
	} {
		name := tc.entryIntent
		if name == "" {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			book, file, identStore, h := newIdentifierFixture(t, "plugin:shisho/audnexus",
				&models.FileIdentifier{Type: "asin", Value: "B01ORIG", Source: "plugin:shisho/audnexus"})

			err := applyForAttribution(t, h, PluginApplyPayload{
				BookID: book.ID,
				FileID: &file.ID,
				Fields: map[string]any{
					"identifiers": []any{identifierEntry("asin", "B02NEW", tc.entryIntent)},
				},
				Sources: map[string]string{"identifiers": SourceIntentUser},
			})
			require.NoError(t, err)

			assert.Equal(t, map[string]string{"asin": tc.wantSource}, bulkSourcesByType(t, identStore))
			require.NotNil(t, file.IdentifierSource)
			assert.Equal(t, models.DataSourceManual, *file.IdentifierSource)
		})
	}
}

func TestApplyMetadata_Identifiers_ExplicitClearRemovesBothLayers(t *testing.T) {
	t.Parallel()

	book, file, identStore, h := newIdentifierFixture(t, applyTestPluginSource,
		&models.FileIdentifier{Type: "asin", Value: "B01ABC1234", Source: applyTestPluginSource})

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{"identifiers": []any{}},
		// A clear carries user intent, which must not produce a manual tombstone.
		Sources: map[string]string{"identifiers": SourceIntentUser},
	})
	require.NoError(t, err)

	assert.Equal(t, []int{file.ID}, identStore.deleteCalls)
	assert.Empty(t, identStore.bulkCalls)
	assert.Nil(t, file.IdentifierSource)
}

func TestApplyMetadata_Identifiers_ClearingEmptyCollectionRemovesStaleSource(t *testing.T) {
	t.Parallel()

	book, file, identStore, h := newIdentifierFixture(t, applyTestPluginSource)

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{"identifiers": []any{}},
	})
	require.NoError(t, err)

	assert.Empty(t, identStore.bulkCalls)
	assert.Nil(t, file.IdentifierSource, "a stale source on an empty collection blocks Scan repopulation")
}

func TestApplyMetadata_Identifiers_ClearingEmptyUnsourcedCollectionIsNoOp(t *testing.T) {
	t.Parallel()

	book, file, identStore, h := newIdentifierFixture(t, "")

	err := applyForAttribution(t, h, PluginApplyPayload{
		BookID: book.ID,
		FileID: &file.ID,
		Fields: map[string]any{"identifiers": []any{}},
	})
	require.NoError(t, err)

	assert.Empty(t, identStore.deleteCalls)
	assert.Empty(t, identStore.bulkCalls)
	assert.Nil(t, file.IdentifierSource)
}
