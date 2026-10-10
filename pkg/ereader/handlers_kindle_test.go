package ereader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	kindleUserAgent = "Mozilla/5.0 (X11; U; Linux armv7l like Android; en-us) AppleWebKit/531.2+ (KHTML, like Gecko) Version/5.0 Safari/531.2+ Kindle/3.0+"
	koboUserAgent   = "Mozilla/5.0 (Linux; U; Android 2.0; en-us;) AppleWebKit/538.1 (KHTML, like Gecko) Version/4.0 Mobile Safari/538.1 (Kobo Touch 0373/4.38.21908)"
)

func TestIsKindle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		userAgent string
		want      bool
	}{
		// Firmware 5.x sends this, and the Chromium browser of 5.16.4 and
		// later keeps it by passing it as a launch flag:
		// https://user-agents.net/string/mozilla-5-0-x11-u-linux-armv7l-like-android-en-us-applewebkit-531-2-khtml-like-gecko-version-5-0-safari-533-2-kindle-3-0
		{"kindle 5.x firmware, webkit and chromium browsers", "Mozilla/5.0 (X11; U; Linux armv7l like Android; en-us) AppleWebKit/531.2+ (KHTML, like Gecko) Version/5.0 Safari/533.2+ Kindle/3.0+", true},
		{"kindle keyboard", kindleUserAgent, true},
		{"kindle netfront", "Mozilla/4.0 (compatible; Linux 2.6.10) NetFront/3.3 Kindle/1.0 (screen 600x800)", true},
		{"kobo", koboUserAgent, false},
		{"kindle fire silk", "Mozilla/5.0 (Linux; U; Android 4.0.3; en-us; Kindle Fire HD Build/IML74K) AppleWebKit/535.19 (KHTML, like Gecko) Silk/2.4 Safari/535.19 Silk-Accelerated=true", false},
		{"fire tablet silk", "Mozilla/5.0 (Linux; Android 9; KFTRWI) AppleWebKit/537.36 (KHTML, like Gecko) Silk/120.4.1 like Chrome/120.0.6099.230 Safari/537.36", false},
		{"desktop", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isKindle(tt.userAgent))
		})
	}
}

func TestKindleFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   []*models.File
		wantIDs []int
	}{
		{
			name: "mobi before azw3",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain},
				{ID: 2, FileType: models.FileTypeAZW3, FileRole: models.FileRoleMain},
				{ID: 3, FileType: models.FileTypeMOBI, FileRole: models.FileRoleMain},
			},
			wantIDs: []int{3},
		},
		{
			name: "every mobi",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeMOBI, FileRole: models.FileRoleMain},
				{ID: 2, FileType: models.FileTypeAZW3, FileRole: models.FileRoleMain},
				{ID: 3, FileType: models.FileTypeMOBI, FileRole: models.FileRoleMain},
			},
			wantIDs: []int{1, 3},
		},
		{
			name: "azw3 when no mobi",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain},
				{ID: 2, FileType: models.FileTypeAZW3, FileRole: models.FileRoleMain},
			},
			wantIDs: []int{2},
		},
		{
			name: "skips supplements",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeMOBI, FileRole: models.FileRoleSupplement},
				{ID: 2, FileType: models.FileTypeAZW3, FileRole: models.FileRoleMain},
			},
			wantIDs: []int{2},
		},
		{
			name: "none, pdf included",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain},
				{ID: 2, FileType: models.FileTypePDF, FileRole: models.FileRoleMain},
			},
			wantIDs: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []int
			for _, f := range kindleFiles(tt.files) {
				got = append(got, f.ID)
			}
			assert.Equal(t, tt.wantIDs, got)
		})
	}
}

func TestKindleFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file *models.File
		want string
	}{
		{"book title", &models.File{FileType: models.FileTypeMOBI}, "The_Book_s_Title.mobi"},
		{"file name with accents", &models.File{FileType: models.FileTypeAZW3, Name: strPtr("Édition Spéciale (2nd)")}, "Edition_Speciale_2nd.azw3"},
		{"name ending in its extension", &models.File{FileType: models.FileTypeMOBI, Name: strPtr("Some Book.MOBI")}, "Some_Book.mobi"},
		{"nothing ascii", &models.File{FileType: models.FileTypeMOBI, Name: strPtr("日本語")}, "book.mobi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, kindleFilename("The Book's Title", tt.file))
		})
	}
}

func strPtr(s string) *string { return &s }

// kindleFixture is the access fixture with five books in libA: one with an
// EPUB, a MOBI, an AZW3, and a PDF, one with an EPUB and an AZW3, one with an
// EPUB and a PDF, one with only an EPUB, and one with an EPUB main file and a
// MOBI supplement.
type kindleFixture struct {
	*eReaderAccessFixture
	allFormats *models.Book
	azw3Only   *models.Book
	epubPDF    *models.Book
	epubOnly   *models.Book
	supplement *models.Book
	files      map[string]*models.File
}

func newKindleFixture(t *testing.T) *kindleFixture {
	t.Helper()
	ctx := context.Background()
	f := &kindleFixture{eReaderAccessFixture: newEReaderAccessFixture(t), files: map[string]*models.File{}}

	// addBook takes file types, each a main file unless prefixed with
	// "supplement:".
	addBook := func(title string, fileTypes ...string) *models.Book {
		t.Helper()
		book := &models.Book{
			LibraryID:       f.libA.ID,
			Title:           title,
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       title,
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        "/books/" + title,
		}
		_, err := f.db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)
		for _, fileType := range fileTypes {
			role := models.FileRoleMain
			if after, ok := strings.CutPrefix(fileType, "supplement:"); ok {
				fileType, role = after, models.FileRoleSupplement
			}
			file := &models.File{
				LibraryID:     f.libA.ID,
				BookID:        book.ID,
				FileType:      fileType,
				FileRole:      role,
				Filepath:      "/books/" + title + "/book." + fileType,
				FilesizeBytes: 1024,
			}
			_, err := f.db.NewInsert().Model(file).Exec(ctx)
			require.NoError(t, err)
			f.files[title+"/"+fileType] = file
		}
		return book
	}

	f.allFormats = addBook("All Formats", models.FileTypeEPUB, models.FileTypeMOBI, models.FileTypeAZW3, models.FileTypePDF)
	f.azw3Only = addBook("Has AZW3", models.FileTypeEPUB, models.FileTypeAZW3)
	f.epubPDF = addBook("Has PDF", models.FileTypeEPUB, models.FileTypePDF)
	f.epubOnly = addBook("EPUB Only", models.FileTypeEPUB)
	f.supplement = addBook("Has Supplement", models.FileTypeEPUB, "supplement:"+models.FileTypeMOBI)
	return f
}

func (f *kindleFixture) page(t *testing.T, path, userAgent string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ereader/key/"+f.key.Key+path, nil)
	req.Header.Set("User-Agent", userAgent)
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec.Body.String()
}

// fileLink is a file's plain download link as it appears in an href.
func (f *kindleFixture) fileLink(title, fileType string) string {
	return `/file/` + strconv.Itoa(f.files[title+"/"+fileType].ID) + `"`
}

// kindleLink is a file's Kindle download link, which ends in the filename.
func (f *kindleFixture) kindleLink(title, fileType, filename string) string {
	return `/file/` + strconv.Itoa(f.files[title+"/"+fileType].ID) + `/kindle/` + filename + `"`
}

// fileLinkPrefix matches any download link for a file.
func (f *kindleFixture) fileLinkPrefix(title, fileType string) string {
	return `/file/` + strconv.Itoa(f.files[title+"/"+fileType].ID)
}

func TestDownload_KindleGetsMOBI(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)

	body := f.page(t, "/download/"+strconv.Itoa(f.allFormats.ID), kindleUserAgent)

	assert.Contains(t, body, f.kindleLink("All Formats", models.FileTypeMOBI, "All_Formats.mobi"))
	assert.Contains(t, body, "Download MOBI")
	for _, fileType := range []string{models.FileTypeAZW3, models.FileTypePDF, models.FileTypeEPUB} {
		assert.NotContains(t, body, f.fileLinkPrefix("All Formats", fileType), fileType)
	}
	assert.NotContains(t, body, "Unavailable on Kindle")
	assert.NotContains(t, body, "may refuse")
}

func TestDownload_KindleGetsAZW3WithoutMOBI(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)

	body := f.page(t, "/download/"+strconv.Itoa(f.azw3Only.ID), kindleUserAgent)

	assert.Contains(t, body, f.kindleLink("Has AZW3", models.FileTypeAZW3, "Has_AZW3.azw3"))
	assert.Contains(t, body, "Download AZW3")
	assert.Contains(t, body, "Newer Kindles may refuse AZW3 downloads")
	assert.NotContains(t, body, f.fileLinkPrefix("Has AZW3", models.FileTypeEPUB))
}

func TestDownload_KindleBookWithoutKindleFileIsUnavailable(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)

	for _, book := range []*models.Book{f.epubOnly, f.epubPDF, f.supplement} {
		body := f.page(t, "/download/"+strconv.Itoa(book.ID), kindleUserAgent)

		assert.Contains(t, body, "Unavailable on Kindle", book.Title)
		assert.NotContains(t, body, "/file/", book.Title)
		assert.NotContains(t, body, "Download ", book.Title)
	}
}

func TestDownload_NonKindleSeesEveryFile(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)

	for _, ua := range []string{"", koboUserAgent} {
		body := f.page(t, "/download/"+strconv.Itoa(f.allFormats.ID), ua)

		for _, fileType := range []string{models.FileTypeAZW3, models.FileTypeMOBI, models.FileTypePDF} {
			assert.Contains(t, body, f.fileLink("All Formats", fileType), ua)
		}
		assert.Contains(t, body, "Download AZW3", ua)
		assert.Contains(t, body, "Download MOBI", ua)
		assert.NotContains(t, body, "/kindle/", ua)
		assert.NotContains(t, body, "Unavailable on Kindle", ua)
	}

	// A Kobo keeps getting KePub for the EPUB.
	body := f.page(t, "/download/"+strconv.Itoa(f.allFormats.ID), koboUserAgent)
	assert.Contains(t, body, "/file/"+strconv.Itoa(f.files["All Formats/epub"].ID)+"/kepub")
}

func TestBookLists_KindleMarksBooksWithoutKindleFile(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)
	lib := strconv.Itoa(f.libA.ID)

	body := f.page(t, "/libraries/"+lib+"/all", kindleUserAgent)
	// EPUB Only, Has PDF, and Has Supplement.
	assert.Equal(t, 3, strings.Count(body, "Unavailable on Kindle"), body)

	body = f.page(t, "/libraries/"+lib+"/all", "")
	assert.NotContains(t, body, "Unavailable on Kindle")
}

// A list row names the types of a book's main files, not its supplements.
func TestBookLists_FileTypesSkipSupplements(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)

	body := f.page(t, "/libraries/"+strconv.Itoa(f.libA.ID)+"/all", "")

	_, row, found := strings.Cut(body, ">Has Supplement<")
	require.True(t, found, body)
	meta, _, _ := strings.Cut(row, "</a>")
	assert.Contains(t, meta, ">EPUB<")
	assert.NotContains(t, meta, "MOBI")
}

func TestFilterBar_IncludesMOBIAndAZW3(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)
	lib := strconv.Itoa(f.libA.ID)

	body := f.page(t, "/libraries/"+lib+"/all", "")
	assert.Contains(t, body, "types=mobi")
	assert.Contains(t, body, "types=azw3")

	body = f.page(t, "/libraries/"+lib+"/all?types=azw3", "")
	assert.Contains(t, body, "All Formats")
	assert.Contains(t, body, "Has AZW3")
	assert.NotContains(t, body, "EPUB Only")

	// A book matches when any of its main files has the type, so a MOBI
	// next to an EPUB is not hidden behind it, and a supplement never counts.
	body = f.page(t, "/libraries/"+lib+"/all?types=mobi", "")
	assert.Contains(t, body, "All Formats")
	assert.NotContains(t, body, "Has AZW3")
	assert.NotContains(t, body, "Has Supplement")

	body = f.page(t, "/libraries/"+lib+"/all?types=epub", "")
	for _, title := range []string{"All Formats", "Has AZW3", "Has PDF", "EPUB Only", "Has Supplement"} {
		assert.Contains(t, body, title)
	}
}

// The Kindle download route serves the file with no Content-Disposition, so
// the Kindle names it from the URL, whose filename it checks against the
// extensions it accepts.
func TestDownloadFileKindle_ServesWithoutContentDisposition(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		fileType    string
		kind        testgen.MOBIKind
		contentType string
	}{
		{models.FileTypeMOBI, testgen.MOBIKindMOBI6, "application/x-mobipocket-ebook"},
		{models.FileTypeAZW3, testgen.MOBIKindKF8, "application/vnd.amazon.mobi8-ebook"},
	} {
		t.Run(tt.fileType, func(t *testing.T) {
			t.Parallel()
			f := newDownloadFixture(t, "Ünïcode Title")
			file := f.addFile(tt.fileType, models.FileRoleMain, testgen.GenerateMOBI(t, f.dir, "book."+tt.fileType, testgen.MOBIOptions{Kind: tt.kind, Title: "Ünïcode Title"}))

			rec := f.serveKindle(file)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, tt.contentType, rec.Header().Get("Content-Type"))
			assert.Empty(t, rec.Header().Get("Content-Disposition"))
			assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
		})
	}
}

func TestDownloadFileKindle_UnreadableFileIsServerError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}
	f := newDownloadFixture(t, "Locked")
	file := f.addFile(models.FileTypeMOBI, models.FileRoleMain, testgen.GenerateMOBI(t, f.dir, "book.mobi", testgen.MOBIOptions{Kind: testgen.MOBIKindMOBI6, Title: "Locked"}))
	lockFile(t, file.Filepath)

	assertServerFault(t, f.serveKindle(file))
}

// serveKindle runs the Kindle download handler the way serve runs the others.
func (f *downloadFixture) serveKindle(file *models.File) *httptest.ResponseRecorder {
	f.t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	withKey(c, f.t, f.db, f.apiKey)
	c.SetParamNames("apiKey", "fileId", "filename")
	c.SetParamValues(f.apiKey.Key, strconv.Itoa(file.ID), "book."+file.FileType)
	if err := f.h.DownloadFileKindle(c); err != nil {
		errcodes.NewHandler().Handle(err, c)
	}
	return rec
}
