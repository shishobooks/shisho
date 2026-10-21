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
