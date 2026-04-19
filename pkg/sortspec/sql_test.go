package sortspec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderClauses_SingleLevel(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldTitle, Direction: DirAsc},
	})

	// Title maps directly to b.sort_title. NULLS LAST is emulated with a
	// leading "IS NULL" term.
	assert.Len(t, got, 1)
	assert.Equal(t, "b.sort_title IS NULL, b.sort_title ASC", got[0].Expression)
	assert.Nil(t, got[0].Args)
}

func TestOrderClauses_DescDirection(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldDateAdded, Direction: DirDesc},
	})
	assert.Equal(t, "b.created_at IS NULL, b.created_at DESC", got[0].Expression)
}

func TestOrderClauses_SeriesExpandsToTwo(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldSeries, Direction: DirDesc},
	})

	// Series expands to (series name, then series number). The number
	// clause is always ASC regardless of the user's chosen direction —
	// "Stormlight #1 before #2" is not a user preference.
	assert.Len(t, got, 2)
	assert.Contains(t, got[0].Expression, "series") // sort name expression
	assert.Contains(t, got[0].Expression, "DESC")
	assert.Contains(t, got[1].Expression, "series_number")
	assert.Contains(t, got[1].Expression, "ASC")

	// Both inner subqueries must pick the "primary" series via
	// bs.sort_order (consistent with pkg/books/service.go), NOT
	// bs.series_number — series_number is the position within a single
	// series and is not meaningful for choosing which of multiple series
	// is primary. Regression guard for the Task 5 review fix.
	assert.Contains(t, got[0].Expression, "ORDER BY bs.sort_order ASC, bs.id ASC")
	assert.Contains(t, got[1].Expression, "ORDER BY bs.sort_order ASC, bs.id ASC")
	assert.NotContains(t, got[0].Expression, "ORDER BY bs.series_number ASC, bs.id ASC")
	assert.NotContains(t, got[1].Expression, "ORDER BY bs.series_number ASC, bs.id ASC")
}

func TestOrderClauses_PrimaryFileFallback(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldPageCount, Direction: DirAsc},
	})

	// page_count uses the COALESCE(primary file, any file with value)
	// pattern. The generated SQL must reference b.primary_file_id and
	// b.id as correlated subquery columns.
	assert.Len(t, got, 1)
	assert.Contains(t, got[0].Expression, "COALESCE")
	assert.Contains(t, got[0].Expression, "b.primary_file_id")
	assert.Contains(t, got[0].Expression, "b.id")
	assert.Contains(t, got[0].Expression, "page_count")
	assert.Contains(t, got[0].Expression, "ASC")
}

func TestOrderClauses_MultiLevel(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldAuthor, Direction: DirAsc},
		{Field: FieldTitle, Direction: DirAsc},
	})
	assert.Len(t, got, 2)
	assert.Contains(t, got[0].Expression, "sort_name") // author
	assert.Contains(t, got[1].Expression, "sort_title")
}

func TestOrderClauses_Empty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, OrderClauses(nil))
	assert.Empty(t, OrderClauses([]SortLevel{}))
}
