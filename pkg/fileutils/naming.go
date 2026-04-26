package fileutils

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/shishobooks/shisho/pkg/models"
)

// OrganizedNameOptions contains the data needed to generate organized file/folder names.
type OrganizedNameOptions struct {
	AuthorNames      []string // Author names as strings for file naming
	NarratorNames    []string // Narrator names for M4B file naming
	Title            string
	SeriesNumber     *float64
	SeriesNumberUnit *string // for CBZ: models.SeriesNumberUnitVolume or models.SeriesNumberUnitChapter; nil treated as volume
	FileType         string  // for determining number formatting
}

// GenerateOrganizedFolderName creates a standardized folder name: [Author] Title #Volume.
func GenerateOrganizedFolderName(opts OrganizedNameOptions) string {
	var parts []string

	// Add author in brackets if available
	if len(opts.AuthorNames) > 0 && opts.AuthorNames[0] != "" {
		author := sanitizeForFilename(opts.AuthorNames[0])
		parts = append(parts, fmt.Sprintf("[%s]", author))
	}

	// Add title
	if opts.Title != "" {
		title := sanitizeForFilename(opts.Title)
		parts = append(parts, title)
	}

	// Add series number only for CBZ files (manga/comic). But only if the title
	// doesn't already encode a number.
	if opts.SeriesNumber != nil && opts.FileType == models.FileTypeCBZ {
		existingNum, _ := extractSeriesNumberFromTitle(opts.Title)
		if existingNum == nil {
			unit := ""
			if opts.SeriesNumberUnit != nil {
				unit = *opts.SeriesNumberUnit
			}
			parts = append(parts, formatSeriesNumber(*opts.SeriesNumber, unit, opts.FileType))
		}
	}

	name := strings.Join(parts, " ")

	// Ensure we have at least something
	if name == "" {
		name = "Unknown"
	}

	return name
}

// GenerateOrganizedFileName creates a standardized filename: Title.ext.
// For M4B files, includes narrator in braces: Title {Narrator}.m4b.
// Author names are NOT included since files are already inside author-prefixed folders.
func GenerateOrganizedFileName(opts OrganizedNameOptions, originalFilepath string) string {
	ext := filepath.Ext(originalFilepath)

	// For organized files in folders, we don't include volume numbers or author names
	// in the filename since the folder already contains this information.
	// This prevents duplication like: "[Author] Book/[Author] Book.epub"
	// Instead we get: "[Author] Book/Book.epub"

	optsForFilename := opts
	optsForFilename.SeriesNumber = nil
	optsForFilename.AuthorNames = nil
	baseName := GenerateOrganizedFolderName(optsForFilename)

	// Add narrator in braces for M4B files
	if opts.FileType == models.FileTypeM4B && len(opts.NarratorNames) > 0 && opts.NarratorNames[0] != "" {
		narrator := sanitizeForFilename(opts.NarratorNames[0])
		baseName = fmt.Sprintf("%s {%s}", baseName, narrator)
	}

	return baseName + ext
}

// formatSeriesNumber formats a CBZ series number with the appropriate unit prefix:
// "v" for volume (and the empty-unit default), "c" for chapter. Non-CBZ files keep
// the legacy "#N" form, which is currently unused since this helper is only invoked
// for CBZ in GenerateOrganizedFolderName.
func formatSeriesNumber(number float64, unit string, fileType string) string {
	if fileType == models.FileTypeCBZ {
		prefix := "v"
		if unit == models.SeriesNumberUnitChapter {
			prefix = "c"
		}
		if number == float64(int(number)) {
			return fmt.Sprintf("%s%d", prefix, int(number))
		}
		return fmt.Sprintf("%s%.1f", prefix, number)
	}
	if number == float64(int(number)) {
		return fmt.Sprintf("#%d", int(number))
	}
	return fmt.Sprintf("#%.1f", number)
}

// sanitizeForFilename removes or replaces characters that are not safe for filenames.
func sanitizeForFilename(name string) string {
	// Remove/replace problematic characters
	// Replace various quotes and smart quotes with regular quotes
	name = regexp.MustCompile(`[""]`).ReplaceAllString(name, `"`)
	name = regexp.MustCompile(`['']`).ReplaceAllString(name, `'`)

	// Remove or replace characters that are invalid in filenames
	// Different operating systems have different restrictions, so we'll be conservative
	invalidChars := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	name = invalidChars.ReplaceAllString(name, "")

	// Replace multiple spaces with single space
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")

	// Trim spaces and dots from the ends (Windows doesn't like trailing dots)
	name = strings.Trim(name, " .")

	// Limit length to reasonable filesystem limits (255 is common, but we'll be conservative)
	if len(name) > 200 {
		name = name[:200]
		name = strings.Trim(name, " .")
	}

	return name
}

// IsOrganizedName checks if a filename/foldername follows the organized naming pattern.
func IsOrganizedName(name string) bool {
	// Remove extension for analysis
	nameWithoutExt := strings.TrimSuffix(name, filepath.Ext(name))

	// Basic pattern: starts with [Author] or contains series number indicators
	authorPattern := regexp.MustCompile(`^\[.+\]`)
	seriesNumberPattern := regexp.MustCompile(`([vc]\d+(?:\.\d+)?|#\d+(?:\.\d+)?)$`)

	return authorPattern.MatchString(nameWithoutExt) || seriesNumberPattern.MatchString(nameWithoutExt)
}

// NormalizeSeriesNumberInTitle normalizes volume- or chapter-style number indicators
// in CBZ titles. For volume indicators (v01, vol.5, volume 12, #001, bare trailing
// number) the title becomes "Title v{NNN}". For chapter indicators (chapter 5,
// Ch.5, c042) the title becomes "Title c{NNN}". Returns the normalized title,
// the parsed unit (models.SeriesNumberUnitVolume or models.SeriesNumberUnitChapter, "" when no match), and whether a number
// was found. Non-CBZ files are returned unchanged.
func NormalizeSeriesNumberInTitle(title string, fileType string) (string, string, bool) {
	if fileType != models.FileTypeCBZ {
		return title, "", false
	}

	// Pattern table: regex + unit. First match wins; explicit chapter patterns
	// precede explicit volume patterns; ambiguous indicators (#, bare numbers)
	// default to volume to preserve historical behavior.
	patterns := []struct {
		re   *regexp.Regexp
		unit string
	}{
		{regexp.MustCompile(`(?i)\s*chapter\s*(\d+(?:\.\d+)?)\s*$`), models.SeriesNumberUnitChapter},
		{regexp.MustCompile(`(?i)\s*ch\.?\s*(\d+(?:\.\d+)?)\s*$`), models.SeriesNumberUnitChapter},
		{regexp.MustCompile(`(?i)\s*c(\d+(?:\.\d+)?)\s*$`), models.SeriesNumberUnitChapter},
		{regexp.MustCompile(`(?i)\s*#(\d+(?:\.\d+)?)\s*$`), models.SeriesNumberUnitVolume},
		{regexp.MustCompile(`(?i)\s*v(\d+(?:\.\d+)?)\s*$`), models.SeriesNumberUnitVolume},
		{regexp.MustCompile(`(?i)\s*vol\.?\s*(\d+(?:\.\d+)?)\s*$`), models.SeriesNumberUnitVolume},
		{regexp.MustCompile(`(?i)\s*volume\s*(\d+(?:\.\d+)?)\s*$`), models.SeriesNumberUnitVolume},
		{regexp.MustCompile(`\s+(\d+(?:\.\d+)?)\s*$`), models.SeriesNumberUnitVolume},
	}

	for _, p := range patterns {
		matches := p.re.FindStringSubmatch(title)
		if len(matches) < 2 {
			continue
		}
		baseTitle := strings.TrimSpace(p.re.ReplaceAllString(title, ""))
		number, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			continue
		}
		prefix := "v"
		if p.unit == models.SeriesNumberUnitChapter {
			prefix = "c"
		}
		var normalized string
		if number == float64(int(number)) {
			normalized = fmt.Sprintf("%s %s%03d", baseTitle, prefix, int(number))
		} else {
			intPart := int(number)
			fracStr := strconv.FormatFloat(number-float64(intPart), 'f', -1, 64)
			// fracStr is "0.5"; strip the leading "0".
			normalized = fmt.Sprintf("%s %s%03d%s", baseTitle, prefix, intPart, fracStr[1:])
		}
		return strings.TrimSpace(normalized), p.unit, true
	}

	return title, "", false
}

// extractSeriesNumberFromTitle extracts a normalized series number suffix
// ("v003" or "c042") from a title. Returns the number and unit, or (nil, "")
// if no suffix is present.
func extractSeriesNumberFromTitle(title string) (*float64, string) {
	seriesNumberPattern := regexp.MustCompile(`\s+([vc])(\d+(?:\.\d+)?)\s*$`)
	matches := seriesNumberPattern.FindStringSubmatch(title)
	if len(matches) < 3 {
		return nil, ""
	}
	number, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return nil, ""
	}
	unit := models.SeriesNumberUnitVolume
	if strings.EqualFold(matches[1], "c") {
		unit = models.SeriesNumberUnitChapter
	}
	return &number, unit
}

// SplitNames splits a string of names by common delimiters (comma and semicolon),
// trims whitespace from each name, and returns non-empty names.
// This is used for parsing author and narrator lists from metadata.
func SplitNames(s string) []string {
	if s == "" {
		return nil
	}

	// Split by both comma and semicolon
	var parts []string
	for _, segment := range strings.Split(s, ";") {
		for _, part := range strings.Split(segment, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
	}
	return parts
}

// ExtractSeriesFromTitle extracts series name and number from a normalized CBZ title.
// Returns the base title (series name), number, unit (models.SeriesNumberUnitVolume or models.SeriesNumberUnitChapter), and
// whether extraction succeeded. Only applies to CBZ files with normalized "v{N}"
// or "c{N}" suffixes.
func ExtractSeriesFromTitle(title string, fileType string) (seriesName string, number *float64, unit string, ok bool) {
	if fileType != models.FileTypeCBZ {
		return "", nil, "", false
	}
	seriesNumberPattern := regexp.MustCompile(`^(.+?)\s+([vc])(\d+(?:\.\d+)?)\s*$`)
	matches := seriesNumberPattern.FindStringSubmatch(title)
	if len(matches) < 4 {
		return "", nil, "", false
	}
	seriesName = strings.TrimSpace(matches[1])
	if seriesName == "" {
		return "", nil, "", false
	}
	parsed, err := strconv.ParseFloat(matches[3], 64)
	if err != nil {
		return "", nil, "", false
	}
	parsedUnit := models.SeriesNumberUnitVolume
	if strings.EqualFold(matches[2], "c") {
		parsedUnit = models.SeriesNumberUnitChapter
	}
	return seriesName, &parsed, parsedUnit, true
}
