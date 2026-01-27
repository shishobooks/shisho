package filegen

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/mp4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestM4BGenerator_SupportedType(t *testing.T) {
	t.Parallel()
	gen := &M4BGenerator{}
	assert.Equal(t, models.FileTypeM4B, gen.SupportedType())
}

func TestM4BGenerator_Generate(t *testing.T) {
	t.Parallel()
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
		assert.Equal(t, "New Author", meta.Authors[0].Name)
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
		assert.Equal(t, "First Author", meta.Authors[0].Name)
		assert.Equal(t, "Second Author", meta.Authors[1].Name)
		assert.Equal(t, "Third Author", meta.Authors[2].Name)
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
			FileType:           models.FileTypeM4B,
			CoverImageFilename: &coverFilename,
			CoverMimeType:      &mimeType,
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
			FileType:           models.FileTypeM4B,
			CoverImageFilename: &coverPath,
			CoverMimeType:      &mimeType,
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

	t.Run("writes genres to genre atom", func(t *testing.T) {
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
			BookGenres: []*models.BookGenre{
				{Genre: &models.Genre{Name: "Fantasy"}},
				{Genre: &models.Genre{Name: "Science Fiction"}},
			},
		}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		// Genres should be comma-separated
		assert.Equal(t, "Fantasy, Science Fiction", meta.Genre)
	})

	t.Run("writes tags to custom atom", func(t *testing.T) {
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
			BookTags: []*models.BookTag{
				{Tag: &models.Tag{Name: "Must Read"}},
				{Tag: &models.Tag{Name: "Favorites"}},
			},
		}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		// Tags should be comma-separated
		require.Len(t, meta.Tags, 2)
		assert.Equal(t, "Must Read", meta.Tags[0])
		assert.Equal(t, "Favorites", meta.Tags[1])
	})

	t.Run("preserves source genre when book has none", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Original Title",
			Genre:    "Original Genre",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		book := &models.Book{
			Title: "Modified Title",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
			// No genres
		}
		file := &models.File{FileType: models.FileTypeM4B}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		// Genre should be preserved from source
		assert.Equal(t, "Original Genre", meta.Genre)
	})

	t.Run("writes ASIN identifier", func(t *testing.T) {
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
		file := &models.File{
			FileType: models.FileTypeM4B,
			Identifiers: []*models.FileIdentifier{
				{Type: models.IdentifierTypeASIN, Value: "B08N5WRWNW"},
			},
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		// ASIN should be written as identifier
		require.Len(t, meta.Identifiers, 1)
		assert.Equal(t, "asin", meta.Identifiers[0].Type)
		assert.Equal(t, "B08N5WRWNW", meta.Identifiers[0].Value)
	})

	t.Run("uses file.Name for title when available", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Original Title",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		name := "Custom Audiobook Title"
		book := &models.Book{
			Title: "Book Title",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType: models.FileTypeM4B,
			Name:     &name,
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Parse and verify file.Name is used for title
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		assert.Equal(t, "Custom Audiobook Title", meta.Title)
	})

	t.Run("uses book.Title when file.Name is empty", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Original Title",
			Duration: 1.0,
		})

		destPath := filepath.Join(dir, "dest.m4b")

		emptyName := ""
		book := &models.Book{
			Title: "Book Title",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType: models.FileTypeM4B,
			Name:     &emptyName,
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Parse and verify book.Title is used when file.Name is empty
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)
		assert.Equal(t, "Book Title", meta.Title)
	})

	t.Run("uses chapters from file model instead of source", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		// Create source M4B (with no chapters in source)
		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Duration: 10.0, // 10 seconds to accommodate chapter timestamps
		})

		destPath := filepath.Join(dir, "dest.m4b")

		// Create file model with chapters - these should be used in the generated file
		ts1 := int64(0)    // 0ms
		ts2 := int64(3000) // 3000ms
		ts3 := int64(7000) // 7000ms
		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType: models.FileTypeM4B,
			Chapters: []*models.Chapter{
				{Title: "DB Chapter 1", SortOrder: 0, StartTimestampMs: &ts1},
				{Title: "DB Chapter 2", SortOrder: 1, StartTimestampMs: &ts2},
				{Title: "DB Chapter 3", SortOrder: 2, StartTimestampMs: &ts3},
			},
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Parse result and verify chapters match file.Chapters
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)

		// Should have 3 chapters from file.Chapters, not source (which has none)
		require.Len(t, meta.Chapters, 3, "expected 3 chapters from file model")
		assert.Equal(t, "DB Chapter 1", meta.Chapters[0].Title)
		assert.Equal(t, "DB Chapter 2", meta.Chapters[1].Title)
		assert.Equal(t, "DB Chapter 3", meta.Chapters[2].Title)
	})

	t.Run("preserves source chapters when file has no chapters", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		// Create source M4B with chapters
		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Duration: 10.0, // 10 seconds to accommodate chapter timestamps
			Chapters: []testgen.M4BChapter{
				{Title: "Source Chapter 1", Start: 0.0},
				{Title: "Source Chapter 2", Start: 3.0},
				{Title: "Source Chapter 3", Start: 7.0},
			},
		})

		destPath := filepath.Join(dir, "dest.m4b")

		// Create book and file with no chapters (file.Chapters = nil)
		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType: models.FileTypeM4B,
			Chapters: nil, // No chapters in file model
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Parse result and verify source chapters are preserved
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)

		// Should have 3 chapters from source (since file.Chapters is nil)
		require.Len(t, meta.Chapters, 3, "expected 3 chapters from source file")
		assert.Equal(t, "Source Chapter 1", meta.Chapters[0].Title)
		assert.Equal(t, "Source Chapter 2", meta.Chapters[1].Title)
		assert.Equal(t, "Source Chapter 3", meta.Chapters[2].Title)
	})

	t.Run("chapter order in generated M4B matches SortOrder", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		// Create source M4B (with no chapters in source)
		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Duration: 10.0, // 10 seconds to accommodate chapter timestamps
		})

		destPath := filepath.Join(dir, "dest.m4b")

		// Create file model with chapters in WRONG sort order (2, 0, 1)
		// The generator should sort them by SortOrder before writing
		ts1 := int64(0)    // 0ms for chapter with SortOrder 0
		ts2 := int64(3000) // 3000ms for chapter with SortOrder 1
		ts3 := int64(7000) // 7000ms for chapter with SortOrder 2
		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType: models.FileTypeM4B,
			Chapters: []*models.Chapter{
				// Deliberately out of order: SortOrder 2, 0, 1
				{Title: "Third Chapter", SortOrder: 2, StartTimestampMs: &ts3},
				{Title: "First Chapter", SortOrder: 0, StartTimestampMs: &ts1},
				{Title: "Second Chapter", SortOrder: 1, StartTimestampMs: &ts2},
			},
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Parse result and verify chapters are in sorted order (0, 1, 2)
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)

		// Should have 3 chapters sorted by SortOrder, not insertion order
		require.Len(t, meta.Chapters, 3, "expected 3 chapters from file model")
		assert.Equal(t, "First Chapter", meta.Chapters[0].Title, "chapter 0 should be 'First Chapter' (SortOrder 0)")
		assert.Equal(t, "Second Chapter", meta.Chapters[1].Title, "chapter 1 should be 'Second Chapter' (SortOrder 1)")
		assert.Equal(t, "Third Chapter", meta.Chapters[2].Title, "chapter 2 should be 'Third Chapter' (SortOrder 2)")
	})

	t.Run("chapters with missing StartTimestampMs use zero", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		// Create source M4B (with no chapters in source)
		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Duration: 10.0, // 10 seconds to accommodate chapter timestamps
		})

		destPath := filepath.Join(dir, "dest.m4b")

		// Create file model with chapters - one with nil StartTimestampMs
		// The chapter with nil timestamp should default to 0
		ts2 := int64(5000) // 5000ms
		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType: models.FileTypeM4B,
			Chapters: []*models.Chapter{
				{Title: "Chapter With Missing Timestamp", SortOrder: 0, StartTimestampMs: nil}, // nil should default to 0
				{Title: "Chapter With Timestamp", SortOrder: 1, StartTimestampMs: &ts2},
			},
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Parse result and verify chapter with nil timestamp has Start: 0
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)

		require.Len(t, meta.Chapters, 2, "expected 2 chapters from file model")
		assert.Equal(t, "Chapter With Missing Timestamp", meta.Chapters[0].Title)
		assert.Equal(t, time.Duration(0), meta.Chapters[0].Start, "chapter with nil StartTimestampMs should have Start: 0")
		assert.Equal(t, "Chapter With Timestamp", meta.Chapters[1].Title)
		assert.Equal(t, 5*time.Second, meta.Chapters[1].Start, "chapter with 5000ms timestamp should have Start: 5s")
	})

	t.Run("nested chapters use only top-level for M4B", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		// Create source M4B (with no chapters in source)
		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Duration: 15.0, // 15 seconds to accommodate chapter timestamps
		})

		destPath := filepath.Join(dir, "dest.m4b")

		// Create file model with nested chapters (parent with Children)
		// M4B format doesn't support nested chapters, so only top-level should appear
		ts1 := int64(0)     // 0ms
		ts2 := int64(5000)  // 5000ms
		ts3 := int64(8000)  // 8000ms - child chapter timestamp
		ts4 := int64(10000) // 10000ms

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType: models.FileTypeM4B,
			Chapters: []*models.Chapter{
				{Title: "Part 1", SortOrder: 0, StartTimestampMs: &ts1},
				{
					Title:            "Part 2",
					SortOrder:        1,
					StartTimestampMs: &ts2,
					// Nested child chapters - should be IGNORED in M4B
					Children: []*models.Chapter{
						{Title: "Part 2 - Section A", SortOrder: 0, StartTimestampMs: &ts3},
					},
				},
				{Title: "Part 3", SortOrder: 2, StartTimestampMs: &ts4},
			},
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Parse result and verify ONLY top-level chapters appear (Children ignored)
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)

		// Should have exactly 3 top-level chapters, NOT 4 (child chapter should be ignored)
		require.Len(t, meta.Chapters, 3, "expected only 3 top-level chapters, children should be ignored")
		assert.Equal(t, "Part 1", meta.Chapters[0].Title)
		assert.Equal(t, "Part 2", meta.Chapters[1].Title)
		assert.Equal(t, "Part 3", meta.Chapters[2].Title)

		// Verify none of the chapters are the child chapter
		for _, ch := range meta.Chapters {
			assert.NotEqual(t, "Part 2 - Section A", ch.Title, "child chapter should not appear in M4B")
		}
	})

	t.Run("chapters with empty title are included", func(t *testing.T) {
		testgen.SkipIfNoFFmpeg(t)
		dir := testgen.TempDir(t, "m4b-gen-*")

		// Create source M4B (with no chapters in source)
		srcPath := testgen.GenerateM4B(t, dir, "source.m4b", testgen.M4BOptions{
			Title:    "Test Book",
			Duration: 10.0, // 10 seconds to accommodate chapter timestamps
		})

		destPath := filepath.Join(dir, "dest.m4b")

		// Create file model with chapters - one with empty title
		// Empty titles should still be written to the M4B
		ts1 := int64(0)    // 0ms
		ts2 := int64(3000) // 3000ms
		ts3 := int64(7000) // 7000ms

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		file := &models.File{
			FileType: models.FileTypeM4B,
			Chapters: []*models.Chapter{
				{Title: "First Chapter", SortOrder: 0, StartTimestampMs: &ts1},
				{Title: "", SortOrder: 1, StartTimestampMs: &ts2}, // Empty title
				{Title: "Third Chapter", SortOrder: 2, StartTimestampMs: &ts3},
			},
		}

		gen := &M4BGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Parse result and verify all 3 chapters appear, including the one with empty title
		meta, err := mp4.ParseFull(destPath)
		require.NoError(t, err)

		// Should have 3 chapters - empty title chapter should not be skipped
		require.Len(t, meta.Chapters, 3, "expected 3 chapters, chapter with empty title should be included")
		assert.Equal(t, "First Chapter", meta.Chapters[0].Title)
		assert.Empty(t, meta.Chapters[1].Title, "chapter with empty title should be included")
		assert.Equal(t, "Third Chapter", meta.Chapters[2].Title)

		// Verify timestamps are correct
		assert.Equal(t, time.Duration(0), meta.Chapters[0].Start)
		assert.Equal(t, 3*time.Second, meta.Chapters[1].Start)
		assert.Equal(t, 7*time.Second, meta.Chapters[2].Start)
	})
}
