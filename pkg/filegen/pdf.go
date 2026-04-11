package filegen

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdf"
)

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
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	if err := api.AddPropertiesFile(srcPath, destPath, properties, conf); err != nil {
		return NewGenerationError(models.FileTypePDF, err, "failed to write PDF metadata")
	}

	// Write chapters back to the destination as a bookmark outline. Skipped when
	// the file has no chapters in the DB — we don't touch existing bookmarks in
	// the source PDF in that case.
	if file != nil && len(file.Chapters) > 0 {
		pageCount := 0
		if file.PageCount != nil {
			pageCount = *file.PageCount
		}
		bookmarks := convertModelChaptersToPDFBookmarks(file.Chapters, pageCount)
		if len(bookmarks) > 0 {
			// AddBookmarksFile needs distinct input and output paths. Write to a
			// sibling tmp file and rename over destPath on success.
			tmpPath := destPath + ".bookmarks.tmp"
			if err := api.AddBookmarksFile(destPath, tmpPath, bookmarks, true, conf); err != nil {
				_ = os.Remove(tmpPath)
				return NewGenerationError(models.FileTypePDF, err, "failed to write PDF bookmarks")
			}
			if err := os.Rename(tmpPath, destPath); err != nil {
				_ = os.Remove(tmpPath)
				return NewGenerationError(models.FileTypePDF, err, "failed to finalize PDF with bookmarks")
			}
		}
	}

	return nil
}

// convertModelChaptersToPDFBookmarks converts database chapter models into
// pdfcpu bookmark entries, preserving nesting. Chapters with nil StartPage or
// a StartPage outside [0, pageCount) are dropped. Pages are converted from the
// 0-indexed storage format to the 1-indexed form pdfcpu expects. A pageCount
// of 0 disables range filtering (used when the file's PageCount is unknown).
func convertModelChaptersToPDFBookmarks(chapters []*models.Chapter, pageCount int) []pdfcpu.Bookmark {
	if len(chapters) == 0 {
		return nil
	}

	sorted := make([]*models.Chapter, len(chapters))
	copy(sorted, chapters)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SortOrder < sorted[j].SortOrder
	})

	result := make([]pdfcpu.Bookmark, 0, len(sorted))
	for _, ch := range sorted {
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
		bm := pdfcpu.Bookmark{
			Title:    ch.Title,
			PageFrom: page + 1, // pdfcpu is 1-indexed
		}
		if len(ch.Children) > 0 {
			bm.Kids = convertModelChaptersToPDFBookmarks(ch.Children, pageCount)
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
