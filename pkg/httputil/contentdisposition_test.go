package httputil

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetAttachmentFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		expected string
	}{
		{
			name:     "plain ASCII filename",
			filename: "book.epub",
			expected: `attachment; filename="book.epub"`,
		},
		{
			name:     "ASCII with spaces",
			filename: "[Author] Title.epub",
			expected: `attachment; filename="[Author] Title.epub"`,
		},
		{
			name:     "ASCII with backslash gets escaped",
			filename: `weird\name.epub`,
			expected: `attachment; filename="weird\\name.epub"`,
		},
		{
			name:     "ASCII with double quote gets escaped",
			filename: `say "hi".epub`,
			expected: `attachment; filename="say \"hi\".epub"`,
		},
		{
			name:     "Unicode filename emits both forms",
			filename: "[著者] タイトル.epub",
			expected: `attachment; filename="[] .epub"; filename*=UTF-8''%5B%E8%91%97%E8%80%85%5D%20%E3%82%BF%E3%82%A4%E3%83%88%E3%83%AB.epub`,
		},
		{
			name:     "accented Latin emits both forms",
			filename: "Café.epub",
			expected: `attachment; filename="Caf.epub"; filename*=UTF-8''Caf%C3%A9.epub`,
		},
		{
			name:     "unicode collapses internal whitespace in fallback",
			filename: "A 漢 B.epub",
			expected: `attachment; filename="A B.epub"; filename*=UTF-8''A%20%E6%BC%A2%20B.epub`,
		},
		{
			name:     "ASCII control bytes are stripped from fallback",
			filename: "book\x01.epub",
			expected: `attachment; filename="book.epub"; filename*=UTF-8''book%01.epub`,
		},
		{
			name:     "DEL byte triggers extended form and is percent-encoded",
			filename: "book\x7f.epub",
			expected: `attachment; filename="book.epub"; filename*=UTF-8''book%7F.epub`,
		},
		{
			name:     "CR/LF cannot inject extra headers",
			filename: "a\r\nX-Evil: yes\r\nb.epub",
			expected: `attachment; filename="aX-Evil: yesb.epub"; filename*=UTF-8''a%0D%0AX-Evil%3A%20yes%0D%0Ab.epub`,
		},
		{
			name:     "empty filename falls back to download",
			filename: "",
			expected: `attachment; filename="download"`,
		},
		{
			name:     "all-unicode filename uses download fallback",
			filename: "тест.epub",
			expected: `attachment; filename="download.epub"; filename*=UTF-8''%D1%82%D0%B5%D1%81%D1%82.epub`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			SetAttachmentFilename(rec, tt.filename)

			assert.Equal(t, tt.expected, rec.Header().Get("Content-Disposition"))
		})
	}
}
