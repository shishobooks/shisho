package testgen

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// GenerateEPUB creates a valid EPUB file at the specified path with the given options.
// The generated EPUB contains mimetype, container.xml, content.opf with metadata,
// chapter1.xhtml, and optionally a cover image.
func GenerateEPUB(t *testing.T, dir, filename string, opts EPUBOptions) string {
	t.Helper()

	path := filepath.Join(dir, filename)

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create EPUB file: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	// 1. mimetype - must be first and uncompressed
	mimetypeHeader := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store, // No compression
	}
	w, err := zw.CreateHeader(mimetypeHeader)
	if err != nil {
		t.Fatalf("failed to create mimetype entry: %v", err)
	}
	if _, err := w.Write([]byte("application/epub+zip")); err != nil {
		t.Fatalf("failed to write mimetype: %v", err)
	}

	// 2. META-INF/container.xml
	containerXML := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
	if err := writeZipFile(zw, "META-INF/container.xml", []byte(containerXML)); err != nil {
		t.Fatalf("failed to write container.xml: %v", err)
	}

	// 3. Generate cover image if requested
	coverMimeType := opts.CoverMimeType
	if coverMimeType == "" {
		coverMimeType = "image/png"
	}
	var coverFilename string
	var coverData []byte
	if opts.HasCover {
		coverData = generateImage(t, coverMimeType)
		if coverMimeType == "image/jpeg" {
			coverFilename = "cover.jpg"
		} else {
			coverFilename = "cover.png"
		}
		if err := writeZipFile(zw, "OEBPS/"+coverFilename, coverData); err != nil {
			t.Fatalf("failed to write cover image: %v", err)
		}
	}

	// 4. OEBPS/content.opf
	opfContent := generateOPF(opts, coverFilename, coverMimeType)
	if err := writeZipFile(zw, "OEBPS/content.opf", []byte(opfContent)); err != nil {
		t.Fatalf("failed to write content.opf: %v", err)
	}

	// 5. OEBPS/chapter1.xhtml
	chapterContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
  <title>Chapter 1</title>
</head>
<body>
  <h1>Chapter 1</h1>
  <p>This is a test chapter.</p>
</body>
</html>`
	if err := writeZipFile(zw, "OEBPS/chapter1.xhtml", []byte(chapterContent)); err != nil {
		t.Fatalf("failed to write chapter1.xhtml: %v", err)
	}

	return path
}

func generateOPF(opts EPUBOptions, coverFilename, coverMimeType string) string {
	var buf bytes.Buffer

	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
`)

	// Title - only include if provided (allows testing filepath fallback)
	if opts.Title != "" {
		buf.WriteString(fmt.Sprintf("    <dc:title id=\"title\">%s</dc:title>\n", escapeXML(opts.Title)))
	}

	// Authors
	for i, author := range opts.Authors {
		buf.WriteString(fmt.Sprintf("    <dc:creator id=\"creator%d\" opf:role=\"aut\">%s</dc:creator>\n", i, escapeXML(author)))
	}

	// Identifier
	buf.WriteString("    <dc:identifier id=\"bookid\">urn:uuid:test-book-id</dc:identifier>\n")
	buf.WriteString("    <dc:language>en</dc:language>\n")

	// Series (calibre format)
	if opts.Series != "" {
		buf.WriteString(fmt.Sprintf("    <meta name=\"calibre:series\" content=\"%s\"/>\n", escapeXML(opts.Series)))
		if opts.SeriesNumber != nil {
			buf.WriteString(fmt.Sprintf("    <meta name=\"calibre:series_index\" content=\"%.1f\"/>\n", *opts.SeriesNumber))
		}
	}

	// Cover reference
	if coverFilename != "" {
		buf.WriteString("    <meta name=\"cover\" content=\"cover-image\"/>\n")
	}

	buf.WriteString("  </metadata>\n")

	// Manifest
	buf.WriteString("  <manifest>\n")
	buf.WriteString("    <item id=\"chapter1\" href=\"chapter1.xhtml\" media-type=\"application/xhtml+xml\"/>\n")
	if coverFilename != "" {
		buf.WriteString(fmt.Sprintf("    <item id=\"cover-image\" href=\"%s\" media-type=\"%s\"/>\n", coverFilename, coverMimeType))
	}
	buf.WriteString("  </manifest>\n")

	// Spine
	buf.WriteString("  <spine>\n")
	buf.WriteString("    <itemref idref=\"chapter1\"/>\n")
	buf.WriteString("  </spine>\n")

	buf.WriteString("</package>")

	return buf.String()
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func generateImage(t *testing.T, mimeType string) []byte {
	t.Helper()

	// Create a simple 100x100 solid color image
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	blue := color.RGBA{0, 100, 200, 255}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, blue)
		}
	}

	var buf bytes.Buffer
	switch mimeType {
	case "image/jpeg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			t.Fatalf("failed to encode JPEG: %v", err)
		}
	default: // image/png
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("failed to encode PNG: %v", err)
		}
	}

	return buf.Bytes()
}

func escapeXML(s string) string {
	var buf bytes.Buffer
	for _, r := range s {
		switch r {
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '&':
			buf.WriteString("&amp;")
		case '"':
			buf.WriteString("&quot;")
		case '\'':
			buf.WriteString("&apos;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
