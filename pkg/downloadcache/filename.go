package downloadcache

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/shishobooks/shisho/pkg/filenames"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/seriesnum"
)

// volumePattern matches volume indicators in titles (e.g., "v1", "V2", "vol. 3").
var volumePattern = regexp.MustCompile(`(?i)\bv(?:ol\.?)?\s*(\d+)`)

// volumeNumberPattern extracts just the number from a volume match for replacement.
var volumeNumberPattern = regexp.MustCompile(`(\d+)$`)

// padVolumeNumber pads volume numbers in titles to at least 3 digits for lexicographic sorting.
// e.g., "Manga v1" becomes "Manga v001", "Manga vol. 10" becomes "Manga vol. 010".
func padVolumeNumber(title string) string {
	return volumePattern.ReplaceAllStringFunc(title, func(match string) string {
		// Find the number at the end of the match
		return volumeNumberPattern.ReplaceAllStringFunc(match, func(numStr string) string {
			if len(numStr) < 3 {
				return fmt.Sprintf("%03s", numStr)
			}
			return numStr
		})
	})
}

// padSeriesNumber formats a series number with its whole part padded to at
// least three digits, as padVolumeNumber pads a title's volume, so names
// sort in series order: 2 becomes "002" and 1.5 "001.5".
func padSeriesNumber(n float64) string {
	if n == 0 {
		// Negative zero formats as "-0".
		n = 0
	}
	formatted := seriesnum.FormatRange(n, nil)
	if n < 0 {
		return formatted
	}
	whole, fraction, _ := strings.Cut(formatted, ".")
	if len(whole) < 3 {
		whole = strings.Repeat("0", 3-len(whole)) + whole
	}
	if fraction != "" {
		return whole + "." + fraction
	}
	return whole
}

// DownloadFilenames returns the name a file delivered as deliveredType
// downloads under. Every delivered File Type has the same shape:
//
//	Author - Series 002 - Title.ext
//	Author - Series 002 - Title (Narrator).m4b
//
// A part that is empty after sanitizing is left out, and a name too long
// for a filesystem is shortened. The series part is left
// out when the title carries its own volume marker, unless the book is an
// omnibus, whose range the title cannot show. Every part goes through
// filenames.Clean, and a KePub's through filenames.CleanASCII. A
// title that sanitizes to nothing, such as a Japanese title in a KePub,
// becomes "File <file ID>", so the name is never empty and two files of one
// book never share it.
//
// It also returns the same name in ASCII alone, for the clients that ignore
// a UTF-8 one. That name is built from the same parts with the KePub
// character rules, so it matches a KePub's name and falls back to
// "File <file ID>" the same way.
//
// The name is only the Content-Disposition name: the download cache keys its
// files on the file ID, delivered File Type, and fingerprint, so changing the
// name regenerates nothing.
func DownloadFilenames(book *models.Book, file *models.File, deliveredType string) (name, ascii string) {
	ascii = buildDownloadFilename(book, file, deliveredType, filenames.CleanASCII)
	if deliveredType == models.FileTypeKePub {
		return ascii, ascii
	}
	return buildDownloadFilename(book, file, deliveredType, filenames.Clean), ascii
}

// OriginalFilenames returns the name an Original File downloads under, and
// the same name in ASCII alone, the way DownloadFilenames does, with ext
// (including its dot) as the extension. A main File's name has the same
// parts as its Generated File's. A supplement's is its book's name with the
// supplement's own name after it, its Name or else its filename, so a
// book's files sort together:
//
//	Author - Series 002 - Title - Supplement.pdf
//
// A supplement name that sanitizes to nothing becomes "File <file ID>".
func OriginalFilenames(book *models.Book, file *models.File, ext string) (name, ascii string) {
	parts := filenameParts{titleSource: file.ResolveTitle(book.Title), narrators: true}
	if file.FileRole == models.FileRoleSupplement {
		parts = filenameParts{titleSource: book.Title, supplement: supplementName(book, file)}
	}
	ascii = buildFilename(book, file, parts, ext, filenames.CleanASCII)
	if file.FileType == models.FileTypeKePub {
		// A KePub's name is Kobo-safe, as a Generated one's is.
		return ascii, ascii
	}
	return buildFilename(book, file, parts, ext, filenames.Clean), ascii
}

// supplementName returns a supplement's own part of its download name: the
// name the user set, or else its filename without the extension, as
// File.ResolveDisplayName labels it. A filename that starts with the book's
// title, as a supplement beside the book's file at a library's root does,
// loses it, since the name already starts with the title.
func supplementName(book *models.Book, file *models.File) string {
	if file.HasName() && file.NameSource != nil && *file.NameSource == models.DataSourceManual {
		return *file.Name
	}
	base := filepath.Base(file.Filepath)
	stem := strings.TrimSuffix(base, models.FileExtensionForPath(base))
	if title := strings.TrimSpace(book.Title); title != "" && len(stem) > len(title) && strings.EqualFold(stem[:len(title)], title) && strings.ContainsRune(" -_.", rune(stem[len(title)])) {
		if rest := strings.TrimLeft(stem[len(title):], " -_."); rest != "" {
			return rest
		}
	}
	return stem
}

// filenameParts is what a download name is built from beyond the book's
// author and series.
type filenameParts struct {
	// titleSource is the title before sanitizing.
	titleSource string
	// supplement is a supplement's own name, or "" for a main File.
	supplement string
	// narrators adds the File's first narrator.
	narrators bool
}

// buildDownloadFilename builds a Generated File's download name with clean
// applied to each part.
func buildDownloadFilename(book *models.Book, file *models.File, deliveredType string, clean func(string) string) string {
	parts := filenameParts{titleSource: file.ResolveTitle(book.Title), narrators: true}
	return buildFilename(book, file, parts, "."+models.DeliveredFileTypeExtension(deliveredType), clean)
}

// buildFilename builds a download name from parts with clean applied to
// each, and ext, including its dot, as the extension.
func buildFilename(book *models.Book, file *models.File, parts filenameParts, ext string, clean func(string) string) string {
	titleSource := parts.titleSource
	// Pad volume numbers for lexicographic sorting, then sanitize.
	title := clean(padVolumeNumber(titleSource))
	supplement := ""
	if parts.supplement != "" || file.FileRole == models.FileRoleSupplement {
		supplement = clean(parts.supplement)
		if supplement == "" {
			supplement = fmt.Sprintf("File %d", file.ID)
		}
	} else if title == "" {
		title = fmt.Sprintf("File %d", file.ID)
	}

	author := clean(getFirstAuthorName(book))
	narrator := ""
	if parts.narrators {
		narrator = clean(getFirstNarratorName(file))
	}

	// A title volume marker suppresses a redundant single position, but an
	// omnibus still needs its complete range represented in the filename.
	seriesName, number, numberEnd := getFirstSeries(book)
	seriesName = clean(seriesName)
	titleHasVolume := volumePattern.MatchString(titleSource)
	if seriesName == "" || (titleHasVolume && numberEnd == nil) {
		seriesName = ""
	} else if number != nil {
		seriesName += " " + padSeriesNumber(*number)
		if numberEnd != nil {
			seriesName += "-" + padSeriesNumber(*numberEnd)
		}
	}

	ext = clean(ext)
	stem := func() string {
		var parts []string
		for _, part := range []string{author, seriesName, title, supplement} {
			if part != "" {
				parts = append(parts, part)
			}
		}
		name := strings.Join(parts, " - ")
		if narrator != "" {
			name += " (" + narrator + ")"
		}
		return name
	}
	// Shorten a name too long for a filesystem: the title and a
	// supplement's name down to short stubs first, then the series, author,
	// and narrator, and then the title and supplement name the rest of the
	// way.
	budget := maxFilenameBytes - len(ext)
	for _, cut := range []struct {
		part *string
		keep int
	}{{&title, minTitleBytes}, {&supplement, minTitleBytes}, {&seriesName, 0}, {&author, 0}, {&narrator, 0}, {&title, 1}, {&supplement, 1}} {
		if over := len(stem()) - budget; over > 0 {
			*cut.part = truncateBytes(*cut.part, max(len(*cut.part)-over, cut.keep))
		}
	}

	name := strings.TrimLeft(stem(), ". ")
	if name == "" {
		name = fmt.Sprintf("File %d", file.ID)
	}
	if windowsReservedNames[strings.ToUpper(name)] {
		name = "_" + name
	}
	return name + ext
}

const (
	// maxFilenameBytes keeps a download name under the 255-byte limit of
	// common filesystems, with room for a client's " (1)" suffix.
	maxFilenameBytes = 240
	// minTitleBytes is how much of the title shortening keeps before it
	// cuts the other parts.
	minTitleBytes = 48
)

// truncateBytes shortens s to at most n bytes at a character boundary, and
// trims the spaces and punctuation a cut leaves at its end.
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return strings.TrimRight(s[:n], " -,.")
}

// windowsReservedNames are the names Windows reserves for devices. A file
// named one of them, with any extension, cannot be saved there.
var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// getFirstAuthorName returns the name of the first author by sort order.
// Returns empty string if no authors.
func getFirstAuthorName(book *models.Book) string {
	if len(book.Authors) == 0 {
		return ""
	}

	// Sort authors by SortOrder
	authors := make([]*models.Author, len(book.Authors))
	copy(authors, book.Authors)
	sort.Slice(authors, func(i, j int) bool {
		return authors[i].SortOrder < authors[j].SortOrder
	})

	first := authors[0]
	if first.Person != nil {
		return first.Person.Name
	}
	return ""
}

// getFirstNarratorName returns the name of the first narrator by sort order.
// Returns empty string if no narrators.
func getFirstNarratorName(file *models.File) string {
	if len(file.Narrators) == 0 {
		return ""
	}

	// Sort narrators by SortOrder
	narrators := make([]*models.Narrator, len(file.Narrators))
	copy(narrators, file.Narrators)
	sort.Slice(narrators, func(i, j int) bool {
		return narrators[i].SortOrder < narrators[j].SortOrder
	})

	first := narrators[0]
	if first.Person != nil {
		return first.Person.Name
	}
	return ""
}

// getFirstSeries returns the name and number range of the first series by sort order.
// Returns an empty name and nil numbers if there is no series.
func getFirstSeries(book *models.Book) (string, *float64, *float64) {
	if len(book.BookSeries) == 0 {
		return "", nil, nil
	}

	// Sort series by SortOrder
	series := make([]*models.BookSeries, len(book.BookSeries))
	copy(series, book.BookSeries)
	sort.Slice(series, func(i, j int) bool {
		return series[i].SortOrder < series[j].SortOrder
	})

	first := series[0]
	if first.Series != nil {
		return first.Series.Name, first.SeriesNumber, first.SeriesNumberEnd
	}
	return "", nil, nil
}
