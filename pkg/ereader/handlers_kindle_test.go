package ereader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

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
		{"kindle keyboard", kindleUserAgent, true},
		{"paperwhite chromium", "Mozilla/5.0 (Linux; Kindle Paperwhite) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36", true},
		{"kobo", koboUserAgent, false},
		{"kindle fire silk", "Mozilla/5.0 (Linux; U; Android 4.0.3; en-us; Kindle Fire HD Build/IML74K) AppleWebKit/535.19 (KHTML, like Gecko) Silk/2.4 Safari/535.19 Silk-Accelerated=true", false},
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
			name: "azw3 before mobi",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain},
				{ID: 2, FileType: models.FileTypeMOBI, FileRole: models.FileRoleMain},
				{ID: 3, FileType: models.FileTypeAZW3, FileRole: models.FileRoleMain},
			},
			wantIDs: []int{3},
		},
		{
			name: "every azw3",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeAZW3, FileRole: models.FileRoleMain},
				{ID: 2, FileType: models.FileTypeMOBI, FileRole: models.FileRoleMain},
				{ID: 3, FileType: models.FileTypeAZW3, FileRole: models.FileRoleMain},
			},
			wantIDs: []int{1, 3},
		},
		{
			name: "mobi when no azw3",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain},
				{ID: 2, FileType: models.FileTypeMOBI, FileRole: models.FileRoleMain},
			},
			wantIDs: []int{2},
		},
		{
			name: "skips supplements",
			files: []*models.File{
				{ID: 1, FileType: models.FileTypeAZW3, FileRole: models.FileRoleSupplement},
				{ID: 2, FileType: models.FileTypeMOBI, FileRole: models.FileRoleMain},
			},
			wantIDs: []int{2},
		},
		{
			name: "neither",
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

// kindleFixture is the access fixture with three books in libA: one with an
// EPUB, a MOBI, and an AZW3, one with an EPUB and a MOBI, and one with only
// an EPUB.
type kindleFixture struct {
	*eReaderAccessFixture
	allFormats *models.Book
	mobiOnly   *models.Book
	epubOnly   *models.Book
	files      map[string]*models.File
}

func newKindleFixture(t *testing.T) *kindleFixture {
	t.Helper()
	ctx := context.Background()
	f := &kindleFixture{eReaderAccessFixture: newEReaderAccessFixture(t), files: map[string]*models.File{}}

	addBook := func(title string, fileTypes ...string) *models.Book {
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
			file := &models.File{
				LibraryID:     f.libA.ID,
				BookID:        book.ID,
				FileType:      fileType,
				FileRole:      models.FileRoleMain,
				Filepath:      "/books/" + title + "/book." + fileType,
				FilesizeBytes: 1024,
			}
			_, err := f.db.NewInsert().Model(file).Exec(ctx)
			require.NoError(t, err)
			f.files[title+"/"+fileType] = file
		}
		return book
	}

	f.allFormats = addBook("All Formats", models.FileTypeEPUB, models.FileTypeMOBI, models.FileTypeAZW3)
	f.mobiOnly = addBook("Has MOBI", models.FileTypeEPUB, models.FileTypeMOBI)
	f.epubOnly = addBook("EPUB Only", models.FileTypeEPUB)
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

func (f *kindleFixture) fileLink(title, fileType string) string {
	return `/file/` + strconv.Itoa(f.files[title+"/"+fileType].ID) + `"`
}

func TestDownload_KindleGetsAZW3(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)

	body := f.page(t, "/download/"+strconv.Itoa(f.allFormats.ID), kindleUserAgent)

	assert.Contains(t, body, f.fileLink("All Formats", models.FileTypeAZW3))
	assert.Contains(t, body, "Download AZW3")
	assert.NotContains(t, body, f.fileLink("All Formats", models.FileTypeMOBI))
	assert.NotContains(t, body, f.fileLink("All Formats", models.FileTypeEPUB))
	assert.NotContains(t, body, "Unavailable on Kindle")
}

func TestDownload_KindleGetsMOBIWithoutAZW3(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)

	body := f.page(t, "/download/"+strconv.Itoa(f.mobiOnly.ID), kindleUserAgent)

	assert.Contains(t, body, f.fileLink("Has MOBI", models.FileTypeMOBI))
	assert.Contains(t, body, "Download MOBI")
	assert.NotContains(t, body, f.fileLink("Has MOBI", models.FileTypeEPUB))
}

func TestDownload_KindleBookWithoutKindleFileIsUnavailable(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)

	body := f.page(t, "/download/"+strconv.Itoa(f.epubOnly.ID), kindleUserAgent)

	assert.Contains(t, body, "Unavailable on Kindle")
	assert.NotContains(t, body, f.fileLink("EPUB Only", models.FileTypeEPUB))
	assert.NotContains(t, body, "Download EPUB")
}

func TestDownload_NonKindleSeesEveryFile(t *testing.T) {
	t.Parallel()
	f := newKindleFixture(t)

	for _, ua := range []string{"", koboUserAgent} {
		body := f.page(t, "/download/"+strconv.Itoa(f.allFormats.ID), ua)

		assert.Contains(t, body, f.fileLink("All Formats", models.FileTypeAZW3), ua)
		assert.Contains(t, body, f.fileLink("All Formats", models.FileTypeMOBI), ua)
		assert.Contains(t, body, "Download AZW3", ua)
		assert.Contains(t, body, "Download MOBI", ua)
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
	assert.Equal(t, 1, strings.Count(body, "Unavailable on Kindle"), body)
	assert.Contains(t, body, "EPUB Only")

	body = f.page(t, "/libraries/"+lib+"/all", "")
	assert.NotContains(t, body, "Unavailable on Kindle")
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
	assert.NotContains(t, body, "Has MOBI")
	assert.NotContains(t, body, "EPUB Only")

	// A book matches when any of its main files has the type, so a MOBI
	// next to an EPUB is not hidden behind it.
	body = f.page(t, "/libraries/"+lib+"/all?types=mobi", "")
	assert.Contains(t, body, "All Formats")
	assert.Contains(t, body, "Has MOBI")
	assert.NotContains(t, body, "EPUB Only")

	body = f.page(t, "/libraries/"+lib+"/all?types=epub", "")
	assert.Contains(t, body, "All Formats")
	assert.Contains(t, body, "Has MOBI")
	assert.Contains(t, body, "EPUB Only")
}
