package epub

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOPF_Identifiers(t *testing.T) {
	opfXML := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Test Book</dc:title>
    <dc:identifier opf:scheme="ISBN">9780316769488</dc:identifier>
    <dc:identifier opf:scheme="ASIN">B08N5WRWNW</dc:identifier>
    <dc:identifier>urn:uuid:a1b2c3d4-e5f6-7890-abcd-ef1234567890</dc:identifier>
    <dc:identifier opf:scheme="GOODREADS">12345678</dc:identifier>
  </metadata>
</package>`

	opf, err := ParseOPF("test.opf", io.NopCloser(strings.NewReader(opfXML)))
	require.NoError(t, err)

	assert.Len(t, opf.Identifiers, 4)

	// Find each identifier by type
	idByType := make(map[string]string)
	for _, id := range opf.Identifiers {
		idByType[id.Type] = id.Value
	}

	assert.Equal(t, "9780316769488", idByType["isbn_13"])
	assert.Equal(t, "B08N5WRWNW", idByType["asin"])
	assert.Equal(t, "urn:uuid:a1b2c3d4-e5f6-7890-abcd-ef1234567890", idByType["uuid"])
	assert.Equal(t, "12345678", idByType["goodreads"])
}

func TestParseOPF_IdentifiersPatternMatch(t *testing.T) {
	opfXML := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Test Book</dc:title>
    <dc:identifier>9780316769488</dc:identifier>
    <dc:identifier>0316769487</dc:identifier>
  </metadata>
</package>`

	opf, err := ParseOPF("test.opf", io.NopCloser(strings.NewReader(opfXML)))
	require.NoError(t, err)

	assert.Len(t, opf.Identifiers, 2)

	idByType := make(map[string]string)
	for _, id := range opf.Identifiers {
		idByType[id.Type] = id.Value
	}

	assert.Equal(t, "9780316769488", idByType["isbn_13"])
	assert.Equal(t, "0316769487", idByType["isbn_10"])
}
