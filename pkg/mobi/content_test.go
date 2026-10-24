package mobi

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HUFF/CDIC text reads the same as the PalmDOC text of the same book.
func TestReadContent_HuffCDIC(t *testing.T) {
	t.Parallel()
	for name, sample := range map[string]testgen.MOBIOptions{
		"AZW3":  testgen.KF8Sample(testgen.MOBIKindKF8),
		"combo": testgen.KF8Sample(testgen.MOBIKindCombo),
		"KF7":   testgen.KF7Sample(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			want, err := readBuiltContent(t, sample)
			require.NoError(t, err)
			huff := sample
			huff.Compression = testgen.MOBICompressionHuffCDIC
			got, err := readBuiltContent(t, huff)
			require.NoError(t, err)

			require.Len(t, got.Documents, len(want.Documents))
			for i := range want.Documents {
				assert.Equal(t, string(want.Documents[i].Content), string(got.Documents[i].Content), want.Documents[i].Name)
			}
			assert.Equal(t, want.Styles, got.Styles)
			assert.Equal(t, want.TOC, got.TOC)
		})
	}
}

// A combo file converts from its KF8 book, with its stylesheet, even when
// its MOBI6 book has text of its own.
func TestReadContent_ComboPrefersKF8(t *testing.T) {
	t.Parallel()
	opts := testgen.KF8Sample(testgen.MOBIKindCombo)
	opts.MOBI6 = testgen.KF7Sample().MOBI6
	book, err := readBuiltContent(t, opts)
	require.NoError(t, err)
	require.Len(t, book.Documents, 2)
	assert.Contains(t, string(book.Documents[0].Content), `<h1 id="ch1">Chapter One</h1>`)
	assert.Contains(t, string(book.Documents[0].Content), `<link href="flow0001.css" rel="stylesheet" type="text/css"/>`)
	require.Len(t, book.Styles, 1)
	assert.Contains(t, string(book.Styles[0].Content), "h1 { color: red; }")
	for _, doc := range book.Documents {
		assert.NotContains(t, string(doc.Content), "filepos", doc.Name)
	}
}

// Embedded fonts are copied out of their records, which are compressed and
// obfuscated, and the stylesheet names them by file.
func TestReadContent_Fonts(t *testing.T) {
	t.Parallel()
	for name, kind := range map[string]testgen.MOBIKind{"AZW3": testgen.MOBIKindKF8, "combo": testgen.MOBIKindCombo} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			book, err := readBuiltContent(t, testgen.KF8FontSample(kind))
			require.NoError(t, err)
			require.Len(t, book.Fonts, 1)
			font := book.Fonts[0]
			assert.Equal(t, "font0004.ttf", font.Name)
			assert.Equal(t, "font/ttf", font.MediaType)
			assert.Equal(t, testgen.KF8SampleFont, font.Content)
			require.Len(t, book.Styles, 1)
			assert.True(t, strings.HasPrefix(string(book.Styles[0].Content), `@font-face { font-family: "Sample"; src: url(font0004.ttf); }`), string(book.Styles[0].Content))

			var names []string
			for _, r := range book.Resources {
				names = append(names, r.Name)
			}
			assert.Equal(t, []string{"image0001.jpg", "image0003.png"}, names, "fonts are not images")
		})
	}
}

// A damaged file fails with an error, never a panic or a hang: each copy
// here has a run of bytes overwritten somewhere in it, or is cut short.
func TestReadContent_DamagedFiles(t *testing.T) {
	t.Parallel()
	huffKF7 := testgen.KF7Sample()
	huffKF7.Compression = testgen.MOBICompressionHuffCDIC
	for name, opts := range map[string]testgen.MOBIOptions{
		"KF8":           testgen.KF8Sample(testgen.MOBIKindKF8),
		"KF7":           testgen.KF7Sample(),
		"KF7 HUFF/CDIC": huffKF7,
		"fonts":         testgen.KF8FontSample(testgen.MOBIKindKF8),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data := testgen.BuildMOBI(t, opts)
			for at := 0; at < len(data); at += 37 {
				for _, fill := range []byte{0x00, 0xFF, 0x7F} {
					damaged := bytes.Clone(data)
					for i := at; i < min(at+4, len(damaged)); i++ {
						damaged[i] = fill
					}
					assert.NotPanics(t, func() { _, _ = ReadContent(context.Background(), bytes.NewReader(damaged), int64(len(damaged))) }, "bytes at %d set to %#x", at, fill)
				}
				cut := data[:at]
				assert.NotPanics(t, func() { _, _ = ReadContent(context.Background(), bytes.NewReader(cut), int64(len(cut))) }, "cut at %d", at)
			}
		})
	}
}

// The samples end their first text record inside a character, so the
// record's multibyte trailing entry holds the bytes that finish it, which
// reading drops: the other tests read the samples' text whole.
func TestSamples_SplitACharacterAcrossTextRecords(t *testing.T) {
	t.Parallel()
	for name, opts := range map[string]testgen.MOBIOptions{
		"KF8": testgen.KF8Sample(testgen.MOBIKindKF8),
		"KF7": testgen.KF7Sample(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data := testgen.BuildMOBI(t, opts)
			db, err := readPDB(bytes.NewReader(data), int64(len(data)))
			require.NoError(t, err)
			rec, err := db.record(1, maxTextRecord)
			require.NoError(t, err)
			rec = rec[:len(rec)-trailingEntrySize(rec)]
			assert.NotZero(t, rec[len(rec)-1]&0x3, "the first text record ends inside a character")
		})
	}
}

// A crafted dictionary fails with an error: a phrase that contains itself,
// a code that names no phrase, and a record that expands too long.
func TestHuffDecoder_Errors(t *testing.T) {
	t.Parallel()
	// Every code is 8 bits long, and code c names phrase 255-c.
	decoder := func(phrases ...huffPhrase) *huffDecoder {
		d := &huffDecoder{phrases: phrases}
		for i := range d.codes {
			d.codes[i] = huffCode{length: 8, terminal: true, max: 0xFFFFFFFF}
		}
		return d
	}
	_, err := decoder(huffPhrase{data: []byte{0xFF}}).decompress([]byte{0xFF}, 100)
	require.ErrorContains(t, err, "contains itself")
	_, err = decoder(huffPhrase{data: []byte("x"), expanded: true}).decompress([]byte{0x00}, 100)
	require.ErrorContains(t, err, "names no phrase")
	_, err = decoder(huffPhrase{data: []byte("0123456789"), expanded: true}).decompress([]byte{0xFF, 0xFF}, 15)
	require.ErrorContains(t, err, "too long")
	out, err := decoder(huffPhrase{data: []byte("ab"), expanded: true}, huffPhrase{data: []byte{0xFF, 0xFF}}).decompress([]byte{0xFE, 0xFF}, 100)
	require.NoError(t, err)
	assert.Equal(t, "ababab", string(out), "a compressed phrase expands once and is kept")
}

// A font past what the book's fonts may take together is refused, and the
// book is declined.
func TestReadFont_Limit(t *testing.T) {
	t.Parallel()
	data := testgen.BuildMOBI(t, testgen.KF8FontSample(testgen.MOBIKindKF8))
	db, err := readPDB(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	b, err := readBook(db)
	require.NoError(t, err)
	res, err := scanResources(db, b.firstImage)
	require.NoError(t, err)
	record, ok := res.fontByEmbed(4)
	require.True(t, ok)

	font, err := readFont(db, record, 4, int64(len(testgen.KF8SampleFont)))
	require.NoError(t, err)
	assert.Equal(t, Document{Name: "font0004.ttf", MediaType: "font/ttf", Content: testgen.KF8SampleFont}, font)
	_, err = readFont(db, record, 4, int64(len(testgen.KF8SampleFont))-1)
	require.ErrorIs(t, err, errFontsTooLarge)
}
