package search

import "strings"

const maxQueryLength = 100

// SanitizeFTSQuery escapes FTS5 special characters and wraps in quotes for literal matching.
// FTS5 has its own query language with operators (AND, OR, NOT, *, NEAR(), :, ", etc.).
// Even with parameterized queries, the FTS5 engine interprets these.
// This function ensures user input is treated as a literal phrase.
func SanitizeFTSQuery(input string) string {
	// 1. Trim and limit length
	input = strings.TrimSpace(input)
	if len(input) > maxQueryLength {
		input = input[:maxQueryLength]
	}
	if input == "" {
		return ""
	}

	// 2. Escape double quotes (used for phrase matching in FTS5)
	input = strings.ReplaceAll(input, `"`, `""`)

	// 3. Wrap in double quotes to treat as literal phrase
	// This prevents operators like AND/OR/NOT from being interpreted
	return `"` + input + `"`
}

// BuildPrefixQuery creates an FTS5 query for typeahead/prefix search.
// It sanitizes the input and appends a wildcard for prefix matching.
func BuildPrefixQuery(userInput string) string {
	sanitized := SanitizeFTSQuery(userInput)
	if sanitized == "" {
		return ""
	}
	// Add prefix wildcard outside the quotes: "user query"*
	return sanitized + "*"
}
