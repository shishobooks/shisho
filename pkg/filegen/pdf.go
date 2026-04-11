package filegen

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdf"
)

// noParentPage is the sentinel passed to convertModelChaptersToPDFBookmarks at
// the top level, meaning "no parent constraint — any non-negative page is OK".
const noParentPage = -1

// PDFGenerator generates PDF files with modified metadata.
type PDFGenerator struct{}

// SupportedType returns the file type this generator handles.
func (g *PDFGenerator) SupportedType() string {
	return models.FileTypePDF
}

// Generate creates a modified PDF at destPath with updated metadata.
// The source file is never modified; metadata is written into the destination copy.
func (g *PDFGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	// Check context cancellation before starting.
	if err := ctx.Err(); err != nil {
		return NewGenerationError(models.FileTypePDF, err, "context cancelled")
	}

	// Build the info dict properties to update.
	properties := g.buildProperties(book, file)

	// Check context cancellation before the expensive write operation.
	if err := ctx.Err(); err != nil {
		return NewGenerationError(models.FileTypePDF, err, "context cancelled")
	}

	// Ensure pdfcpu global state is initialized before creating a configuration.
	// This avoids data races when Generate is called concurrently.
	pdf.EnsurePdfcpuInit()

	// AddPropertiesFile reads srcPath and writes the result with updated info dict
	// to destPath. When srcPath != destPath, pdfcpu creates destPath directly
	// without modifying srcPath.
	// conf is reused across the two pdfcpu entry points below; both overwrite
	// conf.Cmd on entry so the reuse is safe.
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	if err := api.AddPropertiesFile(srcPath, destPath, properties, conf); err != nil {
		return NewGenerationError(models.FileTypePDF, err, "failed to write PDF metadata")
	}

	// Write chapters back to the destination as a bookmark outline. This is
	// best-effort: a failure here leaves the properties-only destPath in place
	// rather than failing the whole download (matches the cover-extraction
	// pattern in pkg/pdf/pdf.go). Skipped entirely when the DB has no chapters
	// so we don't touch existing bookmarks in the source PDF.
	if file != nil && len(file.Chapters) > 0 {
		pageCount := 0
		if file.PageCount != nil {
			pageCount = *file.PageCount
		}
		// pageCount == 0 means "we don't know how many pages the file has" —
		// the converter treats that as "trust the input, no upper bound".
		bookmarks := convertModelChaptersToPDFBookmarks(file.Chapters, pageCount, noParentPage)
		if len(bookmarks) > 0 {
			// AddBookmarksFile requires distinct input and output paths. Write
			// to a sibling tmp file and rename over destPath on success.
			tmpPath := destPath + ".bookmarks.tmp"
			if err := api.AddBookmarksFile(destPath, tmpPath, bookmarks, true, conf); err != nil {
				_ = os.Remove(tmpPath)
				logger.FromContext(ctx).Warn("failed to write PDF bookmarks, continuing without them", logger.Data{
					"error":    err.Error(),
					"dest":     destPath,
					"chapters": len(bookmarks),
				})
			} else if err := os.Rename(tmpPath, destPath); err != nil {
				_ = os.Remove(tmpPath)
				logger.FromContext(ctx).Warn("failed to finalize PDF with bookmarks, continuing without them", logger.Data{
					"error": err.Error(),
					"dest":  destPath,
				})
			}
		}
	}

	return nil
}

// convertModelChaptersToPDFBookmarks converts database chapter models into
// pdfcpu bookmark entries, preserving nesting.
//
// Chapters are filtered and sorted to satisfy pdfcpu's page-order constraint
// (siblings must be monotonically non-decreasing by page, and a child must not
// start before its parent). Specifically:
//   - nil StartPage, negative StartPage, and StartPage outside [0, pageCount)
//     are dropped
//   - children whose StartPage is less than their parent's are dropped
//   - siblings are sorted by StartPage (ties broken by SortOrder for stability)
//
// Pages are converted from the 0-indexed storage format to the 1-indexed form
// pdfcpu expects. pageCount == 0 disables the upper-bound filter (used when
// the file's PageCount is unknown). parentPage is the parent bookmark's
// 0-indexed StartPage, or noParentPage at the top level.
func convertModelChaptersToPDFBookmarks(chapters []*models.Chapter, pageCount int, parentPage int) []pdfcpu.Bookmark {
	if len(chapters) == 0 {
		return nil
	}

	// First pass: filter out invalid / out-of-range / child-before-parent entries.
	valid := make([]*models.Chapter, 0, len(chapters))
	for _, ch := range chapters {
		if ch.StartPage == nil {
			continue
		}
		page := *ch.StartPage
		if page < 0 {
			continue
		}
		if pageCount > 0 && page >= pageCount {
			continue
		}
		if parentPage != noParentPage && page < parentPage {
			continue
		}
		valid = append(valid, ch)
	}

	// Sort by StartPage; fall back to SortOrder for ties to keep output stable.
	sort.SliceStable(valid, func(i, j int) bool {
		if *valid[i].StartPage != *valid[j].StartPage {
			return *valid[i].StartPage < *valid[j].StartPage
		}
		return valid[i].SortOrder < valid[j].SortOrder
	})

	result := make([]pdfcpu.Bookmark, 0, len(valid))
	for _, ch := range valid {
		page := *ch.StartPage
		bm := pdfcpu.Bookmark{
			Title:    ch.Title,
			PageFrom: page + 1, // pdfcpu is 1-indexed
		}
		if len(ch.Children) > 0 {
			bm.Kids = convertModelChaptersToPDFBookmarks(ch.Children, pageCount, page)
		}
		result = append(result, bm)
	}
	return result
}

// buildProperties constructs the info dict map from book and file models.
// Only fields with non-empty values are included; omitted fields are left
// unchanged by pdfcpu (Producer and Creator are never set here).
func (g *PDFGenerator) buildProperties(book *models.Book, file *models.File) map[string]string {
	props := make(map[string]string)

	// Title
	if book.Title != "" {
		props["Title"] = book.Title
	}

	// Author — join all book authors with ", " sorted by SortOrder.
	if len(book.Authors) > 0 {
		authors := make([]*models.Author, len(book.Authors))
		copy(authors, book.Authors)
		sort.Slice(authors, func(i, j int) bool {
			return authors[i].SortOrder < authors[j].SortOrder
		})
		var names []string
		for _, a := range authors {
			if a.Person != nil && a.Person.Name != "" {
				names = append(names, a.Person.Name)
			}
		}
		if len(names) > 0 {
			props["Author"] = strings.Join(names, ", ")
		}
	}

	// Subject ← book.Description
	if book.Description != nil && *book.Description != "" {
		props["Subject"] = *book.Description
	}

	// Keywords ← tags joined with ", "
	if len(book.BookTags) > 0 {
		var tagNames []string
		for _, bt := range book.BookTags {
			if bt.Tag != nil && bt.Tag.Name != "" {
				tagNames = append(tagNames, bt.Tag.Name)
			}
		}
		if len(tagNames) > 0 {
			props["Keywords"] = strings.Join(tagNames, ", ")
		}
	}

	// CreationDate ← file.ReleaseDate in PDF date format "D:YYYYMMDDHHmmSSZ".
	// Note: pdfcpu always overwrites CreationDate (and ModDate) with the current
	// timestamp during its write phase, so this value will not be visible in the
	// output file. The field is set here for completeness and in case a future
	// version of pdfcpu respects it.
	if file != nil && file.ReleaseDate != nil {
		props["CreationDate"] = file.ReleaseDate.UTC().Format("D:20060102150405Z")
	}

	// Language — set in info dict if available.
	if file != nil && file.Language != nil && *file.Language != "" {
		props["Language"] = *file.Language
	}

	return props
}
