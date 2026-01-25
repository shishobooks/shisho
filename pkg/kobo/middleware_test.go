package kobo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripKoboPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path     string
		expected string
	}{
		{"/kobo/ak_123/all/v1/library/sync", "/v1/library/sync"},
		{"/kobo/ak_123/library/5/v1/library/sync", "/v1/library/sync"},
		{"/kobo/ak_123/all/v1/initialization", "/v1/initialization"},
		{"/kobo/ak_123/list/3/v1/books/shisho-1/file/epub", "/v1/books/shisho-1/file/epub"},
		{"/no-v1-path", "/no-v1-path"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := StripKoboPrefix(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseShishoID_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id       string
		expected int
		ok       bool
	}{
		{"shisho-1", 1, true},
		{"shisho-42", 42, true},
		{"shisho-0", 0, true},
		{"kobo-123", 0, false},
		{"invalid", 0, false},
		{"shisho-", 0, false},
		{"shisho-abc", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			result, ok := ParseShishoID(tt.id)
			assert.Equal(t, tt.expected, result)
			assert.Equal(t, tt.ok, ok)
		})
	}
}

func TestShishoID_TableDriven(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "shisho-1", ShishoID(1))
	assert.Equal(t, "shisho-42", ShishoID(42))
	assert.Equal(t, "shisho-0", ShishoID(0))
}
