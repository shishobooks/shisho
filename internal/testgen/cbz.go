package testgen

import (
	"archive/zip"
	"bytes"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

// GenerateCBZ creates a valid CBZ file at the specified path with the given options.
// The generated CBZ contains:
// - ComicInfo.xml (if HasComicInfo is true)
// - Page images (001.png, 002.png, etc.)
func GenerateCBZ(t *testing.T, dir, filename string, opts CBZOptions) string {
	t.Helper()

	path := filepath.Join(dir, filename)

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create CBZ file: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	// Set defaults
	pageCount := opts.PageCount
	if pageCount <= 0 {
		pageCount = 3
	}
	imageFormat := opts.ImageFormat
	if imageFormat == "" {
		imageFormat = "png"
	}

	// 1. Add ComicInfo.xml if requested
	if opts.HasComicInfo {
		comicInfo := generateComicInfo(opts, pageCount)
		if err := writeZipFile(zw, "ComicInfo.xml", []byte(comicInfo)); err != nil {
			t.Fatalf("failed to write ComicInfo.xml: %v", err)
		}
	}

	// 2. Generate page images
	mimeType := "image/png"
	ext := "png"
	if imageFormat == "jpeg" || imageFormat == "jpg" {
		mimeType = "image/jpeg"
		ext = "jpg"
	}

	for i := 0; i < pageCount; i++ {
		imgData := generateImage(t, mimeType)
		imgName := fmt.Sprintf("%03d.%s", i, ext)
		if err := writeZipFile(zw, imgName, imgData); err != nil {
			t.Fatalf("failed to write page %s: %v", imgName, err)
		}
	}

	return path
}

// GenerateCorruptCBZ creates a CBZ of pageCount stored PNG pages (000.png,
// 001.png, etc.) where page bad fails its checksum when read. Opening the
// archive and listing its pages succeeds; only reading that page fails.
func GenerateCorruptCBZ(t *testing.T, dir, filename string, pageCount, bad int) string {
	t.Helper()

	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create CBZ file: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	img := generateImage(t, "image/png")
	for i := 0; i < pageCount; i++ {
		crc := crc32.ChecksumIEEE(img)
		if i == bad {
			crc++
		}
		w, err := zw.CreateRaw(&zip.FileHeader{
			Name:               fmt.Sprintf("%03d.png", i),
			Method:             zip.Store,
			CRC32:              crc,
			CompressedSize64:   uint64(len(img)),
			UncompressedSize64: uint64(len(img)),
		})
		if err != nil {
			t.Fatalf("failed to create page %d: %v", i, err)
		}
		if _, err := w.Write(img); err != nil {
			t.Fatalf("failed to write page %d: %v", i, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to finalize CBZ: %v", err)
	}

	return path
}

func generateComicInfo(opts CBZOptions, pageCount int) string {
	var buf bytes.Buffer

	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<ComicInfo>
`)

	if opts.Title != "" {
		buf.WriteString(fmt.Sprintf("  <Title>%s</Title>\n", escapeXML(opts.Title)))
	} else if opts.ForceEmptyTitle {
		buf.WriteString("  <Title></Title>\n")
	}
	if opts.Series != "" {
		buf.WriteString(fmt.Sprintf("  <Series>%s</Series>\n", escapeXML(opts.Series)))
	}
	if opts.SeriesNumber != nil {
		// Format as integer if it's a whole number
		if *opts.SeriesNumber == float64(int(*opts.SeriesNumber)) {
			buf.WriteString(fmt.Sprintf("  <Number>%d</Number>\n", int(*opts.SeriesNumber)))
		} else {
			buf.WriteString(fmt.Sprintf("  <Number>%.1f</Number>\n", *opts.SeriesNumber))
		}
	}
	if opts.Writer != "" {
		buf.WriteString(fmt.Sprintf("  <Writer>%s</Writer>\n", escapeXML(opts.Writer)))
	}
	if opts.Penciller != "" {
		buf.WriteString(fmt.Sprintf("  <Penciller>%s</Penciller>\n", escapeXML(opts.Penciller)))
	}
	if opts.Inker != "" {
		buf.WriteString(fmt.Sprintf("  <Inker>%s</Inker>\n", escapeXML(opts.Inker)))
	}
	if opts.Colorist != "" {
		buf.WriteString(fmt.Sprintf("  <Colorist>%s</Colorist>\n", escapeXML(opts.Colorist)))
	}
	if opts.Letterer != "" {
		buf.WriteString(fmt.Sprintf("  <Letterer>%s</Letterer>\n", escapeXML(opts.Letterer)))
	}
	if opts.CoverArtist != "" {
		buf.WriteString(fmt.Sprintf("  <CoverArtist>%s</CoverArtist>\n", escapeXML(opts.CoverArtist)))
	}
	if opts.Editor != "" {
		buf.WriteString(fmt.Sprintf("  <Editor>%s</Editor>\n", escapeXML(opts.Editor)))
	}
	if opts.Translator != "" {
		buf.WriteString(fmt.Sprintf("  <Translator>%s</Translator>\n", escapeXML(opts.Translator)))
	}

	buf.WriteString(fmt.Sprintf("  <PageCount>%d</PageCount>\n", pageCount))

	// Add page info if cover page type is specified
	if opts.CoverPageType != "" {
		buf.WriteString("  <Pages>\n")
		for i := 0; i < pageCount; i++ {
			pageType := ""
			if i == opts.CoverPageIndex && opts.CoverPageType != "" {
				pageType = fmt.Sprintf(" Type=\"%s\"", opts.CoverPageType)
			}
			buf.WriteString(fmt.Sprintf("    <Page Image=\"%d\"%s/>\n", i, pageType))
		}
		buf.WriteString("  </Pages>\n")
	}

	buf.WriteString("</ComicInfo>")

	return buf.String()
}
