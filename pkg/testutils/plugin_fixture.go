package testutils

// Fixture plugin used by E2E tests. Kept as Go string constants (rather than
// a committed testdata directory) so the plugin source stays alongside the
// handlers that build a zip from it and seed it to disk.

const (
	fixtureScope   = "test"
	fixtureID      = "fixture"
	fixtureVersion = "1.0.0"
	fixtureName    = "Fixture Plugin"
)

const fixtureManifestJSON = `{
  "manifestVersion": 1,
  "id": "fixture",
  "name": "Fixture Plugin",
  "version": "1.0.0",
  "description": "Test-only plugin used by E2E tests. Do not ship to users.",
  "homepage": "https://example.test/fixture",
  "capabilities": {
    "metadataEnricher": {
      "description": "E2E fixture enricher",
      "fileTypes": ["epub"],
      "fields": ["title"]
    }
  },
  "configSchema": {
    "apiKey": {
      "type": "string",
      "label": "API Key",
      "description": "Not actually used by the fixture",
      "required": false,
      "secret": true
    }
  }
}
`

const fixtureMainJS = `var plugin = (function () {
  return {
    metadataEnricher: {
      search: function () {
        return { results: [] };
      }
    }
  };
})();
`

// fixtureFiles returns the fixture plugin's on-disk file layout: map keyed
// by filename (no directories, matching the flat zip layout the installer
// expects) to the raw bytes.
func fixtureFiles() map[string][]byte {
	return map[string][]byte{
		"manifest.json": []byte(fixtureManifestJSON),
		"main.js":       []byte(fixtureMainJS),
	}
}
