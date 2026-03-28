package pdf

import (
	"time"

	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/pkg/errors"
)

// OutlineEntry represents a single bookmark from a PDF's outline tree.
type OutlineEntry struct {
	Title     string
	StartPage int // 0-indexed page number
}

// ExtractOutline extracts the bookmark/outline tree from a PDF and returns
// a flat list of entries with their target page numbers.
// Returns an empty slice (not an error) if the PDF has no bookmarks.
func ExtractOutline(path string) ([]OutlineEntry, error) {
	instance, err := PdfiumInstance(30 * time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get pdfium instance")
	}
	defer instance.Close()

	doc, err := instance.OpenDocument(&requests.OpenDocument{
		FilePath: &path,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to open PDF")
	}
	defer func() {
		_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{
			Document: doc.Document,
		})
	}()

	bookmarksResp, err := instance.GetBookmarks(&requests.GetBookmarks{
		Document: doc.Document,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get bookmarks")
	}

	if len(bookmarksResp.Bookmarks) == 0 {
		return nil, nil
	}

	var entries []OutlineEntry
	flattenBookmarks(bookmarksResp.Bookmarks, &entries)
	return entries, nil
}

// flattenBookmarks recursively walks the bookmark tree and appends entries
// with valid page destinations to the result slice.
func flattenBookmarks(bookmarks []responses.GetBookmarksBookmark, result *[]OutlineEntry) {
	for _, bm := range bookmarks {
		if bm.DestInfo != nil {
			*result = append(*result, OutlineEntry{
				Title:     bm.Title,
				StartPage: bm.DestInfo.PageIndex,
			})
		}
		if len(bm.Children) > 0 {
			flattenBookmarks(bm.Children, result)
		}
	}
}
