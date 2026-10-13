package kepub

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"html"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/cbz"
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
	ReleaseDate *time.Time
	Chapters    []CBZChapter
	CoverPage   *int    // 0-indexed page number for cover (nil = first page)
	Language    *string // BCP 47 language tag
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

// pageInfo describes a written page: its image's filename in the images
// directory, dimensions, and media type.
type pageInfo struct {
	filename  string
	width     int
	height    int
	mediaType string
}

// ConvertCBZ converts a CBZ file to a fixed-layout KePub EPUB.
// JPEG/PNG images taller than 2000px are resized to 2000px height.
// PNG images are converted to JPEG for smaller file size.
func (c *Converter) ConvertCBZ(ctx context.Context, srcPath, destPath string) error {
	return c.ConvertCBZWithMetadata(ctx, srcPath, destPath, nil)
}

// ConvertCBZWithMetadata converts a CBZ file to a fixed-layout KePub EPUB with metadata.
// It is a path-based wrapper over PrepareCBZ and CBZConversion.Write; downloads
// call those directly through pkg/filegen.
// JPEG/PNG images taller than 2000px are resized to 2000px height.
// PNG images are converted to JPEG for smaller file size.
func (c *Converter) ConvertCBZWithMetadata(ctx context.Context, srcPath, destPath string, metadata *CBZMetadata) error {
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

	conversion, err := c.PrepareCBZ(srcZip, metadata)
	if err != nil {
		return err
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

	w := bufio.NewWriter(destFile)
	if err := conversion.Write(ctx, w); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return errors.Wrap(err, "failed to write destination file")
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

// CBZConversion is a CBZ ready to be converted to a fixed-layout KePub.
type CBZConversion struct {
	pages    []*zip.File
	metadata *CBZMetadata
}

// PrepareCBZ finds the pages of the CBZ in srcZip and checks that each can
// be opened, so a CBZ the conversion cannot use fails before anything is
// written. Pages are read only by Write, and srcZip must stay readable
// until it returns.
func (c *Converter) PrepareCBZ(srcZip *zip.Reader, metadata *CBZMetadata) (*CBZConversion, error) {
	pages := cbz.PageImages(srcZip)
	if len(pages) == 0 {
		return nil, errors.New("no images found in CBZ file")
	}
	for _, page := range pages {
		if err := cbz.CheckReadable(page); err != nil {
			return nil, err
		}
	}
	return &CBZConversion{pages: pages, metadata: metadata}, nil
}

// Write writes the KePub to w front to back. Each page's XHTML and image
// are written as soon as the page is processed, a bounded number of pages
// ahead, and the zip writer is flushed after each page so output arrives
// about once per page. The OPF, NCX, and nav list every page with its
// dimensions, so they come last; the container only requires mimetype
// first, and container.xml points to the OPF by path.
func (cv *CBZConversion) Write(ctx context.Context, w io.Writer) error {
	destZip := zip.NewWriter(w)

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

	// Generate CSS for fixed layout
	if err := writeZipFile(destZip, "OEBPS/styles.css", generateFixedLayoutCSS()); err != nil {
		return errors.Wrap(err, "failed to write styles.css")
	}
	if err := destZip.Flush(); err != nil {
		return errors.Wrap(err, "failed to write destination")
	}

	// Generate one XHTML page per image (required for fixed-layout), each
	// with its processed image: JPEG/PNG resized to fit the screen, PNG
	// converted to JPEG.
	pages := make([]pageInfo, 0, len(cv.pages))
	err = cbz.ProcessInOrder(ctx, cv.pages, processCBZPage, func(i int, processed *ProcessedImage) error {
		page := pageInfo{
			filename:  fmt.Sprintf("page%04d%s", i+1, processed.Ext),
			width:     processed.Width,
			height:    processed.Height,
			mediaType: processed.MediaType,
		}
		pageName := fmt.Sprintf("OEBPS/page%04d.xhtml", i+1)
		if err := writeZipFile(destZip, pageName, generateImagePage(page, i)); err != nil {
			return errors.Wrapf(err, "failed to write %s", pageName)
		}
		imgPath := "OEBPS/images/" + page.filename
		if err := writeZipFile(destZip, imgPath, processed.Data); err != nil {
			return errors.Wrapf(err, "failed to write %s", imgPath)
		}
		if err := destZip.Flush(); err != nil {
			return errors.Wrapf(err, "failed to write %s", imgPath)
		}
		pages = append(pages, page)
		return nil
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Wrap(ctxErr, "conversion cancelled")
		}
		return err
	}

	// The NCX's dtb:uid must equal the OPF's unique identifier.
	uid := generateUUID()

	// Generate OPF content
	if err := writeZipFile(destZip, "OEBPS/content.opf", generateFixedLayoutOPF(pages, cv.metadata, uid)); err != nil {
		return errors.Wrap(err, "failed to write content.opf")
	}

	// Generate NCX (table of contents)
	if err := writeZipFile(destZip, "OEBPS/toc.ncx", generateNCX(pages, cv.metadata, uid)); err != nil {
		return errors.Wrap(err, "failed to write toc.ncx")
	}

	// Generate nav.xhtml (required for EPUB3 and helps Kobo navigation)
	if err := writeZipFile(destZip, "OEBPS/nav.xhtml", generateNavXHTML(pages, cv.metadata)); err != nil {
		return errors.Wrap(err, "failed to write nav.xhtml")
	}

	if err := destZip.Close(); err != nil {
		return errors.Wrap(err, "failed to finalize destination")
	}
	return nil
}

// processCBZPage reads a page and processes it for e-readers.
func processCBZPage(imgFile *zip.File) (*ProcessedImage, error) {
	data, err := readZipFile(imgFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read image: %s", imgFile.Name)
	}
	return ProcessImageForEreader(data, strings.ToLower(filepath.Ext(imgFile.Name))), nil
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
func generateFixedLayoutOPF(pages []pageInfo, metadata *CBZMetadata, uid string) []byte {
	var buf bytes.Buffer

	// Determine title
	title := getEffectiveTitle(metadata)

	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:`)
	buf.WriteString(uid)
	buf.WriteString(`</dc:identifier>
    <dc:title>`)
	buf.WriteString(html.EscapeString(title))
	buf.WriteString(`</dc:title>
    <dc:language>`)
	lang := "en"
	if metadata != nil && metadata.Language != nil && *metadata.Language != "" {
		lang = *metadata.Language
	}
	buf.WriteString(html.EscapeString(lang))
	buf.WriteString(`</dc:language>
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
func generateNCX(pages []pageInfo, metadata *CBZMetadata, uid string) []byte {
	var buf bytes.Buffer

	// Determine title
	title := getEffectiveTitle(metadata)

	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <head>
    <meta name="dtb:uid" content="urn:uuid:`)
	buf.WriteString(uid)
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

// Kobo Libra Color screen dimensions (from KCC profiles).
// All images are resized to fit within these dimensions while preserving aspect ratio.
const (
	koboWidth  = 1264
	koboHeight = 1680
)

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
// - PNG images are converted to JPEG for smaller file size
// - Other formats (GIF, WebP) are passed through unchanged.
//
// Tones and colors are left alone: one generated file serves every reader,
// from e-ink to phones, so reducing grays or dropping color for a screen is
// left to the reading app. A page comes out gray only if it holds nothing
// but grays: it decoded as gray, or as a palette of grays.
//
// pkg/filegen/cbz.go uses it too, so a change here also changes regular CBZ
// downloads.
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

	if !needsResize && !needsConvert {
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
		// A strip thinner than one pixel at this scale keeps one.
		newWidth = max(1, int(float64(origWidth)*ratio))
		newHeight = max(1, int(float64(origHeight)*ratio))
	}

	// Create the output image
	// image/jpeg writes a gray JPEG only for an *image.Gray.
	outputImg := img
	isGray := isGrayImage(img)
	if needsResize {
		outputImg = shrink(img, newWidth, newHeight, isGray)
	} else if _, ok := img.(*image.Gray); isGray && !ok {
		gray := image.NewGray(bounds)
		draw.Draw(gray, bounds, img, bounds.Min, draw.Src)
		outputImg = gray
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

// isGrayImage reports whether img holds only grays: a gray image, or a
// palette image, as scans are often saved, whose colors are all gray.
func isGrayImage(img image.Image) bool {
	switch img := img.(type) {
	case *image.Gray, *image.Gray16:
		return true
	case *image.Paletted:
		for _, c := range img.Palette {
			r, g, b, _ := c.RGBA()
			if r != g || g != b {
				return false
			}
		}
		return true
	}
	return false
}

// Ensure png package is used for decoding (image.Decode uses registered decoders).
var _ = png.Decode
