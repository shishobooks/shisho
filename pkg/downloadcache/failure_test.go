package downloadcache

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failMidStream opens a download of a Generated File whose build fails
// after its first bytes, reads it to the end, and waits for the build to
// end. The download itself must fail.
func failMidStream(t *testing.T, cache *Cache, gen *filegentest.Pausing, open func() (*Generated, error)) {
	t.Helper()
	g, err := open()
	require.NoError(t, err)
	require.NotNil(t, g.Body)
	defer g.Body.Close()
	assert.Equal(t, string(gen.First), readN(t, g.Body, len(gen.First)))
	gen.Release()
	_, err = io.ReadAll(g.Body)
	require.Error(t, err, "a download that failed partway ended like a complete one")
	cache.Wait()
}

// A failure after bytes have been sent is remembered against the file's
// current fingerprint: later downloads and HEAD get a generation error
// without building again, even after a restart, until the metadata changes.
func TestCache_MidStreamFailureIsRemembered(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	gen.WriteErr = errors.New("page 3 is corrupt")
	cache, book, file := pausingCache(t, gen)
	ctx := context.Background()

	failMidStream(t, cache, gen, func() (*Generated, error) { return cache.Open(ctx, book, file, file.FileType) })

	var genErr *filegen.GenerationError
	_, err := cache.Open(ctx, book, file, file.FileType)
	require.ErrorAs(t, err, &genErr)
	require.ErrorIs(t, err, ErrFailedPartway)
	assert.Contains(t, err.Error(), "page 3 is corrupt")
	_, err = cache.Head(ctx, book, file, file.FileType)
	require.ErrorAs(t, err, &genErr)
	require.ErrorIs(t, err, ErrFailedPartway)
	_, _, err = cache.GetOrGenerate(ctx, book, file, file.FileType)
	require.ErrorAs(t, err, &genErr)
	assert.Equal(t, 1, gen.Writes(), "a remembered failure was built again")
	assertNoStagingLeftovers(t, cache.Dir())

	// The KePub conversion of the same file has its own record.
	g, err := cache.Head(ctx, book, file, models.FileTypeKePub)
	require.NoError(t, err)
	assert.Empty(t, g.Path)

	// The record survives a restart.
	restarted := NewCache(cache.Dir(), 1<<30, WithGenerators(func(string, string) (filegen.StreamGenerator, error) {
		return gen, nil
	}))
	t.Cleanup(restarted.Wait)
	_, err = restarted.Open(ctx, book, file, file.FileType)
	require.ErrorAs(t, err, &genErr)
	assert.Equal(t, 1, gen.Writes())

	// New metadata builds again, and a success clears the record.
	gen.WriteErr = nil
	book.Title = "Fixed Book"
	g, err = restarted.Open(ctx, book, file, file.FileType)
	require.NoError(t, err)
	assert.Equal(t, gen.Output(), readGenerated(t, g))
	restarted.Wait()
	g, err = restarted.Open(ctx, book, file, file.FileType)
	require.NoError(t, err)
	assert.NotEmpty(t, g.Path)
	assert.Equal(t, 2, gen.Writes())
}

// A build canceled by shutdown is not a failure of the file, so it is not
// remembered.
func TestCache_CanceledBuildIsNotRemembered(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	cache, book, file := pausingCache(t, gen)
	ctx := context.Background()

	g, err := cache.Open(ctx, book, file, file.FileType)
	require.NoError(t, err)
	readN(t, g.Body, len(gen.First))
	cache.Close(ctx)
	require.NoError(t, g.Body.Close())

	meta, err := ReadMetadata(cache.Dir(), file.ID, file.FileType)
	require.NoError(t, err)
	assert.Nil(t, meta)
}

// capturedLogLine is one JSON log line.
type capturedLogLine struct {
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

// captureWarnings redirects loggers created during the test into a buffer
// and returns a function that reads back the warnings. Tests that call it
// must not run in parallel, since the log output is process-wide.
func captureWarnings(t *testing.T) func() []capturedLogLine {
	t.Helper()
	// logger.New reads LOG_LEVEL, and a level above warn would hide the lines
	// under test.
	t.Setenv("LOG_LEVEL", "")
	var buf bytes.Buffer
	prev := logger.Output()
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(prev) })
	return func() []capturedLogLine {
		var lines []capturedLogLine
		scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
		for scanner.Scan() {
			var line capturedLogLine
			require.NoError(t, json.Unmarshal(scanner.Bytes(), &line), "log line: %s", scanner.Text())
			if line.Level == "warn" {
				lines = append(lines, line)
			}
		}
		require.NoError(t, scanner.Err())
		return lines
	}
}

// Not parallel: it captures the process-wide log.
func TestCache_MidStreamFailureLogsAWarning(t *testing.T) {
	warnings := captureWarnings(t)
	gen := filegentest.NewPausing("first,", "second")
	gen.WriteErr = errors.New("page 3 is corrupt")
	cache, book, file := pausingCache(t, gen)

	failMidStream(t, cache, gen, func() (*Generated, error) { return cache.Open(context.Background(), book, file, file.FileType) })

	lines := warnings()
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0].Message, "failed partway")
	assert.EqualValues(t, file.ID, lines[0].Data["file_id"])
	assert.Equal(t, file.Filepath, lines[0].Data["filepath"])
	assert.Contains(t, lines[0].Data["error"], "page 3 is corrupt")
}

// failingPartial is a partial file whose writes fail with ENOSPC once limit
// bytes have been written, like a disk filling up.
type failingPartial struct {
	*os.File
	limit   int
	written int
}

func (f *failingPartial) Write(p []byte) (int, error) {
	room := f.limit - f.written
	if room >= len(p) {
		n, err := f.File.Write(p)
		f.written += n
		return n, err
	}
	n, err := f.File.Write(p[:max(room, 0)])
	f.written += n
	if err == nil {
		err = &os.PathError{Op: "write", Path: f.Name(), Err: syscall.ENOSPC}
	}
	return n, err
}

// When writing the cache fails partway, every download already reading the
// file still receives all of it, nothing is published, and a warning is
// logged. Not parallel: it captures the process-wide log.
func TestCache_CacheWriteFailurePartwayKeepsStreaming(t *testing.T) {
	warnings := captureWarnings(t)
	gen := filegentest.NewPausing("first,", "second and more")
	cache, book, file := pausingCache(t, gen)
	cache.createPartial = func(dir string) (partialFile, error) {
		f, err := createPartial(dir)
		if err != nil {
			return nil, err
		}
		return &failingPartial{File: f, limit: len(gen.First) + 3}, nil
	}
	ctx := context.Background()

	a, err := cache.Open(ctx, book, file, file.FileType)
	require.NoError(t, err)
	defer a.Body.Close()
	assert.Equal(t, "first,", readN(t, a.Body, len(gen.First)))
	b, err := cache.Open(ctx, book, file, file.FileType)
	require.NoError(t, err)
	defer b.Body.Close()

	// Once the cache is dropped the build goes at the pace of the slowest
	// download, so both read at once, as real clients do.
	gen.Release()
	var dataB []byte
	var errB error
	readB := make(chan struct{})
	go func() {
		defer close(readB)
		dataB, errB = io.ReadAll(b.Body)
	}()
	dataA, err := io.ReadAll(a.Body)
	require.NoError(t, err)
	<-readB
	require.NoError(t, errB)
	assert.Equal(t, string(gen.Output()), "first,"+string(dataA))
	assert.Equal(t, string(gen.Output()), string(dataB))
	require.NoError(t, a.Body.Close())
	require.NoError(t, b.Body.Close())
	cache.Wait()

	entries, err := os.ReadDir(cache.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries, "a partial cache entry was left behind")
	assert.Equal(t, 1, gen.Writes())
	lines := warnings()
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0].Message, "cache")
	assert.EqualValues(t, file.ID, lines[0].Data["file_id"])

	// The failure was the cache's, not the file's, so nothing is remembered:
	// the next download builds again.
	g, err := cache.Open(ctx, book, file, file.FileType)
	require.NoError(t, err)
	data, err := io.ReadAll(g.Body)
	require.NoError(t, err)
	require.NoError(t, g.Body.Close())
	assert.Equal(t, gen.Output(), data)
	// A build that loses its cache copy before the download attaches is
	// replaced by one streamed straight to it, so this may be the third.
	assert.GreaterOrEqual(t, gen.Writes(), 2)
}

// A download that is the last one reading a build the cache could not keep
// stops the build when it goes away, since nothing would be left of it.
func TestCache_CacheWriteFailureStopsWhenTheLastDownloadLeaves(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	cache, book, file := pausingCache(t, gen)
	cache.createPartial = func(dir string) (partialFile, error) {
		f, err := createPartial(dir)
		if err != nil {
			return nil, err
		}
		return &failingPartial{File: f, limit: 0}, nil
	}

	g, err := cache.Open(context.Background(), book, file, file.FileType)
	require.NoError(t, err)
	gen.Release()
	assert.Equal(t, "first,", readN(t, g.Body, len(gen.First)))
	require.NoError(t, g.Body.Close())
	cache.Wait()

	// The download leaving is not a failure of the file, so nothing is
	// remembered either.
	entries, err := os.ReadDir(cache.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// unwritableCache makes every new cache entry fail to start, as a full or
// read-only cache does, while metadata can still be written.
func unwritableCache(cache *Cache) {
	cache.createPartial = func(string) (partialFile, error) {
		return nil, &os.PathError{Op: "open", Path: "partial", Err: syscall.ENOSPC}
	}
}

// A download streamed without the cache that the client leaves is not
// remembered as a failure, and one whose build fails partway is.
func TestCache_UncachedStreamFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("client leaves", func(t *testing.T) {
		t.Parallel()
		gen := filegentest.NewPausing("first,", "second")
		cache, book, file := pausingCache(t, gen)
		unwritableCache(cache)

		g, err := cache.Open(ctx, book, file, file.FileType)
		require.NoError(t, err)
		assert.Equal(t, "first,", readN(t, g.Body, len(gen.First)))
		require.NoError(t, g.Body.Close())
		gen.Release()
		cache.Wait()

		meta, err := ReadMetadata(cache.Dir(), file.ID, file.FileType)
		require.NoError(t, err)
		assert.Nil(t, meta)
	})
	t.Run("build fails partway", func(t *testing.T) {
		t.Parallel()
		gen := filegentest.NewPausing("first,", "second")
		gen.WriteErr = errors.New("page 3 is corrupt")
		cache, book, file := pausingCache(t, gen)
		unwritableCache(cache)

		failMidStream(t, cache, gen, func() (*Generated, error) { return cache.Open(ctx, book, file, file.FileType) })
		var genErr *filegen.GenerationError
		_, err := cache.Open(ctx, book, file, file.FileType)
		require.ErrorAs(t, err, &genErr)
	})
}

// A failure remembered by another release does not apply: that release
// may have fixed it.
func TestCache_FailureFromAnotherReleaseIsIgnored(t *testing.T) {
	t.Parallel()
	gen := filegentest.NewPausing("first,", "second")
	gen.Release()
	cache, book, file := pausingCache(t, gen)
	e, err := cache.entryFor(book, file, file.FileType)
	require.NoError(t, err)
	require.NoError(t, WriteMetadata(cache.Dir(), &CacheMetadata{
		FileID:            file.ID,
		DeliveredFileType: file.FileType,
		Failure:           &FailureRecord{FingerprintHash: e.hash, Version: "0.0.1-old", Message: "broken"},
	}))

	g, err := cache.Open(context.Background(), book, file, file.FileType)
	require.NoError(t, err)
	assert.Equal(t, gen.Output(), readGenerated(t, g))
}

// readGenerated reads a Generated File whether it is still streaming or
// already cached: a build with nothing to wait for can finish before the
// download attaches.
func readGenerated(t *testing.T, g *Generated) []byte {
	t.Helper()
	if g.Body == nil {
		data, err := os.ReadFile(g.Path)
		require.NoError(t, err)
		return data
	}
	data, err := io.ReadAll(g.Body)
	require.NoError(t, err)
	require.NoError(t, g.Body.Close())
	return data
}

// stagedElsewhere builds its whole output up front outside the cache, as
// the full-build adapter does when the cache has no room to stage it.
type stagedElsewhere struct {
	dir     string
	content string
}

func (s stagedElsewhere) Prepare(context.Context, filegen.Source, *models.Book, *models.File) (filegen.Prepared, error) {
	dir, err := os.MkdirTemp(s.dir, stagingPrefix)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "out")
	if err := os.WriteFile(path, []byte(s.content), 0o600); err != nil {
		return nil, err
	}
	return &builtFile{dir: dir, path: path, size: int64(len(s.content))}, nil
}

// A file built outside the cache, because the cache had no room to stage
// it, is copied into the job rather than renamed, so a cache that fills up
// while copying still delivers the whole file and publishes nothing.
func TestCache_FileBuiltOutsideTheCacheIsCopied(t *testing.T) {
	t.Parallel()
	staging := t.TempDir()
	cache, book, file := pausingCache(t, stagedElsewhere{dir: staging, content: "whole file"})
	cache.createPartial = func(dir string) (partialFile, error) {
		f, err := createPartial(dir)
		if err != nil {
			return nil, err
		}
		return &failingPartial{File: f, limit: 3}, nil
	}

	g, err := cache.Open(context.Background(), book, file, file.FileType)
	require.NoError(t, err)
	assert.Equal(t, "whole file", string(readGenerated(t, g)))
	cache.Wait()

	entries, err := os.ReadDir(cache.Dir())
	require.NoError(t, err)
	assert.Empty(t, entries, "a file the cache could not hold was published")
	entries, err = os.ReadDir(staging)
	require.NoError(t, err)
	assert.Empty(t, entries, "the staging directory was left behind")
}
