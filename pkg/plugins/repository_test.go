package plugins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchRepository_Success(t *testing.T) {
	manifest := RepositoryManifest{
		RepositoryVersion: 1,
		Scope:             "shisho",
		Name:              "Official Shisho Plugins",
		Plugins: []AvailablePlugin{
			{
				ID:          "goodreads-metadata",
				Name:        "Goodreads Metadata",
				Description: "Fetches book metadata from Goodreads",
				Author:      "Shisho Team",
				Homepage:    "https://example.com",
				Versions: []PluginVersion{
					{
						Version:          "1.2.0",
						MinShishoVersion: "1.1.0",
						ManifestVersion:  1,
						ReleaseDate:      "2025-01-15",
						Changelog:        "Added series detection",
						DownloadURL:      "https://github.com/shishobooks/plugins/releases/download/goodreads-1.2.0/goodreads-metadata.zip",
						SHA256:           "abc123",
					},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	// Override allowed hosts to allow test server
	origHosts := AllowedFetchHosts
	AllowedFetchHosts = []string{server.URL}
	defer func() { AllowedFetchHosts = origHosts }()

	result, err := FetchRepository(server.URL + "/plugins.json")
	require.NoError(t, err)
	assert.Equal(t, 1, result.RepositoryVersion)
	assert.Equal(t, "shisho", result.Scope)
	assert.Equal(t, "Official Shisho Plugins", result.Name)
	require.Len(t, result.Plugins, 1)
	assert.Equal(t, "goodreads-metadata", result.Plugins[0].ID)
	assert.Equal(t, "Goodreads Metadata", result.Plugins[0].Name)
	require.Len(t, result.Plugins[0].Versions, 1)
	assert.Equal(t, "1.2.0", result.Plugins[0].Versions[0].Version)
	assert.Equal(t, "abc123", result.Plugins[0].Versions[0].SHA256)
}

func TestFetchRepository_InvalidURL(t *testing.T) {
	// Default AllowedFetchHosts only allows raw.githubusercontent.com
	origHosts := AllowedFetchHosts
	AllowedFetchHosts = []string{"https://raw.githubusercontent.com/"}
	defer func() { AllowedFetchHosts = origHosts }()

	_, err := FetchRepository("https://evil.com/plugins.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid repository URL")
}

func TestFetchRepository_ParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json{{{"))
	}))
	defer server.Close()

	origHosts := AllowedFetchHosts
	AllowedFetchHosts = []string{server.URL}
	defer func() { AllowedFetchHosts = origHosts }()

	_, err := FetchRepository(server.URL + "/plugins.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse repository manifest JSON")
}

func TestFetchRepository_UnsupportedVersion(t *testing.T) {
	manifest := RepositoryManifest{
		RepositoryVersion: 99,
		Scope:             "shisho",
		Name:              "Future Plugins",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	origHosts := AllowedFetchHosts
	AllowedFetchHosts = []string{server.URL}
	defer func() { AllowedFetchHosts = origHosts }()

	_, err := FetchRepository(server.URL + "/plugins.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported repositoryVersion 99")
}

func TestFetchRepository_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	origHosts := AllowedFetchHosts
	AllowedFetchHosts = []string{server.URL}
	defer func() { AllowedFetchHosts = origHosts }()

	_, err := FetchRepository(server.URL + "/plugins.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestFilterCompatibleVersions(t *testing.T) {
	versions := []PluginVersion{
		{Version: "1.0.0", ManifestVersion: 1},
		{Version: "2.0.0", ManifestVersion: 2},
		{Version: "1.1.0", ManifestVersion: 1},
		{Version: "3.0.0", ManifestVersion: 3},
	}

	compatible := FilterCompatibleVersions(versions)
	require.Len(t, compatible, 2)
	assert.Equal(t, "1.0.0", compatible[0].Version)
	assert.Equal(t, "1.1.0", compatible[1].Version)
}

func TestFilterCompatibleVersions_NoMatch(t *testing.T) {
	versions := []PluginVersion{
		{Version: "2.0.0", ManifestVersion: 2},
		{Version: "3.0.0", ManifestVersion: 3},
	}

	compatible := FilterCompatibleVersions(versions)
	assert.Empty(t, compatible)
}

func TestFilterCompatibleVersions_Empty(t *testing.T) {
	compatible := FilterCompatibleVersions(nil)
	assert.Empty(t, compatible)
}
