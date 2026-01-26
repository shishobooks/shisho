package htmlutil

import (
	"regexp"
	"strings"
)

// tagPattern matches HTML tags including self-closing tags.
var tagPattern = regexp.MustCompile(`<[^>]*>`)

// multipleSpacesPattern matches multiple consecutive whitespace characters.
var multipleSpacesPattern = regexp.MustCompile(`\s{2,}`)

// StripTags removes all HTML tags from a string and normalizes whitespace.
// It converts block-level tags (p, div, br, etc.) to newlines to preserve
// paragraph structure, then strips remaining tags and cleans up whitespace.
func StripTags(html string) string {
	if html == "" {
		return ""
	}

	// Replace block-level elements with newlines to preserve paragraph structure
	// This includes common block tags that typically create visual breaks
	blockTags := []string{"</p>", "</div>", "<br>", "<br/>", "<br />", "</li>", "</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>"}
	result := html
	for _, tag := range blockTags {
		result = strings.ReplaceAll(result, tag, "\n")
		// Also handle uppercase variants
		result = strings.ReplaceAll(result, strings.ToUpper(tag), "\n")
	}

	// Remove all remaining HTML tags
	result = tagPattern.ReplaceAllString(result, "")

	// Decode common HTML entities
	result = decodeHTMLEntities(result)

	// Normalize whitespace: collapse multiple spaces/tabs to single space
	// but preserve intentional newlines (from block tags)
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		// Collapse multiple spaces within each line
		line = multipleSpacesPattern.ReplaceAllString(line, " ")
		lines[i] = strings.TrimSpace(line)
	}

	// Rejoin lines, removing empty ones and collapsing multiple newlines
	var nonEmptyLines []string
	for _, line := range lines {
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	return strings.Join(nonEmptyLines, "\n")
}

// decodeHTMLEntities decodes common HTML entities to their character equivalents.
func decodeHTMLEntities(s string) string {
	// Common named entities
	replacements := []struct {
		entity string
		char   string
	}{
		{"&nbsp;", " "},
		{"&amp;", "&"},
		{"&lt;", "<"},
		{"&gt;", ">"},
		{"&quot;", "\""},
		{"&#39;", "'"},
		{"&apos;", "'"},
		{"&mdash;", "\u2014"},  // em dash
		{"&ndash;", "\u2013"},  // en dash
		{"&hellip;", "\u2026"}, // ellipsis
		{"&rsquo;", "\u2019"},  // right single quote
		{"&lsquo;", "\u2018"},  // left single quote
		{"&rdquo;", "\u201D"},  // right double quote
		{"&ldquo;", "\u201C"},  // left double quote
		{"&copy;", "\u00A9"},   // copyright
		{"&reg;", "\u00AE"},    // registered
		{"&trade;", "\u2122"},  // trademark
	}

	result := s
	for _, r := range replacements {
		result = strings.ReplaceAll(result, r.entity, r.char)
	}

	return result
}
