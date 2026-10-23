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
	"sync"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/ziputil"
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
// Downloads stream through WriteEPUB instead; this wraps it for callers
// that have only paths.
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
	entries := make([]Entry, len(srcZip.File))
	for i, f := range srcZip.File {
		entries[i] = Entry{Name: f.Name, File: f}
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

	if err := c.WriteEPUB(ctx, destFile, entries); err != nil {
		return err
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

// Entry is an entry of the EPUB a conversion reads: a zip entry, or content
// that replaces one.
type Entry struct {
	Name string
	// File is the entry read from a zip, or nil when Content replaces it.
	File    *zip.File
	Content []byte
}

// content returns the entry's uncompressed data.
func (e Entry) content() ([]byte, error) {
	if e.File == nil {
		return e.Content, nil
	}
	return readZipFile(e.File)
}

// lookahead is how many entries the conversion transforms ahead of the one
// being written, so the writer rarely waits on a transform.
const lookahead = 4

// WriteEPUB converts the EPUB made of entries to KePub and writes it to w
// front to back. The mimetype entry goes first and kobo.js second; the
// content documents and the OPF are transformed, other replaced entries
// are compressed, and every other entry is copied without recompressing.
func (c *Converter) WriteEPUB(ctx context.Context, w io.Writer, entries []Entry) error {
	contentFiles := make(map[string]bool)
	opfPath := ""
	hasKoboJS := false
	for _, e := range entries {
		if e.Name == "kobo.js" {
			hasKoboJS = true
		}
		if opfPath != "" || filepath.Ext(e.Name) != ".opf" {
			continue
		}
		opfPath = e.Name
		data, err := e.content()
		if err != nil {
			return errors.Wrap(err, "failed to parse OPF manifest")
		}
		for _, item := range extractManifestItems(data) {
			if isContentFile(item.MediaType) {
				contentFiles[manifestEntryPath(opfPath, item.Href)] = true
			}
		}
	}

	// convert returns the compressed output for an entry that is not copied
	// as is, or nil for one that is.
	convert := func(ctx context.Context, e Entry) (*ziputil.Entry, error) {
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "conversion cancelled")
		}
		switch {
		case contentFiles[e.Name]:
			data, err := e.content()
			if err != nil {
				return nil, errors.Wrapf(err, "failed to transform content file: %s", e.Name)
			}
			// Create a new span counter for each file (per-file counters, starting at 1)
			// This matches Kobo's expected format for progress tracking
			var buf bytes.Buffer
			err = TransformContentWithOptions(bytes.NewReader(data), &buf, NewSpanCounter(), computeRelativeScriptPath(e.Name))
			if err != nil {
				return nil, errors.Wrapf(err, "failed to transform content file: %s", e.Name)
			}
			return ziputil.Compress(e.Name, zip.Deflate, buf.Bytes())
		case e.Name == opfPath:
			data, err := e.content()
			if err != nil {
				return nil, errors.Wrap(err, "failed to transform OPF file")
			}
			return ziputil.Compress(e.Name, zip.Deflate, transformKepubOPF(data, opfPath, contentFiles))
		case e.File == nil:
			return ziputil.Compress(e.Name, zip.Deflate, e.Content)
		default:
			return nil, nil
		}
	}

	// The mimetype entry is written first, so any other is dropped.
	var body []Entry
	for _, e := range entries {
		if e.Name != "mimetype" {
			body = append(body, e)
		}
	}
	lead := []leadEntry{{name: "mimetype", method: zip.Store, content: []byte("application/epub+zip")}}
	// Add kobo.js file for pagination and progress tracking (only if not present)
	if !hasKoboJS {
		lead = append(lead, leadEntry{name: koboJSPath, method: zip.Deflate, content: []byte(koboJS)})
	}
	return writeZip(ctx, w, lead, body, convert)
}

// leadEntry is an entry written ahead of the converted ones.
type leadEntry struct {
	name    string
	method  uint16
	content []byte
}

// writeZip writes lead and then body to w front to back. Each entry of body
// is written as convert returns it, or copied without recompressing when
// convert returns nil. convert runs up to lookahead entries ahead of the
// writer.
func writeZip(ctx context.Context, w io.Writer, lead []leadEntry, body []Entry, convert func(context.Context, Entry) (*ziputil.Entry, error)) error {
	ctx, cancel := context.WithCancel(ctx)
	results := convertAhead(ctx, body, convert)
	defer func() {
		cancel()
		results.wait()
	}()

	zw := zip.NewWriter(w)
	for _, l := range lead {
		entry, err := ziputil.Compress(l.name, l.method, l.content)
		if err != nil {
			return err
		}
		if err := entry.Write(zw); err != nil {
			return err
		}
	}
	// Send the first entries now, so the download starts before the first
	// transform finishes.
	if err := zw.Flush(); err != nil {
		return errors.WithStack(err)
	}

	for i, e := range body {
		converted, err := results.next(ctx, i)
		if err != nil {
			return err
		}
		if converted != nil {
			err = converted.Write(zw)
		} else {
			err = ziputil.CopyRaw(ctx, zw, e.File)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to write file to destination: %s", e.Name)
		}
	}

	return errors.Wrap(zw.Close(), "failed to finalize destination")
}

// converted is one entry's result from convertAhead.
type converted struct {
	entry *ziputil.Entry
	err   error
}

// aheadResults hands convertAhead's results to the writer in entry order.
type aheadResults struct {
	// results holds each entry's result once its conversion has started.
	results []chan converted
	// slots holds a token for each conversion the writer has not taken yet.
	slots chan struct{}
	wg    sync.WaitGroup
}

// convertAhead runs convert over entries in the background, at most
// lookahead entries ahead of the writer, which takes the results in order
// with next. Cancel ctx and call wait once the writer is done.
func convertAhead(ctx context.Context, entries []Entry, convert func(context.Context, Entry) (*ziputil.Entry, error)) *aheadResults {
	r := &aheadResults{
		results: make([]chan converted, len(entries)),
		slots:   make(chan struct{}, lookahead),
	}
	for i := range r.results {
		r.results[i] = make(chan converted, 1)
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for i, e := range entries {
			select {
			case r.slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			r.wg.Add(1)
			go func() {
				defer r.wg.Done()
				// A panic on a malformed document fails the conversion
				// instead of the process; the download cache only recovers
				// panics on the goroutine that writes.
				defer func() {
					if p := recover(); p != nil {
						r.results[i] <- converted{err: errors.Errorf("panic converting %s: %v", e.Name, p)}
					}
				}()
				entry, err := convert(ctx, e)
				r.results[i] <- converted{entry: entry, err: err}
			}()
		}
	}()
	return r
}

// next returns entry i's result: its output, or nil when it is copied as
// is.
func (r *aheadResults) next(ctx context.Context, i int) (*ziputil.Entry, error) {
	select {
	case c := <-r.results[i]:
		<-r.slots
		return c.entry, c.err
	case <-ctx.Done():
		return nil, errors.Wrap(ctx.Err(), "conversion cancelled")
	}
}

// wait waits for the background conversions to end.
func (r *aheadResults) wait() {
	r.wg.Wait()
}

// koboJSPath is where kobo.js goes in a KePub: the root of the zip.
const koboJSPath = "kobo.js"

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
