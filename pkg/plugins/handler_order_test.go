package plugins

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	for _, tc := range []struct {
		name    string
		run     func(echo.Context) error
		body    string
		library bool
	}{
		{name: "get global order", run: h.getOrder},
		{name: "set global order", run: h.setOrder, body: `{"order":[{"scope":"test","id":"p"}]}`},
		{name: "get library order", run: h.getLibraryOrder, library: true},
		{name: "set library order", run: h.setLibraryOrder, body: `{"plugins":[{"scope":"test","id":"p"}]}`, library: true},
		{name: "reset library order", run: h.resetLibraryOrder, library: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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

			for _, table := range []string{"plugin_hook_configs", "library_plugin_customizations", "library_plugin_hook_configs"} {
				var n int
				require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n))
				assert.Zero(t, n, table)
			}
		})
	}
}

// The order routes accept exactly the hook types a plugin can provide.
func TestValidHookTypes_MatchRuntimeHookTypes(t *testing.T) {
	t.Parallel()
	all := &Runtime{inputConverter: goja.Undefined(), fileParser: goja.Undefined(), metadataEnricher: goja.Undefined()}
	valid := make([]string, 0, len(validHookTypes))
	for hookType := range validHookTypes {
		valid = append(valid, hookType)
	}
	assert.ElementsMatch(t, all.HookTypes(), valid)
}
