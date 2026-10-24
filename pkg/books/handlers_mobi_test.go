package books

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadOriginalFile_MOBIContentTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		filename    string
		fileType    string
		kind        testgen.MOBIKind
		contentType string
	}{
		{"book.mobi", models.FileTypeMOBI, testgen.MOBIKindMOBI6, "application/x-mobipocket-ebook"},
		{"book.azw", models.FileTypeMOBI, testgen.MOBIKindMOBI6, "application/x-mobipocket-ebook"},
		{"book.azw3", models.FileTypeAZW3, testgen.MOBIKindKF8, "application/vnd.amazon.mobi8-ebook"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			library, book := setupTestLibraryAndBook(t, db)
			content := testgen.BuildMOBI(t, testgen.MOBIOptions{Kind: tt.kind, Title: "Book"})
			path := testgen.WriteFile(t, book.Filepath, tt.filename, content)
			file := setupTestFile(t, db, book, tt.fileType, path)
			user := setupTestUser(t, db, library.ID, true)

			req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/download/original", nil)
			rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)

			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assert.Equal(t, tt.contentType, rr.Header().Get(echo.HeaderContentType))
			assert.Contains(t, rr.Header().Get(echo.HeaderContentDisposition), `filename="Test Book`+filepath.Ext(tt.filename)+`"`, "named after the book, with the file's own extension")
			assert.True(t, bytes.Equal(content, rr.Body.Bytes()), "the file is served untouched")
		})
	}
}

func TestUpdateFile_UpgradesMOBIAndAZW3SupplementsToMain(t *testing.T) {
	t.Parallel()

	for _, fileType := range []string{models.FileTypeMOBI, models.FileTypeAZW3} {
		t.Run(fileType, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			library, book := setupTestLibraryAndBook(t, db)
			path := testgen.WriteFile(t, book.Filepath, "book."+fileType, []byte("content"))
			file := setupTestFile(t, db, book, fileType, path)
			file.FileRole = models.FileRoleSupplement
			_, err := db.NewUpdate().Model(file).Column("file_role").WherePK().Exec(t.Context())
			require.NoError(t, err)
			user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

			req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(`{"file_role":"main"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)

			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			var role string
			require.NoError(t, db.NewSelect().Table("files").Column("file_role").Where("id = ?", file.ID).Scan(t.Context(), &role))
			assert.Equal(t, models.FileRoleMain, role)
		})
	}
}

// Preferred Cover is exclusive within the ebook category, which holds MOBI
// and AZW3 alongside EPUB, CBZ, and PDF.
func TestUpdateFile_PreferredCoverIsExclusiveAcrossEbookTypes(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)

	withCover := func(fileType string) *models.File {
		path := testgen.WriteFile(t, book.Filepath, "book."+fileType, []byte("content"))
		testgen.WriteFile(t, book.Filepath, filepath.Base(path)+".cover.jpg", []byte("jpg"))
		file := setupTestFile(t, db, book, fileType, path)
		cover := filepath.Base(path) + ".cover.jpg"
		file.CoverImageFilename = &cover
		_, err := db.NewUpdate().Model(file).Column("cover_image_filename").WherePK().Exec(t.Context())
		require.NoError(t, err)
		return file
	}
	epub := withCover(models.FileTypeEPUB)
	azw3 := withCover(models.FileTypeAZW3)
	mobi := withCover(models.FileTypeMOBI)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))
	e := setupTestServer(t, db)

	prefer := func(file *models.File) {
		req := httptest.NewRequest(http.MethodPost, "/books/files/"+strconv.Itoa(file.ID), strings.NewReader(`{"is_preferred_cover":true}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rr := executeRequestWithUser(t, e, req, user)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	}
	preferred := func() []int {
		var ids []int
		require.NoError(t, db.NewSelect().Table("files").Column("id").Where("is_preferred_cover").Order("id").Scan(t.Context(), &ids))
		return ids
	}

	prefer(epub)
	assert.Equal(t, []int{epub.ID}, preferred())
	prefer(mobi)
	assert.Equal(t, []int{mobi.ID}, preferred(), "preferring the MOBI clears the EPUB")
	prefer(azw3)
	assert.Equal(t, []int{azw3.ID}, preferred(), "preferring the AZW3 clears the MOBI")
}

// The web app's Download writes the book's metadata into MOBI and AZW3 files,
// as it does for EPUB, and leaves the file on disk alone.
func TestDownloadFile_GeneratesMOBIAndAZW3(t *testing.T) {
	t.Parallel()

	tests := []struct {
		filename    string
		fileType    string
		kind        testgen.MOBIKind
		contentType string
		saveAs      string
	}{
		{"book.mobi", models.FileTypeMOBI, testgen.MOBIKindCombo, "application/x-mobipocket-ebook", "Test Book.mobi"},
		{"book.azw", models.FileTypeMOBI, testgen.MOBIKindMOBI6, "application/x-mobipocket-ebook", "Test Book.mobi"},
		{"book.azw3", models.FileTypeAZW3, testgen.MOBIKindKF8, "application/vnd.amazon.mobi8-ebook", "Test Book.azw3"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			library, book := setupTestLibraryAndBook(t, db)
			content := testgen.BuildMOBI(t, testgen.MOBIOptions{Kind: tt.kind, Title: "Title In File", HasCover: true})
			path := testgen.WriteFile(t, book.Filepath, tt.filename, content)
			file := setupTestFile(t, db, book, tt.fileType, path)
			user := setupTestUser(t, db, library.ID, true)

			req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/download", nil)
			rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)

			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assert.Equal(t, tt.contentType, rr.Header().Get(echo.HeaderContentType))
			assert.Contains(t, rr.Header().Get(echo.HeaderContentDisposition), tt.saveAs)

			meta, err := mobi.Parse(testgen.WriteFile(t, t.TempDir(), tt.filename, rr.Body.Bytes()))
			require.NoError(t, err)
			assert.Equal(t, "Test Book", meta.Title)
			assert.True(t, bytes.Equal(content, testgen.ReadFile(t, path)), "the file on disk is untouched")
		})
	}
}

// A Kindle file the conversion declines, such as a MOBI with only an older
// KF7 book, is a 422 that says why, so the web UI can show it next to
// Download unmodified file.
func TestDownloadFile_DeclinedKindleConversionSaysWhy(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	path := testgen.WriteFile(t, book.Filepath, "book.mobi", testgen.BuildMOBI(t, testgen.MOBIOptions{Kind: testgen.MOBIKindMOBI6, Title: "Old"}))
	file := setupTestFile(t, db, book, models.FileTypeMOBI, path)
	user := setupTestUser(t, db, library.ID, true)

	req := httptest.NewRequest(http.MethodGet, "/books/files/"+strconv.Itoa(file.ID)+"/download/epub", nil)
	rr := executeRequestWithUser(t, setupTestServer(t, db), req, user)

	require.Equal(t, http.StatusUnprocessableEntity, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "This Kindle book can't be converted: it holds only an older MOBI (KF7) book")
	assert.Empty(t, rr.Header().Get(echo.HeaderContentDisposition))
}
