package sortspec

import (
	"fmt"
	"strings"
)

// OrderClause is one unit of ordering for a Bun query, ready to pass
// to q.OrderExpr(clause.Expression, clause.Args...).
//
// Each user-visible sort level produces ONE OrderClause, except the
// series level which expands to two (series sort name, then series
// number ASC).
type OrderClause struct {
	Expression string
	Args       []any
}

// OrderClauses maps a parsed sort spec to the SQL ORDER BY clauses
// that implement it on the `books` table (aliased `b` per pkg/CLAUDE.md).
//
// Every clause includes a NULLS-LAST indicator (`<expr> IS NULL`) so
// books missing the sort key always sit at the end regardless of
// direction. SQLite has no native NULLS LAST.
func OrderClauses(levels []SortLevel) []OrderClause {
	if len(levels) == 0 {
		return nil
	}

	out := make([]OrderClause, 0, len(levels)+1) // +1 for the series expansion
	for _, l := range levels {
		switch l.Field {
		case FieldTitle:
			out = append(out, nullsLast("b.sort_title", l.Direction))

		case FieldAuthor:
			// Primary author = authors row for this book with lowest
			// sort_order, tie-broken by authors.id ASC. Books with zero
			// authors sort last via NULLS LAST.
			expr := `(SELECT p.sort_name
                      FROM authors a
                      JOIN persons p ON p.id = a.person_id
                      WHERE a.book_id = b.id
                      ORDER BY a.sort_order ASC, a.id ASC
                      LIMIT 1)`
			out = append(out, nullsLast(expr, l.Direction))

		case FieldSeries:
			// Primary series sort name, then series number (always ASC
			// within a series). Books with no series row sort last.
			//
			// "Primary" series is picked by bs.sort_order ASC (consistent
			// with how pkg/books/service.go selects the primary series
			// elsewhere). bs.series_number is the position *within* a
			// specific series (e.g., "#3 in Stormlight") and is not
			// meaningful for choosing which series is primary when a
			// book belongs to multiple series. The outer-level sort still
			// uses the primary series's series_number so that books in
			// the same series sort by their position within it.
			nameExpr := `(SELECT s.sort_name
                          FROM book_series bs
                          JOIN series s ON s.id = bs.series_id
                          WHERE bs.book_id = b.id
                          ORDER BY bs.sort_order ASC, bs.id ASC
                          LIMIT 1)`
			numExpr := `(SELECT bs.series_number
                         FROM book_series bs
                         WHERE bs.book_id = b.id
                         ORDER BY bs.sort_order ASC, bs.id ASC
                         LIMIT 1)`
			out = append(out,
				nullsLast(nameExpr, l.Direction),
				nullsLast(numExpr, DirAsc),
			)

		case FieldDateAdded:
			out = append(out, nullsLast("b.created_at", l.Direction))

		case FieldDateReleased:
			out = append(out, nullsLast(primaryFileCoalesce("release_date"), l.Direction))

		case FieldPageCount:
			out = append(out, nullsLast(primaryFileCoalesce("page_count"), l.Direction))

		case FieldDuration:
			out = append(out, nullsLast(primaryFileCoalesce("audiobook_duration_seconds"), l.Direction))
		}
	}
	return out
}

// nullsLast wraps a column/expression with both the NULLS-LAST
// indicator and the chosen direction, producing a single ORDER BY
// fragment like "<expr> IS NULL, <expr> ASC".
//
// The expression is embedded verbatim — callers must ensure it is
// safe (it always is in this package because expressions come from
// whitelisted field branches above).
func nullsLast(expr string, dir Direction) OrderClause {
	return OrderClause{
		Expression: fmt.Sprintf("%s IS NULL, %s %s", expr, expr, strings.ToUpper(string(dir))),
	}
}

// primaryFileCoalesce returns an SQL snippet that reads `field` from
// the book's primary file, falling back to any file on the book with
// a non-NULL value for that field.
//
//	COALESCE(
//	  (SELECT f.<field> FROM files f WHERE f.id = b.primary_file_id),
//	  (SELECT f.<field> FROM files f WHERE f.book_id = b.id AND f.<field> IS NOT NULL ORDER BY f.id LIMIT 1)
//	)
func primaryFileCoalesce(field string) string {
	return fmt.Sprintf(
		`COALESCE(
            (SELECT f.%[1]s FROM files f WHERE f.id = b.primary_file_id),
            (SELECT f.%[1]s FROM files f WHERE f.book_id = b.id AND f.%[1]s IS NOT NULL ORDER BY f.id LIMIT 1)
        )`, field)
}
