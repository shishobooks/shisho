package htmlutil

import (
	"html"
	"regexp"
	"strings"
)

// tagPattern matches HTML tags including self-closing tags.
var tagPattern = regexp.MustCompile(`<[^>]*>`)

// decodedTagPattern matches only what a browser would parse as markup: a "<"
// followed by a letter, "/", "!", or "?". It runs after entities are decoded,
// where a bare "<" is prose ("a < b") and must survive.
var decodedTagPattern = regexp.MustCompile(`<(?:/?[A-Za-z]|[!?])[^>]*>`)

// tagOpenerPattern matches a run of "<" that a browser would read as the start
// of a tag. After complete tags are removed, one can still be left with no
// closing ">" ("&lt;img src=x onerror=..."), or be formed when removing a tag
// joins two pieces ("<<b>img ...>"). Matching the whole run means dropping it
// cannot leave another "<" in front of the letter.
var tagOpenerPattern = regexp.MustCompile(`<+([A-Za-z/!?])`)

// multipleSpacesPattern matches multiple consecutive spaces/tabs (not newlines).
var multipleSpacesPattern = regexp.MustCompile(`[^\S\n]{2,}`)

// threeOrMoreNewlines matches 3+ consecutive newlines (with optional whitespace-only lines between them).
var threeOrMoreNewlines = regexp.MustCompile(`\n(\s*\n){2,}`)

// StripTags converts HTML to plain text with no live HTML tag in it. It turns
// block-level tags (p, div, br, etc.) into newlines to preserve paragraph
// structure, strips the remaining tags, decodes one level of entities, and
// cleans up whitespace.
//
// Markup that was entity-encoded ("&lt;img ...&gt;") becomes a tag once
// decoded, so complete tags are stripped again after decoding and any leftover
// "<" that would open a tag is dropped. Escaped prose such as "a &lt; b" still
// decodes to "a < b". Deeper encodings ("&amp;lt;b&amp;gt;") decode one level
// to inert text ("&lt;b&gt;"). The result is plain text, not safe HTML: every
// consumer that writes it into HTML must still escape it.
func StripTags(s string) string {
	if s == "" {
		return ""
	}

	result := blockTagsToNewlines(s)

	// Remove all remaining HTML tags
	result = tagPattern.ReplaceAllString(result, "")

	// Decode one level of entities. Non-breaking spaces become plain spaces so
	// the whitespace normalization below collapses and trims them.
	result = html.UnescapeString(result)
	result = strings.ReplaceAll(result, "\u00a0", " ")

	// Decoding can turn "&lt;p&gt;" into a live tag. Strip those as well, then
	// drop any "<" still able to open a tag, including one left by that strip.
	// Both are single linear passes.
	result = blockTagsToNewlines(result)
	result = decodedTagPattern.ReplaceAllString(result, "")
	result = tagOpenerPattern.ReplaceAllString(result, "$1")

	// Normalize whitespace: collapse multiple spaces/tabs to single space per line
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		line = multipleSpacesPattern.ReplaceAllString(line, " ")
		lines[i] = strings.TrimSpace(line)
	}
	result = strings.Join(lines, "\n")

	// Collapse 3+ consecutive newlines to double newlines (preserve paragraph breaks)
	result = threeOrMoreNewlines.ReplaceAllString(result, "\n\n")

	return strings.TrimSpace(result)
}

// blockTagsToNewlines replaces paragraph close tags with double newlines and
// other block-level tags with single newlines, so paragraph structure survives
// tag removal.
func blockTagsToNewlines(s string) string {
	result := s
	for _, tag := range []string{"</p>", "</P>"} {
		result = strings.ReplaceAll(result, tag, "\n\n")
	}

	blockTags := []string{"</div>", "<br>", "<br/>", "<br />", "</li>", "</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>"}
	for _, tag := range blockTags {
		result = strings.ReplaceAll(result, tag, "\n")
		// Also handle uppercase variants
		result = strings.ReplaceAll(result, strings.ToUpper(tag), "\n")
	}
	return result
}
