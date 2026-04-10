# Language and Abridged Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `language` (BCP 47 tag) and `abridged` (nullable boolean) fields to files, with full stack support: model, migration, parsers, scanner, sidecar, edit API, file generation, fingerprint, plugin SDK, frontend edit UI, book detail display, and gallery language filter.

**Architecture:** Two new nullable columns on the `files` table with data source tracking. Parsed from EPUB/CBZ/M4B/PDF where supported, editable via API and UI, persisted in sidecars, written back to files on download. A language validation/normalization utility using `golang.org/x/text/language` ensures BCP 47 correctness throughout. A curated language list powers the frontend combobox with free-text fallback.

**Tech Stack:** Go (Echo, Bun, golang.org/x/text/language), React (TypeScript, Tanstack Query, shadcn/ui combobox), SQLite

**Spec:** `docs/superpowers/specs/2026-04-07-language-abridged-fields-design.md`

---

### Task 1: Language Validation Utility

**Files:**
- Create: `pkg/mediafile/language.go`
- Test: `pkg/mediafile/language_test.go`

- [ ] **Step 1: Write failing tests for language validation**

```go
// pkg/mediafile/language_test.go
package mediafile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected *string
	}{
		{"empty string returns nil", "", nil},
		{"valid ISO 639-1 passes through", "en", ptr("en")},
		{"valid BCP 47 with region", "en-US", ptr("en-US")},
		{"valid BCP 47 with script", "zh-Hans", ptr("zh-Hans")},
		{"valid BCP 47 full tag", "zh-Hans-CN", ptr("zh-Hans-CN")},
		{"ISO 639-2/T 3-letter code normalized", "eng", ptr("en")},
		{"ISO 639-2/T 3-letter code French", "fra", ptr("fr")},
		{"ISO 639-2/T 3-letter code German", "deu", ptr("de")},
		{"case normalized", "EN-us", ptr("en-US")},
		{"invalid tag returns nil", "not-a-language", nil},
		{"und (undetermined) returns nil", "und", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := NormalizeLanguage(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func ptr(s string) *string { return &s }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && go test ./pkg/mediafile/ -run TestNormalizeLanguage -v`
Expected: FAIL — `NormalizeLanguage` not defined.

- [ ] **Step 3: Add `golang.org/x/text` dependency**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && go get golang.org/x/text/language`

- [ ] **Step 4: Write the implementation**

```go
// pkg/mediafile/language.go
package mediafile

import (
	"strings"

	"golang.org/x/text/language"
)

// NormalizeLanguage validates and canonicalizes a language string to a BCP 47 tag.
// Accepts ISO 639-1 ("en"), ISO 639-2/T ("eng"), and BCP 47 ("en-US", "zh-Hans").
// Returns nil for empty, invalid, or undetermined ("und") inputs.
func NormalizeLanguage(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	tag, err := language.Parse(s)
	if err != nil {
		return nil
	}

	// Reject "und" (undetermined) — means the library couldn't resolve the tag
	if tag == language.Und {
		return nil
	}

	result := tag.String()
	return &result
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && go test ./pkg/mediafile/ -run TestNormalizeLanguage -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/mediafile/language.go pkg/mediafile/language_test.go go.mod go.sum
git commit -m "[Backend] Add BCP 47 language validation and normalization utility"
```

---

### Task 2: Database Migration and Model

**Files:**
- Create: `pkg/migrations/20260407000000_add_language_abridged.go`
- Modify: `pkg/models/file.go:62` (after `ChapterSource`)

- [ ] **Step 1: Add fields to File model**

In `pkg/models/file.go`, after line 62 (`ChapterSource *string ...`), add:

```go
	Language       *string `json:"language"`
	LanguageSource *string `json:"language_source" tstype:"DataSource"`
	Abridged       *bool   `json:"abridged"`
	AbridgedSource *string `json:"abridged_source" tstype:"DataSource"`
```

- [ ] **Step 2: Create migration**

```go
// pkg/migrations/20260407000000_add_language_abridged.go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE files ADD COLUMN language TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN language_source TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN abridged INTEGER`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN abridged_source TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE files DROP COLUMN language`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files DROP COLUMN language_source`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files DROP COLUMN abridged`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE files DROP COLUMN abridged_source`)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 3: Run migration**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise db:migrate`
Expected: Migration applied successfully.

- [ ] **Step 4: Generate TypeScript types**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise tygo`

- [ ] **Step 5: Verify rollback works**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise db:rollback && mise db:migrate`

- [ ] **Step 6: Commit**

```bash
git add pkg/models/file.go pkg/migrations/20260407000000_add_language_abridged.go
git commit -m "[Backend] Add language and abridged columns to files table"
```

---

### Task 3: ParsedMetadata, Sidecar, and Fingerprint

**Files:**
- Modify: `pkg/mediafile/mediafile.go:66-67` (after Codec field)
- Modify: `pkg/sidecar/types.go:33` (after CoverPage in FileSidecar)
- Modify: `pkg/sidecar/sidecar.go:194` (in FileSidecarFromModel)
- Modify: `pkg/downloadcache/fingerprint.go:41` (after PluginFingerprint in Fingerprint struct)
- Modify: `pkg/downloadcache/fingerprint.go:130` (in ComputeFingerprint, after Imprint)

- [ ] **Step 1: Add fields to ParsedMetadata**

In `pkg/mediafile/mediafile.go`, after line 66 (the `Codec` field), add:

```go
	// Language is a BCP 47 language tag (e.g., "en", "en-US", "zh-Hans")
	Language *string `json:"language,omitempty"`
	// Abridged indicates whether this is an abridged edition
	Abridged *bool `json:"abridged,omitempty"`
```

Also update the `FieldDataSources` comment (line 56) to add `"language"` and `"abridged"` to the list of valid keys.

- [ ] **Step 2: Add fields to FileSidecar**

In `pkg/sidecar/types.go`, after line 33 (CoverPage field), add:

```go
	Language *string `json:"language,omitempty"`
	Abridged *bool   `json:"abridged,omitempty"`
```

- [ ] **Step 3: Update FileSidecarFromModel**

In `pkg/sidecar/sidecar.go`, in the `FileSidecarFromModel` function, add to the struct literal (after `CoverPage: file.CoverPage,` on line 194):

```go
		Language:  file.Language,
		Abridged:  file.Abridged,
```

- [ ] **Step 4: Add fields to Fingerprint struct**

In `pkg/downloadcache/fingerprint.go`, after line 41 (`PluginFingerprint` field), add:

```go
	Language *string `json:"language,omitempty"`
	Abridged *bool   `json:"abridged,omitempty"`
```

- [ ] **Step 5: Add fields to ComputeFingerprint**

In `pkg/downloadcache/fingerprint.go`, in `ComputeFingerprint`, after the Imprint block (line 130), add:

```go
		fp.Language = file.Language
		fp.Abridged = file.Abridged
```

- [ ] **Step 6: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise test`
Expected: All existing tests pass.

- [ ] **Step 7: Commit**

```bash
git add pkg/mediafile/mediafile.go pkg/sidecar/types.go pkg/sidecar/sidecar.go pkg/downloadcache/fingerprint.go
git commit -m "[Backend] Add language and abridged to ParsedMetadata, sidecar, and fingerprint"
```

---

### Task 4: Parser Extraction — EPUB

**Files:**
- Modify: `pkg/epub/opf.go:20-38` (OPF struct — add Language field)
- Modify: `pkg/epub/opf.go:201-219` (ParsedMetadata construction — wire Language)

The EPUB parser already parses `<dc:language>` into `pkg.Metadata.Language` (line 79 of the Package struct). The OPF struct (lines 20-38) does NOT have a Language field — it needs one. Then wire it through from `ParseOPF` to the OPF struct, and from `Parse` to `ParsedMetadata`.

- [ ] **Step 1: Write failing test**

Write a test that parses an EPUB with `<dc:language>en-US</dc:language>` and asserts `metadata.Language` is `ptr("en-US")`. Also test that a 3-letter code like `eng` gets normalized to `en`.

Check existing test files in `pkg/epub/` for the test pattern and fixture approach used.

- [ ] **Step 2: Run test to verify it fails**

Expected: FAIL — OPF struct has no Language field, ParsedMetadata.Language not set.

- [ ] **Step 3: Add Language field to OPF struct and wire through**

In `pkg/epub/opf.go`, add to the `OPF` struct (after `Chapters` on line 37):

```go
	Language *string
```

In `ParseOPF`, after extracting other metadata, add language extraction using `NormalizeLanguage`:

```go
	// Extract language
	if pkg.Metadata.Language != "" {
		opf.Language = mediafile.NormalizeLanguage(pkg.Metadata.Language)
	}
```

In the `Parse` function (line 201), add to the `ParsedMetadata` struct literal:

```go
		Language:      opf.Language,
```

- [ ] **Step 4: Run test to verify it passes**

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/epub/opf.go pkg/epub/*_test.go
git commit -m "[Backend] Extract language from EPUB dc:language metadata"
```

---

### Task 5: Parser Extraction — CBZ

**Files:**
- Modify: `pkg/cbz/cbz.go:251-271` (ParsedMetadata construction — wire LanguageISO)

The CBZ parser already parses `LanguageISO` from ComicInfo.xml (line 53 of ComicInfo struct). It just needs to be wired through to ParsedMetadata.

- [ ] **Step 1: Write failing test**

Write a test that parses a CBZ with `<LanguageISO>en</LanguageISO>` in ComicInfo.xml and asserts `metadata.Language` is `ptr("en")`.

Check existing test files in `pkg/cbz/` for the test pattern.

- [ ] **Step 2: Run test to verify it fails**

Expected: FAIL — Language field not set in ParsedMetadata.

- [ ] **Step 3: Wire LanguageISO through to ParsedMetadata**

In `pkg/cbz/cbz.go`, before the ParsedMetadata construction (around line 251), add:

```go
	// Extract language (ComicInfo uses ISO 639-1, which is valid BCP 47)
	var language *string
	if comicInfo.LanguageISO != "" {
		language = mediafile.NormalizeLanguage(comicInfo.LanguageISO)
	}
```

Then add to the ParsedMetadata struct literal:

```go
		Language:      language,
```

- [ ] **Step 4: Run test to verify it passes**

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/cbz/cbz.go pkg/cbz/*_test.go
git commit -m "[Backend] Extract language from CBZ ComicInfo LanguageISO"
```

---

### Task 6: Parser Extraction — M4B

**Files:**
- Modify: `pkg/mp4/metadata.go:125-165` (freeform atom extraction — add language and abridged)

Extract language from freeform atoms `com.pilabor.tone:LANGUAGE` or `com.apple.iTunes:LANGUAGE`. Extract abridged from `com.pilabor.tone:ABRIDGED`.

- [ ] **Step 1: Write failing tests**

Write tests that parse M4B metadata with freeform atoms for language and abridged, asserting correct extraction. Check existing tests in `pkg/mp4/` for patterns.

- [ ] **Step 2: Run tests to verify they fail**

Expected: FAIL — Language and Abridged not extracted.

- [ ] **Step 3: Add Language and Abridged to Metadata struct**

In `pkg/mp4/metadata.go`, add to the `Metadata` struct (after `Identifiers` on line 42):

```go
	Language *string // from com.pilabor.tone:LANGUAGE or com.apple.iTunes:LANGUAGE freeform atom
	Abridged *bool   // from com.pilabor.tone:ABRIDGED freeform atom
```

- [ ] **Step 4: Extract from freeform atoms in convertRawMetadata**

In `pkg/mp4/metadata.go`, in the `convertRawMetadata` function, inside the `if len(raw.freeform) > 0` block (after the URL extraction around line 148), add:

```go
		// Extract language from freeform
		if lang, ok := raw.freeform["com.pilabor.tone:LANGUAGE"]; ok {
			meta.Language = mediafile.NormalizeLanguage(lang)
		} else if lang, ok := raw.freeform["com.apple.iTunes:LANGUAGE"]; ok {
			meta.Language = mediafile.NormalizeLanguage(lang)
		}
		// Extract abridged from freeform
		if abr, ok := raw.freeform["com.pilabor.tone:ABRIDGED"]; ok {
			switch strings.ToLower(strings.TrimSpace(abr)) {
			case "true":
				b := true
				meta.Abridged = &b
			case "false":
				b := false
				meta.Abridged = &b
			}
		}
```

- [ ] **Step 5: Wire through to ParsedMetadata in the Parse function**

Find where `mp4.Parse()` constructs the `mediafile.ParsedMetadata` return value (likely in `pkg/mp4/mp4.go`). Add:

```go
		Language: meta.Language,
		Abridged: meta.Abridged,
```

- [ ] **Step 6: Run tests to verify they pass**

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/mp4/metadata.go pkg/mp4/mp4.go pkg/mp4/*_test.go
git commit -m "[Backend] Extract language and abridged from M4B freeform atoms"
```

---

### Task 7: Parser Extraction — PDF

**Files:**
- Modify: `pkg/pdf/pdf.go:113-126` (ParsedMetadata construction)

PDF language extraction is best-effort — check if pdfcpu's `XRefTable` exposes a language field. If not available via pdfcpu API, skip PDF language extraction (it can still be set manually or via plugins).

- [ ] **Step 1: Investigate pdfcpu XRefTable for language**

Check if `xrt` (the pdfcpu XRefTable) has a `Language` or `Lang` field by reading the pdfcpu source or checking `xrt` fields. The PDF spec has a `Lang` entry in the catalog.

- [ ] **Step 2: If available, add language extraction**

If `xrt` has a language field, add before the ParsedMetadata construction (around line 112):

```go
	// Extract language from document catalog
	var language *string
	if xrt.Lang != "" {  // field name depends on pdfcpu API
		language = mediafile.NormalizeLanguage(xrt.Lang)
	}
```

And add to ParsedMetadata: `Language: language,`

If NOT available in pdfcpu, skip this and add a comment explaining why. PDF language can still be set manually.

- [ ] **Step 3: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && go test ./pkg/pdf/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/pdf/pdf.go
git commit -m "[Backend] Extract language from PDF catalog if available"
```

---

### Task 8: Scanner Integration

**Files:**
- Modify: `pkg/worker/scan_unified.go` (after URL/sidecar handling ~line 1281)

Follow the exact pattern used for URL (lines 1247-1281): metadata source handling then sidecar source handling.

- [ ] **Step 1: Add language handling from metadata**

After the ReleaseDate sidecar block (around line 1324), add:

```go
	// Language (from metadata)
	if metadata.Language != nil && *metadata.Language != "" {
		existingLanguage := ""
		existingLanguageSource := ""
		if file.Language != nil {
			existingLanguage = *file.Language
		}
		if file.LanguageSource != nil {
			existingLanguageSource = *file.LanguageSource
		}
		langSource := metadata.SourceForField("language")
		if shouldUpdateScalar(*metadata.Language, existingLanguage, langSource, existingLanguageSource, forceRefresh) {
			logInfo("updating file language", logger.Data{"from": existingLanguage, "to": *metadata.Language})
			file.Language = metadata.Language
			file.LanguageSource = &langSource
			fileUpdateOpts.Columns = append(fileUpdateOpts.Columns, "language", "language_source")
		}
	}
	// Language (from sidecar)
	if fileSidecarData != nil && fileSidecarData.Language != nil && *fileSidecarData.Language != "" {
		existingLanguage := ""
		existingLanguageSource := ""
		if file.Language != nil {
			existingLanguage = *file.Language
		}
		if file.LanguageSource != nil {
			existingLanguageSource = *file.LanguageSource
		}
		if shouldApplySidecarScalar(*fileSidecarData.Language, existingLanguage, existingLanguageSource, forceRefresh) {
			logInfo("updating file language from sidecar", logger.Data{"from": existingLanguage, "to": *fileSidecarData.Language})
			file.Language = fileSidecarData.Language
			file.LanguageSource = &sidecarSource
			fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "language", "language_source")
		}
	}

	// Abridged (from metadata)
	if metadata.Abridged != nil {
		existingAbridgedSource := ""
		if file.AbridgedSource != nil {
			existingAbridgedSource = *file.AbridgedSource
		}
		newAbridgedStr := "false"
		if *metadata.Abridged {
			newAbridgedStr = "true"
		}
		existingAbridgedStr := ""
		if file.Abridged != nil {
			if *file.Abridged {
				existingAbridgedStr = "true"
			} else {
				existingAbridgedStr = "false"
			}
		}
		abridgedSource := metadata.SourceForField("abridged")
		if shouldUpdateScalar(newAbridgedStr, existingAbridgedStr, abridgedSource, existingAbridgedSource, forceRefresh) {
			logInfo("updating file abridged", logger.Data{"from": existingAbridgedStr, "to": newAbridgedStr})
			file.Abridged = metadata.Abridged
			file.AbridgedSource = &abridgedSource
			fileUpdateOpts.Columns = append(fileUpdateOpts.Columns, "abridged", "abridged_source")
		}
	}
	// Abridged (from sidecar)
	if fileSidecarData != nil && fileSidecarData.Abridged != nil {
		existingAbridgedSource := ""
		if file.AbridgedSource != nil {
			existingAbridgedSource = *file.AbridgedSource
		}
		newAbridgedStr := "false"
		if *fileSidecarData.Abridged {
			newAbridgedStr = "true"
		}
		existingAbridgedStr := ""
		if file.Abridged != nil {
			if *file.Abridged {
				existingAbridgedStr = "true"
			} else {
				existingAbridgedStr = "false"
			}
		}
		if shouldApplySidecarScalar(newAbridgedStr, existingAbridgedStr, existingAbridgedSource, forceRefresh) {
			logInfo("updating file abridged from sidecar", logger.Data{"from": existingAbridgedStr, "to": newAbridgedStr})
			file.Abridged = fileSidecarData.Abridged
			file.AbridgedSource = &sidecarSource
			fileUpdateOpts.Columns = appendIfMissing(fileUpdateOpts.Columns, "abridged", "abridged_source")
		}
	}
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise test`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/worker/scan_unified.go
git commit -m "[Backend] Handle language and abridged in scanner with data source priority"
```

---

### Task 9: Edit API

**Files:**
- Modify: `pkg/books/validators.go:55` (UpdateFilePayload — add Language and Abridged)
- Modify: `pkg/books/handlers.go:770` (supplement downgrade — clear abridged)
- Modify: `pkg/books/handlers.go:1067` (after ReleaseDate update — add language and abridged handling)

- [ ] **Step 1: Add fields to UpdateFilePayload**

In `pkg/books/validators.go`, after `ReleaseDate` (line 54), add:

```go
	Language *string `json:"language,omitempty" validate:"omitempty,max=35"`
	Abridged *string `json:"abridged,omitempty" validate:"omitempty,oneof=true false"` // "true", "false", or "" to clear
```

Note: Abridged is `*string` in the payload. Absent = don't change. Empty string = clear. "true"/"false" = set value.

- [ ] **Step 2: Add abridged to supplement downgrade clearing**

In `pkg/books/handlers.go`, in the supplement downgrade block, after clearing URL (line 770), add:

```go
			// Clear abridged (language is preserved — it's intrinsic to file content)
			file.Abridged = nil
			file.AbridgedSource = nil
			opts.Columns = append(opts.Columns, "abridged", "abridged_source")
```

- [ ] **Step 3: Add language and abridged update handling**

In `pkg/books/handlers.go`, after the ReleaseDate update block (line 1067), add:

```go
	// Update language
	if params.Language != nil {
		currentLanguage := ""
		if file.Language != nil {
			currentLanguage = *file.Language
		}
		if *params.Language != currentLanguage {
			if *params.Language == "" {
				file.Language = nil
				file.LanguageSource = nil
			} else {
				normalized := mediafile.NormalizeLanguage(*params.Language)
				if normalized == nil {
					return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid language tag: %s", *params.Language))
				}
				file.Language = normalized
				file.LanguageSource = strPtr(models.DataSourceManual)
			}
			opts.Columns = append(opts.Columns, "language", "language_source")
		}
	}

	// Update abridged
	if params.Abridged != nil {
		switch *params.Abridged {
		case "":
			// Clear
			if file.Abridged != nil {
				file.Abridged = nil
				file.AbridgedSource = nil
				opts.Columns = append(opts.Columns, "abridged", "abridged_source")
			}
		case "true":
			if file.Abridged == nil || !*file.Abridged {
				b := true
				file.Abridged = &b
				file.AbridgedSource = strPtr(models.DataSourceManual)
				opts.Columns = append(opts.Columns, "abridged", "abridged_source")
			}
		case "false":
			if file.Abridged == nil || *file.Abridged {
				b := false
				file.Abridged = &b
				file.AbridgedSource = strPtr(models.DataSourceManual)
				opts.Columns = append(opts.Columns, "abridged", "abridged_source")
			}
		}
	}
```

- [ ] **Step 4: Generate TypeScript types**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise tygo`

- [ ] **Step 5: Run tests and lint**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise check:quiet`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/books/validators.go pkg/books/handlers.go
git commit -m "[Backend] Add language and abridged to file edit API"
```

---

### Task 10: File Generation — EPUB, CBZ, M4B, PDF

**Files:**
- Modify: `pkg/filegen/epub.go:283` (after publisher write in modifyOPF)
- Modify: `pkg/filegen/cbz.go:447` (after release date in modifyCBZComicInfo)
- Modify: `pkg/filegen/m4b.go:118` (after URL in buildMetadata)
- Modify: `pkg/filegen/pdf.go:108` (after CreationDate in buildProperties)

- [ ] **Step 1: EPUB — write language to dc:language**

In `pkg/filegen/epub.go`, in `modifyOPF`, after the publisher update (line 279), add:

```go
	// Update language from file if available
	if file != nil && file.Language != nil && *file.Language != "" {
		pkg.Metadata.Language = *file.Language
	}
```

The `opfMetadata` struct already has `Language string` at line 570. This will write `<dc:language>`.

- [ ] **Step 2: CBZ — write language to LanguageISO**

In `pkg/filegen/cbz.go`, in `modifyCBZComicInfo`, after the release date block (line 447), add:

```go
	// Update language (LanguageISO in ComicInfo.xml)
	if file.Language != nil && *file.Language != "" {
		comicInfo.LanguageISO = *file.Language
	}
```

The `cbzComicInfo` struct already has `LanguageISO string` at line 280.

- [ ] **Step 3: M4B — write language and abridged to freeform atoms**

In `pkg/filegen/m4b.go`, in `buildMetadata`, after the URL block (line 118), add:

```go
	// Set language from file if available
	if file.Language != nil && *file.Language != "" {
		if meta.Freeform == nil {
			meta.Freeform = make(map[string]string)
		}
		meta.Freeform["com.pilabor.tone:LANGUAGE"] = *file.Language
	}

	// Set abridged from file if available
	if file.Abridged != nil {
		if meta.Freeform == nil {
			meta.Freeform = make(map[string]string)
		}
		if *file.Abridged {
			meta.Freeform["com.pilabor.tone:ABRIDGED"] = "true"
		} else {
			meta.Freeform["com.pilabor.tone:ABRIDGED"] = "false"
		}
	}
```

- [ ] **Step 4: PDF — write language if pdfcpu supports it**

In `pkg/filegen/pdf.go`, in `buildProperties`, after the CreationDate block (line 108), add:

```go
	// Language — set in info dict if available.
	// Note: pdfcpu's AddPropertiesFile only writes info dict fields, not the
	// catalog Lang entry. Language is included for completeness.
	if file != nil && file.Language != nil && *file.Language != "" {
		props["Language"] = *file.Language
	}
```

- [ ] **Step 5: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise test`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/filegen/epub.go pkg/filegen/cbz.go pkg/filegen/m4b.go pkg/filegen/pdf.go
git commit -m "[Backend] Write language and abridged back to files on generation"
```

---

### Task 11: Plugin SDK

**Files:**
- Modify: `packages/plugin-sdk/metadata.d.ts:43` (after url field)

- [ ] **Step 1: Add fields to ParsedMetadata TypeScript interface**

In `packages/plugin-sdk/metadata.d.ts`, after `url?: string;` (line 43), add:

```typescript
  /** BCP 47 language tag (e.g., "en", "en-US", "zh-Hans"). */
  language?: string;
  /** Whether this is an abridged edition. true=abridged, false=unabridged, undefined=unknown. */
  abridged?: boolean;
```

- [ ] **Step 2: Commit**

```bash
git add packages/plugin-sdk/metadata.d.ts
git commit -m "[Backend] Add language and abridged to plugin SDK ParsedMetadata"
```

---

### Task 12: Curated Language List (Frontend)

**Files:**
- Create: `app/constants/languages.ts`

- [ ] **Step 1: Create the curated BCP 47 language list**

Create `app/constants/languages.ts` with a curated list of common BCP 47 tags. Include all ISO 639-1 languages plus common script/regional variants. Format: `{ tag: string, name: string }[]`.

This is a large static list (~200 entries). Include all common languages (English, French, German, Spanish, Chinese, Japanese, Korean, etc.) with regional variants where meaningful (en-US, en-GB, pt-BR, pt-PT, zh-Hans, zh-Hant, es-419, fr-CA, etc.).

Export as `export const LANGUAGES: { tag: string; name: string }[]` and a helper `export function getLanguageName(tag: string): string | undefined` that looks up a tag in the list.

- [ ] **Step 2: Commit**

```bash
git add app/constants/languages.ts
git commit -m "[Frontend] Add curated BCP 47 language list for combobox"
```

---

### Task 13: Library Languages Endpoint

**Files:**
- Modify: `pkg/books/service.go` (add DistinctFileLanguages method)
- Modify: `pkg/books/handlers.go` (add handler)
- Modify: `pkg/books/routes.go` (add route)

This endpoint returns distinct language values for files in a library, used by both the edit combobox and the gallery filter.

- [ ] **Step 1: Add service method**

In `pkg/books/service.go`, add:

```go
// DistinctFileLanguages returns distinct non-null language values for files in a library.
func (svc *Service) DistinctFileLanguages(ctx context.Context, libraryID int) ([]string, error) {
	var languages []string
	err := svc.db.NewSelect().
		TableExpr("files AS f").
		ColumnExpr("DISTINCT f.language").
		Where("f.library_id = ?", libraryID).
		Where("f.language IS NOT NULL").
		Where("f.language != ''").
		OrderExpr("f.language ASC").
		Scan(ctx, &languages)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return languages, nil
}
```

- [ ] **Step 2: Add handler**

In `pkg/books/handlers.go`, add a handler:

```go
func (h *Handler) listLibraryLanguages(c echo.Context) error {
	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}

	languages, err := h.bookService.DistinctFileLanguages(c.Request().Context(), libraryID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, languages)
}
```

- [ ] **Step 3: Add route**

In `pkg/books/routes.go`, under the libraries group (or a suitable group that has library access middleware), add:

```go
g.GET("/libraries/:id/languages", h.listLibraryLanguages, authMiddleware.RequireLibraryAccess("id"))
```

Find the appropriate route group by checking existing library routes in the file.

- [ ] **Step 4: Add query hook on frontend**

Create or modify the appropriate query hook file in `app/hooks/queries/` to add a `useLibraryLanguages(libraryId)` hook.

- [ ] **Step 5: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise check:quiet`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/books/service.go pkg/books/handlers.go pkg/books/routes.go app/hooks/queries/
git commit -m "[Backend] Add endpoint for distinct file languages per library"
```

---

### Task 14: Frontend — FileEditDialog (Language and Abridged Fields)

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx`

Add language combobox (with curated list + library languages + free-text fallback) and abridged dropdown ("Unknown" / "Unabridged" / "Abridged").

- [ ] **Step 1: Add state and initial values for language and abridged**

In the component state section (around line 130), add:

```tsx
const [language, setLanguage] = useState(file.language || "");
const [languageOpen, setLanguageOpen] = useState(false);
const [languageSearch, setLanguageSearch] = useState("");
const [abridged, setAbridged] = useState<string>(
  file.abridged === true ? "true" : file.abridged === false ? "false" : "",
);
```

In the `initialValues` state type and initialization (around line 200), add `language` and `abridged` fields.

In the `useEffect` that initializes form on dialog open, add:

```tsx
const initialLanguage = file.language || "";
const initialAbridged = file.abridged === true ? "true" : file.abridged === false ? "false" : "";
setLanguage(initialLanguage);
setLanguageSearch("");
setAbridged(initialAbridged);
```

And store in `initialValues`: `language: initialLanguage, abridged: initialAbridged,`

- [ ] **Step 2: Update hasChanges computation**

Add `language !== initialValues.language` and `abridged !== initialValues.abridged` to the `hasChanges` memo.

- [ ] **Step 3: Update save mutation payload**

In the save handler, add language and abridged to the mutation payload:

```tsx
...(language !== initialValues?.language && { language }),
...(abridged !== initialValues?.abridged && { abridged }),
```

- [ ] **Step 4: Add language combobox to the form**

Use the `useLibraryLanguages` hook to fetch library languages. Merge with the curated list from `app/constants/languages.ts`. Build the combobox following the publisher combobox pattern (lines 931-1018).

Place the language field after the URL field (around line 929). Show for all file types (not inside the M4B-only conditional).

The combobox should:
- Filter the merged list as user types
- Show "Use custom tag: {search}" as a fallback option when search doesn't match
- Display as "English (en)" format for curated items, just the tag for custom ones
- Be clearable (a clear button or empty selection sets language to "")

- [ ] **Step 5: Add abridged dropdown to the form**

Place after the language field. Use a `Select` component with three options:

```tsx
<div className="space-y-2">
  <Label>Abridged</Label>
  <Select onValueChange={setAbridged} value={abridged}>
    <SelectTrigger>
      <SelectValue placeholder="Unknown" />
    </SelectTrigger>
    <SelectContent>
      <SelectItem value="">Unknown</SelectItem>
      <SelectItem value="false">Unabridged</SelectItem>
      <SelectItem value="true">Abridged</SelectItem>
    </SelectContent>
  </Select>
</div>
```

Note: shadcn Select may not support empty string as a value. If not, use a sentinel like `"unknown"` and convert in the save handler. Check the Select component behavior first.

- [ ] **Step 6: Run frontend lint and type check**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise lint:js`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add app/components/library/FileEditDialog.tsx
git commit -m "[Frontend] Add language combobox and abridged dropdown to file edit dialog"
```

---

### Task 15: Frontend — BookDetail Display

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

Show language (as human-readable name from the curated list, or raw tag if custom) and abridged status in the file metadata section.

- [ ] **Step 1: Add language and abridged display**

Find where file metadata (URL, publisher, imprint, release date) is displayed in the file section. Add:

- Language: Show as "English (en)" using `getLanguageName()` from the curated list, falling back to the raw tag. Only show if non-null.
- Abridged: Show as "Abridged" or "Unabridged" badge. Only show if non-null.

- [ ] **Step 2: Run frontend lint**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise lint:js`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "[Frontend] Display language and abridged status on book detail page"
```

---

### Task 16: Frontend — Gallery Language Filter

**Files:**
- Modify: `app/hooks/queries/books.ts` or `app/hooks/queries/libraries.ts` (ListBooksQuery)
- Modify: `pkg/books/validators.go` (ListBooksQuery — add Language filter)
- Modify: `pkg/books/service.go` (listBooksWithTotal — add language WHERE clause)
- Modify: Gallery page component (add filter dropdown)

- [ ] **Step 1: Add language filter to backend ListBooksQuery**

In `pkg/books/validators.go`, add to `ListBooksQuery`:

```go
	Language *string `query:"language" json:"language,omitempty" validate:"omitempty,max=35" tstype:"string"`
```

- [ ] **Step 2: Add language filter to listBooksWithTotal**

In `pkg/books/service.go`, in `listBooksWithTotal`, after the tag IDs filter (around line 345), add:

```go
	// Filter by language
	if opts.Language != nil && *opts.Language != "" {
		q = q.Where("b.id IN (SELECT DISTINCT book_id FROM files WHERE language = ?)", *opts.Language)
	}
```

Wire the `Language` field through from `ListBooksQuery` to `ListBooksOptions`.

- [ ] **Step 3: Add language filter dropdown to gallery page**

In the gallery/library page component:
- Query `useLibraryLanguages(libraryId)` to get distinct languages
- If 2+ languages returned, show a filter dropdown
- Display language names using the curated list, grouped by base subtag per the spec
- When a language is selected, pass it as a query param to the books list query
- Include an "All Languages" option to clear the filter

- [ ] **Step 4: Generate TypeScript types**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise tygo`

- [ ] **Step 5: Run all checks**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise check:quiet`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/books/validators.go pkg/books/service.go app/
git commit -m "[Feature] Add conditional language filter to gallery page"
```

---

### Task 17: Documentation Updates

**Files:**
- Modify: `website/docs/metadata.md`
- Modify: `website/docs/sidecar-files.md`
- Modify: `website/docs/supported-formats.md`
- Modify: `website/docs/plugins/` (plugin SDK docs)

- [ ] **Step 1: Update metadata docs**

In `website/docs/metadata.md`, add language and abridged to the file-level metadata fields section. Document:
- Language: BCP 47 tag, editable, extracted from EPUB/CBZ/M4B/PDF
- Abridged: Boolean (true/false/unknown), editable, extracted from M4B

- [ ] **Step 2: Update sidecar docs**

In `website/docs/sidecar-files.md`, add `language` and `abridged` to the file sidecar format documentation.

- [ ] **Step 3: Update supported formats docs**

In `website/docs/supported-formats.md`, note which formats support language and abridged extraction.

- [ ] **Step 4: Update plugin SDK docs**

In the plugin documentation, add `language` and `abridged` to the ParsedMetadata fields list.

- [ ] **Step 5: Commit**

```bash
git add website/docs/
git commit -m "[Docs] Document language and abridged file metadata fields"
```

---

### Task 18: Final Validation

- [ ] **Step 1: Run full check suite**

Run: `cd /Users/robinjoseph/.worktrees/shisho/audiobook-fields && mise check:quiet`
Expected: All checks pass.

- [ ] **Step 2: Manual smoke test**

Start the dev server with `mise start` and verify:
1. Upload/scan an EPUB with `<dc:language>en</dc:language>` — language appears on file
2. Edit a file to set language via combobox — saves and displays correctly
3. Edit a file to set abridged status — saves and displays correctly
4. Gallery language filter appears when 2+ languages exist in a library
5. Clear language and abridged — values reset to nil

- [ ] **Step 3: Commit any remaining fixes**
