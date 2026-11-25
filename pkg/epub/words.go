package epub

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"io"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/wordcount"
)

// maxPicturePageWords is the most words a page with a picture holds and
// still be a picture page, such as a comic page with a caption. A book
// mostly of picture pages, such as manga made into an EPUB, gets no count:
// its words are a copyright page and an introduction, not what the reader
// reads.
const maxPicturePageWords = 20

// maxBookTextBytes bounds how much of a book's documents a count reads, as
// the MOBI reader bounds a book's text, so a crafted EPUB can't make a scan
// decompress without end. The longest books are tens of megabytes.
const maxBookTextBytes = 256 << 20

// WordCount counts the words in the text of the EPUB or KePub at path, as
// Parse and ParseKePub do, without reading the rest of its metadata. It
// returns mediafile.ErrDRMProtected for a book whose content is encrypted.
func WordCount(ctx context.Context, path string) (int, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer zr.Close()
	result, err := findOPF(&zr.Reader)
	if err != nil {
		return 0, err
	}
	return countWords(ctx, &zr.Reader, result)
}

// countWords counts the words in an open EPUB's spine, after checking that
// its content isn't encrypted, which would count noise as words.
func countWords(ctx context.Context, zr *zip.Reader, result *ParseOPFResult) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errors.WithStack(err)
	}
	protected, err := declaresDRM(zr)
	if err != nil {
		return 0, err
	}
	if !protected {
		protected, err = spineEncrypted(zr, result)
		if err != nil {
			return 0, err
		}
	}
	if protected {
		return 0, errors.WithStack(mediafile.ErrDRMProtected)
	}
	return countSpine(ctx, zr, result, maxBookTextBytes)
}

// countSpine counts the words in the spine's HTML documents, each once
// however often the spine lists it, reading at most limit bytes of them.
// Other spine items, such as an SVG page, hold no running text. A spine that
// names no HTML document in the book is an error, not zero words.
func countSpine(ctx context.Context, zr *zip.Reader, result *ParseOPFResult, limit int64) (int, error) {
	// A fixed-layout book lays out pictures of pages, and its few words
	// aren't what the reader reads.
	if fixedLayout(zr, result) {
		return 0, errors.New("a fixed-layout book has no word count")
	}
	seen := map[*zip.File]bool{}
	total, picturePages := 0, 0
	remaining := limit
	for _, doc := range spineDocuments(zr, result) {
		if err := ctx.Err(); err != nil {
			return 0, errors.WithStack(err)
		}
		if !isHTMLMediaType(doc.mediaType) || seen[doc.file] {
			continue
		}
		seen[doc.file] = true
		counted, read, err := countDocument(doc.file, remaining)
		if err != nil {
			return 0, errors.Wrapf(err, "count words in %s", doc.file.Name)
		}
		remaining -= read
		if remaining < 0 {
			return 0, errors.Errorf("book's text is larger than %d bytes", limit)
		}
		total += counted.Words
		if counted.Pictures > 0 && counted.Words <= maxPicturePageWords {
			picturePages++
		}
	}
	if len(seen) == 0 {
		return 0, errors.New("spine names no document in the book")
	}
	if picturePages*2 > len(seen) {
		return 0, errors.New("a book mostly of picture pages has no word count")
	}
	return total, nil
}

// countDocument counts the words and pictures in one document, reading at
// most one byte past limit, and returns how many bytes it read.
func countDocument(f *zip.File, limit int64) (wordcount.Document, int64, error) {
	r, err := f.Open()
	if err != nil {
		return wordcount.Document{}, 0, errors.WithStack(err)
	}
	defer r.Close()
	limited := &io.LimitedReader{R: r, N: limit + 1}
	doc, err := wordcount.Count(limited)
	return doc, limit + 1 - limited.N, err
}

// displayOptionsFiles are where Apple's and Kobo's books say how they're laid
// out, which the reader reads too.
var displayOptionsFiles = []string{"META-INF/com.apple.ibooks.display-options.xml", "META-INF/com.kobobooks.display-options.xml"}

// fixedLayout reports whether a book lays out fixed pages, as the reader
// decides: the package says so, every spine item does, or its display
// options do.
func fixedLayout(zr *zip.Reader, result *ParseOPFResult) bool {
	for _, meta := range result.Package.Metadata.Meta {
		if meta.Property == "rendition:layout" && strings.TrimSpace(meta.Text) == "pre-paginated" {
			return true
		}
	}
	itemrefs := result.Package.Spine.Itemref
	fixedItems := 0
	for _, ref := range itemrefs {
		if slices.Contains(strings.Fields(ref.Properties), "rendition:layout-pre-paginated") {
			fixedItems++
		}
	}
	if len(itemrefs) > 0 && fixedItems == len(itemrefs) {
		return true
	}
	for _, f := range zr.File {
		if slices.Contains(displayOptionsFiles, f.Name) && displayOptionsFixed(f) {
			return true
		}
	}
	return false
}

// displayOptionsFixed reports whether a display options file sets the
// fixed-layout option, for any platform.
func displayOptionsFixed(f *zip.File) bool {
	r, err := f.Open()
	if err != nil {
		return false
	}
	defer r.Close()
	var doc struct {
		Platforms []struct {
			Options []struct {
				Name  string `xml:"name,attr"`
				Value string `xml:",chardata"`
			} `xml:"option"`
		} `xml:"platform"`
	}
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return false
	}
	for _, platform := range doc.Platforms {
		for _, option := range platform.Options {
			if option.Name == "fixed-layout" && strings.TrimSpace(option.Value) == "true" {
				return true
			}
		}
	}
	return false
}

// isHTMLMediaType reports whether a manifest item is an HTML document. An
// item with no media type is read as one, since the spine names documents.
func isHTMLMediaType(mediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "" || mediaType == "application/xhtml+xml" || mediaType == "text/html"
}
