package shishoapp

import (
	"archive/zip"
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rangeCall asks for bytes of path with the Device's credential.
func (f *fixture) rangeCall(method, path, credential, byteRange string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(HeaderAppProtocol, strconv.Itoa(Protocol))
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	return rec
}

// Original Files download and stream through the Device's credential, with
// Range support, and only from the user's Libraries.
func TestFiles_DownloadAndStreamWithRanges(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	dir := t.TempDir()
	audio := filepath.Join(dir, "dune.m4b")
	ebook := filepath.Join(dir, "dune.epub")
	require.NoError(t, os.WriteFile(audio, []byte("0123456789abcdef"), 0o600))
	require.NoError(t, os.WriteFile(ebook, []byte("epub bytes"), 0o600))
	f.addLibrary(1, "Mine")
	f.addLibrary(2, "Theirs")
	f.addBook(1, 1, "Dune")
	f.addBook(2, 2, "Emma")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES
		(5, 1, 1, ?, 'm4b', 16), (6, 1, 1, ?, 'epub', 10), (7, 2, 2, ?, 'm4b', 16), (8, 1, 1, ?, 'm4b', 16)`,
		audio, ebook, audio, filepath.Join(dir, "gone.m4b"))
	f.addUser(1)
	credential := f.signIn()

	for _, path := range []string{"/mobile/files/5/download", "/mobile/files/5/stream"} {
		rec := f.rangeCall(http.MethodGet, path, credential, "")
		require.Equal(t, http.StatusOK, rec.Code, path)
		assert.Equal(t, "0123456789abcdef", rec.Body.String(), path)
		assert.Equal(t, "bytes", rec.Header().Get("Accept-Ranges"), path)
		assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"), path)

		rec = f.rangeCall(http.MethodGet, path, credential, "bytes=10-")
		require.Equal(t, http.StatusPartialContent, rec.Code, path)
		assert.Equal(t, "abcdef", rec.Body.String(), path)
		assert.Equal(t, "bytes 10-15/16", rec.Header().Get("Content-Range"), path)

		rec = f.rangeCall(http.MethodHead, path, credential, "")
		require.Equal(t, http.StatusOK, rec.Code, path)
		assert.Equal(t, "16", rec.Header().Get("Content-Length"), path)
	}
	stream := f.rangeCall(http.MethodGet, "/mobile/files/5/stream", credential, "")
	assert.Equal(t, "audio/mp4", stream.Header().Get("Content-Type"))
	assert.Empty(t, stream.Header().Get("Content-Disposition"))
	download := f.rangeCall(http.MethodGet, "/mobile/files/5/download", credential, "")
	assert.Contains(t, download.Header().Get("Content-Disposition"), "attachment")

	// An ebook downloads but doesn't stream.
	assert.Equal(t, http.StatusOK, f.rangeCall(http.MethodGet, "/mobile/files/6/download", credential, "").Code)
	assert.Equal(t, http.StatusNotFound, f.rangeCall(http.MethodGet, "/mobile/files/6/stream", credential, "").Code)
	// Another Library's File, a File missing from disk, and one that
	// doesn't exist are all 404.
	for _, path := range []string{"/mobile/files/7/download", "/mobile/files/7/stream", "/mobile/files/8/stream", "/mobile/files/99/download"} {
		assert.Equal(t, http.StatusNotFound, f.rangeCall(http.MethodGet, path, credential, "").Code, path)
	}
	// Without the credential, nothing.
	assert.Equal(t, http.StatusUnauthorized, f.rangeCall(http.MethodGet, "/mobile/files/5/stream", "", "").Code)
}

// AVPlayer can't set headers, so a stream also takes the credential and
// protocol from cookies the app hands it. Nothing else does.
func TestFiles_StreamTakesTheCredentialFromACookie(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	audio := filepath.Join(t.TempDir(), "dune.m4b")
	require.NoError(t, os.WriteFile(audio, []byte("0123456789"), 0o600))
	f.addLibrary(1, "Mine")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (5, 1, 1, ?, 'm4b', 10)`, audio)
	f.addUser()
	credential := f.signIn()

	withCookies := func(path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Range", "bytes=2-")
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		f.e.ServeHTTP(rec, req)
		return rec
	}
	credentialCookie := &http.Cookie{Name: CookieCredential, Value: credential}
	protocolCookie := &http.Cookie{Name: CookieProtocol, Value: strconv.Itoa(Protocol)}

	rec := withCookies("/mobile/files/5/stream", credentialCookie, protocolCookie)
	require.Equal(t, http.StatusPartialContent, rec.Code, rec.Body.String())
	assert.Equal(t, "23456789", rec.Body.String())

	assert.Equal(t, http.StatusUnauthorized, withCookies("/mobile/files/5/stream").Code)
	assert.Equal(t, http.StatusUpgradeRequired, withCookies("/mobile/files/5/stream", credentialCookie).Code)
	assert.Equal(t, http.StatusUnauthorized, withCookies("/mobile/files/5/download", credentialCookie, protocolCookie).Code)
	assert.Equal(t, http.StatusUnauthorized, withCookies("/mobile/changes", credentialCookie, protocolCookie).Code)
}

// A File's own cover, keyed by its version so it caches as immutable, and
// only from the user's Libraries.
func TestFiles_CoverIsTheFilesOwn(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.m4b.cover.png"), tinyPNG(t), 0o600))
	f.addLibrary(1, "Mine")
	f.addLibrary(2, "Theirs")
	f.addBook(1, 1, "Dune")
	f.addBook(2, 2, "Emma")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, cover_image_filename) VALUES
		(5, 1, 1, ?, 'm4b', 1, 'a.m4b.cover.png'), (6, 1, 1, ?, 'epub', 1, NULL), (7, 2, 2, ?, 'm4b', 1, 'a.m4b.cover.png')`,
		filepath.Join(dir, "a.m4b"), filepath.Join(dir, "b.epub"), filepath.Join(dir, "a.m4b"))
	f.addUser(1)
	credential := f.signIn()

	rec := f.do(call{path: "/mobile/files/5/cover?v=5-1", credential: credential})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "private, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
	assert.Equal(t, http.StatusNotFound, f.do(call{path: "/mobile/files/6/cover?v=1", credential: credential}).Code)
	assert.Equal(t, http.StatusNotFound, f.do(call{path: "/mobile/files/7/cover?v=1", credential: credential}).Code)
	assert.Equal(t, http.StatusUnauthorized, f.do(call{path: "/mobile/files/5/cover?v=1"}).Code)
}

// Sharing a File out of the app sends its Generated File, so the copy
// carries Shisho's current metadata (ADR 0022). The server builds it into
// the download cache before sending anything, so even the first request
// for a generator that can't tell its size up front gets the size, and a
// Range gets the bytes it asked for.
func TestShare_PreparesTheGeneratedFileAndServesRanges(t *testing.T) {
	t.Parallel()
	f := newFixture(t, downloadcache.WithGenerators(func(string, string) (filegen.StreamGenerator, error) {
		return filegentest.Copy{UnknownSize: true}, nil
	}))
	dir := t.TempDir()
	ebook := filepath.Join(dir, "dune.epub")
	other := filepath.Join(dir, "emma.epub")
	require.NoError(t, os.WriteFile(ebook, []byte(filegentest.CopyMagic+"epub"), 0o600))
	require.NoError(t, os.WriteFile(other, []byte(filegentest.CopyMagic+"epub"), 0o600))
	f.addLibrary(1, "Mine")
	f.addLibrary(2, "Theirs")
	f.addBook(1, 1, "Dune")
	f.addBook(2, 2, "Emma")
	f.addBook(3, 1, "Emma")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES
		(6, 1, 1, ?, 'epub', 8), (7, 2, 2, ?, 'epub', 8), (8, 1, 3, ?, 'epub', 8)`, ebook, ebook, other)
	f.addUser(1)
	credential := f.signIn()
	generated := filegentest.CopyHeader + filegentest.CopyMagic + "epub"
	size := strconv.Itoa(len(generated))

	// The first request, before anything was built, asks for a Range.
	rec := f.rangeCall(http.MethodGet, "/mobile/files/6/share", credential, "bytes=10-")
	require.Equal(t, http.StatusPartialContent, rec.Code, rec.Body.String())
	assert.Equal(t, generated[10:], rec.Body.String())
	assert.Equal(t, "bytes 10-"+strconv.Itoa(len(generated)-1)+"/"+size, rec.Header().Get("Content-Range"))

	rec = f.rangeCall(http.MethodGet, "/mobile/files/6/share", credential, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, generated, rec.Body.String())
	assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "application/epub+zip", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")

	// A HEAD before anything was built prepares the file and knows its size.
	rec = f.rangeCall(http.MethodHead, "/mobile/files/8/share", credential, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, size, rec.Header().Get("Content-Length"))
	assert.Equal(t, "bytes", rec.Header().Get("Accept-Ranges"))

	// Another Library's File, one that doesn't exist, and no credential.
	assert.Equal(t, http.StatusNotFound, f.rangeCall(http.MethodGet, "/mobile/files/7/share", credential, "").Code)
	assert.Equal(t, http.StatusNotFound, f.rangeCall(http.MethodGet, "/mobile/files/99/share", credential, "").Code)
	assert.Equal(t, http.StatusUnauthorized, f.rangeCall(http.MethodGet, "/mobile/files/6/share", "", "").Code)
}

// A File that can't be generated as itself is shared as its Original File,
// as a device's download is: a supplement, and a build that fails.
func TestShare_FallsBackToTheOriginal(t *testing.T) {
	t.Parallel()
	failing := filegentest.NewPausing("", "")
	failing.PrepareErr = filegen.NewGenerationError(models.FileTypeEPUB, errors.New("unreadable"), "the file can't be read")
	f := newFixture(t, downloadcache.WithGenerators(func(string, string) (filegen.StreamGenerator, error) {
		return failing, nil
	}))
	dir := t.TempDir()
	supplement := filepath.Join(dir, "extras.pdf")
	broken := filepath.Join(dir, "broken.epub")
	require.NoError(t, os.WriteFile(supplement, []byte(filegentest.CopyMagic+"pdf"), 0o600))
	require.NoError(t, os.WriteFile(broken, []byte("no magic"), 0o600))
	f.addLibrary(1, "Mine")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, file_role, filesize_bytes) VALUES
		(6, 1, 1, ?, 'pdf', 'supplement', 7), (7, 1, 1, ?, 'epub', 'main', 8)`, supplement, broken)
	f.addUser()
	credential := f.signIn()

	rec := f.rangeCall(http.MethodGet, "/mobile/files/6/share", credential, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, filegentest.CopyMagic+"pdf", rec.Body.String())

	rec = f.rangeCall(http.MethodGet, "/mobile/files/7/share", credential, "bytes=3-")
	require.Equal(t, http.StatusPartialContent, rec.Code, rec.Body.String())
	assert.Equal(t, "magic", rec.Body.String())
}

// A comic's or PDF's pages come as thumbnails, sized as covers are, for the
// app's page grid before the File is downloaded, and only from the user's
// Libraries.
func TestFiles_PageThumbnailsForTheGrid(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	dir := t.TempDir()
	comic := testgen.GenerateCBZ(t, dir, "comic.cbz", testgen.CBZOptions{PageCount: 3})
	manual := testgen.GeneratePDF(t, dir, "manual.pdf", testgen.PDFOptions{PageCount: 2})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "novel.epub"), []byte("epub"), 0o600))
	odd := filepath.Join(dir, "odd.cbz")
	writeZip(t, odd, map[string][]byte{"page.jpg": []byte("not an image")})
	f.addLibrary(1, "Mine")
	f.addLibrary(2, "Theirs")
	f.addBook(1, 1, "Comic")
	f.addBook(2, 2, "Theirs")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, page_count) VALUES
		(5, 1, 1, ?, 'cbz', 1, 3), (6, 1, 1, ?, 'pdf', 1, 2), (7, 2, 2, ?, 'cbz', 1, 3), (8, 1, 1, ?, 'epub', 1, NULL),
		(9, 1, 1, ?, 'cbz', 1, 1)`,
		comic, manual, comic, filepath.Join(dir, "novel.epub"), odd)
	f.addUser(1)
	credential := f.signIn()

	for _, path := range []string{"/mobile/files/5/pages/1?size=256", "/mobile/files/6/pages/1?size=256"} {
		rec := f.do(call{path: path, credential: credential})
		require.Equal(t, http.StatusOK, rec.Code, path+": "+rec.Body.String())
		img, _, err := image.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
		require.NoError(t, err, path)
		assert.LessOrEqual(t, max(img.Width, img.Height), 256, path)
	}
	// A page past the end, another user's Library, a File without pages,
	// and no credential.
	assert.Equal(t, http.StatusNotFound, f.do(call{path: "/mobile/files/5/pages/3?size=256", credential: credential}).Code)
	assert.Equal(t, http.StatusNotFound, f.do(call{path: "/mobile/files/7/pages/0?size=256", credential: credential}).Code)
	assert.Equal(t, http.StatusUnauthorized, f.do(call{path: "/mobile/files/5/pages/0?size=256"}).Code)
	for _, tt := range []struct {
		path string
		code string
	}{
		// Thumbnails only: the reader draws its pages from the File
		// (ADR 0025), so a full page isn't served here.
		{"/mobile/files/5/pages/0", "validation_error"},
		{"/mobile/files/5/pages/0?size=300", "validation_error"},
		{"/mobile/files/5/pages/x?size=256", "validation_error"},
		{"/mobile/files/8/pages/0?size=256", "invalid_state"},
		// A page the server can't make a thumbnail of isn't sent whole.
		{"/mobile/files/9/pages/0?size=256", "invalid_state"},
	} {
		rec := f.do(call{path: tt.path, credential: credential})
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, tt.path)
		assert.Equal(t, tt.code, errorCode(t, rec), tt.path)
	}
}

// writeZip writes an archive of entries to path.
func writeZip(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	out, err := os.Create(path)
	require.NoError(t, err)
	w := zip.NewWriter(out)
	for name, data := range entries {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.NoError(t, out.Close())
}
