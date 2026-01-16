package downloadcache

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/shishobooks/shisho/pkg/models"
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

// invalidFilenameChars contains characters that are not allowed in filenames
// across Windows, macOS, and Linux.
var invalidFilenameChars = []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}

// FormatDownloadFilename generates a formatted filename for downloading a file.
// Format: [Author] Series #Number - Title.ext
// For audiobooks with narrators: [Author] Series #Number - Title {Narrator}.ext
// If no series: [Author] Title.ext.
// If no author: Title.ext.
func FormatDownloadFilename(book *models.Book, file *models.File) string {
	// Use file.Name if available, otherwise fall back to book.Title
	titleSource := book.Title
	if file.Name != nil && *file.Name != "" {
		titleSource = *file.Name
	}
	// Pad volume numbers for lexicographic sorting, then sanitize
	title := sanitizeFilename(padVolumeNumber(titleSource))
	author := getFirstAuthorName(book)
	series, number := getFirstSeries(book)
	narrator := getFirstNarratorName(file)
	ext := file.FileType

	var parts []string

	// Add author if available
	if author != "" {
		parts = append(parts, fmt.Sprintf("[%s]", sanitizeFilename(author)))
	}

	// Add series and number if available, unless title already has a volume number
	titleHasVolume := volumePattern.MatchString(titleSource)
	if series != "" && !titleHasVolume {
		seriesPart := sanitizeFilename(series)
		if number != nil {
			seriesPart += " #" + formatSeriesNumber(*number)
		}
		parts = append(parts, seriesPart)
		parts = append(parts, "-")
	}

	// Add title
	parts = append(parts, title)

	// Add narrator for audiobooks if available
	if narrator != "" {
		parts = append(parts, fmt.Sprintf("{%s}", sanitizeFilename(narrator)))
	}

	// Join parts with spaces and add extension
	filename := strings.Join(parts, " ")
	return filename + "." + ext
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

// getFirstSeries returns the name and number of the first series by sort order.
// Returns empty string and nil if no series.
func getFirstSeries(book *models.Book) (string, *float64) {
	if len(book.BookSeries) == 0 {
		return "", nil
	}

	// Sort series by SortOrder
	series := make([]*models.BookSeries, len(book.BookSeries))
	copy(series, book.BookSeries)
	sort.Slice(series, func(i, j int) bool {
		return series[i].SortOrder < series[j].SortOrder
	})

	first := series[0]
	if first.Series != nil {
		return first.Series.Name, first.SeriesNumber
	}
	return "", nil
}

// formatSeriesNumber formats a series number for display.
// Whole numbers are displayed without decimal (e.g., "1").
// Non-whole numbers keep their decimal (e.g., "1.5").
func formatSeriesNumber(n float64) string {
	if n == math.Floor(n) {
		return strconv.Itoa(int(n))
	}
	return fmt.Sprintf("%g", n)
}

// sanitizeFilename removes or replaces characters that are not valid in filenames.
func sanitizeFilename(s string) string {
	result := s
	for _, char := range invalidFilenameChars {
		result = strings.ReplaceAll(result, char, "")
	}
	// Also trim leading/trailing whitespace and collapse multiple spaces
	result = strings.TrimSpace(result)
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	return result
}

// koboUnsafeChars contains characters that cause problems with Kobo e-readers.
// Based on https://github.com/kobolabs/epub-spec which states:
// "File names containing non-alphanumeric characters are not fully supported,
// and their use may lead to undefined behaviour."
// Additionally, colons are known to prevent Kobo from seeing kepub files:
// https://github.com/seblucas/cops/issues/263
var koboUnsafeChars = regexp.MustCompile(`[^a-zA-Z0-9\s\-_.,()']`)

// sanitizeKoboFilename aggressively sanitizes a filename for Kobo compatibility.
// It removes special characters that can cause Kobo to fail reading the file.
func sanitizeKoboFilename(s string) string {
	// Replace unsafe characters with nothing
	result := koboUnsafeChars.ReplaceAllString(s, "")
	// Trim leading/trailing whitespace and collapse multiple spaces
	result = strings.TrimSpace(result)
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	return result
}

// FormatKepubDownloadFilename generates a formatted filename for downloading a KePub file.
// Uses Kobo-safe characters only to ensure compatibility with Kobo e-readers.
// Format: Author - Series Number - Title.kepub.epub (no brackets, no hash symbols).
func FormatKepubDownloadFilename(book *models.Book, file *models.File) string {
	// Use file.Name if available, otherwise fall back to book.Title
	titleSource := book.Title
	if file.Name != nil && *file.Name != "" {
		titleSource = *file.Name
	}
	// Pad volume numbers for lexicographic sorting, then sanitize for Kobo
	title := sanitizeKoboFilename(padVolumeNumber(titleSource))
	author := sanitizeKoboFilename(getFirstAuthorName(book))
	series, number := getFirstSeries(book)

	var parts []string

	// Add author if available (no brackets - Kobo doesn't like them)
	if author != "" {
		parts = append(parts, author)
		parts = append(parts, "-")
	}

	// Add series and number if available, unless title already has a volume number
	// Use plain number format instead of # (Kobo doesn't like #)
	titleHasVolume := volumePattern.MatchString(titleSource)
	if series != "" && !titleHasVolume {
		seriesPart := sanitizeKoboFilename(series)
		if number != nil {
			seriesPart += " " + formatSeriesNumber(*number)
		}
		parts = append(parts, seriesPart)
		parts = append(parts, "-")
	}

	// Add title
	parts = append(parts, title)

	// Skip narrator for KePub (CBZ converted to KePub won't have narrators anyway,
	// and curly braces are problematic for Kobo)

	// Join parts with spaces and add .kepub.epub extension
	filename := strings.Join(parts, " ")
	return filename + ".kepub.epub"
}
