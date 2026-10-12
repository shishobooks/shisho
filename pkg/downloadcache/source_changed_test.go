package downloadcache

import (
	"context"
	"io"
	"os"
	"path/filepath"
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
			cache := NewCache(cacheDir, 1<<30, WithGenerators(func(string, bool) (filegen.StreamGenerator, error) {
				return changesSource{StreamGenerator: streaming, path: srcPath}, nil
			}))
			t.Cleanup(cache.Wait)
			book := &models.Book{Title: "New"}
			file := &models.File{ID: 1, FileType: name, Filepath: srcPath}

			g, err := cache.Open(context.Background(), book, file, false)
			require.NoError(t, err)
			require.NotNil(t, g.Body)
			defer g.Body.Close()
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
// writing, as a library scan rewriting the file mid-download would.
type changesSource struct {
	filegen.StreamGenerator
	path string
}

func (g changesSource) Prepare(ctx context.Context, src filegen.Source, book *models.Book, file *models.File) (filegen.Prepared, error) {
	p, err := g.StreamGenerator.Prepare(ctx, src, book, file)
	if err != nil {
		return nil, err
	}
	return changesSourcePrepared{Prepared: p, path: g.path}, nil
}

type changesSourcePrepared struct {
	filegen.Prepared
	path string
}

func (p changesSourcePrepared) Write(ctx context.Context, w io.Writer) error {
	return p.Prepared.Write(ctx, &appendOnFirstWrite{w: w, path: p.path})
}

type appendOnFirstWrite struct {
	w     io.Writer
	path  string
	wrote bool
}

func (a *appendOnFirstWrite) Write(b []byte) (int, error) {
	if !a.wrote {
		a.wrote = true
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
