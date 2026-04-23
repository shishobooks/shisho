package plugins

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdate_EnableLoadFailure_ReturnsError confirms that toggling a plugin's
// enable flag to true when the plugin cannot be loaded returns a 422 error and
// still persists the Malfunctioned status + load_error to the database so the
// UI can render it after a refetch.
func TestUpdate_EnableLoadFailure_ReturnsError(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	pluginDir := t.TempDir()

	// Plugin with a manifest declaring an enricher field that doesn't exist.
	// LoadPlugin will reject this with an "invalid metadata field" error.
	scope := "test"
	id := "broken-enricher"
	destDir := filepath.Join(pluginDir, scope, id)
	require.NoError(t, os.MkdirAll(destDir, 0755))
	manifest := `{
  "manifestVersion": 1,
  "id": "broken-enricher",
  "name": "Broken Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "fileTypes": ["epub"],
      "fields": ["nonsenseField"]
    }
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifest), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "main.js"),
		[]byte(`var plugin=(function(){return{metadataEnricher:{search:function(){return{results:[]}}}};})();`), 0644))

	ctx := context.Background()
	// Pre-install as Disabled so the PATCH path exercises the enable branch.
	require.NoError(t, svc.InstallPlugin(ctx, &models.Plugin{
		Scope:       scope,
		ID:          id,
		Name:        "Broken Enricher",
		Version:     "1.0.0",
		Status:      models.PluginStatusDisabled,
		InstalledAt: time.Now(),
	}))

	mgr := NewManager(svc, pluginDir, "")
	h := NewHandler(svc, mgr, nil)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"enabled": true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues(scope, id)

	err := h.update(c)
	// The handler must surface a 422 error via the errcodes pipeline.
	require.Error(t, err)
	var ec *errcodes.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, http.StatusUnprocessableEntity, ec.HTTPCode)
	assert.Contains(t, ec.Message, "nonsenseField")

	// And the plugin row must be persisted as Malfunctioned with the load_error.
	retrieved, err := svc.RetrievePlugin(ctx, scope, id)
	require.NoError(t, err)
	assert.Equal(t, models.PluginStatusMalfunctioned, retrieved.Status)
	require.NotNil(t, retrieved.LoadError)
	assert.Contains(t, *retrieved.LoadError, "nonsenseField")
}
