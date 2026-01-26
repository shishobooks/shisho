package kepub

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/image/draw"
)

// CBZMetadata holds metadata for CBZ to KePub conversion.
type CBZMetadata struct {
	Title       string
	Name        *string // Name takes precedence over Title when non-empty
	Subtitle    *string
	Description *string
	Authors     []CBZAuthor
	Series      []CBZSeries
	Genres      []string
	Tags        []string
	URL         *string
	Publisher   *string
	Imprint     *string
	ReleaseDate *time.Time
	Chapters    []CBZChapter
	CoverPage   *int // 0-indexed page number for cover (nil = first page)
}

// CBZAuthor represents an author/creator for CBZ metadata.
type CBZAuthor struct {
	Name     string
	SortName string // e.g., "Doe, Jane" for sorting/file-as
	Role     string // writer, penciller, inker, etc.
}

// CBZSeries represents series information for CBZ metadata.
type CBZSeries struct {
	Name   string
	Number *float64
}

// CBZChapter represents a chapter/section for CBZ metadata.
type CBZChapter struct {
	Title     string
	StartPage int // 0-indexed page number
}

// pageInfo holds information about a page image.
type pageInfo struct {
	filename  string
	width     int
	height    int
	mediaType string
	data      []byte
}

// ConvertCBZ converts a CBZ file to a fixed-layout KePub EPUB.
// JPEG/PNG images taller than 2000px are resized to 2000px height.
// PNG images are converted to JPEG for smaller file size.
func (c *Converter) ConvertCBZ(ctx context.Context, srcPath, destPath string) error {
	return c.ConvertCBZWithMetadata(ctx, srcPath, destPath, nil)
}

// ConvertCBZWithMetadata converts a CBZ file to a fixed-layout KePub EPUB with metadata.
// JPEG/PNG images taller than 2000px are resized to 2000px height.
// PNG images are converted to JPEG for smaller file size.
func (c *Converter) ConvertCBZWithMetadata(ctx context.Context, srcPath, destPath string, metadata *CBZMetadata) error {
	// Open source CBZ
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return errors.Wrap(err, "failed to open source file")
	}
	defer srcFile.Close()

	srcStat, err := srcFile.Stat()
	if err != nil {
		return errors.Wrap(err, "failed to stat source file")
	}

	srcZip, err := zip.NewReader(srcFile, srcStat.Size())
	if err != nil {
		return errors.Wrap(err, "failed to read source CBZ as zip")
	}

	// Collect image files
	var imageFiles []*zip.File
	for _, f := range srcZip.File {
		if IsImageFile(f.Name) && !strings.HasPrefix(filepath.Base(f.Name), ".") {
			imageFiles = append(imageFiles, f)
		}
	}

	// Sort by filename for proper reading order
	sort.Slice(imageFiles, func(i, j int) bool {
		return naturalLess(imageFiles[i].Name, imageFiles[j].Name)
	})

	if len(imageFiles) == 0 {
		return errors.New("no images found in CBZ file")
	}

	// Create temporary output file
	tmpPath := destPath + ".tmp"
	destFile, err := os.Create(tmpPath)
	if err != nil {
		return errors.Wrap(err, "failed to create destination file")
	}
	defer func() {
		destFile.Close()
		os.Remove(tmpPath)
	}()

	destZip := zip.NewWriter(destFile)

	// Write mimetype first (uncompressed, as per EPUB spec)
	mimeWriter, err := destZip.CreateHeader(&zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store, // Must be uncompressed
	})
	if err != nil {
		return errors.Wrap(err, "failed to create mimetype")
	}
	if _, err := mimeWriter.Write([]byte("application/epub+zip")); err != nil {
		return errors.Wrap(err, "failed to write mimetype")
	}

	// Create META-INF/container.xml
	containerXML := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
	if err := writeZipFile(destZip, "META-INF/container.xml", []byte(containerXML)); err != nil {
		return errors.Wrap(err, "failed to write container.xml")
	}

	// Process images in parallel for better performance
	pages := make([]pageInfo, len(imageFiles))
	processErrors := make([]error, len(imageFiles))

	// Use a worker pool with NumCPU workers
	numWorkers := runtime.NumCPU()
	if numWorkers > len(imageFiles) {
		numWorkers = len(imageFiles)
	}

	var wg sync.WaitGroup
	jobs := make(chan int, len(imageFiles))

	// Start workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				// Check for context cancellation
				select {
				case <-ctx.Done():
					processErrors[i] = ctx.Err()
					continue
				default:
				}

				imgFile := imageFiles[i]
				data, err := readZipFile(imgFile)
				if err != nil {
					processErrors[i] = errors.Wrapf(err, "failed to read image: %s", imgFile.Name)
					continue
				}

				// Process image: resize if too tall, convert PNG to JPEG
				ext := strings.ToLower(filepath.Ext(imgFile.Name))
				processed := ProcessImageForEreader(data, ext)

				pages[i] = pageInfo{
					filename:  fmt.Sprintf("page%04d%s", i+1, processed.Ext),
					width:     processed.Width,
					height:    processed.Height,
					mediaType: processed.MediaType,
					data:      processed.Data,
				}
			}
		}()
	}

	// Send jobs
	for i := range imageFiles {
		jobs <- i
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()

	// Check for errors
	for i, err := range processErrors {
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return errors.Wrap(err, "conversion cancelled")
			}
			return errors.Wrapf(err, "failed to process image %d", i+1)
		}
	}

	// Generate OPF content
	opf := generateFixedLayoutOPF(pages, metadata)
	if err := writeZipFile(destZip, "OEBPS/content.opf", opf); err != nil {
		return errors.Wrap(err, "failed to write content.opf")
	}

	// Generate NCX (table of contents)
	ncx := generateNCX(pages, metadata)
	if err := writeZipFile(destZip, "OEBPS/toc.ncx", ncx); err != nil {
		return errors.Wrap(err, "failed to write toc.ncx")
	}

	// Generate one XHTML page per image (required for fixed-layout)
	for i, page := range pages {
		xhtml := generateImagePage(page, i)
		pageName := fmt.Sprintf("OEBPS/page%04d.xhtml", i+1)
		if err := writeZipFile(destZip, pageName, xhtml); err != nil {
			return errors.Wrapf(err, "failed to write %s", pageName)
		}

		// Write processed image file
		imgPath := "OEBPS/images/" + page.filename
		if err := writeZipFile(destZip, imgPath, page.data); err != nil {
			return errors.Wrapf(err, "failed to write %s", imgPath)
		}
	}

	// Generate CSS for fixed layout
	css := generateFixedLayoutCSS()
	if err := writeZipFile(destZip, "OEBPS/styles.css", css); err != nil {
		return errors.Wrap(err, "failed to write styles.css")
	}

	// Generate nav.xhtml (required for EPUB3 and helps Kobo navigation)
	nav := generateNavXHTML(pages, metadata)
	if err := writeZipFile(destZip, "OEBPS/nav.xhtml", nav); err != nil {
		return errors.Wrap(err, "failed to write nav.xhtml")
	}

	// Close the zip writer
	if err := destZip.Close(); err != nil {
		return errors.Wrap(err, "failed to finalize destination")
	}

	// Close the file before renaming
	if err := destFile.Close(); err != nil {
		return errors.Wrap(err, "failed to close destination file")
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		return errors.Wrap(err, "failed to finalize destination file")
	}

	return nil
}

// IsImageFile returns true if the file extension indicates an image.
func IsImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	}
	return false
}

// imageMediaType returns the MIME type for an image extension.
func imageMediaType(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// writeZipFile writes a file to the zip archive.
func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// getEffectiveTitle returns the title to use for the EPUB.
// It prefers Name over Title when Name is non-nil and non-empty.
func getEffectiveTitle(metadata *CBZMetadata) string {
	if metadata == nil {
		return "Comic Book"
	}
	if metadata.Name != nil && *metadata.Name != "" {
		return *metadata.Name
	}
	if metadata.Title != "" {
		return metadata.Title
	}
	return "Comic Book"
}

// generateFixedLayoutOPF generates the OPF file for a fixed-layout EPUB.
func generateFixedLayoutOPF(pages []pageInfo, metadata *CBZMetadata) []byte {
	var buf bytes.Buffer

	// Determine title
	title := getEffectiveTitle(metadata)

	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:`)
	buf.WriteString(generateUUID())
	buf.WriteString(`</dc:identifier>
    <dc:title>`)
	buf.WriteString(html.EscapeString(title))
	buf.WriteString(`</dc:title>
    <dc:language>en</dc:language>
`)

	// Add authors/creators with roles and file-as metadata
	if metadata != nil && len(metadata.Authors) > 0 {
		// Track written creators to avoid duplicates
		writtenCreators := make(map[string]bool)
		authorIndex := 0
		for _, author := range metadata.Authors {
			// Determine role tag (writer and penciller are most common for comics)
			var role string
			switch strings.ToLower(author.Role) {
			case "", "writer":
				role = "aut"
			case "penciller", "artist", "inker":
				role = "art"
			case "colorist":
				role = "clr"
			case "letterer":
				role = "ill"
			case "cover artist", "cover":
				role = "cov"
			case "editor":
				role = "edt"
			default:
				role = "aut" // Fallback to author for unknown roles
			}

			// Create unique key for deduplication (name + role)
			key := author.Name + "|" + role
			if writtenCreators[key] {
				continue
			}
			writtenCreators[key] = true

			authorID := fmt.Sprintf("author%d", authorIndex)
			authorIndex++

			buf.WriteString(fmt.Sprintf(`    <dc:creator id="%s">`, authorID))
			buf.WriteString(html.EscapeString(author.Name))
			buf.WriteString(`</dc:creator>
`)
			// Add file-as (sort name) if available
			if author.SortName != "" {
				buf.WriteString(fmt.Sprintf(`    <meta refines="#%s" property="file-as">`, authorID))
				buf.WriteString(html.EscapeString(author.SortName))
				buf.WriteString(`</meta>
`)
			}
		}
	}

	// Add series metadata (use first series only)
	if metadata != nil && len(metadata.Series) > 0 {
		series := metadata.Series[0]
		buf.WriteString(`    <meta property="belongs-to-collection" id="series-1">`)
		buf.WriteString(html.EscapeString(series.Name))
		buf.WriteString(`</meta>
    <meta refines="#series-1" property="collection-type">series</meta>
`)
		if series.Number != nil {
			buf.WriteString(fmt.Sprintf(`    <meta refines="#series-1" property="group-position">%g</meta>
`, *series.Number))
		}
	}

	// Add genres as dc:subject elements
	if metadata != nil {
		for _, genre := range metadata.Genres {
			buf.WriteString(`    <dc:subject>`)
			buf.WriteString(html.EscapeString(genre))
			buf.WriteString(`</dc:subject>
`)
		}
	}

	// Add tags as calibre:tags meta (comma-separated)
	if metadata != nil && len(metadata.Tags) > 0 {
		buf.WriteString(`    <meta name="calibre:tags" content="`)
		buf.WriteString(html.EscapeString(strings.Join(metadata.Tags, ", ")))
		buf.WriteString(`"/>
`)
	}

	// Add description
	if metadata != nil && metadata.Description != nil && *metadata.Description != "" {
		buf.WriteString(`    <dc:description>`)
		buf.WriteString(html.EscapeString(*metadata.Description))
		buf.WriteString(`</dc:description>
`)
	}

	// Add publisher
	if metadata != nil && metadata.Publisher != nil && *metadata.Publisher != "" {
		buf.WriteString(`    <dc:publisher>`)
		buf.WriteString(html.EscapeString(*metadata.Publisher))
		buf.WriteString(`</dc:publisher>
`)
	}

	// Add release date
	if metadata != nil && metadata.ReleaseDate != nil {
		buf.WriteString(`    <dc:date>`)
		buf.WriteString(metadata.ReleaseDate.Format("2006-01-02"))
		buf.WriteString(`</dc:date>
`)
	}

	// Add URL as custom meta (not standard EPUB but preserved for round-trip)
	if metadata != nil && metadata.URL != nil && *metadata.URL != "" {
		buf.WriteString(`    <meta name="shisho:url" content="`)
		buf.WriteString(html.EscapeString(*metadata.URL))
		buf.WriteString(`"/>
`)
	}

	// Add imprint as custom meta (not standard EPUB but preserved for round-trip)
	if metadata != nil && metadata.Imprint != nil && *metadata.Imprint != "" {
		buf.WriteString(`    <meta name="shisho:imprint" content="`)
		buf.WriteString(html.EscapeString(*metadata.Imprint))
		buf.WriteString(`"/>
`)
	}

	// Calculate cover page index (default 0 if not set)
	coverPageIdx := 0
	if metadata != nil && metadata.CoverPage != nil {
		coverPageIdx = *metadata.CoverPage
		// Clamp to valid range
		if coverPageIdx < 0 || coverPageIdx >= len(pages) {
			coverPageIdx = 0
		}
	}

	buf.WriteString(`    <meta property="dcterms:modified">`)
	buf.WriteString(time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	buf.WriteString(`</meta>
    <meta property="rendition:layout">pre-paginated</meta>
    <meta property="rendition:spread">landscape</meta>
`)
	buf.WriteString(fmt.Sprintf(`    <meta name="cover" content="img%04d"/>
`, coverPageIdx+1))
	buf.WriteString(`  </metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="nav" href="nav.xhtml" properties="nav" media-type="application/xhtml+xml"/>
    <item id="css" href="styles.css" media-type="text/css"/>
`)

	// Add page XHTML items and image items
	for i, page := range pages {
		pageID := fmt.Sprintf("page%04d", i+1)
		imgID := fmt.Sprintf("img%04d", i+1)

		// Page XHTML item
		buf.WriteString(fmt.Sprintf(`    <item id="%s" href="page%04d.xhtml" media-type="application/xhtml+xml"/>
`, pageID, i+1))

		// Image item
		buf.WriteString(fmt.Sprintf(`    <item id="%s" href="images/%s" media-type="%s"`, imgID, page.filename, page.mediaType))
		if i == coverPageIdx {
			buf.WriteString(` properties="cover-image"`)
		}
		buf.WriteString(`/>
`)
	}

	buf.WriteString(`  </manifest>
  <spine page-progression-direction="ltr" toc="ncx">
`)

	// Add spine items with alternating page-spread properties (like KCC)
	for i := range pages {
		pageID := fmt.Sprintf("page%04d", i+1)
		spread := "left"
		if i%2 == 1 {
			spread = "right"
		}
		buf.WriteString(fmt.Sprintf(`    <itemref idref="%s" properties="rendition:page-spread-%s"/>
`, pageID, spread))
	}

	buf.WriteString(`  </spine>
</package>
`)

	return buf.Bytes()
}

// generateNCX generates the NCX navigation file.
// If chapters are provided in metadata, uses chapter-based navigation.
// Otherwise, falls back to page-based navigation.
func generateNCX(pages []pageInfo, metadata *CBZMetadata) []byte {
	var buf bytes.Buffer

	// Determine title
	title := getEffectiveTitle(metadata)

	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <head>
    <meta name="dtb:uid" content="urn:uuid:`)
	buf.WriteString(generateUUID())
	buf.WriteString(`"/>
    <meta name="dtb:depth" content="1"/>
    <meta name="dtb:totalPageCount" content="`)
	buf.WriteString(strconv.Itoa(len(pages)))
	buf.WriteString(`"/>
    <meta name="dtb:maxPageNumber" content="`)
	buf.WriteString(strconv.Itoa(len(pages)))
	buf.WriteString(`"/>
  </head>
  <docTitle>
    <text>`)
	buf.WriteString(html.EscapeString(title))
	buf.WriteString(`</text>
  </docTitle>
  <navMap>
`)

	// Use chapter-based navPoints if chapters are provided
	if metadata != nil && len(metadata.Chapters) > 0 {
		playOrder := 1
		for _, ch := range metadata.Chapters {
			// Skip chapters beyond page count
			if ch.StartPage >= len(pages) {
				continue
			}
			pageNum := ch.StartPage + 1 // Convert 0-indexed to 1-indexed
			buf.WriteString(fmt.Sprintf(`    <navPoint id="navpoint%d" playOrder="%d">
      <navLabel>
        <text>%s</text>
      </navLabel>
      <content src="page%04d.xhtml"/>
    </navPoint>
`, playOrder, playOrder, html.EscapeString(ch.Title), pageNum))
			playOrder++
		}
	} else {
		// Fall back to page-based nav points
		for i := range pages {
			pageNum := i + 1
			buf.WriteString(fmt.Sprintf(`    <navPoint id="navpoint%d" playOrder="%d">
      <navLabel>
        <text>Page %d</text>
      </navLabel>
      <content src="page%04d.xhtml"/>
    </navPoint>
`, pageNum, pageNum, pageNum, pageNum))
		}
	}

	buf.WriteString(`  </navMap>
</ncx>
`)

	return buf.Bytes()
}

// generateImagePage generates a fixed-layout XHTML page with one image.
// Uses the same structure as KCC (Kindle Comic Converter) for optimal Kobo compatibility.
func generateImagePage(page pageInfo, pageIndex int) []byte {
	var buf bytes.Buffer

	pageNum := pageIndex + 1
	w := strconv.Itoa(page.width)
	h := strconv.Itoa(page.height)

	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head>
<title>Page `)
	buf.WriteString(strconv.Itoa(pageNum))
	buf.WriteString(`</title>
<link href="styles.css" type="text/css" rel="stylesheet"/>
<meta name="viewport" content="width=`)
	buf.WriteString(w)
	buf.WriteString(`, height=`)
	buf.WriteString(h)
	buf.WriteString(`"/>
</head>
<body style="">
<div style="text-align:center;top:0%;">
<img width="`)
	buf.WriteString(w)
	buf.WriteString(`" height="`)
	buf.WriteString(h)
	buf.WriteString(`" src="images/`)
	buf.WriteString(page.filename)
	buf.WriteString(`"/>
</div>
</body>
</html>
`)

	return buf.Bytes()
}

// generateFixedLayoutCSS generates minimal CSS for comic pages.
// Uses the same CSS as KCC for optimal Kobo compatibility.
func generateFixedLayoutCSS() []byte {
	return []byte(`@page { margin: 0; }
body { display: block; margin: 0; padding: 0; }
`)
}

// generateNavXHTML generates the EPUB3 navigation document.
// This is required for EPUB3 and helps with Kobo navigation.
// If chapters are provided in metadata, creates chapter-based TOC.
// Otherwise, uses a single title entry (current behavior).
func generateNavXHTML(pages []pageInfo, metadata *CBZMetadata) []byte {
	var buf bytes.Buffer

	// Determine title
	title := getEffectiveTitle(metadata)

	buf.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head>
<title>`)
	buf.WriteString(html.EscapeString(title))
	buf.WriteString(`</title>
<meta charset="utf-8"/>
</head>
<body>
<nav xmlns:epub="http://www.idpf.org/2007/ops" epub:type="toc" id="toc">
<ol>
`)

	// Use chapter-based TOC if chapters are provided
	if metadata != nil && len(metadata.Chapters) > 0 {
		for _, ch := range metadata.Chapters {
			// Skip chapters beyond page count
			if ch.StartPage >= len(pages) {
				continue
			}
			pageNum := ch.StartPage + 1 // Convert 0-indexed to 1-indexed
			buf.WriteString(fmt.Sprintf(`<li><a href="page%04d.xhtml">%s</a></li>
`, pageNum, html.EscapeString(ch.Title)))
		}
	} else {
		// Fall back to single title entry
		buf.WriteString(`<li><a href="page0001.xhtml">`)
		buf.WriteString(html.EscapeString(title))
		buf.WriteString(`</a></li>
`)
	}

	buf.WriteString(`</ol>
</nav>
<nav epub:type="page-list">
<ol>
<li><a href="page0001.xhtml">`)
	buf.WriteString(html.EscapeString(title))
	buf.WriteString(`</a></li>
</ol>
</nav>
</body>
</html>
`)

	return buf.Bytes()
}

// generateUUID generates a simple UUID-like string.
func generateUUID() string {
	// Use timestamp-based UUID for simplicity
	now := time.Now().UnixNano()
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		now&0xFFFFFFFF,
		(now>>32)&0xFFFF,
		(now>>48)&0x0FFF|0x4000,
		(now>>60)&0x3FFF|0x8000,
		now&0xFFFFFFFFFFFF)
}

// naturalLess compares strings naturally (so "page2" < "page10").
func naturalLess(a, b string) bool {
	// Simple natural sort: extract numbers and compare
	return extractNumber(a) < extractNumber(b)
}

// extractNumber extracts the first number found in a string.
func extractNumber(s string) int {
	var numStr string
	for _, c := range s {
		if c >= '0' && c <= '9' {
			numStr += string(c)
		} else if numStr != "" {
			break
		}
	}
	if numStr == "" {
		return 0
	}
	n, _ := strconv.Atoi(numStr)
	return n
}

// Kobo Libra Color screen dimensions (from KCC profiles).
// All images are resized to fit within these dimensions while preserving aspect ratio.
const (
	koboWidth  = 1264
	koboHeight = 1680
)

// Palette16 is the 16-level grayscale palette used by Kobo e-ink displays.
// This matches KCC (Kindle Comic Converter) for optimal e-ink rendering.
var Palette16 = []uint8{
	0x00, 0x11, 0x22, 0x33,
	0x44, 0x55, 0x66, 0x77,
	0x88, 0x99, 0xaa, 0xbb,
	0xcc, 0xdd, 0xee, 0xff,
}

// ProcessedImage holds the result of image processing.
type ProcessedImage struct {
	Data      []byte
	Width     int
	Height    int
	Ext       string
	MediaType string
}

// ProcessImageForEreader resizes images to fit e-reader screen dimensions.
// - All JPEG/PNG images are resized to fit within Kobo Libra Color resolution (1264x1680)
// - Grayscale images (like manga) are converted to grayscale JPEG for faster rendering
// - PNG images are converted to JPEG for smaller file size
// - Other formats (GIF, WebP) are passed through unchanged.
func ProcessImageForEreader(data []byte, origExt string) *ProcessedImage {
	ext := strings.ToLower(origExt)

	// Only process JPEG and PNG
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		// Pass through GIF, WebP, etc. unchanged
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			cfg = image.Config{Width: koboWidth, Height: koboHeight}
		}
		return &ProcessedImage{
			Data:      data,
			Width:     cfg.Width,
			Height:    cfg.Height,
			Ext:       ext,
			MediaType: imageMediaType(ext),
		}
	}

	// Decode the full image
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// If we can't decode, pass through unchanged
		cfg, _, cfgErr := image.DecodeConfig(bytes.NewReader(data))
		if cfgErr != nil {
			cfg = image.Config{Width: koboWidth, Height: koboHeight}
		}
		return &ProcessedImage{
			Data:      data,
			Width:     cfg.Width,
			Height:    cfg.Height,
			Ext:       ext,
			MediaType: imageMediaType(ext),
		}
	}

	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	// Calculate scale to fit within Kobo screen while preserving aspect ratio
	widthRatio := float64(koboWidth) / float64(origWidth)
	heightRatio := float64(koboHeight) / float64(origHeight)
	ratio := widthRatio
	if heightRatio < widthRatio {
		ratio = heightRatio
	}

	// Check if any processing is needed
	needsResize := ratio < 1.0 // Only resize if image is larger than screen
	needsConvert := format == "png"

	// Check if image is grayscale (like manga pages)
	isGray := isGrayscaleImage(img)

	if !needsResize && !needsConvert && !isGray {
		// No processing needed, return original
		return &ProcessedImage{
			Data:      data,
			Width:     origWidth,
			Height:    origHeight,
			Ext:       ext,
			MediaType: imageMediaType(ext),
		}
	}

	// Calculate new dimensions
	newWidth := origWidth
	newHeight := origHeight
	if needsResize {
		newWidth = int(float64(origWidth) * ratio)
		newHeight = int(float64(origHeight) * ratio)
	}

	// Create the output image
	var outputImg image.Image
	if needsResize {
		if isGray {
			// Resize directly to grayscale for better performance
			resized := image.NewGray(image.Rect(0, 0, newWidth, newHeight))
			draw.BiLinear.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)
			// Apply 16-level palette quantization for optimal e-ink rendering
			quantizeToKoboPalette(resized)
			outputImg = resized
		} else {
			// Resize to RGBA
			resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
			draw.BiLinear.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)
			outputImg = resized
		}
	} else if isGray {
		// Convert to grayscale without resizing
		outputImg = convertToGrayscale(img)
	} else {
		outputImg = img
	}

	// Encode as JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, outputImg, &jpeg.Options{Quality: 85}); err != nil {
		// Fall back to original on encoding error
		return &ProcessedImage{
			Data:      data,
			Width:     origWidth,
			Height:    origHeight,
			Ext:       ext,
			MediaType: imageMediaType(ext),
		}
	}

	return &ProcessedImage{
		Data:      buf.Bytes(),
		Width:     newWidth,
		Height:    newHeight,
		Ext:       ".jpg",
		MediaType: "image/jpeg",
	}
}

// isGrayscaleImage detects if an image is essentially grayscale.
// It samples pixels and checks if R≈G≈B within a tolerance.
// This is useful for manga/comic pages which are often black and white.
func isGrayscaleImage(img image.Image) bool {
	// If it's already a grayscale image type, return true
	switch img.(type) {
	case *image.Gray, *image.Gray16:
		return true
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Sample pixels in a grid pattern (every 10th pixel)
	// to avoid checking every single pixel
	const sampleStep = 10
	const tolerance = 10 // Allow small differences due to compression artifacts

	colorPixels := 0
	sampledPixels := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y += sampleStep {
		for x := bounds.Min.X; x < bounds.Max.X; x += sampleStep {
			r, g, b, _ := img.At(x, y).RGBA()
			// Convert from 16-bit to 8-bit
			r8 := r >> 8
			g8 := g >> 8
			b8 := b >> 8

			// Check if pixel is grayscale (R≈G≈B)
			maxDiff := max3(abs(int(r8)-int(g8)), abs(int(g8)-int(b8)), abs(int(r8)-int(b8)))
			if maxDiff > tolerance {
				colorPixels++
			}
			sampledPixels++
		}
	}

	// If less than 2% of sampled pixels are colored, consider it grayscale
	// This allows for small color elements like chapter markers
	if sampledPixels == 0 {
		return false
	}
	colorRatio := float64(colorPixels) / float64(sampledPixels)

	// Also check total pixel count - very small images might have false positives
	totalPixels := width * height
	if totalPixels < 1000 {
		return false // Don't convert tiny images
	}

	return colorRatio < 0.02
}

// convertToGrayscale converts an image to grayscale with 16-level palette quantization.
// This matches KCC behavior for optimal e-ink display on Kobo devices.
func convertToGrayscale(img image.Image) *image.Gray {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	draw.Draw(gray, bounds, img, bounds.Min, draw.Src)

	// Apply 16-level palette quantization for optimal e-ink rendering
	quantizeToKoboPalette(gray)

	return gray
}

// quantizeToKoboPalette reduces a grayscale image to 16 levels.
// This matches the e-ink display capability of Kobo devices and improves rendering speed.
func quantizeToKoboPalette(gray *image.Gray) {
	bounds := gray.Bounds()
	pix := gray.Pix
	stride := gray.Stride

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		rowOffset := (y - bounds.Min.Y) * stride
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// Get the pixel value directly from the slice for performance
			offset := rowOffset + (x - bounds.Min.X)
			c := pix[offset]

			// Quantize to nearest of 16 levels (0x00, 0x11, 0x22, ..., 0xff)
			// Each level spans ~17 values (256/15 ≈ 17)
			level := (uint16(c) + 8) / 17 // +8 for rounding to nearest
			if level > 15 {
				level = 15
			}

			pix[offset] = Palette16[level]
		}
	}
}

// abs returns the absolute value of an int.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// max3 returns the maximum of three ints.
func max3(a, b, c int) int {
	if a >= b && a >= c {
		return a
	}
	if b >= c {
		return b
	}
	return c
}

// Ensure png package is used for decoding (image.Decode uses registered decoders).
var _ = png.Decode
