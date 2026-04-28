# PDF Supplement Name Detection

## Problem

Shisho currently treats every `.pdf` file the scanner encounters as a main file (see `pkg/worker/scan.go::extensionsToScan` and the discovery flow in `discoverSupplements`, which skips main-file extensions). When a user drops a companion PDF — e.g. `Supplement.pdf` next to `Book.epub` — into a book directory, the PDF becomes a separate main file (or a second main file on the same book) instead of a supplement attached to the EPUB.

This means:
- Companion PDFs pollute the library as standalone "books" or as redundant main files.
- Users have to manually demote them to supplements every time.

## Goal

When the scanner discovers a PDF whose basename matches a configurable list of common supplement names, classify it as a supplement to the sibling main book, provided there's another file in the same directory tree that can serve as the main file. Never silently drop a book — a directory containing only a supplement-named PDF still imports as a main file.

## Non-goals

- Substring matching of supplement words inside arbitrary filenames (avoids false positives on real books with words like "Companion" in the title).
- Reclassifying existing PDF main files on rescan. The check applies only when a brand-new file row is created.
- Auto-linking root-level PDFs (e.g. `Book - Supplement.pdf` next to `Book.epub` at the library root) — root-level supplement linking is the existing basename-prefix flow in `discoverRootLevelSupplements` and is out of scope here.
- Supplement classification for non-PDF main extensions (`.epub`, `.cbz`, `.m4b`, plugin-registered file parsers).

## Design

### Classification rule

When `scanFileCreateNew` (in `pkg/worker/scan_unified.go`) is creating a new file row, after the `bookPath` and `existingBook` lookup but before the `models.File` is constructed:

If **all** of the following are true, set `FileRole = models.FileRoleSupplement`:

1. `fileType == "pdf"`.
2. The basename (extension stripped, whitespace trimmed, lowercased) appears in the configured supplement-name list.
3. **Either** `existingBook != nil` **or** the directory tree containing the PDF has at least one sibling file with a non-PDF main-eligible extension on disk.

Otherwise, the file is created as `FileRoleMain` (current behavior).

The "either / or" in (3) is what gives correct ordering-independent classification: the on-disk sibling check works even when the EPUB and the PDF are dropped together and `scanFileCreateNew` runs for them in unpredictable order under the parallel worker pool. The DB-existing-book check covers the case where a sibling main file was imported in a previous scan.

### Scope of the sibling check

- **Directory-based books:** walk the book directory recursively (`filepath.WalkDir` rooted at `bookPath`) looking for any non-PDF file with extension in the union of `models.FileTypeEPUB`, `models.FileTypeCBZ`, `models.FileTypeM4B`, and `pluginManager.RegisteredFileExtensions()`. First hit wins; bail early.
- **Root-level files:** skip the sibling check and treat the PDF as main. Root-level files don't share a directory in the book sense, and root-level supplement linking is its own basename-matching flow.

### Match style

Case-insensitive exact match of the trimmed basename against the configured names list. Examples (with default list):
- `Supplement.pdf` → matches `supplement` → supplement.
- `BONUS MATERIAL.pdf` → matches `bonus material` → supplement.
- `Companion Guide.pdf` → does not match any list entry → main.
- `My Book - Supplement.pdf` → does not match → main (substring matching is non-goal).

### Configuration

New field on `pkg/config/config.go::Config`:

```go
PDFSupplementFilenames []string `mapstructure:"pdf_supplement_filenames"`
```

Default value (set in `defaultPDFSupplementFilenames` slice or via `viper.SetDefault`):

```
supplement, supplemental, bonus, bonus material, bonus content,
companion, notes, liner notes, errata, booklet, digital booklet,
appendix, map, maps, insert, guide, reference, cheat sheet,
cheatsheet, cribsheet, pamphlet, extras
```

Setting the YAML key to `[]` (empty list) disables the feature. Omitting the key uses the default. Mirrored in `shisho.example.yaml` and documented in `website/docs/configuration.md`.

### Touch points

| File | Change |
|------|--------|
| `pkg/config/config.go` | Add `PDFSupplementFilenames []string` field with default. |
| `shisho.example.yaml` | Add the field with the default list and a comment. |
| `pkg/worker/scan.go` | Add `looksLikePDFSupplement(filename string, names []string) bool` helper. Add `hasNonPDFMainSibling(dir string, pluginExts map[string]struct{}) (bool, error)` helper. |
| `pkg/worker/scan.go::ProcessScanJob` | After the walk fills `filesToScan`, stable-sort to push supplement-named PDFs to the end. Optimization for log readability and parallel-worker scheduling — not load-bearing for correctness. |
| `pkg/worker/scan_unified.go::scanFileCreateNew` | Compute the FileRole using the rule above and set it on the `models.File` before `CreateFile`. Skip cover extraction (`extractAndSaveCover`) when classified as supplement, matching the existing supplement create path at L2554. |
| `website/docs/configuration.md` | Document `pdf_supplement_filenames`. |
| `website/docs/supplement-files.md` | New section "Supplement-named PDFs" explaining auto-classification and pointing at the config option. |

### Edge cases

| Scenario | Outcome |
|----------|---------|
| `Book.epub` and `Supplement.pdf` dropped together in same directory | EPUB and PDF processed in any order; PDF sees on-disk EPUB sibling → supplement. EPUB has no rule applied → main. |
| Directory contains only `Supplement.pdf` | No existing book at `bookPath`, no non-PDF main sibling on disk → main. Book imports normally. |
| `Supplement.pdf` added later to a directory that already has a book row | `existingBook != nil` → supplement. |
| `Book.epub` deleted leaving only an existing main `Supplement.pdf` | Existing rescan path doesn't reclassify the PDF; orphan cleanup (`scan_orphans.go`) handles the EPUB removal independently. (No change from current behavior.) |
| `Book.epub` deleted leaving an existing supplement `Supplement.pdf` | Existing orphan cleanup at `pkg/worker/scan_orphans.go:174-186` promotes the PDF to main via `PromoteSupplementToMain`. Book survives. (No change from current behavior.) |
| User has manually named a real book "Companion.pdf" and is importing it for the first time | Matches the default list → classified as supplement if a sibling main file exists, or main if it's alone in its directory. If the user imports it next to another book they want, they can demote/promote manually, or remove `companion` from `pdf_supplement_filenames` in config. |
| Plugin-registered file parser extension (e.g. `.azw3`) sits next to `Supplement.pdf` | Sibling check includes plugin-registered file extensions → PDF classified as supplement. |
| Monitor (single-file rescan via `pkg/worker/monitor.go`) creates a new PDF row | Goes through the same `scanFileCreateNew` path; rule applies. |
| Two supplement-named PDFs in a directory with no other main file (`Bonus.pdf` + `Notes.pdf`) | Sort puts both at the end; whichever the worker pool picks first sees no existing book and no sibling main → main. The second sees the first as an existing book → supplement. Deterministic by sort order, but the user effectively has a PDF-only book either way. |

### Tests

Extend `pkg/worker/supplement_test.go` and add cases covering:

1. PDF named `Supplement.pdf` next to an EPUB in a directory → PDF created as `FileRoleSupplement`, EPUB as `FileRoleMain`, both attached to one book.
2. Same as (1) but the PDF is processed before the EPUB (directly invoke `scanInternal` in PDF-then-EPUB order). Outcome must be identical, proving ordering-independence.
3. PDF named `Supplement.pdf` alone in a directory → main. Book imports.
4. PDF named `supplement.PDF` (mixed case) → matches case-insensitively → supplement when sibling exists.
5. PDF named `Companion Guide.pdf` (does not exactly match any list entry) → main.
6. PDF named `Bonus.pdf` next to `Notes.pdf` only → first one main, second one supplement (per sort + sibling rule).
7. Existing main-file PDF whose name happens to match a list entry → unchanged on rescan (no reclassification).
8. Plugin-registered file extension (`.azw3` registered as a parser) sitting next to `Supplement.pdf` → PDF classified as supplement (plugin extensions count as siblings).
9. Empty `pdf_supplement_filenames` list in config → all PDFs classified as main (feature disabled).
10. `ProcessScanJob` sort: supplement-named PDFs end up after non-supplement files in `filesToScan`.

### Verification

After implementation:
- `mise lint test` for the Go-side changes (this is a Go-only change touching scan/config/docs).
- Manually drop a `Supplement.pdf` and a `Book.epub` into a test library directory, trigger a scan, and confirm the PDF is attached as a supplement on the book detail page.
- `mise check:quiet` once before pushing/PR.
