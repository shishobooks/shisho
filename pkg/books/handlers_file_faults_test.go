package books

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// servingFixture is one book in one library, a user who can read it, and the
// books routes with a cache directory the test can reach into.
type servingFixture struct {
	t        *testing.T
	db       *bun.DB
	e        *echo.Echo
	user     *models.User
	book     *models.Book
	dir      string
	cacheDir string
}

func newServingFixture(t *testing.T, title string) *servingFixture {
	t.Helper()
	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	book.Title = title
	_, err := db.NewUpdate().Model(book).Column("title").WherePK().Exec(context.Background())
	require.NoError(t, err)
	f := &servingFixture{t: t, db: db, user: setupTestUser(t, db, library.ID, true), book: book, dir: t.TempDir()}
	f.e = setupTestServerWithConfig(t, db, func(cfg *config.Config) { f.cacheDir = cfg.CacheDir })
	return f
}

func (f *servingFixture) addFile(fileType, path string) *models.File {
	f.t.Helper()
	return setupTestFile(f.t, f.db, f.book, fileType, path)
}

func (f *servingFixture) get(url string, headers ...string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return executeRequestWithUser(f.t, f.e, req, f.user)
}

func fileURL(file *models.File, suffix string) string {
	return "/books/files/" + strconv.Itoa(file.ID) + suffix
}

// lockFile removes every permission bit from path, so it still stats but
// cannot be opened, and restores them when the test ends.
func lockFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
}

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}
}

// assertServerFault checks the whole failed response: a 500 with the JSON
// error body, and none of the headers the successful response would carry.
func assertServerFault(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusInternalServerError, rr.Code, rr.Body.String())
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.NotContains(t, rr.Header().Get("Cache-Control"), "immutable")
	assert.Empty(t, rr.Header().Get("Content-Disposition"))
	assert.Contains(t, rr.Body.String(), `"code":"internal_server_error"`)
}

// cachedPages returns the page images the page cache has written.
func cachedPages(t *testing.T, cacheDir string) []string {
	t.Helper()
	var pages []string
	require.NoError(t, filepath.WalkDir(cacheDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), "page_") {
			pages = append(pages, path)
		}
		return nil
	}))
	require.NotEmpty(t, pages)
	return pages
}

// A served file that exists but cannot be opened is a server fault on every
// books route, never Echo's generic 404, and the failure carries none of the
// success headers.
func TestFileServing_UnreadableFileIsServerError(t *testing.T) {
	t.Parallel()
	skipIfRoot(t)

	t.Run("file cover", func(t *testing.T) {
		t.Parallel()
		f := newServingFixture(t, "Cover Book")
		file := f.addFile(models.FileTypeEPUB, testgen.GenerateEPUB(t, f.dir, "book.epub", testgen.EPUBOptions{Title: "Cover Book"}))
		coverName := "book.epub.cover.jpg"
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, coverName), []byte("jpeg"), 0o644))
		_, err := f.db.NewUpdate().Model(file).Set("cover_image_filename = ?", coverName).WherePK().Exec(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, f.get(fileURL(file, "/cover")).Code)

		lockFile(t, filepath.Join(f.dir, coverName))
		assertServerFault(t, f.get(fileURL(file, "/cover")))
	})

	t.Run("download from the cache", func(t *testing.T) {
		t.Parallel()
		f := newServingFixture(t, "Download Book")
		file := f.addFile(models.FileTypeEPUB, testgen.GenerateEPUB(t, f.dir, "book.epub", testgen.EPUBOptions{Title: "Download Book"}))
		require.Equal(t, http.StatusOK, f.get(fileURL(file, "/download")).Code)

		lockFile(t, filepath.Join(f.cacheDir, strconv.Itoa(file.ID)+".epub"))
		assertServerFault(t, f.get(fileURL(file, "/download")))
	})

	t.Run("download original", func(t *testing.T) {
		t.Parallel()
		f := newServingFixture(t, "Original Book")
		path := testgen.GenerateEPUB(t, f.dir, "book.epub", testgen.EPUBOptions{Title: "Original Book"})
		file := f.addFile(models.FileTypeEPUB, path)

		lockFile(t, path)
		assertServerFault(t, f.get(fileURL(file, "/download/original")))
	})

	t.Run("kepub from the cache", func(t *testing.T) {
		t.Parallel()
		f := newServingFixture(t, "Kepub Book")
		file := f.addFile(models.FileTypeEPUB, testgen.GenerateEPUB(t, f.dir, "book.epub", testgen.EPUBOptions{Title: "Kepub Book"}))
		require.Equal(t, http.StatusOK, f.get(fileURL(file, "/download/kepub")).Code)

		lockFile(t, filepath.Join(f.cacheDir, strconv.Itoa(file.ID)+".kepub.epub"))
		assertServerFault(t, f.get(fileURL(file, "/download/kepub")))
	})

	t.Run("page from the cache", func(t *testing.T) {
		t.Parallel()
		f := newServingFixture(t, "Page Book")
		path := filepath.Join(f.dir, "book.cbz")
		createTestCBZWithPages(t, path, 3)
		file := f.addFile(models.FileTypeCBZ, path)
		require.Equal(t, http.StatusOK, f.get(fileURL(file, "/page/0")).Code)

		for _, page := range cachedPages(t, f.cacheDir) {
			lockFile(t, page)
		}
		assertServerFault(t, f.get(fileURL(file, "/page/0")))
	})

	for _, rangeHeader := range []string{"", "bytes=0-99", "bytes=-500"} {
		t.Run("stream with Range "+rangeHeader, func(t *testing.T) {
			t.Parallel()
			f := newServingFixture(t, "Stream Book")
			path := createTestM4BFile(t, 1000)
			file := f.addFile(models.FileTypeM4B, path)

			lockFile(t, path)
			assertServerFault(t, f.get(fileURL(file, "/stream"), "Range", rangeHeader))
		})
	}
}

// Range requests on the audio stream are answered by http.ServeContent: an
// open-ended or suffix range and an end past the size are 206, a start past
// the size is 416, and a malformed bytes range is 416 as Go answers it.
func TestStreamFile_RangeHandledByServeContent(t *testing.T) {
	t.Parallel()
	f := newServingFixture(t, "Stream Book")
	path := createTestM4BFile(t, 5000)
	file := f.addFile(models.FileTypeM4B, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	tests := []struct {
		rangeHeader  string
		status       int
		contentRange string
		body         []byte
	}{
		{"bytes=0-", http.StatusPartialContent, "bytes 0-4999/5000", data},
		{"bytes=4000-9999", http.StatusPartialContent, "bytes 4000-4999/5000", data[4000:]},
		{"bytes=-500", http.StatusPartialContent, "bytes 4500-4999/5000", data[4500:]},
		{"bytes=5000-", http.StatusRequestedRangeNotSatisfiable, "bytes */5000", nil},
		{"bytes=abc", http.StatusRequestedRangeNotSatisfiable, "", nil},
		{"items=0-10", http.StatusOK, "", data},
	}
	for _, tt := range tests {
		t.Run(tt.rangeHeader, func(t *testing.T) {
			t.Parallel()
			rr := f.get(fileURL(file, "/stream"), "Range", tt.rangeHeader)
			require.Equal(t, tt.status, rr.Code, rr.Body.String())
			assert.Equal(t, tt.contentRange, rr.Header().Get("Content-Range"))
			if tt.body != nil {
				assert.Equal(t, tt.body, rr.Body.Bytes())
				assert.Equal(t, "audio/mp4", rr.Header().Get("Content-Type"))
				assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
				assert.NotEmpty(t, rr.Header().Get("Last-Modified"))
			}
		})
	}
}

// The M4B check runs after the library access check, so a user without
// access to the library cannot tell an M4B from any other file.
func TestStreamFile_AccessCheckedBeforeFileType(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	file := setupTestFile(t, db, book, models.FileTypeEPUB, createTestEPUBFile(t))
	outsider := setupTestUser(t, db, library.ID, false)
	e := setupTestServer(t, db)

	rr := executeRequestWithUser(t, e, httptest.NewRequest(http.MethodGet, fileURL(file, "/stream"), nil), outsider)
	assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

// Downloads are typed from the file type, not the host's mime table, which
// in the Alpine image has no entry for .epub, .cbz, or .m4b. The original
// cases use an unknown extension, so they fail on any host if the type came
// from the extension.
func TestDownload_ContentTypeFromFileType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileType string
		suffix   string
		want     string
	}{
		{"original epub", models.FileTypeEPUB, "/download/original", "application/epub+zip"},
		{"original cbz", models.FileTypeCBZ, "/download/original", "application/vnd.comicbook+zip"},
		{"original m4b", models.FileTypeM4B, "/download/original", "audio/mp4"},
		{"generated epub", models.FileTypeEPUB, "/download", "application/epub+zip"},
		{"generated cbz", models.FileTypeCBZ, "/download", "application/vnd.comicbook+zip"},
		{"kepub", models.FileTypeEPUB, "/download/kepub", "application/epub+zip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newServingFixture(t, "Typed Book")
			var path string
			switch tt.fileType {
			case models.FileTypeEPUB:
				path = testgen.GenerateEPUB(t, f.dir, "book.epub", testgen.EPUBOptions{Title: "Typed Book"})
			case models.FileTypeCBZ:
				path = testgen.GenerateCBZ(t, f.dir, "book.cbz", testgen.CBZOptions{Title: "Typed Book"})
			}
			if tt.suffix == "/download/original" {
				// An extension no mime table knows, so only the file type
				// can produce the expected Content-Type.
				path = filepath.Join(f.dir, "book.bin")
				require.NoError(t, os.WriteFile(path, []byte("original bytes"), 0o644))
			}
			file := f.addFile(tt.fileType, path)

			rr := f.get(fileURL(file, tt.suffix))
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assert.Equal(t, tt.want, rr.Header().Get("Content-Type"))
		})
	}
}

// A title with quotes and non-ASCII letters is sent as an escaped ASCII
// filename plus the UTF-8 filename* form.
func TestDownload_FilenameEscaping(t *testing.T) {
	t.Parallel()
	f := newServingFixture(t, `Title "Quoted" Ünïcode`)
	file := f.addFile(models.FileTypeEPUB, testgen.GenerateEPUB(t, f.dir, "book.epub", testgen.EPUBOptions{Title: "x"}))

	rr := f.get(fileURL(file, "/download"))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	disposition := rr.Header().Get("Content-Disposition")
	assert.Contains(t, disposition, `attachment; filename="`)
	assert.Contains(t, disposition, "; filename*=UTF-8''")
	assert.Contains(t, disposition, "%C3%9Cn%C3%AFcode")
}

// A page past the end is a 404 even when the file has no stored page count,
// so the page cache is the first to know the page does not exist.
func TestGetPage_OutOfRangeWithoutPageCountIsNotFound(t *testing.T) {
	t.Parallel()

	for _, fileType := range []string{models.FileTypeCBZ, models.FileTypePDF} {
		t.Run(fileType, func(t *testing.T) {
			t.Parallel()
			f := newServingFixture(t, "Paged")
			var path string
			if fileType == models.FileTypeCBZ {
				path = filepath.Join(f.dir, "book.cbz")
				createTestCBZWithPages(t, path, 3)
			} else {
				path = testgen.GeneratePDF(t, f.dir, "book.pdf", testgen.PDFOptions{PageCount: 2})
			}
			file := f.addFile(fileType, path)
			require.Nil(t, file.PageCount)

			requireErrcodesNotFound(t, f.get(fileURL(file, "/page/99")), "Page not found.")
		})
	}
}

// The web download answers a type with no generator with a 422: the user
// can use Download Original instead.
func TestDownload_NoGeneratorIsInvalidState(t *testing.T) {
	t.Parallel()
	f := newServingFixture(t, "Plugin Format")
	path := filepath.Join(f.dir, "book.fb2")
	require.NoError(t, os.WriteFile(path, []byte("fb2"), 0o644))
	file := f.addFile("fb2", path)

	rr := f.get(fileURL(file, "/download"))
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), `"invalid_state"`)
}

// A Range that starts past the end is a 416 that does not go out as an
// attachment, so a download manager cannot save the error as the book.
func TestDownload_UnsatisfiableRangeIsNotAnAttachment(t *testing.T) {
	t.Parallel()
	f := newServingFixture(t, "Range Book")
	file := f.addFile(models.FileTypeEPUB, createTestEPUBFile(t))

	rr := f.get(fileURL(file, "/download/original"), "Range", "bytes=5000-")
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, rr.Code)
	assert.Empty(t, rr.Header().Get("Content-Disposition"))
}
