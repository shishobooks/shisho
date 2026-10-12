package worker

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mimetypeLayout is how a non-conforming EPUB holds its mimetype entry.
type mimetypeLayout int

const (
	// mimetypeMissing leaves the entry out.
	mimetypeMissing mimetypeLayout = iota
	// mimetypeDeflatedLast compresses the entry and puts it last.
	mimetypeDeflatedLast
)

// writeNonConformingEPUB generates a valid EPUB titled "Loose EPUB" and
// rewrites it with its mimetype entry missing or compressed and last, as
// hand-zipped and some older tools' EPUBs are. Content sniffing reads such
// a file as a plain zip.
func writeNonConformingEPUB(t *testing.T, dir, filename string, layout mimetypeLayout) string {
	t.Helper()
	src := testgen.GenerateEPUB(t, t.TempDir(), filename, testgen.EPUBOptions{
		Title:   "Loose EPUB",
		Authors: []string{"Test Author"},
	})
	reader, err := zip.OpenReader(src)
	require.NoError(t, err)
	defer reader.Close()

	dst := filepath.Join(dir, filename)
	out, err := os.Create(dst)
	require.NoError(t, err)
	writer := zip.NewWriter(out)
	copyEntry := func(entry *zip.File) {
		rc, err := entry.Open()
		require.NoError(t, err)
		w, err := writer.Create(entry.Name)
		require.NoError(t, err)
		_, err = io.Copy(w, rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
	}
	var mimetype *zip.File
	for _, entry := range reader.File {
		if entry.Name == "mimetype" {
			mimetype = entry
			continue
		}
		copyEntry(entry)
	}
	if layout == mimetypeDeflatedLast {
		require.NotNil(t, mimetype)
		copyEntry(mimetype)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, out.Close())
	return dst
}

func TestCheckExpectedMimeType_NonConformingEPUB(t *testing.T) {
	t.Parallel()
	for name, layout := range map[string]mimetypeLayout{"missing": mimetypeMissing, "deflated last": mimetypeDeflatedLast} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := writeNonConformingEPUB(t, t.TempDir(), "book.epub", layout)
			mime, ok, err := checkExpectedMimeType(path)
			require.NoError(t, err)
			assert.Equal(t, "application/zip", mime)
			assert.True(t, ok)
		})
	}
}

// A library scan imports an EPUB whose mimetype entry is not first and
// stored, with its metadata.
func TestProcessScanJob_NonConformingEPUBIsImported(t *testing.T) {
	t.Parallel()
	for name, layout := range map[string]mimetypeLayout{"missing": mimetypeMissing, "deflated last": mimetypeDeflatedLast} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tc := newTestContext(t)
			libraryPath := testgen.TempLibraryDir(t)
			tc.createLibrary([]string{libraryPath})
			writeNonConformingEPUB(t, testgen.CreateSubDir(t, libraryPath, "Loose EPUB"), "book.epub", layout)

			tc.runScanWithJob()

			books := tc.listBooks()
			require.Len(t, books, 1)
			assert.Equal(t, "Loose EPUB", books[0].Title)
		})
	}
}

// A zip that is not an EPUB, named .epub, passes the mime check but is
// skipped with a warning instead of becoming a book.
func TestProcessScanJob_ZipNamedEPUBIsSkipped(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	dir := testgen.CreateSubDir(t, libraryPath, "Just A Zip")
	path := filepath.Join(dir, "archive.epub")
	f, err := os.Create(path)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create("notes.txt")
	require.NoError(t, err)
	_, err = io.WriteString(w, "not a book")
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	logs := tc.runScanWithJob()

	assert.Empty(t, tc.listBooks())
	warnings := warningsFor(logs, path)
	require.NotEmpty(t, warnings, "the skipped file is named in a warning")
	assert.Contains(t, *warnings[0].Data, "no opf file found")
}
