package filegen

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEPUBGenerator_Generate(t *testing.T) {
	t.Run("modifies title and authors", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a simple test EPUB
		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:   "Original Title",
			authors: []string{"Original Author"},
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

		book := &models.Book{
			Title: "New Title",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "New Author", SortName: "Author, New"}},
			},
		}
		file := &models.File{FileType: models.FileTypeEPUB}

		gen := &EPUBGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Verify the destination file exists
		_, err = os.Stat(destPath)
		require.NoError(t, err)

		// Verify the modified metadata
		pkg := readOPFFromEPUB(t, destPath)
		assert.Equal(t, "New Title", pkg.Metadata.Titles[0].Text)
		require.Len(t, pkg.Metadata.Creators, 1)
		assert.Equal(t, "New Author", pkg.Metadata.Creators[0].Text)
		assert.Equal(t, "aut", pkg.Metadata.Creators[0].Role)
		assert.Equal(t, "Author, New", pkg.Metadata.Creators[0].FileAs)
	})

	t.Run("modifies multiple authors in sort order", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:   "Test Book",
			authors: []string{"Original"},
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 1, Person: &models.Person{Name: "Second Author"}},
				{SortOrder: 0, Person: &models.Person{Name: "First Author"}},
				{SortOrder: 2, Person: &models.Person{Name: "Third Author"}},
			},
		}
		file := &models.File{FileType: models.FileTypeEPUB}

		gen := &EPUBGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		pkg := readOPFFromEPUB(t, destPath)
		require.Len(t, pkg.Metadata.Creators, 3)
		assert.Equal(t, "First Author", pkg.Metadata.Creators[0].Text)
		assert.Equal(t, "Second Author", pkg.Metadata.Creators[1].Text)
		assert.Equal(t, "Third Author", pkg.Metadata.Creators[2].Text)
	})

	t.Run("adds series metadata", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:   "Test Book",
			authors: []string{"Author"},
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
			BookSeries: []*models.BookSeries{
				{SortOrder: 0, SeriesNumber: pointerutil.Float64(3), Series: &models.Series{Name: "Test Series"}},
			},
		}
		file := &models.File{FileType: models.FileTypeEPUB}

		gen := &EPUBGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		pkg := readOPFFromEPUB(t, destPath)

		// Find calibre:series and calibre:series_index meta tags (Calibre compatibility)
		var calibreSeriesName, calibreSeriesIndex string
		// Find EPUB3-style series meta tags (Kobo and modern readers)
		var epub3SeriesID, epub3SeriesName string
		var epub3CollectionTypeRefines, epub3CollectionType string
		var epub3GroupPositionRefines, epub3GroupPosition string
		for _, meta := range pkg.Metadata.Meta {
			if meta.Name == "calibre:series" {
				calibreSeriesName = meta.Content
			}
			if meta.Name == "calibre:series_index" {
				calibreSeriesIndex = meta.Content
			}
			if meta.Property == "belongs-to-collection" {
				epub3SeriesID = meta.ID
				epub3SeriesName = meta.Text
			}
			if meta.Property == "collection-type" {
				epub3CollectionTypeRefines = meta.Refines
				epub3CollectionType = meta.Text
			}
			if meta.Property == "group-position" {
				epub3GroupPositionRefines = meta.Refines
				epub3GroupPosition = meta.Text
			}
		}
		// Verify Calibre-style metadata
		assert.Equal(t, "Test Series", calibreSeriesName)
		assert.Equal(t, "3", calibreSeriesIndex)
		// Verify EPUB3-style metadata with proper id and refines attributes
		assert.Equal(t, "series-1", epub3SeriesID, "belongs-to-collection should have id attribute")
		assert.Equal(t, "Test Series", epub3SeriesName)
		assert.Equal(t, "#series-1", epub3CollectionTypeRefines, "collection-type should refine series-1")
		assert.Equal(t, "series", epub3CollectionType)
		assert.Equal(t, "#series-1", epub3GroupPositionRefines, "group-position should refine series-1")
		assert.Equal(t, "3", epub3GroupPosition)
	})

	t.Run("handles decimal series number", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:   "Test Book",
			authors: []string{"Author"},
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

		book := &models.Book{
			Title: "Test Book",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
			BookSeries: []*models.BookSeries{
				{SortOrder: 0, SeriesNumber: pointerutil.Float64(1.5), Series: &models.Series{Name: "Series"}},
			},
		}
		file := &models.File{FileType: models.FileTypeEPUB}

		gen := &EPUBGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		pkg := readOPFFromEPUB(t, destPath)

		var calibreSeriesIndex, epub3GroupPosition string
		for _, meta := range pkg.Metadata.Meta {
			if meta.Name == "calibre:series_index" {
				calibreSeriesIndex = meta.Content
			}
			if meta.Property == "group-position" {
				epub3GroupPosition = meta.Text
			}
		}
		// Verify decimal series number in both formats
		assert.Equal(t, "1.5", calibreSeriesIndex)
		assert.Equal(t, "1.5", epub3GroupPosition)
	})

	t.Run("replaces cover image", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a book directory structure (non-root-level book)
		bookDir := filepath.Join(tmpDir, "book")
		require.NoError(t, os.MkdirAll(bookDir, 0755))

		// Create a test cover image in the book directory
		// Using the standard naming pattern: {filename}.cover.{ext}
		coverFilename := "source.epub.cover.jpg"
		coverFullPath := filepath.Join(bookDir, coverFilename)
		newCoverData := []byte("new cover image data")
		err := os.WriteFile(coverFullPath, newCoverData, 0644)
		require.NoError(t, err)

		srcPath := filepath.Join(bookDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:     "Test Book",
			authors:   []string{"Author"},
			coverData: []byte("original cover"),
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

		// Book.Filepath is the book directory
		book := &models.Book{
			Title:    "Test Book",
			Filepath: bookDir,
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		mimeType := "image/jpeg"
		// CoverImagePath is just the filename, not the full path
		file := &models.File{
			FileType:       models.FileTypeEPUB,
			CoverImagePath: &coverFilename,
			CoverMimeType:  &mimeType,
		}

		gen := &EPUBGenerator{}
		err = gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Read the cover from the generated EPUB
		coverData := readFileFromEPUB(t, destPath, "cover.jpg")
		assert.Equal(t, newCoverData, coverData)
	})

	t.Run("replaces cover image for root-level book", func(t *testing.T) {
		tmpDir := t.TempDir()

		// For root-level books, book.Filepath is the actual file path, not a directory
		// Covers are stored next to the file in the same directory

		// Create a test cover image in the same directory as the book file
		coverFilename := "source.epub.cover.jpg"
		coverFullPath := filepath.Join(tmpDir, coverFilename)
		newCoverData := []byte("new cover image data for root level")
		err := os.WriteFile(coverFullPath, newCoverData, 0644)
		require.NoError(t, err)

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:     "Test Book",
			authors:   []string{"Author"},
			coverData: []byte("original cover"),
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

		// For root-level book, Filepath is the actual epub file path
		book := &models.Book{
			Title:    "Test Book",
			Filepath: srcPath, // Points to the epub file, not a directory
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
		}
		mimeType := "image/jpeg"
		file := &models.File{
			FileType:       models.FileTypeEPUB,
			CoverImagePath: &coverFilename,
			CoverMimeType:  &mimeType,
		}

		gen := &EPUBGenerator{}
		err = gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Read the cover from the generated EPUB
		coverData := readFileFromEPUB(t, destPath, "cover.jpg")
		assert.Equal(t, newCoverData, coverData)
	})

	t.Run("preserves other files unchanged", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:   "Test Book",
			authors: []string{"Author"},
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

		book := &models.Book{
			Title: "New Title",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "New Author"}},
			},
		}
		file := &models.File{FileType: models.FileTypeEPUB}

		gen := &EPUBGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Check that mimetype file is preserved
		mimetypeData := readFileFromEPUB(t, destPath, "mimetype")
		assert.Equal(t, "application/epub+zip", string(mimetypeData))

		// Check that chapter content is preserved
		chapterData := readFileFromEPUB(t, destPath, "chapter1.xhtml")
		assert.Contains(t, string(chapterData), "Chapter 1 content")
	})

	t.Run("returns error for non-existent source file", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "nonexistent.epub")
		destPath := filepath.Join(tmpDir, "dest.epub")

		book := &models.Book{Title: "Test"}
		file := &models.File{FileType: models.FileTypeEPUB}

		gen := &EPUBGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.Error(t, err)

		var genErr *GenerationError
		require.ErrorAs(t, err, &genErr)
		assert.Equal(t, models.FileTypeEPUB, genErr.FileType)
	})

	t.Run("returns error for invalid EPUB (not a zip)", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "invalid.epub")
		err := os.WriteFile(srcPath, []byte("not a zip file"), 0644)
		require.NoError(t, err)

		destPath := filepath.Join(tmpDir, "dest.epub")

		book := &models.Book{Title: "Test"}
		file := &models.File{FileType: models.FileTypeEPUB}

		gen := &EPUBGenerator{}
		err = gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.Error(t, err)

		var genErr *GenerationError
		require.ErrorAs(t, err, &genErr)
		assert.Contains(t, genErr.Message, "zip")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:   "Test Book",
			authors: []string{"Author"},
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

		book := &models.Book{Title: "Test"}
		file := &models.File{FileType: models.FileTypeEPUB}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		gen := &EPUBGenerator{}
		err := gen.Generate(ctx, srcPath, destPath, book, file)
		require.Error(t, err)

		var genErr *GenerationError
		require.ErrorAs(t, err, &genErr)
		assert.Contains(t, genErr.Message, "cancelled")
	})

	t.Run("writes genres as dc:subject", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:   "Test Book",
			authors: []string{"Author"},
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

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
		file := &models.File{FileType: models.FileTypeEPUB}

		gen := &EPUBGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		pkg := readOPFFromEPUB(t, destPath)
		require.Len(t, pkg.Metadata.Subjects, 2)
		assert.Equal(t, "Fantasy", pkg.Metadata.Subjects[0])
		assert.Equal(t, "Science Fiction", pkg.Metadata.Subjects[1])
	})

	t.Run("writes tags as calibre:tags meta", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:   "Test Book",
			authors: []string{"Author"},
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

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
		file := &models.File{FileType: models.FileTypeEPUB}

		gen := &EPUBGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		pkg := readOPFFromEPUB(t, destPath)

		// Find calibre:tags meta
		var calibreTags string
		for _, meta := range pkg.Metadata.Meta {
			if meta.Name == "calibre:tags" {
				calibreTags = meta.Content
				break
			}
		}
		assert.Equal(t, "Must Read, Favorites", calibreTags)
	})

	t.Run("preserves source genres when book has none", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title:    "Test Book",
			authors:  []string{"Author"},
			subjects: []string{"Original Genre"},
		})

		destPath := filepath.Join(tmpDir, "dest.epub")

		book := &models.Book{
			Title: "Modified Title",
			Authors: []*models.Author{
				{SortOrder: 0, Person: &models.Person{Name: "Author"}},
			},
			// No genres
		}
		file := &models.File{FileType: models.FileTypeEPUB}

		gen := &EPUBGenerator{}
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		pkg := readOPFFromEPUB(t, destPath)
		// Genre should be preserved from source
		require.Len(t, pkg.Metadata.Subjects, 1)
		assert.Equal(t, "Original Genre", pkg.Metadata.Subjects[0])
	})
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{1, "1"},
		{1.0, "1"},
		{10, "10"},
		{1.5, "1.5"},
		{2.25, "2.25"},
		{0.5, "0.5"},
		{100, "100"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatFloat(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper types and functions for testing

type testEPUBOptions struct {
	title     string
	authors   []string
	coverData []byte
	subjects  []string
}

func createTestEPUB(t *testing.T, path string, opts testEPUBOptions) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)

	// Add mimetype (must be first and uncompressed)
	mimetypeHeader := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	mimetypeWriter, err := w.CreateHeader(mimetypeHeader)
	require.NoError(t, err)
	_, err = mimetypeWriter.Write([]byte("application/epub+zip"))
	require.NoError(t, err)

	// Add container.xml
	containerWriter, err := w.Create("META-INF/container.xml")
	require.NoError(t, err)
	_, err = containerWriter.Write([]byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))
	require.NoError(t, err)

	// Build OPF content
	var authorsXML strings.Builder
	for _, author := range opts.authors {
		authorsXML.WriteString(`    <dc:creator opf:role="aut">`)
		authorsXML.WriteString(author)
		authorsXML.WriteString("</dc:creator>\n")
	}

	var subjectsXML strings.Builder
	for _, subject := range opts.subjects {
		subjectsXML.WriteString(`    <dc:subject>`)
		subjectsXML.WriteString(subject)
		subjectsXML.WriteString("</dc:subject>\n")
	}

	var manifestItems strings.Builder
	manifestItems.WriteString(`    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>`)
	manifestItems.WriteString("\n")

	var coverMeta string
	if len(opts.coverData) > 0 {
		coverMeta = `    <meta name="cover" content="cover-image"/>` + "\n"
		manifestItems.WriteString(`    <item id="cover-image" href="cover.jpg" media-type="image/jpeg"/>`)
		manifestItems.WriteString("\n")
	}

	opfContent := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>` + opts.title + `</dc:title>
` + authorsXML.String() + subjectsXML.String() + `    <dc:identifier id="bookid">test-book-id</dc:identifier>
    <dc:language>en</dc:language>
` + coverMeta + `  </metadata>
  <manifest>
` + manifestItems.String() + `  </manifest>
  <spine>
    <itemref idref="chapter1"/>
  </spine>
</package>`

	opfWriter, err := w.Create("content.opf")
	require.NoError(t, err)
	_, err = opfWriter.Write([]byte(opfContent))
	require.NoError(t, err)

	// Add a sample chapter
	chapterWriter, err := w.Create("chapter1.xhtml")
	require.NoError(t, err)
	_, err = chapterWriter.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body><p>Chapter 1 content</p></body>
</html>`))
	require.NoError(t, err)

	// Add cover if provided
	if len(opts.coverData) > 0 {
		coverWriter, err := w.Create("cover.jpg")
		require.NoError(t, err)
		_, err = coverWriter.Write(opts.coverData)
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())
}

func readOPFFromEPUB(t *testing.T, path string) *opfPackage {
	t.Helper()

	data := readFileFromEPUB(t, path, "content.opf")

	var pkg opfPackage
	err := xml.Unmarshal(data, &pkg)
	require.NoError(t, err)

	return &pkg
}

func readFileFromEPUB(t *testing.T, epubPath, fileName string) []byte {
	t.Helper()

	r, err := zip.OpenReader(epubPath)
	require.NoError(t, err)
	defer r.Close()

	for _, f := range r.File {
		if f.Name == fileName {
			rc, err := f.Open()
			require.NoError(t, err)
			defer rc.Close()

			data, err := io.ReadAll(rc)
			require.NoError(t, err)
			return data
		}
	}

	t.Fatalf("file %s not found in EPUB", fileName)
	return nil
}

func TestEPUBGenerator_UsesFileNameForTitle(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple test EPUB
	srcPath := filepath.Join(tmpDir, "source.epub")
	createTestEPUB(t, srcPath, testEPUBOptions{
		title:   "Original Title",
		authors: []string{"Author"},
	})

	destPath := filepath.Join(tmpDir, "output.epub")

	name := "Custom Edition Title"
	book := &models.Book{
		Title: "Book Title",
		Authors: []*models.Author{
			{SortOrder: 0, Person: &models.Person{Name: "Author"}},
		},
	}
	file := &models.File{
		FileType: models.FileTypeEPUB,
		Name:     &name,
	}

	generator := &EPUBGenerator{}
	err := generator.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Parse the output and verify title is set to file.Name, not book.Title
	pkg := readOPFFromEPUB(t, destPath)
	assert.Equal(t, "Custom Edition Title", pkg.Metadata.Titles[0].Text)
}

func TestEPUBGenerator_UsesBookTitleWhenFileNameEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple test EPUB
	srcPath := filepath.Join(tmpDir, "source.epub")
	createTestEPUB(t, srcPath, testEPUBOptions{
		title:   "Original Title",
		authors: []string{"Author"},
	})

	destPath := filepath.Join(tmpDir, "output.epub")

	emptyName := ""
	book := &models.Book{
		Title: "Book Title",
		Authors: []*models.Author{
			{SortOrder: 0, Person: &models.Person{Name: "Author"}},
		},
	}
	file := &models.File{
		FileType: models.FileTypeEPUB,
		Name:     &emptyName,
	}

	generator := &EPUBGenerator{}
	err := generator.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// When file.Name is empty, should fall back to book.Title
	pkg := readOPFFromEPUB(t, destPath)
	assert.Equal(t, "Book Title", pkg.Metadata.Titles[0].Text)
}

func TestEPUBGenerator_UsesBookTitleWhenFileNameNil(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple test EPUB
	srcPath := filepath.Join(tmpDir, "source.epub")
	createTestEPUB(t, srcPath, testEPUBOptions{
		title:   "Original Title",
		authors: []string{"Author"},
	})

	destPath := filepath.Join(tmpDir, "output.epub")

	book := &models.Book{
		Title: "Book Title",
		Authors: []*models.Author{
			{SortOrder: 0, Person: &models.Person{Name: "Author"}},
		},
	}
	file := &models.File{
		FileType: models.FileTypeEPUB,
		Name:     nil, // Nil name
	}

	generator := &EPUBGenerator{}
	err := generator.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// When file.Name is nil, should fall back to book.Title
	pkg := readOPFFromEPUB(t, destPath)
	assert.Equal(t, "Book Title", pkg.Metadata.Titles[0].Text)
}

func TestGenerateEPUB_Identifiers(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal source EPUB for generation
	srcPath := filepath.Join(tmpDir, "source.epub")
	createTestEPUB(t, srcPath, testEPUBOptions{
		title:   "Test Book",
		authors: []string{"Author"},
	})

	outPath := filepath.Join(tmpDir, "output.epub")

	// Create book and file with identifiers
	book := &models.Book{
		Title: "Test Book",
		Authors: []*models.Author{
			{SortOrder: 0, Person: &models.Person{Name: "Author"}},
		},
	}
	file := &models.File{
		FileType: models.FileTypeEPUB,
		Identifiers: []*models.FileIdentifier{
			{Type: "isbn_13", Value: "9780316769488"},
			{Type: "asin", Value: "B08N5WRWNW"},
		},
	}

	generator := &EPUBGenerator{}
	err := generator.Generate(context.Background(), srcPath, outPath, book, file)
	require.NoError(t, err)

	// Parse and verify
	metadata, err := epub.Parse(outPath)
	require.NoError(t, err)

	require.Len(t, metadata.Identifiers, 2)
	// Verify identifiers are present
	idByType := make(map[string]string)
	for _, id := range metadata.Identifiers {
		idByType[id.Type] = id.Value
	}
	assert.Equal(t, "9780316769488", idByType["isbn_13"])
	assert.Equal(t, "B08N5WRWNW", idByType["asin"])
}
