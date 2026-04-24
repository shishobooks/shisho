package series

import (
	"context"
	"database/sql"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupSeriesTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// seedSeriesWithCover creates a library, series, book, book_series join row,
// and a file with a real cover image on disk. Returns the series ID.
func seedSeriesWithCover(ctx context.Context, t *testing.T, db *bun.DB) int {
	t.Helper()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	seriesRecord := &models.Series{
		LibraryID:      library.ID,
		Name:           "Test Series",
		NameSource:     string(models.DataSourceFilepath),
		SortName:       "Test Series",
		SortNameSource: string(models.DataSourceFilepath),
	}
	_, err = db.NewInsert().Model(seriesRecord).Exec(ctx)
	require.NoError(t, err)

	bookDir := filepath.Join(t.TempDir(), "Test Book")
	require.NoError(t, os.MkdirAll(bookDir, 0o755))

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

	seriesNumber := 1.0
	bookSeries := &models.BookSeries{
		BookID:       book.ID,
		SeriesID:     seriesRecord.ID,
		SeriesNumber: &seriesNumber,
		SortOrder:    1,
	}
	_, err = db.NewInsert().Model(bookSeries).Exec(ctx)
	require.NoError(t, err)

	filePath := filepath.Join(bookDir, "test.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0o644))

	coverFilename := "test.epub.cover.jpg"
	coverPath := filepath.Join(bookDir, coverFilename)
	coverFile, err := os.Create(coverPath)
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(coverFile, image.NewRGBA(image.Rect(0, 0, 10, 10)), nil))
	require.NoError(t, coverFile.Close())

	mimeType := "image/jpeg"
	file := &models.File{
		LibraryID:          library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           filePath,
		FilesizeBytes:      1000,
		CoverImageFilename: &coverFilename,
		CoverMimeType:      &mimeType,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	return seriesRecord.ID
}

func TestSeriesCover_SetsCacheControlPrivateNoCache(t *testing.T) {
	t.Parallel()

	db := setupSeriesTestDB(t)
	ctx := context.Background()
	e := echo.New()
	h := &handler{
		seriesService:  NewService(db),
		bookService:    books.NewService(db),
		libraryService: libraries.NewService(db),
	}

	seriesID := seedSeriesWithCover(ctx, t, db)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(seriesID))

	require.NoError(t, h.seriesCover(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
	assert.NotEmpty(t, rec.Header().Get("ETag"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestSeriesCover_Returns304WhenIfNoneMatchMatches(t *testing.T) {
	t.Parallel()

	db := setupSeriesTestDB(t)
	ctx := context.Background()
	e := echo.New()
	h := &handler{
		seriesService:  NewService(db),
		bookService:    books.NewService(db),
		libraryService: libraries.NewService(db),
	}

	seriesID := seedSeriesWithCover(ctx, t, db)

	// First GET: capture ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("id")
	c1.SetParamValues(strconv.Itoa(seriesID))
	require.NoError(t, h.seriesCover(c1))
	etag := rec1.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// Second GET with If-None-Match.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(strconv.Itoa(seriesID))
	require.NoError(t, h.seriesCover(c2))

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())
	assert.Equal(t, etag, rec2.Header().Get("ETag"))
	assert.Equal(t, "private, no-cache", rec2.Header().Get("Cache-Control"))
}

func TestSeriesCover_Returns200WhenIfNoneMatchMismatches(t *testing.T) {
	t.Parallel()

	db := setupSeriesTestDB(t)
	ctx := context.Background()
	e := echo.New()
	h := &handler{
		seriesService:  NewService(db),
		bookService:    books.NewService(db),
		libraryService: libraries.NewService(db),
	}

	seriesID := seedSeriesWithCover(ctx, t, db)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", `"stale-etag"`)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(seriesID))
	require.NoError(t, h.seriesCover(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Body.Bytes())
	assert.NotEmpty(t, rec.Header().Get("ETag"))
	assert.NotEqual(t, `"stale-etag"`, rec.Header().Get("ETag"))
}

// Regression: when the series' first book changes to a different book whose
// cover file has an older mtime than the previous first book's cover, a client
// holding the previous cover's ETag must NOT receive 304 — the ETag bakes in
// the selected file's identity, so it bumps even when mtime goes backwards.
func TestSeriesCover_FirstBookChangeInvalidatesEtagEvenWhenNewCoverMtimeIsOlder(t *testing.T) {
	t.Parallel()

	db := setupSeriesTestDB(t)
	ctx := context.Background()
	e := echo.New()
	h := &handler{
		seriesService:  NewService(db),
		bookService:    books.NewService(db),
		libraryService: libraries.NewService(db),
	}

	// Seed with book A as the first book.
	seriesID := seedSeriesWithCover(ctx, t, db)

	// Look up the series' library ID so we can add book B in the same library.
	var seriesRecord models.Series
	require.NoError(t, db.NewSelect().Model(&seriesRecord).Where("id = ?", seriesID).Scan(ctx))

	// Add book B (sorts AFTER A), with its cover file's mtime set OLDER than A's.
	bookBDir := filepath.Join(t.TempDir(), "Book B")
	require.NoError(t, os.MkdirAll(bookBDir, 0o755))

	bookB := &models.Book{
		LibraryID:       seriesRecord.LibraryID,
		Title:           "Book B",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book B",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookBDir,
	}
	_, err := db.NewInsert().Model(bookB).Exec(ctx)
	require.NoError(t, err)

	seriesNumberB := 2.0
	_, err = db.NewInsert().Model(&models.BookSeries{
		BookID:       bookB.ID,
		SeriesID:     seriesID,
		SeriesNumber: &seriesNumberB,
		SortOrder:    2,
	}).Exec(ctx)
	require.NoError(t, err)

	bookBFilePath := filepath.Join(bookBDir, "bookB.epub")
	require.NoError(t, os.WriteFile(bookBFilePath, []byte("fake epub"), 0o644))

	bookBCoverFilename := "bookB.epub.cover.jpg"
	bookBCoverPath := filepath.Join(bookBDir, bookBCoverFilename)
	bookBCoverHandle, err := os.Create(bookBCoverPath)
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(bookBCoverHandle, image.NewRGBA(image.Rect(0, 0, 10, 10)), nil))
	require.NoError(t, bookBCoverHandle.Close())

	mimeType := "image/jpeg"
	bookBFile := &models.File{
		LibraryID:          seriesRecord.LibraryID,
		BookID:             bookB.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           bookBFilePath,
		FilesizeBytes:      1000,
		CoverImageFilename: &bookBCoverFilename,
		CoverMimeType:      &mimeType,
	}
	_, err = db.NewInsert().Model(bookBFile).Exec(ctx)
	require.NoError(t, err)

	// Give book B's cover an OLDER mtime than book A's cover will have.
	oldTime := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(bookBCoverPath, oldTime, oldTime))

	// Request the cover: book A is still first, capture its ETag.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetParamNames("id")
	c1.SetParamValues(strconv.Itoa(seriesID))
	require.NoError(t, h.seriesCover(c1))
	require.Equal(t, http.StatusOK, rec1.Code)
	etagA := rec1.Header().Get("ETag")
	require.NotEmpty(t, etagA)

	// Delete book A → book B becomes the first book.
	var bookABookSeries models.BookSeries
	require.NoError(t, db.NewSelect().Model(&bookABookSeries).
		Where("series_id = ? AND book_id != ?", seriesID, bookB.ID).Scan(ctx))
	_, err = db.NewDelete().Model((*models.Book)(nil)).
		Where("id = ?", bookABookSeries.BookID).Exec(ctx)
	require.NoError(t, err)

	// Client revalidates with book A's ETag — must NOT 304; book B's cover is now served.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-None-Match", etagA)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(strconv.Itoa(seriesID))
	require.NoError(t, h.seriesCover(c2))

	assert.Equal(t, http.StatusOK, rec2.Code,
		"expected 200 after first-book change (ETag must change with file identity, not just mtime)")
	assert.NotEmpty(t, rec2.Body.Bytes())
	etagB := rec2.Header().Get("ETag")
	assert.NotEmpty(t, etagB)
	assert.NotEqual(t, etagA, etagB, "ETag must change when the selected file changes")
	// Sanity check the ETag format encodes the book B file ID.
	assert.Contains(t, etagB, fmt.Sprintf("%d-", bookBFile.ID))
}
