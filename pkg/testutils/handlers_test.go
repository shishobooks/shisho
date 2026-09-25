package testutils

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bun writes a zero time.Time instead of omitting the column, so the users
// DEFAULT CURRENT_TIMESTAMP never applies. The e2e seeding endpoint must set
// both timestamps itself so seeded users look like real ones.
func TestCreateUserSetsTimestamps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)

	e := echo.New()
	RegisterRoutes(e.Group("/api"), db, nil, nil)

	body := `{"username": "seeded", "password": "password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/test/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp createUserResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	user := &models.User{}
	require.NoError(t, db.NewSelect().Model(user).Where("id = ?", resp.ID).Scan(ctx))
	assert.WithinDuration(t, time.Now(), user.CreatedAt, time.Minute)
	assert.WithinDuration(t, time.Now(), user.UpdatedAt, time.Minute)
}
