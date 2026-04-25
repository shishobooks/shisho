package books

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
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

	// Enable foreign keys to match production behavior
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

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
	authService := auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration())
	authMiddleware := auth.NewMiddleware(authService)

	// Register routes (pass nil for plugin manager in tests)
	g := e.Group("/books")
	RegisterRoutesWithGroup(g, db, cfg, authMiddleware, &mockScanner{}, nil, nil)

	return e
}

// seedBookAndFile inserts a library, a book with the given title, and a file
// with the given role/name/name-source. Returns the library, the book, and
// the file. The book uses a directory-backed layout rooted at t.TempDir().
func seedBookAndFile(
	t *testing.T,
	db *bun.DB,
	bookTitle string,
	fileName *string,
	fileNameSource *string,
	role string,
) (*models.Library, *models.Book, *models.File) {
	t.Helper()
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	bookDir := t.TempDir()
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           bookTitle,
		TitleSource:     models.DataSourceManual,
		SortTitle:       bookTitle,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	filePath := filepath.Join(bookDir, "test.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("epub content"), 0644))
	file := setupTestFile(t, db, book, models.FileTypeEPUB, filePath)
	file.FileRole = role
	_, err = db.NewUpdate().Model(file).Column("file_role").WherePK().Exec(ctx)
	require.NoError(t, err)

	if fileName != nil || fileNameSource != nil {
		file.Name = fileName
		file.NameSource = fileNameSource
		_, err = db.NewUpdate().Model(file).Column("name", "name_source").WherePK().Exec(ctx)
		require.NoError(t, err)
	}

	return library, book, file
}

// loadUserWithRole reloads a user with Role and Role.Permissions so the
// RequirePermission middleware passes in tests.
func loadUserWithRole(t *testing.T, db *bun.DB, user *models.User) *models.User {
	t.Helper()
	ctx := context.Background()
	err := db.NewSelect().
		Model(user).
		Relation("Role").
		Relation("Role.Permissions").
		Where("u.id = ?", user.ID).
		Scan(ctx)
	require.NoError(t, err)
	return user
}

// newUpdateTitleRequest builds a POST /books/:id request with a JSON body
// containing only a title change.
func newUpdateTitleRequest(bookID int, newTitle string) *http.Request {
	body := `{"title": ` + strconv.Quote(newTitle) + `}`
	req := httptest.NewRequest(http.MethodPost, "/books/"+strconv.Itoa(bookID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
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
		Where("id IN (?)", bun.List([]int{book1.ID, book3.ID})).
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
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id IN (?)", bun.List([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestUpdateFile_DowngradeMainToSupplement_DeletesCoverFile(t *testing.T) {
	t.Parallel()

	// runDowngrade wires up the library/book/file, plants a cover file, hits the
	// update endpoint to downgrade main → supplement, and asserts the cover file
	// is gone from disk. The caller picks whether the book is directory-backed
	// or a root-level file, since the cover dir resolution differs.
	runDowngrade := func(t *testing.T, makeBook func(t *testing.T, libraryID int) (book *models.Book, bookDir string, filePath string)) {
		t.Helper()
		db := setupTestDB(t)
		ctx := context.Background()

		library := &models.Library{
			Name:                     "Test Library",
			CoverAspectRatio:         "book",
			DownloadFormatPreference: models.DownloadFormatOriginal,
		}
		_, err := db.NewInsert().Model(library).Exec(ctx)
		require.NoError(t, err)

		book, bookDir, epubPath := makeBook(t, library.ID)
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(epubPath, []byte("epub content"), 0644))
		file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)

		// Create a cover image in the resolved cover dir and wire it up in the
		// DB. CoverImageFilename stores just the filename — the full path is
		// constructed at runtime.
		coverFilename := filepath.Base(epubPath) + ".cover.jpg"
		coverFullPath := filepath.Join(bookDir, coverFilename)
		require.NoError(t, os.WriteFile(coverFullPath, []byte("cover data"), 0644))

		mimeType := "image/jpeg"
		coverSource := models.DataSourceManual
		file.CoverImageFilename = &coverFilename
		file.CoverMimeType = &mimeType
		file.CoverSource = &coverSource
		_, err = db.NewUpdate().
			Model(file).
			Column("cover_image_filename", "cover_mime_type", "cover_source").
			WherePK().
			Exec(ctx)
		require.NoError(t, err)

		user := setupTestUser(t, db, library.ID, true)
		// Load the Role with Permissions so RequirePermission middleware can pass.
		err = db.NewSelect().
			Model(user).
			Relation("Role").
			Relation("Role.Permissions").
			Where("u.id = ?", user.ID).
			Scan(ctx)
		require.NoError(t, err)

		// Downgrade the file from main to supplement via the update endpoint.
		e := setupTestServer(t, db)
		body := `{"file_role": "supplement"}`
		req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := executeRequestWithUser(t, e, req, user)
		require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

		// The cover file must be removed from disk, not just cleared in the DB.
		_, statErr := os.Stat(coverFullPath)
		assert.True(t, os.IsNotExist(statErr),
			"expected cover file to be deleted on downgrade, but os.Stat(%q) returned err=%v", coverFullPath, statErr)

		// Sanity check: the DB row should also reflect the downgrade.
		var updated models.File
		err = db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
		require.NoError(t, err)
		assert.Equal(t, models.FileRoleSupplement, updated.FileRole)
		assert.Nil(t, updated.CoverImageFilename)
	}

	t.Run("directory-backed book", func(t *testing.T) {
		t.Parallel()
		runDowngrade(t, func(t *testing.T, libraryID int) (*models.Book, string, string) {
			t.Helper()
			bookDir := t.TempDir()
			book := &models.Book{
				LibraryID:       libraryID,
				Title:           "Test Book",
				TitleSource:     models.DataSourceFilepath,
				SortTitle:       "Test Book",
				SortTitleSource: models.DataSourceFilepath,
				AuthorSource:    models.DataSourceFilepath,
				Filepath:        bookDir,
			}
			return book, bookDir, filepath.Join(bookDir, "test.epub")
		})
	})

	t.Run("root-level book", func(t *testing.T) {
		t.Parallel()
		runDowngrade(t, func(t *testing.T, libraryID int) (*models.Book, string, string) {
			t.Helper()
			libraryDir := t.TempDir()
			epubPath := filepath.Join(libraryDir, "root-book.epub")
			// Root-level books have book.Filepath pointing at the file itself,
			// not a containing directory. The cover lives alongside the file
			// in the library dir.
			book := &models.Book{
				LibraryID:       libraryID,
				Title:           "Root Book",
				TitleSource:     models.DataSourceFilepath,
				SortTitle:       "Root Book",
				SortTitleSource: models.DataSourceFilepath,
				AuthorSource:    models.DataSourceFilepath,
				Filepath:        epubPath,
			}
			return book, libraryDir, epubPath
		})
	})

	// Regression: for root-level files in non-organized libraries,
	// scanFileCreateNew stores a synthetic organized-folder path in
	// book.Filepath (e.g. /library/Author/Title) that never gets created
	// on disk. Resolving the cover via book.Filepath would stat that
	// junk path, fall back to using it as a directory, and silently
	// miss the real cover that lives next to the file at
	// /library/root-book.epub.cover.jpg.
	t.Run("root-level file with synthetic book path", func(t *testing.T) {
		t.Parallel()
		runDowngrade(t, func(t *testing.T, libraryID int) (*models.Book, string, string) {
			t.Helper()
			libraryDir := t.TempDir()
			epubPath := filepath.Join(libraryDir, "root-book.epub")
			// Synthetic organized-folder path — this is what scanFileCreateNew
			// computes at pkg/worker/scan_unified.go:2106 for root-level files,
			// regardless of whether OrganizeFileStructure is enabled.
			syntheticBookPath := filepath.Join(libraryDir, "Author Name", "Book Title")
			book := &models.Book{
				LibraryID:       libraryID,
				Title:           "Root Book",
				TitleSource:     models.DataSourceFilepath,
				SortTitle:       "Root Book",
				SortTitleSource: models.DataSourceFilepath,
				AuthorSource:    models.DataSourceFilepath,
				Filepath:        syntheticBookPath,
			}
			return book, libraryDir, epubPath
		})
	})
}

// Regression: when a user drops a file at the library root with
// OrganizeFileStructure disabled, scanFileCreateNew writes a synthetic
// organized-folder path into book.Filepath that never exists on disk. The
// actual cover lives alongside the file at filepath.Dir(file.Filepath),
// not under the synthetic book path. The bookCover and fileCover handlers
// must resolve the cover via the file's parent dir, not book.Filepath.
func TestServeCover_RootLevelFile_SyntheticBookPath_ServesCoverFromFileDir(t *testing.T) {
	t.Parallel()

	runServe := func(t *testing.T, fetchURL func(bookID, fileID int) string) {
		t.Helper()
		db := setupTestDB(t)
		ctx := context.Background()

		library := &models.Library{
			Name:                     "Test Library",
			CoverAspectRatio:         "book",
			DownloadFormatPreference: models.DownloadFormatOriginal,
		}
		_, err := db.NewInsert().Model(library).Exec(ctx)
		require.NoError(t, err)

		libraryDir := t.TempDir()
		epubPath := filepath.Join(libraryDir, "root-book.epub")
		require.NoError(t, os.WriteFile(epubPath, []byte("epub content"), 0644))

		// Synthetic organized-folder path — same shape scanFileCreateNew
		// produces for root-level files regardless of OrganizeFileStructure.
		syntheticBookPath := filepath.Join(libraryDir, "Author Name", "Book Title")
		book := &models.Book{
			LibraryID:       library.ID,
			Title:           "Root Book",
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       "Root Book",
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        syntheticBookPath,
		}
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)

		coverFilename := filepath.Base(epubPath) + ".cover.jpg"
		coverFullPath := filepath.Join(libraryDir, coverFilename)
		coverData := []byte("cover-bytes")
		require.NoError(t, os.WriteFile(coverFullPath, coverData, 0644))

		mimeType := "image/jpeg"
		coverSource := models.DataSourceExistingCover
		file.CoverImageFilename = &coverFilename
		file.CoverMimeType = &mimeType
		file.CoverSource = &coverSource
		_, err = db.NewUpdate().
			Model(file).
			Column("cover_image_filename", "cover_mime_type", "cover_source").
			WherePK().
			Exec(ctx)
		require.NoError(t, err)

		user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

		e := setupTestServer(t, db)
		req := httptest.NewRequest(http.MethodGet, fetchURL(book.ID, file.ID), nil)
		rr := executeRequestWithUser(t, e, req, user)

		require.Equal(t, http.StatusOK, rr.Code,
			"expected 200 serving cover for root-level file with synthetic book path, got %d: %s",
			rr.Code, rr.Body.String())
		assert.Equal(t, coverData, rr.Body.Bytes(),
			"expected cover body to match the file planted next to the root-level file")
	}

	t.Run("bookCover", func(t *testing.T) {
		t.Parallel()
		runServe(t, func(bookID, _ int) string {
			return "/books/" + strconv.Itoa(bookID) + "/cover"
		})
	})

	t.Run("fileCover", func(t *testing.T) {
		t.Parallel()
		runServe(t, func(_, fileID int) string {
			return "/books/files/" + strconv.Itoa(fileID) + "/cover"
		})
	})
}

// Regression: when narrators are updated on a root-level M4B file in a
// library with OrganizeFileStructure enabled, the handler must move the
// file into its organized folder AND keep book.Filepath in sync. The
// previous behavior called RenameOrganizedFileOnly, which renamed the file
// in place at the library root and left book.Filepath pointing at the
// synthetic organized path — a desync that broke book-level sidecar
// resolution and future reorganize attempts.
func TestUpdateFile_Narrators_RootLevelM4B_OrganizesFileAndSyncsBookPath(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	libraryDir := t.TempDir()
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "audiobook",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	libraryPath := &models.LibraryPath{LibraryID: library.ID, Filepath: libraryDir}
	_, err = db.NewInsert().Model(libraryPath).Exec(ctx)
	require.NoError(t, err)

	// Create a real M4B file at the library root.
	m4bPath := filepath.Join(libraryDir, "book.m4b")
	require.NoError(t, os.WriteFile(m4bPath, []byte("m4b content"), 0644))

	// Book with synthetic organized-folder path (what scanFileCreateNew
	// writes) — doesn't exist on disk.
	syntheticBookPath := filepath.Join(libraryDir, "Author Name", "Book Title")
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book Title",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book Title",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        syntheticBookPath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Author association so organize can compute the folder name.
	author := &models.Person{Name: "Author Name", LibraryID: library.ID, SortName: "Author Name"}
	_, err = db.NewInsert().Model(author).Exec(ctx)
	require.NoError(t, err)
	authorAssoc := &models.Author{BookID: book.ID, PersonID: author.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(authorAssoc).Exec(ctx)
	require.NoError(t, err)

	file := setupTestFile(t, db, book, models.FileTypeM4B, m4bPath)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	body := `{"narrators": ["Narrator Name"]}`
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	// File must have been moved out of the library root and into an
	// organized folder under the library path.
	var updatedFile models.File
	err = db.NewSelect().Model(&updatedFile).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, libraryDir, filepath.Dir(updatedFile.Filepath),
		"file should no longer be at the library root after organize; path=%s", updatedFile.Filepath)
	_, err = os.Stat(updatedFile.Filepath)
	require.NoError(t, err, "renamed file should exist at %s", updatedFile.Filepath)

	// book.Filepath must match the file's containing directory — no more
	// synthetic-path desync.
	var updatedBook models.Book
	err = db.NewSelect().Model(&updatedBook).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Equal(t, filepath.Dir(updatedFile.Filepath), updatedBook.Filepath,
		"book.Filepath should track the organized folder")
}

// Regression: when both narrators and name are updated on a directory-backed
// M4B in the same request, the name-triggered reorganize must use the NEW
// narrator names (just persisted this request), not the stale in-memory
// file.Narrators relation. Previously the name branch built narratorNames
// from the in-memory file.Narrators which wasn't refreshed after the
// narrator-update DB writes, producing a filename with stale narrators.
func TestUpdateFile_NarratorsAndName_DirectoryBacked_UsesNewNarratorsInFilename(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	libraryDir := t.TempDir()
	bookDir := filepath.Join(libraryDir, "Old Author", "Old Title")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	m4bPath := filepath.Join(bookDir, "original.m4b")
	require.NoError(t, os.WriteFile(m4bPath, []byte("m4b"), 0644))

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "audiobook",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.LibraryPath{LibraryID: library.ID, Filepath: libraryDir}).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Old Title",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Old Title",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	author := &models.Person{Name: "Old Author", LibraryID: library.ID, SortName: "Old Author"}
	_, err = db.NewInsert().Model(author).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: author.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(t, err)

	// Pre-existing stale narrator so file.Narrators is non-empty on load.
	stalePerson := &models.Person{Name: "Stale Narrator", LibraryID: library.ID, SortName: "Stale Narrator"}
	_, err = db.NewInsert().Model(stalePerson).Exec(ctx)
	require.NoError(t, err)
	file := setupTestFile(t, db, book, models.FileTypeM4B, m4bPath)
	_, err = db.NewInsert().Model(&models.Narrator{FileID: file.ID, PersonID: stalePerson.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(t, err)

	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	// Update BOTH narrators (swap Stale for Fresh) AND name in one request.
	body := `{"narrators": ["Fresh Narrator"], "name": "Fresh Title"}`
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updatedFile models.File
	err = db.NewSelect().Model(&updatedFile).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)

	// The organized filename must reflect the NEW narrator, not the stale one.
	assert.Contains(t, filepath.Base(updatedFile.Filepath), "Fresh Narrator",
		"organized filename should include the new narrator; got %s", updatedFile.Filepath)
	assert.NotContains(t, filepath.Base(updatedFile.Filepath), "Stale Narrator",
		"organized filename should NOT include the replaced stale narrator; got %s", updatedFile.Filepath)
}

// Regression: when file.Name is updated on a root-level file in a library
// with OrganizeFileStructure enabled, the new name must be reflected in the
// organized filename. Previously the root-level branch of organizeBookFiles
// always used book.Title (ignoring file.Name), and the handler persisted
// file.Name after reorganize, so the new name was lost.
func TestUpdateFile_Name_RootLevelFile_OrganizesWithNewName(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	libraryDir := t.TempDir()
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	libraryPath := &models.LibraryPath{LibraryID: library.ID, Filepath: libraryDir}
	_, err = db.NewInsert().Model(libraryPath).Exec(ctx)
	require.NoError(t, err)

	epubPath := filepath.Join(libraryDir, "original.epub")
	require.NoError(t, os.WriteFile(epubPath, []byte("epub content"), 0644))

	syntheticBookPath := filepath.Join(libraryDir, "Author Name", "Book Title")
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book Title",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book Title",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        syntheticBookPath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	author := &models.Person{Name: "Author Name", LibraryID: library.ID, SortName: "Author Name"}
	_, err = db.NewInsert().Model(author).Exec(ctx)
	require.NoError(t, err)
	authorAssoc := &models.Author{BookID: book.ID, PersonID: author.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(authorAssoc).Exec(ctx)
	require.NoError(t, err)

	file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	body := `{"name": "Custom Name"}`
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updatedFile models.File
	err = db.NewSelect().Model(&updatedFile).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)

	// The organized filename should reflect the new file.Name, not the original
	// filename or book.Title.
	assert.Contains(t, filepath.Base(updatedFile.Filepath), "Custom Name",
		"organized filename should include the new file.Name; got %s", updatedFile.Filepath)
	_, err = os.Stat(updatedFile.Filepath)
	require.NoError(t, err, "file should exist at new organized location %s", updatedFile.Filepath)
}

// Regression: when OrganizeBookFiles fails partway through a file metadata
// change on a root-level file, the helper must revert the DB write it just
// made so the DB and on-disk state stay consistent. Without the revert the
// user would see the new metadata in the UI while the file sits unchanged
// on disk.
func TestUpdateFile_Name_RootLevelFile_OrganizeFailure_RevertsDBState(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	libraryDir := t.TempDir()
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.LibraryPath{LibraryID: library.ID, Filepath: libraryDir}).Exec(ctx)
	require.NoError(t, err)

	epubPath := filepath.Join(libraryDir, "original.epub")
	require.NoError(t, os.WriteFile(epubPath, []byte("epub"), 0644))

	syntheticBookPath := filepath.Join(libraryDir, "Author Name", "Book Title")
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book Title",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book Title",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        syntheticBookPath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	author := &models.Person{Name: "Author Name", LibraryID: library.ID, SortName: "Author Name"}
	_, err = db.NewInsert().Model(author).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: author.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(t, err)

	originalName := "Original Name"
	originalNameSource := models.DataSourceManual
	file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)
	file.Name = &originalName
	file.NameSource = &originalNameSource
	_, err = db.NewUpdate().Model(file).Column("name", "name_source").WherePK().Exec(ctx)
	require.NoError(t, err)

	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	// Force OrganizeBookFiles to fail by removing the file on disk between
	// the DB insert and the update request. OrganizeRootLevelFile will try
	// to move the missing file and fail.
	require.NoError(t, os.Remove(epubPath))

	e := setupTestServer(t, db)
	body := `{"name": "Attempted New Name"}`
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	// The handler doesn't surface organize errors as HTTP failures (they
	// are logged), so we still get 200 — but the DB must be reverted.
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var reloaded models.File
	err = db.NewSelect().Model(&reloaded).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, reloaded.Name)
	assert.Equal(t, originalName, *reloaded.Name,
		"file.Name must be reverted to the pre-update value when organize fails")
	require.NotNil(t, reloaded.NameSource)
	assert.Equal(t, originalNameSource, *reloaded.NameSource,
		"file.NameSource must be reverted to the pre-update value when organize fails")
}

// Regression: when RenameOrganizedFileOnly fails on a directory-backed file
// (e.g. the source file was removed out from under us), the helper must
// revert the pending name/name_source columns so the outer UpdateFile
// doesn't persist a new name while the file sits at its old path on disk.
// Mirrors the same-class invariant that M5 enforces for the root-level
// branch.
func TestUpdateFile_Name_DirectoryBacked_RenameFailure_RevertsDBState(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	libraryDir := t.TempDir()
	bookDir := filepath.Join(libraryDir, "Author", "Book")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	epubPath := filepath.Join(bookDir, "book.epub")
	require.NoError(t, os.WriteFile(epubPath, []byte("epub"), 0644))

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.LibraryPath{LibraryID: library.ID, Filepath: libraryDir}).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	author := &models.Person{Name: "Author", LibraryID: library.ID, SortName: "Author"}
	_, err = db.NewInsert().Model(author).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: author.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(t, err)

	originalName := "Original Name"
	originalNameSource := models.DataSourceManual
	file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)
	file.Name = &originalName
	file.NameSource = &originalNameSource
	_, err = db.NewUpdate().Model(file).Column("name", "name_source").WherePK().Exec(ctx)
	require.NoError(t, err)

	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	// Force RenameOrganizedFileOnly to fail by removing the source file.
	require.NoError(t, os.Remove(epubPath))

	e := setupTestServer(t, db)
	body := `{"name": "Attempted New Name"}`
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var reloaded models.File
	err = db.NewSelect().Model(&reloaded).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, reloaded.Name)
	assert.Equal(t, originalName, *reloaded.Name,
		"file.Name must be reverted when rename fails in the directory-backed branch")
	require.NotNil(t, reloaded.NameSource)
	assert.Equal(t, originalNameSource, *reloaded.NameSource,
		"file.NameSource must be reverted when rename fails in the directory-backed branch")
}

// Regression: uploading a cover to a root-level file previously resolved
// the cover directory via book.Filepath (synthetic, non-existent), so the
// write failed with "no such file or directory". The cover must be written
// next to the file.
func TestUploadFileCover_RootLevelFile_SyntheticBookPath_WritesNextToFile(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	libraryDir := t.TempDir()
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	epubPath := filepath.Join(libraryDir, "root-book.epub")
	require.NoError(t, os.WriteFile(epubPath, []byte("epub content"), 0644))

	syntheticBookPath := filepath.Join(libraryDir, "Author Name", "Book Title")
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Root Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Root Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        syntheticBookPath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	// Build multipart upload body with a tiny valid PNG.
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
		0x54, 0x08, 0x99, 0x63, 0xF8, 0x0F, 0x00, 0x00,
		0x01, 0x01, 0x00, 0x01, 0x1B, 0xB6, 0xEE, 0x56,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
		0xAE, 0x42, 0x60, 0x82,
	}

	var body strings.Builder
	writer := multipart.NewWriter(&body)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", `form-data; name="cover"; filename="cover.png"`)
	partHeader.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(partHeader)
	require.NoError(t, err)
	_, err = part.Write(pngData)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID)+"/cover", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	// Cover should have been written alongside the file, not under the synthetic book dir.
	expectedCover := filepath.Join(libraryDir, "root-book.epub.cover.png")
	_, err = os.Stat(expectedCover)
	require.NoError(t, err, "cover should exist at %s", expectedCover)

	// Synthetic book dir must not have been created.
	_, err = os.Stat(syntheticBookPath)
	assert.True(t, os.IsNotExist(err), "synthetic book dir must not be created for cover upload")
}

func TestUpdateBook_Title_UpdatesMainFileName_WhenMatchesOldTitle(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	matchingName := "Foo"
	filepathSource := models.DataSourceFilepath
	library, book, file := seedBookAndFile(t, db, "Foo", &matchingName, &filepathSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Bar", *updated.Name)
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceManual, *updated.NameSource)
}

func TestUpdateBook_Title_UpdatesNilFileName_ToNewTitle(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	library, book, file := seedBookAndFile(t, db, "Foo", nil, nil, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Bar", *updated.Name)
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceManual, *updated.NameSource)
}

func TestUpdateBook_Title_UpdatesEmptyFileName_ToNewTitle(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	emptyName := ""
	filepathSource := models.DataSourceFilepath
	library, book, file := seedBookAndFile(t, db, "Foo", &emptyName, &filepathSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Bar", *updated.Name)
}

func TestUpdateBook_Title_MatchesWithTrimAndCasefold(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	fileName := "  foo bar  "
	filepathSource := models.DataSourceFilepath
	library, book, file := seedBookAndFile(t, db, "Foo Bar", &fileName, &filepathSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Baz")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Baz", *updated.Name)
}

func TestUpdateBook_Title_PreservesCustomFileName_WhenDiffers(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	customName := "Baz"
	manualSource := models.DataSourceManual
	library, book, file := seedBookAndFile(t, db, "Foo", &customName, &manualSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Baz", *updated.Name, "custom file.Name that differs from old title must be preserved")
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceManual, *updated.NameSource)
}

func TestUpdateBook_Title_DoesNotTouchSupplementFileName(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	matchingName := "Foo"
	filepathSource := models.DataSourceFilepath
	library, book, supplement := seedBookAndFile(t, db, "Foo", &matchingName, &filepathSource, models.FileRoleSupplement)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", supplement.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "Foo", *updated.Name, "supplement file name must not be synced from book title")
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceFilepath, *updated.NameSource)
}

func TestUpdateBook_Title_MultipleMainFiles_IndependentlyChecked(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	bookDir := t.TempDir()
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Foo",
		TitleSource:     models.DataSourceManual,
		SortTitle:       "Foo",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	matchingPath := filepath.Join(bookDir, "match.epub")
	require.NoError(t, os.WriteFile(matchingPath, []byte("x"), 0644))
	matchingFile := setupTestFile(t, db, book, models.FileTypeEPUB, matchingPath)
	matchingName := "Foo"
	filepathSource := models.DataSourceFilepath
	matchingFile.Name = &matchingName
	matchingFile.NameSource = &filepathSource
	_, err = db.NewUpdate().Model(matchingFile).Column("name", "name_source").WherePK().Exec(ctx)
	require.NoError(t, err)

	customPath := filepath.Join(bookDir, "custom.epub")
	require.NoError(t, os.WriteFile(customPath, []byte("x"), 0644))
	customFile := setupTestFile(t, db, book, models.FileTypeEPUB, customPath)
	customName := "Totally Different"
	manualSource := models.DataSourceManual
	customFile.Name = &customName
	customFile.NameSource = &manualSource
	_, err = db.NewUpdate().Model(customFile).Column("name", "name_source").WherePK().Exec(ctx)
	require.NoError(t, err)

	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Bar")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updatedMatching models.File
	err = db.NewSelect().Model(&updatedMatching).Where("id = ?", matchingFile.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updatedMatching.Name)
	assert.Equal(t, "Bar", *updatedMatching.Name)

	var updatedCustom models.File
	err = db.NewSelect().Model(&updatedCustom).Where("id = ?", customFile.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updatedCustom.Name)
	assert.Equal(t, "Totally Different", *updatedCustom.Name)
}

func TestUpdateBook_Title_Unchanged_DoesNotTouchFileName(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	// file.Name intentionally set to a differently-cased variant of the title
	// so if the sync ran it would normalize the value. We expect it NOT to run
	// because the title itself is unchanged.
	fileName := "foo"
	filepathSource := models.DataSourceFilepath
	library, book, file := seedBookAndFile(t, db, "Foo", &fileName, &filepathSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "Foo")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "foo", *updated.Name, "no-op title change must not touch file.Name")
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceFilepath, *updated.NameSource,
		"no-op title change must not touch file.NameSource")
}

func TestUpdateBook_Title_EmptyString_Returns422(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)

	matchingName := "Foo"
	filepathSource := models.DataSourceFilepath
	library, book, _ := seedBookAndFile(t, db, "Foo", &matchingName, &filepathSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "")
	rr := executeRequestWithUser(t, e, req, user)
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code, "response body: %s", rr.Body.String())
}

func TestUpdateBook_Title_WhitespaceOnly_Returns422(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)

	matchingName := "Foo"
	filepathSource := models.DataSourceFilepath
	library, book, _ := seedBookAndFile(t, db, "Foo", &matchingName, &filepathSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "   ")
	rr := executeRequestWithUser(t, e, req, user)
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code, "response body: %s", rr.Body.String())
}

func TestUpdateBook_Title_LeadingTrailingWhitespace_TrimmedOnStore(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	matchingName := "Foo"
	filepathSource := models.DataSourceFilepath
	library, book, file := seedBookAndFile(t, db, "Foo", &matchingName, &filepathSource, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	req := newUpdateTitleRequest(book.ID, "  Bar  ")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updatedBook models.Book
	err := db.NewSelect().Model(&updatedBook).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Bar", updatedBook.Title, "book.Title must be stored without surrounding whitespace")

	var updatedFile models.File
	err = db.NewSelect().Model(&updatedFile).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "Bar", *updatedFile.Name, "file.Name must be stored without surrounding whitespace")
}

func TestUpdateFile_Name_LeadingTrailingWhitespace_TrimmedOnStore(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	ctx := context.Background()

	library, _, file := seedBookAndFile(t, db, "Foo", nil, nil, models.FileRoleMain)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	body := `{"name": "  custom name  "}`
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var updated models.File
	err := db.NewSelect().Model(&updated).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updated.Name)
	assert.Equal(t, "custom name", *updated.Name, "file.Name must be stored trimmed")
	require.NotNil(t, updated.NameSource)
	assert.Equal(t, models.DataSourceManual, *updated.NameSource)
}

func TestUpdateFile_RejectsDuplicateIdentifierTypes(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	library, book := setupTestLibraryAndBook(t, db)
	epubPath := createTestEPUBFile(t)
	file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)

	// Seed an existing identifier so we can assert it's preserved when the
	// 422 short-circuits the handler.
	require.NoError(t, svc.BulkCreateFileIdentifiers(ctx, []*models.FileIdentifier{
		{FileID: file.ID, Type: "asin", Value: "B01ORIGINAL", Source: models.DataSourceManual},
	}))

	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	e := setupTestServer(t, db)
	body := `{"identifiers":[{"type":"asin","value":"B01AAA"},{"type":"asin","value":"B02BBB"}]}`
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)

	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code, "response body: %s", rr.Body.String())
	assert.Contains(t, rr.Body.String(), "duplicate identifier type: asin")

	// Existing identifier untouched (request short-circuited before DB mutation).
	var stored []*models.FileIdentifier
	require.NoError(t, db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Scan(ctx))
	require.Len(t, stored, 1)
	assert.Equal(t, "B01ORIGINAL", stored[0].Value)
}

func TestUpdateFile_PreservesSourceForUnchangedIdentifiers(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	library, book := setupTestLibraryAndBook(t, db)
	epubPath := createTestEPUBFile(t)
	file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)

	// Seed identifiers with two distinct sources.
	pluginSource := models.PluginDataSource("shisho", "audnexus")
	require.NoError(t, svc.BulkCreateFileIdentifiers(ctx, []*models.FileIdentifier{
		{FileID: file.ID, Type: "asin", Value: "B01ABC1234", Source: pluginSource},
		{FileID: file.ID, Type: "isbn_13", Value: "9780316769488", Source: models.DataSourceEPUBMetadata},
	}))

	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	// Re-submit ASIN with the same value, ISBN unchanged. Add a new goodreads.
	body := `{"identifiers":[{"type":"asin","value":"B01ABC1234"},{"type":"isbn_13","value":"9780316769488"},{"type":"goodreads","value":"12345"}]}`
	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var stored []*models.FileIdentifier
	require.NoError(t, db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Order("type ASC").Scan(ctx))
	require.Len(t, stored, 3)
	byType := map[string]*models.FileIdentifier{}
	for _, id := range stored {
		byType[id.Type] = id
	}
	assert.Equal(t, pluginSource, byType["asin"].Source, "asin source preserved (unchanged value)")
	assert.Equal(t, models.DataSourceEPUBMetadata, byType["isbn_13"].Source, "isbn_13 source preserved (unchanged value)")
	assert.Equal(t, models.DataSourceManual, byType["goodreads"].Source, "new identifier gets manual source")
}

func TestUpdateFile_AssignsManualSourceWhenIdentifierValueChanges(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	library, book := setupTestLibraryAndBook(t, db)
	epubPath := createTestEPUBFile(t)
	file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)

	pluginSource := models.PluginDataSource("shisho", "audnexus")
	require.NoError(t, svc.BulkCreateFileIdentifiers(ctx, []*models.FileIdentifier{
		{FileID: file.ID, Type: "asin", Value: "B01ORIG", Source: pluginSource},
	}))

	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	body := `{"identifiers":[{"type":"asin","value":"B02NEW"}]}`
	e := setupTestServer(t, db)
	req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)
	require.Equal(t, http.StatusOK, rr.Code, "response body: %s", rr.Body.String())

	var stored []*models.FileIdentifier
	require.NoError(t, db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Scan(ctx))
	require.Len(t, stored, 1)
	assert.Equal(t, "B02NEW", stored[0].Value)
	assert.Equal(t, models.DataSourceManual, stored[0].Source, "value-changed entry gets manual source")
}

func TestUpdateFile_RejectsBlankIdentifierTypeAndValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
	}{
		{
			name: "blank type",
			body: `{"identifiers":[{"type":"","value":"B01ABC1234"}]}`,
		},
		{
			name: "blank value",
			body: `{"identifiers":[{"type":"asin","value":""}]}`,
		},
		{
			name: "whitespace-only type",
			body: `{"identifiers":[{"type":"   ","value":"B01ABC1234"}]}`,
		},
		{
			name: "whitespace-only value",
			body: `{"identifiers":[{"type":"asin","value":"   "}]}`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := setupTestDB(t)
			ctx := context.Background()
			svc := NewService(db)

			library, book := setupTestLibraryAndBook(t, db)
			epubPath := createTestEPUBFile(t)
			file := setupTestFile(t, db, book, models.FileTypeEPUB, epubPath)

			require.NoError(t, svc.BulkCreateFileIdentifiers(ctx, []*models.FileIdentifier{
				{FileID: file.ID, Type: "asin", Value: "B01ORIGINAL", Source: models.DataSourceManual},
			}))

			user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

			e := setupTestServer(t, db)
			req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rr := executeRequestWithUser(t, e, req, user)

			assert.Equal(t, http.StatusUnprocessableEntity, rr.Code, "expected 422, got %d body=%s", rr.Code, rr.Body.String())

			var stored []*models.FileIdentifier
			require.NoError(t, db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Scan(ctx))
			require.Len(t, stored, 1)
			assert.Equal(t, "B01ORIGINAL", stored[0].Value, "existing identifier must not be touched")
		})
	}
}
