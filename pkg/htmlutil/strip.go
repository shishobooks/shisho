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
	// Common named and numeric entities
	replacements := []struct {
		entity string
		char   string
	}{
		{"&nbsp;", " "},
		{"&#160;", " "}, // nbsp numeric
		{"&amp;", "&"},
		{"&#38;", "&"}, // ampersand numeric
		{"&lt;", "<"},
		{"&#60;", "<"}, // less than numeric
		{"&gt;", ">"},
		{"&#62;", ">"}, // greater than numeric
		{"&quot;", "\""},
		{"&#34;", "\""}, // quote numeric
		{"&#39;", "'"},
		{"&apos;", "'"},
		{"&mdash;", "\u2014"},  // em dash
		{"&#8212;", "\u2014"},  // em dash numeric
		{"&ndash;", "\u2013"},  // en dash
		{"&#8211;", "\u2013"},  // en dash numeric
		{"&hellip;", "\u2026"}, // ellipsis
		{"&#8230;", "\u2026"},  // ellipsis numeric
		{"&rsquo;", "\u2019"},  // right single quote
		{"&#8217;", "\u2019"},  // right single quote numeric
		{"&lsquo;", "\u2018"},  // left single quote
		{"&#8216;", "\u2018"},  // left single quote numeric
		{"&rdquo;", "\u201D"},  // right double quote
		{"&#8221;", "\u201D"},  // right double quote numeric
		{"&ldquo;", "\u201C"},  // left double quote
		{"&#8220;", "\u201C"},  // left double quote numeric
		{"&copy;", "\u00A9"},   // copyright
		{"&#169;", "\u00A9"},   // copyright numeric
		{"&reg;", "\u00AE"},    // registered
		{"&#174;", "\u00AE"},   // registered numeric
		{"&trade;", "\u2122"},  // trademark
		{"&#8482;", "\u2122"},  // trademark numeric
	}

	result := s
	for _, r := range replacements {
		result = strings.ReplaceAll(result, r.entity, r.char)
	}

	return result
}
