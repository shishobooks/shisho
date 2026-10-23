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
	"github.com/shishobooks/shisho/pkg/devices"
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

func TestKindleFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file *models.File
		want string
	}{
		{"book title", &models.File{FileType: models.FileTypeMOBI}, "The_Book_s_Title.mobi"},
		{"delivered as another type", &models.File{FileType: models.FileTypeEPUB}, "The_Book_s_Title.azw3"},
		{"file name with accents", &models.File{FileType: models.FileTypeAZW3, Name: strPtr("Édition Spéciale (2nd)")}, "Edition_Speciale_2nd.azw3"},
		{"name ending in its extension", &models.File{FileType: models.FileTypeMOBI, Name: strPtr("Some Book.MOBI")}, "Some_Book.mobi"},
		{"nothing ascii", &models.File{FileType: models.FileTypeMOBI, Name: strPtr("日本語")}, "book.mobi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deliveredType := tt.file.FileType
			if deliveredType == models.FileTypeEPUB {
				deliveredType = models.FileTypeAZW3
			}
			assert.Equal(t, tt.want, kindleFilename("The Book's Title", tt.file, deliveredType))
		})
	}
}

func strPtr(s string) *string { return &s }

// kindleFixture is the access fixture with six books in libA: one with an
// EPUB, a MOBI, an AZW3, and a PDF, one with an EPUB and an AZW3, one with an
// EPUB and a MOBI, one with an EPUB and a PDF, one with only an EPUB, and one
// with an EPUB main file and a MOBI supplement.
type kindleFixture struct {
	*eReaderAccessFixture
	allFormats *models.Book
	azw3Only   *models.Book
	mobiOnly   *models.Book
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
	f.mobiOnly = addBook("Has MOBI", models.FileTypeEPUB, models.FileTypeMOBI)
	f.epubPDF = addBook("Has PDF", models.FileTypeEPUB, models.FileTypePDF)
	f.epubOnly = addBook("EPUB Only", models.FileTypeEPUB)
	f.supplement = addBook("Has Supplement", models.FileTypeEPUB, "supplement:"+models.FileTypeMOBI)
	return f
}

// setType changes the fixture's Device to deviceType, which puts it on the
// rules that type starts with.
func (f *kindleFixture) setType(t *testing.T, deviceType string) {
	t.Helper()
	device, err := devices.NewService(f.db).Update(context.Background(), f.user.ID, f.device.ID, devices.UpdateDeviceOptions{DeviceType: &deviceType})
	require.NoError(t, err)
	f.device = device
}

func (f *kindleFixture) page(t *testing.T, path, userAgent string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ereader/key/"+f.device.Credential+path, nil)
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

// A Kindle Device follows its preset: AZW3 first, with a Kindle download
// URL, and MOBI for a book without one.
func TestDownload_KindleDeviceGetsAZW3First(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)
	f.setType(t, devices.DeviceTypeKindle)

	body := f.page(t, "/download/"+strconv.Itoa(f.allFormats.ID), kindleUserAgent)
	assert.Contains(t, body, f.kindleLink("All Formats", models.FileTypeAZW3, "All_Formats.azw3"))
	assert.Contains(t, body, "Download AZW3")
	assert.Contains(t, body, "Some Kindles refuse AZW3 downloads. If yours does, put MOBI first in this Device's Delivery Rules.")
	for _, fileType := range []string{models.FileTypeMOBI, models.FileTypePDF, models.FileTypeEPUB} {
		assert.NotContains(t, body, f.fileLinkPrefix("All Formats", fileType), fileType)
	}

	body = f.page(t, "/download/"+strconv.Itoa(f.mobiOnly.ID), kindleUserAgent)
	assert.Contains(t, body, f.kindleLink("Has MOBI", models.FileTypeMOBI, "Has_MOBI.mobi"))
	assert.Contains(t, body, "Download MOBI")
	assert.NotContains(t, body, "refuse AZW3")
	assert.NotContains(t, body, f.fileLinkPrefix("Has MOBI", models.FileTypeEPUB))
}

func TestDownload_KindleDeviceBookWithoutKindleFileIsNotDelivered(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)
	f.setType(t, devices.DeviceTypeKindle)

	for _, book := range []*models.Book{f.epubOnly, f.epubPDF, f.supplement} {
		body := f.page(t, "/download/"+strconv.Itoa(book.ID), kindleUserAgent)

		assert.Contains(t, body, "Not delivered to this Device.", book.Title)
		assert.NotContains(t, body, "/file/", book.Title)
		assert.NotContains(t, body, "Download ", book.Title)
	}
}

// What a Device is served depends on its type and rules alone. A Browser
// Device on a Kindle or a Kobo gets the same downloads as anywhere else:
// the built-in rules' EPUB, with a plain link.
func TestDownload_UserAgentDoesNotChangeWhatIsServed(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)

	for _, ua := range []string{"", kindleUserAgent, koboUserAgent} {
		body := f.page(t, "/download/"+strconv.Itoa(f.allFormats.ID), ua)

		assert.Contains(t, body, f.fileLink("All Formats", models.FileTypeEPUB), ua)
		assert.Contains(t, body, "Download EPUB", ua)
		for _, fileType := range []string{models.FileTypeAZW3, models.FileTypeMOBI, models.FileTypePDF} {
			assert.NotContains(t, body, f.fileLinkPrefix("All Formats", fileType), ua)
		}
		assert.NotContains(t, body, "/kindle/", ua)
		assert.NotContains(t, body, "/kepub", ua)
	}
}

// The User-Agent only shows a banner, on every page, when it suggests a
// Kindle or a Kobo and the Device's type is another.
func TestTypeMismatchBanner(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)
	lib := "/libraries/" + strconv.Itoa(f.libA.ID) + "/all"

	for _, path := range []string{"/", lib, "/download/" + strconv.Itoa(f.allFormats.ID)} {
		assert.Contains(t, f.page(t, path, kindleUserAgent), "This looks like a Kindle, but this Device is set up as a Browser Device.", path)
		assert.Contains(t, f.page(t, path, koboUserAgent), "This looks like a Kobo, but this Device is set up as a Browser Device.", path)
		assert.NotContains(t, f.page(t, path, ""), "This looks like", path)
	}

	f.setType(t, devices.DeviceTypeKindle)
	assert.NotContains(t, f.page(t, "/", kindleUserAgent), "This looks like")
	assert.Contains(t, f.page(t, "/", koboUserAgent), "This looks like a Kobo, but this Device is set up as a Kindle Device.")
}

func TestTypeMismatchBanner_EscapesNothingFromTheUserAgent(t *testing.T) {
	t.Parallel()
	assert.NotContains(t, typeMismatchBanner(devices.DeviceTypeBrowser, `Kindle/3.0 <script>`), "<script>")
}

// Book lists hide books the Device's rules deliver nothing from, and each
// row names the File Types a book is delivered as.
func TestBookLists_FollowTheDeviceRules(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)
	lib := "/libraries/" + strconv.Itoa(f.libA.ID) + "/all"

	f.setType(t, devices.DeviceTypeKindle)
	body := f.page(t, lib, kindleUserAgent)
	for _, title := range []string{"All Formats", "Has AZW3", "Has MOBI"} {
		assert.Contains(t, body, ">"+title+"<")
	}
	for _, title := range []string{"EPUB Only", "Has PDF", "Has Supplement"} {
		assert.NotContains(t, body, ">"+title+"<")
	}

	f.setType(t, devices.DeviceTypeKobo)
	body = f.page(t, lib, koboUserAgent)
	_, row, found := strings.Cut(body, ">EPUB Only<")
	require.True(t, found, body)
	meta, _, _ := strings.Cut(row, "</a>")
	assert.Contains(t, meta, ">KePub<")
}

// A list row names the types a book is delivered as, not its supplements.
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

// The type filter offers the File Types the Device's rules deliver the
// page's books as, and narrows to the books delivered as the chosen one, so
// no filter opens an empty page.
func TestFilterBar_FollowsTheDeviceRules(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)
	lib := strconv.Itoa(f.libA.ID)

	// The built-in rules send every book here as its EPUB, so there is
	// nothing to filter by.
	body := f.page(t, "/libraries/"+lib+"/all", "")
	assert.NotContains(t, body, "types=")

	// A Kindle gets AZW3 before MOBI, so a book with both is sent as AZW3,
	// and a MOBI supplement doesn't count.
	f.setType(t, devices.DeviceTypeKindle)
	body = f.page(t, "/libraries/"+lib+"/all", kindleUserAgent)
	assert.Contains(t, body, "types=azw3")
	assert.Contains(t, body, "types=mobi")
	assert.NotContains(t, body, "types=epub")

	body = f.page(t, "/libraries/"+lib+"/all?types=azw3", kindleUserAgent)
	assert.Contains(t, body, "All Formats")
	assert.Contains(t, body, "Has AZW3")
	assert.NotContains(t, body, "Has MOBI")

	body = f.page(t, "/libraries/"+lib+"/all?types=mobi", kindleUserAgent)
	assert.Contains(t, body, "Has MOBI")
	assert.NotContains(t, body, "All Formats", "its AZW3 is sent instead")
	assert.NotContains(t, body, "Has Supplement")

	body = f.page(t, "/libraries/"+lib+"/all?types=epub", kindleUserAgent)
	assert.Contains(t, body, "All Formats", "a type the rules don't deliver shows every book")
	assert.Contains(t, body, "Has MOBI")
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
	withDevice(c, f.t, f.db, f.device)
	c.SetParamNames("credential", "fileId", "filename")
	c.SetParamValues(f.device.Credential, strconv.Itoa(file.ID), "book."+file.FileType)
	if err := f.h.DownloadFileKindle(c); err != nil {
		errcodes.NewHandler().Handle(err, c)
	}
	return rec
}

// The Kindle download route reads the delivered File Type from the URL's
// extension, so one no File Type has names nothing.
func TestDownloadFileKindle_UnknownExtensionIsNotFound(t *testing.T) {
	t.Parallel()
	f := newDownloadFixture(t, "Book")
	file := f.addFile(models.FileTypeMOBI, models.FileRoleMain, testgen.GenerateMOBI(t, f.dir, "book.mobi", testgen.MOBIOptions{Kind: testgen.MOBIKindMOBI6, Title: "Book"}))

	for _, filename := range []string{"book.fb2", "book"} {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		withDevice(c, t, f.db, f.device)
		c.SetParamNames("credential", "fileId", "filename")
		c.SetParamValues(f.device.Credential, strconv.Itoa(file.ID), filename)
		err := f.h.DownloadFileKindle(c)
		var codeErr *errcodes.Error
		require.ErrorAs(t, err, &codeErr, filename)
		assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode, filename)
	}
}

// No page lists an entry that opens empty: the libraries page leaves out a
// library with nothing the Device's rules deliver, and a library's page
// leaves out Series and Authors when it has none to deliver.
func TestEmptyEntriesAreLeftOut(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)
	libA := "/libraries/" + strconv.Itoa(f.libA.ID)
	libB := "/libraries/" + strconv.Itoa(f.libB.ID)

	// libB has no books at all.
	body := f.page(t, "/", "")
	assert.Contains(t, body, libA+`"`)
	assert.NotContains(t, body, libB+`"`)

	// The fixture's books have no series or authors.
	body = f.page(t, libA, "")
	assert.Contains(t, body, libA+"/all")
	assert.Contains(t, body, libA+"/search")
	assert.NotContains(t, body, libA+"/series")
	assert.NotContains(t, body, libA+"/authors")

	// Once a deliverable book has an author, Authors is listed.
	ctx := context.Background()
	person := &models.Person{LibraryID: f.libA.ID, Name: "Writer", SortName: "Writer", SortNameSource: models.DataSourceFilepath}
	_, err := f.db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)
	_, err = f.db.NewInsert().Model(&models.Author{BookID: f.epubOnly.ID, PersonID: person.ID, SortOrder: 1}).Exec(ctx)
	require.NoError(t, err)
	assert.Contains(t, f.page(t, libA, ""), libA+"/authors")

	// A Kindle receives nothing from that EPUB-only book, so not for it.
	f.setType(t, devices.DeviceTypeKindle)
	assert.NotContains(t, f.page(t, libA, kindleUserAgent), libA+"/authors")
}
