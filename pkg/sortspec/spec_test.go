package sortspec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []SortLevel
	}{
		{
			name:  "single level asc",
			input: "title:asc",
			expected: []SortLevel{
				{Field: FieldTitle, Direction: DirAsc},
			},
		},
		{
			name:  "single level desc",
			input: "date_added:desc",
			expected: []SortLevel{
				{Field: FieldDateAdded, Direction: DirDesc},
			},
		},
		{
			name:  "multi level",
			input: "author:asc,series:asc,title:asc",
			expected: []SortLevel{
				{Field: FieldAuthor, Direction: DirAsc},
				{Field: FieldSeries, Direction: DirAsc},
				{Field: FieldTitle, Direction: DirAsc},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParse_Invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"missing direction", "title"},
		{"unknown field", "bogus:asc"},
		{"bad direction", "title:sideways"},
		{"duplicate field", "title:asc,title:desc"},
		{"trailing comma", "title:asc,"},
		{"leading comma", ",title:asc"},
		{"empty pair", "title:asc,,series:asc"},
		{"whitespace around pair", " title:asc "},
		{"too many levels", "title:asc,author:asc,series:asc,date_added:asc,date_released:asc,page_count:asc,duration:asc,title:desc,author:desc,series:desc,date_added:desc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestSerialize(t *testing.T) {
	t.Parallel()

	levels := []SortLevel{
		{Field: FieldAuthor, Direction: DirAsc},
		{Field: FieldSeries, Direction: DirAsc},
	}
	assert.Equal(t, "author:asc,series:asc", Serialize(levels))
	assert.Empty(t, Serialize(nil))
}

func TestParseSerialize_RoundTrip(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"title:asc",
		"date_added:desc",
		"author:asc,series:asc,title:asc",
		"page_count:desc,duration:asc",
	}

	for _, input := range inputs {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			parsed, err := Parse(input)
			require.NoError(t, err)
			assert.Equal(t, input, Serialize(parsed))
		})
	}
}
