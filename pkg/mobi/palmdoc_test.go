package mobi

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPalmDOCCompress_RoundTrips(t *testing.T) {
	t.Parallel()
	random := make([]byte, textRecordSize)
	rand.New(rand.NewSource(1)).Read(random)
	cases := map[string][]byte{
		"empty":           {},
		"markup":          []byte(strings.Repeat(`<p class="body">It was a dark and stormy night. </p>`, 80)[:textRecordSize]),
		"space pairs":     []byte(" a b c d e f g h ~ @ "),
		"literal escapes": {0x01, 0x02, 0x08, 0x80, 0xFF, 0xC3, 0xA9, 0x00, 0x09, 'x', 0x7F},
		"utf-8":           []byte(strings.Repeat("Ça va très bien, merci: 日本語。", 40)),
		"random":          random,
		"one run":         bytes.Repeat([]byte{'a'}, textRecordSize),
		"long literal":    bytes.Repeat([]byte{0x81, 0x92, 0xA3, 0xB4, 0xC5, 0xD6, 0xE7, 0xF8, 0x89, 0x9A}, 20),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out := palmDOCCompress(in)
			got, err := palmDOCDecompress(out)
			require.NoError(t, err)
			assert.Equal(t, in, got)
		})
	}
}

func TestPalmDOCCompress_ShrinksText(t *testing.T) {
	t.Parallel()
	in := []byte(strings.Repeat(`<p class="body">It was a dark and stormy night. </p>`, 80)[:textRecordSize])
	assert.Less(t, len(palmDOCCompress(in)), len(in)/3)
}
