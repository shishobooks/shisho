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

// NormalizeValue returns a canonical form of an identifier value for storage,
// based on its type. ISBN-10/13 values are stripped of hyphens/spaces/prefixes;
// ASINs are uppercased; UUIDs are lowercased with any urn:uuid: prefix removed;
// all other types are returned with surrounding whitespace trimmed.
func NormalizeValue(identifierType, value string) string {
	value = strings.TrimSpace(value)
	switch Type(identifierType) {
	case TypeISBN10, TypeISBN13:
		return NormalizeISBN(value)
	case TypeASIN:
		return strings.ToUpper(value)
	case TypeUUID:
		lower := strings.ToLower(value)
		return strings.TrimPrefix(lower, "urn:uuid:")
	case TypeGoodreads, TypeGoogle, TypeOther, TypeUnknown:
		// Fallthrough to trim-only return below.
	}
	return value
}

// Key returns a stable comparison key for an identifier, using the canonical
// form of the value. Two identifiers with semantically identical values but
// cosmetic differences (hyphens, prefixes, case) produce the same Key, so
// diff-based code (e.g. scan reconciliation) can tell when a set has actually
// changed without treating "978-0-316-76948-8" and "9780316769488" as distinct.
func Key(identifierType, value string) string {
	return identifierType + ":" + NormalizeValue(identifierType, value)
}

// CandidateForms returns all plausible canonical forms of a user-provided
// identifier value for lookup purposes. Because the type is not known at query
// time (a user may search by ISBN, ASIN, or UUID without specifying which),
// this enumerates the normalized variants across the supported types so a
// single query can match legacy rows stored in any format. The raw input is
// always included as the first element. Duplicates are removed. Returns nil
// for an empty or whitespace-only input.
func CandidateForms(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	seen := map[string]bool{value: true}
	out := []string{value}
	addIfNew := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	addIfNew(NormalizeISBN(value))
	addIfNew(strings.ToUpper(value))
	addIfNew(strings.TrimPrefix(strings.ToLower(value), "urn:uuid:"))
	return out
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
