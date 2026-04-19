package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateReleaseDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty is allowed", "", false},
		{"RFC3339 UTC", "2026-04-14T00:00:00Z", false},
		{"RFC3339 offset", "2026-04-14T09:30:00-05:00", false},
		{"date only", "2026-04-14", false},
		{"random garbage", "not-a-date", true},
		{"partial date", "2026-04", true},
		{"wrong separator", "2026/04/14", true},
		{"empty space", "   ", true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateReleaseDate(tc.input)
			if tc.wantErr {
				assert.Error(t, err, "expected error for %q", tc.input)
			} else {
				assert.NoError(t, err, "unexpected error for %q", tc.input)
			}
		})
	}
}

func TestFilterInvalidReleaseDates(t *testing.T) {
	t.Parallel()

	manifest := &RepositoryManifest{
		Scope: "test",
		Plugins: []AvailablePlugin{{
			ID: "p",
			Versions: []PluginVersion{
				{Version: "1.0.0", ReleaseDate: "2026-04-14"},
				{Version: "1.1.0", ReleaseDate: "garbage"},
				{Version: "1.2.0", ReleaseDate: ""},
			},
		}},
	}

	filterInvalidReleaseDates(manifest)

	assert.Len(t, manifest.Plugins[0].Versions, 2)
	assert.Equal(t, "1.0.0", manifest.Plugins[0].Versions[0].Version)
	assert.Equal(t, "1.2.0", manifest.Plugins[0].Versions[1].Version)
}
