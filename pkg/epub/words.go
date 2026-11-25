package epub

import (
	"archive/zip"
	"context"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/wordcount"
)

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
	seen := map[*zip.File]bool{}
	total := 0
	remaining := limit
	for _, doc := range spineDocuments(zr, result) {
		if err := ctx.Err(); err != nil {
			return 0, errors.WithStack(err)
		}
		if !isHTMLMediaType(doc.mediaType) || seen[doc.file] {
			continue
		}
		seen[doc.file] = true
		n, read, err := countDocument(doc.file, remaining)
		if err != nil {
			return 0, errors.Wrapf(err, "count words in %s", doc.file.Name)
		}
		remaining -= read
		if remaining < 0 {
			return 0, errors.Errorf("book's text is larger than %d bytes", limit)
		}
		total += n
	}
	if len(seen) == 0 {
		return 0, errors.New("spine names no document in the book")
	}
	return total, nil
}

// countDocument counts the words in one document, reading at most one byte
// past limit, and returns how many bytes it read.
func countDocument(f *zip.File, limit int64) (int, int64, error) {
	r, err := f.Open()
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}
	defer r.Close()
	limited := &io.LimitedReader{R: r, N: limit + 1}
	n, err := wordcount.HTML(limited)
	return n, limit + 1 - limited.N, err
}

// isHTMLMediaType reports whether a manifest item is an HTML document. An
// item with no media type is read as one, since the spine names documents.
func isHTMLMediaType(mediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "" || mediaType == "application/xhtml+xml" || mediaType == "text/html"
}
