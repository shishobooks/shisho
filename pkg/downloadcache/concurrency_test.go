package downloadcache

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readZipEntry returns the contents of name inside the zip at path. It fails
// the test if the zip is truncated or the entry is missing.
func readZipEntry(t *testing.T, path, name string) string {
	t.Helper()
	r, err := zip.OpenReader(path)
	require.NoError(t, err, "cached file %s is not a complete zip", path)
	defer r.Close()
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		defer rc.Close()
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		return string(data)
	}
	t.Fatalf("entry %s not found in %s", name, path)
	return ""
}

func TestCache_GetOrGenerate_ConcurrentColdReads(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))

	srcPath := filepath.Join(tmpDir, "source.epub")
	createTestEPUB(t, srcPath, "Original Title", []string{"Original Author"})

	cache := NewCache(cacheDir, 1<<30)
	t.Cleanup(cache.Wait)

	book := &models.Book{Title: "Shared Title"}
	file := &models.File{ID: 7, FileType: models.FileTypeEPUB, Filepath: srcPath}

	const readers = 16
	start := make(chan struct{})
	paths := make([]string, readers)
	errs := make([]error, readers)
	var wg sync.WaitGroup
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			paths[i], _, errs[i] = cache.GetOrGenerate(context.Background(), book, file)
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < readers; i++ {
		require.NoError(t, errs[i], "reader %d", i)
		assert.Contains(t, readZipEntry(t, paths[i], "content.opf"), "Shared Title")
	}

	meta, err := ReadMetadata(cacheDir, file.ID)
	require.NoError(t, err)
	require.NotNil(t, meta)

	// A later read is a cache hit on the same generation.
	path, _, err := cache.GetOrGenerate(context.Background(), book, file)
	require.NoError(t, err)
	assert.Equal(t, paths[0], path)
	after, err := ReadMetadata(cacheDir, file.ID)
	require.NoError(t, err)
	assert.True(t, meta.GeneratedAt.Equal(after.GeneratedAt))

	cache.Wait() // background cleanup holds a .cleanup.lock file while it runs
	assertNoStagingLeftovers(t, cacheDir)
}

func TestCache_GetOrGenerate_FailureLeavesNoPartialOutput(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))

	// Not a zip, so the EPUB generator fails after the cache has started.
	srcPath := filepath.Join(tmpDir, "broken.epub")
	require.NoError(t, os.WriteFile(srcPath, []byte("not a zip"), 0600))

	cache := NewCache(cacheDir, 1<<30)
	t.Cleanup(cache.Wait)

	file := &models.File{ID: 3, FileType: models.FileTypeEPUB, Filepath: srcPath}
	_, _, err := cache.GetOrGenerate(context.Background(), &models.Book{Title: "x"}, file)
	require.Error(t, err)

	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// assertNoStagingLeftovers fails if anything other than published cache
// entries remains in dir.
func assertNoStagingLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		name := e.Name()
		assert.False(t, e.IsDir(), "unexpected directory %s", name)
		assert.False(t, strings.HasSuffix(name, ".tmp"), "unexpected temp file %s", name)
		assert.False(t, strings.HasPrefix(name, "."), "unexpected hidden file %s", name)
	}
}

// pausingCache is a cache whose every generator is gen, with one source file
// on disk for it to open.
func pausingCache(t *testing.T, gen filegen.StreamGenerator) (*Cache, *models.Book, *models.File) {
	t.Helper()
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	src := filepath.Join(dir, "source.epub")
	require.NoError(t, os.WriteFile(src, []byte("source"), 0o600))
	cache := NewCache(cacheDir, 1<<30, WithGenerators(func(string, bool) (filegen.StreamGenerator, error) {
		return gen, nil
	}))
	t.Cleanup(cache.Wait)
	return cache, &models.Book{Title: "Book"}, &models.File{ID: 1, FileType: models.FileTypeEPUB, Filepath: src}
}

// readN reads exactly n bytes from r.
func readN(t *testing.T, r io.Reader, n int) string {
	t.Helper()
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	require.NoError(t, err)
	return string(buf)
}

// A download reads the file while it is built, and the partial file is
// never served as a cache hit.
func TestCache_Open_StreamsWhileGenerating(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first part,", " rest")
	cache, book, file := pausingCache(t, gen)

	g, err := cache.Open(context.Background(), book, file, false)
	require.NoError(t, err)
	require.NotNil(t, g.Body)
	defer g.Body.Close()
	assert.Empty(t, g.Path)
	assert.Equal(t, int64(len(gen.Output())), g.Size)

	assert.Equal(t, "first part,", readN(t, g.Body, len(gen.First)))

	hit, err := cache.GetCachedPath(file.ID, file.FileType, book, file)
	require.NoError(t, err)
	assert.Empty(t, hit, "a partial file was a cache hit")
	_, err = os.Stat(cachedFilename(cache.Dir(), file.ID, file.FileType))
	assert.True(t, os.IsNotExist(err), "the partial file was visible at the cached path")

	gen.Release()
	rest, err := io.ReadAll(g.Body)
	require.NoError(t, err)
	assert.Equal(t, " rest", string(rest))

	cache.Wait()
	hit, err = cache.GetCachedPath(file.ID, file.FileType, book, file)
	require.NoError(t, err)
	require.NotEmpty(t, hit)
	data, err := os.ReadFile(hit)
	require.NoError(t, err)
	assert.Equal(t, gen.Output(), data)
	assertNoStagingLeftovers(t, cache.Dir())
}

// A second download of the same file joins the build already running and
// gets bytes before it finishes.
func TestCache_Open_SecondDownloadJoinsTheBuild(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	cache, book, file := pausingCache(t, gen)

	a, err := cache.Open(context.Background(), book, file, false)
	require.NoError(t, err)
	defer a.Body.Close()
	<-gen.Started()
	b, err := cache.Open(context.Background(), book, file, false)
	require.NoError(t, err)
	defer b.Body.Close()

	assert.Equal(t, "first,", readN(t, a.Body, len(gen.First)))
	assert.Equal(t, "first,", readN(t, b.Body, len(gen.First)))
	gen.Release()
	restA, err := io.ReadAll(a.Body)
	require.NoError(t, err)
	restB, err := io.ReadAll(b.Body)
	require.NoError(t, err)
	assert.Equal(t, "second", string(restA))
	assert.Equal(t, "second", string(restB))
	assert.Equal(t, 1, gen.Prepares())
	assert.Equal(t, 1, gen.Writes())
}

// A download that goes away leaves the build running, so the file lands in
// the cache and the next download is a hit.
func TestCache_Open_DisconnectLeavesTheBuildRunning(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	cache, book, file := pausingCache(t, gen)

	ctx, cancel := context.WithCancel(context.Background())
	g, err := cache.Open(ctx, book, file, false)
	require.NoError(t, err)
	readN(t, g.Body, len(gen.First))
	cancel()
	_, err = g.Body.Read(make([]byte, 1))
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, g.Body.Close())

	gen.Release()
	cache.Wait()

	again, err := cache.Open(context.Background(), book, file, false)
	require.NoError(t, err)
	require.NotEmpty(t, again.Path, "the next download was not served from the cache")
	data, err := os.ReadFile(again.Path)
	require.NoError(t, err)
	assert.Equal(t, gen.Output(), data)
	assert.Equal(t, 1, gen.Writes())
}

// Head runs only the preparation step: it reports the size and creates
// nothing in the cache.
func TestCache_Head_PreparesOnly(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	cache, book, file := pausingCache(t, gen)

	g, err := cache.Head(context.Background(), book, file, false)
	require.NoError(t, err)
	assert.Nil(t, g.Body)
	assert.Empty(t, g.Path)
	assert.Equal(t, int64(len(gen.Output())), g.Size)
	assert.Equal(t, 1, gen.Prepares())
	assert.Equal(t, 0, gen.Writes())

	entries, err := os.ReadDir(cache.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// A size not known up front is reported as -1.
func TestCache_Head_UnknownSize(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	gen.UnknownSize = true
	cache, book, file := pausingCache(t, gen)

	g, err := cache.Head(context.Background(), book, file, false)
	require.NoError(t, err)
	assert.Equal(t, int64(-1), g.Size)
}

// A preparation failure is returned before anything is sent or cached, for
// downloads and for HEAD.
func TestCache_PrepareFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	for _, head := range []bool{false, true} {
		gen := filegentest.NewPausing("first,", "second")
		gen.PrepareErr = boom
		cache, book, file := pausingCache(t, gen)

		var err error
		if head {
			_, err = cache.Head(context.Background(), book, file, false)
		} else {
			_, err = cache.Open(context.Background(), book, file, false)
		}
		require.ErrorIs(t, err, boom)
		cache.Wait()
		entries, err := os.ReadDir(cache.Dir())
		require.NoError(t, err)
		assert.Empty(t, entries)
	}
}

// A generator that writes a different number of bytes than it reported
// fails the build rather than publishing a file that does not match the
// Content-Length already sent.
func TestCache_SizeMismatchFailsTheBuild(t *testing.T) {
	t.Parallel()
	cache, book, file := pausingCache(t, wrongSize{})

	_, _, err := cache.GetOrGenerate(context.Background(), book, file)
	require.ErrorContains(t, err, "reported 100 bytes and wrote 5")
	cache.Wait()
	entries, err := os.ReadDir(cache.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

type wrongSize struct{}

func (wrongSize) Prepare(context.Context, filegen.Source, *models.Book, *models.File) (filegen.Prepared, error) {
	return wrongSize{}, nil
}
func (wrongSize) Size() (int64, bool) { return 100, true }
func (wrongSize) Write(_ context.Context, w io.Writer) error {
	_, err := io.WriteString(w, "short")
	return err
}

// A waiter whose context ends returns promptly; the build continues.
func TestCache_GetOrGenerate_CanceledWaiterReturnsPromptly(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	cache, book, file := pausingCache(t, gen)

	g, err := cache.Open(context.Background(), book, file, false)
	require.NoError(t, err)
	defer g.Body.Close()
	<-gen.Started()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = cache.GetOrGenerate(ctx, book, file)
	require.ErrorIs(t, err, context.Canceled)

	gen.Release()
	path, _, err := cache.GetOrGenerate(context.Background(), book, file)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, gen.Output(), data)
	assert.Equal(t, 1, gen.Writes())
}

// A build of one file does not hold up another.
func TestCache_UnrelatedFilesDoNotWait(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	cache, book, file := pausingCache(t, gen)

	g, err := cache.Open(context.Background(), book, file, false)
	require.NoError(t, err)
	defer g.Body.Close()
	<-gen.Started()

	other := *file
	other.ID = 2
	done := make(chan error, 1)
	go func() {
		g2, err := cache.Open(context.Background(), book, &other, false)
		if err == nil {
			g2.Body.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("a build of another file waited")
	}
	gen.Release()
}

// Close cancels running builds, deletes their partial files, and fails the
// downloads still reading them.
func TestCache_CloseCancelsBuilds(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	cache, book, file := pausingCache(t, gen)

	g, err := cache.Open(context.Background(), book, file, false)
	require.NoError(t, err)
	defer g.Body.Close()
	readN(t, g.Body, len(gen.First))

	cache.Close(context.Background())
	_, err = io.ReadAll(g.Body)
	require.ErrorIs(t, err, context.Canceled)
	entries, err := os.ReadDir(cache.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// Partial files and scratch directories left by a crash are deleted;
// published entries stay.
func TestCache_RemoveLeftovers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, partialPrefix+"123"), []byte("half"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, stagingPrefix+"456"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, stagingPrefix+"456", "1.epub"), []byte("half"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "1.epub"), []byte("whole"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "1.meta.json"), []byte("{}"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bulk"), 0o755))

	require.NoError(t, NewCache(dir, 1<<30).RemoveLeftovers())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{"1.epub", "1.meta.json", "bulk"}, names)
	require.NoError(t, NewCache(filepath.Join(dir, "missing"), 1<<30).RemoveLeftovers())
}

// A generator that panics fails its build like any other error instead of
// taking down the process, which a detached goroutine would otherwise do.
func TestCache_GeneratorPanicFailsTheBuild(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"prepare", "write"} {
		cache, book, file := pausingCache(t, panicking{step: step})

		_, _, err := cache.GetOrGenerate(context.Background(), book, file)
		require.ErrorContains(t, err, "generator panicked")
		cache.Wait()
		entries, err := os.ReadDir(cache.Dir())
		require.NoError(t, err)
		assert.Empty(t, entries)
	}
}

type panicking struct{ step string }

func (p panicking) Prepare(context.Context, filegen.Source, *models.Book, *models.File) (filegen.Prepared, error) {
	if p.step == "prepare" {
		panic("bad file")
	}
	return p, nil
}
func (panicking) Size() (int64, bool) { return 0, false }
func (panicking) Write(_ context.Context, w io.Writer) error {
	_, _ = io.WriteString(w, "half")
	panic("bad file")
}

// A download joining a build that has already finished is served the
// published file.
func TestCache_Open_JoinAfterFinishServesPublishedFile(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	cache, book, file := pausingCache(t, gen)

	g, err := cache.Open(context.Background(), book, file, false)
	require.NoError(t, err)
	require.NoError(t, g.Body.Close())
	e, err := cache.entryFor(book, file, false)
	require.NoError(t, err)
	cache.mu.Lock()
	j := cache.jobs[e.key()]
	cache.mu.Unlock()
	require.NotNil(t, j)

	gen.Release()
	<-j.finished
	_, ok := j.reader(context.Background())
	require.False(t, ok)
	got, err := cache.served(context.Background(), e, j)
	require.NoError(t, err)
	require.NotEmpty(t, got.Path)
	data, err := os.ReadFile(got.Path)
	require.NoError(t, err)
	assert.Equal(t, gen.Output(), data)
}

// Close makes later downloads fail rather than start builds it would not
// wait for.
func TestCache_OpenAfterCloseFails(t *testing.T) {
	t.Parallel()
	cache, book, file := pausingCache(t, filegentest.NewPausing("a", "b"))
	cache.Close(context.Background())
	_, err := cache.Open(context.Background(), book, file, false)
	require.ErrorContains(t, err, "closed")
	entries, err := os.ReadDir(cache.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries)
}
