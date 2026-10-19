package downloadcache

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A source that changes while it is copied fails the download short of its
// Content-Length, and nothing is cached.
func TestCache_SourceChangedDuringCopy(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]func(t *testing.T, dir string) string{
		models.FileTypeM4B: func(t *testing.T, dir string) string {
			testgen.SkipIfNoFFmpeg(t)
			return testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{Title: "Original", Duration: 10})
		},
		models.FileTypeMOBI: func(t *testing.T, dir string) string {
			return testgen.GenerateMOBI(t, dir, "source.mobi", testgen.MOBIOptions{Kind: testgen.MOBIKindCombo, Title: "Original"})
		},
		models.FileTypeAZW3: func(t *testing.T, dir string) string {
			return testgen.GenerateMOBI(t, dir, "source.azw3", testgen.MOBIOptions{Kind: testgen.MOBIKindKF8, Title: "Original"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			cacheDir := filepath.Join(dir, "cache")
			require.NoError(t, os.MkdirAll(cacheDir, 0o755))
			srcPath := source(t, dir)

			gen, err := filegen.GetGenerator(name)
			require.NoError(t, err)
			streaming, ok := gen.(filegen.StreamGenerator)
			require.True(t, ok)
			attached := make(chan struct{})
			attach := sync.OnceFunc(func() { close(attached) })
			cache := NewCache(cacheDir, 1<<30, WithGenerators(func(string, string) (filegen.StreamGenerator, error) {
				return changesSource{StreamGenerator: streaming, path: srcPath, attached: attached}, nil
			}))
			t.Cleanup(cache.Wait)
			t.Cleanup(attach) // runs before cache.Wait, so a failed Open does not hang the build
			book := &models.Book{Title: "New"}
			file := &models.File{ID: 1, FileType: name, Filepath: srcPath}

			g, err := cache.Open(context.Background(), book, file, file.FileType)
			require.NoError(t, err)
			require.NotNil(t, g.Body)
			defer g.Body.Close()
			attach()
			got, err := io.ReadAll(g.Body)
			require.Error(t, err, "a download of a changed source ended like a complete one")
			assert.Less(t, int64(len(got)), g.Size)
			cache.Wait()

			_, err = os.Stat(cachedFilename(cacheDir, file.ID, file.FileType))
			assert.True(t, os.IsNotExist(err), "the file was cached")
			assertNoStagingLeftovers(t, cacheDir)
		})
	}
}

// changesSource appends to the source at path once the generator starts
// writing, as a library scan rewriting the file mid-download would. Its
// first write waits for attached, so the build cannot fail before the
// download attaches; that case returns the error from Open instead of a
// stream.
type changesSource struct {
	filegen.StreamGenerator
	path     string
	attached <-chan struct{}
}

func (g changesSource) Prepare(ctx context.Context, src filegen.Source, book *models.Book, file *models.File) (filegen.Prepared, error) {
	p, err := g.StreamGenerator.Prepare(ctx, src, book, file)
	if err != nil {
		return nil, err
	}
	return changesSourcePrepared{Prepared: p, path: g.path, attached: g.attached}, nil
}

type changesSourcePrepared struct {
	filegen.Prepared
	path     string
	attached <-chan struct{}
}

func (p changesSourcePrepared) Write(ctx context.Context, w io.Writer) error {
	return p.Prepared.Write(ctx, &appendOnFirstWrite{ctx: ctx, w: w, path: p.path, attached: p.attached})
}

type appendOnFirstWrite struct {
	ctx      context.Context
	w        io.Writer
	path     string
	attached <-chan struct{}
	wrote    bool
}

func (a *appendOnFirstWrite) Write(b []byte) (int, error) {
	if !a.wrote {
		a.wrote = true
		select {
		case <-a.attached:
		case <-a.ctx.Done():
			return 0, a.ctx.Err()
		}
		f, err := os.OpenFile(a.path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return 0, err
		}
		_, err = f.Write([]byte("more"))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return 0, err
		}
	}
	return a.w.Write(b)
}
