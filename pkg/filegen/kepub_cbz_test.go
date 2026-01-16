package filegen

import (
	"archive/zip"
	"bytes"
	"context"
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

func TestBuildCBZMetadata(t *testing.T) {
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
