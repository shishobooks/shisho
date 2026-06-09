package roles

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	// Shared-cache in-memory DSN so every pooled connection sees the same DB
	// (a bare ":memory:" gives each connection its own empty database).
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	sqldb, err := sql.Open(sqliteshim.ShimName, dsn)
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() { db.Close() })

	return db
}

func newRolesEcho(t *testing.T) *echo.Echo {
	t.Helper()

	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	return e
}

func TestHandlerList_ResponseUsesItemsKey(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	h := &handler{roleService: NewService(db)}

	e := newRolesEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/roles", nil)
	rr := httptest.NewRecorder()
	c := e.NewContext(req, rr)

	require.NoError(t, h.list(c))
	assert.Equal(t, http.StatusOK, rr.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &raw))

	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	_, hasRoles := raw["roles"]
	assert.True(t, hasItems, "list response must use 'items' key")
	assert.True(t, hasTotal, "list response must have 'total' key")
	assert.False(t, hasRoles, "list response must NOT use 'roles' key")
	assert.Len(t, raw, 2, "list response must have exactly 'items' and 'total' keys")
}

func TestHandlerDelete_Returns204NoContent(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	h := &handler{roleService: NewService(db)}
	ctx := context.Background()

	role, err := h.roleService.Create(ctx, "deletable", []PermissionInput{})
	require.NoError(t, err)

	e := newRolesEcho(t)
	req := httptest.NewRequest(http.MethodDelete, "/roles/"+strconv.Itoa(role.ID), nil)
	rr := httptest.NewRecorder()
	c := e.NewContext(req, rr)
	c.SetPath("/roles/:id")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(role.ID))

	require.NoError(t, h.delete(c))
	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Empty(t, rr.Body.String(), "204 response must have an empty body")

	_, err = h.roleService.Retrieve(ctx, role.ID)
	require.Error(t, err, "role should be deleted")
}
