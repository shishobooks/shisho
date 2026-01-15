package worker

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestShouldUpdateScalar(t *testing.T) {
	tests := []struct {
		name           string
		newValue       string
		existingValue  string
		newSource      string
		existingSource string
		want           bool
	}{
		{
			name:           "higher priority source with value updates",
			newValue:       "New Title",
			existingValue:  "Old Title",
			newSource:      models.DataSourceSidecar,
			existingSource: models.DataSourceEPUBMetadata,
			want:           true,
		},
		{
			name:           "higher priority source with empty value does not update",
			newValue:       "",
			existingValue:  "Old Title",
			newSource:      models.DataSourceSidecar,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "higher priority source with same value does not update",
			newValue:       "Same Title",
			existingValue:  "Same Title",
			newSource:      models.DataSourceSidecar,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "same priority with different value updates",
			newValue:       "New Title",
			existingValue:  "Old Title",
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           true,
		},
		{
			name:           "same priority with same value does not update",
			newValue:       "Same Title",
			existingValue:  "Same Title",
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "same priority with empty new value does not update",
			newValue:       "",
			existingValue:  "Old Title",
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "lower priority source never updates",
			newValue:       "New Title",
			existingValue:  "Old Title",
			newSource:      models.DataSourceFilepath,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "manual source is never overwritten",
			newValue:       "New Title",
			existingValue:  "Manual Title",
			newSource:      models.DataSourceSidecar,
			existingSource: models.DataSourceManual,
			want:           false,
		},
		{
			name:           "empty existing source treated as filepath priority",
			newValue:       "New Title",
			existingValue:  "Old Title",
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: "",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUpdateScalar(tt.newValue, tt.existingValue, tt.newSource, tt.existingSource)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldUpdateRelationship(t *testing.T) {
	tests := []struct {
		name           string
		newItems       []string
		existingItems  []string
		newSource      string
		existingSource string
		want           bool
	}{
		{
			name:           "higher priority source with items updates",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Author B"},
			newSource:      models.DataSourceSidecar,
			existingSource: models.DataSourceEPUBMetadata,
			want:           true,
		},
		{
			name:           "higher priority source with empty items does not update",
			newItems:       []string{},
			existingItems:  []string{"Author B"},
			newSource:      models.DataSourceSidecar,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "higher priority source with same items does not update",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Author A"},
			newSource:      models.DataSourceSidecar,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "same priority with different items updates",
			newItems:       []string{"Author A", "Author B"},
			existingItems:  []string{"Author C"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           true,
		},
		{
			name:           "same priority with same items does not update",
			newItems:       []string{"Author A", "Author B"},
			existingItems:  []string{"Author A", "Author B"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "same priority with same items different order updates",
			newItems:       []string{"Author B", "Author A"},
			existingItems:  []string{"Author A", "Author B"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           true,
		},
		{
			name:           "same priority with empty new items does not update",
			newItems:       []string{},
			existingItems:  []string{"Author A"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "lower priority source never updates",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Author B"},
			newSource:      models.DataSourceFilepath,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "manual source is never overwritten",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Manual Author"},
			newSource:      models.DataSourceSidecar,
			existingSource: models.DataSourceManual,
			want:           false,
		},
		{
			name:           "empty existing source treated as filepath priority",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Author B"},
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: "",
			want:           true,
		},
		{
			name:           "nil existing items with new items updates",
			newItems:       []string{"Author A"},
			existingItems:  nil,
			newSource:      models.DataSourceEPUBMetadata,
			existingSource: models.DataSourceFilepath,
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUpdateRelationship(tt.newItems, tt.existingItems, tt.newSource, tt.existingSource)
			assert.Equal(t, tt.want, got)
		})
	}
}
