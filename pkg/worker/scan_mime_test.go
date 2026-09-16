package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeMP4Header writes a minimal ISO base media file whose ftyp box carries
// the given major brand. That is all the mime detector inspects.
func writeMP4Header(t *testing.T, path, brand string) {
	t.Helper()
	require.Len(t, brand, 4)
	header := []byte{0x00, 0x00, 0x00, 0x1c, 'f', 't', 'y', 'p'}
	header = append(header, brand...)
	header = append(header, 0x00, 0x00, 0x02, 0x00)
	header = append(header, brand...)
	header = append(header, "iso2mp41"...)
	// Pad so the detector has enough bytes to work with.
	header = append(header, make([]byte, 64)...)
	require.NoError(t, os.WriteFile(path, header, 0o644))
}

func TestCheckExpectedMimeType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	t.Run("m4b with M4B brand is accepted", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(dir, "m4b-brand.m4b")
		writeMP4Header(t, path, "M4B ")
		mime, ok, err := checkExpectedMimeType(path)
		require.NoError(t, err)
		assert.True(t, ok, "detected %s", mime)
	})

	t.Run("m4b with M4A brand is accepted", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(dir, "m4a-brand.m4b")
		writeMP4Header(t, path, "M4A ")
		_, ok, err := checkExpectedMimeType(path)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("m4b with isom brand is accepted", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(dir, "isom-brand.m4b")
		writeMP4Header(t, path, "isom")
		_, ok, err := checkExpectedMimeType(path)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("epub containing plain text is rejected", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(dir, "not-really.epub")
		require.NoError(t, os.WriteFile(path, []byte("this is not a zip file at all\n"), 0o644))
		mime, ok, err := checkExpectedMimeType(path)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, "text/plain; charset=utf-8", mime)
	})

	t.Run("real epub is accepted", func(t *testing.T) {
		t.Parallel()
		path := testgen.GenerateEPUB(t, dir, "real.epub", testgen.EPUBOptions{Title: "Real"})
		mime, ok, err := checkExpectedMimeType(path)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "application/epub+zip", mime)
	})

	t.Run("unknown extension is accepted without detection", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(dir, "notes.txt")
		require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))
		_, ok, err := checkExpectedMimeType(path)
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

// TestScan_ImportsM4BBrandedAudiobook covers audiobooks whose container brand
// is "M4B " (detected as audio/mp4). These are real audiobooks written by
// tools that use the dedicated audiobook brand, and the library scan walker
// used to skip them with a mime type warning.
func TestScan_ImportsM4BBrandedAudiobook(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	m4bPath := testgen.GenerateM4B(t, libraryPath, "audiobook.m4b", testgen.M4BOptions{
		Title:  "Branded Audiobook",
		Artist: "Some Author",
	})

	// Rewrite the major brand in the ftyp box. The brand is a label only, so
	// the file stays a valid audiobook.
	f, err := os.OpenFile(m4bPath, os.O_RDWR, 0)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte("M4B "), 8)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	mime, expected, err := checkExpectedMimeType(m4bPath)
	require.NoError(t, err)
	require.Equal(t, "audio/mp4", mime)
	require.True(t, expected)

	require.NoError(t, tc.runScan())
	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, m4bPath, files[0].Filepath)
	assert.Nil(t, files[0].ScanError)
}
