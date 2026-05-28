package ereader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetBookFileType_UsesFirstMainFile confirms that getBookFileType
// returns the type of the first main file (by order in the slice),
// ignoring supplement files.
func TestGetBookFileType_UsesFirstMainFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		book *models.Book
		want string
	}{
		{
			name: "single main file",
			book: &models.Book{
				Files: []*models.File{
					{ID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain},
				},
			},
			want: models.FileTypeEPUB,
		},
		{
			name: "multiple main files returns first",
			book: &models.Book{
				Files: []*models.File{
					{ID: 1, FileType: models.FileTypeCBZ, FileRole: models.FileRoleMain},
					{ID: 2, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain},
				},
			},
			want: models.FileTypeCBZ,
		},
		{
			name: "skips supplement files",
			book: &models.Book{
				Files: []*models.File{
					{ID: 1, FileType: models.FileTypePDF, FileRole: models.FileRoleSupplement},
					{ID: 2, FileType: models.FileTypeM4B, FileRole: models.FileRoleMain},
				},
			},
			want: models.FileTypeM4B,
		},
		{
			name: "no files returns empty",
			book: &models.Book{
				Files: []*models.File{},
			},
			want: "",
		},
		{
			name: "only supplement files returns empty",
			book: &models.Book{
				Files: []*models.File{
					{ID: 1, FileType: models.FileTypePDF, FileRole: models.FileRoleSupplement},
				},
			},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, getBookFileType(tc.book))
		})
	}
}

// TestDownload_ShowsAllMainFiles confirms the Download handler renders
// a download link for each main file and excludes supplement files.
func TestDownload_ShowsAllMainFiles(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	e := echo.New()

	// Create admin user
	var roleID int
	err := db.QueryRow("SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?)",
		"download_test_user", "hash", roleID, true)
	require.NoError(t, err)

	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE username = 'download_test_user'").Scan(&userID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)", userID)
	require.NoError(t, err)

	apiKeyService := apikeys.NewService(db)
	apiKey, err := apiKeyService.Create(ctx, userID, "Test Key")
	require.NoError(t, err)

	libraryService := libraries.NewService(db)
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	require.NoError(t, libraryService.CreateLibrary(ctx, library))

	bookDir := filepath.Join(t.TempDir(), "Test Book")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	bookService := books.NewService(db)
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "My Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "My Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create three files: EPUB (main), M4B (main), PDF (supplement)
	epubFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath.Join(bookDir, "my-book.epub"),
		FilesizeBytes: 1024 * 1024, // 1 MB
	}
	_, err = db.NewInsert().Model(epubFile).Exec(ctx)
	require.NoError(t, err)

	m4bFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath.Join(bookDir, "my-book.m4b"),
		FilesizeBytes: 50 * 1024 * 1024, // 50 MB
	}
	_, err = db.NewInsert().Model(m4bFile).Exec(ctx)
	require.NoError(t, err)

	supplementFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypePDF,
		FileRole:      models.FileRoleSupplement,
		Filepath:      filepath.Join(bookDir, "supplement.pdf"),
		FilesizeBytes: 512 * 1024, // 512 KB
	}
	_, err = db.NewInsert().Model(supplementFile).Exec(ctx)
	require.NoError(t, err)

	h := &handler{
		db:             db,
		bookService:    bookService,
		libraryService: libraryService,
	}

	apiKeyCtx := context.WithValue(ctx, contextKeyAPIKey, apiKey)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(apiKeyCtx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiKey", "bookId")
	c.SetParamValues(apiKey.Key, strconv.Itoa(book.ID))

	require.NoError(t, h.Download(c))

	body := rec.Body.String()

	// Should show download links for both main files
	assert.Contains(t, body, "EPUB", "page shows EPUB type badge")
	assert.Contains(t, body, "M4B", "page shows M4B type badge")

	// Should have download links for each file
	assert.Contains(t, body, "/file/"+strconv.Itoa(epubFile.ID), "page has EPUB download link")
	assert.Contains(t, body, "/file/"+strconv.Itoa(m4bFile.ID), "page has M4B download link")

	// Should NOT show the supplement file
	assert.NotContains(t, body, "/file/"+strconv.Itoa(supplementFile.ID), "supplement file is excluded")

	// File sizes should be shown
	assert.Contains(t, body, "1.0 MB", "EPUB file size shown")
	assert.Contains(t, body, "50.0 MB", "M4B file size shown")
}

// TestDownload_SingleFileStillWorks confirms that a book with a single
// main file renders normally with its download link.
func TestDownload_SingleFileStillWorks(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	e := echo.New()

	var roleID int
	err := db.QueryRow("SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?)",
		"single_file_user", "hash", roleID, true)
	require.NoError(t, err)

	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE username = 'single_file_user'").Scan(&userID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)", userID)
	require.NoError(t, err)

	apiKeyService := apikeys.NewService(db)
	apiKey, err := apiKeyService.Create(ctx, userID, "Test Key")
	require.NoError(t, err)

	libraryService := libraries.NewService(db)
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	require.NoError(t, libraryService.CreateLibrary(ctx, library))

	bookDir := filepath.Join(t.TempDir(), "Single File Book")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	bookService := books.NewService(db)
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Single File Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Single File Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	epubFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath.Join(bookDir, "book.epub"),
		FilesizeBytes: 2 * 1024 * 1024,
	}
	_, err = db.NewInsert().Model(epubFile).Exec(ctx)
	require.NoError(t, err)

	h := &handler{
		db:             db,
		bookService:    bookService,
		libraryService: libraryService,
	}

	apiKeyCtx := context.WithValue(ctx, contextKeyAPIKey, apiKey)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(apiKeyCtx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiKey", "bookId")
	c.SetParamValues(apiKey.Key, strconv.Itoa(book.ID))

	require.NoError(t, h.Download(c))

	body := rec.Body.String()

	// Should show the file's download link
	assert.Contains(t, body, "/file/"+strconv.Itoa(epubFile.ID), "single file has download link")
	assert.Contains(t, body, "EPUB", "shows file type")
	assert.Contains(t, body, "2.0 MB", "shows file size")
}

// TestDownload_KoboGetsKepubLinksForEpubAndCbz confirms that Kobo
// devices see KePub download links for EPUB and CBZ files, but regular
// links for other file types.
func TestDownload_KoboGetsKepubLinksForEpubAndCbz(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	e := echo.New()

	var roleID int
	err := db.QueryRow("SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?)",
		"kobo_test_user", "hash", roleID, true)
	require.NoError(t, err)

	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE username = 'kobo_test_user'").Scan(&userID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)", userID)
	require.NoError(t, err)

	apiKeyService := apikeys.NewService(db)
	apiKey, err := apiKeyService.Create(ctx, userID, "Test Key")
	require.NoError(t, err)

	libraryService := libraries.NewService(db)
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	require.NoError(t, libraryService.CreateLibrary(ctx, library))

	bookDir := filepath.Join(t.TempDir(), "Kobo Book")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	bookService := books.NewService(db)
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Kobo Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Kobo Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	epubFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath.Join(bookDir, "book.epub"),
		FilesizeBytes: 1024,
	}
	_, err = db.NewInsert().Model(epubFile).Exec(ctx)
	require.NoError(t, err)

	m4bFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath.Join(bookDir, "book.m4b"),
		FilesizeBytes: 1024,
	}
	_, err = db.NewInsert().Model(m4bFile).Exec(ctx)
	require.NoError(t, err)

	h := &handler{
		db:             db,
		bookService:    bookService,
		libraryService: libraryService,
	}

	apiKeyCtx := context.WithValue(ctx, contextKeyAPIKey, apiKey)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; U; Android 2.0; Kobo Touch)")
	req = req.WithContext(apiKeyCtx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiKey", "bookId")
	c.SetParamValues(apiKey.Key, strconv.Itoa(book.ID))

	require.NoError(t, h.Download(c))

	body := rec.Body.String()

	// EPUB should get /kepub link on Kobo
	assert.Contains(t, body, "/file/"+strconv.Itoa(epubFile.ID)+"/kepub", "EPUB gets KePub link on Kobo")
	// M4B should NOT get /kepub link (not a supported kepub format)
	assert.NotContains(t, body, "/file/"+strconv.Itoa(m4bFile.ID)+"/kepub", "M4B does not get KePub link")
	assert.Contains(t, body, "/file/"+strconv.Itoa(m4bFile.ID), "M4B still gets a regular download link")
}

// TestDownload_ShowsFileNames confirms the Download handler shows file
// names (falling back to book title when file has no name).
func TestDownload_ShowsFileNames(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()
	e := echo.New()

	var roleID int
	err := db.QueryRow("SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?)",
		"filename_test_user", "hash", roleID, true)
	require.NoError(t, err)

	var userID int
	err = db.QueryRow("SELECT id FROM users WHERE username = 'filename_test_user'").Scan(&userID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, "INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)", userID)
	require.NoError(t, err)

	apiKeyService := apikeys.NewService(db)
	apiKey, err := apiKeyService.Create(ctx, userID, "Test Key")
	require.NoError(t, err)

	libraryService := libraries.NewService(db)
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	require.NoError(t, libraryService.CreateLibrary(ctx, library))

	bookDir := filepath.Join(t.TempDir(), "Named Files")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

	bookService := books.NewService(db)
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "The Book Title",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book Title, The",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// File with a custom name
	fileName := "Abridged Edition"
	namedFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath.Join(bookDir, "abridged.epub"),
		FilesizeBytes: 1024,
		Name:          &fileName,
	}
	_, err = db.NewInsert().Model(namedFile).Exec(ctx)
	require.NoError(t, err)

	// File without a name (should fall back to book title)
	unnamedFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath.Join(bookDir, "unnamed.m4b"),
		FilesizeBytes: 1024,
	}
	_, err = db.NewInsert().Model(unnamedFile).Exec(ctx)
	require.NoError(t, err)

	h := &handler{
		db:             db,
		bookService:    bookService,
		libraryService: libraryService,
	}

	apiKeyCtx := context.WithValue(ctx, contextKeyAPIKey, apiKey)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(apiKeyCtx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("apiKey", "bookId")
	c.SetParamValues(apiKey.Key, strconv.Itoa(book.ID))

	require.NoError(t, h.Download(c))

	body := rec.Body.String()

	// The named file shows its custom name
	assert.Contains(t, body, "Abridged Edition", "named file shows its name")
	// The unnamed file falls back to book title
	assert.Contains(t, body, "The Book Title", "unnamed file falls back to book title")
}
