package sortspec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidField(t *testing.T) {
	t.Parallel()

	valid := []string{
		FieldTitle, FieldAuthor, FieldSeries,
		FieldDateAdded, FieldDateReleased,
		FieldPageCount, FieldDuration,
	}
	for _, f := range valid {
		f := f
		t.Run("valid/"+f, func(t *testing.T) {
			t.Parallel()
			assert.True(t, IsValidField(f))
		})
	}

	invalid := []string{"", "bogus", "TITLE", "date_published", "Title"}
	for _, f := range invalid {
		f := f
		t.Run("invalid/"+f, func(t *testing.T) {
			t.Parallel()
			assert.False(t, IsValidField(f))
		})
	}
}

func TestAllFields_Stable(t *testing.T) {
	t.Parallel()
	// AllFields is documented/consumed by the frontend via tygo; if this
	// list changes, the TS whitelist must be updated in lockstep. This
	// test just pins the expected order so additions are explicit.
	expected := []string{
		"title", "author", "series",
		"date_added", "date_released",
		"page_count", "duration",
	}
	assert.Equal(t, expected, AllFields())
}
