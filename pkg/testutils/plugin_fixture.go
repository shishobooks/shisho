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
      "fields": ["title", "abridged"]
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

// The enricher proposes one fixed result so Identify flows can be driven in
// a browser. Abridged is proposed as false on purpose: Identify must be able
// to apply an explicit false without turning it into a clear.
const fixtureMainJS = `var plugin = (function () {
  return {
    metadataEnricher: {
      search: function () {
        return { results: [{ title: "Fixture Title", abridged: false }] };
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
