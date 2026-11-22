package shishoapp

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A cover that exists but can't be opened, and a database that fails, are
// server faults: a 500 with a JSON body and no image headers. That holds
// for a Book's cover and for a File's own.
func TestCover_FaultsAreServerErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	dir := t.TempDir()
	cover := filepath.Join(dir, "a.epub.cover.png")
	require.NoError(t, os.WriteFile(cover, tinyPNG(t), 0o600))
	f.addLibrary(1, "Mine")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, cover_image_filename) VALUES (1, 1, 1, ?, 'epub', 1, 'a.epub.cover.png')`,
		filepath.Join(dir, "a.epub"))
	f.addUser()
	credential := f.signIn()
	paths := []string{"/mobile/books/1/cover?v=1-1", "/mobile/files/1/cover?v=1-1"}

	// Permission bits don't restrict root, so only the database fault runs
	// there.
	if os.Geteuid() != 0 {
		require.NoError(t, os.Chmod(cover, 0o000))
		t.Cleanup(func() { _ = os.Chmod(cover, 0o600) })
		for _, path := range paths {
			assertServerFault(t, f.do(call{path: path, credential: credential}))
		}
	}

	f.exec(`DROP TABLE files`)
	for _, path := range paths {
		assertServerFault(t, f.do(call{path: path, credential: credential}))
	}
}

// The same for Original File downloads and streams: an unreadable file and
// a failing database are server faults, never a 404 or a partial file.
func TestFiles_FaultsAreServerErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	dir := t.TempDir()
	audio := filepath.Join(dir, "a.m4b")
	require.NoError(t, os.WriteFile(audio, []byte("audio"), 0o600))
	f.addLibrary(1, "Mine")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (1, 1, 1, ?, 'm4b', 5)`, audio)
	f.addUser()
	credential := f.signIn()
	paths := []string{"/mobile/files/1/download", "/mobile/files/1/stream"}

	if os.Geteuid() != 0 {
		require.NoError(t, os.Chmod(audio, 0o000))
		t.Cleanup(func() { _ = os.Chmod(audio, 0o600) })
		for _, path := range paths {
			assertServerFault(t, f.do(call{path: path, credential: credential}))
		}
	}

	f.exec(`DROP TABLE files`)
	for _, path := range paths {
		assertServerFault(t, f.do(call{path: path, credential: credential}))
	}
}

func assertServerFault(t *testing.T, rec interface {
	Result() *http.Response
}) {
	t.Helper()
	resp := rec.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Empty(t, resp.Header.Get("Cache-Control"))
	assert.Empty(t, resp.Header.Get("ETag"))
}

// A database that fails while a scoped fetch checks access is a server
// fault, never the 404 for a Library out of reach.
func TestFeed_ScopedFetchFaultsAreServerErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.addLibrary(1, "Mine")
	f.addUser()
	credential := f.signIn()
	f.exec(`DROP TABLE list_shares`)
	for _, path := range []string{"/mobile/changes?cursor=0&library_id=1", "/mobile/changes?cursor=0&list_id=1"} {
		assertServerFault(t, f.do(call{path: path, credential: credential}))
	}
}
