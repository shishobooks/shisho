package testutils

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// newTestDB creates an in-memory SQLite DB with all migrations applied,
// matching the pattern used by pkg/apikeys/service_test.go.
func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// Enable foreign keys to match production behavior.
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func TestFixtureZipIsDeterministicAndMatchesInfo(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h := &handler{} // db not needed for fixture endpoints
	e.GET("/test/plugins/fixture.zip", h.fixtureZip)
	e.GET("/test/plugins/fixture-info", h.fixtureInfo)

	// Fetch the zip
	req := httptest.NewRequest(http.MethodGet, "/test/plugins/fixture.zip", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/zip", rec.Header().Get("Content-Type"))

	zipBytes, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	require.NotEmpty(t, zipBytes)

	h1 := sha256.Sum256(zipBytes)

	// Fetch it again: bytes must be identical (deterministic build).
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/test/plugins/fixture.zip", nil))
	h2 := sha256.Sum256(rec2.Body.Bytes())
	assert.Equal(t, h1, h2, "fixture.zip must be deterministic across requests")

	// Fetch info and check the sha256 matches what we got from fixture.zip.
	rec3 := httptest.NewRecorder()
	e.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/test/plugins/fixture-info", nil))
	require.Equal(t, http.StatusOK, rec3.Code)

	var info struct {
		Scope       string `json:"scope"`
		ID          string `json:"id"`
		Version     string `json:"version"`
		DownloadURL string `json:"download_url"`
		SHA256      string `json:"sha256"`
	}
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &info))

	assert.Equal(t, "test", info.Scope)
	assert.Equal(t, "fixture", info.ID)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, hex.EncodeToString(h1[:]), info.SHA256)
	assert.Contains(t, info.DownloadURL, "/test/plugins/fixture.zip")
}

func TestSeedPluginWritesDBRow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)

	tmp := t.TempDir()
	installer := plugins.NewInstaller(tmp)
	e := echo.New()
	h := &handler{db: db, manager: nil, installer: installer}
	e.POST("/test/plugins", h.seedPlugin)

	body := `{
		"scope": "test",
		"id": "fixture",
		"name": "Fixture Plugin",
		"version": "1.0.0",
		"status": 0,
		"update_available_version": "2.0.0"
	}`
	req := httptest.NewRequest(http.MethodPost, "/test/plugins", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	// DB row exists.
	count, err := db.NewSelect().Model((*models.Plugin)(nil)).
		Where("scope = ? AND id = ?", "test", "fixture").
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Files exist on disk.
	assert.FileExists(t, filepath.Join(tmp, "test", "fixture", "manifest.json"))
	assert.FileExists(t, filepath.Join(tmp, "test", "fixture", "main.js"))
}

func TestDeleteAllPluginsWipesStateAndDisk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)

	tmp := t.TempDir()
	// Seed a fake plugin dir.
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "test", "fixture"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "test", "fixture", "x.txt"), []byte("x"), 0644))

	// Seed a DB row.
	_, err := db.NewInsert().Model(&models.Plugin{
		Scope: "test", ID: "fixture", Name: "F", Version: "1.0.0",
		Status: models.PluginStatusActive, InstalledAt: time.Now(),
	}).Exec(ctx)
	require.NoError(t, err)

	installer := plugins.NewInstaller(tmp)
	e := echo.New()
	h := &handler{db: db, manager: nil, installer: installer}
	e.DELETE("/test/plugins", h.deleteAllPlugins)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/test/plugins", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	// Row gone.
	count, err := db.NewSelect().Model((*models.Plugin)(nil)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Directory gone.
	_, err = os.Stat(filepath.Join(tmp, "test"))
	assert.True(t, os.IsNotExist(err), "plugin dir should be removed")
}
