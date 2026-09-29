package ereader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireErrcode(t *testing.T, err error, status int, code, message string) {
	t.Helper()
	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr, "want an errcodes error, got %T: %v", err, err)
	assert.Equal(t, status, ecErr.HTTPCode)
	assert.Equal(t, code, ecErr.Code)
	assert.Equal(t, message, ecErr.Message)
}

// A path ID that does not parse names no row, so every eReader page returns
// the resource's 404, the same as an ID with no row.
func TestEReaderHandlers_NonNumericIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	h := &handler{}

	tests := []struct {
		name     string
		fn       echo.HandlerFunc
		params   []string
		resource string
	}{
		{"all books library", h.LibraryAllBooks, []string{"libraryId", "abc"}, "Library"},
		{"series list library", h.LibrarySeries, []string{"libraryId", "abc"}, "Library"},
		{"series books library", h.SeriesBooks, []string{"libraryId", "abc", "seriesId", "1"}, "Library"},
		{"series books series", h.SeriesBooks, []string{"libraryId", "1", "seriesId", "abc"}, "Series"},
		{"authors library", h.LibraryAuthors, []string{"libraryId", "abc"}, "Library"},
		{"author books library", h.AuthorBooks, []string{"libraryId", "abc", "authorId", "1"}, "Library"},
		{"author books author", h.AuthorBooks, []string{"libraryId", "1", "authorId", "abc"}, "Author"},
		{"search library", h.LibrarySearch, []string{"libraryId", "abc"}, "Library"},
		{"download book", h.Download, []string{"bookId", "abc"}, "Book"},
		{"cover book", h.Cover, []string{"bookId", "abc"}, "Book"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			apikeys.SetKey(c, &apikeys.APIKey{Key: "key"})
			var names, values []string
			for i := 0; i < len(tt.params); i += 2 {
				names = append(names, tt.params[i])
				values = append(values, tt.params[i+1])
			}
			c.SetParamNames(names...)
			c.SetParamValues(values...)

			requireErrcode(t, tt.fn(c), http.StatusNotFound, "not_found", tt.resource+" not found.")
		})
	}
}

// A Book whose only Files are supplements has nothing to download, which the
// download page reports as a missing File rather than a doubled "not found".
func TestDownload_BookWithoutMainFilesReturnsFileNotFound(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	var roleID int
	require.NoError(t, db.QueryRow("SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID))
	user := &models.User{Username: "supplement_only_user", PasswordHash: "hash", RoleID: roleID, IsActive: true}
	_, err := db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO user_library_access (user_id, library_id) VALUES (?, NULL)", user.ID)
	require.NoError(t, err)
	apiKey, err := apikeys.NewService(db).Create(ctx, user.ID, "Test Key")
	require.NoError(t, err)

	lib := &models.Library{Name: "Lib", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err = db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)
	dir := t.TempDir()
	book := &models.Book{LibraryID: lib.ID, Title: "Extras", TitleSource: models.DataSourceFilepath, SortTitle: "Extras", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: dir}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{LibraryID: lib.ID, BookID: book.ID, Filepath: dir + "/notes.pdf", FileType: models.FileTypePDF, FileRole: models.FileRoleSupplement, FilesizeBytes: 4}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	h := &handler{bookService: books.NewService(db)}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := echo.New().NewContext(req, httptest.NewRecorder())
	withKey(c, t, db, apiKey)
	c.SetParamNames("apiKey", "bookId")
	c.SetParamValues(apiKey.Key, strconv.Itoa(book.ID))

	requireErrcode(t, h.Download(c), http.StatusNotFound, "not_found", "File not found.")
}

// An unknown or expired short code returns the errcodes 404 body.
func TestResolveShortURL_UnknownCodeReturnsErrcodesNotFound(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c.SetParamNames("shortCode")
	c.SetParamValues("nope")

	requireErrcode(t, ResolveShortURL(c, apikeys.NewService(db)), http.StatusNotFound, "not_found", "Short URL not found.")
}
