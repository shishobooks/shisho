package epub

import (
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKePub(t *testing.T) {
	t.Parallel()

	path := testgen.GenerateEPUB(t, t.TempDir(), "book.kepub.epub", testgen.EPUBOptions{
		Title: "Kobo Book", Authors: []string{"Jane Doe"}, KoboMarkup: true,
	})

	metadata, err := ParseKePub(path)
	require.NoError(t, err)
	assert.Equal(t, "Kobo Book", metadata.Title)
	require.Len(t, metadata.Authors, 1)
	assert.Equal(t, "Jane Doe", metadata.Authors[0].Name)
}

func TestParseKePub_KoboStoreDRM(t *testing.T) {
	t.Parallel()

	path := testgen.GenerateEPUB(t, t.TempDir(), "book.kepub.epub", testgen.EPUBOptions{
		Title: "Locked", KoboMarkup: true, KoboDRM: true,
	})

	_, err := ParseKePub(path)
	assert.ErrorIs(t, err, mediafile.ErrDRMProtected)
}

func TestParseKePub_EncryptionXML(t *testing.T) {
	t.Parallel()

	encryption := func(algorithm string) string {
		return `<?xml version="1.0"?>
<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:enc="http://www.w3.org/2001/04/xmlenc#">
  <enc:EncryptedData>
    <enc:EncryptionMethod Algorithm="` + algorithm + `"/>
    <enc:CipherData><enc:CipherReference URI="OEBPS/font.otf"/></enc:CipherData>
  </enc:EncryptedData>
</encryption>`
	}

	tests := []struct {
		name      string
		entries   map[string]string
		protected bool
	}{
		{"Adobe rights.xml", map[string]string{"META-INF/rights.xml": "<rights/>"}, true},
		{"content encrypted with AES", map[string]string{"META-INF/encryption.xml": encryption("http://www.w3.org/2001/04/xmlenc#aes128-cbc")}, true},
		{"IDPF font obfuscation only", map[string]string{"META-INF/encryption.xml": encryption("http://www.idpf.org/2008/embedding")}, false},
		{"font obfuscation listed before an Adobe rights.xml", map[string]string{
			"META-INF/encryption.xml": encryption("http://ns.adobe.com/pdf/enc#RC"),
			"META-INF/rights.xml":     "<rights/>",
		}, true},
		{"Adobe font obfuscation only", map[string]string{"META-INF/encryption.xml": encryption("http://ns.adobe.com/pdf/enc#RC")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := testgen.GenerateEPUB(t, t.TempDir(), "book.kepub.epub", testgen.EPUBOptions{
				Title: "Book", KoboMarkup: true, ExtraEntries: tt.entries,
			})
			_, err := ParseKePub(path)
			if tt.protected {
				assert.ErrorIs(t, err, mediafile.ErrDRMProtected)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Only the spine's content documents count, and one that is not markup is
// not enough: entries macOS adds, UTF-16 documents, and stray files are
// readable books.
func TestParseKePub_ReadableContentIsNotDRM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		chapter []byte
		entries map[string]string
	}{
		{name: "macOS AppleDouble entry", entries: map[string]string{"__MACOSX/OEBPS/._chapter1.xhtml": "\x00\x05\x16\x07binary"}},
		{name: "stray HTML outside the spine", entries: map[string]string{"OEBPS/notes.html": "plain text, not markup"}},
		{name: "UTF-16 content document", chapter: []byte("\xff\xfe<\x00h\x00t\x00m\x00l\x00/\x00>\x00")},
		{name: "UTF-16 content document without a byte order mark", chapter: []byte("<\x00h\x00t\x00m\x00l\x00/\x00>\x00")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := testgen.GenerateEPUB(t, t.TempDir(), "book.kepub.epub", testgen.EPUBOptions{
				Title: "Book", KoboMarkup: true, Chapter: tt.chapter, ExtraEntries: tt.entries,
			})
			_, err := ParseKePub(path)
			assert.NoError(t, err)
		})
	}
}
