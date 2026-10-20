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
		{"/kobo/abcdefghjkmnpqrs/v1/library/sync", "/v1/library/sync"},
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

func TestBasePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path     string
		expected string
	}{
		{"/kobo/abcdefghjkmnpqrs/v1/library/sync", "/kobo/abcdefghjkmnpqrs"},
		{"/kobo/abcdefghjkmnpqrs/download/shisho-1/kepub", "/kobo/abcdefghjkmnpqrs"},
		{"/kobo/ak_123/all/v1/initialization", "/kobo/ak_123/all"},
		{"/kobo/ak_123/library/5/v1/library/sync", "/kobo/ak_123/library/5"},
		{"/kobo/ak_123/list/3/download/shisho-1/kepub", "/kobo/ak_123/list/3"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, basePath(tt.path))
		})
	}
}
