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
			expected: `attachment; filename="download.epub"; filename*=UTF-8''%5B%E8%91%97%E8%80%85%5D%20%E3%82%BF%E3%82%A4%E3%83%88%E3%83%AB.epub`,
		},
		{
			name:     "accented Latin emits both forms, plain in the fallback",
			filename: "Café.epub",
			expected: `attachment; filename="Cafe.epub"; filename*=UTF-8''Caf%C3%A9.epub`,
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
			expected: `attachment; filename="a X-Evil yes b.epub"; filename*=UTF-8''a%0D%0AX-Evil%3A%20yes%0D%0Ab.epub`,
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

// A caller that builds the ASCII name itself, as generated downloads do,
// passes it, and it is sent as the fallback as it is.
func TestSetAttachmentFilenames(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	SetAttachmentFilenames(rec, "Soseki Natsume - 吾輩.epub", "Soseki Natsume - File 3.epub")
	assert.Equal(t, `attachment; filename="Soseki Natsume - File 3.epub"; filename*=UTF-8''Soseki%20Natsume%20-%20%E5%90%BE%E8%BC%A9.epub`, rec.Header().Get("Content-Disposition"))

	rec = httptest.NewRecorder()
	SetAttachmentFilenames(rec, "Plain.epub", "Plain.epub")
	assert.Equal(t, `attachment; filename="Plain.epub"`, rec.Header().Get("Content-Disposition"))
}

// A name derived for an original file is cleaned without its extension, so
// a part with nothing left in ASCII leaves no dangling separator, and a
// given or ASCII name never goes out hidden or with a line break.
func TestSetAttachmentFilenames_Fallbacks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		filename string
		ascii    string
		fallback string
	}{
		{name: "a dash before a dropped title", filename: "Author - 吾輩.epub", fallback: "Author.epub"},
		{name: "brackets and a dropped title", filename: "[Soseki Natsume] 吾輩は猫である.epub", fallback: "Soseki Natsume.epub"},
		{name: "a two-part extension stays whole", filename: "Ünïcode - 漢.kepub.epub", fallback: "Unicode.kepub.epub"},
		{name: "nothing left", filename: "吾輩.epub", fallback: "download.epub"},
		{name: "an ASCII name starting with a dot", filename: ".hidden.epub", fallback: "download.hidden.epub"},
		{name: "a given name with a line break is derived instead", filename: "漢 Book.epub", ascii: "a\r\nX: y", fallback: "Book.epub"},
		{name: "a given name starting with a dot", filename: "漢.epub", ascii: ".epub", fallback: "download.epub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			SetAttachmentFilenames(rec, tt.filename, tt.ascii)
			assert.Contains(t, rec.Header().Get("Content-Disposition"), `attachment; filename="`+tt.fallback+`"`)
		})
	}
}
