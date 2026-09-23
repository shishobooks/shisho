package plugins

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cover application through POST /plugins/apply. Cover has no edit state:
// keeping the current Cover sends no cover field, and choosing the proposed
// Cover is always a Proposal Acceptance that stamps the plugin source. Both
// the image-based path (coverUrl) and the page-based path (coverPage) must
// write the complete Cover state in one UpdateFile column set.

var coverColumns = []string{"cover_page", "cover_image_filename", "cover_mime_type", "cover_source"}

func makeApplyTestPNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// newCoverImageServer serves one image body with the given Content-Type.
// A non-2xx status simulates a failed download.
func newCoverImageServer(t *testing.T, status int, contentType string, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newCoverApplyTestHandler wires the apply handler with the test server's
// host allowed for cover downloads and an optional page extractor.
func newCoverApplyTestHandler(store *stubBookStoreForApply, srv *httptest.Server, extractor *stubPageExtractor) *handler {
	h := newApplyTestHandler(store)
	if srv != nil {
		rt := h.manager.plugins[pluginKey("test", "enricher")]
		rt.manifest.Capabilities.HTTPAccess = &HTTPAccessCap{Domains: []string{testServerHost(srv)}}
	}
	if extractor != nil {
		h.enrich.pageExtractor = extractor
	}
	return h
}

func hasCoverColumn(columns []string) bool {
	for _, col := range columns {
		if strings.HasPrefix(col, "cover_") {
			return true
		}
	}
	return false
}

func TestApplyMetadata_Cover_ImageBased_WritesCompleteProvenance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		contentType  string
		body         []byte
		wantFilename string
		wantMime     string
	}{
		{
			name:         "jpeg stays jpeg",
			contentType:  "image/jpeg",
			body:         makePersistTestJPEG(400, 600),
			wantFilename: "main.epub.cover.jpg",
			wantMime:     "image/jpeg",
		},
		{
			// A PNG mislabeled by the remote server is normalized to PNG on
			// disk. The stored MIME type must describe the stored bytes, not
			// echo the download's Content-Type.
			name:         "mislabeled png stores normalized mime",
			contentType:  "image/webp",
			body:         makeApplyTestPNG(400, 600),
			wantFilename: "main.epub.cover.png",
			wantMime:     "image/png",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeEPUB)
			require.NoError(t, os.WriteFile(file.Filepath, []byte("fake epub"), 0600))
			srv := newCoverImageServer(t, http.StatusOK, tc.contentType, tc.body)
			store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
			h := newCoverApplyTestHandler(store, srv, nil)

			c := newApplyEchoContext(t, map[string]any{"cover_url": srv.URL + "/cover"})
			require.NoError(t, h.applyMetadata(c))

			require.Len(t, store.updatedFileColumns, 1, "cover state must be written in one UpdateFile call")
			assert.ElementsMatch(t, []string{"cover_image_filename", "cover_mime_type", "cover_source"}, store.updatedFileColumns[0])

			require.NotNil(t, file.CoverImageFilename)
			assert.Equal(t, tc.wantFilename, *file.CoverImageFilename, "stored filename must be a bare filename")
			assert.Equal(t, filepath.Base(*file.CoverImageFilename), *file.CoverImageFilename)
			require.NotNil(t, file.CoverMimeType)
			assert.Equal(t, tc.wantMime, *file.CoverMimeType)
			require.NotNil(t, file.CoverSource)
			assert.Equal(t, models.PluginDataSource("test", "enricher"), *file.CoverSource)
			assert.Nil(t, file.CoverPage, "image-based covers have no page")

			_, err := os.Stat(filepath.Join(filepath.Dir(file.Filepath), tc.wantFilename))
			require.NoError(t, err, "cover must be written next to the file")
		})
	}
}

func TestApplyMetadata_Cover_ImageBased_ResolvesRelativeToFile(t *testing.T) {
	t.Parallel()

	// The book path is a synthetic organized-folder path that does not
	// exist on disk; the cover must still land next to the file.
	libraryDir := t.TempDir()
	filePath := filepath.Join(libraryDir, "book.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("fake epub"), 0600))
	book := &models.Book{ID: 1, LibraryID: 1, Title: "Book", Filepath: filepath.Join(libraryDir, "Author", "Book")}
	file := &models.File{ID: 1, BookID: 1, LibraryID: 1, Filepath: filePath, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain}
	book.Files = []*models.File{file}

	srv := newCoverImageServer(t, http.StatusOK, "image/jpeg", makePersistTestJPEG(400, 600))
	store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
	h := newCoverApplyTestHandler(store, srv, nil)

	c := newApplyEchoContext(t, map[string]any{"cover_url": srv.URL + "/cover"})
	require.NoError(t, h.applyMetadata(c))

	_, err := os.Stat(filepath.Join(libraryDir, "book.epub.cover.jpg"))
	require.NoError(t, err)
	_, err = os.Stat(book.Filepath)
	assert.True(t, os.IsNotExist(err), "synthetic book path must not be created")
	require.NotNil(t, file.CoverSource)
	assert.Equal(t, models.PluginDataSource("test", "enricher"), *file.CoverSource)
}

func TestApplyMetadata_Cover_ImageBased_FailedDownloadLeavesPreviousIntact(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeEPUB)
	prevFilename, prevMime, prevSource := "main.epub.cover.png", "image/png", models.DataSourceManual
	file.CoverImageFilename = &prevFilename
	file.CoverMimeType = &prevMime
	file.CoverSource = &prevSource

	srv := newCoverImageServer(t, http.StatusNotFound, "text/html", []byte("missing"))
	store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
	h := newCoverApplyTestHandler(store, srv, nil)

	c := newApplyEchoContext(t, map[string]any{"cover_url": srv.URL + "/cover"})
	require.NoError(t, h.applyMetadata(c))

	assert.Empty(t, store.updatedFileColumns, "no file columns should be written after a failed download")
	assert.Equal(t, prevFilename, *file.CoverImageFilename)
	assert.Equal(t, prevMime, *file.CoverMimeType)
	assert.Equal(t, prevSource, *file.CoverSource)
}

func TestApplyMetadata_Cover_ImageBased_FailedWriteLeavesPreviousIntact(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeEPUB)
	// Point the file at a directory that does not exist so the cover write
	// fails. The book path is also missing, so nothing can be created.
	file.Filepath = filepath.Join(book.Filepath, "missing", "main.epub")
	book.Filepath = filepath.Join(book.Filepath, "missing")
	prevFilename, prevMime, prevSource := "main.epub.cover.png", "image/png", models.DataSourceManual
	file.CoverImageFilename = &prevFilename
	file.CoverMimeType = &prevMime
	file.CoverSource = &prevSource

	srv := newCoverImageServer(t, http.StatusOK, "image/jpeg", makePersistTestJPEG(400, 600))
	store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
	h := newCoverApplyTestHandler(store, srv, nil)

	c := newApplyEchoContext(t, map[string]any{"cover_url": srv.URL + "/cover"})
	require.NoError(t, h.applyMetadata(c))

	assert.Empty(t, store.updatedFileColumns, "no file columns should be written after a failed cover write")
	assert.Equal(t, prevFilename, *file.CoverImageFilename)
	assert.Equal(t, prevMime, *file.CoverMimeType)
	assert.Equal(t, prevSource, *file.CoverSource)
}

func TestApplyMetadata_Cover_PageBased_WritesCompleteProvenance(t *testing.T) {
	t.Parallel()

	for _, fileType := range []string{models.FileTypeCBZ, models.FileTypePDF} {
		t.Run(fileType, func(t *testing.T) {
			t.Parallel()

			book, file := newApplyTestBookWithFile(t, "Book", fileType)
			pageCount := 10
			file.PageCount = &pageCount
			prevPage, prevFilename, prevMime, prevSource := 0, "main."+fileType+".cover.png", "image/png", models.DataSourceManual
			file.CoverPage = &prevPage
			file.CoverImageFilename = &prevFilename
			file.CoverMimeType = &prevMime
			file.CoverSource = &prevSource

			extractor := &stubPageExtractor{filename: "main." + fileType + ".cover.jpg", mimeType: "image/jpeg"}
			store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
			h := newCoverApplyTestHandler(store, nil, extractor)

			c := newApplyEchoContext(t, map[string]any{"cover_page": 3})
			require.NoError(t, h.applyMetadata(c))

			require.Len(t, extractor.calls, 1)
			assert.Equal(t, 3, extractor.calls[0].Page)

			require.Len(t, store.updatedFileColumns, 1, "cover state must be written in one UpdateFile call")
			assert.ElementsMatch(t, coverColumns, store.updatedFileColumns[0])

			require.NotNil(t, file.CoverPage)
			assert.Equal(t, 3, *file.CoverPage)
			require.NotNil(t, file.CoverImageFilename)
			assert.Equal(t, "main."+fileType+".cover.jpg", *file.CoverImageFilename)
			require.NotNil(t, file.CoverMimeType)
			assert.Equal(t, "image/jpeg", *file.CoverMimeType)
			require.NotNil(t, file.CoverSource)
			assert.Equal(t, models.PluginDataSource("test", "enricher"), *file.CoverSource)
		})
	}
}

func TestApplyMetadata_Cover_PageBased_SamePageIsNoOp(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeCBZ)
	pageCount := 10
	file.PageCount = &pageCount
	prevPage, prevFilename, prevMime, prevSource := 3, "main.cbz.cover.jpg", "image/jpeg", models.DataSourceManual
	file.CoverPage = &prevPage
	file.CoverImageFilename = &prevFilename
	file.CoverMimeType = &prevMime
	file.CoverSource = &prevSource

	extractor := &stubPageExtractor{filename: "main.cbz.cover.jpg", mimeType: "image/jpeg"}
	store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
	h := newCoverApplyTestHandler(store, nil, extractor)

	c := newApplyEchoContext(t, map[string]any{"cover_page": 3})
	require.NoError(t, h.applyMetadata(c))

	assert.Empty(t, extractor.calls, "choosing the current page must not re-extract")
	assert.Empty(t, store.updatedFileColumns, "choosing the current page must not write file columns")
	assert.Equal(t, 3, *file.CoverPage)
	assert.Equal(t, prevFilename, *file.CoverImageFilename)
	assert.Equal(t, prevMime, *file.CoverMimeType)
	assert.Equal(t, models.DataSourceManual, *file.CoverSource, "existing provenance must be preserved")
}

func TestApplyMetadata_Cover_PageBased_SamePageWithoutImageExtracts(t *testing.T) {
	t.Parallel()

	// cover_page is set but the cover image is missing (for example the
	// extraction failed on an earlier scan). Identity alone is not enough:
	// the page must be extracted so the file actually gets a cover.
	book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeCBZ)
	pageCount := 10
	file.PageCount = &pageCount
	prevPage := 3
	file.CoverPage = &prevPage

	extractor := &stubPageExtractor{filename: "main.cbz.cover.jpg", mimeType: "image/jpeg"}
	store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
	h := newCoverApplyTestHandler(store, nil, extractor)

	c := newApplyEchoContext(t, map[string]any{"cover_page": 3})
	require.NoError(t, h.applyMetadata(c))

	require.Len(t, extractor.calls, 1)
	require.Len(t, store.updatedFileColumns, 1)
	assert.ElementsMatch(t, coverColumns, store.updatedFileColumns[0])
	require.NotNil(t, file.CoverSource)
	assert.Equal(t, models.PluginDataSource("test", "enricher"), *file.CoverSource)
}

func TestApplyMetadata_Cover_PageBased_FailedExtractionLeavesPreviousIntact(t *testing.T) {
	t.Parallel()

	book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeCBZ)
	pageCount := 10
	file.PageCount = &pageCount
	prevPage, prevFilename, prevMime, prevSource := 0, "main.cbz.cover.jpg", "image/jpeg", models.DataSourceManual
	file.CoverPage = &prevPage
	file.CoverImageFilename = &prevFilename
	file.CoverMimeType = &prevMime
	file.CoverSource = &prevSource

	extractor := &stubPageExtractor{wantErr: errors.New("boom")}
	store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
	h := newCoverApplyTestHandler(store, nil, extractor)

	c := newApplyEchoContext(t, map[string]any{"cover_page": 5})
	require.NoError(t, h.applyMetadata(c))

	assert.Empty(t, store.updatedFileColumns, "no file columns should be written after a failed extraction")
	assert.Equal(t, 0, *file.CoverPage)
	assert.Equal(t, prevFilename, *file.CoverImageFilename)
	assert.Equal(t, prevMime, *file.CoverMimeType)
	assert.Equal(t, prevSource, *file.CoverSource)
}

func TestApplyMetadata_Cover_KeepCurrent_ChangesNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileType string
		page     *int
	}{
		{name: "image-based", fileType: models.FileTypeEPUB},
		{name: "page-based", fileType: models.FileTypeCBZ, page: new(int)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			book, file := newApplyTestBookWithFile(t, "Book", tc.fileType)
			pageCount := 10
			file.PageCount = &pageCount
			prevFilename, prevMime, prevSource := "main."+tc.fileType+".cover.jpg", "image/jpeg", models.DataSourceManual
			file.CoverPage = tc.page
			file.CoverImageFilename = &prevFilename
			file.CoverMimeType = &prevMime
			file.CoverSource = &prevSource

			extractor := &stubPageExtractor{filename: "unexpected.jpg", mimeType: "image/jpeg"}
			store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
			h := newCoverApplyTestHandler(store, nil, extractor)

			// Keeping the current Cover sends no cover field. Another field is
			// applied so the request itself is not a no-op.
			c := newApplyEchoContext(t, map[string]any{"description": "A new description"})
			require.NoError(t, h.applyMetadata(c))

			assert.Empty(t, extractor.calls)
			for _, columns := range store.updatedFileColumns {
				assert.False(t, hasCoverColumn(columns), "no cover column should be written: %v", columns)
			}
			assert.Equal(t, tc.page, file.CoverPage)
			assert.Equal(t, prevFilename, *file.CoverImageFilename)
			assert.Equal(t, prevMime, *file.CoverMimeType)
			assert.Equal(t, prevSource, *file.CoverSource)
		})
	}
}

func TestApplyMetadata_Cover_ImageBased_RemovesStaleCoverWithOtherExtension(t *testing.T) {
	t.Parallel()

	// The previous cover is a PNG; the proposed cover is a JPEG. The stale
	// PNG must not linger next to the new JPEG, otherwise a later scan could
	// adopt it again by base name.
	book, file := newApplyTestBookWithFile(t, "Book", models.FileTypeEPUB)
	require.NoError(t, os.WriteFile(file.Filepath, []byte("fake epub"), 0600))
	stalePath := filepath.Join(book.Filepath, "main.epub.cover.png")
	require.NoError(t, os.WriteFile(stalePath, makeApplyTestPNG(10, 10), 0600))
	prevFilename, prevMime, prevSource := "main.epub.cover.png", "image/png", models.DataSourceManual
	file.CoverImageFilename = &prevFilename
	file.CoverMimeType = &prevMime
	file.CoverSource = &prevSource

	srv := newCoverImageServer(t, http.StatusOK, "image/jpeg", makePersistTestJPEG(300, 450))
	store := &stubBookStoreForApply{stubBookStoreForPersist: stubBookStoreForPersist{book: book}}
	h := newCoverApplyTestHandler(store, srv, nil)

	c := newApplyEchoContext(t, map[string]any{"cover_url": srv.URL + "/cover"})
	require.NoError(t, h.applyMetadata(c))

	_, err := os.Stat(filepath.Join(book.Filepath, "main.epub.cover.jpg"))
	require.NoError(t, err)
	_, err = os.Stat(stalePath)
	assert.True(t, os.IsNotExist(err), "stale cover with the old extension must be removed")
	assert.Equal(t, "main.epub.cover.jpg", *file.CoverImageFilename)
	assert.Equal(t, "image/jpeg", *file.CoverMimeType)
	assert.Equal(t, models.PluginDataSource("test", "enricher"), *file.CoverSource)
}
