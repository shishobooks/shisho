package cbz

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCBZ_SeriesRange(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "Saga v01-03.cbz")
	f, err := os.Create(cbzPath)
	require.NoError(t, err)
	zw := zip.NewWriter(f)

	imgWriter, err := zw.Create("page001.jpg")
	require.NoError(t, err)
	_, err = imgWriter.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})
	require.NoError(t, err)

	comicInfoWriter, err := zw.Create("ComicInfo.xml")
	require.NoError(t, err)
	_, err = comicInfoWriter.Write([]byte(`<ComicInfo><Title>Saga</Title><Series>Saga</Series><Number>1-3</Number></ComicInfo>`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	metadata, err := Parse(cbzPath)
	require.NoError(t, err)
	require.NotNil(t, metadata.SeriesNumber)
	require.NotNil(t, metadata.SeriesNumberEnd)
	assert.InDelta(t, 1, *metadata.SeriesNumber, 0.001)
	assert.InDelta(t, 3, *metadata.SeriesNumberEnd, 0.001)
}

func TestParseCBZ_ComicInfoRangeDoesNotMixFilenameUnit(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cbzPath := filepath.Join(tmpDir, "Saga c05-08.cbz")
	f, err := os.Create(cbzPath)
	require.NoError(t, err)
	zw := zip.NewWriter(f)

	imgWriter, err := zw.Create("page001.jpg")
	require.NoError(t, err)
	_, err = imgWriter.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})
	require.NoError(t, err)

	comicInfoWriter, err := zw.Create("ComicInfo.xml")
	require.NoError(t, err)
	_, err = comicInfoWriter.Write([]byte(`<ComicInfo><Title>Saga</Title><Series>Saga</Series><Number>1-3</Number></ComicInfo>`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	metadata, err := Parse(cbzPath)
	require.NoError(t, err)
	require.NotNil(t, metadata.SeriesNumber)
	require.NotNil(t, metadata.SeriesNumberEnd)
	assert.InDelta(t, 1, *metadata.SeriesNumber, 0.001)
	assert.InDelta(t, 3, *metadata.SeriesNumberEnd, 0.001)
	assert.Nil(t, metadata.SeriesNumberUnit)
}

func TestParseCBZ_Identifiers(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestParseCBZ_Language(t *testing.T) {
	t.Parallel()

	ptrStr := func(s string) *string { return &s }

	tests := []struct {
		name      string
		comicInfo string
		wantLang  *string
	}{
		{
			name: "language present",
			comicInfo: `<?xml version="1.0"?>
<ComicInfo>
  <Title>Test Comic</Title>
  <LanguageISO>en</LanguageISO>
</ComicInfo>`,
			wantLang: ptrStr("en"),
		},
		{
			name: "no language",
			comicInfo: `<?xml version="1.0"?>
<ComicInfo>
  <Title>Test Comic</Title>
</ComicInfo>`,
			wantLang: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			cbzPath := filepath.Join(tmpDir, "test.cbz")

			f, err := os.Create(cbzPath)
			require.NoError(t, err)

			zw := zip.NewWriter(f)

			imgWriter, err := zw.Create("page001.jpg")
			require.NoError(t, err)
			_, err = imgWriter.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0}) // JPEG header
			require.NoError(t, err)

			comicInfoWriter, err := zw.Create("ComicInfo.xml")
			require.NoError(t, err)
			_, err = comicInfoWriter.Write([]byte(tt.comicInfo))
			require.NoError(t, err)

			require.NoError(t, zw.Close())
			require.NoError(t, f.Close())

			metadata, err := Parse(cbzPath)
			require.NoError(t, err)

			assert.Equal(t, tt.wantLang, metadata.Language)
		})
	}
}

func TestParseCBZ_PublisherPrefersImprint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		comicInfo     string
		wantPublisher string
	}{
		{
			name: "imprint overrides publisher",
			comicInfo: `<?xml version="1.0"?>
<ComicInfo>
  <Title>Test Comic</Title>
  <Publisher>Big Publisher</Publisher>
  <Imprint>Cool Imprint</Imprint>
</ComicInfo>`,
			wantPublisher: "Cool Imprint",
		},
		{
			name: "publisher used when no imprint",
			comicInfo: `<?xml version="1.0"?>
<ComicInfo>
  <Title>Test Comic</Title>
  <Publisher>Big Publisher</Publisher>
</ComicInfo>`,
			wantPublisher: "Big Publisher",
		},
		{
			name: "imprint only",
			comicInfo: `<?xml version="1.0"?>
<ComicInfo>
  <Title>Test Comic</Title>
  <Imprint>Cool Imprint</Imprint>
</ComicInfo>`,
			wantPublisher: "Cool Imprint",
		},
		{
			name: "neither present",
			comicInfo: `<?xml version="1.0"?>
<ComicInfo>
  <Title>Test Comic</Title>
</ComicInfo>`,
			wantPublisher: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			cbzPath := filepath.Join(tmpDir, "test.cbz")

			f, err := os.Create(cbzPath)
			require.NoError(t, err)

			zw := zip.NewWriter(f)

			imgWriter, err := zw.Create("page001.jpg")
			require.NoError(t, err)
			_, err = imgWriter.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0}) // JPEG header
			require.NoError(t, err)

			comicInfoWriter, err := zw.Create("ComicInfo.xml")
			require.NoError(t, err)
			_, err = comicInfoWriter.Write([]byte(tt.comicInfo))
			require.NoError(t, err)

			require.NoError(t, zw.Close())
			require.NoError(t, f.Close())

			metadata, err := Parse(cbzPath)
			require.NoError(t, err)

			assert.Equal(t, tt.wantPublisher, metadata.Publisher)
		})
	}
}

func TestExtractSeriesNumberFromFilename(t *testing.T) {
	t.Parallel()
	floatPtr := func(f float64) *float64 { return &f }

	tests := []struct {
		name     string
		filename string
		want     *float64
		wantEnd  *float64
		wantUnit *string
	}{
		{"v prefix", "Comic Title v2.cbz", floatPtr(2), nil, stringPtr("volume")},
		{"v range", "Comic Title v01-03.cbz", floatPtr(1), floatPtr(3), stringPtr("volume")},
		{"vol range", "Comic Title vol. 1 - 3.cbz", floatPtr(1), floatPtr(3), stringPtr("volume")},
		{"volume decimal range", "Comic Title volume 1.5—3.5.cbz", floatPtr(1.5), floatPtr(3.5), stringPtr("volume")},
		{"hash range", "Comic Title #1–3.cbz", floatPtr(1), floatPtr(3), stringPtr("volume")},
		{"bare range", "Comic Title 1-3.cbz", floatPtr(1), floatPtr(3), stringPtr("volume")},
		{"chapter range", "Comic Title chapter 5-8.cbz", floatPtr(5), floatPtr(8), stringPtr("chapter")},
		{"ch range", "Comic Title ch. 5 - 8.cbz", floatPtr(5), floatPtr(8), stringPtr("chapter")},
		{"c range", "Comic Title c05-08.cbz", floatPtr(5), floatPtr(8), stringPtr("chapter")},
		{"decimal volume", "Comic Title v1.5.cbz", floatPtr(1.5), nil, stringPtr("volume")},
		{"strips parenthesized metadata", "Comic Title v02 (2020) (Digital) (group).cbz", floatPtr(2), nil, stringPtr("volume")},
		{"reversed range rejected", "Comic Title v3-1.cbz", nil, nil, nil},
		{"equal range rejected", "Comic Title c5-5.cbz", nil, nil, nil},
		{"non-contiguous range rejected", "Comic Title v1,3.cbz", nil, nil, nil},
		{"no volume number", "Comic Title.cbz", nil, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, gotEnd, gotUnit := extractSeriesNumberFromFilename(tt.filename)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.InDelta(t, *tt.want, *got, 0.001)
			}
			if tt.wantEnd == nil {
				assert.Nil(t, gotEnd)
			} else {
				require.NotNil(t, gotEnd)
				assert.InDelta(t, *tt.wantEnd, *gotEnd, 0.001)
			}
			assert.Equal(t, tt.wantUnit, gotUnit)
		})
	}
}

func stringPtr(value string) *string { return &value }
