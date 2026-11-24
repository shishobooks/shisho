package epub

import (
	"archive/zip"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/wordcount"
)

// WordCount counts the words in the text of the EPUB or KePub at path, as
// Parse and ParseKePub do, without reading the rest of its metadata. It
// returns mediafile.ErrDRMProtected for a book whose content is encrypted.
func WordCount(path string) (int, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer zr.Close()
	result, err := findOPF(&zr.Reader)
	if err != nil {
		return 0, err
	}
	return countWords(&zr.Reader, result)
}

// countWords counts the words in an open EPUB's spine, after checking that
// its content isn't encrypted, which would count noise as words.
func countWords(zr *zip.Reader, result *ParseOPFResult) (int, error) {
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
	return countSpine(zr, result)
}

// countSpine counts the words in the spine's HTML documents. Other spine
// items, such as an SVG page, hold no running text.
func countSpine(zr *zip.Reader, result *ParseOPFResult) (int, error) {
	total := 0
	for _, doc := range spineDocuments(zr, result) {
		if !isHTMLMediaType(doc.mediaType) {
			continue
		}
		n, err := countDocument(doc.file)
		if err != nil {
			return 0, errors.Wrapf(err, "count words in %s", doc.file.Name)
		}
		total += n
	}
	return total, nil
}

func countDocument(f *zip.File) (int, error) {
	r, err := f.Open()
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer r.Close()
	return wordcount.HTML(r)
}

// isHTMLMediaType reports whether a manifest item is an HTML document. An
// item with no media type is read as one, since the spine names documents.
func isHTMLMediaType(mediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "" || mediaType == "application/xhtml+xml" || mediaType == "text/html"
}
