# File Identifiers Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add support for file identifiers (ISBN, ASIN, UUID, etc.) to track unique identifiers for ebook and audiobook files.

**Architecture:** Identifiers are tied to files (not books) because each file represents a different edition with potentially different identifiers. A new `file_identifiers` table stores identifiers with type, value, and source tracking. Parsing logic detects identifier types from EPUB `dc:identifier`, CBZ `<GTIN>`, and M4B freeform atoms.

**Tech Stack:** Go (backend), SQLite (Bun ORM), React/TypeScript (frontend), TailwindCSS

**Reference:** See `docs/plans/2026-01-12-file-identifiers-design.md` for detailed design decisions.

---

## Task 1: Create Identifier Detection Package

**Files:**
- Create: `pkg/identifiers/identifiers.go`
- Create: `pkg/identifiers/identifiers_test.go`

**Step 1: Write failing tests for identifier detection**

```go
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
		{"0804429573", true},
		{"0316769488", false}, // bad checksum
		{"123456789", false},  // too short
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
		{"9780316769489", false}, // bad checksum
		{"978031676948", false},  // too short
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
```

**Step 2: Run tests to verify they fail**

Run: `TZ=America/Chicago CI=true go test ./pkg/identifiers/... -v`
Expected: FAIL with "no Go files in pkg/identifiers"

**Step 3: Write identifier detection implementation**

```go
// pkg/identifiers/identifiers.go
package identifiers

import (
	"regexp"
	"strings"
	"unicode"
)

// Type represents the type of identifier
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

// detectISBNType determines if an ISBN is ISBN-10 or ISBN-13
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

// NormalizeISBN removes hyphens, spaces, and common prefixes from an ISBN
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

// ValidateISBN10 validates an ISBN-10 checksum
// ISBN-10 uses modulo 11 with weights 10,9,8,7,6,5,4,3,2,1
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

// ValidateISBN13 validates an ISBN-13 checksum
// ISBN-13 uses alternating weights of 1 and 3
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
```

**Step 4: Run tests to verify they pass**

Run: `TZ=America/Chicago CI=true go test ./pkg/identifiers/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/identifiers/
git commit -m "$(cat <<'EOF'
[Feature] Add identifier detection package

Add pkg/identifiers with type detection and validation for ISBN-10,
ISBN-13, ASIN, UUID, Goodreads, and Google identifiers. Includes
checksum validation for ISBNs and pattern matching for other types.
EOF
)"
```

---

## Task 2: Create FileIdentifier Model

**Files:**
- Create: `pkg/models/file-identifier.go`
- Modify: `pkg/models/file.go:35` (add Identifiers relation)

**Step 1: Write failing test for FileIdentifier model**

Create a simple compile test to ensure the model is correct.

```go
// pkg/models/file-identifier_test.go
package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileIdentifierFields(t *testing.T) {
	fi := FileIdentifier{
		FileID: 1,
		Type:   "isbn_13",
		Value:  "9780316769488",
		Source: DataSourceEPUBMetadata,
	}
	assert.Equal(t, 1, fi.FileID)
	assert.Equal(t, "isbn_13", fi.Type)
	assert.Equal(t, "9780316769488", fi.Value)
	assert.Equal(t, DataSourceEPUBMetadata, fi.Source)
}
```

**Step 2: Run test to verify it fails**

Run: `TZ=America/Chicago CI=true go test ./pkg/models/... -run TestFileIdentifierFields -v`
Expected: FAIL with "undefined: FileIdentifier"

**Step 3: Write FileIdentifier model**

```go
// pkg/models/file-identifier.go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

// FileIdentifier type constants
const (
	//tygo:emit export type IdentifierType = typeof IdentifierTypeISBN10 | typeof IdentifierTypeISBN13 | typeof IdentifierTypeASIN | typeof IdentifierTypeUUID | typeof IdentifierTypeGoodreads | typeof IdentifierTypeGoogle | typeof IdentifierTypeOther;
	IdentifierTypeISBN10    = "isbn_10"
	IdentifierTypeISBN13    = "isbn_13"
	IdentifierTypeASIN      = "asin"
	IdentifierTypeUUID      = "uuid"
	IdentifierTypeGoodreads = "goodreads"
	IdentifierTypeGoogle    = "google"
	IdentifierTypeOther     = "other"
)

type FileIdentifier struct {
	bun.BaseModel `bun:"table:file_identifiers,alias:fi" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	FileID    int       `bun:",nullzero" json:"file_id"`
	Type      string    `bun:",nullzero" json:"type" tstype:"IdentifierType"`
	Value     string    `bun:",nullzero" json:"value"`
	Source    string    `bun:",nullzero" json:"source" tstype:"DataSource"`
}
```

**Step 4: Add Identifiers relation to File model**

Modify `pkg/models/file.go` to add the Identifiers field after line 35 (after Narrators):

```go
// Add after line 35 (Narrators field)
Identifiers      []*FileIdentifier `bun:"rel:has-many,join:id=file_id" json:"identifiers,omitempty" tstype:"FileIdentifier[]"`
IdentifierSource *string           `json:"identifier_source" tstype:"DataSource"`
```

**Step 5: Run test to verify it passes**

Run: `TZ=America/Chicago CI=true go test ./pkg/models/... -run TestFileIdentifierFields -v`
Expected: PASS

**Step 6: Generate TypeScript types**

Run: `make tygo`
Expected: TypeScript types updated in `app/types/generated/`

**Step 7: Commit**

```bash
git add pkg/models/file-identifier.go pkg/models/file.go pkg/models/file-identifier_test.go
git commit -m "$(cat <<'EOF'
[Feature] Add FileIdentifier model with File relation

Add FileIdentifier model with type constants for ISBN-10, ISBN-13,
ASIN, UUID, Goodreads, Google, and Other. Link to File model via
has-many relation.
EOF
)"
```

---

## Task 3: Create Database Migration

**Files:**
- Create: `pkg/migrations/20260113000000_add_file_identifiers.go`

**Step 1: Create migration file**

```go
// pkg/migrations/20260113000000_add_file_identifiers.go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Create file_identifiers table
		_, err := db.Exec(`
			CREATE TABLE file_identifiers (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
				type TEXT NOT NULL,
				value TEXT NOT NULL,
				source TEXT NOT NULL
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index on file_id for fast lookups by file
		_, err = db.Exec(`CREATE INDEX ix_file_identifiers_file_id ON file_identifiers (file_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index on value for search functionality
		_, err = db.Exec(`CREATE INDEX ix_file_identifiers_value ON file_identifiers (value)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Unique constraint: one identifier of each type per file
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_file_identifiers_file_type ON file_identifiers (file_id, type)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Add identifier_source column to files table
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN identifier_source TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		// SQLite doesn't support DROP COLUMN in older versions, so we skip dropping identifier_source
		// The column will just be unused after rollback

		_, err := db.Exec("DROP TABLE IF EXISTS file_identifiers")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

**Step 2: Run migration**

Run: `make db:migrate`
Expected: Migration applied successfully

**Step 3: Verify migration**

Run: `sqlite3 tmp/data.sqlite ".schema file_identifiers"`
Expected: Table schema matches definition

**Step 4: Test rollback and re-migrate**

Run: `make db:rollback && make db:migrate`
Expected: Both operations succeed

**Step 5: Commit**

```bash
git add pkg/migrations/20260113000000_add_file_identifiers.go
git commit -m "$(cat <<'EOF'
[Database] Add file_identifiers table migration

Create file_identifiers table with file_id FK (CASCADE delete),
type, value, and source columns. Add indexes for file_id and value,
plus unique constraint on (file_id, type).
EOF
)"
```

---

## Task 4: Add ParsedIdentifier to mediafile Package

**Files:**
- Modify: `pkg/mediafile/mediafile.go:17` (add ParsedIdentifier struct)
- Modify: `pkg/mediafile/mediafile.go:42` (add Identifiers field to ParsedMetadata)

**Step 1: Write failing test**

```go
// Add to pkg/mediafile/mediafile_test.go (create if doesn't exist)
package mediafile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsedIdentifier(t *testing.T) {
	id := ParsedIdentifier{
		Type:  "isbn_13",
		Value: "9780316769488",
	}
	assert.Equal(t, "isbn_13", id.Type)
	assert.Equal(t, "9780316769488", id.Value)
}

func TestParsedMetadataIdentifiers(t *testing.T) {
	m := ParsedMetadata{
		Title: "Test Book",
		Identifiers: []ParsedIdentifier{
			{Type: "isbn_13", Value: "9780316769488"},
			{Type: "asin", Value: "B08N5WRWNW"},
		},
	}
	assert.Len(t, m.Identifiers, 2)
	assert.Equal(t, "isbn_13", m.Identifiers[0].Type)
}
```

**Step 2: Run test to verify it fails**

Run: `TZ=America/Chicago CI=true go test ./pkg/mediafile/... -v`
Expected: FAIL with "undefined: ParsedIdentifier"

**Step 3: Add ParsedIdentifier struct**

Add after line 15 in `pkg/mediafile/mediafile.go`:

```go
// ParsedIdentifier represents an identifier parsed from file metadata
type ParsedIdentifier struct {
	Type  string // One of the IdentifierType constants (isbn_10, isbn_13, asin, uuid, goodreads, google, other)
	Value string // The identifier value
}
```

**Step 4: Add Identifiers field to ParsedMetadata**

Add after line 41 (after PageCount field) in `pkg/mediafile/mediafile.go`:

```go
// Identifiers contains file identifiers (ISBN, ASIN, etc.) parsed from metadata
Identifiers []ParsedIdentifier
```

**Step 5: Run test to verify it passes**

Run: `TZ=America/Chicago CI=true go test ./pkg/mediafile/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add pkg/mediafile/
git commit -m "$(cat <<'EOF'
[Feature] Add ParsedIdentifier to mediafile package

Add ParsedIdentifier struct with Type and Value fields. Add
Identifiers slice to ParsedMetadata for parsed identifier storage.
EOF
)"
```

---

## Task 5: Parse EPUB Identifiers

**Files:**
- Modify: `pkg/epub/opf.go` (parse dc:identifier elements)
- Modify: `pkg/epub/opf_test.go` (add identifier tests)

**Step 1: Write failing test for EPUB identifier parsing**

Add to `pkg/epub/opf_test.go`:

```go
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

	opf, err := ParseOPF("test.opf", strings.NewReader(opfXML))
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

	opf, err := ParseOPF("test.opf", strings.NewReader(opfXML))
	require.NoError(t, err)

	assert.Len(t, opf.Identifiers, 2)

	idByType := make(map[string]string)
	for _, id := range opf.Identifiers {
		idByType[id.Type] = id.Value
	}

	assert.Equal(t, "9780316769488", idByType["isbn_13"])
	assert.Equal(t, "0316769487", idByType["isbn_10"])
}
```

**Step 2: Run test to verify it fails**

Run: `TZ=America/Chicago CI=true go test ./pkg/epub/... -run TestParseOPF_Identifiers -v`
Expected: FAIL with field not found or wrong values

**Step 3: Add Identifiers field to OPF struct**

Add after line 32 (after CoverData) in `pkg/epub/opf.go`:

```go
Identifiers []mediafile.ParsedIdentifier
```

**Step 4: Implement identifier parsing in ParseOPF**

Find the ParseOPF function and add identifier parsing after other metadata parsing. Look for where Genres are set (around line 210) and add after:

```go
// Parse identifiers from dc:identifier elements
for _, identifier := range pkg.Metadata.Identifier {
	value := strings.TrimSpace(identifier.Text)
	if value == "" {
		continue
	}
	idType := identifiers.DetectType(value, identifier.Scheme)
	if idType == identifiers.TypeUnknown {
		// Skip unknown identifier types for EPUB
		continue
	}
	opf.Identifiers = append(opf.Identifiers, mediafile.ParsedIdentifier{
		Type:  string(idType),
		Value: value,
	})
}
```

Add import for identifiers package:

```go
"github.com/shishobooks/shisho/pkg/identifiers"
```

**Step 5: Update Parse function to include identifiers**

Find where ParsedMetadata is returned (around line 175) and add:

```go
Identifiers: opf.Identifiers,
```

**Step 6: Run test to verify it passes**

Run: `TZ=America/Chicago CI=true go test ./pkg/epub/... -run TestParseOPF_Identifiers -v`
Expected: PASS

**Step 7: Commit**

```bash
git add pkg/epub/
git commit -m "$(cat <<'EOF'
[Feature] Parse identifiers from EPUB dc:identifier elements

Parse ISBN-10, ISBN-13, ASIN, UUID, Goodreads, and Google identifiers
from EPUB metadata. Uses scheme attribute when present, otherwise
pattern matching. Unknown types are skipped.
EOF
)"
```

---

## Task 6: Parse CBZ Identifiers

**Files:**
- Modify: `pkg/cbz/cbz.go` (parse GTIN element)
- Modify: `pkg/cbz/cbz_test.go` (add identifier tests)

**Step 1: Write failing test for CBZ identifier parsing**

Add to `pkg/cbz/cbz_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `TZ=America/Chicago CI=true go test ./pkg/cbz/... -run TestParseCBZ_Identifiers -v`
Expected: FAIL

**Step 3: Implement GTIN parsing**

Find where ComicInfo is parsed in `pkg/cbz/cbz.go` and add identifier parsing after GTIN is read. Look for where metadata.Tags is set and add after:

```go
// Parse GTIN as identifier
if comicInfo.GTIN != "" {
	gtin := strings.TrimSpace(comicInfo.GTIN)
	idType := identifiers.DetectType(gtin, "")
	if idType == identifiers.TypeUnknown {
		// For CBZ, unknown GTIN is stored as "other"
		idType = identifiers.TypeOther
	}
	metadata.Identifiers = append(metadata.Identifiers, mediafile.ParsedIdentifier{
		Type:  string(idType),
		Value: gtin,
	})
}
```

Add import:

```go
"github.com/shishobooks/shisho/pkg/identifiers"
```

**Step 4: Run test to verify it passes**

Run: `TZ=America/Chicago CI=true go test ./pkg/cbz/... -run TestParseCBZ_Identifiers -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/cbz/
git commit -m "$(cat <<'EOF'
[Feature] Parse identifiers from CBZ GTIN element

Parse GTIN values from ComicInfo.xml and detect type (ISBN-10,
ISBN-13, ASIN, UUID). Unrecognized GTINs are stored as "other"
type, unlike EPUB which skips unknown types.
EOF
)"
```

---

## Task 7: Parse M4B ASIN

**Files:**
- Modify: `pkg/mp4/metadata.go` (parse com.apple.iTunes:ASIN)
- Modify: `pkg/mp4/metadata_test.go` (add ASIN test)

**Step 1: Write failing test for M4B ASIN parsing**

Add test case to existing M4B tests (check existing test patterns in `pkg/mp4/metadata_test.go`):

```go
func TestParseM4B_ASIN(t *testing.T) {
	// Use test helper to generate M4B with ASIN
	tmpDir := t.TempDir()
	m4bPath := filepath.Join(tmpDir, "test.m4b")

	// This test will need a fixture file with ASIN metadata
	// For now, skip if no fixture exists
	fixturePath := "testdata/with_asin.m4b"
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("No ASIN fixture file available")
	}

	metadata, err := Parse(fixturePath)
	require.NoError(t, err)

	require.Len(t, metadata.Identifiers, 1)
	assert.Equal(t, "asin", metadata.Identifiers[0].Type)
}
```

**Step 2: Check existing freeform atom parsing**

Look in `pkg/mp4/metadata.go` for how freeform atoms are parsed. Find the pattern for `com.shisho:tags` or similar.

**Step 3: Add ASIN parsing**

Find where freeform atoms are processed and add:

```go
// Parse ASIN from freeform atom
if atom.Name == "com.apple.iTunes:ASIN" {
	asin := strings.TrimSpace(string(atom.Data))
	if asin != "" {
		metadata.Identifiers = append(metadata.Identifiers, mediafile.ParsedIdentifier{
			Type:  string(identifiers.TypeASIN),
			Value: asin,
		})
	}
}
```

**Step 4: Run all M4B tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/mp4/... -v`
Expected: PASS (or skip if no fixture)

**Step 5: Commit**

```bash
git add pkg/mp4/
git commit -m "$(cat <<'EOF'
[Feature] Parse ASIN from M4B freeform atoms

Parse com.apple.iTunes:ASIN freeform atom for audiobook ASIN
identifier support.
EOF
)"
```

---

## Task 8: Add Identifiers to Sidecar Types

**Files:**
- Modify: `pkg/sidecar/types.go` (add IdentifierMetadata struct and FileSidecar.Identifiers)
- Modify: `pkg/sidecar/sidecar.go` (update FileSidecarFromModel)

**Step 1: Add IdentifierMetadata struct and field**

Add to `pkg/sidecar/types.go` after NarratorMetadata:

```go
// IdentifierMetadata represents an identifier in the sidecar file.
type IdentifierMetadata struct {
	Type  string `json:"type"`  // isbn_10, isbn_13, asin, uuid, goodreads, google, other
	Value string `json:"value"`
}
```

Add to FileSidecar struct (after Imprint field):

```go
Identifiers []IdentifierMetadata `json:"identifiers,omitempty"`
```

**Step 2: Update FileSidecarFromModel**

Find `FileSidecarFromModel` in `pkg/sidecar/sidecar.go` and add identifier mapping after other fields:

```go
// Map identifiers
if len(file.Identifiers) > 0 {
	s.Identifiers = make([]IdentifierMetadata, len(file.Identifiers))
	for i, id := range file.Identifiers {
		s.Identifiers[i] = IdentifierMetadata{
			Type:  id.Type,
			Value: id.Value,
		}
	}
}
```

**Step 3: Run sidecar tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/sidecar/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add pkg/sidecar/
git commit -m "$(cat <<'EOF'
[Feature] Add identifiers to sidecar types

Add IdentifierMetadata struct and Identifiers field to FileSidecar.
Update FileSidecarFromModel to include file identifiers in sidecar
export.
EOF
)"
```

---

## Task 9: Update Scan Worker to Save Identifiers

**Files:**
- Modify: `pkg/worker/scan.go` (save identifiers from parsed metadata)
- Modify: `pkg/books/service.go` (add CreateFileIdentifier method)

**Step 1: Add CreateFileIdentifier to books service**

Find `pkg/books/service.go` and add after existing Create methods:

```go
// CreateFileIdentifier creates a new file identifier record
func (s *Service) CreateFileIdentifier(ctx context.Context, identifier *models.FileIdentifier) error {
	identifier.CreatedAt = time.Now()
	identifier.UpdatedAt = time.Now()
	_, err := s.db.NewInsert().Model(identifier).Exec(ctx)
	return errors.WithStack(err)
}

// DeleteFileIdentifiers deletes all identifiers for a file
func (s *Service) DeleteFileIdentifiers(ctx context.Context, fileID int) error {
	_, err := s.db.NewDelete().Model((*models.FileIdentifier)(nil)).Where("file_id = ?", fileID).Exec(ctx)
	return errors.WithStack(err)
}
```

**Step 2: Update scan.go to save identifiers**

Find where file is created in `pkg/worker/scan.go` (around line 925) and add identifier saving after file creation. Look for where narrators are saved and follow the same pattern:

Add variables after narrator variables (around line 266):

```go
var identifiers []mediafile.ParsedIdentifier
identifierSource := models.DataSourceFilepath
```

Add identifier extraction after metadata processing (around line 386, after tag processing):

```go
if len(metadata.Identifiers) > 0 {
	identifiers = metadata.Identifiers
	identifierSource = metadata.DataSource
}
```

Add sidecar identifier processing (after sidecar narrator processing, around line 965):

```go
// Apply file sidecar data for identifiers (higher priority than file metadata)
if fileSidecarData != nil && len(fileSidecarData.Identifiers) > 0 {
	if models.DataSourcePriority[models.DataSourceSidecar] < models.DataSourcePriority[identifierSource] {
		jobLog.Info("applying file sidecar data for identifiers", logger.Data{"identifier_count": len(fileSidecarData.Identifiers)})
		identifierSource = models.DataSourceSidecar
		identifiers = make([]mediafile.ParsedIdentifier, 0)
		for _, id := range fileSidecarData.Identifiers {
			identifiers = append(identifiers, mediafile.ParsedIdentifier{
				Type:  id.Type,
				Value: id.Value,
			})
		}
	}
}
```

Add identifier saving after file creation (after narrator saving):

```go
// Create FileIdentifier entries for each identifier
if len(identifiers) > 0 {
	file.IdentifierSource = &identifierSource
	err = w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
		Columns: []string{"identifier_source"},
	})
	if err != nil {
		jobLog.Error("failed to update file identifier source", nil, logger.Data{"file_id": file.ID, "error": err.Error()})
	}
	for _, parsedId := range identifiers {
		fileId := &models.FileIdentifier{
			FileID: file.ID,
			Type:   parsedId.Type,
			Value:  parsedId.Value,
			Source: identifierSource,
		}
		err = w.bookService.CreateFileIdentifier(ctx, fileId)
		if err != nil {
			jobLog.Error("failed to create file identifier", nil, logger.Data{"file_id": file.ID, "type": parsedId.Type, "error": err.Error()})
		}
	}
}
```

**Step 3: Run scan tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/worker/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add pkg/worker/scan.go pkg/books/service.go
git commit -m "$(cat <<'EOF'
[Feature] Save file identifiers during scan

Add CreateFileIdentifier and DeleteFileIdentifiers to books service.
Update scan worker to extract identifiers from parsed metadata and
sidecar files, then save to database with source tracking.
EOF
)"
```

---

## Task 10: Add Identifiers to Book Service Queries

**Files:**
- Modify: `pkg/books/service.go` (add .Relation("Files.Identifiers") to query methods)

**Step 1: Find and update RetrieveBook**

Look for RetrieveBook method and add Identifiers relation to File loading:

```go
.Relation("Files.Identifiers")
```

**Step 2: Find and update ListBooks**

Look for ListBooks method and add Identifiers relation if Files are loaded.

**Step 3: Run book service tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/books/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add pkg/books/service.go
git commit -m "$(cat <<'EOF'
[Feature] Load file identifiers in book queries

Add .Relation("Files.Identifiers") to RetrieveBook and ListBooks
query methods to eager-load identifiers with files.
EOF
)"
```

---

## Task 11: Add Identifiers to Download Cache Fingerprint

**Files:**
- Modify: `pkg/downloadcache/fingerprint.go` (add identifiers to Fingerprint)

**Step 1: Add FingerprintIdentifier struct**

Add after FingerprintSeries struct:

```go
type FingerprintIdentifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
```

**Step 2: Add Identifiers field to Fingerprint struct**

Add to Fingerprint struct:

```go
Identifiers []FingerprintIdentifier `json:"identifiers,omitempty"`
```

**Step 3: Update ComputeFingerprint**

Find ComputeFingerprint and add identifier processing (similar to how narrators are handled):

```go
// Add identifiers from all files (sorted by type, then value for consistency)
var allIdentifiers []FingerprintIdentifier
for _, file := range book.Files {
	for _, id := range file.Identifiers {
		allIdentifiers = append(allIdentifiers, FingerprintIdentifier{
			Type:  id.Type,
			Value: id.Value,
		})
	}
}
sort.Slice(allIdentifiers, func(i, j int) bool {
	if allIdentifiers[i].Type != allIdentifiers[j].Type {
		return allIdentifiers[i].Type < allIdentifiers[j].Type
	}
	return allIdentifiers[i].Value < allIdentifiers[j].Value
})
fp.Identifiers = allIdentifiers
```

**Step 4: Run fingerprint tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/downloadcache/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/downloadcache/fingerprint.go
git commit -m "$(cat <<'EOF'
[Feature] Include identifiers in download cache fingerprint

Add FingerprintIdentifier struct and include file identifiers in
fingerprint computation for download cache invalidation.
EOF
)"
```

---

## Task 12: Add Search by Identifier

**Files:**
- Modify: `pkg/search/service.go` (add identifier search)

**Step 1: Find unified search implementation**

Look for the search method that handles book search and understand how it works.

**Step 2: Add identifier search**

Add a separate query or modify existing query to check `file_identifiers.value`:

```go
// Check if query looks like an identifier (ISBN, ASIN pattern)
// If so, also search file_identifiers table
identifierMatches, err := svc.db.NewSelect().
	TableExpr("file_identifiers fi").
	ColumnExpr("DISTINCT f.book_id").
	Join("JOIN files f ON f.id = fi.file_id").
	Where("fi.value = ?", query).
	Where("f.library_id = ?", libraryID).
	Exec(ctx)
```

**Step 3: Run search tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/search/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add pkg/search/service.go
git commit -m "$(cat <<'EOF'
[Feature] Add identifier search support

Add exact match search on file_identifiers.value column to support
searching books by ISBN, ASIN, or other identifiers.
EOF
)"
```

---

## Task 13: Write Identifiers to EPUB

**Files:**
- Modify: `pkg/filegen/epub.go` (write dc:identifier elements)
- Modify: `pkg/filegen/epub_test.go` (add identifier test)

**Step 1: Write failing test**

Add to `pkg/filegen/epub_test.go`:

```go
func TestGenerateEPUB_Identifiers(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.epub")

	opts := GenerateOptions{
		Title: "Test Book",
		Identifiers: []IdentifierOption{
			{Type: "isbn_13", Value: "9780316769488"},
			{Type: "asin", Value: "B08N5WRWNW"},
		},
	}

	err := GenerateEPUB(outPath, opts)
	require.NoError(t, err)

	// Parse and verify
	metadata, err := epub.Parse(outPath)
	require.NoError(t, err)

	require.Len(t, metadata.Identifiers, 2)
	// Verify identifiers are present
	idByType := make(map[string]string)
	for _, id := range metadata.Identifiers {
		idByType[id.Type] = id.Value
	}
	assert.Equal(t, "9780316769488", idByType["isbn_13"])
	assert.Equal(t, "B08N5WRWNW", idByType["asin"])
}
```

**Step 2: Run test to verify it fails**

Run: `TZ=America/Chicago CI=true go test ./pkg/filegen/... -run TestGenerateEPUB_Identifiers -v`
Expected: FAIL

**Step 3: Add IdentifierOption struct**

Add to `pkg/filegen/types.go` or at top of epub.go:

```go
type IdentifierOption struct {
	Type  string
	Value string
}
```

Add Identifiers field to GenerateOptions:

```go
Identifiers []IdentifierOption
```

**Step 4: Implement identifier writing**

Find where OPF XML is generated and add dc:identifier elements:

```go
// Write identifiers
for _, id := range opts.Identifiers {
	scheme := identifierTypeToScheme(id.Type)
	if scheme != "" {
		fmt.Fprintf(opf, `    <dc:identifier opf:scheme="%s">%s</dc:identifier>\n`, scheme, html.EscapeString(id.Value))
	}
}
```

Add helper function:

```go
func identifierTypeToScheme(idType string) string {
	switch idType {
	case "isbn_10", "isbn_13":
		return "ISBN"
	case "asin":
		return "ASIN"
	case "uuid":
		return "UUID"
	case "goodreads":
		return "GOODREADS"
	case "google":
		return "GOOGLE"
	default:
		return ""
	}
}
```

**Step 5: Run test to verify it passes**

Run: `TZ=America/Chicago CI=true go test ./pkg/filegen/... -run TestGenerateEPUB_Identifiers -v`
Expected: PASS

**Step 6: Commit**

```bash
git add pkg/filegen/
git commit -m "$(cat <<'EOF'
[Feature] Write identifiers to generated EPUB files

Add identifier writing to EPUB generation with proper scheme
attributes (ISBN, ASIN, UUID, GOODREADS, GOOGLE).
EOF
)"
```

---

## Task 14: Write Identifiers to CBZ

**Files:**
- Modify: `pkg/filegen/cbz.go` (write GTIN element)
- Modify: `pkg/filegen/cbz_test.go` (add identifier test)

**Step 1: Write failing test**

```go
func TestGenerateCBZ_Identifiers(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.cbz")

	opts := GenerateOptions{
		Title: "Test Comic",
		Identifiers: []IdentifierOption{
			{Type: "isbn_13", Value: "9780316769488"},
			{Type: "isbn_10", Value: "0316769487"},
			{Type: "asin", Value: "B08N5WRWNW"},
		},
	}

	err := GenerateCBZ(outPath, opts)
	require.NoError(t, err)

	// Parse and verify - should use ISBN-13 (highest priority)
	metadata, err := cbz.Parse(outPath)
	require.NoError(t, err)

	require.Len(t, metadata.Identifiers, 1)
	assert.Equal(t, "isbn_13", metadata.Identifiers[0].Type)
	assert.Equal(t, "9780316769488", metadata.Identifiers[0].Value)
}
```

**Step 2: Run test to verify it fails**

Run: `TZ=America/Chicago CI=true go test ./pkg/filegen/... -run TestGenerateCBZ_Identifiers -v`
Expected: FAIL

**Step 3: Implement GTIN writing with priority**

Find where ComicInfo.xml is generated and add GTIN:

```go
// Write GTIN (use priority: ISBN-13 > ISBN-10 > Other > ASIN)
gtin := selectGTIN(opts.Identifiers)
if gtin != "" {
	fmt.Fprintf(comicInfo, "  <GTIN>%s</GTIN>\n", html.EscapeString(gtin))
}
```

Add helper function:

```go
func selectGTIN(identifiers []IdentifierOption) string {
	priorityOrder := []string{"isbn_13", "isbn_10", "other", "asin"}
	for _, priority := range priorityOrder {
		for _, id := range identifiers {
			if id.Type == priority {
				return id.Value
			}
		}
	}
	return ""
}
```

**Step 4: Run test to verify it passes**

Run: `TZ=America/Chicago CI=true go test ./pkg/filegen/... -run TestGenerateCBZ_Identifiers -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/filegen/
git commit -m "$(cat <<'EOF'
[Feature] Write GTIN to generated CBZ files

Add GTIN writing to CBZ generation with priority selection:
ISBN-13 > ISBN-10 > Other > ASIN.
EOF
)"
```

---

## Task 15: Write ASIN to M4B

**Files:**
- Modify: `pkg/filegen/m4b.go` (write com.apple.iTunes:ASIN atom)
- Modify: `pkg/filegen/m4b_test.go` (add ASIN test)

**Step 1: Write failing test**

```go
func TestGenerateM4B_ASIN(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.m4b")

	opts := GenerateOptions{
		Title: "Test Audiobook",
		Identifiers: []IdentifierOption{
			{Type: "asin", Value: "B08N5WRWNW"},
		},
	}

	err := GenerateM4B(outPath, opts)
	require.NoError(t, err)

	// Parse and verify
	metadata, err := mp4.Parse(outPath)
	require.NoError(t, err)

	require.Len(t, metadata.Identifiers, 1)
	assert.Equal(t, "asin", metadata.Identifiers[0].Type)
	assert.Equal(t, "B08N5WRWNW", metadata.Identifiers[0].Value)
}
```

**Step 2: Run test to verify it fails**

Run: `TZ=America/Chicago CI=true go test ./pkg/filegen/... -run TestGenerateM4B_ASIN -v`
Expected: FAIL

**Step 3: Implement ASIN atom writing**

Find where freeform atoms are written and add:

```go
// Write ASIN freeform atom if present
for _, id := range opts.Identifiers {
	if id.Type == "asin" {
		// Write com.apple.iTunes:ASIN freeform atom
		// Use the same pattern as tags freeform atom
		writeASINAtom(writer, id.Value)
		break
	}
}
```

**Step 4: Run test to verify it passes**

Run: `TZ=America/Chicago CI=true go test ./pkg/filegen/... -run TestGenerateM4B_ASIN -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/filegen/
git commit -m "$(cat <<'EOF'
[Feature] Write ASIN to generated M4B files

Add com.apple.iTunes:ASIN freeform atom writing to M4B generation
for audiobook ASIN support.
EOF
)"
```

---

## Task 16: Add File Update Handler for Identifiers

**Files:**
- Modify: `pkg/handlers/books.go` (handle identifier updates in UpdateFile)

**Step 1: Add identifier update handling**

Find UpdateFile handler and add identifier processing similar to narrator handling:

```go
// Handle identifier updates
if payload.Identifiers != nil {
	// Delete existing identifiers
	err := h.bookService.DeleteFileIdentifiers(ctx, file.ID)
	if err != nil {
		return err
	}
	// Create new identifiers
	for _, id := range *payload.Identifiers {
		fileId := &models.FileIdentifier{
			FileID: file.ID,
			Type:   id.Type,
			Value:  id.Value,
			Source: models.DataSourceManual,
		}
		err = h.bookService.CreateFileIdentifier(ctx, fileId)
		if err != nil {
			return err
		}
	}
	file.IdentifierSource = &models.DataSourceManual
	updateColumns = append(updateColumns, "identifier_source")
}
```

**Step 2: Add IdentifierPayload struct**

```go
type IdentifierPayload struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
```

Add to UpdateFilePayload:

```go
Identifiers *[]IdentifierPayload `json:"identifiers"`
```

**Step 3: Run handler tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/handlers/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add pkg/handlers/books.go
git commit -m "$(cat <<'EOF'
[Feature] Add identifier update support to file handler

Handle identifier updates in UpdateFile endpoint. Delete existing
identifiers and create new ones with manual source.
EOF
)"
```

---

## Task 17: Run Full Test Suite

**Files:**
- None (verification only)

**Step 1: Run all tests**

Run: `make check`
Expected: All tests pass, linting passes

**Step 2: If any failures, fix them**

Debug and fix any failing tests.

**Step 3: Commit any fixes**

```bash
git add -A
git commit -m "$(cat <<'EOF'
[Fix] Address test failures from identifier implementation
EOF
)"
```

---

## Task 18: Frontend - Display Identifiers in File Details

**Files:**
- Modify: `app/components/pages/BookDetail.tsx` (display identifiers)

**Step 1: Add identifier display in file section**

Find the file rendering loop (around line 443-530) and add identifier display after file metadata:

```tsx
{/* Identifiers */}
{file.identifiers && file.identifiers.length > 0 && (
  <div className="flex flex-wrap gap-1 mt-1">
    {file.identifiers.map((id, idx) => (
      <Badge key={idx} variant="outline" className="text-xs">
        <span className="font-medium uppercase">{formatIdentifierType(id.type)}</span>
        <span className="mx-1">:</span>
        <span className="font-mono select-all">{id.value}</span>
      </Badge>
    ))}
  </div>
)}
```

Add helper function:

```tsx
function formatIdentifierType(type: string): string {
  switch (type) {
    case "isbn_10": return "ISBN-10";
    case "isbn_13": return "ISBN-13";
    case "asin": return "ASIN";
    case "uuid": return "UUID";
    case "goodreads": return "Goodreads";
    case "google": return "Google";
    case "other": return "Other";
    default: return type;
  }
}
```

**Step 2: Verify display works**

Start dev server and check a book with identifiers displays correctly.

**Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[UI] Display file identifiers in book detail view

Show identifiers as badges with formatted type labels and
monospace, selectable values.
EOF
)"
```

---

## Task 19: Frontend - Edit Identifiers in FileEditDialog

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx` (add identifier editing)

**Step 1: Add identifier state**

Add after narrator state (around line 50):

```tsx
const [identifiers, setIdentifiers] = useState<Array<{type: string; value: string}>>(
  file.identifiers?.map(id => ({ type: id.type, value: id.value })) || []
);
const [newIdentifierType, setNewIdentifierType] = useState<string>("isbn_13");
const [newIdentifierValue, setNewIdentifierValue] = useState("");
```

**Step 2: Add identifier UI section**

Add after narrator section in the dialog (similar pattern):

```tsx
{/* Identifiers */}
<div className="space-y-2">
  <Label>Identifiers</Label>
  <div className="flex flex-wrap gap-2">
    {identifiers.map((id, idx) => (
      <Badge key={idx} variant="secondary" className="gap-1">
        <span className="uppercase text-xs">{formatIdentifierType(id.type)}</span>: {id.value}
        <button
          type="button"
          className="ml-1 hover:text-destructive"
          onClick={() => {
            setIdentifiers(identifiers.filter((_, i) => i !== idx));
          }}
        >
          <X className="h-3 w-3" />
        </button>
      </Badge>
    ))}
  </div>
  <div className="flex gap-2">
    <Select value={newIdentifierType} onValueChange={setNewIdentifierType}>
      <SelectTrigger className="w-32">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="isbn_10">ISBN-10</SelectItem>
        <SelectItem value="isbn_13">ISBN-13</SelectItem>
        <SelectItem value="asin">ASIN</SelectItem>
        <SelectItem value="uuid">UUID</SelectItem>
        <SelectItem value="goodreads">Goodreads</SelectItem>
        <SelectItem value="google">Google</SelectItem>
        <SelectItem value="other">Other</SelectItem>
      </SelectContent>
    </Select>
    <Input
      placeholder="Enter value..."
      value={newIdentifierValue}
      onChange={(e) => setNewIdentifierValue(e.target.value)}
      className="flex-1"
    />
    <Button
      type="button"
      variant="outline"
      onClick={() => {
        if (newIdentifierValue.trim()) {
          setIdentifiers([...identifiers, { type: newIdentifierType, value: newIdentifierValue.trim() }]);
          setNewIdentifierValue("");
        }
      }}
    >
      Add
    </Button>
  </div>
</div>
```

**Step 3: Add identifier change detection in submit**

In the handleSubmit function, add:

```tsx
const originalIdentifiers = file.identifiers?.map(id => ({ type: id.type, value: id.value })) || [];
if (JSON.stringify(identifiers) !== JSON.stringify(originalIdentifiers)) {
  payload.identifiers = identifiers;
}
```

**Step 4: Reset state on dialog open**

In the useEffect that resets form state:

```tsx
setIdentifiers(file.identifiers?.map(id => ({ type: id.type, value: id.value })) || []);
setNewIdentifierType("isbn_13");
setNewIdentifierValue("");
```

**Step 5: Verify editing works**

Test adding, removing, and saving identifiers.

**Step 6: Commit**

```bash
git add app/components/library/FileEditDialog.tsx
git commit -m "$(cat <<'EOF'
[UI] Add identifier editing to FileEditDialog

Add identifier management with type dropdown, value input, and
badge display with remove buttons.
EOF
)"
```

---

## Task 20: Add Identifier Validation (Frontend)

**Files:**
- Create: `app/utils/identifiers.ts`
- Modify: `app/components/library/FileEditDialog.tsx` (add validation)

**Step 1: Create validation utilities**

```tsx
// app/utils/identifiers.ts

export function validateISBN10(isbn: string): boolean {
  const normalized = isbn.replace(/[-\s]/g, "").toUpperCase();
  if (normalized.length !== 10) return false;

  let sum = 0;
  for (let i = 0; i < 10; i++) {
    const char = normalized[i];
    const digit = char === "X" ? 10 : parseInt(char, 10);
    if (isNaN(digit) && char !== "X") return false;
    if (char === "X" && i !== 9) return false;
    sum += digit * (10 - i);
  }
  return sum % 11 === 0;
}

export function validateISBN13(isbn: string): boolean {
  const normalized = isbn.replace(/[-\s]/g, "");
  if (normalized.length !== 13) return false;

  let sum = 0;
  for (let i = 0; i < 13; i++) {
    const digit = parseInt(normalized[i], 10);
    if (isNaN(digit)) return false;
    sum += digit * (i % 2 === 0 ? 1 : 3);
  }
  return sum % 10 === 0;
}

export function validateASIN(asin: string): boolean {
  return /^B0[A-Z0-9]{8}$/i.test(asin);
}

export function validateUUID(uuid: string): boolean {
  return /^(?:urn:uuid:)?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(uuid);
}

export function validateIdentifier(type: string, value: string): { valid: boolean; error?: string } {
  switch (type) {
    case "isbn_10":
      return validateISBN10(value)
        ? { valid: true }
        : { valid: false, error: "Invalid ISBN-10 checksum" };
    case "isbn_13":
      return validateISBN13(value)
        ? { valid: true }
        : { valid: false, error: "Invalid ISBN-13 checksum" };
    case "asin":
      return validateASIN(value)
        ? { valid: true }
        : { valid: false, error: "ASIN must be 10 alphanumeric characters starting with B0" };
    case "uuid":
      return validateUUID(value)
        ? { valid: true }
        : { valid: false, error: "Invalid UUID format" };
    default:
      return { valid: true };
  }
}
```

**Step 2: Add validation to FileEditDialog**

Add validation before adding identifier:

```tsx
const handleAddIdentifier = () => {
  if (!newIdentifierValue.trim()) return;

  const validation = validateIdentifier(newIdentifierType, newIdentifierValue.trim());
  if (!validation.valid) {
    // Show error toast or inline error
    toast.error(validation.error);
    return;
  }

  setIdentifiers([...identifiers, { type: newIdentifierType, value: newIdentifierValue.trim() }]);
  setNewIdentifierValue("");
};
```

**Step 3: Run frontend linting**

Run: `yarn lint`
Expected: PASS

**Step 4: Commit**

```bash
git add app/utils/identifiers.ts app/components/library/FileEditDialog.tsx
git commit -m "$(cat <<'EOF'
[UI] Add identifier validation

Add frontend validation for ISBN-10, ISBN-13, ASIN, and UUID
formats with user-friendly error messages.
EOF
)"
```

---

## Task 21: Final Integration Test

**Files:**
- Create: `pkg/worker/scan_identifiers_test.go`

**Step 1: Write integration test**

```go
func TestProcessScanJob_Identifiers(t *testing.T) {
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[John Doe] My Test Book")
	testgen.GenerateEPUB(t, bookDir, "test.epub", testgen.EPUBOptions{
		Title:   "My Book",
		Authors: []string{"Jane Smith"},
		Identifiers: []testgen.IdentifierOption{
			{Type: "isbn_13", Value: "9780316769488"},
			{Type: "asin", Value: "B08N5WRWNW"},
		},
	})

	err := tc.runScan()
	require.NoError(t, err)

	books := tc.listBooks()
	require.Len(t, books, 1)
	require.Len(t, books[0].Files, 1)
	require.Len(t, books[0].Files[0].Identifiers, 2)

	idByType := make(map[string]string)
	for _, id := range books[0].Files[0].Identifiers {
		idByType[id.Type] = id.Value
	}
	assert.Equal(t, "9780316769488", idByType["isbn_13"])
	assert.Equal(t, "B08N5WRWNW", idByType["asin"])
}
```

**Step 2: Update testgen to support identifiers**

Add IdentifierOption to testgen.EPUBOptions and implement in GenerateEPUB.

**Step 3: Run integration test**

Run: `TZ=America/Chicago CI=true go test ./pkg/worker/... -run TestProcessScanJob_Identifiers -v`
Expected: PASS

**Step 4: Run full test suite**

Run: `make check`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/worker/scan_identifiers_test.go
git commit -m "$(cat <<'EOF'
[Test] Add integration test for identifier scanning

Test end-to-end identifier parsing and storage from EPUB files
during library scan.
EOF
)"
```

---

## Task 22: Final Cleanup and Documentation

**Files:**
- Modify: `docs/plans/2026-01-12-file-identifiers-design.md` (mark as implemented)

**Step 1: Update design doc**

Add implementation status header:

```markdown
> **Status:** Implemented (2026-01-13)
```

**Step 2: Run final checks**

Run: `make check`
Expected: PASS

**Step 3: Commit**

```bash
git add docs/plans/2026-01-12-file-identifiers-design.md
git commit -m "$(cat <<'EOF'
[Docs] Mark file identifiers design as implemented
EOF
)"
```

---

## Summary

This plan covers:
1. **Backend:** Identifier detection package, FileIdentifier model, migration, parsing (EPUB/CBZ/M4B), sidecar support, scan worker integration, search, file generation
2. **Frontend:** Display and editing in BookDetail and FileEditDialog with validation
3. **Testing:** Unit tests for each component plus integration tests

Total: 22 tasks with TDD approach (test first, then implement)
