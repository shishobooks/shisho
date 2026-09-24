package worker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const identifierEnricherSource = "plugin:test/auto-enricher"

func identifierEntryField(idType, value, intent string) map[string]any {
	entry := map[string]any{"type": idType, "value": value}
	if intent != "" {
		entry["source"] = intent
	}
	return entry
}

func identifierSourcesByType(file *models.File) map[string]string {
	sources := make(map[string]string, len(file.Identifiers))
	for _, id := range file.Identifiers {
		sources[id.Type] = id.Source
	}
	return sources
}

// The auto-enricher proposes an ISBN-13 and an ASIN on every ordinary Scan, so
// the fixture's File starts with both at plugin priority.
func requireEnricherIdentifiers(t *testing.T, file *models.File) {
	t.Helper()
	require.NotNil(t, file.IdentifierSource, "precondition: the first Scan stores enricher identifiers")
	require.Equal(t, identifierEnricherSource, *file.IdentifierSource)
	require.Equal(t, map[string]string{"isbn_13": identifierEnricherSource, "asin": identifierEnricherSource}, identifierSourcesByType(file))
}

func TestIdentifyIdentifiers_DuplicateTypeIsRejectedBeforeDelete(t *testing.T) {
	t.Parallel()
	f := newIdentifyScanFixture(t)
	book, file := f.retrieve(t)
	requireEnricherIdentifiers(t, file)

	payload := plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields: map[string]any{"identifiers": []any{
			identifierEntryField("asin", "B01AAA", plugins.SourceIntentUser),
			identifierEntryField("asin", "B02BBB", plugins.SourceIntentUser),
		}},
		Sources: map[string]string{"identifiers": plugins.SourceIntentUser}, PluginScope: "test", PluginID: "auto-enricher",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/plugins/apply", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newIdentifyApplyServer(t, f.tc).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "duplicate identifier type: asin")

	_, updated := f.retrieve(t)
	requireEnricherIdentifiers(t, updated)
}

func TestIdentifyIdentifiers_NoOpPreservesSourcesWithoutWrites(t *testing.T) {
	t.Parallel()
	f := newIdentifyScanFixture(t)
	book, file := f.retrieve(t)
	requireEnricherIdentifiers(t, file)
	_, err := f.tc.db.NewRaw("UPDATE files SET identifier_source = ? WHERE id = ?", models.DataSourceManual, file.ID).Exec(f.tc.ctx)
	require.NoError(t, err)
	_, err = f.tc.db.NewRaw("UPDATE file_identifiers SET source = ? WHERE file_id = ? AND type = ?", models.DataSourceManual, file.ID, "asin").Exec(f.tc.ctx)
	require.NoError(t, err)

	writes := &identifyWrites{}
	f.tc.db.AddQueryHook(writes)
	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields: map[string]any{"identifiers": []any{
			// Reordered and unnormalized, but the same set.
			identifierEntryField("asin", "b01abc1234", plugins.SourceIntentPlugin),
			identifierEntryField("isbn_13", "978-0-316-76948-8", plugins.SourceIntentPlugin),
		}},
		Sources: map[string]string{"identifiers": plugins.SourceIntentPlugin}, PluginScope: "test", PluginID: "auto-enricher",
	})

	_, updated := f.retrieve(t)
	require.NotNil(t, updated.IdentifierSource)
	assert.Equal(t, models.DataSourceManual, *updated.IdentifierSource, "a no-op must not downgrade a manual aggregate")
	assert.Equal(t, map[string]string{"isbn_13": identifierEnricherSource, "asin": models.DataSourceManual}, identifierSourcesByType(updated))
	assert.Empty(t, writes.queries, "a no-op must not delete or reinsert identifiers")
}

func TestIdentifyIdentifiers_MixedCollectionKeepsPerEntryOrigin(t *testing.T) {
	t.Parallel()
	f := newIdentifyScanFixture(t)
	book, file := f.retrieve(t)
	requireEnricherIdentifiers(t, file)

	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields: map[string]any{"identifiers": []any{
			identifierEntryField("isbn_13", "9780316769488", plugins.SourceIntentPlugin),
			identifierEntryField("asin", "B09NEWASIN", plugins.SourceIntentUser),
			identifierEntryField("goodreads", "12345", plugins.SourceIntentUser),
		}},
		Sources: map[string]string{"identifiers": plugins.SourceIntentUser}, PluginScope: "test", PluginID: "auto-enricher",
	})

	_, updated := f.retrieve(t)
	require.NotNil(t, updated.IdentifierSource)
	assert.Equal(t, models.DataSourceManual, *updated.IdentifierSource)
	assert.Equal(t, map[string]string{
		"isbn_13":   identifierEnricherSource,
		"asin":      models.DataSourceManual,
		"goodreads": models.DataSourceManual,
	}, identifierSourcesByType(updated))
}

func TestIdentifyIdentifiers_ClearThenOrdinaryScanRepopulates(t *testing.T) {
	t.Parallel()
	f := newIdentifyScanFixture(t)
	book, file := f.retrieve(t)
	requireEnricherIdentifiers(t, file)

	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields:  map[string]any{"identifiers": []any{}},
		Sources: map[string]string{"identifiers": plugins.SourceIntentUser}, PluginScope: "test", PluginID: "auto-enricher",
	})

	_, updated := f.retrieve(t)
	assert.Empty(t, updated.Identifiers)
	assert.Nil(t, updated.IdentifierSource, "an Explicit Clear leaves no source behind")

	f.ordinaryScan(t, file.ID)

	_, rescanned := f.retrieve(t)
	requireEnricherIdentifiers(t, rescanned)
}

func TestIdentifyIdentifiers_PartialRemovalSurvivesOrdinaryScan(t *testing.T) {
	t.Parallel()
	f := newIdentifyScanFixture(t)
	book, file := f.retrieve(t)
	requireEnricherIdentifiers(t, file)

	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields:  map[string]any{"identifiers": []any{identifierEntryField("isbn_13", "9780316769488", plugins.SourceIntentPlugin)}},
		Sources: map[string]string{"identifiers": plugins.SourceIntentUser}, PluginScope: "test", PluginID: "auto-enricher",
	})

	_, updated := f.retrieve(t)
	require.NotNil(t, updated.IdentifierSource)
	require.Equal(t, models.DataSourceManual, *updated.IdentifierSource)
	require.Equal(t, map[string]string{"isbn_13": identifierEnricherSource}, identifierSourcesByType(updated), "the retained entry keeps its origin")

	f.ordinaryScan(t, file.ID)

	_, rescanned := f.retrieve(t)
	require.NotNil(t, rescanned.IdentifierSource)
	assert.Equal(t, models.DataSourceManual, *rescanned.IdentifierSource, "a manual collection must survive auto-enrichment")
	assert.Equal(t, map[string]string{"isbn_13": identifierEnricherSource}, identifierSourcesByType(rescanned))
}

func TestIdentifyIdentifiers_AcceptedProposalUsesPluginAggregate(t *testing.T) {
	t.Parallel()
	f := newIdentifyScanFixture(t)
	book, file := f.retrieve(t)
	requireEnricherIdentifiers(t, file)
	_, err := f.tc.db.NewRaw("UPDATE files SET identifier_source = ? WHERE id = ?", models.DataSourceManual, file.ID).Exec(f.tc.ctx)
	require.NoError(t, err)

	// Replace the ASIN with the value a newer proposal carries.
	postIdentifyApply(t, newIdentifyApplyServer(t, f.tc), plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields: map[string]any{"identifiers": []any{
			identifierEntryField("isbn_13", "9780316769488", plugins.SourceIntentPlugin),
			identifierEntryField("asin", "B09PROPOSED", plugins.SourceIntentPlugin),
		}},
		Sources: map[string]string{"identifiers": plugins.SourceIntentPlugin}, PluginScope: "test", PluginID: "auto-enricher",
	})

	_, updated := f.retrieve(t)
	require.NotNil(t, updated.IdentifierSource)
	assert.Equal(t, identifierEnricherSource, *updated.IdentifierSource)
	assert.Equal(t, map[string]string{"isbn_13": identifierEnricherSource, "asin": identifierEnricherSource}, identifierSourcesByType(updated))
}
