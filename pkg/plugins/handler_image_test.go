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

func TestGetImage_PathTraversal(t *testing.T) {
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

			err := h.GetImage(c)
			require.Error(t, err)

			var ecErr *errcodes.Error
			require.ErrorAs(t, err, &ecErr)
			assert.Equal(t, http.StatusUnprocessableEntity, ecErr.HTTPCode)
		})
	}
}

func TestGetImage_ValidPlugin(t *testing.T) {
	pluginDir := t.TempDir()
	installer := NewInstaller(pluginDir)
	h := NewHandler(nil, nil, installer)

	// Create a valid plugin icon
	iconDir := filepath.Join(pluginDir, "shisho", "test-plugin")
	require.NoError(t, os.MkdirAll(iconDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(iconDir, "icon.png"), []byte("fake-png"), 0o644))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues("shisho", "test-plugin")

	err := h.GetImage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "fake-png", rec.Body.String())
}

func TestGetImage_NotFound(t *testing.T) {
	pluginDir := t.TempDir()
	installer := NewInstaller(pluginDir)
	h := NewHandler(nil, nil, installer)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues("shisho", "nonexistent")

	err := h.GetImage(c)
	require.Error(t, err)

	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr)
	assert.Equal(t, http.StatusNotFound, ecErr.HTTPCode)
}
