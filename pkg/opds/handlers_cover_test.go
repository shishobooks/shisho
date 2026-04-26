package opds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupCoverTest builds a fully wired handler against a real in-memory
// DB and seeds one library, one book, and one file with a cover image
// on disk. Returns the handler, library ID, book ID, cover bytes.
func setupCoverTest(t *testing.T) (*handler, int, int, []byte) {
	t.Helper()

	db := setupOPDSDB(t)

	dir := t.TempDir()
	bookPath := filepath.Join(dir, "book.epub")
	require.NoError(t, os.WriteFile(bookPath, []byte("epub"), 0o644))

	coverFilename := "book.epub.cover.jpg"
	coverBytes := []byte("\xff\xd8\xff\xe0fakejpeg")
	require.NoError(t, os.WriteFile(filepath.Join(dir, coverFilename), coverBytes, 0o644))

	ctx := context.Background()
	lib := &models.Library{
		Name:                     "Lib",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Test",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        dir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:          lib.ID,
		BookID:             book.ID,
		Filepath:           bookPath,
		FileType:           models.FileTypeEPUB,
		FilesizeBytes:      4,
		CoverImageFilename: &coverFilename,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	h := &handler{
		bookService:    books.NewService(db),
		libraryService: libraries.NewService(db),
	}
	return h, lib.ID, book.ID, coverBytes
}

func newCoverRequest(t *testing.T, h *handler, bookID int, user *models.User) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/opds/v1/books/"+strconv.Itoa(bookID)+"/cover", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/opds/v1/books/:id/cover")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(bookID))
	if user != nil {
		c.Set("user", user)
	}
	return rec, h.bookCover(c)
}

// TestBookCover_ServesImageWhenAuthorized confirms the happy path:
// authenticated user with library access gets the cover bytes back
// with the no-cache header that prevents stale clients from serving
// an outdated image after a re-cover.
func TestBookCover_ServesImageWhenAuthorized(t *testing.T) {
	t.Parallel()

	h, libID, bookID, want := setupCoverTest(t)

	user := &models.User{
		ID:       1,
		Username: "alice",
		IsActive: true,
		LibraryAccess: []*models.UserLibraryAccess{
			{LibraryID: &libID},
		},
	}

	rec, err := newCoverRequest(t, h, bookID, user)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, want, rec.Body.Bytes())
	assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
}

// TestBookCover_ForbiddenWithoutLibraryAccess confirms the access-control
// branch: a user with no entry for this library gets 403 even though
// they're authenticated. This is the security-relevant branch — without
// it the OPDS feed would leak covers across libraries.
func TestBookCover_ForbiddenWithoutLibraryAccess(t *testing.T) {
	t.Parallel()

	h, _, bookID, _ := setupCoverTest(t)

	otherLib := 999
	user := &models.User{
		ID:       2,
		Username: "bob",
		IsActive: true,
		LibraryAccess: []*models.UserLibraryAccess{
			{LibraryID: &otherLib},
		},
	}

	_, err := newCoverRequest(t, h, bookID, user)
	require.Error(t, err)
	ec, ok := err.(*errcodes.Error)
	require.True(t, ok, "expected *errcodes.Error, got %T: %v", err, err)
	assert.Equal(t, http.StatusForbidden, ec.HTTPCode)
}

// TestBookCover_NotFoundWhenBookHasNoCover confirms the no-cover branch
// returns 404 rather than serving an empty body or some default. A book
// with files but no CoverImageFilename should be unambiguous to clients.
func TestBookCover_NotFoundWhenBookHasNoCover(t *testing.T) {
	t.Parallel()

	db := setupOPDSDB(t)
	ctx := context.Background()

	lib := &models.Library{
		Name:                     "Lib",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "No Cover",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "No Cover",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/tmp/nocover",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     lib.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/nocover/book.epub",
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	h := &handler{
		bookService:    books.NewService(db),
		libraryService: libraries.NewService(db),
	}

	user := &models.User{
		ID:            3,
		Username:      "carol",
		IsActive:      true,
		LibraryAccess: []*models.UserLibraryAccess{{LibraryID: &lib.ID}},
	}

	_, err = newCoverRequest(t, h, book.ID, user)
	require.Error(t, err)
	ec, ok := err.(*errcodes.Error)
	require.True(t, ok, "expected *errcodes.Error, got %T: %v", err, err)
	assert.Equal(t, http.StatusNotFound, ec.HTTPCode)
}
