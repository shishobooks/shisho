package filenames

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClean(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
		kobo     string
	}{
		{"normal name", "normal name", "normal name"},
		{"all:invalid/chars\\here*?\"<>|", "allinvalidcharshere", "allinvalidcharshere"},
		{"[Author] {Narrator} #1", "Author Narrator 1", "Author Narrator 1"},
		{"Title (2024) - Sub_title, Vol. 1 Harry's", "Title (2024) - Sub_title, Vol. 1 Harry's", "Title (2024) - Sub_title, Vol. 1 Harry's"},
		{"Pride & Prejudice!", "Pride & Prejudice!", "Pride Prejudice"},
		{"  multiple   spaces\u00a0here  ", "multiple spaces here", "multiple spaces here"},
		{"Crème brûlée", "Creme brulee", "Creme brulee"},
		{"Ελληνικά", "Ελληνικά", ""},
		{"日本語", "日本語", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, Clean(tt.input), "every File Type")
			assert.Equal(t, tt.kobo, CleanASCII(tt.input), "KePub")
		})
	}
}
