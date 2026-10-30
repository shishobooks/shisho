package sortspec

// Canonical sort field tokens accepted by Parse.
//
// When adding a new field here, you MUST also:
//   - Update AllFields() below (order matters — it's pinned in tests).
//   - Add an SQL case to OrderClauses in sql.go.
//   - Update the TS whitelist in app/libraries/sortSpec.ts (it mirrors this file).
//   - Document semantics in website/docs/browsing-search-bulk-actions.md (the user-facing reference).
const (
	FieldTitle        = "title"
	FieldAuthor       = "author"
	FieldSeries       = "series"
	FieldDateAdded    = "date_added"
	FieldDateReleased = "date_released"
	FieldPageCount    = "page_count"
	FieldDuration     = "duration"
	// FieldRating and FieldDateFinished sort on the reader's own Rating and
	// latest finished Read-through, so they need a reader.
	FieldRating       = "rating"
	FieldDateFinished = "date_finished"
)

// AllFields returns the canonical field list in UI display order.
func AllFields() []string {
	return []string{
		FieldTitle,
		FieldAuthor,
		FieldSeries,
		FieldDateAdded,
		FieldDateReleased,
		FieldPageCount,
		FieldDuration,
		FieldRating,
		FieldDateFinished,
	}
}

// IsValidField returns true if s is a whitelisted sort field token.
func IsValidField(s string) bool {
	switch s {
	case FieldTitle, FieldAuthor, FieldSeries,
		FieldDateAdded, FieldDateReleased,
		FieldPageCount, FieldDuration,
		FieldRating, FieldDateFinished:
		return true
	}
	return false
}
