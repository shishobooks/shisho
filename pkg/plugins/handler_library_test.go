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
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibraryPluginOrder_GetDefault(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	ctx := t.Context()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup: install plugin and set global order
	plugin := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Test Enricher", Version: "1.0.0", Status: models.PluginStatusActive}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)
	err = svc.SetOrder(ctx, "metadataEnricher", []models.PluginHookConfig{
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

	var resp LibraryPluginOrderResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.Customized)
	require.Len(t, resp.Plugins, 1)
	assert.Equal(t, "enricher1", resp.Plugins[0].ID)
	assert.Equal(t, "Test Enricher", resp.Plugins[0].Name)
	assert.Equal(t, models.PluginModeEnabled, resp.Plugins[0].Mode)
}

func TestLibraryPluginOrder_SetAndGet(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	ctx := t.Context()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup
	p1 := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Enricher 1", Version: "1.0.0", Status: models.PluginStatusActive}
	p2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Enricher 2", Version: "1.0.0", Status: models.PluginStatusActive}
	_, err := db.NewInsert().Model(p1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(p2).Exec(ctx)
	require.NoError(t, err)

	// PUT - set custom order
	e := echo.New()
	payload := `{"plugins":[{"scope":"test","id":"enricher2","mode":"enabled"},{"scope":"test","id":"enricher1","mode":"disabled"}]}`
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

	var resp LibraryPluginOrderResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Customized)
	require.Len(t, resp.Plugins, 2)
	assert.Equal(t, "enricher2", resp.Plugins[0].ID)
	assert.Equal(t, models.PluginModeEnabled, resp.Plugins[0].Mode)
	assert.Equal(t, "enricher1", resp.Plugins[1].ID)
	assert.Equal(t, models.PluginModeDisabled, resp.Plugins[1].Mode)
}

func TestLibraryPluginOrder_ResetHookType(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	ctx := t.Context()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup
	plugin := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Test Enricher", Version: "1.0.0", Status: models.PluginStatusActive}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	// Set custom order first
	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPluginHookConfig{
		{Scope: "test", PluginID: "enricher1", Mode: models.PluginModeEnabled},
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
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	ctx := t.Context()

	library := insertTestLibrary(t, db, "Test Library")

	// Setup
	plugin := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Test Enricher", Version: "1.0.0", Status: models.PluginStatusActive}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	// Set custom orders for multiple hook types
	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPluginHookConfig{
		{Scope: "test", PluginID: "enricher1", Mode: models.PluginModeEnabled},
	})
	require.NoError(t, err)
	err = svc.SetLibraryOrder(ctx, library.ID, "fileParser", []models.LibraryPluginHookConfig{
		{Scope: "test", PluginID: "enricher1", Mode: models.PluginModeEnabled},
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

// The order routes take the hook type from the URL. An unknown one, such as
// the removed outputGenerator, is rejected instead of read or stored.
func TestPluginOrder_RejectsUnknownHookType(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	library := insertTestLibrary(t, db, "Test Library")
	plugin := &models.Plugin{Scope: "test", ID: "p", Name: "P", Version: "1.0.0", Status: models.PluginStatusActive}
	_, err := db.NewInsert().Model(plugin).Exec(t.Context())
	require.NoError(t, err)
	h := NewHandler(svc, nil, nil)

	for name, tc := range map[string]struct {
		run     func(echo.Context) error
		body    string
		library bool
	}{
		"get global order":    {run: h.getOrder},
		"set global order":    {run: h.setOrder, body: `{"order":[{"scope":"test","id":"p"}]}`},
		"get library order":   {run: h.getLibraryOrder, library: true},
		"set library order":   {run: h.setLibraryOrder, body: `{"plugins":[{"scope":"test","id":"p"}]}`, library: true},
		"reset library order": {run: h.resetLibraryOrder, library: true},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c := echo.New().NewContext(req, httptest.NewRecorder())
			if tc.library {
				c.SetParamNames("id", "hookType")
				c.SetParamValues(strconv.Itoa(library.ID), "outputGenerator")
			} else {
				c.SetParamNames("hookType")
				c.SetParamValues("outputGenerator")
			}
			assertErrcodeFields(t, tc.run(c), http.StatusUnprocessableEntity, "validation_error")
		})
	}

	for _, table := range []string{"plugin_hook_configs", "library_plugin_customizations", "library_plugin_hook_configs"} {
		var n int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n))
		assert.Zero(t, n, table)
	}
}
