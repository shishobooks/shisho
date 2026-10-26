package filegen

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fontsZip(t *testing.T, entries map[string][]byte) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	return zr
}

const fontsEncryption = `<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:enc="http://www.w3.org/2001/04/xmlenc#">
<enc:EncryptedData><enc:EncryptionMethod Algorithm="http://ns.adobe.com/pdf/enc#RC"/><enc:CipherData><enc:CipherReference URI="OEBPS/a.otf"/></enc:CipherData></enc:EncryptedData>
<enc:EncryptedData><enc:EncryptionMethod Algorithm="http://www.idpf.org/2008/embedding"/><enc:CipherData><enc:CipherReference URI="OEBPS/b%20b.ttf"/></enc:CipherData></enc:EncryptedData>
</encryption>`

// A font whose key the new identifiers no longer make is obfuscated again
// under the new unique identifier, with its algorithm in encryption.xml
// changed to match; one whose key survives is left alone.
func TestRekeyFonts(t *testing.T) {
	t.Parallel()
	oldUUID, oldUnique, newUnique := "urn:uuid:0f1e2d3c-4b5a-4968-8776-a5b4c3d2e1f0", "isbn-old", "isbn-new"
	a, b := append([]byte("OTTO"), make([]byte, 1200)...), append([]byte("\x00\x01\x00\x00"), make([]byte, 1200)...)
	adobeKey, adobeN, _ := obfuscationKey(adobeObfuscation, oldUUID)
	idpfKey, idpfN, _ := obfuscationKey(idpfObfuscation, oldUnique)
	src := fontsZip(t, map[string][]byte{
		encryptionPath:  []byte(fontsEncryption),
		"OEBPS/a.otf":   xorFont(a, adobeKey, adobeN),
		"OEBPS/b b.ttf": xorFont(b, idpfKey, idpfN),
	})
	before := []string{oldUnique, oldUUID}

	// The UUID survives, so the Adobe font stays; the unique identifier
	// changed, so the IDPF font is keyed again.
	fonts, enc, err := rekeyFonts(src, before, []string{newUnique, oldUUID}, newUnique)
	require.NoError(t, err)
	require.Len(t, fonts, 1)
	newKey, n, _ := obfuscationKey(idpfObfuscation, newUnique)
	assert.Equal(t, b, xorFont(fonts["OEBPS/b b.ttf"], newKey, n))
	assert.Equal(t, fontsEncryption, string(enc), "both fonts keep their algorithms")

	// Both keys gone: the Adobe font becomes an IDPF one.
	fonts, enc, err = rekeyFonts(src, before, []string{newUnique}, newUnique)
	require.NoError(t, err)
	require.Len(t, fonts, 2)
	assert.Equal(t, a, xorFont(fonts["OEBPS/a.otf"], newKey, n))
	assert.Equal(t, map[string]string{"OEBPS/a.otf": idpfObfuscation, "OEBPS/b b.ttf": idpfObfuscation}, obfuscatedFonts(enc))

	// With no unique identifier left, the fonts are written in the clear
	// and dropped from encryption.xml.
	fonts, enc, err = rekeyFonts(src, before, []string{"other"}, "")
	require.NoError(t, err)
	assert.Equal(t, map[string][]byte{"OEBPS/a.otf": a, "OEBPS/b b.ttf": b}, fonts)
	assert.Empty(t, obfuscatedFonts(enc))
	assert.Contains(t, string(enc), "<encryption")

	// Unchanged identifiers need nothing.
	fonts, enc, err = rekeyFonts(src, before, before, oldUnique)
	require.NoError(t, err)
	assert.Nil(t, fonts)
	assert.Nil(t, enc)
}
