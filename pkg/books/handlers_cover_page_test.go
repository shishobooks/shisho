package books

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestCBZWithPages creates a test CBZ file with the specified number of pages.
//
//nolint:unparam // numPages is always 5 in tests but kept for clarity and potential future use
func createTestCBZWithPages(t *testing.T, path string, numPages int) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)

	// Create test images
	img := image.NewRGBA(image.Rect(0, 0, 100, 150))
	c := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	for y := 0; y < 150; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	imgData := buf.Bytes()

	// Add pages
	for i := 0; i < numPages; i++ {
		pageWriter, err := w.Create(filepath.Join("images", "page_"+strconv.Itoa(i)+".jpg"))
		require.NoError(t, err)
		_, err = pageWriter.Write(imgData)
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())
}

func TestUpdateFileCoverPage(t *testing.T) {
	t.Run("sets cover page and extracts cover image", func(t *testing.T) {
		db := setupTestDB(t)
		ctx := context.Background()
		cfg := &config.Config{CacheDir: t.TempDir()}
		e := echo.New()
		bookService := NewService(db)
		pageCache := cbzpages.NewCache(cfg.CacheDir)

		h := &handler{
			bookService: bookService,
			pageCache:   pageCache,
		}

		// Create test library
		library := &models.Library{
			Name:                     "Test Library",
			CoverAspectRatio:         "book",
			DownloadFormatPreference: models.DownloadFormatOriginal,
		}
		_, err := db.NewInsert().Model(library).Exec(ctx)
		require.NoError(t, err)

		// Create book directory
		bookDir := filepath.Join(t.TempDir(), "Test Book")
		err = os.MkdirAll(bookDir, 0755)
		require.NoError(t, err)

		// Create test CBZ file with 5 pages
		cbzPath := filepath.Join(bookDir, "test.cbz")
		createTestCBZWithPages(t, cbzPath, 5)

		// Create book
		book := &models.Book{
			LibraryID:       library.ID,
			Title:           "Test Book",
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       "Test Book",
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        bookDir,
		}
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		// Create file
		pageCount := 5
		file := &models.File{
			LibraryID:     library.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeCBZ,
			FileRole:      models.FileRoleMain,
			Filepath:      cbzPath,
			FilesizeBytes: 1000,
			PageCount:     &pageCount,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)

		// Make request to set cover page to page 2 (0-indexed)
		payload := map[string]int{"page": 2}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.Itoa(file.ID))

		err = h.updateFileCoverPage(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify file was updated
		var updatedFile models.File
		err = db.NewSelect().Model(&updatedFile).Where("id = ?", file.ID).Scan(ctx)
		require.NoError(t, err)

		require.NotNil(t, updatedFile.CoverPage)
		assert.Equal(t, 2, *updatedFile.CoverPage)
		assert.NotNil(t, updatedFile.CoverMimeType)
		assert.NotNil(t, updatedFile.CoverSource)
		assert.Equal(t, models.DataSourceManual, *updatedFile.CoverSource)
		assert.NotNil(t, updatedFile.CoverImagePath)

		// Verify cover file was created
		coverPath := filepath.Join(bookDir, *updatedFile.CoverImagePath)
		_, err = os.Stat(coverPath)
		assert.NoError(t, err, "Cover file should exist at %s", coverPath)
	})

	t.Run("returns 400 for invalid page number", func(t *testing.T) {
		db := setupTestDB(t)
		ctx := context.Background()
		cfg := &config.Config{CacheDir: t.TempDir()}
		e := echo.New()
		bookService := NewService(db)
		pageCache := cbzpages.NewCache(cfg.CacheDir)

		h := &handler{
			bookService: bookService,
			pageCache:   pageCache,
		}

		// Create test library
		library := &models.Library{
			Name:                     "Test Library",
			CoverAspectRatio:         "book",
			DownloadFormatPreference: models.DownloadFormatOriginal,
		}
		_, err := db.NewInsert().Model(library).Exec(ctx)
		require.NoError(t, err)

		// Create book directory
		bookDir := filepath.Join(t.TempDir(), "Test Book")
		err = os.MkdirAll(bookDir, 0755)
		require.NoError(t, err)

		// Create test CBZ file with 5 pages
		cbzPath := filepath.Join(bookDir, "test.cbz")
		createTestCBZWithPages(t, cbzPath, 5)

		// Create book
		book := &models.Book{
			LibraryID:       library.ID,
			Title:           "Test Book",
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       "Test Book",
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        bookDir,
		}
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		// Create file
		pageCount := 5
		file := &models.File{
			LibraryID:     library.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeCBZ,
			FileRole:      models.FileRoleMain,
			Filepath:      cbzPath,
			FilesizeBytes: 1000,
			PageCount:     &pageCount,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)

		// Request page 10 which is out of bounds
		payload := map[string]int{"page": 10}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.Itoa(file.ID))

		err = h.updateFileCoverPage(c)
		require.Error(t, err)
	})

	t.Run("returns 400 for negative page number", func(t *testing.T) {
		db := setupTestDB(t)
		ctx := context.Background()
		cfg := &config.Config{CacheDir: t.TempDir()}
		e := echo.New()
		bookService := NewService(db)
		pageCache := cbzpages.NewCache(cfg.CacheDir)

		h := &handler{
			bookService: bookService,
			pageCache:   pageCache,
		}

		// Create test library
		library := &models.Library{
			Name:                     "Test Library",
			CoverAspectRatio:         "book",
			DownloadFormatPreference: models.DownloadFormatOriginal,
		}
		_, err := db.NewInsert().Model(library).Exec(ctx)
		require.NoError(t, err)

		// Create book directory
		bookDir := filepath.Join(t.TempDir(), "Test Book")
		err = os.MkdirAll(bookDir, 0755)
		require.NoError(t, err)

		// Create test CBZ file with 5 pages
		cbzPath := filepath.Join(bookDir, "test.cbz")
		createTestCBZWithPages(t, cbzPath, 5)

		// Create book
		book := &models.Book{
			LibraryID:       library.ID,
			Title:           "Test Book",
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       "Test Book",
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        bookDir,
		}
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		// Create file
		pageCount := 5
		file := &models.File{
			LibraryID:     library.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeCBZ,
			FileRole:      models.FileRoleMain,
			Filepath:      cbzPath,
			FilesizeBytes: 1000,
			PageCount:     &pageCount,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)

		// Request page -1 which is invalid
		payload := map[string]int{"page": -1}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.Itoa(file.ID))

		err = h.updateFileCoverPage(c)
		require.Error(t, err)
	})

	t.Run("returns 400 for non-CBZ file", func(t *testing.T) {
		db := setupTestDB(t)
		ctx := context.Background()
		cfg := &config.Config{CacheDir: t.TempDir()}
		e := echo.New()
		bookService := NewService(db)
		pageCache := cbzpages.NewCache(cfg.CacheDir)

		h := &handler{
			bookService: bookService,
			pageCache:   pageCache,
		}

		// Create test library
		library := &models.Library{
			Name:                     "Test Library",
			CoverAspectRatio:         "book",
			DownloadFormatPreference: models.DownloadFormatOriginal,
		}
		_, err := db.NewInsert().Model(library).Exec(ctx)
		require.NoError(t, err)

		// Create book directory
		bookDir := filepath.Join(t.TempDir(), "Test Book")
		err = os.MkdirAll(bookDir, 0755)
		require.NoError(t, err)

		// Create a fake EPUB file
		epubPath := filepath.Join(bookDir, "test.epub")
		err = os.WriteFile(epubPath, []byte("fake epub content"), 0644)
		require.NoError(t, err)

		// Create book
		book := &models.Book{
			LibraryID:       library.ID,
			Title:           "Test Book",
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       "Test Book",
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        bookDir,
		}
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		// Create EPUB file record
		file := &models.File{
			LibraryID:     library.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			Filepath:      epubPath,
			FilesizeBytes: 17,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)

		// Try to set cover page for EPUB file
		payload := map[string]int{"page": 0}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.Itoa(file.ID))

		err = h.updateFileCoverPage(c)
		require.Error(t, err)
	})

	t.Run("returns 404 for non-existent file", func(t *testing.T) {
		db := setupTestDB(t)
		cfg := &config.Config{CacheDir: t.TempDir()}
		e := echo.New()
		bookService := NewService(db)
		pageCache := cbzpages.NewCache(cfg.CacheDir)

		h := &handler{
			bookService: bookService,
			pageCache:   pageCache,
		}

		// Request for non-existent file
		payload := map[string]int{"page": 0}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("99999")

		err := h.updateFileCoverPage(c)
		require.Error(t, err)
	})

	t.Run("returns 400 for file with no page count", func(t *testing.T) {
		db := setupTestDB(t)
		ctx := context.Background()
		cfg := &config.Config{CacheDir: t.TempDir()}
		e := echo.New()
		bookService := NewService(db)
		pageCache := cbzpages.NewCache(cfg.CacheDir)

		h := &handler{
			bookService: bookService,
			pageCache:   pageCache,
		}

		// Create test library
		library := &models.Library{
			Name:                     "Test Library",
			CoverAspectRatio:         "book",
			DownloadFormatPreference: models.DownloadFormatOriginal,
		}
		_, err := db.NewInsert().Model(library).Exec(ctx)
		require.NoError(t, err)

		// Create book directory
		bookDir := filepath.Join(t.TempDir(), "Test Book")
		err = os.MkdirAll(bookDir, 0755)
		require.NoError(t, err)

		// Create test CBZ file
		cbzPath := filepath.Join(bookDir, "test.cbz")
		createTestCBZWithPages(t, cbzPath, 5)

		// Create book
		book := &models.Book{
			LibraryID:       library.ID,
			Title:           "Test Book",
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       "Test Book",
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        bookDir,
		}
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		// Create file WITHOUT page count
		file := &models.File{
			LibraryID:     library.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeCBZ,
			FileRole:      models.FileRoleMain,
			Filepath:      cbzPath,
			FilesizeBytes: 1000,
			PageCount:     nil, // No page count
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)

		// Request should fail since page count is nil
		payload := map[string]int{"page": 0}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(strconv.Itoa(file.ID))

		err = h.updateFileCoverPage(c)
		require.Error(t, err)
	})
}
