package shishoapp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

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
