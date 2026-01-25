package plugins

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseManifest_Valid(t *testing.T) {
	manifest := map[string]interface{}{
		"manifestVersion":  1,
		"id":               "enricher-goodreads",
		"name":             "Goodreads Enricher",
		"version":          "1.0.0",
		"description":      "Enriches book metadata from Goodreads",
		"author":           "Shisho",
		"homepage":         "https://github.com/shishobooks/plugin-goodreads",
		"license":          "MIT",
		"minShishoVersion": "0.5.0",
		"capabilities": map[string]interface{}{
			"metadataEnricher": map[string]interface{}{
				"description": "Fetches ratings and reviews from Goodreads",
				"fileTypes":   []string{"epub", "pdf"},
			},
			"httpAccess": map[string]interface{}{
				"description": "Access Goodreads API",
				"domains":     []string{"api.goodreads.com", "goodreads.com"},
			},
		},
		"configSchema": map[string]interface{}{
			"apiKey": map[string]interface{}{
				"type":        "string",
				"label":       "API Key",
				"description": "Your Goodreads API key",
				"required":    true,
				"secret":      true,
			},
		},
	}

	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	m, err := ParseManifest(data)
	require.NoError(t, err)

	assert.Equal(t, 1, m.ManifestVersion)
	assert.Equal(t, "enricher-goodreads", m.ID)
	assert.Equal(t, "Goodreads Enricher", m.Name)
	assert.Equal(t, "1.0.0", m.Version)
	assert.Equal(t, "Enriches book metadata from Goodreads", m.Description)
	assert.Equal(t, "Shisho", m.Author)
	assert.Equal(t, "https://github.com/shishobooks/plugin-goodreads", m.Homepage)
	assert.Equal(t, "MIT", m.License)
	assert.Equal(t, "0.5.0", m.MinShishoVersion)

	require.NotNil(t, m.Capabilities.MetadataEnricher)
	assert.Equal(t, "Fetches ratings and reviews from Goodreads", m.Capabilities.MetadataEnricher.Description)
	assert.Equal(t, []string{"epub", "pdf"}, m.Capabilities.MetadataEnricher.FileTypes)

	require.NotNil(t, m.Capabilities.HTTPAccess)
	assert.Equal(t, "Access Goodreads API", m.Capabilities.HTTPAccess.Description)
	assert.Equal(t, []string{"api.goodreads.com", "goodreads.com"}, m.Capabilities.HTTPAccess.Domains)

	require.NotNil(t, m.ConfigSchema)
	apiKey, ok := m.ConfigSchema["apiKey"]
	require.True(t, ok)
	assert.Equal(t, "string", apiKey.Type)
	assert.Equal(t, "API Key", apiKey.Label)
	assert.Equal(t, "Your Goodreads API key", apiKey.Description)
	assert.True(t, apiKey.Required)
	assert.True(t, apiKey.Secret)
}

func TestParseManifest_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name        string
		manifest    map[string]interface{}
		expectedErr string
	}{
		{
			name: "missing manifestVersion",
			manifest: map[string]interface{}{
				"id":      "test-plugin",
				"name":    "Test Plugin",
				"version": "1.0.0",
			},
			expectedErr: "manifestVersion is required",
		},
		{
			name: "missing id",
			manifest: map[string]interface{}{
				"manifestVersion": 1,
				"name":            "Test Plugin",
				"version":         "1.0.0",
			},
			expectedErr: "id is required",
		},
		{
			name: "missing name",
			manifest: map[string]interface{}{
				"manifestVersion": 1,
				"id":              "test-plugin",
				"version":         "1.0.0",
			},
			expectedErr: "name is required",
		},
		{
			name: "missing version",
			manifest: map[string]interface{}{
				"manifestVersion": 1,
				"id":              "test-plugin",
				"name":            "Test Plugin",
			},
			expectedErr: "version is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.manifest)
			require.NoError(t, err)

			_, err = ParseManifest(data)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestParseManifest_UnsupportedManifestVersion(t *testing.T) {
	manifest := map[string]interface{}{
		"manifestVersion": 99,
		"id":              "test-plugin",
		"name":            "Test Plugin",
		"version":         "1.0.0",
	}

	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	_, err = ParseManifest(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported manifestVersion 99")
}

func TestParseManifest_InputConverter(t *testing.T) {
	manifest := map[string]interface{}{
		"manifestVersion": 1,
		"id":              "converter-cbz-to-epub",
		"name":            "CBZ to EPUB Converter",
		"version":         "1.0.0",
		"capabilities": map[string]interface{}{
			"inputConverter": map[string]interface{}{
				"description": "Converts CBZ files to EPUB",
				"sourceTypes": []string{"cbz", "cbr"},
				"mimeTypes":   []string{"application/x-cbz", "application/x-cbr"},
				"targetType":  "epub",
			},
		},
	}

	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	m, err := ParseManifest(data)
	require.NoError(t, err)

	require.NotNil(t, m.Capabilities.InputConverter)
	assert.Equal(t, "Converts CBZ files to EPUB", m.Capabilities.InputConverter.Description)
	assert.Equal(t, []string{"cbz", "cbr"}, m.Capabilities.InputConverter.SourceTypes)
	assert.Equal(t, []string{"application/x-cbz", "application/x-cbr"}, m.Capabilities.InputConverter.MIMETypes)
	assert.Equal(t, "epub", m.Capabilities.InputConverter.TargetType)
}

func TestParseManifest_FileParser(t *testing.T) {
	manifest := map[string]interface{}{
		"manifestVersion": 1,
		"id":              "parser-mobi",
		"name":            "MOBI Parser",
		"version":         "1.0.0",
		"capabilities": map[string]interface{}{
			"fileParser": map[string]interface{}{
				"description": "Parses MOBI and AZW3 files",
				"types":       []string{"mobi", "azw3"},
				"mimeTypes":   []string{"application/x-mobipocket-ebook", "application/x-mobi8-ebook"},
			},
		},
	}

	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	m, err := ParseManifest(data)
	require.NoError(t, err)

	require.NotNil(t, m.Capabilities.FileParser)
	assert.Equal(t, "Parses MOBI and AZW3 files", m.Capabilities.FileParser.Description)
	assert.Equal(t, []string{"mobi", "azw3"}, m.Capabilities.FileParser.Types)
	assert.Equal(t, []string{"application/x-mobipocket-ebook", "application/x-mobi8-ebook"}, m.Capabilities.FileParser.MIMETypes)
}

func TestParseManifest_IdentifierTypes(t *testing.T) {
	manifest := map[string]interface{}{
		"manifestVersion": 1,
		"id":              "identifier-goodreads",
		"name":            "Goodreads Identifiers",
		"version":         "1.0.0",
		"capabilities": map[string]interface{}{
			"identifierTypes": []map[string]interface{}{
				{
					"id":          "goodreads",
					"name":        "Goodreads ID",
					"urlTemplate": "https://www.goodreads.com/book/show/{value}",
					"pattern":     "^[0-9]+$",
				},
				{
					"id":          "librarything",
					"name":        "LibraryThing ID",
					"urlTemplate": "https://www.librarything.com/work/{value}",
					"pattern":     "^[0-9]+$",
				},
			},
		},
	}

	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	m, err := ParseManifest(data)
	require.NoError(t, err)

	require.Len(t, m.Capabilities.IdentifierTypes, 2)

	assert.Equal(t, "goodreads", m.Capabilities.IdentifierTypes[0].ID)
	assert.Equal(t, "Goodreads ID", m.Capabilities.IdentifierTypes[0].Name)
	assert.Equal(t, "https://www.goodreads.com/book/show/{value}", m.Capabilities.IdentifierTypes[0].URLTemplate)
	assert.Equal(t, "^[0-9]+$", m.Capabilities.IdentifierTypes[0].Pattern)

	assert.Equal(t, "librarything", m.Capabilities.IdentifierTypes[1].ID)
	assert.Equal(t, "LibraryThing ID", m.Capabilities.IdentifierTypes[1].Name)
	assert.Equal(t, "https://www.librarything.com/work/{value}", m.Capabilities.IdentifierTypes[1].URLTemplate)
	assert.Equal(t, "^[0-9]+$", m.Capabilities.IdentifierTypes[1].Pattern)
}

func TestParseManifest_FileParserReservedExtensions(t *testing.T) {
	manifest := map[string]interface{}{
		"manifestVersion": 1,
		"id":              "parser-custom-epub",
		"name":            "Custom EPUB Parser",
		"version":         "1.0.0",
		"capabilities": map[string]interface{}{
			"fileParser": map[string]interface{}{
				"description": "Custom parser for EPUB files",
				"types":       []string{"epub"},
				"mimeTypes":   []string{"application/epub+zip"},
			},
		},
	}

	data, err := json.Marshal(manifest)
	require.NoError(t, err)

	// Parsing should succeed even with reserved extensions like "epub".
	// Reserved extension checking happens at load time, not parse time.
	m, err := ParseManifest(data)
	require.NoError(t, err)

	require.NotNil(t, m.Capabilities.FileParser)
	assert.Equal(t, []string{"epub"}, m.Capabilities.FileParser.Types)
	assert.Equal(t, []string{"application/epub+zip"}, m.Capabilities.FileParser.MIMETypes)
}
