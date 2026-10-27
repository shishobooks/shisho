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

// A font entry that can't be read, such as one whose checksum is wrong or
// whose compression Go can't decode, is left for the generator to copy as
// it is, as it was before fonts were checked, so the book still downloads.
func TestRekeyFonts_SkipsUnreadableFonts(t *testing.T) {
	t.Parallel()
	font := append([]byte("OTTO"), make([]byte, 1200)...)
	key, n, _ := obfuscationKey(idpfObfuscation, "old")
	hidden := xorFont(font, key, n)
	enc := `<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:enc="http://www.w3.org/2001/04/xmlenc#">
<enc:EncryptedData><enc:EncryptionMethod Algorithm="http://www.idpf.org/2008/embedding"/><enc:CipherData><enc:CipherReference URI="bad.otf"/></enc:CipherData></enc:EncryptedData>
<enc:EncryptedData><enc:EncryptionMethod Algorithm="http://www.idpf.org/2008/embedding"/><enc:CipherData><enc:CipherReference URI="bzip.otf"/></enc:CipherData></enc:EncryptedData>
</encryption>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(encryptionPath)
	require.NoError(t, err)
	_, err = w.Write([]byte(enc))
	require.NoError(t, err)
	for name, method := range map[string]uint16{"bad.otf": zip.Store, "bzip.otf": 12} {
		w, err := zw.CreateRaw(&zip.FileHeader{Name: name, Method: method, CRC32: 1, CompressedSize64: uint64(len(hidden)), UncompressedSize64: uint64(len(hidden))})
		require.NoError(t, err)
		_, err = w.Write(hidden)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	src, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)

	for _, after := range []string{"old", "new"} {
		fonts, rewritten, err := rekeyFonts(src, []string{"old"}, []string{after}, after)
		require.NoError(t, err, "identifiers %s", after)
		assert.Nil(t, fonts)
		assert.Nil(t, rewritten)
	}
}

// encryption.xml is rewritten by the URI its CipherReference gives,
// entities decoded, past any other URI in the block, and only the
// EncryptionMethod's algorithm changes.
func TestRewriteEncryption_MatchesTheCipherReference(t *testing.T) {
	t.Parallel()
	enc := `<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:enc="http://www.w3.org/2001/04/xmlenc#" xmlns:ds="http://www.w3.org/2000/09/xmldsig#">
<enc:EncryptedData><enc:EncryptionMethod Algorithm="http://ns.adobe.com/pdf/enc#RC"/><ds:KeyInfo><ds:RetrievalMethod URI="#k" Type="t" Algorithm="keep"/></ds:KeyInfo><enc:CipherData><enc:CipherReference URI="OEBPS/a&amp;b.otf"/></enc:CipherData></enc:EncryptedData>
</encryption>`
	got := string(rewriteEncryption([]byte(enc), map[string]string{"OEBPS/a&b.otf": idpfObfuscation}))
	assert.Contains(t, got, `<enc:EncryptionMethod Algorithm="http://www.idpf.org/2008/embedding"/>`)
	assert.Contains(t, got, `Algorithm="keep"`)
	assert.Equal(t, map[string]string{"OEBPS/a&b.otf": idpfObfuscation}, obfuscatedFonts([]byte(got)))
}

// An Adobe font stays as it is when the new identifiers still make its
// key, however the UUID is written.
func TestRekeyFonts_AdobeKeyWrittenAnotherWay(t *testing.T) {
	t.Parallel()
	font := append([]byte("OTTO"), make([]byte, 1200)...)
	key, n, _ := obfuscationKey(adobeObfuscation, "urn:uuid:0F1E2D3C-4B5A-4968-8776-A5B4C3D2E1F0")
	src := fontsZip(t, map[string][]byte{
		encryptionPath: []byte(fontsEncryption),
		"OEBPS/a.otf":  xorFont(font, key, n),
	})
	fonts, enc, err := rekeyFonts(src, []string{"urn:uuid:0F1E2D3C-4B5A-4968-8776-A5B4C3D2E1F0"}, []string{"isbn", "0f1e2d3c-4b5a-4968-8776-a5b4c3d2e1f0"}, "isbn")
	require.NoError(t, err)
	assert.Nil(t, fonts)
	assert.Nil(t, enc)
}
