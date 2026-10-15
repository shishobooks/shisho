package sortspec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testResourceFields = ResourceFields{
	FieldName:      "t.name",
	FieldBookCount: "(SELECT COUNT(*) FROM book_tags bt WHERE bt.tag_id = t.id)",
	FieldDateAdded: "t.created_at",
}

func TestResourceFields_Parse(t *testing.T) {
	t.Parallel()

	levels, err := testResourceFields.Parse("book_count:desc,name:asc")
	require.NoError(t, err)
	assert.Equal(t, []SortLevel{
		{Field: FieldBookCount, Direction: DirDesc},
		{Field: FieldName, Direction: DirAsc},
	}, levels)
}

func TestResourceFields_ParseRejectsFieldsOutsideTheResource(t *testing.T) {
	t.Parallel()

	// A book sort field is not valid for a resource that does not list it.
	_, err := testResourceFields.Parse("title:asc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `Unknown sort field "title".`)
}

func TestResourceFields_ParseKeepsTheBookGrammar(t *testing.T) {
	t.Parallel()

	for _, spec := range []string{"", "name", "name:up", "name:asc,name:desc", "name:asc,", " name:asc"} {
		_, err := testResourceFields.Parse(spec)
		assert.Error(t, err, spec)
	}
}

func TestResourceFields_OrderClauses(t *testing.T) {
	t.Parallel()

	got := testResourceFields.OrderClauses([]SortLevel{
		{Field: FieldBookCount, Direction: DirDesc},
		{Field: FieldName, Direction: DirAsc},
	}, "t.id")

	assert.Equal(t, []OrderClause{
		{Expression: "(SELECT COUNT(*) FROM book_tags bt WHERE bt.tag_id = t.id) DESC"},
		{Expression: "t.name ASC"},
		{Expression: "t.id ASC"},
	}, got)
}

func TestResourceFields_OrderClausesAlwaysEndsWithTheTiebreaker(t *testing.T) {
	t.Parallel()

	// Rows with equal counts need a stable order, or pagination can show a
	// row twice and skip another.
	got := testResourceFields.OrderClauses(nil, "t.id")
	assert.Equal(t, []OrderClause{{Expression: "t.id ASC"}}, got)
}
