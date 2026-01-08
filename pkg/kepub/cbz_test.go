package kepub

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
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
				{filename: "page2.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page10.jpg", width: 100, height: 100, format: "jpeg"},
				{filename: "page1.jpg", width: 100, height: 100, format: "jpeg"},
			},
		})

		destPath := filepath.Join(tmpDir, "comic.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertCBZ(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// Read the OPF to check image order (page0001 = page1, page0002 = page2, page0003 = page10)
		opfData := readFileFromKepub(t, destPath, "content.opf")
		opfContent := string(opfData)

		// Images should be in manifest in correct order
		img1Pos := strings.Index(opfContent, "img0001")
		img2Pos := strings.Index(opfContent, "img0002")
		img3Pos := strings.Index(opfContent, "img0003")

		assert.Less(t, img1Pos, img2Pos, "img0001 should come before img0002 in manifest")
		assert.Less(t, img2Pos, img3Pos, "img0002 should come before img0003 in manifest")
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

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"page.jpg", true},
		{"page.jpeg", true},
		{"page.JPG", true},
		{"page.png", true},
		{"page.PNG", true},
		{"page.gif", true},
		{"page.webp", true},
		{"readme.txt", false},
		{"comic.cbz", false},
		{"metadata.xml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isImageFile(tt.name)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestImageMediaType(t *testing.T) {
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

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool
	}{
		{"page1", "page2", true},
		{"page2", "page10", true},
		{"page10", "page2", false},
		{"1", "2", true},
		{"page001", "page002", true},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			result := naturalLess(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"page1", 1},
		{"page10", 10},
		{"123", 123},
		{"page001.jpg", 1},
		{"nonum", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractNumber(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConverter_ConvertCBZWithMetadata(t *testing.T) {
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

func TestIsGrayscaleImage(t *testing.T) {
	t.Run("detects grayscale image", func(t *testing.T) {
		// Create a grayscale image (all pixels have R=G=B)
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		gray := color.RGBA{R: 128, G: 128, B: 128, A: 255}
		for y := 0; y < 100; y++ {
			for x := 0; x < 100; x++ {
				img.Set(x, y, gray)
			}
		}
		assert.True(t, isGrayscaleImage(img), "should detect grayscale image")
	})

	t.Run("detects color image", func(t *testing.T) {
		// Create a color image with distinct R, G, B values
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
		for y := 0; y < 100; y++ {
			for x := 0; x < 100; x++ {
				img.Set(x, y, red)
			}
		}
		assert.False(t, isGrayscaleImage(img), "should detect color image")
	})

	t.Run("detects native Gray image type", func(t *testing.T) {
		img := image.NewGray(image.Rect(0, 0, 100, 100))
		assert.True(t, isGrayscaleImage(img), "should detect native Gray image")
	})

	t.Run("allows small color elements in grayscale", func(t *testing.T) {
		// Create mostly grayscale image with 1% color pixels
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		gray := color.RGBA{R: 128, G: 128, B: 128, A: 255}
		red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
		for y := 0; y < 100; y++ {
			for x := 0; x < 100; x++ {
				if x == 0 && y == 0 {
					img.Set(x, y, red) // Only 1 color pixel
				} else {
					img.Set(x, y, gray)
				}
			}
		}
		assert.True(t, isGrayscaleImage(img), "should still be grayscale with tiny color elements")
	})

	t.Run("rejects small images", func(t *testing.T) {
		// Very small images should not be converted
		img := image.NewRGBA(image.Rect(0, 0, 10, 10))
		gray := color.RGBA{R: 128, G: 128, B: 128, A: 255}
		for y := 0; y < 10; y++ {
			for x := 0; x < 10; x++ {
				img.Set(x, y, gray)
			}
		}
		assert.False(t, isGrayscaleImage(img), "should not convert tiny images")
	})
}
