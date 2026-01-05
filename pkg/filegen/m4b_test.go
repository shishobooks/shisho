package filegen

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestM4BGenerator_SupportedType(t *testing.T) {
	gen := &M4BGenerator{}
	assert.Equal(t, models.FileTypeM4B, gen.SupportedType())
}

func TestM4BGenerator_Generate(t *testing.T) {
	t.Run("modifies title and authors", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Original Title",
			Artist:   "Original Author",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		book := &models.Book{
			Title: "New Title",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "New Author"}},
			},
		}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Verify destination file exists
		_, err = os.Stat(destPath)
		require.NoError(t, err)

		// Verify the modified metadata
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		assert.Equal(t, "New Title", meta.Title)
		require.Len(t, meta.Authors, 1)
		assert.Equal(t, "New Author", meta.Authors[0])
	})

	t.Run("modifies multiple authors in sort order", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Artist:   "Original",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 1, Person: &models.Person{Name: "Second Author"}},
				{SortOrder: 0, Person: &models.Person{Name: "First Author"}},
				{SortOrder: 2, Person: &models.Person{Name: "Third Author"}},
			},
		}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		require.Len(t, meta.Authors, 3)
		assert.Equal(t, "First Author", meta.Authors[0])
		assert.Equal(t, "Second Author", meta.Authors[1])
		assert.Equal(t, "Third Author", meta.Authors[2])
	})

	t.Run("modifies narrators", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Composer: "Original Narrator",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType: models.FileTypeM4B,
			Narrators: []*models.Narrator{
				{SortOrder: 1, Person: &models.Person{Name: "Second Narrator"}},
				{SortOrder: 0, Person: &models.Person{Name: "First Narrator"}},
			},
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		require.Len(t, meta.Narrators, 2)
		assert.Equal(t, "First Narrator", meta.Narrators[0])
		assert.Equal(t, "Second Narrator", meta.Narrators[1])
	})

	t.Run("formats series as album", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
			BookSeries: []*models.BookSeries{
				{SortOrder: 0, SeriesNumber: pointerutil.Float64(3), Series: &models.Series{Name: "Test Series"}},
			},
		}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		assert.Equal(t, "Test Series", meta.Series)
		require.NotNil(t, meta.SeriesNumber)
		assert.InDelta(t, 3.0, *meta.SeriesNumber, 0.001)
		// Album should be formatted as "Series Name #N"
		assert.Equal(t, "Test Series #3", meta.Album)
	})

	t.Run("handles decimal series number", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
			BookSeries: []*models.BookSeries{
				{SortOrder: 0, SeriesNumber: pointerutil.Float64(1.5), Series: &models.Series{Name: "Series"}},
			},
		}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		// Album should be formatted with decimal
		assert.Equal(t, "Series #1.5", meta.Album)
	})

	t.Run("replaces cover image", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		// Create a book directory structure
		bookDir := filepath.Join(dir, "book")
		require.NoError(t, os.MkdirAll(bookDir, 0755))

		// Create a test cover image
		coverFilename := "source.m4b.cover.jpg"
		coverFullPath := filepath.Join(bookDir, coverFilename)
		newCoverData := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG magic bytes
		err := os.WriteFile(coverFullPath, newCoverData, 0644)
		require.NoError(t, err)

		srcPath := testgen.GenerateM4B(t, bookDir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			HasCover: true,
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		mimeType := "image/jpeg"
		book := &models.Book{
			Title:    "Test Book",
			Filepath: bookDir,
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType:       models.FileTypeM4B,
			CoverImagePath: &coverFilename,
			CoverMimeType:  &mimeType,
		}

		gen := &M4BGenerator{}
		err = gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Read the cover from the generated M4B
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		assert.Equal(t, newCoverData, meta.CoverData)
		assert.Equal(t, "image/jpeg", meta.CoverMimeType)
	})

	t.Run("preserves description and genre", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Original Title",
			Artist:   "Original Author",
			Genre:    "Fantasy",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		// Only modify title
		book := &models.Book{
			Title: "Modified Title",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "New Author"}},
			},
		}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Verify title changed but genre preserved
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		assert.Equal(t, "Modified Title", meta.Title)
		assert.Equal(t, "Fantasy", meta.Genre)
	})

	t.Run("writes subtitle as freeform atom", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Main Title",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		subtitle := "A Compelling Subtitle"
		book := &models.Book{
			Title:    "Main Title",
			Subtitle: &subtitle,
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Re-read and verify subtitle was written
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		assert.Equal(t, "A Compelling Subtitle", meta.Subtitle)
	})

	t.Run("returns error for missing cover", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			HasCover: true,
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		// Point to non-existent cover
		mimeType := "image/jpeg"
		coverPath := "nonexistent.jpg"
		book := &models.Book{
			Title:    "Test Book",
			Filepath: dir,
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType:       models.FileTypeM4B,
			CoverImagePath: &coverPath,
			CoverMimeType:  &mimeType,
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.Error(t, err)

		var genErr *GenerationError
		require.ErrorAs(t, err, &genErr)
		assert.Equal(t, models.FileTypeM4B, genErr.FileType)
		assert.Contains(t, genErr.Message, "cover")
	})

	t.Run("context cancellation", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		book := &models.Book{Title: "Test"}
		file := &models.File{FileType: models.FileTypeM4B}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		gen := &M4BGenerator{}
		err := gen.Generate(ctx, srcPath, destPath, book, file)
		require.Error(t, err)

		var genErr *GenerationError
		require.ErrorAs(t, err, &genErr)
		assert.Contains(t, genErr.Message, "cancelled")
	})

	t.Run("returns error for non-existent source file", func(t *testing.T) {
		dir := t.TempDir()

		srcPath := filepath.Join(dir, "nonexistent.m4b")
		destPath := filepath.Join(dir, "dest.m4b")

		book := &models.Book{Title: "Test"}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.Error(t, err)

		var genErr *GenerationError
		require.ErrorAs(t, err, &genErr)
		assert.Equal(t, models.FileTypeM4B, genErr.FileType)
	})

	t.Run("returns error for empty file", func(t *testing.T) {
		dir := t.TempDir()

		srcPath := filepath.Join(dir, "empty.m4b")
		err := os.WriteFile(srcPath, []byte{}, 0644)
		require.NoError(t, err)

		destPath := filepath.Join(dir, "dest.m4b")

		book := &models.Book{Title: "Test"}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err = gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.Error(t, err)

		var genErr *GenerationError
		require.ErrorAs(t, err, &genErr)
		assert.Equal(t, models.FileTypeM4B, genErr.FileType)
	})

	t.Run("no temp file remains after generation", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Verify no temp file remains
		assert.NoFileExists(t, destPath+".tmp")
	})
}
