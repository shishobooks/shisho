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
// server faults: a 500 with a JSON body and no image headers.
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

	require.NoError(t, os.Chmod(cover, 0o000))
	t.Cleanup(func() { _ = os.Chmod(cover, 0o600) })
	rec := f.do(call{path: "/mobile/books/1/cover?v=1-1", credential: credential})
	assertServerFault(t, rec)

	f.exec(`DROP TABLE files`)
	rec = f.do(call{path: "/mobile/books/1/cover?v=1-1", credential: credential})
	assertServerFault(t, rec)
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
