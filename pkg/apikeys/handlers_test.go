package apikeys

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func newHandlerTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func setupTestHandler(t *testing.T) (*handler, *bun.DB, *echo.Echo) {
	db := newHandlerTestDB(t)
	svc := NewService(db)
	h := newHandler(svc)
	e := echo.New()
	return h, db, e
}

func setUserInContext(c echo.Context, user *models.User) {
	c.Set("user", user)
}

func TestHandler_List(t *testing.T) {
	h, db, e := setupTestHandler(t)
	ctx := context.Background()

	// Create test user
	user := &models.User{ID: 1, Username: "testuser"}
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create an API key
	_, err = h.service.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/user/api-keys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setUserInContext(c, user)

	// Call handler
	err = h.List(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var keys []*APIKey
	err = json.Unmarshal(rec.Body.Bytes(), &keys)
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "My Kobo", keys[0].Name)
}

func TestHandler_List_Unauthenticated(t *testing.T) {
	h, _, e := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/user/api-keys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	// Don't set user in context

	err := h.List(c)
	assert.Error(t, err)
}

func TestHandler_Create(t *testing.T) {
	h, db, e := setupTestHandler(t)
	ctx := context.Background()

	// Create test user
	user := &models.User{ID: 1, Username: "testuser"}
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create request
	body := `{"name": "My Kindle"}`
	req := httptest.NewRequest(http.MethodPost, "/user/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setUserInContext(c, user)

	// Call handler
	err = h.Create(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var apiKey APIKey
	err = json.Unmarshal(rec.Body.Bytes(), &apiKey)
	require.NoError(t, err)
	assert.Equal(t, "My Kindle", apiKey.Name)
	assert.NotEmpty(t, apiKey.Key)
}

func TestHandler_Create_EmptyName(t *testing.T) {
	h, db, e := setupTestHandler(t)
	ctx := context.Background()

	// Create test user
	user := &models.User{ID: 1, Username: "testuser"}
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create request with empty name
	body := `{"name": ""}`
	req := httptest.NewRequest(http.MethodPost, "/user/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setUserInContext(c, user)

	// Call handler
	err = h.Create(c)
	assert.Error(t, err)
}

func TestHandler_UpdateName(t *testing.T) {
	h, db, e := setupTestHandler(t)
	ctx := context.Background()

	// Create test user
	user := &models.User{ID: 1, Username: "testuser"}
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create an API key
	created, err := h.service.Create(ctx, 1, "Old Name")
	require.NoError(t, err)

	// Create request
	body := `{"name": "New Name"}`
	req := httptest.NewRequest(http.MethodPatch, "/user/api-keys/"+created.ID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/user/api-keys/:id")
	c.SetParamNames("id")
	c.SetParamValues(created.ID)
	setUserInContext(c, user)

	// Call handler
	err = h.UpdateName(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var apiKey APIKey
	err = json.Unmarshal(rec.Body.Bytes(), &apiKey)
	require.NoError(t, err)
	assert.Equal(t, "New Name", apiKey.Name)
}

func TestHandler_Delete(t *testing.T) {
	h, db, e := setupTestHandler(t)
	ctx := context.Background()

	// Create test user
	user := &models.User{ID: 1, Username: "testuser"}
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create an API key
	created, err := h.service.Create(ctx, 1, "To Delete")
	require.NoError(t, err)

	// Create request
	req := httptest.NewRequest(http.MethodDelete, "/user/api-keys/"+created.ID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/user/api-keys/:id")
	c.SetParamNames("id")
	c.SetParamValues(created.ID)
	setUserInContext(c, user)

	// Call handler
	err = h.Delete(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify deleted
	keys, err := h.service.List(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func TestHandler_AddPermission(t *testing.T) {
	h, db, e := setupTestHandler(t)
	ctx := context.Background()

	// Create test user
	user := &models.User{ID: 1, Username: "testuser"}
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create an API key
	created, err := h.service.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/user/api-keys/"+created.ID+"/permissions/"+PermissionEReaderBrowser, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/user/api-keys/:id/permissions/:permission")
	c.SetParamNames("id", "permission")
	c.SetParamValues(created.ID, PermissionEReaderBrowser)
	setUserInContext(c, user)

	// Call handler
	err = h.AddPermission(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var apiKey APIKey
	err = json.Unmarshal(rec.Body.Bytes(), &apiKey)
	require.NoError(t, err)
	assert.True(t, apiKey.HasPermission(PermissionEReaderBrowser))
}

func TestHandler_RemovePermission(t *testing.T) {
	h, db, e := setupTestHandler(t)
	ctx := context.Background()

	// Create test user
	user := &models.User{ID: 1, Username: "testuser"}
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create an API key with permission
	created, err := h.service.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)
	created, err = h.service.AddPermission(ctx, 1, created.ID, PermissionEReaderBrowser)
	require.NoError(t, err)

	// Create request
	req := httptest.NewRequest(http.MethodDelete, "/user/api-keys/"+created.ID+"/permissions/"+PermissionEReaderBrowser, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/user/api-keys/:id/permissions/:permission")
	c.SetParamNames("id", "permission")
	c.SetParamValues(created.ID, PermissionEReaderBrowser)
	setUserInContext(c, user)

	// Call handler
	err = h.RemovePermission(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var apiKey APIKey
	err = json.Unmarshal(rec.Body.Bytes(), &apiKey)
	require.NoError(t, err)
	assert.False(t, apiKey.HasPermission(PermissionEReaderBrowser))
}

func TestHandler_GenerateShortURL(t *testing.T) {
	h, db, e := setupTestHandler(t)
	ctx := context.Background()

	// Create test user
	user := &models.User{ID: 1, Username: "testuser"}
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create an API key
	created, err := h.service.Create(ctx, 1, "My Kobo")
	require.NoError(t, err)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/user/api-keys/"+created.ID+"/short-url", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/user/api-keys/:id/short-url")
	c.SetParamNames("id")
	c.SetParamValues(created.ID)
	setUserInContext(c, user)

	// Call handler
	err = h.GenerateShortURL(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var shortURL APIKeyShortURL
	err = json.Unmarshal(rec.Body.Bytes(), &shortURL)
	require.NoError(t, err)
	assert.Equal(t, created.ID, shortURL.APIKeyID)
	assert.Len(t, shortURL.ShortCode, 6)
}
