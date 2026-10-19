package filegen

import (
	"archive/zip"
	"context"
	"io"
	"sort"

	"github.com/shishobooks/shisho/pkg/fixedlayout"
	"github.com/shishobooks/shisho/pkg/models"
)

// cbzFixedLayout is the Conversion step from CBZ to a fixed-layout EPUB,
// one page per image. Its output is plain EPUB 3, so the CBZ to KePub and
// CBZ to EPUB steps both run it; fileType names the one delivered.
type cbzFixedLayout struct {
	fileType string
}

// prepare reads the source's zip directory and checks that every page can
// be read. Pages are read and processed only while writing. It reads the
// Original CBZ, so it can only be a chain's first step.
func (s cbzFixedLayout) prepare(_ context.Context, in stepInput) (Prepared, error) {
	if in.entries != nil {
		return nil, NewGenerationError(s.fileType, nil, "a CBZ Conversion must read the Original File")
	}
	srcZip, err := zip.NewReader(in.src, in.src.Size())
	if err != nil {
		return nil, NewGenerationError(s.fileType, err, "failed to read source CBZ as zip")
	}
	conversion, err := fixedlayout.PrepareCBZ(srcZip, buildCBZMetadata(in.book, in.file))
	if err != nil {
		return nil, NewGenerationError(s.fileType, err, "failed to convert CBZ: "+err.Error())
	}
	return &cbzFixedLayoutPrepared{fileType: s.fileType, conversion: conversion}, nil
}

// cbzFixedLayoutPrepared writes a fixed-layout EPUB converted from a CBZ.
type cbzFixedLayoutPrepared struct {
	fileType   string
	conversion *fixedlayout.Conversion
}

// Size reports no size: processed pages are only measured once written.
func (p *cbzFixedLayoutPrepared) Size() (int64, bool) {
	return 0, false
}

// Write writes the EPUB to w, about one page at a time.
func (p *cbzFixedLayoutPrepared) Write(ctx context.Context, w io.Writer) error {
	if err := p.conversion.Write(ctx, w); err != nil {
		return NewGenerationError(p.fileType, err, "failed to convert CBZ: "+err.Error())
	}
	return nil
}

// buildCBZMetadata creates the fixed-layout EPUB metadata from a book and file model.
func buildCBZMetadata(book *models.Book, file *models.File) *fixedlayout.Metadata {
	if book == nil {
		return nil
	}

	metadata := &fixedlayout.Metadata{
		Title:    file.ResolveTitle(book.Title),
		Subtitle: book.Subtitle,
	}
	if file != nil {
		metadata.Language = file.Language
	}

	// Add authors with their roles and sort names
	for _, author := range book.Authors {
		if author.Person != nil {
			role := ""
			if author.Role != nil {
				role = *author.Role
			}
			metadata.Authors = append(metadata.Authors, fixedlayout.Author{
				Name:     author.Person.Name,
				SortName: author.Person.SortName,
				Role:     role,
			})
		}
	}

	// Add series information
	for _, bs := range book.BookSeries {
		if bs.Series != nil {
			metadata.Series = append(metadata.Series, fixedlayout.Series{
				Name:   bs.Series.Name,
				Number: bs.SeriesNumber,
			})
		}
	}

	// Add genres
	for _, bg := range book.BookGenres {
		if bg.Genre != nil {
			metadata.Genres = append(metadata.Genres, bg.Genre.Name)
		}
	}

	// Add tags
	for _, bt := range book.BookTags {
		if bt.Tag != nil {
			metadata.Tags = append(metadata.Tags, bt.Tag.Name)
		}
	}

	// Add chapters (top-level only, no children)
	if file != nil && len(file.Chapters) > 0 {
		metadata.Chapters = convertModelChaptersToCBZ(file.Chapters)
	}

	// Set cover page if available
	if file != nil && file.CoverPage != nil {
		metadata.CoverPage = file.CoverPage
	}

	return metadata
}

// convertModelChaptersToCBZ converts model chapters to CBZ chapters.
// Only top-level chapters are included (children are flattened/ignored).
//
// Sorts by StartPage so the generated EPUB's chapter navigation is in page
// order even if the DB's SortOrder disagrees. Ties break on SortOrder for
// stability. Chapters with nil StartPage are treated as page 0.
func convertModelChaptersToCBZ(chapters []*models.Chapter) []fixedlayout.Chapter {
	// Copy to avoid modifying the original slice
	sorted := make([]*models.Chapter, len(chapters))
	copy(sorted, chapters)

	// Sort by StartPage (nil == 0), fall back to SortOrder for ties.
	sort.SliceStable(sorted, func(i, j int) bool {
		ai := 0
		if sorted[i].StartPage != nil {
			ai = *sorted[i].StartPage
		}
		aj := 0
		if sorted[j].StartPage != nil {
			aj = *sorted[j].StartPage
		}
		if ai != aj {
			return ai < aj
		}
		return sorted[i].SortOrder < sorted[j].SortOrder
	})

	result := make([]fixedlayout.Chapter, 0, len(sorted))
	for _, ch := range sorted {
		startPage := 0
		if ch.StartPage != nil {
			startPage = *ch.StartPage
		}
		result = append(result, fixedlayout.Chapter{
			Title:     ch.Title,
			StartPage: startPage,
		})
	}
	return result
}
