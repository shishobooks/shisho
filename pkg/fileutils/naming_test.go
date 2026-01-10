package fileutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitNames(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single name",
			input:    "John Doe",
			expected: []string{"John Doe"},
		},
		{
			name:     "comma separated",
			input:    "John Doe, Jane Smith",
			expected: []string{"John Doe", "Jane Smith"},
		},
		{
			name:     "semicolon separated",
			input:    "John Doe; Jane Smith",
			expected: []string{"John Doe", "Jane Smith"},
		},
		{
			name:     "mixed comma and semicolon",
			input:    "John Doe, Jane Smith; Bob Wilson",
			expected: []string{"John Doe", "Jane Smith", "Bob Wilson"},
		},
		{
			name:     "with extra whitespace",
			input:    "  John Doe  ,  Jane Smith  ;  Bob Wilson  ",
			expected: []string{"John Doe", "Jane Smith", "Bob Wilson"},
		},
		{
			name:     "empty parts filtered",
			input:    "John Doe,,Jane Smith;;Bob Wilson",
			expected: []string{"John Doe", "Jane Smith", "Bob Wilson"},
		},
		{
			name:     "only delimiters",
			input:    ",;,;",
			expected: nil,
		},
		{
			name:     "multiple semicolons then commas",
			input:    "Author A; Author B, Author C",
			expected: []string{"Author A", "Author B", "Author C"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitNames(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
