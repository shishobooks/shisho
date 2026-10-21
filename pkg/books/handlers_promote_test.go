package books

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A scan reads only a supplement's name, and records its size and mtime, so
// a supplement made a main File is scanned at once, and its stored mtime is
// cleared so that scan reads it in full instead of skipping it as unchanged.
func TestUpdateFile_UpgradeToMainScansTheFile(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	path := testgen.WriteFile(t, book.Filepath, "book.kepub", []byte("content"))
	file := setupTestFile(t, db, book, models.FileTypeKePub, path)
	modTime := time.Now()
	file.FileRole = models.FileRoleSupplement
	file.FileModifiedAt = &modTime
	_, err := db.NewUpdate().Model(file).Column("file_role", "file_modified_at").WherePK().Exec(t.Context())
	require.NoError(t, err)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))
	scanner := &recordingScanner{}

	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(`{"file_role":"main"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rr := executeRequestWithUser(t, setupTestServerWithScanner(t, db, scanner), req, user)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.True(t, scanner.called, "the promoted File is scanned")
	assert.Equal(t, ScanOptions{FileID: file.ID}, scanner.opts, "an ordinary scan, so the book's metadata keeps its priorities")
	var stored models.File
	require.NoError(t, db.NewSelect().Model(&stored).Where("id = ?", file.ID).Scan(t.Context()))
	assert.Equal(t, models.FileRoleMain, stored.FileRole)
	assert.Nil(t, stored.FileModifiedAt)
}

func TestPromoteSupplementToMain_ClearsTheStoredModTime(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	_, book := setupTestLibraryAndBook(t, db)
	path := testgen.WriteFile(t, book.Filepath, "book.kepub", []byte("content"))
	file := setupTestFile(t, db, book, models.FileTypeKePub, path)
	modTime := time.Now()
	file.FileRole = models.FileRoleSupplement
	file.FileModifiedAt = &modTime
	_, err := db.NewUpdate().Model(file).Column("file_role", "file_modified_at").WherePK().Exec(t.Context())
	require.NoError(t, err)

	require.NoError(t, NewService(db, nil).PromoteSupplementToMain(t.Context(), file.ID))

	var stored models.File
	require.NoError(t, db.NewSelect().Model(&stored).Where("id = ?", file.ID).Scan(t.Context()))
	assert.Equal(t, models.FileRoleMain, stored.FileRole)
	assert.Nil(t, stored.FileModifiedAt)
}
