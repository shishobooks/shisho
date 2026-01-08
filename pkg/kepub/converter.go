// Package kepub provides conversion utilities for creating KePub files
// compatible with Kobo e-readers.
package kepub

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// Converter handles EPUB to KePub conversion.
type Converter struct {
	// AddDummyTitlepage adds a dummy titlepage if needed for fullscreen covers.
	AddDummyTitlepage bool
}

// NewConverter creates a new Converter with default settings.
func NewConverter() *Converter {
	return &Converter{
		AddDummyTitlepage: true,
	}
}

// ConvertEPUB converts an EPUB file to KePub format.
// The source file is read and a new KePub file is written to destPath.
func (c *Converter) ConvertEPUB(ctx context.Context, srcPath, destPath string) error {
	// Open source EPUB
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
		return errors.Wrap(err, "failed to read source EPUB as zip")
	}

	// Create temporary output file
	tmpPath := destPath + ".tmp"
	destFile, err := os.Create(tmpPath)
	if err != nil {
		return errors.Wrap(err, "failed to create destination file")
	}
	defer func() {
		destFile.Close()
		os.Remove(tmpPath) // Clean up temp file if we don't rename it
	}()

	destZip := zip.NewWriter(destFile)

	// Track content files that need transformation
	contentFiles := make(map[string]bool)
	var opfPath string

	// First pass: identify content files from OPF
	for _, f := range srcZip.File {
		if filepath.Ext(f.Name) == ".opf" {
			opfPath = f.Name
			items, err := parseOPFManifest(f)
			if err != nil {
				return errors.Wrap(err, "failed to parse OPF manifest")
			}
			opfDir := filepath.Dir(opfPath)
			if opfDir == "." {
				opfDir = ""
			}
			for _, item := range items {
				if isContentFile(item.MediaType) {
					itemPath := item.Href
					if opfDir != "" {
						itemPath = opfDir + "/" + itemPath
					}
					contentFiles[itemPath] = true
				}
			}
			break
		}
	}

	// Write mimetype first (must be uncompressed per EPUB spec)
	mimetypeWritten := false
	for _, srcZipFile := range srcZip.File {
		if srcZipFile.Name == "mimetype" {
			content, err := readZipFile(srcZipFile)
			if err != nil {
				return errors.Wrap(err, "failed to read mimetype")
			}
			mimeWriter, err := destZip.CreateHeader(&zip.FileHeader{
				Name:   "mimetype",
				Method: zip.Store, // Must be uncompressed
			})
			if err != nil {
				return errors.Wrap(err, "failed to create mimetype")
			}
			if _, err := mimeWriter.Write(content); err != nil {
				return errors.Wrap(err, "failed to write mimetype")
			}
			mimetypeWritten = true
			break
		}
	}

	// Check if kobo.js already exists in the source EPUB
	hasKoboJS := false
	for _, f := range srcZip.File {
		if f.Name == "kobo.js" {
			hasKoboJS = true
			break
		}
	}

	// Add kobo.js file for pagination and progress tracking (only if not present)
	if !hasKoboJS {
		koboJSWriter, err := destZip.CreateHeader(&zip.FileHeader{
			Name:   "kobo.js",
			Method: zip.Deflate,
		})
		if err != nil {
			return errors.Wrap(err, "failed to create kobo.js")
		}
		if _, err := koboJSWriter.Write([]byte(koboJS)); err != nil {
			return errors.Wrap(err, "failed to write kobo.js")
		}
	}

	// Process each file in the source EPUB (skip mimetype as it's already written)
	for _, srcZipFile := range srcZip.File {
		// Skip mimetype - already written first
		if srcZipFile.Name == "mimetype" && mimetypeWritten {
			continue
		}
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "conversion cancelled")
		default:
		}

		var destContent []byte
		var err error

		if contentFiles[srcZipFile.Name] {
			// Create a new span counter for each file (per-file counters, starting at 1)
			// This matches Kobo's expected format for progress tracking
			spanCounter := NewSpanCounter()
			// Compute relative path to kobo.js from content file location
			scriptPath := computeRelativeScriptPath(srcZipFile.Name)
			destContent, err = c.transformContentFileWithOptions(srcZipFile, spanCounter, scriptPath)
			if err != nil {
				return errors.Wrapf(err, "failed to transform content file: %s", srcZipFile.Name)
			}
		} else if srcZipFile.Name == opfPath {
			// Transform OPF file
			destContent, err = c.transformOPFFile(srcZipFile)
			if err != nil {
				return errors.Wrap(err, "failed to transform OPF file")
			}
		} else {
			// Copy file unchanged
			destContent, err = readZipFile(srcZipFile)
			if err != nil {
				return errors.Wrapf(err, "failed to read file: %s", srcZipFile.Name)
			}
		}

		// Write to destination
		destZipFile, err := destZip.CreateHeader(&zip.FileHeader{
			Name:   srcZipFile.Name,
			Method: zip.Deflate,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to create file in destination: %s", srcZipFile.Name)
		}

		if _, err := destZipFile.Write(destContent); err != nil {
			return errors.Wrapf(err, "failed to write file to destination: %s", srcZipFile.Name)
		}
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

// transformContentFileWithOptions transforms a content file with full options.
func (c *Converter) transformContentFileWithOptions(f *zip.File, counter *SpanCounter, scriptPath string) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var buf bytes.Buffer
	if err := TransformContentWithOptions(r, &buf, counter, scriptPath); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// computeRelativeScriptPath computes the relative path to kobo.js from a content file.
// kobo.js is always at the root of the EPUB.
func computeRelativeScriptPath(filePath string) string {
	// Count directory depth
	depth := strings.Count(filePath, "/")
	if depth == 0 {
		return "kobo.js"
	}
	// Build relative path with appropriate number of "../"
	parts := make([]string, depth)
	for i := range parts {
		parts[i] = ".."
	}
	return strings.Join(parts, "/") + "/kobo.js"
}

// transformOPFFile transforms the OPF file for KePub.
func (c *Converter) transformOPFFile(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var buf bytes.Buffer
	if err := TransformOPF(r, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// isContentFile returns true if the media type indicates a transformable content file.
// Note: NCX files (application/x-dtbncx+xml) are NOT content files - they are navigation
// files and should not have Kobo spans added to them.
func isContentFile(mediaType string) bool {
	switch mediaType {
	case "application/xhtml+xml", "text/html":
		return true
	}
	return false
}

// readZipFile reads the contents of a zip file entry.
func readZipFile(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

// manifestItem represents an item from the OPF manifest.
type manifestItem struct {
	ID        string
	Href      string
	MediaType string
}

// parseOPFManifest parses manifest items from an OPF file.
func parseOPFManifest(f *zip.File) ([]manifestItem, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return extractManifestItems(data), nil
}

// extractManifestItems extracts manifest items from OPF XML data.
// Uses simple string parsing to avoid XML parsing issues with namespaces.
func extractManifestItems(data []byte) []manifestItem {
	content := string(data)
	var items []manifestItem

	// Find manifest section
	manifestStart := strings.Index(content, "<manifest")
	if manifestStart == -1 {
		return items
	}
	manifestEnd := strings.Index(content[manifestStart:], "</manifest>")
	if manifestEnd == -1 {
		return items
	}
	manifestContent := content[manifestStart : manifestStart+manifestEnd]

	// Find all item tags
	pos := 0
	for {
		itemStart := strings.Index(manifestContent[pos:], "<item")
		if itemStart == -1 {
			break
		}
		itemStart += pos
		itemEnd := strings.Index(manifestContent[itemStart:], ">")
		if itemEnd == -1 {
			break
		}
		itemTag := manifestContent[itemStart : itemStart+itemEnd+1]

		item := manifestItem{
			ID:        extractAttribute(itemTag, "id"),
			Href:      extractAttribute(itemTag, "href"),
			MediaType: extractAttribute(itemTag, "media-type"),
		}
		if item.Href != "" {
			items = append(items, item)
		}

		pos = itemStart + itemEnd + 1
	}

	return items
}

// extractAttribute extracts an attribute value from an XML tag string.
func extractAttribute(tag, attr string) string {
	// Try both single and double quotes
	for _, quote := range []string{"\"", "'"} {
		search := attr + "=" + quote
		start := strings.Index(tag, search)
		if start == -1 {
			continue
		}
		start += len(search)
		end := strings.Index(tag[start:], quote)
		if end == -1 {
			continue
		}
		return tag[start : start+end]
	}
	return ""
}

// koboJS is the JavaScript file content that handles pagination and progress tracking
// for Kobo e-readers. This provides the interface Kobo devices expect for reading
// position tracking, page navigation, and bookmark management.
const koboJS = `var gPosition = 0;
var gProgress = 0;
var gCurrentPage = 0;
var gPageCount = 0;
var gClientHeight = null;

function getPosition() { return gPosition; }
function getProgress() { return gProgress; }
function getPageCount() { return gPageCount; }
function getCurrentPage() { return gCurrentPage; }

function setupBookColumns() {
	var body = document.getElementsByTagName('body')[0].style;
	body.marginLeft = '0px !important';
	body.marginRight = '0px !important';
	body.marginTop = '0px !important';
	body.marginBottom = '0px !important';
	body.paddingTop = '0px !important';
	body.paddingBottom = '0px !important';

	var bc = document.getElementById('book-columns');
	if (!bc) return;
	bc = bc.style;
	bc.width = (window.innerWidth * 2) + 'px !important';
	bc.height = window.innerHeight + 'px !important';
	bc.marginTop = '0px !important';
	bc.webkitColumnWidth = window.innerWidth + 'px !important';
	bc.webkitColumnGap = '0px !important';
	bc.overflow = 'none';
	bc.paddingTop = '0px !important';
	bc.paddingBottom = '0px !important';

	var bi = document.getElementById('book-inner');
	if (bi) {
		bi = bi.style;
		bi.marginLeft = '10px';
		bi.marginRight = '10px';
		bi.padding = '0';
	}

	gCurrentPage = 1;
	gProgress = gPosition = 0;
	gPageCount = document.body.scrollWidth / window.innerWidth;
	if (gClientHeight < window.innerHeight) {
		gPageCount = 1;
	}
}

function paginate(tagId) {
	if (gClientHeight == undefined) {
		var bc = document.getElementById('book-columns');
		if (bc) gClientHeight = bc.clientHeight;
	}
	setupBookColumns();
	if (window.device) {
		window.device.reportPageCount(gPageCount);
		var tagIdPageNumber = 0;
		if (tagId && tagId.length > 0) {
			tagIdPageNumber = estimatePageNumberForAnchor(tagId);
		}
		window.device.finishedPagination(tagId, tagIdPageNumber);
	}
}

function repaginate(tagId) { paginate(tagId); }

function updateProgress() {
	gProgress = (gCurrentPage - 1.0) / gPageCount;
}

function updateBookmark() {
	gProgress = (gCurrentPage - 1.0) / gPageCount;
	var anchorName = estimateFirstAnchorForPageNumber(gCurrentPage - 1);
	if (window.device) window.device.finishedUpdateBookmark(anchorName);
}

function goBack() {
	if (gCurrentPage > 1) {
		--gCurrentPage;
		gPosition -= window.innerWidth;
		window.scrollTo(gPosition, 0);
		if (window.device) window.device.pageChanged();
	} else {
		if (window.device) window.device.previousChapter();
	}
}

function goForward() {
	if (gCurrentPage < gPageCount) {
		++gCurrentPage;
		gPosition += window.innerWidth;
		window.scrollTo(gPosition, 0);
		if (window.device) window.device.pageChanged();
	} else {
		if (window.device) window.device.nextChapter();
	}
}

function goPage(pageNumber, callPageReadyWhenDone) {
	if (pageNumber > 0 && pageNumber <= gPageCount) {
		gCurrentPage = pageNumber;
		gPosition = (gCurrentPage - 1) * window.innerWidth;
		window.scrollTo(gPosition, 0);
		if (window.device) {
			if (callPageReadyWhenDone > 0) {
				window.device.pageReady();
			} else {
				window.device.pageChanged();
			}
		}
	}
}

function goProgress(progress) {
	progress += 0.0001;
	var progressPerPage = 1.0 / gPageCount;
	var newPage = 0;
	for (var page = 0; page < gPageCount; page++) {
		var low = page * progressPerPage;
		var high = low + progressPerPage;
		if (progress >= low && progress < high) {
			newPage = page;
			break;
		}
	}
	gCurrentPage = newPage + 1;
	gPosition = (gCurrentPage - 1) * window.innerWidth;
	window.scrollTo(gPosition, 0);
	updateProgress();
}

function estimateFirstAnchorForPageNumber(page) {
	var spans = document.getElementsByTagName('span');
	var lastKoboSpanId = "";
	for (var i = 0; i < spans.length; i++) {
		if (spans[i].id.substr(0, 5) == "kobo.") {
			lastKoboSpanId = spans[i].id;
			if (spans[i].offsetTop >= (page * window.innerHeight)) {
				return spans[i].id;
			}
		}
	}
	return lastKoboSpanId;
}

function estimatePageNumberForAnchor(spanId) {
	var span = document.getElementById(spanId);
	if (span) {
		return Math.floor(span.offsetTop / window.innerHeight);
	}
	return 0;
}
`
