# PDF Format Reference

This file documents the PDF format as used in Shisho for parsing.

## Metadata Extraction

Metadata is extracted from the PDF info dictionary via pdfcpu.

### Info Dict Fields Extracted

| Field | Info Dict Key | Shisho Usage |
|-------|---------------|--------------|
| Title | `Title` | Book title |
| Authors | `Author` | Split on `,` / `&` / `;` into multiple authors |
| Description | `Subject` | Book description |
| Tags | `Keywords` | Split on `,` / `;` into tag list |
| Release Date | `CreationDate` | PDF date format `D:YYYYMMDDHHmmSSOHH'mm'` |
| Page Count | Page tree | Total page count from PDF structure |

**Data Source:** `models.DataSourcePDFMetadata` ("pdf_metadata")

### PDF Date Format

PDF dates use the format `D:YYYYMMDDHHmmSSOHH'mm'` where:
- `D:` is a literal prefix
- `YYYY` is the year
- `MM` is the month (01-12)
- `DD` is the day (01-31)
- `HH` is the hour (00-23)
- `mm` is the minute (00-59)
- `SS` is the second (00-59)
- `O` is the timezone offset direction (`+`, `-`, or `Z`)
- `HH'mm'` is the timezone offset hours and minutes

Parsing uses pdfcpu's `types.DateTime()` with relaxed mode, plus fallback formats for non-standard dates.

### Author Splitting

The `Author` field is a single string. Multiple authors are split on:
- Comma (`,`) - e.g., "Author One, Author Two"
- Ampersand (`&`) - e.g., "Author One & Author Two"
- Semicolon (`;`) - e.g., "Author One; Author Two"

All PDF authors have an empty role (generic author, same as EPUB).

### Thread Safety

pdfcpu's `NewDefaultConfiguration()` initializes global state (config files, font caches) that is not thread-safe. The parser uses `sync.Once` to ensure this initialization happens exactly once before any concurrent access.

## Key Functions

```go
// Parse metadata from PDF file
func Parse(path string) (*mediafile.ParsedMetadata, error)
```

## Test Fixtures

Test PDFs are created as raw PDF files in `TestMain` rather than using pdfcpu's creation API. This is because pdfcpu's write path always overwrites `CreationDate`, `ModDate`, and `Producer` in the info dict, making it impossible to set a specific `CreationDate` for testing. Raw PDF construction gives full control over the info dict contents.

## Related Files

- `pkg/pdf/pdf.go` - PDF parsing
- `pkg/pdf/pdf_test.go` - PDF parsing tests with fixture generation
- `pkg/mediafile/mediafile.go` - ParsedMetadata type definition
- `pkg/models/data-source.go` - DataSourcePDFMetadata constant
- `pkg/models/file.go` - FileTypePDF constant
