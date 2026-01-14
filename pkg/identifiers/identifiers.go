// pkg/identifiers/identifiers.go
package identifiers

import (
	"regexp"
	"strings"
	"unicode"
)

// Type represents the type of identifier.
type Type string

const (
	TypeISBN10    Type = "isbn_10"
	TypeISBN13    Type = "isbn_13"
	TypeASIN      Type = "asin"
	TypeUUID      Type = "uuid"
	TypeGoodreads Type = "goodreads"
	TypeGoogle    Type = "google"
	TypeOther     Type = "other"
	TypeUnknown   Type = ""
)

var (
	uuidRegex = regexp.MustCompile(`^(?:urn:uuid:)?[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	asinRegex = regexp.MustCompile(`^B0[A-Z0-9]{8}$`)
)

// DetectType determines the identifier type from a value and optional scheme.
// If scheme is provided, it takes precedence. Otherwise, pattern matching is used.
func DetectType(value, scheme string) Type {
	value = strings.TrimSpace(value)
	scheme = strings.ToUpper(strings.TrimSpace(scheme))

	// Check explicit scheme first
	switch scheme {
	case "ISBN":
		return detectISBNType(value)
	case "ASIN":
		return TypeASIN
	case "GOODREADS":
		return TypeGoodreads
	case "GOOGLE":
		return TypeGoogle
	case "UUID":
		return TypeUUID
	case "":
		// No scheme, use pattern matching
		break
	default:
		// Unknown scheme
		return TypeUnknown
	}

	// Pattern matching on value
	normalized := NormalizeISBN(value)
	if len(normalized) == 13 && ValidateISBN13(normalized) {
		return TypeISBN13
	}
	if len(normalized) == 10 && ValidateISBN10(normalized) {
		return TypeISBN10
	}
	if uuidRegex.MatchString(value) {
		return TypeUUID
	}
	if asinRegex.MatchString(strings.ToUpper(value)) {
		return TypeASIN
	}

	return TypeUnknown
}

// detectISBNType determines if an ISBN is ISBN-10 or ISBN-13.
func detectISBNType(value string) Type {
	normalized := NormalizeISBN(value)
	if len(normalized) == 13 && ValidateISBN13(normalized) {
		return TypeISBN13
	}
	if len(normalized) == 10 && ValidateISBN10(normalized) {
		return TypeISBN10
	}
	return TypeUnknown
}

// NormalizeISBN removes hyphens, spaces, and common prefixes from an ISBN.
func NormalizeISBN(value string) string {
	// Remove common prefixes
	value = strings.TrimPrefix(strings.ToUpper(value), "ISBN:")
	value = strings.TrimPrefix(value, "ISBN")
	value = strings.TrimSpace(value)

	// Keep only digits and X (for ISBN-10 checksum)
	var result strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) || r == 'X' || r == 'x' {
			result.WriteRune(r)
		}
	}
	return strings.ToUpper(result.String())
}

// ValidateISBN10 validates an ISBN-10 checksum.
// ISBN-10 uses modulo 11 with weights 10,9,8,7,6,5,4,3,2,1.
func ValidateISBN10(isbn string) bool {
	if len(isbn) != 10 {
		return false
	}

	var sum int
	for i, r := range isbn {
		var digit int
		if r == 'X' || r == 'x' {
			if i != 9 {
				return false // X only valid as last digit
			}
			digit = 10
		} else if unicode.IsDigit(r) {
			digit = int(r - '0')
		} else {
			return false
		}
		sum += digit * (10 - i)
	}
	return sum%11 == 0
}

// ValidateISBN13 validates an ISBN-13 checksum.
// ISBN-13 uses alternating weights of 1 and 3.
func ValidateISBN13(isbn string) bool {
	if len(isbn) != 13 {
		return false
	}

	var sum int
	for i, r := range isbn {
		if !unicode.IsDigit(r) {
			return false
		}
		digit := int(r - '0')
		if i%2 == 0 {
			sum += digit
		} else {
			sum += digit * 3
		}
	}
	return sum%10 == 0
}
