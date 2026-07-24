package fileutils

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/seriesnum"
)

// OrganizedNameOptions contains the data needed to generate organized file/folder names.
type OrganizedNameOptions struct {
	AuthorNames      []string // Author names as strings for file naming
	NarratorNames    []string // Narrator names for M4B file naming
	Title            string
	SeriesNumber     *float64
	SeriesNumberEnd  *float64
	SeriesNumberUnit *string // for CBZ: models.SeriesNumberUnitVolume or models.SeriesNumberUnitChapter; nil treated as volume
	FileType         string  // for determining number formatting
}

// GenerateOrganizedFolderName creates a standardized folder name: [Author] Title <number>.
// For CBZ files, the number is formatted as "v{N}" for volumes or "c{N}" for chapters.
func GenerateOrganizedFolderName(opts OrganizedNameOptions) string {
	var parts []string

	// Add author in brackets if available
	if len(opts.AuthorNames) > 0 && opts.AuthorNames[0] != "" {
		author := sanitizeForFilename(opts.AuthorNames[0])
		parts = append(parts, fmt.Sprintf("[%s]", author))
	}

	// Strip a normalized CBZ number from the title before applying the current
	// atomic number group. This lets edits to either endpoint update the name
	// instead of preserving a stale suffix.
	title := opts.Title
	if opts.SeriesNumber != nil && opts.FileType == models.FileTypeCBZ {
		if seriesName, _, _, _, ok := ExtractSeriesFromTitle(title, opts.FileType); ok {
			title = seriesName
		}
	}
	if title != "" {
		parts = append(parts, sanitizeForFilename(title))
	}

	if opts.SeriesNumber != nil && opts.FileType == models.FileTypeCBZ {
		unit := ""
		if opts.SeriesNumberUnit != nil {
			unit = *opts.SeriesNumberUnit
		}
		parts = append(parts, formatSeriesNumber(*opts.SeriesNumber, opts.SeriesNumberEnd, unit, opts.FileType))
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

	// For organized files in folders, we don't include series numbers or author names
	// in the filename since the folder already contains this information.
	// This prevents duplication like: "[Author] Book/[Author] Book.epub"
	// Instead we get: "[Author] Book/Book.epub"

	optsForFilename := opts
	optsForFilename.SeriesNumber = nil
	optsForFilename.SeriesNumberEnd = nil
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
func formatSeriesNumber(number float64, numberEnd *float64, unit string, fileType string) string {
	if fileType == models.FileTypeCBZ {
		prefix := "v"
		if unit == models.SeriesNumberUnitChapter {
			prefix = "c"
		}
		return prefix + formatPaddedSeriesRange(number, numberEnd)
	}
	return "#" + seriesnum.FormatRange(number, numberEnd)
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
	seriesNumberPattern := regexp.MustCompile(`([vc]\d+(?:\.\d+)?(?:-\d+(?:\.\d+)?)?|#\d+(?:\.\d+)?)$`)

	return authorPattern.MatchString(nameWithoutExt) || seriesNumberPattern.MatchString(nameWithoutExt)
}

// seriesNumberPatterns is the regex pattern table used by NormalizeSeriesNumberInTitle.
// Each entry pairs a compiled regexp with the unit it implies. First match wins;
// explicit chapter patterns precede explicit volume patterns; ambiguous indicators
// (#, bare numbers) default to volume to preserve historical behavior.
const (
	seriesNumberRE = `\d+(?:\.\d+)?`
	seriesRangeRE  = seriesNumberRE + `(?:\s*[-–—]\s*` + seriesNumberRE + `)?`
)

var seriesNumberPatterns = []struct {
	re   *regexp.Regexp
	unit string
}{
	{regexp.MustCompile(`(?i)\s*chapter\s*(` + seriesRangeRE + `)\s*$`), models.SeriesNumberUnitChapter},
	{regexp.MustCompile(`(?i)\s*ch\.?\s*(` + seriesRangeRE + `)\s*$`), models.SeriesNumberUnitChapter},
	{regexp.MustCompile(`(?i)\s+c(` + seriesRangeRE + `)\s*$`), models.SeriesNumberUnitChapter},
	{regexp.MustCompile(`(?i)\s*#(` + seriesRangeRE + `)\s*$`), models.SeriesNumberUnitVolume},
	{regexp.MustCompile(`(?i)\s+v(` + seriesRangeRE + `)\s*$`), models.SeriesNumberUnitVolume},
	{regexp.MustCompile(`(?i)\s*vol\.?\s*(` + seriesRangeRE + `)\s*$`), models.SeriesNumberUnitVolume},
	{regexp.MustCompile(`(?i)\s*volume\s*(` + seriesRangeRE + `)\s*$`), models.SeriesNumberUnitVolume},
	{regexp.MustCompile(`\s+(` + seriesRangeRE + `)\s*$`), models.SeriesNumberUnitVolume},
}

// NormalizeSeriesNumberInTitle normalizes volume- or chapter-style number
// indicators in CBZ titles. For volume indicators (v01, vol.5, volume 12,
// #001, bare trailing number) the title becomes "Title v{NNN}". For chapter
// indicators (chapter 5, Ch.5, c042) the title becomes "Title c{NNN}".
// Returns the normalized title, the parsed unit
// (models.SeriesNumberUnitVolume or models.SeriesNumberUnitChapter, "" when
// no match), and whether a number was found. Non-CBZ files are returned
// unchanged.
func NormalizeSeriesNumberInTitle(title string, fileType string) (string, string, bool) {
	if fileType != models.FileTypeCBZ {
		return title, "", false
	}

	for _, p := range seriesNumberPatterns {
		matches := p.re.FindStringSubmatch(title)
		if len(matches) < 2 {
			continue
		}
		start, end, ok := seriesnum.ParseRange(matches[1])
		if !ok {
			continue
		}
		baseTitle := strings.TrimSpace(p.re.ReplaceAllString(title, ""))
		prefix := "v"
		if p.unit == models.SeriesNumberUnitChapter {
			prefix = "c"
		}
		normalized := fmt.Sprintf("%s %s%s", baseTitle, prefix, formatPaddedSeriesRange(start, end))
		return strings.TrimSpace(normalized), p.unit, true
	}

	return title, "", false
}

func formatPaddedSeriesRange(start float64, end *float64) string {
	formatted := formatPaddedSeriesEndpoint(start)
	if end != nil {
		formatted += "-" + formatPaddedSeriesEndpoint(*end)
	}
	return formatted
}

func formatPaddedSeriesEndpoint(number float64) string {
	formatted := seriesnum.FormatRange(number, nil)
	integer, fraction, hasFraction := strings.Cut(formatted, ".")
	if len(integer) < 3 {
		integer = strings.Repeat("0", 3-len(integer)) + integer
	}
	if hasFraction {
		return integer + "." + fraction
	}
	return integer
}

// extractSeriesNumberFromTitle extracts a normalized series number suffix
// ("v003", "c042", or a range such as "v001-003") from a title.
func extractSeriesNumberFromTitle(title string) (start, end *float64, unit string) {
	seriesNumberPattern := regexp.MustCompile(`\s+([vc])(` + seriesRangeRE + `)\s*$`)
	matches := seriesNumberPattern.FindStringSubmatch(title)
	if len(matches) < 3 {
		return nil, nil, ""
	}
	startValue, end, ok := seriesnum.ParseRange(matches[2])
	if !ok {
		return nil, nil, ""
	}
	unit = models.SeriesNumberUnitVolume
	if strings.EqualFold(matches[1], "c") {
		unit = models.SeriesNumberUnitChapter
	}
	return &startValue, end, unit
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

// ExtractSeriesFromTitle extracts a series name and atomic number group from a
// normalized CBZ title. It accepts "v{N}" / "c{N}" suffixes and their ranges.
func ExtractSeriesFromTitle(title string, fileType string) (seriesName string, start, end *float64, unit string, ok bool) {
	if fileType != models.FileTypeCBZ {
		return "", nil, nil, "", false
	}
	seriesNumberPattern := regexp.MustCompile(`^(.+?)\s+([vc])(` + seriesRangeRE + `)\s*$`)
	matches := seriesNumberPattern.FindStringSubmatch(title)
	if len(matches) < 4 {
		return "", nil, nil, "", false
	}
	seriesName = strings.TrimSpace(matches[1])
	if seriesName == "" {
		return "", nil, nil, "", false
	}
	startValue, end, ok := seriesnum.ParseRange(matches[3])
	if !ok {
		return "", nil, nil, "", false
	}
	unit = models.SeriesNumberUnitVolume
	if strings.EqualFold(matches[2], "c") {
		unit = models.SeriesNumberUnitChapter
	}
	return seriesName, &startValue, end, unit, true
}
