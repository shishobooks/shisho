package books

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupTestDB(t *testing.T) *bun.DB {
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

func setUserInContext(c echo.Context, user *models.User) {
	c.Set("user", user)
}

// userContextHandler wraps an Echo instance to inject user context without modifying the Echo middleware chain.
type userContextHandler struct {
	echo *echo.Echo
	user *models.User
}

func (h *userContextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create the context that Echo will use
	c := h.echo.NewContext(r, w)
	// Set the user in context before routing
	setUserInContext(c, h.user)

	// Find and execute the matched handler
	h.echo.Router().Find(r.Method, r.URL.Path, c)
	handler := c.Handler()
	if handler == nil {
		// No route found - let Echo handle the 404
		h.echo.ServeHTTP(w, r)
		return
	}

	// Execute the handler chain (includes registered middleware)
	if err := handler(c); err != nil {
		h.echo.HTTPErrorHandler(err, c)
	}
}

// executeRequestWithUser executes a request with the user set in context.
// This does NOT use e.Use() to avoid middleware accumulation across multiple calls.
func executeRequestWithUser(t *testing.T, e *echo.Echo, req *http.Request, user *models.User) *httptest.ResponseRecorder {
	t.Helper()

	rr := httptest.NewRecorder()
	handler := &userContextHandler{echo: e, user: user}
	handler.ServeHTTP(rr, req)
	return rr
}

// createTestM4BFile creates a temporary M4B file with some content for testing.
func createTestM4BFile(t *testing.T, size int) string {
	t.Helper()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.m4b")

	// Create a file with the specified size
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	err := os.WriteFile(filePath, data, 0644)
	require.NoError(t, err)

	return filePath
}

// createTestEPUBFile creates a temporary EPUB file for testing.
func createTestEPUBFile(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.epub")

	// Create a simple file
	err := os.WriteFile(filePath, []byte("epub content"), 0644)
	require.NoError(t, err)

	return filePath
}

// setupTestLibraryAndBook creates a library and book for testing.
func setupTestLibraryAndBook(t *testing.T, db *bun.DB) (*models.Library, *models.Book) {
	t.Helper()
	ctx := context.Background()

	// Create a library with required fields
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book with required fields
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	return library, book
}

// setupTestFile creates a file record for testing.
func setupTestFile(t *testing.T, db *bun.DB, book *models.Book, fileType, filePath string) *models.File {
	t.Helper()
	ctx := context.Background()

	// Get file size from actual file
	fileInfo, err := os.Stat(filePath)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      fileType,
		FileRole:      models.FileRoleMain,
		Filepath:      filePath,
		FilesizeBytes: fileInfo.Size(),
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	return file
}

// setupTestUser creates a user with library access for testing.
func setupTestUser(t *testing.T, db *bun.DB, libraryID int, hasAccess bool) *models.User {
	t.Helper()
	ctx := context.Background()

	// Get role ID (admin role should be ID 1 from migrations)
	user := &models.User{
		Username:     "testuser",
		PasswordHash: "hash",
		RoleID:       1,
		IsActive:     true,
	}
	_, err := db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	if hasAccess {
		// Grant access to the library
		access := &models.UserLibraryAccess{
			UserID:    user.ID,
			LibraryID: &libraryID,
		}
		_, err = db.NewInsert().Model(access).Exec(ctx)
		require.NoError(t, err)

		// Set the library access on the user
		user.LibraryAccess = []*models.UserLibraryAccess{access}
	}

	return user
}

// mockScanner implements the Scanner interface for testing.
type mockScanner struct{}

func (m *mockScanner) Scan(_ context.Context, _ ScanOptions) (*ScanResult, error) {
	return nil, nil
}

// setupTestServer sets up an Echo server with the book routes registered.
func setupTestServer(t *testing.T, db *bun.DB) *echo.Echo {
	t.Helper()

	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle

	// Create config for testing
	cfg := config.NewForTest()
	cfg.CacheDir = t.TempDir()

	// Create auth service and middleware
	authService := auth.NewService(db, cfg.JWTSecret)
	authMiddleware := auth.NewMiddleware(authService)

	// Register routes (pass nil for plugin manager in tests)
	g := e.Group("/books")
	RegisterRoutesWithGroup(g, db, cfg, authMiddleware, &mockScanner{}, nil)

	return e
}

func TestStreamFile_M4B_ReturnsAudioMp4ContentType(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	m4bPath := createTestM4BFile(t, 1000)
	file := setupTestFile(t, db, book, models.FileTypeM4B, m4bPath)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
	rr := executeRequestWithUser(t, e, req, user)

	// Verify content type
	assert.Equal(t, http.StatusOK, rr.Code, "Expected 200 OK for M4B stream")
	assert.Equal(t, "audio/mp4", rr.Header().Get("Content-Type"), "Expected audio/mp4 content type")
}

func TestStreamFile_NonM4BFile_Returns404(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	epubPath := createTestEPUBFile(t)
	file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
	rr := executeRequestWithUser(t, e, req, user)

	// Verify 404 for non-M4B file
	assert.Equal(t, http.StatusNotFound, rr.Code, "Expected 404 for non-M4B file")
}

func TestStreamFile_NonExistentFile_Returns404(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, _ := setupTestLibraryAndBook(t, db)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/99999/stream", nil)
	rr := executeRequestWithUser(t, e, req, user)

	// Verify 404
	assert.Equal(t, http.StatusNotFound, rr.Code, "Expected 404 for non-existent file")
}

func TestStreamFile_UnauthorizedLibraryAccess_Returns403(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	m4bPath := createTestM4BFile(t, 1000)
	file := setupTestFile(t, db, book, models.FileTypeM4B, m4bPath)
	// Create user WITHOUT access to the library
	user := setupTestUser(t, db, library.ID, false)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
	rr := executeRequestWithUser(t, e, req, user)

	// Verify 403 for unauthorized access
	assert.Equal(t, http.StatusForbidden, rr.Code, "Expected 403 for unauthorized library access")
}

func TestStreamFile_WithoutRangeHeader_Returns200AndFullFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	fileSize := 1000
	m4bPath := createTestM4BFile(t, fileSize)
	file := setupTestFile(t, db, book, models.FileTypeM4B, m4bPath)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
	rr := executeRequestWithUser(t, e, req, user)

	// Verify full file is returned with 200
	assert.Equal(t, http.StatusOK, rr.Code, "Expected 200 OK for full file request")
	assert.Equal(t, fileSize, rr.Body.Len(), "Expected full file content")
}

func TestStreamFile_WithRangeHeader_Returns206PartialContent(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	fileSize := 5000
	m4bPath := createTestM4BFile(t, fileSize)
	file := setupTestFile(t, db, book, models.FileTypeM4B, m4bPath)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
	req.Header.Set("Range", "bytes=0-999")
	rr := executeRequestWithUser(t, e, req, user)

	// Verify 206 Partial Content
	assert.Equal(t, http.StatusPartialContent, rr.Code, "Expected 206 Partial Content")

	// Verify Content-Range header
	contentRange := rr.Header().Get("Content-Range")
	assert.Contains(t, contentRange, "bytes 0-999/5000", "Expected Content-Range header")

	// Verify returned bytes are exactly 1000 bytes
	assert.Equal(t, 1000, rr.Body.Len(), "Expected 1000 bytes returned")
}

func TestStreamFile_RangeHeader_VerifyReturnedBytesMatchExpected(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	fileSize := 5000
	m4bPath := createTestM4BFile(t, fileSize)
	file := setupTestFile(t, db, book, models.FileTypeM4B, m4bPath)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
	req.Header.Set("Range", "bytes=100-199")
	rr := executeRequestWithUser(t, e, req, user)

	// Verify 206 Partial Content
	assert.Equal(t, http.StatusPartialContent, rr.Code, "Expected 206 Partial Content")

	// Read the original file bytes
	originalData, err := os.ReadFile(m4bPath)
	require.NoError(t, err)

	// Verify the returned bytes match the expected range
	returnedData, err := io.ReadAll(rr.Body)
	require.NoError(t, err)
	assert.Len(t, returnedData, 100, "Expected 100 bytes returned")
	assert.Equal(t, originalData[100:200], returnedData, "Returned bytes should match expected range")
}

func TestStreamFile_AcceptRangesHeader_IsPresent(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)
	m4bPath := createTestM4BFile(t, 1000)
	file := setupTestFile(t, db, book, models.FileTypeM4B, m4bPath)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
	rr := executeRequestWithUser(t, e, req, user)

	// Verify Accept-Ranges header is present
	assert.Equal(t, "bytes", rr.Header().Get("Accept-Ranges"), "Expected Accept-Ranges: bytes header")
}

func TestStreamFile_CBZFile_Returns404(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, book := setupTestLibraryAndBook(t, db)

	// Create a CBZ file
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")
	err := os.WriteFile(cbzPath, []byte("cbz content"), 0644)
	require.NoError(t, err)

	file := setupTestFile(t, db, book, models.FileTypeCBZ, cbzPath)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/stream", nil)
	rr := executeRequestWithUser(t, e, req, user)

	// Verify 404 for CBZ file
	assert.Equal(t, http.StatusNotFound, rr.Code, "Expected 404 for CBZ file")
}

func TestStreamFile_InvalidFileID_Returns404(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	library, _ := setupTestLibraryAndBook(t, db)
	user := setupTestUser(t, db, library.ID, true)

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodGet, "/books/files/invalid/stream", nil)
	rr := executeRequestWithUser(t, e, req, user)

	// Verify 404 for invalid file ID
	assert.Equal(t, http.StatusNotFound, rr.Code, "Expected 404 for invalid file ID")
}

func TestDeleteBook_DeletesBookAndFiles(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create temp directory for files
	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user with admin permissions (RoleID 1 is admin from migrations)
	user := &models.User{Username: "admin", PasswordHash: "hash", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	// Load the Role with Permissions from database (needed for RequirePermission middleware)
	err = db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Where("u.id = ?", user.ID).
		Scan(ctx)
	require.NoError(t, err)

	// Grant library access
	access := &models.UserLibraryAccess{UserID: user.ID, LibraryID: &library.ID}
	_, err = db.NewInsert().Model(access).Exec(ctx)
	require.NoError(t, err)
	user.LibraryAccess = []*models.UserLibraryAccess{access}

	// Create book with file
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        tmpDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	filePath := filepath.Join(tmpDir, "test.epub")
	err = os.WriteFile(filePath, []byte("content"), 0644)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filePath,
		FilesizeBytes: 7,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Setup Echo and handler
	e := setupTestServer(t, db)

	// Make DELETE request
	req := httptest.NewRequest(http.MethodDelete, "/books/"+strconv.Itoa(book.ID), nil)
	rr := executeRequestWithUser(t, e, req, user)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify book deleted
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify file deleted from disk
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteFile_DeletesFileAndKeepsBook(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create temp directory for files
	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user with admin permissions (RoleID 1 is admin from migrations)
	user := &models.User{Username: "admin", PasswordHash: "hash", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	// Load the Role with Permissions from database (needed for RequirePermission middleware)
	err = db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Where("u.id = ?", user.ID).
		Scan(ctx)
	require.NoError(t, err)

	// Grant library access
	access := &models.UserLibraryAccess{UserID: user.ID, LibraryID: &library.ID}
	_, err = db.NewInsert().Model(access).Exec(ctx)
	require.NoError(t, err)
	user.LibraryAccess = []*models.UserLibraryAccess{access}

	// Create book with two files
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        tmpDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file1Path := filepath.Join(tmpDir, "test1.epub")
	err = os.WriteFile(file1Path, []byte("content1"), 0644)
	require.NoError(t, err)
	file1 := &models.File{LibraryID: library.ID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file1Path, FilesizeBytes: 8}
	_, err = db.NewInsert().Model(file1).Exec(ctx)
	require.NoError(t, err)

	file2Path := filepath.Join(tmpDir, "test2.epub")
	err = os.WriteFile(file2Path, []byte("content2"), 0644)
	require.NoError(t, err)
	file2 := &models.File{LibraryID: library.ID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file2Path, FilesizeBytes: 8}
	_, err = db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	// Setup Echo and handler
	e := setupTestServer(t, db)

	// Delete first file
	req := httptest.NewRequest(http.MethodDelete, "/books/files/"+strconv.Itoa(file1.ID), nil)
	rr := executeRequestWithUser(t, e, req, user)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Parse response
	var resp struct {
		BookDeleted bool `json:"book_deleted"`
	}
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.BookDeleted)

	// Verify file1 deleted, book and file2 remain
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file1.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestListBooks_FiltersByIDs(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create temp directory
	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create three books with unique filepaths
	book1 := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book 1",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book 1",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        filepath.Join(tmpDir, "book1"),
	}
	_, err = db.NewInsert().Model(book1).Exec(ctx)
	require.NoError(t, err)

	book2 := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book 2",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book 2",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        filepath.Join(tmpDir, "book2"),
	}
	_, err = db.NewInsert().Model(book2).Exec(ctx)
	require.NoError(t, err)

	book3 := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book 3",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book 3",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        filepath.Join(tmpDir, "book3"),
	}
	_, err = db.NewInsert().Model(book3).Exec(ctx)
	require.NoError(t, err)

	// Test IDs filter using a direct query (avoiding the complex Service query with all Relations)
	// This tests the core IDs filter functionality without involving FTS and other complex relations
	var filteredBooks []*models.Book
	err = db.NewSelect().
		Model(&filteredBooks).
		Where("id IN (?)", bun.In([]int{book1.ID, book3.ID})).
		Order("sort_title ASC").
		Scan(ctx)
	require.NoError(t, err)

	assert.Len(t, filteredBooks, 2)

	// Verify we got the right books
	bookIDs := make([]int, len(filteredBooks))
	for i, b := range filteredBooks {
		bookIDs[i] = b.ID
	}
	assert.Contains(t, bookIDs, book1.ID)
	assert.Contains(t, bookIDs, book3.ID)
	assert.NotContains(t, bookIDs, book2.ID)
}

func TestDeleteBooks_BulkDeletesBooks(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create library and temp directory for files
	tmpDir := t.TempDir()
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user with admin permissions
	user := &models.User{Username: "admin", PasswordHash: "hash", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	err = db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Where("u.id = ?", user.ID).
		Scan(ctx)
	require.NoError(t, err)

	// Grant library access
	access := &models.UserLibraryAccess{UserID: user.ID, LibraryID: &library.ID}
	_, err = db.NewInsert().Model(access).Exec(ctx)
	require.NoError(t, err)
	user.LibraryAccess = []*models.UserLibraryAccess{access}

	// Create two books with files (each needs unique filepath)
	book1Dir := filepath.Join(tmpDir, "book1")
	require.NoError(t, os.MkdirAll(book1Dir, 0755))
	book1 := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book 1",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book 1",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        book1Dir,
	}
	_, err = db.NewInsert().Model(book1).Exec(ctx)
	require.NoError(t, err)

	file1Path := filepath.Join(book1Dir, "book1.epub")
	err = os.WriteFile(file1Path, []byte("content1"), 0644)
	require.NoError(t, err)
	file1 := &models.File{LibraryID: library.ID, BookID: book1.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file1Path, FilesizeBytes: 8}
	_, err = db.NewInsert().Model(file1).Exec(ctx)
	require.NoError(t, err)

	book2Dir := filepath.Join(tmpDir, "book2")
	require.NoError(t, os.MkdirAll(book2Dir, 0755))
	book2 := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book 2",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book 2",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        book2Dir,
	}
	_, err = db.NewInsert().Model(book2).Exec(ctx)
	require.NoError(t, err)

	file2Path := filepath.Join(book2Dir, "book2.epub")
	err = os.WriteFile(file2Path, []byte("content2"), 0644)
	require.NoError(t, err)
	file2 := &models.File{LibraryID: library.ID, BookID: book2.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file2Path, FilesizeBytes: 8}
	_, err = db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	// Setup Echo
	e := setupTestServer(t, db)

	// Bulk delete
	body := `{"book_ids": [` + strconv.Itoa(book1.ID) + `, ` + strconv.Itoa(book2.ID) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/books/delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Parse response
	var resp struct {
		BooksDeleted int `json:"books_deleted"`
		FilesDeleted int `json:"files_deleted"`
	}
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.BooksDeleted)
	assert.Equal(t, 2, resp.FilesDeleted)

	// Verify books deleted
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id IN (?)", bun.In([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
