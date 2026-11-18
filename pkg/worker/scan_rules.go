package worker

import (
	"slices"
	"strings"

	"github.com/shishobooks/shisho/pkg/htmlutil"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
)

// scanRules is how a scan settles a field between the value it found and the
// value stored. Lower source priority wins, and a tie goes to the newer value,
// with the exceptions below (ADR 0027).
type scanRules struct {
	// forceRefresh bypasses source priority (Refresh all metadata, Reset).
	forceRefresh bool
	// keepPluginValues keeps a stored plugin value when the scan's value also
	// comes from a plugin, instead of letting the newer one win the tie. It is
	// set when the record was in the library before this scan, so an
	// ordinary scan never swaps one plugin's value, or one Identify match, for
	// another. Refresh all metadata and Identify still can.
	keepPluginValues bool
	// parserSource is the source of the file parser that read the file. A
	// plugin file parser's values are the file's own metadata, so they rank
	// as file metadata and an enricher can replace them.
	parserSource string
}

// fileScanState is what a scan knows about a file and its Book before it
// settles fields.
type fileScanState struct {
	// bookKnown: the Book was in the library before this scan, so its stored
	// values and Sidecar came from Shisho.
	bookKnown bool
	// fileKnown: the File was in the library before this scan (FileID mode).
	fileKnown bool
	// parserSource is the source the file's parser stamped on its values.
	parserSource string
}

// rules returns the rules for the Book's fields and for the File's fields.
func (s fileScanState) rules(forceRefresh bool) (book, file scanRules) {
	book = scanRules{forceRefresh: forceRefresh, keepPluginValues: !forceRefresh && s.bookKnown, parserSource: s.parserSource}
	file = scanRules{forceRefresh: forceRefresh, keepPluginValues: !forceRefresh && s.fileKnown, parserSource: s.parserSource}
	return book, file
}

// priority is a source's priority under these rules: a plugin file parser's
// source ranks as file metadata.
func (r scanRules) priority(source string) int {
	if source == "" {
		source = models.DataSourceFilepath
	}
	if r.parserSource != "" && source == r.parserSource {
		return models.DataSourceFileMetadataPriority
	}
	return models.GetDataSourcePriority(source)
}

// keepsStoredPluginValue reports whether rules keep a stored plugin value
// against a new plugin value of the same priority.
func (r scanRules) keepsStoredPluginValue(newPriority, existingPriority int) bool {
	return r.keepPluginValues &&
		newPriority == models.DataSourcePluginPriority &&
		existingPriority == models.DataSourcePluginPriority
}

// keepsStoredCover reports whether a stored cover must stay rather than be
// replaced by a plugin's: one the user chose, a sidecar's, or, under
// keepPluginValues, one a plugin set. A cover from the file can be upgraded.
func (r scanRules) keepsStoredCover(existingSource string) bool {
	if r.forceRefresh {
		return false
	}
	p := r.priority(existingSource)
	return p < models.DataSourcePluginPriority ||
		(p == models.DataSourcePluginPriority && r.keepPluginValues)
}

// dropSidecarCopies removes from the sidecars every field that holds the
// value stored before this scan. Shisho rewrites a Book's sidecars after each
// scan and edit, so such a field is only Shisho's own copy of the database,
// not metadata from elsewhere, and it must not outrank a plugin the way a
// sidecar carried over from another library or edited on disk does. Fields
// that differ are left for the sidecar rules to apply as before.
//
// book and file are the stored records with their relations loaded. A
// relation that isn't loaded reads as empty, so its sidecar field differs and
// is kept, as before. Chapters and the cover page are left alone: their own
// rules read the sidecar, and the cover page follows keepsStoredPluginValue
// where the scan applies a plugin's page.
func dropSidecarCopies(bookSidecar *sidecar.BookSidecar, fileSidecar *sidecar.FileSidecar, book *models.Book, file *models.File) {
	if bookSidecar != nil && book != nil {
		dropBookSidecarCopies(bookSidecar, book)
	}
	if fileSidecar != nil && file != nil {
		dropFileSidecarCopies(fileSidecar, file)
	}
}

func dropBookSidecarCopies(s *sidecar.BookSidecar, book *models.Book) {
	if s.Title == book.Title {
		s.Title = ""
	}
	if s.SortTitle == book.SortTitle {
		s.SortTitle = ""
	}
	if s.Subtitle != nil && strings.TrimSpace(*s.Subtitle) == deref(book.Subtitle) {
		s.Subtitle = nil
	}
	if s.Description != nil && htmlutil.StripTags(strings.TrimSpace(*s.Description)) == deref(book.Description) {
		s.Description = nil
	}

	authors := make([]string, 0, len(s.Authors))
	for _, a := range s.Authors {
		authors = append(authors, a.Name)
	}
	storedAuthors := make([]string, 0, len(book.Authors))
	for _, a := range book.Authors {
		if a.Person != nil {
			storedAuthors = append(storedAuthors, a.Person.Name)
		}
	}
	if len(s.Authors) > 0 && slices.Equal(authors, storedAuthors) {
		s.Authors = nil
	}

	if len(s.Series) > 0 && seriesSidecarMatches(s.Series, book.BookSeries) {
		s.Series = nil
	}

	storedGenres := make([]string, 0, len(book.BookGenres))
	for _, bg := range book.BookGenres {
		if bg.Genre != nil {
			storedGenres = append(storedGenres, bg.Genre.Name)
		}
	}
	if len(s.Genres) > 0 && sameNameSet(s.Genres, storedGenres) {
		s.Genres = nil
	}

	storedTags := make([]string, 0, len(book.BookTags))
	for _, bt := range book.BookTags {
		if bt.Tag != nil {
			storedTags = append(storedTags, bt.Tag.Name)
		}
	}
	if len(s.Tags) > 0 && sameNameSet(s.Tags, storedTags) {
		s.Tags = nil
	}
}

func dropFileSidecarCopies(s *sidecar.FileSidecar, file *models.File) {
	stored := sidecar.FileSidecarFromModel(file)

	narrators := make([]string, 0, len(s.Narrators))
	for _, n := range s.Narrators {
		narrators = append(narrators, n.Name)
	}
	storedNarrators := make([]string, 0, len(stored.Narrators))
	for _, n := range stored.Narrators {
		storedNarrators = append(storedNarrators, n.Name)
	}
	if len(s.Narrators) > 0 && slices.Equal(narrators, storedNarrators) {
		s.Narrators = nil
	}

	if equalStringPointers(s.URL, stored.URL) {
		s.URL = nil
	}
	if equalStringPointers(s.Publisher, stored.Publisher) {
		s.Publisher = nil
	}
	if equalStringPointers(s.ReleaseDate, stored.ReleaseDate) {
		s.ReleaseDate = nil
	}
	if equalStringPointers(s.Name, stored.Name) {
		s.Name = nil
	}
	if equalStringPointers(s.Language, stored.Language) {
		s.Language = nil
	}
	if equalStringPointers(s.ReadingDirection, stored.ReadingDirection) {
		s.ReadingDirection = nil
	}
	if s.Abridged != nil && stored.Abridged != nil && *s.Abridged == *stored.Abridged {
		s.Abridged = nil
	}

	if len(s.Identifiers) > 0 {
		keys := make([]string, 0, len(s.Identifiers))
		for _, id := range s.Identifiers {
			keys = append(keys, identifiers.Key(id.Type, id.Value))
		}
		if sameNameSet(keys, fileIdentifierKeys(file.Identifiers)) {
			s.Identifiers = nil
		}
	}
}

// sameNameSet reports whether a and b hold the same names, in any order.
func sameNameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
