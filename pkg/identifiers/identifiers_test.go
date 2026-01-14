// pkg/identifiers/identifiers_test.go
package identifiers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectType(t *testing.T) {
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

func TestNormalizeISBN(t *testing.T) {
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
