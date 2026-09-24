package identifiers

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestReconcileSources_UnchangedEntriesKeepStoredSource(t *testing.T) {
	t.Parallel()

	existing := []*models.FileIdentifier{
		{Type: "asin", Value: "B01ABC1234", Source: "plugin:shisho/audnexus"},
		{Type: "isbn_13", Value: "9780316769488", Source: models.DataSourceEPUBMetadata},
	}
	incoming := []*models.FileIdentifier{
		{Type: "asin", Value: "B01ABC1234", Source: models.DataSourceManual},
		{Type: "isbn_13", Value: "9780316769488", Source: "plugin:test/enricher"},
		{Type: "goodreads", Value: "12345", Source: models.DataSourceManual},
	}

	unchanged := ReconcileSources(existing, incoming)

	assert.False(t, unchanged)
	assert.Equal(t, "plugin:shisho/audnexus", incoming[0].Source, "unchanged entry keeps its stored source")
	assert.Equal(t, models.DataSourceEPUBMetadata, incoming[1].Source, "unchanged entry keeps its stored source even with plugin intent")
	assert.Equal(t, models.DataSourceManual, incoming[2].Source, "new entry keeps the caller's source")
}

func TestReconcileSources_KeysBothSidesOnNormalizedValue(t *testing.T) {
	t.Parallel()

	// A stored value that predates normalization must still match its
	// normalized incoming form, and an unnormalized incoming form must match
	// a normalized stored value.
	existing := []*models.FileIdentifier{
		{Type: "asin", Value: "b01abc1234", Source: "plugin:shisho/audnexus"},
		{Type: "isbn_13", Value: "9780316769488", Source: models.DataSourceEPUBMetadata},
	}
	incoming := []*models.FileIdentifier{
		{Type: "asin", Value: "B01ABC1234", Source: models.DataSourceManual},
		{Type: "isbn_13", Value: "978-0-316-76948-8", Source: models.DataSourceManual},
	}

	unchanged := ReconcileSources(existing, incoming)

	assert.True(t, unchanged)
	assert.Equal(t, "plugin:shisho/audnexus", incoming[0].Source)
	assert.Equal(t, models.DataSourceEPUBMetadata, incoming[1].Source)
}

func TestReconcileSources_ReplacedValueKeepsCallerSource(t *testing.T) {
	t.Parallel()

	existing := []*models.FileIdentifier{{Type: "asin", Value: "B01ORIG", Source: "plugin:shisho/audnexus"}}
	incoming := []*models.FileIdentifier{{Type: "asin", Value: "B02NEW", Source: models.DataSourceManual}}

	unchanged := ReconcileSources(existing, incoming)

	assert.False(t, unchanged)
	assert.Equal(t, models.DataSourceManual, incoming[0].Source)
}

func TestReconcileSources_ReportsUnchangedForEqualSets(t *testing.T) {
	t.Parallel()

	existing := []*models.FileIdentifier{
		{Type: "asin", Value: "B01ABC1234", Source: models.DataSourceManual},
		{Type: "isbn_13", Value: "9780316769488", Source: models.DataSourceManual},
	}
	incoming := []*models.FileIdentifier{
		{Type: "isbn_13", Value: "9780316769488", Source: "plugin:test/enricher"},
		{Type: "asin", Value: "B01ABC1234", Source: "plugin:test/enricher"},
	}
	assert.True(t, ReconcileSources(existing, incoming), "order is not significant")

	assert.True(t, ReconcileSources(nil, nil), "two empty collections are unchanged")
	assert.False(t, ReconcileSources(existing, nil), "a clear is a change")
	assert.False(t, ReconcileSources(nil, incoming), "a first population is a change")
	assert.False(t, ReconcileSources(existing, incoming[:1]), "a partial removal is a change")
}
