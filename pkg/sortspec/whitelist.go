package sortspec

// Canonical sort field tokens accepted by Parse.
//
// When adding a new field here, you MUST also:
//   - Update AllFields() below (order matters — it's pinned in tests).
//   - Add an SQL case to OrderClauses in sql.go.
//   - Update the TS whitelist in app/libraries/sortSpec.ts (it mirrors this file).
//   - Document semantics in website/docs/gallery-sort.md (the user-facing reference).
const (
	FieldTitle        = "title"
	FieldAuthor       = "author"
	FieldSeries       = "series"
	FieldDateAdded    = "date_added"
	FieldDateReleased = "date_released"
	FieldPageCount    = "page_count"
	FieldDuration     = "duration"
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
	}
}

// IsValidField returns true if s is a whitelisted sort field token.
func IsValidField(s string) bool {
	switch s {
	case FieldTitle, FieldAuthor, FieldSeries,
		FieldDateAdded, FieldDateReleased,
		FieldPageCount, FieldDuration:
		return true
	}
	return false
}
