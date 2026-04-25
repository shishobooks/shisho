package chapters

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestShouldUpdateChapters(t *testing.T) {
	t.Parallel()

	href := "ch1.xhtml"
	chapters := []mediafile.ParsedChapter{{Title: "Chapter 1", Href: &href}}

	manualSource := models.DataSourceManual
	sidecarSource := models.DataSourceSidecar
	pluginSource := models.DataSourcePluginPrefix + "test"
	m4bSource := models.DataSourceM4BMetadata
	filepathSource := models.DataSourceFilepath

	tests := []struct {
		name           string
		chapters       []mediafile.ParsedChapter
		newSource      string
		existingSource *string
		forceRefresh   bool
		want           bool
	}{
		{
			name:           "empty chapters never apply",
			chapters:       nil,
			newSource:      m4bSource,
			existingSource: nil,
			forceRefresh:   true,
			want:           false,
		},
		{
			name:           "force refresh + file source overrides manual",
			chapters:       chapters,
			newSource:      m4bSource,
			existingSource: &manualSource,
			forceRefresh:   true,
			want:           true,
		},
		{
			name:           "force refresh + sidecar source is skipped (matches scalar/relationship convention)",
			chapters:       chapters,
			newSource:      sidecarSource,
			existingSource: &m4bSource,
			forceRefresh:   true,
			want:           false,
		},
		{
			name:           "force refresh + plugin source applies",
			chapters:       chapters,
			newSource:      pluginSource,
			existingSource: &manualSource,
			forceRefresh:   true,
			want:           true,
		},
		{
			name:           "no force refresh: higher priority sidecar overrides file metadata",
			chapters:       chapters,
			newSource:      sidecarSource,
			existingSource: &m4bSource,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "no force refresh: lower priority file metadata cannot override manual",
			chapters:       chapters,
			newSource:      m4bSource,
			existingSource: &manualSource,
			forceRefresh:   false,
			want:           false,
		},
		{
			name:           "no force refresh: equal priority sources can replace (≤)",
			chapters:       chapters,
			newSource:      m4bSource,
			existingSource: &m4bSource,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "no force refresh: nil existing source treated as filepath (lowest)",
			chapters:       chapters,
			newSource:      m4bSource,
			existingSource: nil,
			forceRefresh:   false,
			want:           true,
		},
		{
			name:           "no force refresh: filepath source cannot replace filepath (equal allowed)",
			chapters:       chapters,
			newSource:      filepathSource,
			existingSource: &filepathSource,
			forceRefresh:   false,
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ShouldUpdateChapters(tt.chapters, tt.newSource, tt.existingSource, tt.forceRefresh)
			assert.Equal(t, tt.want, got)
		})
	}
}
