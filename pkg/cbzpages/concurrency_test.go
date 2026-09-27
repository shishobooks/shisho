package cbzpages

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCBZ writes a CBZ containing the given pages in order and returns its path.
func writeCBZ(t *testing.T, dir string, pages ...[]byte) string {
	t.Helper()
	path := filepath.Join(dir, "book.cbz")
	f, err := os.Create(path)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	for i, page := range pages {
		// Stored, not deflated, so tests can corrupt the bytes in place.
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:   filepath.Join("pages", string(rune('a'+i))+".jpg"),
			Method: zip.Store,
		})
		require.NoError(t, err)
		_, err = w.Write(page)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	return path
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return b
}

func TestCache_GetPage_ConcurrentReadersSeeCompletePage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	page := randomBytes(t, 8<<20)
	cbzPath := writeCBZ(t, dir, page)
	c := NewCache(filepath.Join(dir, "cache"))

	const readers = 16
	start := make(chan struct{})
	contents := make([][]byte, readers)
	errs := make([]error, readers)
	var wg sync.WaitGroup
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			path, _, err := c.GetPage(cbzPath, 1, 0)
			if err != nil {
				errs[i] = err
				return
			}
			contents[i], errs[i] = os.ReadFile(path)
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < readers; i++ {
		require.NoError(t, errs[i], "reader %d", i)
		assert.True(t, bytes.Equal(page, contents[i]), "reader %d got %d of %d bytes", i, len(contents[i]), len(page))
	}

	entries, err := os.ReadDir(c.pageDir(1))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "page_0.jpg", entries[0].Name())
}

func TestCache_GetPage_IgnoresInProgressTempFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	page := []byte("full page bytes")
	cbzPath := writeCBZ(t, dir, page)
	c := NewCache(filepath.Join(dir, "cache"))

	// Simulate another request's unfinished extraction.
	pageDir := c.pageDir(1)
	require.NoError(t, os.MkdirAll(pageDir, 0755))
	stray, err := os.CreateTemp(pageDir, tempPagePattern(0))
	require.NoError(t, err)
	_, err = stray.WriteString("half")
	require.NoError(t, err)
	require.NoError(t, stray.Close())

	path, mime, err := c.GetPage(cbzPath, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(pageDir, "page_0.jpg"), path)
	assert.Equal(t, "image/jpeg", mime)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, page, data)

	// The other request's temp file is not ours to delete.
	_, err = os.Stat(stray.Name())
	assert.NoError(t, err)
}

func TestCache_GetPage_FailedExtractionLeavesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cbzPath := writeCBZ(t, dir, []byte("page bytes"))

	// Flip a byte in the stored data so the CRC check fails at the end of
	// the copy, after bytes have already been written.
	raw, err := os.ReadFile(cbzPath)
	require.NoError(t, err)
	idx := bytes.Index(raw, []byte("page bytes"))
	require.GreaterOrEqual(t, idx, 0, "writeCBZ must store pages uncompressed")
	raw[idx] ^= 0xff
	require.NoError(t, os.WriteFile(cbzPath, raw, 0600))

	c := NewCache(filepath.Join(dir, "cache"))
	_, _, err = c.GetPage(cbzPath, 1, 0)
	require.Error(t, err)

	entries, err := os.ReadDir(c.pageDir(1))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// pausingReader returns the first half of its data, signals halfway, and waits
// for proceed before returning the rest.
type pausingReader struct {
	data    []byte
	off     int
	halfway chan<- struct{}
	proceed <-chan struct{}
	paused  bool
}

func (r *pausingReader) Read(p []byte) (int, error) {
	half := len(r.data) / 2
	if r.off >= half && !r.paused {
		r.paused = true
		r.halfway <- struct{}{}
		<-r.proceed
	}
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	end := len(r.data)
	if !r.paused {
		end = half
	}
	n := copy(p, r.data[r.off:end])
	r.off += n
	return n, nil
}

func (r *pausingReader) Close() error { return nil }

func TestCache_GetPage_ReaderDuringExtractionGetsCompletePage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	page := randomBytes(t, 64<<10)
	cbzPath := writeCBZ(t, dir, page)
	c := NewCache(filepath.Join(dir, "cache"))

	halfway := make(chan struct{})
	proceed := make(chan struct{})
	var first sync.Once
	c.openEntry = func(f *zip.File) (io.ReadCloser, error) {
		paused := false
		first.Do(func() { paused = true })
		if !paused {
			return f.Open()
		}
		return &pausingReader{data: page, halfway: halfway, proceed: proceed}, nil
	}

	firstDone := make(chan error, 1)
	var firstPath string
	go func() {
		var err error
		firstPath, _, err = c.GetPage(cbzPath, 1, 0)
		firstDone <- err
	}()
	<-halfway // the first extraction has written half the page

	path, _, err := c.GetPage(cbzPath, 1, 0)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Len(t, data, len(page), "second request read a partially written page")

	close(proceed)
	require.NoError(t, <-firstDone)
	data, err = os.ReadFile(firstPath)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(page, data))

	entries, err := os.ReadDir(c.pageDir(1))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "page_0.jpg", entries[0].Name())
}
