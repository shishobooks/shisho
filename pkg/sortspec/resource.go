package sortspec

import (
	"fmt"
	"strings"
)

// Sort field tokens for the resource lists (tags, genres, series, people,
// publishers). Each count token matches the response field it sorts by.
// FieldDateAdded is shared with books.
//
// The TS mirror is app/libraries/resourceSort.ts, and the user-facing
// reference is website/docs/browsing-search-bulk-actions.md.
const (
	FieldName              = "name"
	FieldBookCount         = "book_count"
	FieldFileCount         = "file_count"
	FieldAuthoredBookCount = "authored_book_count"
	FieldNarratedFileCount = "narrated_file_count"
)

// ResourceFields maps each sort field a resource list accepts to the SQL
// expression it orders by. Expressions are written by the owning package and
// never hold user input.
type ResourceFields map[string]string

// Parse reads a spec with the same grammar as the package-level Parse,
// accepting only this resource's fields.
func (f ResourceFields) Parse(s string) ([]SortLevel, error) {
	return parse(s, func(field string) bool {
		_, ok := f[field]
		return ok
	})
}

// OrderClauses maps parsed levels to ORDER BY clauses and always ends with
// idColumn ascending, so rows that tie on every level keep a stable order
// across pages.
func (f ResourceFields) OrderClauses(levels []SortLevel, idColumn string) []OrderClause {
	out := make([]OrderClause, 0, len(levels)+1)
	for _, l := range levels {
		expr, ok := f[l.Field]
		if !ok {
			continue
		}
		out = append(out, OrderClause{
			Expression: fmt.Sprintf("%s %s", expr, strings.ToUpper(string(l.Direction))),
		})
	}
	return append(out, OrderClause{Expression: idColumn + " ASC"})
}
