package plugins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibraryPluginOrder_GetDefault(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := t.Context()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup: install plugin and set global order
	plugin := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)
	err = svc.SetOrder(ctx, "metadataEnricher", []models.PluginOrder{
		{Scope: "test", PluginID: "enricher1"},
	})
	require.NoError(t, err)

	// GET - should return global default (customized=false)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "hookType")
	c.SetParamValues(strconv.Itoa(library.ID), "metadataEnricher")

	h := NewHandler(svc, nil, nil)
	err = h.GetLibraryOrder(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp libraryOrderResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.Customized)
	require.Len(t, resp.Plugins, 1)
	assert.Equal(t, "enricher1", resp.Plugins[0].ID)
	assert.Equal(t, "Test Enricher", resp.Plugins[0].Name)
	assert.True(t, resp.Plugins[0].Enabled)
}

func TestLibraryPluginOrder_SetAndGet(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := t.Context()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup
	p1 := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Enricher 1", Version: "1.0.0", Enabled: true}
	p2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Enricher 2", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(p1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(p2).Exec(ctx)
	require.NoError(t, err)

	// PUT - set custom order
	e := echo.New()
	payload := `{"plugins":[{"scope":"test","id":"enricher2","enabled":true},{"scope":"test","id":"enricher1","enabled":false}]}`
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "hookType")
	c.SetParamValues(strconv.Itoa(library.ID), "metadataEnricher")

	h := NewHandler(svc, nil, nil)
	err = h.SetLibraryOrder(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// GET - should return customized order
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "hookType")
	c.SetParamValues(strconv.Itoa(library.ID), "metadataEnricher")

	err = h.GetLibraryOrder(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp libraryOrderResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Customized)
	require.Len(t, resp.Plugins, 2)
	assert.Equal(t, "enricher2", resp.Plugins[0].ID)
	assert.True(t, resp.Plugins[0].Enabled)
	assert.Equal(t, "enricher1", resp.Plugins[1].ID)
	assert.False(t, resp.Plugins[1].Enabled)
}

func TestLibraryPluginOrder_ResetHookType(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := t.Context()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup
	plugin := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	// Set custom order first
	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher1", Enabled: true},
	})
	require.NoError(t, err)

	// DELETE - reset hook type
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "hookType")
	c.SetParamValues(strconv.Itoa(library.ID), "metadataEnricher")

	h := NewHandler(svc, nil, nil)
	err = h.ResetLibraryOrder(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify no longer customized
	customized, err := svc.IsLibraryCustomized(ctx, library.ID, "metadataEnricher")
	require.NoError(t, err)
	assert.False(t, customized)
}

func TestLibraryPluginOrder_ResetAll(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := t.Context()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup
	plugin := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	// Set custom orders for multiple hook types
	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher1", Enabled: true},
	})
	require.NoError(t, err)
	err = svc.SetLibraryOrder(ctx, library.ID, "fileParser", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher1", Enabled: true},
	})
	require.NoError(t, err)

	// DELETE - reset all
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(library.ID))

	h := NewHandler(svc, nil, nil)
	err = h.ResetAllLibraryOrders(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify both hook types are no longer customized
	customized, err := svc.IsLibraryCustomized(ctx, library.ID, "metadataEnricher")
	require.NoError(t, err)
	assert.False(t, customized)
	customized, err = svc.IsLibraryCustomized(ctx, library.ID, "fileParser")
	require.NoError(t, err)
	assert.False(t, customized)
}
