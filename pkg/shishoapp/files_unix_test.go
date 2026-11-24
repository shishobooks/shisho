//go:build unix

package shishoapp

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A download cache that can't take the build, here a read-only one, still
// shares the Generated File, streamed while it's built.
func TestShare_StreamsWhenTheCacheCantTakeIt(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes to read-only directories")
	}
	readOnly := t.TempDir()
	require.NoError(t, os.Chmod(readOnly, 0o555))
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o755) })
	f := newFixtureWithCache(t, downloadcache.NewCache(readOnly, 1<<30, downloadcache.WithGenerators(func(string, string) (filegen.StreamGenerator, error) {
		return filegentest.Copy{}, nil
	})))
	ebook := filepath.Join(t.TempDir(), "dune.epub")
	require.NoError(t, os.WriteFile(ebook, []byte(filegentest.CopyMagic+"epub"), 0o600))
	f.addLibrary(1, "Mine")
	f.addBook(1, 1, "Dune")
	f.exec(`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes) VALUES (6, 1, 1, ?, 'epub', 8)`, ebook)
	f.addUser()
	credential := f.signIn()

	rec := f.rangeCall(http.MethodGet, "/mobile/files/6/share", credential, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, filegentest.CopyHeader+filegentest.CopyMagic+"epub", rec.Body.String())
}
