// pkg/identifiers/identifiers_test.go
package identifiers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		value    string
		scheme   string
		expected Type
	}{
		// ISBN-13 with scheme
		{"isbn13 with scheme", "9780316769488", "ISBN", TypeISBN13},
		// ISBN-10 with scheme
		{"isbn10 with scheme", "0316769487", "ISBN", TypeISBN10},
		// ISBN-13 with hyphens and scheme
		{"isbn13 hyphens with scheme", "978-0-316-76948-8", "ISBN", TypeISBN13},
		// ASIN with scheme
		{"asin with scheme", "B08N5WRWNW", "ASIN", TypeASIN},
		// Calibre-style MOBI-ASIN scheme, typically on EPUBs converted from MOBI/AZW3
		{"mobi-asin scheme", "B002MQYOFW", "MOBI-ASIN", TypeASIN},
		// Scheme matching is case-insensitive (lowercased input still resolves)
		{"mobi-asin lowercase scheme", "B002MQYOFW", "mobi-asin", TypeASIN},
		// Amazon storefront schemes used by Calibre (amazon, amazon_de, amazon_uk, ...)
		{"amazon scheme", "B08N5WRWNW", "AMAZON", TypeASIN},
		{"amazon_de scheme", "B08N5WRWNW", "AMAZON_DE", TypeASIN},
		// Unknown scheme but ASIN-shaped value should still be detected via pattern fallback
		{"unknown scheme asin value", "B002MQYOFW", "SOMETHING-WEIRD", TypeASIN},
		// Unknown scheme but valid ISBN-13 value should still be detected
		{"unknown scheme isbn13 value", "9780316769488", "VENDOR-X", TypeISBN13},
		// Goodreads with scheme
		{"goodreads with scheme", "12345678", "GOODREADS", TypeGoodreads},
		// Google with scheme
		{"google with scheme", "abc123", "GOOGLE", TypeGoogle},
		// ISBN-13 pattern match (no scheme)
		{"isbn13 pattern", "9780316769488", "", TypeISBN13},
		// ISBN-10 pattern match (no scheme)
		{"isbn10 pattern", "0316769487", "", TypeISBN10},
		// ISBN-10 with X checksum
		{"isbn10 with X", "080442957X", "", TypeISBN10},
		// UUID pattern match
		{"uuid pattern", "urn:uuid:a1b2c3d4-e5f6-7890-abcd-ef1234567890", "", TypeUUID},
		// UUID without urn prefix
		{"uuid no prefix", "a1b2c3d4-e5f6-7890-abcd-ef1234567890", "", TypeUUID},
		// ASIN pattern match (starts with B0)
		{"asin pattern", "B08N5WRWNW", "", TypeASIN},
		// Unknown scheme
		{"unknown scheme", "somevalue", "UNKNOWN", TypeUnknown},
		// Invalid ISBN (bad checksum)
		{"invalid isbn", "9780316769489", "", TypeUnknown},
		// Random value
		{"random value", "random text", "", TypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectType(tt.value, tt.scheme)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateISBN10(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value    string
		expected bool
	}{
		{"0316769487", true},
		{"080442957X", true},
		{"0451524934", true},   // 1984 by George Orwell
		{"0316769488", false},  // bad checksum
		{"123456789", false},   // too short
		{"12345678901", false}, // too long
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			assert.Equal(t, tt.expected, ValidateISBN10(tt.value))
		})
	}
}

func TestValidateISBN13(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value    string
		expected bool
	}{
		{"9780316769488", true},
		{"9780804429573", true},
		{"9780316769489", false},  // bad checksum
		{"978031676948", false},   // too short
		{"97803167694888", false}, // too long
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			assert.Equal(t, tt.expected, ValidateISBN13(tt.value))
		})
	}
}

func TestNormalizeValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		identType string
		value     string
		expected  string
	}{
		{"isbn13 hyphens", string(TypeISBN13), "978-0-316-76948-8", "9780316769488"},
		{"isbn13 spaces", string(TypeISBN13), " 978 0 316 76948 8 ", "9780316769488"},
		{"isbn13 with prefix", string(TypeISBN13), "ISBN: 978-0-316-76948-8", "9780316769488"},
		{"isbn10 hyphens", string(TypeISBN10), "0-316-76948-7", "0316769487"},
		{"isbn10 lowercase x", string(TypeISBN10), "0-8044-2957-x", "080442957X"},
		{"asin lowercase", string(TypeASIN), "b08n5wrwnw", "B08N5WRWNW"},
		{"asin whitespace", string(TypeASIN), "  B08N5WRWNW  ", "B08N5WRWNW"},
		{"uuid urn prefix", string(TypeUUID), "URN:UUID:A1B2C3D4-E5F6-7890-ABCD-EF1234567890", "a1b2c3d4-e5f6-7890-abcd-ef1234567890"},
		{"uuid mixed case", string(TypeUUID), "  A1b2C3d4-e5f6-7890-abcd-ef1234567890 ", "a1b2c3d4-e5f6-7890-abcd-ef1234567890"},
		{"other trimmed", string(TypeOther), "  some-value  ", "some-value"},
		{"goodreads trimmed", string(TypeGoodreads), " 12345 ", "12345"},
		{"google trimmed", string(TypeGoogle), "  abc123  ", "abc123"},
		{"custom vendor trimmed", "custom_vendor", "  Mixed-Case-Val  ", "Mixed-Case-Val"},
		{"empty type trims", "", "  raw  ", "raw"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizeValue(tt.identType, tt.value))
		})
	}
}

func TestKey_StableAcrossFormatting(t *testing.T) {
	t.Parallel()
	// Semantically identical identifier values with cosmetic differences must
	// produce the same Key so diff-based scans don't report spurious changes.
	tests := []struct {
		name string
		a    [2]string // type, value
		b    [2]string
	}{
		{"isbn13 hyphens vs clean", [2]string{string(TypeISBN13), "978-0-316-76948-8"}, [2]string{string(TypeISBN13), "9780316769488"}},
		{"isbn13 isbn-prefix vs clean", [2]string{string(TypeISBN13), "ISBN: 978-0-316-76948-8"}, [2]string{string(TypeISBN13), "9780316769488"}},
		{"isbn10 lowercase x vs upper", [2]string{string(TypeISBN10), "0-8044-2957-x"}, [2]string{string(TypeISBN10), "080442957X"}},
		{"asin lowercase vs upper", [2]string{string(TypeASIN), "b08n5wrwnw"}, [2]string{string(TypeASIN), "B08N5WRWNW"}},
		{"uuid urn prefix vs bare", [2]string{string(TypeUUID), "URN:UUID:A1B2C3D4-E5F6-7890-ABCD-EF1234567890"}, [2]string{string(TypeUUID), "a1b2c3d4-e5f6-7890-abcd-ef1234567890"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, Key(tt.a[0], tt.a[1]), Key(tt.b[0], tt.b[1]))
		})
	}
}

func TestKey_DistinctTypes(t *testing.T) {
	t.Parallel()
	// Same value string but different types must produce distinct keys.
	assert.NotEqual(t, Key(string(TypeISBN13), "9780316769488"), Key(string(TypeOther), "9780316769488"))
}

func TestCandidateForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  []string // exact expected output (order matters: raw first)
	}{
		{"isbn hyphenated", "978-0-316-76948-8", []string{"978-0-316-76948-8", "9780316769488"}},
		{"isbn clean", "9780316769488", []string{"9780316769488"}},
		{"isbn with prefix", "ISBN: 978-0-316-76948-8", []string{"ISBN: 978-0-316-76948-8", "9780316769488"}},
		{"asin lowercase", "b08n5wrwnw", []string{"b08n5wrwnw", "B08N5WRWNW"}},
		{"asin already canonical", "B08N5WRWNW", []string{"B08N5WRWNW"}},
		{"uuid urn prefix mixed case", "URN:UUID:A1B2C3D4-E5F6-7890-ABCD-EF1234567890", []string{
			"URN:UUID:A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
			"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		}},
		{"empty returns nil", "", nil},
		{"whitespace only returns nil", "   ", nil},

		// False-positive guards: values containing ISBN-like digit substrings
		// must NOT generate an ISBN candidate, or a vendor id search would
		// incidentally match a real ISBN row.
		{"vendor id with embedded isbn digits", "ref-9780316769488-v2", []string{"ref-9780316769488-v2"}},
		{"goodreads numeric id", "12345678", []string{"12345678"}},
		{"random text", "not an identifier", []string{"not an identifier"}},
		{"asin-shaped but wrong length", "B08N5WRW", []string{"B08N5WRW"}},
		{"looks like isbn but bad checksum", "9780316769489", []string{"9780316769489"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CandidateForms(tt.input)
			assert.Equal(t, tt.want, got)
			// No duplicates (redundant given exact match, but guards against future loosening).
			seen := map[string]bool{}
			for _, v := range got {
				assert.False(t, seen[v], "duplicate value %q in candidates %v", v, got)
				seen[v] = true
			}
		})
	}
}

// TestKey_FormatContract locks in the exact string format of Key's output.
// Callers (notably pkg/worker scan diff code) depend on this format being
// "<type>:<normalized-value>" and on it being distinguishable from any other
// stringification scheme. Changing the format is a deliberate breaking change
// for those callers.
func TestKey_FormatContract(t *testing.T) {
	t.Parallel()
	// The format is <type>:<NormalizeValue(type,value)>.
	assert.Equal(t, "isbn_13:9780316769488", Key("isbn_13", "978-0-316-76948-8"))
	assert.Equal(t, "asin:B08N5WRWNW", Key("asin", "b08n5wrwnw"))
	assert.Equal(t, "uuid:a1b2c3d4-e5f6-7890-abcd-ef1234567890", Key("uuid", "URN:UUID:A1B2C3D4-E5F6-7890-ABCD-EF1234567890"))
	assert.Equal(t, "other:some-value", Key("other", "  some-value  "))
	// Empty type is allowed and produces ":<value>".
	assert.Equal(t, ":raw", Key("", " raw "))
}

func TestNormalizeISBN(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value    string
		expected string
	}{
		{"978-0-316-76948-8", "9780316769488"},
		{"0-316-76948-7", "0316769487"},
		{"978 0 316 76948 8", "9780316769488"},
		{"ISBN: 9780316769488", "9780316769488"},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizeISBN(tt.value))
		})
	}
}
