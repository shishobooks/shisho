package filegen

import (
	"bufio"
	"context"
	"os"
	"sort"

	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
)

// MOBIGenerator generates MOBI and AZW3 files with the book's metadata and
// cover written into their EXTH blocks. Series, tags, roles, and narrators
// have no place in the format and are not written; series is never folded
// into the title.
type MOBIGenerator struct {
	fileType string
}

// SupportedType returns the file type this generator handles.
func (g *MOBIGenerator) SupportedType() string {
	return g.fileType
}

// Generate writes a copy of the MOBI or AZW3 file at srcPath to destPath
// with updated metadata. The source file is never modified.
func (g *MOBIGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	fileType := g.fileType
	if err := ctx.Err(); err != nil {
		return NewGenerationError(fileType, err, "context cancelled")
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return NewGenerationError(fileType, err, "failed to open source file")
	}
	defer src.Close()
	stat, err := src.Stat()
	if err != nil {
		return NewGenerationError(fileType, err, "failed to stat source file")
	}

	meta, err := mobiMetadata(book, file)
	if err != nil {
		return NewGenerationError(fileType, err, "failed to read cover image")
	}
	if err := ctx.Err(); err != nil {
		return NewGenerationError(fileType, err, "context cancelled")
	}

	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return NewGenerationError(fileType, err, "failed to create destination file")
	}
	w := bufio.NewWriter(dest)
	err = mobi.Rewrite(src, stat.Size(), w, meta)
	if err == nil {
		err = w.Flush()
	}
	if closeErr := dest.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(destPath)
		return NewGenerationError(fileType, err, "failed to write MOBI metadata")
	}
	return nil
}

// mobiMetadata collects what a MOBI file can hold. Like the EPUB generator,
// it leaves the file's own value where Shisho has none.
func mobiMetadata(book *models.Book, file *models.File) (*mobi.Metadata, error) {
	meta := &mobi.Metadata{Title: book.Title}
	if file.Name != nil && *file.Name != "" {
		meta.Title = *file.Name
	}

	authors := make([]*models.Author, len(book.Authors))
	copy(authors, book.Authors)
	sort.SliceStable(authors, func(i, j int) bool { return authors[i].SortOrder < authors[j].SortOrder })
	for _, a := range authors {
		if a.Person != nil && a.Person.Name != "" {
			meta.Authors = append(meta.Authors, a.Person.Name)
		}
	}

	if book.Description != nil {
		meta.Description = *book.Description
	}
	if file.Publisher != nil {
		meta.Publisher = file.Publisher.Name
	}
	meta.ReleaseDate = file.ReleaseDate
	if file.Language != nil {
		meta.Language = *file.Language
	}
	for _, bg := range book.BookGenres {
		if bg.Genre != nil && bg.Genre.Name != "" {
			meta.Genres = append(meta.Genres, bg.Genre.Name)
		}
	}

	if len(file.Identifiers) > 0 {
		ids := &mobi.Identifiers{}
		for _, id := range file.Identifiers {
			switch identifiers.Type(id.Type) {
			case identifiers.TypeISBN10, identifiers.TypeISBN13:
				ids.ISBNs = append(ids.ISBNs, id.Value)
			case identifiers.TypeASIN:
				if ids.ASIN == "" {
					ids.ASIN = id.Value
				}
			case identifiers.TypeUUID, identifiers.TypeGoodreads, identifiers.TypeGoogle,
				identifiers.TypeOther, identifiers.TypeUnknown:
				// MOBI has no record for these.
			}
		}
		meta.Identifiers = ids
	}

	if file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
		data, err := os.ReadFile(covers.FileCoverPath(file))
		switch {
		case os.IsNotExist(err):
			// A cover missing from disk keeps the file's own cover, as the
			// EPUB and M4B generators do; any other read failure is a fault.
		case err != nil:
			return nil, err
		default:
			meta.Cover = data
		}
	}
	return meta, nil
}
