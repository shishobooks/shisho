package worker

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/mediafile"
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
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           true,
		},
		{
			name:           "higher priority source with empty value does not update",
			newValue:       "",
			existingValue:  "Old Title",
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "higher priority source with same value does not update",
			newValue:       "Same Title",
			existingValue:  "Same Title",
			newSource:      models.DataSourceFileMetadata,
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
			newSource:      models.DataSourceFileMetadata,
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
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           true,
		},
		{
			name:           "higher priority source with empty items does not update",
			newItems:       []string{},
			existingItems:  []string{"Author B"},
			newSource:      models.DataSourceFileMetadata,
			existingSource: models.DataSourceEPUBMetadata,
			want:           false,
		},
		{
			name:           "higher priority source with same items does not update",
			newItems:       []string{"Author A"},
			existingItems:  []string{"Author A"},
			newSource:      models.DataSourceFileMetadata,
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
			newSource:      models.DataSourceFileMetadata,
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

func TestGenerateCBZFileName(t *testing.T) {
	floatPtr := func(f float64) *float64 { return &f }

	tests := []struct {
		name     string
		metadata *mediafile.ParsedMetadata
		filename string
		want     string
	}{
		{
			name: "title from metadata is preferred over series+number",
			metadata: &mediafile.ParsedMetadata{
				Title:        "My Awesome Comic",
				Series:       "Some Series",
				SeriesNumber: floatPtr(1),
			},
			filename: "[Author] Some Series v1.cbz",
			want:     "My Awesome Comic",
		},
		{
			name: "series+number used when title is empty",
			metadata: &mediafile.ParsedMetadata{
				Title:        "",
				Series:       "Demon Slayer",
				SeriesNumber: floatPtr(1),
			},
			filename: "[Koyoharu Gotouge] Demon Slayer v1.cbz",
			want:     "Demon Slayer v1",
		},
		{
			name: "series+number used when title looks like filename with brackets",
			metadata: &mediafile.ParsedMetadata{
				Title:        "[Author] Comic Title v1",
				Series:       "Comic Title",
				SeriesNumber: floatPtr(1),
			},
			filename: "[Author] Comic Title v1.cbz",
			want:     "Comic Title v1",
		},
		{
			name: "series only when no number",
			metadata: &mediafile.ParsedMetadata{
				Title:        "",
				Series:       "One Piece",
				SeriesNumber: nil,
			},
			filename: "[Oda] One Piece.cbz",
			want:     "One Piece",
		},
		{
			name: "decimal series number preserved",
			metadata: &mediafile.ParsedMetadata{
				Title:        "",
				Series:       "Naruto",
				SeriesNumber: floatPtr(1.5),
			},
			filename: "[Kishimoto] Naruto v1.5.cbz",
			want:     "Naruto v1.5",
		},
		{
			name: "parse from filename when no metadata",
			metadata: &mediafile.ParsedMetadata{
				Title:        "",
				Series:       "",
				SeriesNumber: nil,
			},
			filename: "[Author Name] Comic Title v1.cbz",
			want:     "Comic Title v1",
		},
		{
			name: "parse from filename with multiple bracket sections",
			metadata: &mediafile.ParsedMetadata{
				Title:        "",
				Series:       "",
				SeriesNumber: nil,
			},
			filename: "[Author] [Publisher] Comic Title.cbz",
			want:     "Comic Title",
		},
		{
			name: "whitespace-only title falls through to series",
			metadata: &mediafile.ParsedMetadata{
				Title:        "   ",
				Series:       "My Series",
				SeriesNumber: floatPtr(5),
			},
			filename: "whatever.cbz",
			want:     "My Series v5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateCBZFileName(tt.metadata, tt.filename)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCleanCBZFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{
			name:     "removes author brackets and extension",
			filename: "[Author Name] Comic Title v1.cbz",
			want:     "Comic Title v1",
		},
		{
			name:     "removes multiple bracket sections",
			filename: "[Author] [Publisher] [Year] Comic Title.cbz",
			want:     "Comic Title",
		},
		{
			name:     "handles no brackets",
			filename: "Comic Title v1.cbz",
			want:     "Comic Title v1",
		},
		{
			name:     "collapses multiple spaces",
			filename: "[Author]   Comic   Title.cbz",
			want:     "Comic Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanCBZFilename(tt.filename)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatSeriesNumber(t *testing.T) {
	tests := []struct {
		name string
		num  float64
		want string
	}{
		{name: "whole number", num: 1, want: "1"},
		{name: "whole number larger", num: 42, want: "42"},
		{name: "decimal", num: 1.5, want: "1.5"},
		{name: "decimal with trailing zeros", num: 2.50, want: "2.5"},
		{name: "zero", num: 0, want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSeriesNumber(tt.num)
			assert.Equal(t, tt.want, got)
		})
	}
}
