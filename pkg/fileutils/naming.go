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
	AuthorNames   []string // Author names as strings for file naming
	NarratorNames []string // Narrator names for M4B file naming
	Title         string
	SeriesNumber  *float64
	FileType      string // for determining volume number formatting
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

	// Add volume number only for CBZ files (manga/comic volumes)
	// But only if the title doesn't already contain volume information
	if opts.SeriesNumber != nil && opts.FileType == models.FileTypeCBZ {
		// Check if title already contains volume information
		titleHasVolume := extractVolumeFromTitle(opts.Title) != nil
		if !titleHasVolume {
			volumeStr := formatVolumeNumber(*opts.SeriesNumber, opts.FileType)
			parts = append(parts, volumeStr)
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

// formatVolumeNumber formats volume numbers appropriately based on file type.
func formatVolumeNumber(volume float64, fileType string) string {
	// For CBZ files, use v{number} format without zero-padding
	if fileType == models.FileTypeCBZ {
		if volume == float64(int(volume)) {
			return fmt.Sprintf("v%d", int(volume))
		}
		return fmt.Sprintf("v%.1f", volume)
	}

	// For other types, still use # format (though this shouldn't be called for non-CBZ)
	if volume == float64(int(volume)) {
		return fmt.Sprintf("#%d", int(volume))
	}

	return fmt.Sprintf("#%.1f", volume)
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

	// Basic pattern: starts with [Author] or contains volume indicators
	authorPattern := regexp.MustCompile(`^\[.+\]`)
	volumePattern := regexp.MustCompile(`(v\d+(?:\.\d+)?|#\d+(?:\.\d+)?)$`)

	return authorPattern.MatchString(nameWithoutExt) || volumePattern.MatchString(nameWithoutExt)
}

// NormalizeVolumeInTitle normalizes volume indicators in titles to the standard v{number} format.
// Only applies to CBZ files. Returns the normalized title and whether a volume was found.
func NormalizeVolumeInTitle(title string, fileType string) (string, bool) {
	if fileType != models.FileTypeCBZ {
		return title, false
	}

	// Patterns to match various volume indicators
	// Order matters: more specific patterns (with prefixes) should come before bare numbers
	volumePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s*#(\d+(?:\.\d+)?)\s*$`),         // matches "#001", "#7", "#7.5"
		regexp.MustCompile(`(?i)\s*v(\d+(?:\.\d+)?)\s*$`),         // matches "v12", "v7.5"
		regexp.MustCompile(`(?i)\s*vol\.?\s*(\d+(?:\.\d+)?)\s*$`), // matches "vol12", "vol.12", "vol 12"
		regexp.MustCompile(`(?i)\s*volume\s*(\d+(?:\.\d+)?)\s*$`), // matches "volume12", "volume 12"
		regexp.MustCompile(`\s+(\d+(?:\.\d+)?)\s*$`),              // matches bare numbers like "Title 1", "Title 2"
	}

	for _, pattern := range volumePatterns {
		if matches := pattern.FindStringSubmatch(title); len(matches) >= 2 {
			// Extract the base title without volume indicator
			baseTitle := pattern.ReplaceAllString(title, "")
			baseTitle = strings.TrimSpace(baseTitle)

			// Parse the volume number
			volumeStr := matches[1]
			if volume, err := strconv.ParseFloat(volumeStr, 64); err == nil {
				// Create normalized title with v{number} format
				var normalizedTitle string
				if volume == float64(int(volume)) {
					normalizedTitle = fmt.Sprintf("%s v%d", baseTitle, int(volume))
				} else {
					normalizedTitle = fmt.Sprintf("%s v%.1f", baseTitle, volume)
				}
				return strings.TrimSpace(normalizedTitle), true
			}
		}
	}

	return title, false
}

// extractVolumeFromTitle extracts the volume number from a normalized title.
// Returns nil if no volume is found.
func extractVolumeFromTitle(title string) *float64 {
	volumePattern := regexp.MustCompile(`\s+v(\d+(?:\.\d+)?)\s*$`)
	if matches := volumePattern.FindStringSubmatch(title); len(matches) >= 2 {
		if volume, err := strconv.ParseFloat(matches[1], 64); err == nil {
			return &volume
		}
	}
	return nil
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

// ExtractSeriesFromTitle extracts series name and volume number from a normalized title.
// Only applies to CBZ files with volume indicators in the "v{number}" format.
// Returns the base title (series name), volume number, and whether extraction succeeded.
func ExtractSeriesFromTitle(title string, fileType string) (seriesName string, volumeNumber *float64, ok bool) {
	if fileType != models.FileTypeCBZ {
		return "", nil, false
	}

	// Match normalized volume format: "Title v{number}"
	volumePattern := regexp.MustCompile(`^(.+?)\s+v(\d+(?:\.\d+)?)\s*$`)
	matches := volumePattern.FindStringSubmatch(title)
	if len(matches) < 3 {
		return "", nil, false
	}

	seriesName = strings.TrimSpace(matches[1])
	if seriesName == "" {
		return "", nil, false
	}

	volume, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return "", nil, false
	}

	return seriesName, &volume, true
}
