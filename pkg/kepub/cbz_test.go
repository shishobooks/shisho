package kepub

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCBZOptions configures a test CBZ file.
type testCBZOptions struct {
	pages     []testPage
	extraFile string // non-image file to include
}

// testPage represents a page image in the test CBZ.
type testPage struct {
	filename string
	width    int
	height   int
	format   string // "jpeg" or "png"
}

// createTestImage creates a test image with the specified dimensions.
func createTestImage(width, height int, format string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with a solid color
	c := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}

	var buf bytes.Buffer
	switch format {
	case "png":
		_ = png.Encode(&buf, img)
	default: // jpeg
		_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	}
	return buf.Bytes()
}

// createTestCBZ creates a valid CBZ file for testing.
func createTestCBZ(t *testing.T, path string, opts testCBZOptions) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)

	// Default pages if none provided
	pages := opts.pages
	if len(pages) == 0 {
		pages = []testPage{
			{filename: "page001.jpg", width: 800, height: 1200, format: "jpeg"},
			{filename: "page002.jpg", width: 800, height: 1200, format: "jpeg"},
		}
	}

	for _, page := range pages {
		imgData := createTestImage(page.width, page.height, page.format)
		writer, err := w.Create(page.filename)
		require.NoError(t, err)
		_, err = writer.Write(imgData)
		require.NoError(t, err)
	}

	// Add extra non-image file if specified
	if opts.extraFile != "" {
		writer, err := w.Create(opts.extraFile)
		require.NoError(t, err)
		_, err = writer.Write([]byte("extra file content"))
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())
}

// readFileFromKepub reads a file from a KePub (EPUB) archive.
func readFileFromKepub(t *testing.T, kepubPath, fileName string) []byte {
	t.Helper()

	r, err := zip.OpenReader(kepubPath)
	require.NoError(t, err)
	defer r.Close()

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, fileName) || f.Name == fileName {
			rc, err := f.Open()
			require.NoError(t, err)
			defer rc.Close()

			data, err := io.ReadAll(rc)
			require.NoError(t, err)
			return data
		}
	}

	t.Fatalf("file %s not found in KePub", fileName)
	return nil
}

// listFilesInKepub returns all file paths in a KePub.
func listFilesInKepub(t *testing.T, kepubPath string) []string {
	t.Helper()

	r, err := zip.OpenReader(kepubPath)
	require.NoError(t, err)
	defer r.Close()

	var files []string
	for _, f := range r.File {
		files = append(files, f.Name)
	}
	return files
}

func TestConverter_ConvertCBZ(t *testing.T) {
	t.Parallel()
	t.Run("creates valid EPUB structure", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// Verify EPUB structure
		files := listFilesInKepub(t, destPath)
		assert.Contains(t, files, "mimetype")
		assert.Contains(t, files, "META-INF/container.xml")

		// Check mimetype is correct
		mimetypeData := readFileFromKepub(t, destPath, "mimetype")
		assert.Equal(t, "application/epub+zip", string(mimetypeData))
	})

	t.Run("includes OPF with cover metadata", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)

		// Should have cover image property and page references
		assert.Contains(t, opfContent, `cover-image`)
		assert.Contains(t, opfContent, `page0001.xhtml`)
		assert.Contains(t, opfContent, `<dc:title>`)
	})

	t.Run("preserves images byte-for-byte (lossless)", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create CBZ with specific image
		srcPath := filepath.Join(tmpDir, "comic.cbz")
		originalImageData := createTestImage(640, 480, "jpeg")

		f, err := os.Create(srcPath)
		require.NoError(t, err)
		w := zip.NewWriter(f)
		writer, err := w.Create("page001.jpg")
		require.NoError(t, err)
		_, err = writer.Write(originalImageData)
		require.NoError(t, err)
		require.NoError(t, w.Close())
		f.Close()

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err = converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// Read the image from the converted file
		resultImage := readFileFromKepub(t, destPath, "page0001.jpg")

		// Verify byte-for-byte equality
		assert.Equal(t, originalImageData, resultImage, "image should be preserved byte-for-byte")
	})

	t.Run("creates XHTML pages with KCC-style div and img", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 800, height: 1200, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		pageData := readFileFromKepub(t, destPath, "page0001.xhtml")
		pageContent := string(pageData)

		// Should use KCC-style div+img structure (not SVG)
		assert.Contains(t, pageContent, `<div style="text-align:center`)
		assert.Contains(t, pageContent, `<img width="800" height="1200"`)
		assert.Contains(t, pageContent, `src="images/page0001.jpg"`)
		assert.Contains(t, pageContent, `viewport`)
		assert.NotContains(t, pageContent, `<svg`)
	})

	t.Run("includes images with correct references", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				// Create image smaller than Kobo screen so it won't be resized
				{filename: "page1.jpg", width: 800, height: 1200, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// Check XHTML references images correctly via img tag
		pageData := readFileFromKepub(t, destPath, "page0001.xhtml")
		pageContent := string(pageData)

		assert.Contains(t, pageContent, `src="images/page0001.jpg"`)
		assert.Contains(t, pageContent, `width="800"`)
		assert.Contains(t, pageContent, `height="1200"`)
	})

	t.Run("maintains correct page order with natural sorting", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page2.jpg", width: 102, height: 100, format: "jpeg"},
				{filename: "page10.jpg", width: 110, height: 100, format: "jpeg"},
				{filename: "page1.jpg", width: 101, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// Each source page has its own width, so the width on each KePub
		// page names the image it shows: page1, page2, then page10.
		for i, width := range []int{101, 102, 110} {
			pageContent := string(readFileFromKepub(t, destPath, fmt.Sprintf("page%04d.xhtml", i+1)))
			assert.Contains(t, pageContent, fmt.Sprintf(`<img width="%d"`, width), "page %d", i+1)
		}
	})

	t.Run("includes NCX navigation file", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		ncxData := readFileFromKepub(t, destPath, "toc.ncx")
		ncxContent := string(ncxData)

		assert.Contains(t, ncxContent, "<ncx")
		assert.Contains(t, ncxContent, "<navMap>")
		assert.Contains(t, ncxContent, "navPoint")
	})

	t.Run("includes CSS for styling", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		cssData := readFileFromKepub(t, destPath, "styles.css")
		cssContent := string(cssData)

		// CSS should have basic reset styles
		assert.Contains(t, cssContent, "margin: 0")
		assert.Contains(t, cssContent, "padding: 0")
	})

	t.Run("ignores non-image files in CBZ", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
			},
			extraFile: "readme.txt",
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		files := listFilesInKepub(t, destPath)

		// Should not contain readme.txt
		for _, f := range files {
			assert.NotContains(t, f, "readme.txt")
		}
	})

	t.Run("converts PNG to JPEG", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.png", width: 100, height: 100, format: "png"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)

		// PNG should be converted to JPEG
		assert.Contains(t, opfContent, `media-type="image/jpeg"`)
		assert.NotContains(t, opfContent, `media-type="image/png"`)

		// Check that the file extension was changed
		assert.Contains(t, opfContent, `images/page0002.jpg`)
	})

	t.Run("resizes large images to fit Kobo screen", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		// Create image larger than Kobo screen (1196x1680)
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 1500, height: 3000, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// Read the resized image
		imgData := readFileFromKepub(t, destPath, "page0001.jpg")

		// Decode and check dimensions
		img, _, err := image.DecodeConfig(bytes.NewReader(imgData))
		require.NoError(t, err)

		// Should fit within Kobo screen dimensions (1196x1680)
		// Height ratio: 1680/3000 = 0.56, Width ratio: 1196/1500 = 0.797
		// Use smaller ratio (0.56) to fit: 1500*0.56=840, 3000*0.56=1680
		assert.Equal(t, 1680, img.Height, "image height should fit Kobo screen")
		assert.Equal(t, 840, img.Width, "image width should be proportionally scaled")
	})

	t.Run("preserves small images unchanged", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		// Create image smaller than Kobo screen (1196x1680)
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 800, height: 1200, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// Read the image
		imgData := readFileFromKepub(t, destPath, "page0001.jpg")

		// Decode and check dimensions
		img, _, err := image.DecodeConfig(bytes.NewReader(imgData))
		require.NoError(t, err)

		// Dimensions should be unchanged since image fits within Kobo screen
		assert.Equal(t, 800, img.Width, "image width should be unchanged")
		assert.Equal(t, 1200, img.Height, "image height should be unchanged")
	})

	t.Run("returns error for empty CBZ", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create empty CBZ (no images)
		srcPath := filepath.Join(tmpDir, "empty.cbz")
		f, err := os.Create(srcPath)
		require.NoError(t, err)
		w := zip.NewWriter(f)
		// Only add a non-image file
		writer, err := w.Create("readme.txt")
		require.NoError(t, err)
		_, err = writer.Write([]byte("no images here"))
		require.NoError(t, err)
		require.NoError(t, w.Close())
		f.Close()

		destPath := filepath.Join(tmpDir, "empty.kepub.epub")

		converter := NewConverter()
		err = converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no images found")
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "nonexistent.cbz")
		destPath := filepath.Join(tmpDir, "output.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open source file")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		converter := NewConverter()
		err := converter.ConvertCBZ(ctx, srcPath, destPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cancelled")
	})
}

func TestImageMediaType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ext      string
		expected string
	}{
		{".jpg", "image/jpeg"},
		{".jpeg", "image/jpeg"},
		{".png", "image/png"},
		{".gif", "image/gif"},
		{".webp", "image/webp"},
		{".unknown", "image/jpeg"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			result := imageMediaType(tt.ext)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConverter_ConvertCBZWithMetadata(t *testing.T) {
	t.Parallel()
	t.Run("uses title from metadata", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Batman: Year One",
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `<dc:title>Batman: Year One</dc:title>`)

		ncxData := readFileFromKepub(t, destPath, "toc.ncx")
		ncxContent := string(ncxData)
		assert.Contains(t, ncxContent, `Batman: Year One`)
	})

	t.Run("includes authors from metadata", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Test Comic",
			Authors: []CBZAuthor{
				{Name: "Frank Miller", Role: "writer"},
				{Name: "David Mazzucchelli", Role: "penciller"},
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `>Frank Miller</dc:creator>`)
		assert.Contains(t, opfContent, `>David Mazzucchelli</dc:creator>`)
	})

	t.Run("includes author sort name as file-as", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Test Comic",
			Authors: []CBZAuthor{
				{Name: "Jane Doe", SortName: "Doe, Jane", Role: "writer"},
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `<dc:creator id="author0">Jane Doe</dc:creator>`)
		assert.Contains(t, opfContent, `<meta refines="#author0" property="file-as">Doe, Jane</meta>`)
	})

	t.Run("deduplicates authors with same name and role", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Test Comic",
			Authors: []CBZAuthor{
				{Name: "Jim Lee", Role: "writer"},
				{Name: "Jim Lee", Role: "penciller"},
				{Name: "Jim Lee", Role: "writer"}, // Duplicate - should be skipped
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)

		// Count occurrences of Jim Lee (now with id attribute)
		count := strings.Count(opfContent, `>Jim Lee</dc:creator>`)
		assert.Equal(t, 2, count, "should have 2 entries for Jim Lee (writer and penciller)")
	})

	t.Run("includes series from metadata", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		seriesNum := 5.0
		metadata := &CBZMetadata{
			Title: "Test Comic",
			Series: []CBZSeries{
				{Name: "Batman", Number: &seriesNum},
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)
		// Verify EPUB3 series metadata with proper id and refines attributes
		assert.Contains(t, opfContent, `<meta property="belongs-to-collection" id="series-1">Batman</meta>`)
		assert.Contains(t, opfContent, `<meta refines="#series-1" property="collection-type">series</meta>`)
		assert.Contains(t, opfContent, `<meta refines="#series-1" property="group-position">5</meta>`)
	})

	t.Run("escapes special XML characters in metadata", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Batman & Robin: <Heroes>",
			Authors: []CBZAuthor{
				{Name: "O'Neil & Adams", Role: "writer"},
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)

		// Should be XML-escaped
		assert.Contains(t, opfContent, `Batman &amp; Robin: &lt;Heroes&gt;`)
		assert.Contains(t, opfContent, `O&#39;Neil &amp; Adams`)
	})

	t.Run("uses default title when metadata is nil", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, nil)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `<dc:title>Comic Book</dc:title>`)
	})
}

func TestConverter_ConvertCBZWithMetadata_UsesNameForTitle(t *testing.T) {
	t.Parallel()
	t.Run("uses Name for title when provided", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		name := "Custom File Name"
		metadata := &CBZMetadata{
			Title: "Book Title",
			Name:  &name,
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		// Verify OPF uses Name, not Title
		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `<dc:title>Custom File Name</dc:title>`)
		assert.NotContains(t, opfContent, `<dc:title>Book Title</dc:title>`)

		// Verify NCX uses Name
		ncxData := readFileFromKepub(t, destPath, "toc.ncx")
		ncxContent := string(ncxData)
		assert.Contains(t, ncxContent, `Custom File Name`)

		// Verify nav.xhtml uses Name
		navData := readFileFromKepub(t, destPath, "nav.xhtml")
		navContent := string(navData)
		assert.Contains(t, navContent, `Custom File Name`)
	})

	t.Run("falls back to Title when Name is nil", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Book Title",
			Name:  nil,
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `<dc:title>Book Title</dc:title>`)
	})

	t.Run("falls back to Title when Name is empty", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		emptyName := ""
		metadata := &CBZMetadata{
			Title: "Book Title",
			Name:  &emptyName,
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)
		assert.Contains(t, opfContent, `<dc:title>Book Title</dc:title>`)
	})
}

func TestGenerateNCX_WithChapters(t *testing.T) {
	t.Parallel()
	t.Run("includes chapter navPoints when chapters provided", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page3.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Test Comic",
			Chapters: []CBZChapter{
				{Title: "Chapter 1", StartPage: 0},
				{Title: "Chapter 2", StartPage: 2},
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		ncxData := readFileFromKepub(t, destPath, "toc.ncx")
		ncxContent := string(ncxData)

		// Should have chapter navPoints
		assert.Contains(t, ncxContent, `<text>Chapter 1</text>`)
		assert.Contains(t, ncxContent, `<text>Chapter 2</text>`)
		assert.Contains(t, ncxContent, `<content src="page0001.xhtml"/>`)
		assert.Contains(t, ncxContent, `<content src="page0003.xhtml"/>`)
		// Should NOT have "Page 1", "Page 2", "Page 3" labels
		assert.NotContains(t, ncxContent, `<text>Page 1</text>`)
		assert.NotContains(t, ncxContent, `<text>Page 2</text>`)
		assert.NotContains(t, ncxContent, `<text>Page 3</text>`)
	})

	t.Run("uses page navPoints when no chapters", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title:    "Test Comic",
			Chapters: nil, // No chapters
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		ncxData := readFileFromKepub(t, destPath, "toc.ncx")
		ncxContent := string(ncxData)

		// Should have page-based navPoints
		assert.Contains(t, ncxContent, `<text>Page 1</text>`)
		assert.Contains(t, ncxContent, `<text>Page 2</text>`)
	})

	t.Run("escapes special characters in chapter titles", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Test Comic",
			Chapters: []CBZChapter{
				{Title: "Part 1 & <Beginning>", StartPage: 0},
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		ncxData := readFileFromKepub(t, destPath, "toc.ncx")
		ncxContent := string(ncxData)

		// Should be XML-escaped
		assert.Contains(t, ncxContent, `Part 1 &amp; &lt;Beginning&gt;`)
	})

	t.Run("skips chapters beyond page count", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Test Comic",
			Chapters: []CBZChapter{
				{Title: "Chapter 1", StartPage: 0},
				{Title: "Chapter 2", StartPage: 10}, // Beyond page count (only 2 pages)
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		ncxData := readFileFromKepub(t, destPath, "toc.ncx")
		ncxContent := string(ncxData)

		// Should only have Chapter 1
		assert.Contains(t, ncxContent, `<text>Chapter 1</text>`)
		assert.NotContains(t, ncxContent, `<text>Chapter 2</text>`)
	})
}

func TestGenerateNavXHTML_WithChapters(t *testing.T) {
	t.Parallel()
	t.Run("includes chapter TOC when chapters provided", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page3.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Test Comic",
			Chapters: []CBZChapter{
				{Title: "Chapter 1", StartPage: 0},
				{Title: "Chapter 2", StartPage: 2},
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		navData := readFileFromKepub(t, destPath, "nav.xhtml")
		navContent := string(navData)

		// Should have chapter entries in the TOC
		assert.Contains(t, navContent, `<a href="page0001.xhtml">Chapter 1</a>`)
		assert.Contains(t, navContent, `<a href="page0003.xhtml">Chapter 2</a>`)
	})

	t.Run("uses title-only when no chapters", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title:    "Test Comic",
			Chapters: nil, // No chapters
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		navData := readFileFromKepub(t, destPath, "nav.xhtml")
		navContent := string(navData)

		// Should have single title entry (current behavior)
		assert.Contains(t, navContent, `<a href="page0001.xhtml">Test Comic</a>`)
		// Should NOT have Chapter 1, Chapter 2 entries
		assert.NotContains(t, navContent, `Chapter 1`)
		assert.NotContains(t, navContent, `Chapter 2`)
	})

	t.Run("escapes special characters in chapter titles", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Test Comic",
			Chapters: []CBZChapter{
				{Title: "Part 1 & <Beginning>", StartPage: 0},
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		navData := readFileFromKepub(t, destPath, "nav.xhtml")
		navContent := string(navData)

		// Should be XML-escaped
		assert.Contains(t, navContent, `Part 1 &amp; &lt;Beginning&gt;`)
	})

	t.Run("skips chapters beyond page count", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "comic.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		metadata := &CBZMetadata{
			Title: "Test Comic",
			Chapters: []CBZChapter{
				{Title: "Chapter 1", StartPage: 0},
				{Title: "Chapter 2", StartPage: 10}, // Beyond page count (only 2 pages)
			},
		}

		converter := NewConverter()
		err := converter.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		navData := readFileFromKepub(t, destPath, "nav.xhtml")
		navContent := string(navData)

		// Should only have Chapter 1
		assert.Contains(t, navContent, `>Chapter 1</a>`)
		assert.NotContains(t, navContent, `>Chapter 2</a>`)
	})
}

// Pages keep their tones and colors: a gray page keeps every gray level,
// and a gray page with a small colored element stays in color. Only the
// size and, for PNGs, the format change.
func TestProcessImageForEreader_KeepsTonesAndColor(t *testing.T) {
	t.Parallel()

	t.Run("gray page keeps every level", func(t *testing.T) {
		t.Parallel()
		img := image.NewGray(image.Rect(0, 0, 2048, 2730))
		for y := range 2730 {
			for x := range 2048 {
				img.SetGray(x, y, color.Gray{Y: uint8(x * 256 / 2048)})
			}
		}
		var buf bytes.Buffer
		require.NoError(t, png.Encode(&buf, img))

		processed := ProcessImageForEreader(buf.Bytes(), ".png")
		require.Equal(t, ".jpg", processed.Ext)
		out, err := jpeg.Decode(bytes.NewReader(processed.Data))
		require.NoError(t, err)
		levels := map[uint8]bool{}
		b := out.Bounds()
		for x := b.Min.X; x < b.Max.X; x++ {
			levels[color.GrayModel.Convert(out.At(x, b.Min.Y+b.Dy()/2)).(color.Gray).Y] = true
		}
		assert.Greater(t, len(levels), 100, "a gradient across the page should keep far more than 16 levels")
	})

	t.Run("gray page with a colored element stays in color", func(t *testing.T) {
		t.Parallel()
		img := image.NewRGBA(image.Rect(0, 0, 2048, 2730))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 128, G: 128, B: 128, A: 255}), image.Point{}, draw.Src)
		red := image.Rect(0, 0, 200, 200)
		draw.Draw(img, red, image.NewUniform(color.RGBA{R: 255, A: 255}), image.Point{}, draw.Src)
		var buf bytes.Buffer
		require.NoError(t, png.Encode(&buf, img))

		processed := ProcessImageForEreader(buf.Bytes(), ".png")
		out, err := jpeg.Decode(bytes.NewReader(processed.Data))
		require.NoError(t, err)
		r, g, _, _ := out.At(20, 20).RGBA()
		assert.Greater(t, r>>8, uint32(200), "the red element should stay red")
		assert.Less(t, g>>8, uint32(60), "the red element should stay red")
	})

	// Scans are often saved as palette PNGs, which decode as paletted
	// rather than gray.
	t.Run("palette PNG of grays becomes a gray JPEG", func(t *testing.T) {
		t.Parallel()
		palettePage := func(pal color.Palette) []byte {
			img := image.NewPaletted(image.Rect(0, 0, 2000, 3000), pal)
			for i := range img.Pix {
				img.Pix[i] = uint8(i % len(pal))
			}
			var buf bytes.Buffer
			require.NoError(t, png.Encode(&buf, img))
			return buf.Bytes()
		}
		grays := color.Palette{}
		for i := range 16 {
			grays = append(grays, color.Gray{Y: uint8(i * 17)})
		}
		out, err := jpeg.Decode(bytes.NewReader(ProcessImageForEreader(palettePage(grays), ".png").Data))
		require.NoError(t, err)
		assert.IsType(t, &image.Gray{}, out)

		// A page that fits the screen is not resized, and still comes out gray.
		small := image.NewPaletted(image.Rect(0, 0, 800, 1200), grays)
		var smallPNG bytes.Buffer
		require.NoError(t, png.Encode(&smallPNG, small))
		out, err = jpeg.Decode(bytes.NewReader(ProcessImageForEreader(smallPNG.Bytes(), ".png").Data))
		require.NoError(t, err)
		assert.IsType(t, &image.Gray{}, out, "a palette PNG that fits the screen")

		withRed := append(color.Palette{color.RGBA{R: 255, A: 255}}, grays[1:]...)
		out, err = jpeg.Decode(bytes.NewReader(ProcessImageForEreader(palettePage(withRed), ".png").Data))
		require.NoError(t, err)
		assert.IsType(t, &image.YCbCr{}, out, "a palette with a color in it stays in color")
	})

	t.Run("16-bit gray PNG that fits the screen becomes a gray JPEG", func(t *testing.T) {
		t.Parallel()
		img := image.NewGray16(image.Rect(0, 0, 800, 1200))
		var buf bytes.Buffer
		require.NoError(t, png.Encode(&buf, img))
		out, err := jpeg.Decode(bytes.NewReader(ProcessImageForEreader(buf.Bytes(), ".png").Data))
		require.NoError(t, err)
		assert.IsType(t, &image.Gray{}, out)
	})

	// A page thinner than one output pixel at the screen's scale keeps one.
	t.Run("very tall page", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		require.NoError(t, png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 20000))))
		processed := ProcessImageForEreader(buf.Bytes(), ".png")
		assert.Equal(t, 1, processed.Width)
		assert.Equal(t, koboHeight, processed.Height)
	})

	t.Run("gray JPEG that fits the screen is passed through", func(t *testing.T) {
		t.Parallel()
		img := image.NewGray(image.Rect(0, 0, 800, 1200))
		for y := range 1200 {
			for x := range 800 {
				img.SetGray(x, y, color.Gray{Y: uint8(x + y)})
			}
		}
		var buf bytes.Buffer
		require.NoError(t, jpeg.Encode(&buf, img, nil))

		processed := ProcessImageForEreader(buf.Bytes(), ".jpg")
		assert.Equal(t, buf.Bytes(), processed.Data)
	})
}

func TestConvertCBZWithMetadata_CoverPage(t *testing.T) {
	t.Parallel()
	t.Run("uses specified cover page for cover-image property", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test CBZ with 3 pages
		srcPath := filepath.Join(tmpDir, "test.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page3.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "output.kepub.epub")

		coverPage := 2 // Third page (0-indexed)
		metadata := &CBZMetadata{
			Title:     "Test Comic",
			CoverPage: &coverPage,
		}

		c := NewConverter()
		err := c.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		// Verify the OPF has cover-image on the correct page
		opfContent := string(readFileFromKepub(t, destPath, "content.opf"))

		// Check that cover meta tag references the correct image (img0003 for page index 2)
		assert.Contains(t, opfContent, `<meta name="cover" content="img0003"/>`)

		// Check that cover-image property is on the third image (img0003)
		// The manifest should have img0003 with properties="cover-image"
		assert.Contains(t, opfContent, `id="img0003" href="images/page0003.jpg" media-type="image/jpeg" properties="cover-image"`)

		// First and second images should NOT have cover-image property
		assert.Contains(t, opfContent, `id="img0001" href="images/page0001.jpg" media-type="image/jpeg"/>`)
		assert.Contains(t, opfContent, `id="img0002" href="images/page0002.jpg" media-type="image/jpeg"/>`)
	})

	t.Run("defaults to first page when CoverPage is nil", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "test.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "output.kepub.epub")

		metadata := &CBZMetadata{
			Title:     "Test Comic",
			CoverPage: nil, // Not specified
		}

		c := NewConverter()
		err := c.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfContent := string(readFileFromKepub(t, destPath, "content.opf"))

		// Should default to first page (img0001)
		assert.Contains(t, opfContent, `<meta name="cover" content="img0001"/>`)
		assert.Contains(t, opfContent, `id="img0001" href="images/page0001.jpg" media-type="image/jpeg" properties="cover-image"`)
	})

	t.Run("defaults to first page when metadata is nil", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "test.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "output.kepub.epub")

		c := NewConverter()
		err := c.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, nil)
		require.NoError(t, err)

		opfContent := string(readFileFromKepub(t, destPath, "content.opf"))

		// Should default to first page (img0001)
		assert.Contains(t, opfContent, `<meta name="cover" content="img0001"/>`)
		assert.Contains(t, opfContent, `id="img0001" href="images/page0001.jpg" media-type="image/jpeg" properties="cover-image"`)
	})

	t.Run("clamps out-of-bounds cover page to first page", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "test.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "output.kepub.epub")

		// Cover page index 10 is out of bounds for a 2-page CBZ
		coverPage := 10
		metadata := &CBZMetadata{
			Title:     "Test Comic",
			CoverPage: &coverPage,
		}

		c := NewConverter()
		err := c.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfContent := string(readFileFromKepub(t, destPath, "content.opf"))

		// Should clamp to first page (img0001) since index 10 exceeds page count
		assert.Contains(t, opfContent, `<meta name="cover" content="img0001"/>`)
		assert.Contains(t, opfContent, `id="img0001" href="images/page0001.jpg" media-type="image/jpeg" properties="cover-image"`)
	})

	t.Run("clamps negative cover page to first page", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "test.cbz")
		createTestCBZ(t, srcPath, testCBZOptions{
			pages: []testPage{
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "output.kepub.epub")

		// Negative cover page index
		coverPage := -1
		metadata := &CBZMetadata{
			Title:     "Test Comic",
			CoverPage: &coverPage,
		}

		c := NewConverter()
		err := c.ConvertCBZWithMetadata(context.Background(), srcPath, destPath, metadata)
		require.NoError(t, err)

		opfContent := string(readFileFromKepub(t, destPath, "content.opf"))

		// Should clamp to first page (img0001) since index -1 is invalid
		assert.Contains(t, opfContent, `<meta name="cover" content="img0001"/>`)
		assert.Contains(t, opfContent, `id="img0001" href="images/page0001.jpg" media-type="image/jpeg" properties="cover-image"`)
	})
}

// The NCX names the same book as the OPF: EPUB readers and validators
// require its dtb:uid to equal the OPF's unique identifier.
func TestConvertCBZ_NCXIdentifierMatchesOPF(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "comic.cbz")
	destPath := filepath.Join(dir, "comic.kepub.epub")
	createTestCBZ(t, srcPath, testCBZOptions{})
	require.NoError(t, NewConverter().ConvertCBZ(context.Background(), srcPath, destPath))

	opfID := regexp.MustCompile(`<dc:identifier id="uid">([^<]+)</dc:identifier>`).FindStringSubmatch(string(readFileFromKepub(t, destPath, "content.opf")))
	require.NotNil(t, opfID)
	ncxID := regexp.MustCompile(`<meta name="dtb:uid" content="([^"]+)"/>`).FindStringSubmatch(string(readFileFromKepub(t, destPath, "toc.ncx")))
	require.NotNil(t, ncxID)
	assert.Equal(t, opfID[1], ncxID[1])
}
