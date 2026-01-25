package kepub

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEPUBOptions configures a test EPUB file.
type testEPUBOptions struct {
	title        string
	authors      []string
	coverData    []byte
	chapters     []testChapter
	hasCoverMeta bool
}

// testChapter represents a chapter in the test EPUB.
type testChapter struct {
	id      string
	content string
}

// createTestEPUB creates a valid EPUB file for testing.
// Uses root-level OPF to match the structure that works in filegen tests.
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

	// Build manifest items
	var manifestItems strings.Builder
	var spineItems strings.Builder

	// Default chapter if none provided
	chapters := opts.chapters
	if len(chapters) == 0 {
		chapters = []testChapter{
			{id: "chapter1", content: "<p>Chapter 1 content. This is a test sentence.</p>"},
		}
	}

	for _, ch := range chapters {
		manifestItems.WriteString(`    <item id="` + ch.id + `" href="` + ch.id + `.xhtml" media-type="application/xhtml+xml"/>` + "\n")
		spineItems.WriteString(`    <itemref idref="` + ch.id + `"/>` + "\n")
	}

	var coverMeta string
	if opts.hasCoverMeta || len(opts.coverData) > 0 {
		coverMeta = `    <meta name="cover" content="cover-image"/>` + "\n"
		manifestItems.WriteString(`    <item id="cover-image" href="cover.jpg" media-type="image/jpeg"/>` + "\n")
	}

	// Build authors
	var authorsXML strings.Builder
	for _, author := range opts.authors {
		authorsXML.WriteString(`    <dc:creator>` + author + `</dc:creator>` + "\n")
	}

	title := opts.title
	if title == "" {
		title = "Test Book"
	}

	opfContent := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>` + title + `</dc:title>
` + authorsXML.String() + `    <dc:identifier id="bookid">test-book-id</dc:identifier>
    <dc:language>en</dc:language>
` + coverMeta + `  </metadata>
  <manifest>
` + manifestItems.String() + `  </manifest>
  <spine>
` + spineItems.String() + `  </spine>
</package>`

	opfWriter, err := w.Create("content.opf")
	require.NoError(t, err)
	_, err = opfWriter.Write([]byte(opfContent))
	require.NoError(t, err)

	// Add chapters
	for _, ch := range chapters {
		chapterWriter, err := w.Create(ch.id + ".xhtml")
		require.NoError(t, err)
		_, err = chapterWriter.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>` + ch.id + `</title></head>
<body>` + ch.content + `</body>
</html>`))
		require.NoError(t, err)
	}

	// Add cover if provided
	if len(opts.coverData) > 0 {
		coverWriter, err := w.Create("cover.jpg")
		require.NoError(t, err)
		_, err = coverWriter.Write(opts.coverData)
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())
}

// readFileFromEPUB reads a file from an EPUB archive.
func readFileFromEPUB(t *testing.T, epubPath, fileName string) []byte {
	t.Helper()

	r, err := zip.OpenReader(epubPath)
	require.NoError(t, err)
	defer r.Close()

	for _, f := range r.File {
		if f.Name == fileName || strings.HasSuffix(f.Name, "/"+fileName) {
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

// listFilesInEPUB returns all file paths in an EPUB.
func listFilesInEPUB(t *testing.T, epubPath string) []string {
	t.Helper()

	r, err := zip.OpenReader(epubPath)
	require.NoError(t, err)
	defer r.Close()

	var files []string
	for _, f := range r.File {
		files = append(files, f.Name)
	}
	return files
}

func TestConverter_ConvertEPUB(t *testing.T) {
	t.Parallel()
	t.Run("adds Kobo spans to content files", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title: "Test Book",
			chapters: []testChapter{
				{id: "chapter1", content: "<p>Hello world. This is a test.</p>"},
			},
		})

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertEPUB(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// Check that the destination file exists
		_, err = os.Stat(destPath)
		require.NoError(t, err)

		// Read the chapter content and verify spans were added
		chapterData := readFileFromEPUB(t, destPath, "chapter1.xhtml")
		content := string(chapterData)
		assert.Contains(t, content, `class="koboSpan"`)
		assert.Contains(t, content, `id="kobo.`)
	})

	t.Run("adds wrapper divs to body", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			title: "Test Book",
		})

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertEPUB(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		chapterData := readFileFromEPUB(t, destPath, "chapter1.xhtml")
		content := string(chapterData)
		assert.Contains(t, content, `id="book-columns"`)
		assert.Contains(t, content, `id="book-inner"`)
	})

	t.Run("preserves original text content", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			chapters: []testChapter{
				{id: "chapter1", content: "<p>Unique text content 12345.</p>"},
			},
		})

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertEPUB(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		chapterData := readFileFromEPUB(t, destPath, "chapter1.xhtml")
		assert.Contains(t, string(chapterData), "Unique text content 12345.")
	})

	t.Run("includes OPF in output", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			hasCoverMeta: true,
			coverData:    []byte("fake cover data"),
		})

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertEPUB(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// Verify OPF is present and contains expected metadata
		opfData := readFileFromEPUB(t, destPath, "content.opf")
		assert.Contains(t, string(opfData), `<package`)
		assert.Contains(t, string(opfData), `Test Book`)
		assert.Contains(t, string(opfData), `<manifest>`)
	})

	t.Run("preserves non-content files unchanged", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		coverData := []byte("original cover image bytes")
		createTestEPUB(t, srcPath, testEPUBOptions{
			coverData: coverData,
		})

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertEPUB(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// Cover image should be preserved byte-for-byte
		resultCover := readFileFromEPUB(t, destPath, "cover.jpg")
		assert.Equal(t, coverData, resultCover)

		// Mimetype should be preserved
		mimetypeData := readFileFromEPUB(t, destPath, "mimetype")
		assert.Equal(t, "application/epub+zip", string(mimetypeData))
	})

	t.Run("handles multiple content files", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			chapters: []testChapter{
				{id: "chapter1", content: "<p>Chapter 1 text.</p>"},
				{id: "chapter2", content: "<p>Chapter 2 text.</p>"},
				{id: "chapter3", content: "<p>Chapter 3 text.</p>"},
			},
		})

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertEPUB(context.Background(), srcPath, destPath)
		require.NoError(t, err)

		// All chapters should have Kobo spans
		for _, chID := range []string{"chapter1", "chapter2", "chapter3"} {
			chapterData := readFileFromEPUB(t, destPath, chID+".xhtml")
			assert.Contains(t, string(chapterData), `class="koboSpan"`, "chapter %s should have spans", chID)
		}
	})

	t.Run("returns error for non-existent source file", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "nonexistent.epub")
		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		converter := NewConverter()
		err := converter.ConvertEPUB(context.Background(), srcPath, destPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open source file")
	})

	t.Run("returns error for invalid zip file", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "invalid.epub")
		err := os.WriteFile(srcPath, []byte("not a valid zip file"), 0644)
		require.NoError(t, err)

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		converter := NewConverter()
		err = converter.ConvertEPUB(context.Background(), srcPath, destPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "zip")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{})

		destPath := filepath.Join(tmpDir, "dest.kepub.epub")

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		converter := NewConverter()
		err := converter.ConvertEPUB(ctx, srcPath, destPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cancelled")
	})

	t.Run("conversion is idempotent", func(t *testing.T) {
		tmpDir := t.TempDir()

		srcPath := filepath.Join(tmpDir, "source.epub")
		createTestEPUB(t, srcPath, testEPUBOptions{
			chapters: []testChapter{
				{id: "chapter1", content: "<p>Test content.</p>"},
			},
		})

		// First conversion
		firstDest := filepath.Join(tmpDir, "first.kepub.epub")
		converter := NewConverter()
		err := converter.ConvertEPUB(context.Background(), srcPath, firstDest)
		require.NoError(t, err)

		// Second conversion of the already-converted file
		secondDest := filepath.Join(tmpDir, "second.kepub.epub")
		err = converter.ConvertEPUB(context.Background(), firstDest, secondDest)
		require.NoError(t, err)

		// Both should have the same structure (though span IDs might differ)
		firstFiles := listFilesInEPUB(t, firstDest)
		secondFiles := listFilesInEPUB(t, secondDest)
		assert.Len(t, secondFiles, len(firstFiles))
	})
}

func TestExtractManifestItems(t *testing.T) {
	t.Parallel()
	t.Run("extracts items from manifest", func(t *testing.T) {
		opf := `<manifest>
			<item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
			<item id="cover" href="cover.jpg" media-type="image/jpeg"/>
		</manifest>`

		items := extractManifestItems([]byte(opf))

		require.Len(t, items, 2)
		assert.Equal(t, "chapter1", items[0].ID)
		assert.Equal(t, "chapter1.xhtml", items[0].Href)
		assert.Equal(t, "application/xhtml+xml", items[0].MediaType)
	})

	t.Run("handles empty manifest", func(t *testing.T) {
		opf := `<manifest></manifest>`
		items := extractManifestItems([]byte(opf))
		assert.Empty(t, items)
	})
}

func TestExtractAttribute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tag      string
		attr     string
		expected string
	}{
		{`<item id="test"/>`, "id", "test"},
		{`<item href='chapter.xhtml'/>`, "href", "chapter.xhtml"},
		{`<item id="cover" href="cover.jpg"/>`, "href", "cover.jpg"},
		{`<item id="test"/>`, "nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.attr, func(t *testing.T) {
			result := extractAttribute(tt.tag, tt.attr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsContentFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mediaType string
		expected  bool
	}{
		{"application/xhtml+xml", true},
		{"text/html", true},
		{"application/x-dtbncx+xml", false}, // NCX is navigation, not content
		{"image/jpeg", false},
		{"text/css", false},
		{"application/epub+zip", false},
	}

	for _, tt := range tests {
		t.Run(tt.mediaType, func(t *testing.T) {
			result := isContentFile(tt.mediaType)
			assert.Equal(t, tt.expected, result)
		})
	}
}
