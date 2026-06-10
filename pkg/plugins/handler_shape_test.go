package plugins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Wire-shape regression tests for the heavily-consumed plugin responses
// (ADR 0004 safety net, TestList_ResponseUsesItemsKey style). These pin the
// exact JSON keys the frontend consumes so the types.go consolidation (and any
// future change) cannot silently alter the wire format. Note the intentional
// camelCase keys (declaredFields, fieldSettings) — the manifest passthrough
// exemption documented in ADR 0004.

// installShapeEnricher writes an enricher plugin to disk, records it in the
// DB, and loads it into a fresh manager.
func installShapeEnricher(t *testing.T, service *Service) *Manager {
	t.Helper()

	pluginDir := t.TempDir()
	scope, id := "test", "shape-enricher"
	destDir := filepath.Join(pluginDir, scope, id)
	require.NoError(t, os.MkdirAll(destDir, 0o755))

	manifestJSON := `{
  "manifestVersion": 1,
  "id": "shape-enricher",
  "name": "Shape Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Enriches metadata",
      "fileTypes": ["epub"],
      "fields": ["title", "description", "genres", "cover"]
    }
  },
  "configSchema": {
    "apiKey": {"type": "string", "label": "API Key", "required": true, "secret": true}
  }
}`
	mainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      search: function(ctx) {
        return {
          results: [{
            title: "Found: " + ctx.query,
            description: "A description",
            genres: ["Fantasy"],
            coverUrl: "https://example.com/cover.jpg",
            confidence: 0.9
          }]
        };
      }
    }
  };
})();`
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifestJSON), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJS), 0o644))

	plugin := &models.Plugin{
		Scope:       scope,
		ID:          id,
		Name:        "Shape Enricher",
		Version:     "1.0.0",
		Status:      models.PluginStatusActive,
		InstalledAt: time.Now(),
	}
	require.NoError(t, service.InstallPlugin(t.Context(), plugin))

	mgr := NewManager(service, pluginDir, "")
	require.NoError(t, mgr.LoadPlugin(t.Context(), scope, id))
	return mgr
}

func sortedJSONKeys(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestSearchMetadata_ResponseWireShape pins the exact wire shape of
// POST /plugins/search, the heaviest-consumed plugin response (Identify flow).
func TestSearchMetadata_ResponseWireShape(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	service := NewService(db)
	mgr := installShapeEnricher(t, service)

	book, _ := newApplyTestBookWithFile(t, "Some Title", "epub")
	h := &handler{
		service: service,
		manager: mgr,
		enrich: &enrichDeps{
			bookStore: &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"query":"dune","book_id":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", &models.User{
		ID:            1,
		LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}},
	})

	require.NoError(t, h.searchMetadata(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Top-level envelope: results + total_plugins only (errors and
	// skipped_plugins are omitempty and absent on the happy path).
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &top))
	assert.Equal(t, []string{"results", "total_plugins"}, sortedJSONKeys(t, rec.Body.Bytes()))

	var results []json.RawMessage
	require.NoError(t, json.Unmarshal(top["results"], &results))
	require.Len(t, results, 1)

	// Each result is ParsedMetadata flattened with server-added plugin_scope
	// and plugin_id. disabled_fields is omitempty and absent when no fields
	// are disabled. Keys are snake_case (this is NOT manifest passthrough).
	assert.Equal(t, []string{
		"authors",
		"bitrate_bps",
		"chapters",
		"confidence",
		"cover_mime_type",
		"cover_url",
		"description",
		"duration",
		"genres",
		"identifiers",
		"narrators",
		"plugin_id",
		"plugin_scope",
		"publisher",
		"series",
		"subtitle",
		"tags",
		"title",
		"url",
	}, sortedJSONKeys(t, results[0]))

	var result struct {
		Title       string   `json:"title"`
		Genres      []string `json:"genres"`
		PluginScope string   `json:"plugin_scope"`
		PluginID    string   `json:"plugin_id"`
		Confidence  float64  `json:"confidence"`
	}
	require.NoError(t, json.Unmarshal(results[0], &result))
	assert.Equal(t, "Found: dune", result.Title)
	assert.Equal(t, []string{"Fantasy"}, result.Genres)
	assert.Equal(t, "test", result.PluginScope)
	assert.Equal(t, "shape-enricher", result.PluginID)
	assert.InDelta(t, 0.9, result.Confidence, 0.0001)
}

// TestGetConfig_ResponseWireShape pins the exact wire shape of
// GET /plugins/installed/:scope/:id/config, including the intentionally
// camelCase declaredFields / fieldSettings keys and the nested config-field
// keys (manifest passthrough, ADR 0004 exemption).
func TestGetConfig_ResponseWireShape(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	service := NewService(db)
	mgr := installShapeEnricher(t, service)
	h := &handler{service: service, manager: mgr}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues("test", "shape-enricher")

	require.NoError(t, h.getConfig(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, []string{
		"confidence_threshold",
		"declaredFields",
		"fieldSettings",
		"schema",
		"values",
	}, sortedJSONKeys(t, rec.Body.Bytes()))

	var body struct {
		Schema         map[string]json.RawMessage `json:"schema"`
		DeclaredFields []string                   `json:"declaredFields"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, []string{"title", "description", "genres", "cover"}, body.DeclaredFields)
	require.Contains(t, body.Schema, "apiKey")

	// ConfigField wire keys are camelCase-free but include every field of the
	// parsed manifest representation (no omitempty on the Go struct).
	assert.Equal(t, []string{
		"default",
		"description",
		"label",
		"max",
		"min",
		"options",
		"required",
		"secret",
		"type",
	}, sortedJSONKeys(t, body.Schema["apiKey"]))
}
