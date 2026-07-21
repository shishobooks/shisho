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
}

func TestOrderClauses_DescDirection(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldDateAdded, Direction: DirDesc},
	})
	assert.Equal(t, "b.created_at IS NULL, b.created_at DESC", got[0].Expression)
}

func TestOrderClauses_SeriesIncludesOmnibusOrdering(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldSeries, Direction: DirDesc},
	})

	// Series expands to name, range discriminator, start, and endpoint.
	// Position clauses are always ASC regardless of the chosen name direction.
	assert.Len(t, got, 4)
	assert.Contains(t, got[0].Expression, "series")
	assert.Contains(t, got[0].Expression, "DESC")
	assert.Contains(t, got[1].Expression, "series_number_end IS NOT NULL")
	assert.Contains(t, got[1].Expression, "ASC")
	assert.Contains(t, got[2].Expression, "series_number")
	assert.Contains(t, got[2].Expression, "ASC")
	assert.Contains(t, got[3].Expression, "COALESCE")
	assert.Contains(t, got[3].Expression, "series_number_end")
	assert.Contains(t, got[3].Expression, "ASC")

	// Every inner subquery must pick the "primary" series via
	// bs.sort_order (consistent with pkg/books/service.go), NOT
	// bs.series_number — series_number is the position within a single
	// series and is not meaningful for choosing which of multiple series
	// is primary. Regression guard for the Task 5 review fix.
	for _, clause := range got {
		assert.Contains(t, clause.Expression, "ORDER BY bs.sort_order ASC, bs.id ASC")
		assert.NotContains(t, clause.Expression, "ORDER BY bs.series_number ASC, bs.id ASC")
	}
}

func TestOrderClauses_NewestFileFallback(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldPageCount, Direction: DirAsc},
	})

	// page_count uses the COALESCE(newest file, any file with value)
	// pattern. The generated SQL must prefer the newest file (f.id DESC).
	assert.Len(t, got, 1)
	assert.Contains(t, got[0].Expression, "COALESCE")
	assert.NotContains(t, got[0].Expression, "primary_file_id")
	assert.Contains(t, got[0].Expression, "f.id DESC")
	assert.Contains(t, got[0].Expression, "b.id")
	assert.Contains(t, got[0].Expression, "page_count")
	assert.Contains(t, got[0].Expression, "ASC")
}

func TestOrderClauses_NewestFileCoalesce_AllFields(t *testing.T) {
	t.Parallel()

	// All file-level sort fields (date_released, page_count, duration)
	// must use the newest-file coalesce.
	tests := []struct {
		field   string
		dbField string
	}{
		{FieldDateReleased, "release_date"},
		{FieldPageCount, "page_count"},
		{FieldDuration, "audiobook_duration_seconds"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()
			got := OrderClauses([]SortLevel{
				{Field: tt.field, Direction: DirDesc},
			})
			assert.Len(t, got, 1)
			assert.Contains(t, got[0].Expression, "COALESCE")
			assert.NotContains(t, got[0].Expression, "primary_file_id")
			assert.Contains(t, got[0].Expression, "f.id DESC")
			assert.Contains(t, got[0].Expression, tt.dbField)
		})
	}
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
