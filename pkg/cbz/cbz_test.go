package cbz

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCBZ_Identifiers(t *testing.T) {
	// Create test CBZ with ComicInfo.xml containing GTIN
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	// Create minimal CBZ with ComicInfo.xml
	f, err := os.Create(cbzPath)
	require.NoError(t, err)

	zw := zip.NewWriter(f)

	// Add a dummy image
	imgWriter, err := zw.Create("page001.jpg")
	require.NoError(t, err)
	_, err = imgWriter.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0}) // JPEG header
	require.NoError(t, err)

	// Add ComicInfo.xml with GTIN
	comicInfoWriter, err := zw.Create("ComicInfo.xml")
	require.NoError(t, err)
	_, err = comicInfoWriter.Write([]byte(`<?xml version="1.0"?>
<ComicInfo>
  <Title>Test Comic</Title>
  <GTIN>9780316769488</GTIN>
</ComicInfo>`))
	require.NoError(t, err)

	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	// Parse the CBZ
	metadata, err := Parse(cbzPath)
	require.NoError(t, err)

	require.Len(t, metadata.Identifiers, 1)
	assert.Equal(t, "isbn_13", metadata.Identifiers[0].Type)
	assert.Equal(t, "9780316769488", metadata.Identifiers[0].Value)
}

func TestParseCBZ_GTINAsOther(t *testing.T) {
	// Create test CBZ with ComicInfo.xml containing unrecognized GTIN
	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "test.cbz")

	f, err := os.Create(cbzPath)
	require.NoError(t, err)

	zw := zip.NewWriter(f)

	imgWriter, err := zw.Create("page001.jpg")
	require.NoError(t, err)
	_, err = imgWriter.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})
	require.NoError(t, err)

	comicInfoWriter, err := zw.Create("ComicInfo.xml")
	require.NoError(t, err)
	_, err = comicInfoWriter.Write([]byte(`<?xml version="1.0"?>
<ComicInfo>
  <Title>Test Comic</Title>
  <GTIN>1234567890123</GTIN>
</ComicInfo>`))
	require.NoError(t, err)

	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	metadata, err := Parse(cbzPath)
	require.NoError(t, err)

	// Unrecognized GTIN should be stored as "other"
	require.Len(t, metadata.Identifiers, 1)
	assert.Equal(t, "other", metadata.Identifiers[0].Type)
	assert.Equal(t, "1234567890123", metadata.Identifiers[0].Value)
}

func TestExtractSeriesNumberFromFilename(t *testing.T) {
	floatPtr := func(f float64) *float64 { return &f }

	tests := []struct {
		name     string
		filename string
		want     *float64
	}{
		{"v prefix", "Comic Title v2.cbz", floatPtr(2)},
		{"v prefix with leading zeros", "Comic Title v02.cbz", floatPtr(2)},
		{"hash prefix", "Comic Title #7.cbz", floatPtr(7)},
		{"bare number", "Comic Title 3.cbz", floatPtr(3)},
		{"decimal volume", "Comic Title v1.5.cbz", floatPtr(1.5)},
		{"strips parenthesized metadata", "Comic Title v02 (2020) (Digital) (group).cbz", floatPtr(2)},
		{"strips parens with hash", "Title #5 (HD) (Group).cbz", floatPtr(5)},
		{"no volume number", "Comic Title.cbz", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSeriesNumberFromFilename(tt.filename)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.InDelta(t, *tt.want, *got, 0.001)
			}
		})
	}
}
