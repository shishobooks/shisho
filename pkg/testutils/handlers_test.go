package testutils

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bun writes a zero time.Time instead of omitting the column, so the users
// DEFAULT CURRENT_TIMESTAMP never applies. The e2e seeding endpoint must set
// both timestamps itself so seeded users look like real ones.
func TestCreateUserSetsTimestamps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)

	e := echo.New()
	RegisterRoutes(e.Group("/api"), db, nil, nil, "")

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

// Each browser's E2E server gets its own file directory, so one browser's
// wipe cannot delete a file another browser's test is downloading.
func TestCreateBookWithFileOnDiskUsesTheServersOwnDirectory(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ownRoot := filepath.Join(t.TempDir(), "own")
	otherRoot := filepath.Join(t.TempDir(), "other")
	require.NoError(t, os.MkdirAll(otherRoot, 0o755))
	otherFile := filepath.Join(otherRoot, "keep.epub")
	require.NoError(t, os.WriteFile(otherFile, []byte("x"), 0o600))

	e := echo.New()
	RegisterRoutes(e.Group("/api"), db, nil, nil, ownRoot)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	rec := do(http.MethodPost, "/api/test/libraries", `{"name":"Lib"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var lib createLibraryResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lib))

	rec = do(http.MethodPost, "/api/test/books", `{"libraryId":`+strconv.Itoa(lib.ID)+`,"title":"A/B","withFileOnDisk":true}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var book createBookResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &book))

	file := &models.File{}
	require.NoError(t, db.NewSelect().Model(file).Where("id = ?", book.FileID).Scan(context.Background()))
	assert.True(t, strings.HasPrefix(file.Filepath, ownRoot+string(filepath.Separator)), file.Filepath)
	assert.Equal(t, "A_B.epub", filepath.Base(file.Filepath), "a slash in the title stays in the file name")
	_, err := os.Stat(file.Filepath)
	require.NoError(t, err)

	rec = do(http.MethodDelete, "/api/test/ereader", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, err = os.Stat(ownRoot)
	assert.True(t, os.IsNotExist(err), "the wipe removes this server's files")
	_, err = os.Stat(otherFile)
	assert.NoError(t, err, "the wipe leaves another server's files alone")
}

// The MOBI reader e2e test needs a real MOBI behind the seeded file.
func TestCreateBookWithFileOnDiskWritesAMOBI(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)

	e := echo.New()
	RegisterRoutes(e.Group("/api"), db, nil, nil, t.TempDir())

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	rec := do(http.MethodPost, "/api/test/libraries", `{"name":"Lib"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var lib createLibraryResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lib))

	rec = do(http.MethodPost, "/api/test/books", `{"libraryId":`+strconv.Itoa(lib.ID)+`,"title":"Kindle Book","fileType":"mobi","withFileOnDisk":true}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var book createBookResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &book))

	file := &models.File{}
	require.NoError(t, db.NewSelect().Model(file).Where("id = ?", book.FileID).Scan(context.Background()))
	assert.Equal(t, "Kindle Book.mobi", filepath.Base(file.Filepath))
	parsed, err := mobi.Parse(file.Filepath)
	require.NoError(t, err)
	assert.Equal(t, "Kindle Book", parsed.Title)
}
