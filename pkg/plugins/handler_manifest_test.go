package plugins

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_GetManifest_PathTraversal(t *testing.T) {
	pluginDir := t.TempDir()
	installer := NewInstaller(pluginDir)
	h := NewHandler(nil, nil, installer)

	tests := []struct {
		name  string
		scope string
		id    string
	}{
		{"dot-dot in scope", "..", "plugin"},
		{"dot-dot in id", "scope", ".."},
		{"dot-dot path in scope", "scope/../etc", "plugin"},
		{"dot-dot path in id", "scope", "plugin/../../etc"},
		{"forward slash in scope", "scope/sub", "plugin"},
		{"forward slash in id", "scope", "plugin/sub"},
		{"backslash in scope", "scope\\sub", "plugin"},
		{"backslash in id", "scope", "plugin\\sub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("scope", "id")
			c.SetParamValues(tt.scope, tt.id)

			err := h.GetManifest(c)
			require.Error(t, err)

			var ecErr *errcodes.Error
			require.ErrorAs(t, err, &ecErr)
			assert.Equal(t, http.StatusUnprocessableEntity, ecErr.HTTPCode)
		})
	}
}

func TestHandler_GetManifest_ReturnsFileContents(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	pluginDir := t.TempDir()
	installer := NewInstaller(pluginDir)
	h := NewHandler(svc, nil, installer)

	scope, id := "test", "echo"
	pluginPath := filepath.Join(installer.PluginDir(), scope, id)
	require.NoError(t, os.MkdirAll(pluginPath, 0o755))
	manifestJSON := `{"manifestVersion":1,"id":"echo","name":"Echo","version":"1.0.0"}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginPath, "manifest.json"), []byte(manifestJSON), 0o644))
	insertTestPlugin(t, db, scope, id)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/plugins/installed/test/echo/manifest", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues(scope, id)

	require.NoError(t, h.GetManifest(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, manifestJSON, rec.Body.String())
}

func TestHandler_GetManifest_Returns404WhenPluginNotInDB(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	pluginDir := t.TempDir()
	installer := NewInstaller(pluginDir)
	h := NewHandler(svc, nil, installer)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/plugins/installed/nope/nope/manifest", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues("nope", "nope")

	err := h.GetManifest(c)
	require.Error(t, err)

	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr)
	assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
}

func TestHandler_GetManifest_Returns404WhenFileMissing(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	pluginDir := t.TempDir()
	installer := NewInstaller(pluginDir)
	h := NewHandler(svc, nil, installer)

	insertTestPlugin(t, db, "test", "ghost")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/plugins/installed/test/ghost/manifest", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues("test", "ghost")

	err := h.GetManifest(c)
	require.Error(t, err)

	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr)
	assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
}
