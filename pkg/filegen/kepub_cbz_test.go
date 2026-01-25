package filegen

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestCBZForKepub creates a valid CBZ file for testing KePub generation.
func createTestCBZForKepub(t *testing.T, path string) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)

	// Create a simple test image
	img := image.NewRGBA(image.Rect(0, 0, 100, 150))
	c := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	for y := 0; y < 150; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})

	// Add a page
	writer, err := w.Create("page001.jpg")
	require.NoError(t, err)
	_, err = writer.Write(buf.Bytes())
	require.NoError(t, err)

	require.NoError(t, w.Close())
}

// readOPFFromKepub reads the content.opf file from a KePub (EPUB) archive.
func readOPFFromKepub(t *testing.T, kepubPath string) []byte {
	t.Helper()

	r, err := zip.OpenReader(kepubPath)
	require.NoError(t, err)
	defer r.Close()

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "content.opf") || f.Name == "content.opf" {
			rc, err := f.Open()
			require.NoError(t, err)
			defer rc.Close()

			data, err := io.ReadAll(rc)
			require.NoError(t, err)
			return data
		}
	}

	t.Fatalf("content.opf not found in KePub")
	return nil
}

func TestKepubCBZGenerator_Generate(t *testing.T) {
	t.Parallel()
	t.Run("uses file.Name for title when available", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZForKepub(t, srcPath)

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		name := "Custom File Name"
		book := &models.Book{
			Title: "Book Title",
		}
		file := &models.File{
			FileType: models.FileTypeCBZ,
			Name:     &name,
		}

		gen := NewKepubCBZGenerator()
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Verify the OPF uses file.Name, not book.Title
		opfData := readOPFFromKepub(t, destPath)
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `<dc:title>Custom File Name</dc:title>`)
		assert.NotContains(t, opfContent, `<dc:title>Book Title</dc:title>`)
	})

	t.Run("uses book.Title when file.Name is nil", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZForKepub(t, srcPath)

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		book := &models.Book{
			Title: "Book Title",
		}
		file := &models.File{
			FileType: models.FileTypeCBZ,
			Name:     nil,
		}

		gen := NewKepubCBZGenerator()
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// When file.Name is nil, should fall back to book.Title
		opfData := readOPFFromKepub(t, destPath)
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `<dc:title>Book Title</dc:title>`)
	})

	t.Run("uses book.Title when file.Name is empty", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZForKepub(t, srcPath)

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		emptyName := ""
		book := &models.Book{
			Title: "Book Title",
		}
		file := &models.File{
			FileType: models.FileTypeCBZ,
			Name:     &emptyName,
		}

		gen := NewKepubCBZGenerator()
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// When file.Name is empty, should fall back to book.Title
		opfData := readOPFFromKepub(t, destPath)
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `<dc:title>Book Title</dc:title>`)
	})

	t.Run("uses book.Title when file is nil", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZForKepub(t, srcPath)

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		book := &models.Book{
			Title: "Book Title",
		}

		gen := NewKepubCBZGenerator()
		err := gen.Generate(context.Background(), srcPath, destPath, book, nil)
		require.NoError(t, err)

		// When file is nil, should fall back to book.Title
		opfData := readOPFFromKepub(t, destPath)
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `<dc:title>Book Title</dc:title>`)
	})
}

func TestBuildCBZMetadata_Chapters(t *testing.T) {
	t.Parallel()
	t.Run("includes chapters from file", func(t *testing.T) {
		startPage0 := 0
		startPage5 := 5
		book := &models.Book{
			Title: "Comic Book",
		}
		file := &models.File{
			Chapters: []*models.Chapter{
				{Title: "Chapter 1", StartPage: &startPage0, SortOrder: 0},
				{Title: "Chapter 2", StartPage: &startPage5, SortOrder: 1},
			},
		}

		metadata := buildCBZMetadata(book, file)

		require.NotNil(t, metadata)
		require.Len(t, metadata.Chapters, 2)
		assert.Equal(t, "Chapter 1", metadata.Chapters[0].Title)
		assert.Equal(t, 0, metadata.Chapters[0].StartPage)
		assert.Equal(t, "Chapter 2", metadata.Chapters[1].Title)
		assert.Equal(t, 5, metadata.Chapters[1].StartPage)
	})

	t.Run("excludes nested children", func(t *testing.T) {
		startPage0 := 0
		startPage2 := 2
		book := &models.Book{
			Title: "Comic Book",
		}
		file := &models.File{
			Chapters: []*models.Chapter{
				{
					Title:     "Part 1",
					StartPage: &startPage0,
					SortOrder: 0,
					Children: []*models.Chapter{
						{Title: "Chapter 1.1", StartPage: &startPage2, SortOrder: 0},
					},
				},
			},
		}

		metadata := buildCBZMetadata(book, file)

		require.NotNil(t, metadata)
		// Should only have the top-level chapter, not children
		require.Len(t, metadata.Chapters, 1)
		assert.Equal(t, "Part 1", metadata.Chapters[0].Title)
	})

	t.Run("handles nil chapters", func(t *testing.T) {
		book := &models.Book{
			Title: "Comic Book",
		}
		file := &models.File{
			Chapters: nil,
		}

		metadata := buildCBZMetadata(book, file)

		require.NotNil(t, metadata)
		assert.Empty(t, metadata.Chapters)
	})

	t.Run("handles chapters with nil StartPage", func(t *testing.T) {
		book := &models.Book{
			Title: "Comic Book",
		}
		file := &models.File{
			Chapters: []*models.Chapter{
				{Title: "Chapter 1", StartPage: nil, SortOrder: 0},
			},
		}

		metadata := buildCBZMetadata(book, file)

		require.NotNil(t, metadata)
		require.Len(t, metadata.Chapters, 1)
		assert.Equal(t, "Chapter 1", metadata.Chapters[0].Title)
		assert.Equal(t, 0, metadata.Chapters[0].StartPage) // Defaults to 0
	})

	t.Run("sorts chapters by SortOrder", func(t *testing.T) {
		startPage0 := 0
		startPage5 := 5
		startPage10 := 10
		book := &models.Book{
			Title: "Comic Book",
		}
		file := &models.File{
			Chapters: []*models.Chapter{
				{Title: "Chapter 3", StartPage: &startPage10, SortOrder: 2},
				{Title: "Chapter 1", StartPage: &startPage0, SortOrder: 0},
				{Title: "Chapter 2", StartPage: &startPage5, SortOrder: 1},
			},
		}

		metadata := buildCBZMetadata(book, file)

		require.NotNil(t, metadata)
		require.Len(t, metadata.Chapters, 3)
		assert.Equal(t, "Chapter 1", metadata.Chapters[0].Title)
		assert.Equal(t, "Chapter 2", metadata.Chapters[1].Title)
		assert.Equal(t, "Chapter 3", metadata.Chapters[2].Title)
	})
}

func TestBuildCBZMetadata(t *testing.T) {
	t.Parallel()
	t.Run("sets Name from file when available", func(t *testing.T) {
		name := "Custom Name"
		book := &models.Book{
			Title: "Book Title",
		}
		file := &models.File{
			Name: &name,
		}

		metadata := buildCBZMetadata(book, file)

		require.NotNil(t, metadata)
		assert.Equal(t, "Book Title", metadata.Title)
		require.NotNil(t, metadata.Name)
		assert.Equal(t, "Custom Name", *metadata.Name)
	})

	t.Run("does not set Name when file is nil", func(t *testing.T) {
		book := &models.Book{
			Title: "Book Title",
		}

		metadata := buildCBZMetadata(book, nil)

		require.NotNil(t, metadata)
		assert.Equal(t, "Book Title", metadata.Title)
		assert.Nil(t, metadata.Name)
	})

	t.Run("does not set Name when file.Name is nil", func(t *testing.T) {
		book := &models.Book{
			Title: "Book Title",
		}
		file := &models.File{
			Name: nil,
		}

		metadata := buildCBZMetadata(book, file)

		require.NotNil(t, metadata)
		assert.Equal(t, "Book Title", metadata.Title)
		assert.Nil(t, metadata.Name)
	})

	t.Run("returns nil when book is nil", func(t *testing.T) {
		name := "Custom Name"
		file := &models.File{
			Name: &name,
		}

		metadata := buildCBZMetadata(nil, file)

		assert.Nil(t, metadata)
	})
}

// readNCXFromKepub reads the toc.ncx file from a KePub (EPUB) archive.
func readNCXFromKepub(t *testing.T, kepubPath string) []byte {
	t.Helper()

	r, err := zip.OpenReader(kepubPath)
	require.NoError(t, err)
	defer r.Close()

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "toc.ncx") || f.Name == "toc.ncx" {
			rc, err := f.Open()
			require.NoError(t, err)
			defer rc.Close()

			data, err := io.ReadAll(rc)
			require.NoError(t, err)
			return data
		}
	}

	t.Fatalf("toc.ncx not found in KePub")
	return nil
}

// readNavFromKepub reads the nav.xhtml file from a KePub (EPUB) archive.
func readNavFromKepub(t *testing.T, kepubPath string) []byte {
	t.Helper()

	r, err := zip.OpenReader(kepubPath)
	require.NoError(t, err)
	defer r.Close()

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "nav.xhtml") || f.Name == "nav.xhtml" {
			rc, err := f.Open()
			require.NoError(t, err)
			defer rc.Close()

			data, err := io.ReadAll(rc)
			require.NoError(t, err)
			return data
		}
	}

	t.Fatalf("nav.xhtml not found in KePub")
	return nil
}

func TestKepubCBZGenerator_Generate_WithChapters(t *testing.T) {
	t.Parallel()
	t.Run("generated KePub has chapter navigation", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a CBZ with 3 pages
		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZWithPages(t, srcPath, 3)

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		startPage0 := 0
		startPage2 := 2
		book := &models.Book{
			Title: "Test Comic",
		}
		file := &models.File{
			FileType: models.FileTypeCBZ,
			Chapters: []*models.Chapter{
				{Title: "Chapter 1", StartPage: &startPage0, SortOrder: 0},
				{Title: "Chapter 2", StartPage: &startPage2, SortOrder: 1},
			},
		}

		gen := NewKepubCBZGenerator()
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Verify NCX contains chapters
		ncxData := readNCXFromKepub(t, destPath)
		ncxContent := string(ncxData)
		assert.Contains(t, ncxContent, `<text>Chapter 1</text>`)
		assert.Contains(t, ncxContent, `<text>Chapter 2</text>`)
		assert.Contains(t, ncxContent, `<content src="page0001.xhtml"/>`)
		assert.Contains(t, ncxContent, `<content src="page0003.xhtml"/>`)
		// Should NOT have page-based navPoints
		assert.NotContains(t, ncxContent, `<text>Page 1</text>`)

		// Verify nav.xhtml contains chapters
		navData := readNavFromKepub(t, destPath)
		navContent := string(navData)
		assert.Contains(t, navContent, `<a href="page0001.xhtml">Chapter 1</a>`)
		assert.Contains(t, navContent, `<a href="page0003.xhtml">Chapter 2</a>`)
	})

	t.Run("chapters beyond page count are skipped", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a CBZ with only 2 pages
		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZWithPages(t, srcPath, 2)

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		startPage0 := 0
		startPage10 := 10 // Beyond page count
		book := &models.Book{
			Title: "Test Comic",
		}
		file := &models.File{
			FileType: models.FileTypeCBZ,
			Chapters: []*models.Chapter{
				{Title: "Chapter 1", StartPage: &startPage0, SortOrder: 0},
				{Title: "Chapter 2", StartPage: &startPage10, SortOrder: 1},
			},
		}

		gen := NewKepubCBZGenerator()
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Verify NCX only has Chapter 1
		ncxData := readNCXFromKepub(t, destPath)
		ncxContent := string(ncxData)
		assert.Contains(t, ncxContent, `<text>Chapter 1</text>`)
		assert.NotContains(t, ncxContent, `<text>Chapter 2</text>`)

		// Verify nav.xhtml only has Chapter 1
		navData := readNavFromKepub(t, destPath)
		navContent := string(navData)
		assert.Contains(t, navContent, `>Chapter 1</a>`)
		assert.NotContains(t, navContent, `>Chapter 2</a>`)
	})

	t.Run("falls back to page navigation when no chapters", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.cbz")
		createTestCBZWithPages(t, srcPath, 2)

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		book := &models.Book{
			Title: "Test Comic",
		}
		file := &models.File{
			FileType: models.FileTypeCBZ,
			Chapters: nil, // No chapters
		}

		gen := NewKepubCBZGenerator()
		err := gen.Generate(context.Background(), srcPath, destPath, book, file)
		require.NoError(t, err)

		// Verify NCX has page-based navPoints
		ncxData := readNCXFromKepub(t, destPath)
		ncxContent := string(ncxData)
		assert.Contains(t, ncxContent, `<text>Page 1</text>`)
		assert.Contains(t, ncxContent, `<text>Page 2</text>`)

		// Verify nav.xhtml uses title-only
		navData := readNavFromKepub(t, destPath)
		navContent := string(navData)
		assert.Contains(t, navContent, `<a href="page0001.xhtml">Test Comic</a>`)
	})
}

// createTestCBZWithPages creates a CBZ with the specified number of pages.
func createTestCBZWithPages(t *testing.T, path string, numPages int) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)

	// Create test images
	img := image.NewRGBA(image.Rect(0, 0, 100, 150))
	c := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	for y := 0; y < 150; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	imgData := buf.Bytes()

	for i := 1; i <= numPages; i++ {
		writer, err := w.Create(fmt.Sprintf("page%03d.jpg", i))
		require.NoError(t, err)
		_, err = writer.Write(imgData)
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())
}
