package mediafile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected *string
	}{
		{
			name:     "empty string returns nil",
			input:    "",
			expected: nil,
		},
		{
			name:     "valid ISO 639-1 en",
			input:    "en",
			expected: strPtr("en"),
		},
		{
			name:     "valid BCP 47 with region en-US",
			input:    "en-US",
			expected: strPtr("en-US"),
		},
		{
			name:     "valid BCP 47 with script zh-Hans",
			input:    "zh-Hans",
			expected: strPtr("zh-Hans"),
		},
		{
			name:     "valid BCP 47 full tag zh-Hans-CN",
			input:    "zh-Hans-CN",
			expected: strPtr("zh-Hans-CN"),
		},
		{
			name:     "ISO 639-2/T three-letter eng normalizes to en",
			input:    "eng",
			expected: strPtr("en"),
		},
		{
			name:     "ISO 639-2/T three-letter fra normalizes to fr",
			input:    "fra",
			expected: strPtr("fr"),
		},
		{
			name:     "ISO 639-2/T three-letter deu normalizes to de",
			input:    "deu",
			expected: strPtr("de"),
		},
		{
			name:     "case normalized EN-us becomes en-US",
			input:    "EN-us",
			expected: strPtr("en-US"),
		},
		{
			name:     "invalid tag returns nil",
			input:    "not-a-language",
			expected: nil,
		},
		{
			name:     "und undetermined returns nil",
			input:    "und",
			expected: nil,
		},
		{
			name:     "whitespace-padded input normalized",
			input:    "  en  ",
			expected: strPtr("en"),
		},
		{
			name:     "Valencian ca-valencia parses and canonicalizes",
			input:    "ca-valencia",
			expected: strPtr("ca-valencia"),
		},
		{
			name:     "legacy va subtag is rejected",
			input:    "va",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := NormalizeLanguage(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
