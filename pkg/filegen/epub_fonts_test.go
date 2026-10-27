package filegen_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // the IDPF font obfuscation key is a SHA-1
	"encoding/hex"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const adobeFontsID = "urn:uuid:0f1e2d3c-4b5a-4968-8776-a5b4c3d2e1f0"

// adobeObfuscate XORs a font's first 1024 bytes with the 16 bytes of a
// urn:uuid identifier, as Adobe's font obfuscation does.
func adobeObfuscate(font []byte, id string) []byte {
	key, err := hex.DecodeString(strings.ReplaceAll(strings.TrimPrefix(id, "urn:uuid:"), "-", ""))
	if err != nil {
		panic(err)
	}
	out := bytes.Clone(font)
	for i := range min(1024, len(out)) {
		out[i] ^= key[i%len(key)]
	}
	return out
}

// writeAdobeFontEPUB writes an EPUB 2 with one font obfuscated with
// Adobe's algorithm, keyed on its urn:uuid unique identifier, and returns
// the font in the clear.
func writeAdobeFontEPUB(t *testing.T, path string) []byte {
	t.Helper()
	font := roundTripFont("OTTO", 9)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range []struct {
		name string
		data []byte
	}{
		{"mimetype", []byte("application/epub+zip")},
		{"META-INF/container.xml", []byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)},
		{"META-INF/encryption.xml", []byte(`<?xml version="1.0"?>
<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:enc="http://www.w3.org/2001/04/xmlenc#">
  <enc:EncryptedData>
    <enc:EncryptionMethod Algorithm="http://ns.adobe.com/pdf/enc#RC"/>
    <enc:CipherData><enc:CipherReference URI="OEBPS/fonts/face.otf"/></enc:CipherData>
  </enc:EncryptedData>
</encryption>`)},
		{"OEBPS/content.opf", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<package version="2.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Adobe Fonts</dc:title>
    <dc:identifier id="uid" opf:scheme="UUID">` + adobeFontsID + `</dc:identifier>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/>
    <item id="font" href="fonts/face.otf" media-type="application/vnd.ms-opentype"/>
  </manifest>
  <spine><itemref idref="c1"/></spine>
</package>`)},
		{"OEBPS/c1.xhtml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>One</title></head><body><p>One</p></body></html>`)},
		{"OEBPS/fonts/face.otf", adobeObfuscate(font, adobeFontsID)},
	} {
		method := zip.Deflate
		if e.name == "mimetype" {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: method})
		require.NoError(t, err)
		_, err = w.Write(e.data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
	return font
}

// generatedEPUB generates the EPUB of src for file and returns its archive.
func generatedEPUB(t *testing.T, src string, file *models.File) *zip.Reader {
	t.Helper()
	gen, err := filegen.GeneratorFor(models.FileTypeEPUB, models.FileTypeEPUB)
	require.NoError(t, err)
	dest := filepath.Join(t.TempDir(), "out.epub")
	require.NoError(t, gen.Generate(context.Background(), src, dest, &models.Book{Title: "Fonts"}, file))
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	return zr
}

// readObfuscatedFont reads a font from a generated EPUB as a reading
// system does: with the algorithm its encryption.xml gives, keyed on the
// identifiers its OPF holds.
func readObfuscatedFont(t *testing.T, zr *zip.Reader, name string) []byte {
	t.Helper()
	var doc struct {
		Data []struct {
			Method struct {
				Algorithm string `xml:"Algorithm,attr"`
			} `xml:"EncryptionMethod"`
			Cipher struct {
				Reference struct {
					URI string `xml:"URI,attr"`
				} `xml:"CipherReference"`
			} `xml:"CipherData"`
		} `xml:"EncryptedData"`
	}
	require.NoError(t, xml.Unmarshal(readZipEntry(t, findZipEntry(t, zr, "META-INF/encryption.xml")), &doc))
	algorithm := ""
	for _, d := range doc.Data {
		if d.Cipher.Reference.URI == name {
			algorithm = d.Method.Algorithm
		}
	}
	opf, err := epub.ParseOPF("OEBPS/content.opf", readerOf(readZipEntry(t, findZipEntry(t, zr, "OEBPS/content.opf"))))
	require.NoError(t, err)
	data := readZipEntry(t, findZipEntry(t, zr, name))
	switch algorithm {
	case "http://www.idpf.org/2008/embedding":
		var unique string
		for _, id := range opf.Package.Metadata.Identifier {
			if id.ID == opf.Package.UniqueIdentifier {
				unique = strings.TrimSpace(id.Text)
			}
		}
		key := sha1.Sum([]byte(strings.Join(strings.Fields(unique), ""))) //nolint:gosec // the algorithm's key
		out := bytes.Clone(data)
		for i := range min(1040, len(out)) {
			out[i] ^= key[i%len(key)]
		}
		return out
	case "http://ns.adobe.com/pdf/enc#RC":
		for _, id := range opf.Package.Metadata.Identifier {
			if v := strings.TrimSpace(id.Text); strings.HasPrefix(v, "urn:uuid:") {
				return adobeObfuscate(data, v)
			}
		}
		require.Fail(t, "no UUID identifier for the Adobe key")
	}
	require.Failf(t, "font not listed", "%s is not in encryption.xml", name)
	return nil
}

func readerOf(data []byte) *readCloser { return &readCloser{bytes.NewReader(data)} }

type readCloser struct{ *bytes.Reader }

func (readCloser) Close() error { return nil }

// A font is obfuscated with a key made from the book's identifiers, so
// when Shisho's identifiers replace the one that keyed it, the font is
// obfuscated again under the new one, and a reading system still reads it.
func TestEPUBGenerator_KeepsObfuscatedFontsReadable(t *testing.T) {
	t.Parallel()
	newID := []*models.FileIdentifier{{Type: models.IdentifierTypeUUID, Value: "11111111-2222-4333-8444-555555555555"}}

	t.Run("IDPF", func(t *testing.T) {
		t.Parallel()
		src := filepath.Join(t.TempDir(), "book.epub")
		_, hidden := writeRoundTripEPUB(t, src)
		zr := generatedEPUB(t, src, &models.File{ID: 1, FileType: models.FileTypeEPUB, Identifiers: newID})
		assert.Contains(t, string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/content.opf"))), "11111111-2222-4333-8444-555555555555")
		assert.Equal(t, hidden, readObfuscatedFont(t, zr, "OEBPS/fonts/hidden.otf"))
	})

	t.Run("Adobe", func(t *testing.T) {
		t.Parallel()
		src := filepath.Join(t.TempDir(), "adobe.epub")
		font := writeAdobeFontEPUB(t, src)
		zr := generatedEPUB(t, src, &models.File{ID: 1, FileType: models.FileTypeEPUB, Identifiers: newID})
		assert.NotContains(t, string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/content.opf"))), adobeFontsID)
		assert.Equal(t, font, readObfuscatedFont(t, zr, "OEBPS/fonts/face.otf"))
	})

	// With the identifiers unchanged, the font and encryption.xml are
	// copied as they are.
	t.Run("unchanged", func(t *testing.T) {
		t.Parallel()
		src := filepath.Join(t.TempDir(), "book.epub")
		writeRoundTripEPUB(t, src)
		data, err := os.ReadFile(src)
		require.NoError(t, err)
		srcZip, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		require.NoError(t, err)
		zr := generatedEPUB(t, src, &models.File{ID: 1, FileType: models.FileTypeEPUB})
		for _, name := range []string{"OEBPS/fonts/hidden.otf", "META-INF/encryption.xml"} {
			assert.Equal(t, readZipEntry(t, findZipEntry(t, srcZip, name)), readZipEntry(t, findZipEntry(t, zr, name)), name)
		}
	})
}
