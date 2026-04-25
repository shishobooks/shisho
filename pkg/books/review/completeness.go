package review

import (
	"github.com/shishobooks/shisho/pkg/models"
)

// MissingFields returns the list of required-field names that are not satisfied
// for a single main file. The book and file must have all relations loaded.
//
// For supplements, returns nil.
//
// The criteria's BookFields apply to every file; AudioFields apply when the
// file is m4b.
func MissingFields(book *models.Book, file *models.File, criteria Criteria) []string {
	if file.FileRole == models.FileRoleSupplement {
		return nil
	}
	missing := make([]string, 0)
	for _, f := range criteria.BookFields {
		if !isPresent(book, file, f) {
			missing = append(missing, f)
		}
	}
	if file.FileType == models.FileTypeM4B {
		for _, f := range criteria.AudioFields {
			if !isPresent(book, file, f) {
				missing = append(missing, f)
			}
		}
	}
	return missing
}

// IsComplete returns true iff MissingFields returns an empty slice for a main file.
func IsComplete(book *models.Book, file *models.File, criteria Criteria) bool {
	return len(MissingFields(book, file, criteria)) == 0
}

func isPresent(book *models.Book, file *models.File, field string) bool {
	switch field {
	case FieldAuthors:
		return len(book.Authors) > 0
	case FieldDescription:
		return book.Description != nil && *book.Description != ""
	case FieldGenres:
		return len(book.BookGenres) > 0
	case FieldTags:
		return len(book.BookTags) > 0
	case FieldSeries:
		return len(book.BookSeries) > 0
	case FieldSubtitle:
		return book.Subtitle != nil && *book.Subtitle != ""
	case FieldCover:
		return file.CoverImageFilename != nil && *file.CoverImageFilename != ""
	case FieldPublisher:
		return file.PublisherID != nil
	case FieldImprint:
		return file.ImprintID != nil
	case FieldIdentifiers:
		return len(file.Identifiers) > 0
	case FieldReleaseDate:
		return file.ReleaseDate != nil
	case FieldLanguage:
		return file.Language != nil && *file.Language != ""
	case FieldURL:
		return file.URL != nil && *file.URL != ""
	case FieldNarrators:
		return len(file.Narrators) > 0
	case FieldChapters:
		return len(file.Chapters) > 0
	case FieldAbridged:
		return file.Abridged != nil
	}
	return false
}
