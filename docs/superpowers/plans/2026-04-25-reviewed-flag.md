# Reviewed Flag — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a per-file `Reviewed` state with admin-configurable required fields, sticky manual overrides, automatic recompute on metadata changes, a library "needs review" filter, and corresponding badges/UI in the book and file detail views.

**Architecture:** Three new columns on `files` (`review_override`, `review_overridden_at`, `reviewed`). A new `app_settings` key/value table holds the runtime-mutable review criteria (universal + audio-only). A `pkg/books/review` package centralizes the completeness rule and recompute helpers. A new `recompute_review` background job populates `files.reviewed` after migrations and on settings changes. PATCH endpoints (file/book/bulk) set overrides and trigger recompute. Frontend adds a filter sheet entry, a card badge, review panels in `BookDetail` / `BookEditDialog` / `FileEditDialog`, an overflow popover in the bulk selection toolbar, and a Review Criteria section in `AdminSettings`.

**Tech Stack:** Go + Echo + Bun + SQLite (backend), React 19 + TanStack Query + Tailwind/shadcn (frontend), Vitest + Playwright (tests).

**Reference spec:** `docs/superpowers/specs/2026-04-25-reviewed-flag-design.md`

---

## Before starting

Establish a green baseline before touching code:

```bash
mise check:quiet
```

Expected: all checks pass.

Generate types after any Go struct changes:

```bash
mise tygo
```

If it prints "skipping, outputs are up-to-date", that's normal — see CLAUDE.md.

---

## Phase 1 — Foundation: app_settings table

The spec requires runtime-mutable server-level settings (admin can change without restart). The codebase doesn't yet have a generic mechanism for this — we introduce one with `app_settings`, a JSON key/value store.

### Task 1: Migration — `app_settings` table

**Files:**
- Create: `pkg/migrations/20260425100000_add_app_settings.go`

- [ ] **Step 1.1: Write the migration**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `
			CREATE TABLE app_settings (
				key TEXT PRIMARY KEY NOT NULL,
				value TEXT NOT NULL,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			)
		`)
		return errors.WithStack(err)
	}

	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `DROP TABLE app_settings`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 1.2: Run migrations to verify**

```bash
mise db:migrate
mise db:rollback
mise db:migrate
```

Expected: no errors.

- [ ] **Step 1.3: Commit**

```bash
git add pkg/migrations/20260425100000_add_app_settings.go
git commit -m "[Backend] Add app_settings runtime config table"
```

---

### Task 2: AppSetting model and Service

**Files:**
- Create: `pkg/models/app-setting.go`
- Create: `pkg/appsettings/service.go`
- Create: `pkg/appsettings/service_test.go`

- [ ] **Step 2.1: Write the model**

`pkg/models/app-setting.go`:

```go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

type AppSetting struct {
	bun.BaseModel `bun:"table:app_settings,alias:as" tstype:"-"`

	Key       string    `bun:",pk,nullzero" json:"key"`
	Value     string    `bun:",notnull" json:"value"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}
```

- [ ] **Step 2.2: Write the service test (failing)**

`pkg/appsettings/service_test.go`:

```go
package appsettings

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/database"
	"github.com/stretchr/testify/require"
)

type sampleConfig struct {
	BookFields  []string `json:"book_fields"`
	AudioFields []string `json:"audio_fields"`
}

func TestGetSetJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := database.NewTestDB(t)
	svc := NewService(db)

	want := sampleConfig{
		BookFields:  []string{"authors", "description"},
		AudioFields: []string{"narrators"},
	}
	require.NoError(t, svc.SetJSON(ctx, "review_criteria", want))

	var got sampleConfig
	ok, err := svc.GetJSON(ctx, "review_criteria", &got)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestGetJSON_Missing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := database.NewTestDB(t)
	svc := NewService(db)

	var got sampleConfig
	ok, err := svc.GetJSON(ctx, "missing_key", &got)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestSetJSON_Overwrite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := database.NewTestDB(t)
	svc := NewService(db)

	require.NoError(t, svc.SetJSON(ctx, "k", sampleConfig{BookFields: []string{"a"}}))
	require.NoError(t, svc.SetJSON(ctx, "k", sampleConfig{BookFields: []string{"b"}}))

	var got sampleConfig
	_, err := svc.GetJSON(ctx, "k", &got)
	require.NoError(t, err)
	require.Equal(t, []string{"b"}, got.BookFields)
}
```

- [ ] **Step 2.3: Run tests to verify failure**

```bash
go test ./pkg/appsettings/...
```

Expected: FAIL — package missing.

- [ ] **Step 2.4: Implement the service**

`pkg/appsettings/service.go`:

```go
package appsettings

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// GetJSON loads the JSON-encoded value for key into out. Returns (false, nil) if no row exists.
func (svc *Service) GetJSON(ctx context.Context, key string, out interface{}) (bool, error) {
	row := &models.AppSetting{}
	err := svc.db.NewSelect().Model(row).Where("key = ?", key).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	if err := json.Unmarshal([]byte(row.Value), out); err != nil {
		return false, errors.WithStack(err)
	}
	return true, nil
}

// SetJSON stores the JSON-encoded value at key. Upserts on conflict.
func (svc *Service) SetJSON(ctx context.Context, key string, value interface{}) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return errors.WithStack(err)
	}
	row := &models.AppSetting{Key: key, Value: string(encoded)}
	_, err = svc.db.NewInsert().
		Model(row).
		On("CONFLICT (key) DO UPDATE").
		Set("value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP").
		Exec(ctx)
	return errors.WithStack(err)
}
```

- [ ] **Step 2.5: Run tests to verify pass**

```bash
go test ./pkg/appsettings/... -race
```

Expected: PASS.

- [ ] **Step 2.6: Commit**

```bash
git add pkg/models/app-setting.go pkg/appsettings/
git commit -m "[Backend] Add AppSetting model and JSON-keyed service"
```

---

## Phase 2 — Schema: file review columns

### Task 3: Migration — files review columns

**Files:**
- Create: `pkg/migrations/20260425100100_add_file_review_fields.go`

- [ ] **Step 3.1: Write the migration**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		stmts := []string{
			`ALTER TABLE files ADD COLUMN review_override TEXT
				CHECK (review_override IS NULL OR review_override IN ('reviewed','unreviewed'))`,
			`ALTER TABLE files ADD COLUMN review_overridden_at TIMESTAMP`,
			`ALTER TABLE files ADD COLUMN reviewed BOOLEAN`,
			`CREATE INDEX idx_files_book_reviewed
				ON files(book_id, reviewed)
				WHERE file_role = 'main'`,
		}
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		stmts := []string{
			`DROP INDEX IF EXISTS idx_files_book_reviewed`,
			`ALTER TABLE files DROP COLUMN reviewed`,
			`ALTER TABLE files DROP COLUMN review_overridden_at`,
			`ALTER TABLE files DROP COLUMN review_override`,
		}
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 3.2: Verify roundtrip**

```bash
mise db:migrate && mise db:rollback && mise db:migrate
```

Expected: no errors.

- [ ] **Step 3.3: Commit**

```bash
git add pkg/migrations/20260425100100_add_file_review_fields.go
git commit -m "[Backend] Add review fields and index to files table"
```

---

### Task 4: Update File model

**Files:**
- Modify: `pkg/models/file.go`

- [ ] **Step 4.1: Add the new fields**

In `pkg/models/file.go`, add the constants near the top (after `FileRoleSupplement`):

```go
const (
	//tygo:emit export type ReviewOverride = typeof ReviewOverrideReviewed | typeof ReviewOverrideUnreviewed;
	ReviewOverrideReviewed   = "reviewed"
	ReviewOverrideUnreviewed = "unreviewed"
)
```

In the `File` struct, add (alphabetically in the existing block, or grouped at the end of the struct):

```go
ReviewOverride     *string    `json:"review_override" tstype:"ReviewOverride"`
ReviewOverriddenAt *time.Time `json:"review_overridden_at"`
Reviewed           *bool      `json:"reviewed"`
```

- [ ] **Step 4.2: Regenerate types**

```bash
mise tygo
```

Expected: `app/types/generated/` is updated to include the new fields.

- [ ] **Step 4.3: Verify compile**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4.4: Commit**

```bash
git add pkg/models/file.go app/types/generated/
git commit -m "[Backend] Add review fields to File model"
```

---

## Phase 3 — Completeness logic

### Task 5: `pkg/books/review` package — completeness rule

**Files:**
- Create: `pkg/books/review/criteria.go`
- Create: `pkg/books/review/criteria_test.go`
- Create: `pkg/books/review/completeness.go`
- Create: `pkg/books/review/completeness_test.go`

- [ ] **Step 5.1: Write criteria definition**

`pkg/books/review/criteria.go`:

```go
package review

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/appsettings"
)

const SettingsKey = "review_criteria"

// Field constants — every name must be in either UniversalCandidates or AudioCandidates.
const (
	FieldAuthors     = "authors"
	FieldDescription = "description"
	FieldCover       = "cover"
	FieldGenres      = "genres"
	FieldTags        = "tags"
	FieldSeries      = "series"
	FieldSubtitle    = "subtitle"
	FieldPublisher   = "publisher"
	FieldImprint     = "imprint"
	FieldIdentifiers = "identifiers"
	FieldReleaseDate = "release_date"
	FieldLanguage    = "language"
	FieldURL         = "url"
	FieldNarrators   = "narrators"
	FieldChapters    = "chapters"
	FieldAbridged    = "abridged"
)

// UniversalCandidates is the set of fields that can be required for all books.
var UniversalCandidates = []string{
	FieldAuthors, FieldDescription, FieldCover, FieldGenres, FieldTags,
	FieldSeries, FieldSubtitle, FieldPublisher, FieldImprint, FieldIdentifiers,
	FieldReleaseDate, FieldLanguage, FieldURL,
}

// AudioCandidates is the set of fields that can be required only when the book has at least one audio file.
var AudioCandidates = []string{FieldNarrators, FieldChapters, FieldAbridged}

type Criteria struct {
	BookFields  []string `json:"book_fields"`
	AudioFields []string `json:"audio_fields"`
}

// Default returns the seeded review criteria.
func Default() Criteria {
	return Criteria{
		BookFields:  []string{FieldAuthors, FieldDescription, FieldCover, FieldGenres},
		AudioFields: []string{FieldNarrators},
	}
}

// Load reads the criteria from app settings, falling back to Default if unset.
func Load(ctx context.Context, settings *appsettings.Service) (Criteria, error) {
	var c Criteria
	ok, err := settings.GetJSON(ctx, SettingsKey, &c)
	if err != nil {
		return Criteria{}, errors.WithStack(err)
	}
	if !ok {
		return Default(), nil
	}
	// Defensive: if either slice is nil from older serializations, use defaults for that side.
	if c.BookFields == nil {
		c.BookFields = Default().BookFields
	}
	if c.AudioFields == nil {
		c.AudioFields = Default().AudioFields
	}
	return c, nil
}

// Save persists the criteria.
func Save(ctx context.Context, settings *appsettings.Service, c Criteria) error {
	return errors.WithStack(settings.SetJSON(ctx, SettingsKey, c))
}

// Validate returns an error if any field name in the criteria isn't a known candidate.
func Validate(c Criteria) error {
	if err := validateAgainst(c.BookFields, UniversalCandidates); err != nil {
		return errors.Wrap(err, "book_fields")
	}
	if err := validateAgainst(c.AudioFields, AudioCandidates); err != nil {
		return errors.Wrap(err, "audio_fields")
	}
	return nil
}

func validateAgainst(fields []string, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = struct{}{}
	}
	for _, f := range fields {
		if _, ok := allowedSet[f]; !ok {
			return errors.Errorf("unknown field %q", f)
		}
	}
	return nil
}
```

- [ ] **Step 5.2: Write criteria tests**

`pkg/books/review/criteria_test.go`:

```go
package review

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/database"
	"github.com/stretchr/testify/require"
)

func TestDefault_HasExpectedFields(t *testing.T) {
	t.Parallel()
	d := Default()
	require.ElementsMatch(t, []string{"authors", "description", "cover", "genres"}, d.BookFields)
	require.ElementsMatch(t, []string{"narrators"}, d.AudioFields)
}

func TestLoad_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := database.NewTestDB(t)
	svc := appsettings.NewService(db)

	c, err := Load(ctx, svc)
	require.NoError(t, err)
	require.Equal(t, Default(), c)
}

func TestLoad_ReadsSavedValue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := database.NewTestDB(t)
	svc := appsettings.NewService(db)

	want := Criteria{BookFields: []string{"authors"}, AudioFields: []string{}}
	require.NoError(t, Save(ctx, svc, want))

	got, err := Load(ctx, svc)
	require.NoError(t, err)
	require.Equal(t, want.BookFields, got.BookFields)
	require.Equal(t, want.AudioFields, got.AudioFields)
}

func TestValidate_RejectsUnknownField(t *testing.T) {
	t.Parallel()
	err := Validate(Criteria{BookFields: []string{"made_up_field"}})
	require.Error(t, err)
}

func TestValidate_RejectsAudioFieldInUniversal(t *testing.T) {
	t.Parallel()
	err := Validate(Criteria{BookFields: []string{"narrators"}})
	require.Error(t, err)
}

func TestValidate_AcceptsKnown(t *testing.T) {
	t.Parallel()
	err := Validate(Default())
	require.NoError(t, err)
}
```

- [ ] **Step 5.3: Run criteria tests**

```bash
go test ./pkg/books/review/... -race
```

Expected: PASS.

- [ ] **Step 5.4: Write completeness function**

`pkg/books/review/completeness.go`:

```go
package review

import (
	"github.com/shishobooks/shisho/pkg/models"
)

// MissingFields returns the list of required-field names that are not satisfied
// for a single main file. The book and file must have all relations loaded.
//
// For supplements, returns nil.
//
// The criteria's BookFields apply to every file; AudioFields apply when the
// containing book has at least one m4b file.
func MissingFields(book *models.Book, file *models.File, criteria Criteria) []string {
	if file.FileRole == models.FileRoleSupplement {
		return nil
	}
	missing := make([]string, 0)
	for _, f := range criteria.BookFields {
		if !isPresent(book, file, f) {
			missing = append(missing, f)
		}
	}
	if file.FileType == models.FileTypeM4B {
		for _, f := range criteria.AudioFields {
			if !isPresent(book, file, f) {
				missing = append(missing, f)
			}
		}
	}
	return missing
}

// IsComplete returns true iff MissingFields returns an empty slice for a main file.
func IsComplete(book *models.Book, file *models.File, criteria Criteria) bool {
	return len(MissingFields(book, file, criteria)) == 0
}

func isPresent(book *models.Book, file *models.File, field string) bool {
	switch field {
	case FieldAuthors:
		return len(book.Authors) > 0
	case FieldDescription:
		return book.Description != nil && *book.Description != ""
	case FieldGenres:
		return len(book.BookGenres) > 0
	case FieldTags:
		return len(book.BookTags) > 0
	case FieldSeries:
		return len(book.BookSeries) > 0
	case FieldSubtitle:
		return book.Subtitle != nil && *book.Subtitle != ""
	case FieldCover:
		return file.CoverImageFilename != nil && *file.CoverImageFilename != ""
	case FieldPublisher:
		return file.PublisherID != nil
	case FieldImprint:
		return file.ImprintID != nil
	case FieldIdentifiers:
		return len(file.Identifiers) > 0
	case FieldReleaseDate:
		return file.ReleaseDate != nil
	case FieldLanguage:
		return file.Language != nil && *file.Language != ""
	case FieldURL:
		return file.URL != nil && *file.URL != ""
	case FieldNarrators:
		return len(file.Narrators) > 0
	case FieldChapters:
		return len(file.Chapters) > 0
	case FieldAbridged:
		return file.Abridged != nil
	}
	return false
}
```

- [ ] **Step 5.5: Write completeness tests**

`pkg/books/review/completeness_test.go`:

```go
package review

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

func ptrStr(s string) *string { return &s }
func ptrInt(i int) *int       { return &i }
func ptrBool(b bool) *bool    { return &b }
func ptrTime(t time.Time) *time.Time { return &t }

func TestMissingFields_Supplement_NoCheck(t *testing.T) {
	t.Parallel()
	book := &models.Book{}
	file := &models.File{FileRole: models.FileRoleSupplement}
	require.Nil(t, MissingFields(book, file, Default()))
}

func TestMissingFields_EpubMissingAll_Default(t *testing.T) {
	t.Parallel()
	book := &models.Book{}
	file := &models.File{FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain}
	missing := MissingFields(book, file, Default())
	require.ElementsMatch(t, []string{"authors", "description", "cover", "genres"}, missing)
}

func TestMissingFields_EpubComplete(t *testing.T) {
	t.Parallel()
	book := &models.Book{
		Authors:     []*models.Author{{}},
		BookGenres:  []*models.BookGenre{{}},
		Description: ptrStr("desc"),
	}
	file := &models.File{
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		CoverImageFilename: ptrStr("cover.jpg"),
	}
	require.Empty(t, MissingFields(book, file, Default()))
}

func TestMissingFields_M4BNeedsNarrators(t *testing.T) {
	t.Parallel()
	book := &models.Book{
		Authors:     []*models.Author{{}},
		BookGenres:  []*models.BookGenre{{}},
		Description: ptrStr("desc"),
	}
	file := &models.File{
		FileType:           models.FileTypeM4B,
		FileRole:           models.FileRoleMain,
		CoverImageFilename: ptrStr("cover.jpg"),
	}
	require.Contains(t, MissingFields(book, file, Default()), "narrators")
}

func TestMissingFields_M4BComplete(t *testing.T) {
	t.Parallel()
	book := &models.Book{
		Authors:     []*models.Author{{}},
		BookGenres:  []*models.BookGenre{{}},
		Description: ptrStr("desc"),
	}
	file := &models.File{
		FileType:           models.FileTypeM4B,
		FileRole:           models.FileRoleMain,
		CoverImageFilename: ptrStr("cover.jpg"),
		Narrators:          []*models.Narrator{{}},
	}
	require.Empty(t, MissingFields(book, file, Default()))
}

func TestMissingFields_NonAudio_DoesNotCheckAudioFields(t *testing.T) {
	t.Parallel()
	criteria := Criteria{
		BookFields:  []string{},
		AudioFields: []string{"narrators"},
	}
	book := &models.Book{}
	file := &models.File{FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain}
	require.Empty(t, MissingFields(book, file, criteria))
}

func TestMissingFields_AllFields(t *testing.T) {
	t.Parallel()
	criteria := Criteria{
		BookFields: []string{
			FieldAuthors, FieldDescription, FieldCover, FieldGenres, FieldTags,
			FieldSeries, FieldSubtitle, FieldPublisher, FieldImprint,
			FieldIdentifiers, FieldReleaseDate, FieldLanguage, FieldURL,
		},
		AudioFields: []string{FieldNarrators, FieldChapters, FieldAbridged},
	}
	book := &models.Book{
		Authors:     []*models.Author{{}},
		BookGenres:  []*models.BookGenre{{}},
		BookTags:    []*models.BookTag{{}},
		BookSeries:  []*models.BookSeries{{}},
		Description: ptrStr("desc"),
		Subtitle:    ptrStr("sub"),
	}
	file := &models.File{
		FileType:           models.FileTypeM4B,
		FileRole:           models.FileRoleMain,
		CoverImageFilename: ptrStr("c.jpg"),
		PublisherID:        ptrInt(1),
		ImprintID:          ptrInt(1),
		Identifiers:        []*models.FileIdentifier{{}},
		ReleaseDate:        ptrTime(time.Now()),
		Language:           ptrStr("en"),
		URL:                ptrStr("https://example"),
		Narrators:          []*models.Narrator{{}},
		Chapters:           []*models.Chapter{{}},
		Abridged:           ptrBool(false),
	}
	require.Empty(t, MissingFields(book, file, criteria))
}

func TestIsComplete_True(t *testing.T) {
	t.Parallel()
	book := &models.Book{
		Authors:     []*models.Author{{}},
		BookGenres:  []*models.BookGenre{{}},
		Description: ptrStr("d"),
	}
	file := &models.File{
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		CoverImageFilename: ptrStr("c.jpg"),
	}
	require.True(t, IsComplete(book, file, Default()))
}

func TestIsComplete_False(t *testing.T) {
	t.Parallel()
	require.False(t, IsComplete(&models.Book{}, &models.File{
		FileType: models.FileTypeEPUB,
		FileRole: models.FileRoleMain,
	}, Default()))
}
```

- [ ] **Step 5.6: Run tests**

```bash
go test ./pkg/books/review/... -race
```

Expected: PASS.

- [ ] **Step 5.7: Commit**

```bash
git add pkg/books/review/
git commit -m "[Backend] Add review criteria and completeness rule"
```

---

### Task 6: Recompute helpers

**Files:**
- Create: `pkg/books/review/recompute.go`
- Create: `pkg/books/review/recompute_test.go`

- [ ] **Step 6.1: Write the recompute helpers**

`pkg/books/review/recompute.go`:

```go
package review

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RecomputeForFile reloads the file (with relations needed by completeness),
// computes its `reviewed` value, and persists. Override-set rows short-circuit.
// Supplements get reviewed=NULL.
func RecomputeForFile(ctx context.Context, db bun.IDB, fileID int, criteria Criteria) error {
	file := &models.File{}
	err := db.NewSelect().
		Model(file).
		Where("f.id = ?", fileID).
		Relation("Narrators").
		Relation("Identifiers").
		Relation("Chapters").
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	if file.FileRole == models.FileRoleSupplement {
		_, err := db.NewUpdate().
			Model((*models.File)(nil)).
			Set("reviewed = NULL").
			Where("id = ?", fileID).
			Exec(ctx)
		return errors.WithStack(err)
	}

	book := &models.Book{}
	err = db.NewSelect().
		Model(book).
		Where("b.id = ?", file.BookID).
		Relation("Authors").
		Relation("BookSeries").
		Relation("BookGenres").
		Relation("BookTags").
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	reviewed := computeReviewedValue(book, file, criteria)
	_, err = db.NewUpdate().
		Model((*models.File)(nil)).
		Set("reviewed = ?", reviewed).
		Where("id = ?", fileID).
		Exec(ctx)
	return errors.WithStack(err)
}

// RecomputeForBook recomputes reviewed for every file belonging to the book.
func RecomputeForBook(ctx context.Context, db bun.IDB, bookID int, criteria Criteria) error {
	var fileIDs []int
	err := db.NewSelect().
		Model((*models.File)(nil)).
		Column("id").
		Where("book_id = ?", bookID).
		Scan(ctx, &fileIDs)
	if err != nil {
		return errors.WithStack(err)
	}
	for _, id := range fileIDs {
		if err := RecomputeForFile(ctx, db, id, criteria); err != nil {
			return err
		}
	}
	return nil
}

// SetOverride writes an explicit override and the timestamp, then writes the
// effective `reviewed` value. Pass override=nil to clear (then completeness
// drives reviewed).
func SetOverride(ctx context.Context, db bun.IDB, fileID int, override *string, criteria Criteria) error {
	if override != nil && *override != models.ReviewOverrideReviewed && *override != models.ReviewOverrideUnreviewed {
		return errors.Errorf("invalid override value: %q", *override)
	}
	now := time.Now().UTC()
	q := db.NewUpdate().Model((*models.File)(nil)).Where("id = ?", fileID)
	if override == nil {
		q = q.Set("review_override = NULL").Set("review_overridden_at = NULL")
	} else {
		q = q.Set("review_override = ?", *override).Set("review_overridden_at = ?", now)
	}
	if _, err := q.Exec(ctx); err != nil {
		return errors.WithStack(err)
	}
	return RecomputeForFile(ctx, db, fileID, criteria)
}

// computeReviewedValue returns the effective reviewed value for a (book, main-file)
// pair given the override and completeness. Returns nil for supplements (caller
// is responsible for that branch).
func computeReviewedValue(book *models.Book, file *models.File, criteria Criteria) *bool {
	if file.ReviewOverride != nil {
		switch *file.ReviewOverride {
		case models.ReviewOverrideReviewed:
			t := true
			return &t
		case models.ReviewOverrideUnreviewed:
			f := false
			return &f
		}
	}
	v := IsComplete(book, file, criteria)
	return &v
}
```

- [ ] **Step 6.2: Write the recompute test**

`pkg/books/review/recompute_test.go`:

```go
package review

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/database"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestRecomputeForFile_Incomplete_SetsFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := database.NewTestDB(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/x.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, RecomputeForFile(ctx, db, file.ID, Default()))

	var got bool
	err = db.NewSelect().Table("files").Column("reviewed").Where("id = ?", file.ID).Scan(ctx, &got)
	require.NoError(t, err)
	require.False(t, got)
}

func TestRecomputeForFile_OverrideReviewed_ShortCircuits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := database.NewTestDB(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	override := models.ReviewOverrideReviewed
	file := &models.File{
		LibraryID:      library.ID,
		BookID:         book.ID,
		Filepath:       "/tmp/x.epub",
		FileType:       models.FileTypeEPUB,
		FileRole:       models.FileRoleMain,
		ReviewOverride: &override,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// File has no metadata, but override should win
	require.NoError(t, RecomputeForFile(ctx, db, file.ID, Default()))

	var got bool
	err = db.NewSelect().Table("files").Column("reviewed").Where("id = ?", file.ID).Scan(ctx, &got)
	require.NoError(t, err)
	require.True(t, got)
}

func TestRecomputeForFile_Supplement_SetsNull(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := database.NewTestDB(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/x.pdf",
		FileType:  models.FileTypePDF,
		FileRole:  models.FileRoleSupplement,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, RecomputeForFile(ctx, db, file.ID, Default()))

	var got *bool
	err = db.NewSelect().Table("files").Column("reviewed").Where("id = ?", file.ID).Scan(ctx, &got)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestSetOverride_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := database.NewTestDB(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/x.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	v := models.ReviewOverrideReviewed
	require.NoError(t, SetOverride(ctx, db, file.ID, &v, Default()))

	var got models.File
	err = db.NewSelect().Model(&got).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.ReviewOverride)
	require.Equal(t, "reviewed", *got.ReviewOverride)
	require.NotNil(t, got.ReviewOverriddenAt)
	require.NotNil(t, got.Reviewed)
	require.True(t, *got.Reviewed)

	// Clear
	require.NoError(t, SetOverride(ctx, db, file.ID, nil, Default()))
	err = db.NewSelect().Model(&got).Where("id = ?", file.ID).Scan(ctx)
	require.NoError(t, err)
	require.Nil(t, got.ReviewOverride)
	require.Nil(t, got.ReviewOverriddenAt)
	// Now driven by completeness — incomplete book → false
	require.False(t, *got.Reviewed)
}
```

- [ ] **Step 6.3: Run tests**

```bash
go test ./pkg/books/review/... -race
```

Expected: PASS.

- [ ] **Step 6.4: Commit**

```bash
git add pkg/books/review/recompute.go pkg/books/review/recompute_test.go
git commit -m "[Backend] Add review recompute and override helpers"
```

---

## Phase 4 — Recompute job

### Task 7: Add `recompute_review` job type

**Files:**
- Modify: `pkg/models/job.go`

- [ ] **Step 7.1: Add the constant and payload**

In `pkg/models/job.go`:

Update the `tygo:emit` line:

```go
//tygo:emit export type JobType = typeof JobTypeExport | typeof JobTypeScan | typeof JobTypeBulkDownload | typeof JobTypeHashGeneration | typeof JobTypeRecomputeReview;
```

Add the constant in the same `const` block:

```go
JobTypeRecomputeReview = "recompute_review"
```

Update the `tstype` on `DataParsed`:

```go
DataParsed interface{} `bun:"-" json:"data" tstype:"JobExportData | JobScanData | JobBulkDownloadData | JobHashGenerationData | JobRecomputeReviewData"`
```

Add the `case` to `UnmarshalData`:

```go
case JobTypeRecomputeReview:
	job.DataParsed = &JobRecomputeReviewData{}
```

Add the payload struct at the end of the file:

```go
type JobRecomputeReviewData struct {
	// ClearOverrides, when true, sets review_override and review_overridden_at to NULL
	// for every main file before recomputing reviewed.
	ClearOverrides bool `json:"clear_overrides"`
}
```

- [ ] **Step 7.2: Regenerate types**

```bash
mise tygo
```

- [ ] **Step 7.3: Verify build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 7.4: Commit**

```bash
git add pkg/models/job.go app/types/generated/
git commit -m "[Backend] Add recompute_review job type"
```

---

### Task 8: Worker handler `ProcessRecomputeReviewJob`

**Files:**
- Create: `pkg/worker/recompute_review.go`
- Create: `pkg/worker/recompute_review_test.go`
- Modify: `pkg/worker/worker.go` (registration)

- [ ] **Step 8.1: Write the failing test first**

`pkg/worker/recompute_review_test.go`:

```go
package worker

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestProcessRecomputeReviewJob_PopulatesReviewed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newTestWorker(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := w.db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T"}
	_, err = w.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	main := &models.File{
		LibraryID: library.ID, BookID: book.ID, Filepath: "/tmp/m.epub",
		FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain,
	}
	_, err = w.db.NewInsert().Model(main).Exec(ctx)
	require.NoError(t, err)
	supp := &models.File{
		LibraryID: library.ID, BookID: book.ID, Filepath: "/tmp/s.pdf",
		FileType: models.FileTypePDF, FileRole: models.FileRoleSupplement,
	}
	_, err = w.db.NewInsert().Model(supp).Exec(ctx)
	require.NoError(t, err)

	job := &models.Job{
		Type:   models.JobTypeRecomputeReview,
		Status: models.JobStatusPending,
		Data:   `{"clear_overrides":false}`,
	}
	_, err = w.db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, w.ProcessRecomputeReviewJob(ctx, job))

	var mainReviewed *bool
	require.NoError(t, w.db.NewSelect().Table("files").Column("reviewed").Where("id = ?", main.ID).Scan(ctx, &mainReviewed))
	require.NotNil(t, mainReviewed)
	require.False(t, *mainReviewed)

	var suppReviewed *bool
	require.NoError(t, w.db.NewSelect().Table("files").Column("reviewed").Where("id = ?", supp.ID).Scan(ctx, &suppReviewed))
	require.Nil(t, suppReviewed)
}

func TestProcessRecomputeReviewJob_ClearOverrides(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newTestWorker(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := w.db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T"}
	_, err = w.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	override := models.ReviewOverrideReviewed
	file := &models.File{
		LibraryID: library.ID, BookID: book.ID, Filepath: "/tmp/x.epub",
		FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain,
		ReviewOverride: &override,
	}
	_, err = w.db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	job := &models.Job{
		Type:   models.JobTypeRecomputeReview,
		Status: models.JobStatusPending,
		Data:   `{"clear_overrides":true}`,
	}
	_, err = w.db.NewInsert().Model(job).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, w.ProcessRecomputeReviewJob(ctx, job))

	var got models.File
	require.NoError(t, w.db.NewSelect().Model(&got).Where("id = ?", file.ID).Scan(ctx))
	require.Nil(t, got.ReviewOverride)
	require.Nil(t, got.ReviewOverriddenAt)
	require.NotNil(t, got.Reviewed)
	require.False(t, *got.Reviewed)
}

// Note: newTestWorker is the existing helper in pkg/worker/testhelpers_test.go.
// If it doesn't already wire an appsettings service, extend it as part of this task.
// The recompute job needs access to appsettings.Service to load criteria.
var _ = appsettings.NewService
var _ = review.Default
```

- [ ] **Step 8.2: Run test to verify failure**

```bash
go test -run TestProcessRecomputeReviewJob ./pkg/worker/... -race
```

Expected: FAIL — undefined: ProcessRecomputeReviewJob.

- [ ] **Step 8.3: Write the worker method**

`pkg/worker/recompute_review.go`:

```go
package worker

import (
	"context"

	"github.com/pkg/errors"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/models"
)

// ProcessRecomputeReviewJob iterates every file in the database and recomputes
// `reviewed`. When ClearOverrides is set, it first wipes overrides for all main
// files. Supplements get reviewed=NULL.
//
// Progress: emits 1..100 as integer percent based on processed/total files.
func (w *Worker) ProcessRecomputeReviewJob(ctx context.Context, job *models.Job) error {
	var data models.JobRecomputeReviewData
	if err := json.Unmarshal([]byte(job.Data), &data); err != nil {
		return errors.WithStack(err)
	}

	if data.ClearOverrides {
		if _, err := w.db.NewUpdate().
			Model((*models.File)(nil)).
			Set("review_override = NULL").
			Set("review_overridden_at = NULL").
			Where("file_role = ?", models.FileRoleMain).
			Exec(ctx); err != nil {
			return errors.WithStack(err)
		}
	}

	criteria, err := review.Load(ctx, w.appSettingsService)
	if err != nil {
		return errors.WithStack(err)
	}

	var fileIDs []int
	if err := w.db.NewSelect().
		Model((*models.File)(nil)).
		Column("id").
		Order("id ASC").
		Scan(ctx, &fileIDs); err != nil {
		return errors.WithStack(err)
	}

	total := len(fileIDs)
	for i, id := range fileIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := review.RecomputeForFile(ctx, w.db, id, criteria); err != nil {
			return errors.WithStack(err)
		}
		if total > 0 {
			pct := int(float64(i+1) / float64(total) * 100)
			if err := w.jobService.UpdateProgress(ctx, job.ID, pct); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	return nil
}
```

- [ ] **Step 8.4: Wire the appsettings service into the worker**

In `pkg/worker/worker.go`:

1. Add an `appSettingsService *appsettings.Service` field to the `Worker` struct.
2. Add `appSettingsService *appsettings.Service` parameter to `New(...)` and assign it.
3. In the dispatch map, add: `models.JobTypeRecomputeReview: w.ProcessRecomputeReviewJob,`.
4. Update every caller of `worker.New(...)` (typically `cmd/api/main.go`) to pass an `appsettings.NewService(db)` instance.

(The exact form depends on the current `New` signature. Match the pattern of other `*Service` fields like `jobService`, `bookService`, etc.)

- [ ] **Step 8.5: Update the test helper**

In `pkg/worker/testhelpers_test.go` (existing helper used by other worker tests), wire an `appsettings.Service` into `newTestWorker`. The expected signature change is to instantiate `appsettings.NewService(db)` and pass it through to `worker.New(...)`.

- [ ] **Step 8.6: Run tests**

```bash
go test ./pkg/worker/... -race
mise check:quiet
```

Expected: all pass.

- [ ] **Step 8.7: Commit**

```bash
git add pkg/worker/ cmd/api/
git commit -m "[Backend] Add recompute_review worker handler"
```

---

## Phase 5 — Migration enqueues recompute

### Task 9: Initial population migration step

**Files:**
- Create: `pkg/migrations/20260425100200_seed_review_criteria_and_enqueue_recompute.go`

- [ ] **Step 9.1: Write the migration**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/segmentio/encoding/json"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// Seed default review criteria.
		defaultJSON, err := json.Marshal(map[string]interface{}{
			"book_fields":  []string{"authors", "description", "cover", "genres"},
			"audio_fields": []string{"narrators"},
		})
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO app_settings (key, value, updated_at)
			VALUES ('review_criteria', ?, CURRENT_TIMESTAMP)
			ON CONFLICT(key) DO NOTHING
		`, string(defaultJSON)); err != nil {
			return errors.WithStack(err)
		}

		// Enqueue a recompute job so the worker fills in files.reviewed
		// asynchronously after migrations finish. Until then, files.reviewed
		// is NULL and the books/list query treats NULL as needs-review.
		jobData, err := json.Marshal(map[string]interface{}{"clear_overrides": false})
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO jobs (type, status, data, progress, created_at, updated_at)
			VALUES ('recompute_review', 'pending', ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, string(jobData)); err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		stmts := []string{
			`DELETE FROM jobs WHERE type = 'recompute_review'`,
			`DELETE FROM app_settings WHERE key = 'review_criteria'`,
		}
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 9.2: Verify roundtrip**

```bash
mise db:rollback && mise db:rollback && mise db:rollback && mise db:migrate
```

Expected: no errors. (The triple-rollback is to undo the migrations from this plan; adjust based on what's currently applied.)

- [ ] **Step 9.3: Commit**

```bash
git add pkg/migrations/20260425100200_seed_review_criteria_and_enqueue_recompute.go
git commit -m "[Backend] Seed default review criteria and enqueue initial recompute"
```

---

## Phase 6 — Books service: hook recompute into mutations

### Task 10: Service-level recompute helpers

**Files:**
- Modify: `pkg/books/service.go`

We'll add two thin wrappers that load the current criteria and call into `pkg/books/review`. These are the call sites every mutation will use.

- [ ] **Step 10.1: Add Service field**

In `pkg/books/service.go`, add `appSettingsService *appsettings.Service` to the `Service` struct (along with the existing dependencies). Update `NewService` to accept and store it. Update every caller of `books.NewService(...)` (search the repo for `books.NewService(`) to pass an appsettings instance.

- [ ] **Step 10.2: Add wrappers**

In `pkg/books/service.go`, add at the bottom of the file:

```go
// recomputeReviewedForFile loads the active criteria and refreshes files.reviewed
// for the given file. Errors are logged but do not propagate to the caller —
// review state is non-critical metadata.
func (svc *Service) recomputeReviewedForFile(ctx context.Context, fileID int) {
	criteria, err := review.Load(ctx, svc.appSettingsService)
	if err != nil {
		logger.Warn(ctx, "review: load criteria failed", logger.Data{"err": err.Error()})
		return
	}
	if err := review.RecomputeForFile(ctx, svc.db, fileID, criteria); err != nil {
		logger.Warn(ctx, "review: recompute for file failed", logger.Data{"err": err.Error(), "file_id": fileID})
	}
}

// recomputeReviewedForBook refreshes files.reviewed for every file of the book.
func (svc *Service) recomputeReviewedForBook(ctx context.Context, bookID int) {
	criteria, err := review.Load(ctx, svc.appSettingsService)
	if err != nil {
		logger.Warn(ctx, "review: load criteria failed", logger.Data{"err": err.Error()})
		return
	}
	if err := review.RecomputeForBook(ctx, svc.db, bookID, criteria); err != nil {
		logger.Warn(ctx, "review: recompute for book failed", logger.Data{"err": err.Error(), "book_id": bookID})
	}
}
```

Add the imports: `github.com/shishobooks/shisho/pkg/appsettings`, `github.com/shishobooks/shisho/pkg/books/review`, and the project's `logger` package (match what's already imported in `service.go`).

- [ ] **Step 10.3: Hook into write paths**

Add a call to the appropriate wrapper at the end of each successful mutation inside `pkg/books/service.go`:

- `CreateFile` → `svc.recomputeReviewedForFile(ctx, file.ID)`
- `UpdateFile` → `svc.recomputeReviewedForFile(ctx, file.ID)`
- `UpdateBook` → `svc.recomputeReviewedForBook(ctx, book.ID)`
- `DeleteFile` / `DeleteFilesByIDs` → no-op (file is gone — nothing to recompute)
- `CreateFileIdentifier`, `DeleteFileIdentifiers` → `svc.recomputeReviewedForFile(ctx, fileID)`

For the book-relation services living in their own packages (`pkg/genres/service.go`, `pkg/tags/service.go`, `pkg/authors/service.go`, `pkg/series/service.go`, `pkg/people/service.go`, `pkg/publishers/service.go`, `pkg/imprints/service.go`, `pkg/chapters/service.go`, `pkg/narrators/`, `pkg/identifiers/`), each FindOrCreate / Delete / Replace / Update method that mutates a book or file relation should call back into `bookService.RecomputeReviewedForBook(...)` (book-level relations) or `bookService.RecomputeReviewedForFile(...)` (file-level relations). Promote `recomputeReviewedForFile` / `recomputeReviewedForBook` to **exported** methods (`RecomputeReviewedForFile` / `RecomputeReviewedForBook`) so the other packages can call them. The wrappers' implementations stay the same; only the receiver method names change.

Touch points (search for the relation name in handlers/services to confirm exhaustive coverage):

| Relation | Service / Function | Recompute target |
|---|---|---|
| Authors | `pkg/books/handlers.go: updateBookAuthors` (or wherever authors are replaced for a book) | book |
| Genres | `pkg/genres/service.go: AssociateGenresToBook` (and any direct INSERT into `book_genres`) | book |
| Tags | `pkg/tags/service.go: AssociateTagsToBook` (and direct INSERT into `book_tags`) | book |
| Series | `pkg/series/service.go: AssociateSeriesToBook` (and `book_series` writes) | book |
| Narrators | `pkg/narrators/...` or wherever they're attached to a file | file |
| Identifiers | `pkg/books/service.go: CreateFileIdentifier`, `DeleteFileIdentifiers` | file (already covered above) |
| Chapters | `pkg/chapters/service.go: ReplaceChapters` | file |
| Publisher / Imprint | `pkg/books/service.go: UpdateFile` (already covered, since these are file columns) | file |

If you find a relation-mutation path that doesn't go through the books service, add a `bookService` dependency to its package the same way `searchService` is wired up today and call the recompute helper there.

- [ ] **Step 10.4: Verify build and tests**

```bash
mise check:quiet
```

Expected: PASS.

- [ ] **Step 10.5: Commit**

```bash
git add pkg/
git commit -m "[Backend] Recompute reviewed on book/file mutations"
```

---

### Task 11: Hook recompute into scan/enrichment

**Files:**
- Modify: `pkg/worker/scan_unified.go`

- [ ] **Step 11.1: Identify the enrich apply path**

Open `pkg/worker/scan_unified.go` and find the spot inside `runMetadataEnrichers` (or its called helpers) where enricher metadata is persisted to the file/book. After the persist, call `review.RecomputeForFile(ctx, w.db, file.ID, criteria)`. Load `criteria` once near the top of the enrichment phase using `review.Load(ctx, w.appSettingsService)`; reuse the same value for every file in the batch.

- [ ] **Step 11.2: Add explicit recompute after the scan-level mutations**

Anywhere the scan job creates or updates files (`scanFileCreateNew`, `scanFileByPath` update branch, etc.) and the call doesn't already go through `bookService.CreateFile` / `bookService.UpdateFile`, add a recompute call right after the persist. Use the criteria value loaded at the top of the scan.

- [ ] **Step 11.3: Add a test**

Extend `pkg/worker/scan_enricher_test.go` (or a sibling test if more appropriate) with a case asserting that enricher results that fill all required fields produce `files.reviewed = TRUE`, and partial enrichment (missing one required field) produces `files.reviewed = FALSE`. Use `t.Parallel()`.

- [ ] **Step 11.4: Run tests**

```bash
go test ./pkg/worker/... -race
```

Expected: PASS.

- [ ] **Step 11.5: Commit**

```bash
git add pkg/worker/
git commit -m "[Backend] Recompute reviewed after scan and enrichment"
```

---

## Phase 7 — API endpoints

### Task 12: PATCH /books/files/:id/review

**Files:**
- Modify: `pkg/books/handlers.go` (or split into `handlers_review.go` if `handlers.go` is already large — current file is large, split is preferred)
- Create: `pkg/books/handlers_review.go`
- Modify: `pkg/books/routes.go`
- Create: `pkg/books/handlers_review_test.go`

- [ ] **Step 12.1: Write the failing handler test**

`pkg/books/handlers_review_test.go`:

```go
package books

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestSetFileReview_SetsOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h, db := newTestHandler(t) // existing helper

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{
		LibraryID: library.ID, BookID: book.ID, Filepath: "/tmp/x.epub",
		FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	body := []byte(`{"override":"reviewed"}`)
	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/books/files/:id/review")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(file.ID))
	setUserOnContext(c, &models.User{ID: 1, /* admin user setup */})

	require.NoError(t, h.setFileReview(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var got models.File
	require.NoError(t, db.NewSelect().Model(&got).Where("id = ?", file.ID).Scan(ctx))
	require.NotNil(t, got.ReviewOverride)
	require.Equal(t, "reviewed", *got.ReviewOverride)
	require.NotNil(t, got.Reviewed)
	require.True(t, *got.Reviewed)
}

func TestSetFileReview_ClearsOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h, db := newTestHandler(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	override := models.ReviewOverrideUnreviewed
	file := &models.File{
		LibraryID: library.ID, BookID: book.ID, Filepath: "/tmp/x.epub",
		FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain,
		ReviewOverride: &override,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	body := []byte(`{"override":null}`)
	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/books/files/:id/review")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(file.ID))
	setUserOnContext(c, &models.User{ID: 1})

	require.NoError(t, h.setFileReview(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var got models.File
	require.NoError(t, db.NewSelect().Model(&got).Where("id = ?", file.ID).Scan(ctx))
	require.Nil(t, got.ReviewOverride)
	require.Nil(t, got.ReviewOverriddenAt)
}
```

(The test uses an existing `newTestHandler` helper — if one doesn't exist, add it modeled on `handlers_test.go` setup.)

- [ ] **Step 12.2: Run to verify failure**

```bash
go test -run TestSetFileReview ./pkg/books/... -race
```

Expected: FAIL — undefined: `setFileReview`.

- [ ] **Step 12.3: Implement the handler**

`pkg/books/handlers_review.go`:

```go
package books

import (
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// SetReviewPayload is shared by the file, book, and bulk endpoints.
type SetReviewPayload struct {
	// Override: nil clears the override. "reviewed" or "unreviewed" sets it.
	Override *string `json:"override" validate:"omitempty,oneof=reviewed unreviewed"`
}

func (h *Handler) setFileReview(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.BadRequest("Invalid file id")
	}

	var payload SetReviewPayload
	if err := c.Bind(&payload); err != nil {
		return err
	}

	ctx := c.Request().Context()
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: id})
	if err != nil {
		return err
	}
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	criteria, err := review.Load(ctx, h.appSettingsService)
	if err != nil {
		return err
	}
	if err := review.SetOverride(ctx, h.bookService.DB(), id, payload.Override, criteria); err != nil {
		return err
	}

	updated, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: id})
	if err != nil {
		return err
	}
	return c.JSON(200, updated)
}
```

Notes:
- Add `appSettingsService *appsettings.Service` to the books `Handler` struct and wire it in `NewHandler`. Update every caller of `NewHandler`.
- If `bookService.DB()` doesn't exist, expose it via a small accessor `func (svc *Service) DB() *bun.DB { return svc.db }`. The function is the simplest way to give the handler the DB without leaking the field.

- [ ] **Step 12.4: Register the route**

In `pkg/books/routes.go`, add inside the books group with `books:write` permission and library-access middleware (matching neighbors like `setPrimaryFile`):

```go
g.PATCH("/files/:id/review", h.setFileReview, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

- [ ] **Step 12.5: Run test to verify pass**

```bash
go test -run TestSetFileReview ./pkg/books/... -race
```

Expected: PASS.

- [ ] **Step 12.6: Commit**

```bash
git add pkg/books/
git commit -m "[Backend] Add PATCH /books/files/:id/review endpoint"
```

---

### Task 13: PATCH /books/:id/review (cascade)

**Files:**
- Modify: `pkg/books/handlers_review.go`
- Modify: `pkg/books/routes.go`
- Modify: `pkg/books/handlers_review_test.go`

- [ ] **Step 13.1: Write a test**

Add to `pkg/books/handlers_review_test.go`:

```go
func TestSetBookReview_CascadesToAllFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h, db := newTestHandler(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{LibraryID: library.ID, Title: "T"}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	for _, ft := range []string{models.FileTypeEPUB, models.FileTypeM4B} {
		f := &models.File{
			LibraryID: library.ID, BookID: book.ID,
			Filepath: "/tmp/" + ft, FileType: ft, FileRole: models.FileRoleMain,
		}
		_, err = db.NewInsert().Model(f).Exec(ctx)
		require.NoError(t, err)
	}

	body := []byte(`{"override":"reviewed"}`)
	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/books/:id/review")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(book.ID))
	setUserOnContext(c, &models.User{ID: 1})

	require.NoError(t, h.setBookReview(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var rows []*models.File
	require.NoError(t, db.NewSelect().Model(&rows).Where("book_id = ?", book.ID).Scan(ctx))
	for _, f := range rows {
		require.NotNil(t, f.ReviewOverride)
		require.Equal(t, "reviewed", *f.ReviewOverride)
	}
}
```

- [ ] **Step 13.2: Implement cascade**

In `pkg/books/handlers_review.go`:

```go
func (h *Handler) setBookReview(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.BadRequest("Invalid book id")
	}

	var payload SetReviewPayload
	if err := c.Bind(&payload); err != nil {
		return err
	}

	ctx := c.Request().Context()
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: id})
	if err != nil {
		return err
	}
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	criteria, err := review.Load(ctx, h.appSettingsService)
	if err != nil {
		return err
	}
	for _, f := range book.Files {
		if f.FileRole != models.FileRoleMain {
			continue
		}
		if err := review.SetOverride(ctx, h.bookService.DB(), f.ID, payload.Override, criteria); err != nil {
			return err
		}
	}

	updated, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: id})
	if err != nil {
		return err
	}
	return c.JSON(200, updated)
}
```

- [ ] **Step 13.3: Register the route**

In `pkg/books/routes.go`:

```go
g.PATCH("/:id/review", h.setBookReview, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

- [ ] **Step 13.4: Run tests**

```bash
go test -run TestSetBookReview ./pkg/books/... -race
```

Expected: PASS.

- [ ] **Step 13.5: Commit**

```bash
git add pkg/books/
git commit -m "[Backend] Add PATCH /books/:id/review cascade endpoint"
```

---

### Task 14: POST /books/bulk/review

**Files:**
- Modify: `pkg/books/handlers_review.go`
- Modify: `pkg/books/routes.go`
- Modify: `pkg/books/handlers_review_test.go`

- [ ] **Step 14.1: Write the test**

Add to `pkg/books/handlers_review_test.go`:

```go
func TestBulkSetReview_AppliesToAllSpecifiedBooks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h, db := newTestHandler(t)

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	bookIDs := make([]int, 0, 2)
	for i := 0; i < 2; i++ {
		book := &models.Book{LibraryID: library.ID, Title: "T" + strconv.Itoa(i)}
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)
		bookIDs = append(bookIDs, book.ID)
		f := &models.File{
			LibraryID: library.ID, BookID: book.ID,
			Filepath: "/tmp/" + strconv.Itoa(i), FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain,
		}
		_, err = db.NewInsert().Model(f).Exec(ctx)
		require.NoError(t, err)
	}

	payload := []byte(`{"book_ids":[` + strconv.Itoa(bookIDs[0]) + `,` + strconv.Itoa(bookIDs[1]) + `],"override":"reviewed"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/books/bulk/review")
	setUserOnContext(c, &models.User{ID: 1})

	require.NoError(t, h.bulkSetReview(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var files []*models.File
	require.NoError(t, db.NewSelect().Model(&files).Where("book_id IN (?)", bun.In(bookIDs)).Scan(ctx))
	for _, f := range files {
		require.NotNil(t, f.ReviewOverride)
		require.Equal(t, "reviewed", *f.ReviewOverride)
	}
}
```

- [ ] **Step 14.2: Implement the handler**

In `pkg/books/handlers_review.go`:

```go
type BulkSetReviewPayload struct {
	BookIDs  []int   `json:"book_ids" validate:"required,min=1,max=500"`
	Override *string `json:"override" validate:"omitempty,oneof=reviewed unreviewed"`
}

func (h *Handler) bulkSetReview(c echo.Context) error {
	var payload BulkSetReviewPayload
	if err := c.Bind(&payload); err != nil {
		return err
	}

	ctx := c.Request().Context()
	user, _ := c.Get("user").(*models.User)

	criteria, err := review.Load(ctx, h.appSettingsService)
	if err != nil {
		return err
	}

	for _, bookID := range payload.BookIDs {
		book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: bookID})
		if err != nil {
			continue // silently skip missing books — consistent with bulk delete
		}
		if user != nil && !user.HasLibraryAccess(book.LibraryID) {
			continue
		}
		for _, f := range book.Files {
			if f.FileRole != models.FileRoleMain {
				continue
			}
			if err := review.SetOverride(ctx, h.bookService.DB(), f.ID, payload.Override, criteria); err != nil {
				return err
			}
		}
	}
	return c.NoContent(204)
}
```

- [ ] **Step 14.3: Register the route**

In `pkg/books/routes.go`:

```go
g.POST("/bulk/review", h.bulkSetReview, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

- [ ] **Step 14.4: Run tests**

```bash
go test -run TestBulkSetReview ./pkg/books/... -race
```

Expected: PASS.

- [ ] **Step 14.5: Commit**

```bash
git add pkg/books/
git commit -m "[Backend] Add POST /books/bulk/review endpoint"
```

---

### Task 15: Add `reviewed_filter` query param to GET /books

**Files:**
- Modify: `pkg/books/service.go` (`ListBooksOptions`, `listBooksWithTotal`)
- Modify: `pkg/books/handlers.go` (extract param)
- Modify: `pkg/books/service_filter_test.go` (extend)

- [ ] **Step 15.1: Add the option**

In `pkg/books/service.go` `ListBooksOptions`:

```go
ReviewedFilter string // "" (default = all), "needs_review", "reviewed"
```

- [ ] **Step 15.2: Add a failing filter test**

In `pkg/books/service_filter_test.go`, add:

```go
func TestListBooks_ReviewedFilter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestService(t) // existing helper

	library := &models.Library{Name: "L", CoverAspectRatio: "book"}
	_, err := svc.db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	mkBook := func(reviewed *bool) int {
		book := &models.Book{LibraryID: library.ID, Title: "T"}
		_, err := svc.db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)
		f := &models.File{
			LibraryID: library.ID, BookID: book.ID, Filepath: "/tmp/" + strconv.Itoa(book.ID),
			FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain,
			Reviewed: reviewed,
		}
		_, err = svc.db.NewInsert().Model(f).Exec(ctx)
		require.NoError(t, err)
		return book.ID
	}
	tru := true
	fal := false
	_ = mkBook(&tru)
	bookFalse := mkBook(&fal)
	bookNull := mkBook(nil)

	books, _, err := svc.ListBooksWithTotal(ctx, ListBooksOptions{LibraryID: &library.ID, ReviewedFilter: "needs_review"})
	require.NoError(t, err)
	gotIDs := make([]int, 0, len(books))
	for _, b := range books {
		gotIDs = append(gotIDs, b.ID)
	}
	require.ElementsMatch(t, []int{bookFalse, bookNull}, gotIDs)
}
```

- [ ] **Step 15.3: Apply filter in `listBooksWithTotal`**

In `pkg/books/service.go`, inside `listBooksWithTotal`, after the existing filter-application section:

```go
switch opts.ReviewedFilter {
case "needs_review":
	q = q.Where(`EXISTS (
		SELECT 1 FROM files f_rev
		WHERE f_rev.book_id = b.id
		  AND f_rev.file_role = 'main'
		  AND (f_rev.reviewed = FALSE OR f_rev.reviewed IS NULL)
	)`)
case "reviewed":
	q = q.Where(`NOT EXISTS (
		SELECT 1 FROM files f_rev
		WHERE f_rev.book_id = b.id
		  AND f_rev.file_role = 'main'
		  AND (f_rev.reviewed = FALSE OR f_rev.reviewed IS NULL)
	)`).Where(`EXISTS (
		SELECT 1 FROM files f_rev2
		WHERE f_rev2.book_id = b.id AND f_rev2.file_role = 'main'
	)`)
}
```

- [ ] **Step 15.4: Wire the handler**

In `pkg/books/handlers.go` `list` (or whichever method binds query options), parse `c.QueryParam("reviewed_filter")` into `ListBooksOptions.ReviewedFilter`. Validate it's in `{"", "all", "needs_review", "reviewed"}`; treat `"all"` as empty.

- [ ] **Step 15.5: Run tests**

```bash
go test -run TestListBooks_ReviewedFilter ./pkg/books/... -race
```

Expected: PASS.

- [ ] **Step 15.6: Commit**

```bash
git add pkg/books/
git commit -m "[Backend] Add reviewed_filter query param to book list"
```

---

## Phase 8 — Settings API

### Task 16: GET / PUT /settings/review-criteria

**Files:**
- Create: `pkg/settings/review_criteria_handlers.go`
- Modify: `pkg/settings/routes.go`
- Modify: `pkg/settings/handlers.go` (or handlers struct definition) to inject `appSettingsService` + `jobsService`
- Create: `pkg/settings/review_criteria_handlers_test.go`

- [ ] **Step 16.1: Write the failing test**

`pkg/settings/review_criteria_handlers_test.go`:

```go
package settings

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/database"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestGetReviewCriteria_ReturnsDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := database.NewTestDB(t)
	apps := appsettings.NewService(db)
	jobsSvc := jobs.NewService(db)
	h := NewHandler(NewService(db), apps, jobsSvc /* match real signature */)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	require.NoError(t, h.getReviewCriteria(c))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"authors"`)
	_ = ctx
	_ = review.Default
	_ = models.JobTypeRecomputeReview
}

func TestPutReviewCriteria_PersistsAndEnqueuesJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := database.NewTestDB(t)
	apps := appsettings.NewService(db)
	jobsSvc := jobs.NewService(db)
	h := NewHandler(NewService(db), apps, jobsSvc)

	body := []byte(`{"book_fields":["authors"],"audio_fields":[],"clear_overrides":false}`)
	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	require.NoError(t, h.putReviewCriteria(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var saved review.Criteria
	_, err := apps.GetJSON(ctx, review.SettingsKey, &saved)
	require.NoError(t, err)
	require.Equal(t, []string{"authors"}, saved.BookFields)

	var jobCount int
	require.NoError(t, db.NewSelect().Table("jobs").ColumnExpr("count(*)").Where("type = ?", models.JobTypeRecomputeReview).Scan(ctx, &jobCount))
	require.Equal(t, 1, jobCount)
}
```

- [ ] **Step 16.2: Implement handlers**

`pkg/settings/review_criteria_handlers.go`:

```go
package settings

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type reviewCriteriaResponse struct {
	BookFields           []string `json:"book_fields"`
	AudioFields          []string `json:"audio_fields"`
	UniversalCandidates  []string `json:"universal_candidates"`
	AudioCandidates      []string `json:"audio_candidates"`
	OverrideCount        int      `json:"override_count"`
	MainFileCount        int      `json:"main_file_count"`
}

type putReviewCriteriaPayload struct {
	BookFields     []string `json:"book_fields" validate:"required"`
	AudioFields    []string `json:"audio_fields" validate:"required"`
	ClearOverrides bool     `json:"clear_overrides"`
}

func (h *Handler) getReviewCriteria(c echo.Context) error {
	ctx := c.Request().Context()
	criteria, err := review.Load(ctx, h.appSettingsService)
	if err != nil {
		return err
	}
	resp := reviewCriteriaResponse{
		BookFields:          criteria.BookFields,
		AudioFields:         criteria.AudioFields,
		UniversalCandidates: review.UniversalCandidates,
		AudioCandidates:     review.AudioCandidates,
	}
	if err := h.appSettingsService.DB(). /* expose accessor or pass db */ NewSelect().
		Table("files").ColumnExpr("count(*)").Where("file_role = 'main' AND review_override IS NOT NULL").Scan(ctx, &resp.OverrideCount); err != nil {
		return err
	}
	if err := h.appSettingsService.DB().NewSelect().
		Table("files").ColumnExpr("count(*)").Where("file_role = 'main'").Scan(ctx, &resp.MainFileCount); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) putReviewCriteria(c echo.Context) error {
	var payload putReviewCriteriaPayload
	if err := c.Bind(&payload); err != nil {
		return err
	}
	criteria := review.Criteria{BookFields: payload.BookFields, AudioFields: payload.AudioFields}
	if err := review.Validate(criteria); err != nil {
		return errcodes.BadRequest(err.Error())
	}

	ctx := c.Request().Context()
	if err := review.Save(ctx, h.appSettingsService, criteria); err != nil {
		return err
	}
	jobData, _ := json.Marshal(models.JobRecomputeReviewData{ClearOverrides: payload.ClearOverrides})
	if _, err := h.jobsService.CreateJob(ctx, &models.Job{
		Type:   models.JobTypeRecomputeReview,
		Status: models.JobStatusPending,
		Data:   string(jobData),
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}
```

(The `appSettingsService.DB()` calls assume a `DB()` accessor on the appsettings service. Either add the accessor or pass the `*bun.DB` directly into the handler — pick whichever is cleaner with what already exists.)

Update the `Handler` struct in `pkg/settings/handlers.go` to include `appSettingsService *appsettings.Service` and `jobsService *jobs.Service`. Update `NewHandler` and all callers.

- [ ] **Step 16.3: Register routes**

In `pkg/settings/routes.go`:

```go
g.GET("/review-criteria", h.getReviewCriteria, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead))
g.PUT("/review-criteria", h.putReviewCriteria, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
```

(If `ResourceConfig` doesn't have a `write` operation registered, fall back to `ResourceUsers`/admin-only — match how other admin-only settings endpoints are gated.)

- [ ] **Step 16.4: Run tests**

```bash
go test ./pkg/settings/... -race
```

Expected: PASS.

- [ ] **Step 16.5: Commit**

```bash
git add pkg/settings/
git commit -m "[Backend] Add review-criteria settings endpoints"
```

---

### Task 17: Manual recompute endpoint via existing jobs route

The existing `POST /jobs` already supports custom job types. No new endpoint needed — confirm `recompute_review` works through it and add a small handler test if missing.

**Files:**
- Modify: `pkg/jobs/handlers_test.go` (or add `pkg/jobs/recompute_review_test.go`)

- [ ] **Step 17.1: Add a handler test**

```go
func TestCreateJob_RecomputeReview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h, db := newTestHandler(t)

	body := []byte(`{"type":"recompute_review","data":{"clear_overrides":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	setUserOnContext(c, &models.User{ID: 1 /* admin */})

	require.NoError(t, h.create(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var got models.Job
	require.NoError(t, db.NewSelect().Model(&got).Where("type = ?", models.JobTypeRecomputeReview).Scan(ctx))
	require.Equal(t, models.JobStatusPending, got.Status)
}
```

- [ ] **Step 17.2: Run**

```bash
go test ./pkg/jobs/... -race
```

Expected: PASS. If `create` rejects unknown types, add `recompute_review` to the allowlist.

- [ ] **Step 17.3: Commit**

```bash
git add pkg/jobs/
git commit -m "[Backend] Allow recompute_review jobs from POST /jobs"
```

---

## Phase 9 — Frontend types & query hooks

### Task 18: Query hooks

**Files:**
- Create: `app/hooks/queries/review.ts`
- Modify: `app/libraries/api.ts` (add fetcher functions)

- [ ] **Step 18.1: Add fetchers**

In `app/libraries/api.ts`, append:

```ts
export interface ReviewCriteriaResponse {
  book_fields: string[];
  audio_fields: string[];
  universal_candidates: string[];
  audio_candidates: string[];
  override_count: number;
  main_file_count: number;
}

export const getReviewCriteria = (): Promise<ReviewCriteriaResponse> =>
  apiGet("/api/settings/review-criteria");

export const putReviewCriteria = (payload: {
  book_fields: string[];
  audio_fields: string[];
  clear_overrides: boolean;
}): Promise<void> => apiPut("/api/settings/review-criteria", payload);

export const setFileReview = (
  fileId: number,
  override: "reviewed" | "unreviewed" | null,
): Promise<File> =>
  apiPatch(`/api/books/files/${fileId}/review`, { override });

export const setBookReview = (
  bookId: number,
  override: "reviewed" | "unreviewed" | null,
): Promise<Book> =>
  apiPatch(`/api/books/${bookId}/review`, { override });

export const bulkSetReview = (
  bookIds: number[],
  override: "reviewed" | "unreviewed" | null,
): Promise<void> =>
  apiPost("/api/books/bulk/review", { book_ids: bookIds, override });
```

(Match the actual `apiGet` / `apiPost` / `apiPatch` / `apiPut` helper names used elsewhere — if `apiPatch` doesn't exist, add it modeled on `apiPost`.)

- [ ] **Step 18.2: Add hooks**

`app/hooks/queries/review.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  bulkSetReview,
  getReviewCriteria,
  putReviewCriteria,
  setBookReview,
  setFileReview,
} from "@/libraries/api";
import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";

export const QueryKey = {
  ReviewCriteria: ["review-criteria"] as const,
};

export const useReviewCriteria = () =>
  useQuery({ queryKey: QueryKey.ReviewCriteria, queryFn: getReviewCriteria });

export const useUpdateReviewCriteria = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: putReviewCriteria,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QueryKey.ReviewCriteria });
      qc.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      qc.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useSetFileReview = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      fileId,
      override,
    }: {
      fileId: number;
      override: "reviewed" | "unreviewed" | null;
    }) => setFileReview(fileId, override),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      qc.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useSetBookReview = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      bookId,
      override,
    }: {
      bookId: number;
      override: "reviewed" | "unreviewed" | null;
    }) => setBookReview(bookId, override),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      qc.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useBulkSetReview = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      bookIds,
      override,
    }: {
      bookIds: number[];
      override: "reviewed" | "unreviewed" | null;
    }) => bulkSetReview(bookIds, override),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      qc.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};
```

- [ ] **Step 18.3: Verify types**

```bash
pnpm lint:types
```

Expected: no errors.

- [ ] **Step 18.4: Commit**

```bash
git add app/
git commit -m "[Frontend] Add review query hooks and API fetchers"
```

---

### Task 19: Reviewed-filter param in book list query

**Files:**
- Modify: `app/hooks/queries/books.ts` (the `useBooks` hook)
- Modify: `app/libraries/api.ts` (`getBooks`)

- [ ] **Step 19.1: Add the option**

In `app/libraries/api.ts`, extend the `GetBooksOptions` (or matching name) interface to include `reviewed_filter?: "all" | "needs_review" | "reviewed"`. Pass it through to the URL query string.

In `app/hooks/queries/books.ts`, accept the same option in the hook's input and pass it down.

- [ ] **Step 19.2: Verify**

```bash
pnpm lint:types
```

Expected: no errors.

- [ ] **Step 19.3: Commit**

```bash
git add app/
git commit -m "[Frontend] Plumb reviewed_filter through book list query"
```

---

## Phase 10 — Frontend UI

### Task 20: Review panel in `BookEditDialog`

**Files:**
- Modify: `app/components/library/BookEditDialog.tsx`

- [ ] **Step 20.1: Add the panel**

Add a new section inside the dialog body, near the top of the form (above existing fields):

```tsx
<ReviewPanel
  files={book.files ?? []}
  onChange={(override) =>
    setBookReviewMutation.mutate({ bookId: book.id, override })
  }
/>
```

Build the `ReviewPanel` component as a new sub-component file (`app/components/library/ReviewPanel.tsx`). It accepts `files` and `onChange`. It computes:

- Aggregate "Reviewed" boolean: `true` iff every main file has `reviewed === true`.
- Mixed state: aggregate is mixed if some main files are `true` and some are `false`/`null`.
- Indicator label: "Auto" / "Manually marked reviewed on {date}" / "Manually marked needs review on {date}" / "Mixed" — derive from `review_override` and `review_overridden_at` across the book's main files.
- Missing-fields hint: aggregated across files (use the spec format `Missing on EPUB: cover; Missing on M4B: narrators`). Use the per-file `reviewed === false` files; for each, derive missing fields by inspecting which required fields are empty (read criteria from `useReviewCriteria()`).

Render a `Switch` (shadcn) bound to the aggregate. When toggled, call `onChange("reviewed")` or `onChange("unreviewed")` based on the new state.

- [ ] **Step 20.2: Add a unit test**

`app/components/library/ReviewPanel.test.tsx` covers:

- All files reviewed → toggle is on, indicator says "Auto" or "Manually marked reviewed".
- Some files reviewed, some not → indicator says "Mixed".
- File missing fields → missing-fields list rendered.

- [ ] **Step 20.3: Run lint and tests**

```bash
pnpm lint:eslint && pnpm lint:types && pnpm test:unit -- ReviewPanel
```

Expected: PASS.

- [ ] **Step 20.4: Commit**

```bash
git add app/components/library/
git commit -m "[Frontend] Add review panel to BookEditDialog"
```

---

### Task 21: Review panel in `FileEditDialog`

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx`

- [ ] **Step 21.1: Add the panel**

Reuse `ReviewPanel` if practical; otherwise create `FileReviewPanel.tsx` for the per-file scope:

- Accepts a single `file` prop.
- Computes the indicator (Auto / Manually set on {date}).
- Renders a `Switch` bound to `file.reviewed === true` that calls `useSetFileReview()`.
- Shows missing fields specifically for that file.

- [ ] **Step 21.2: Add unit test**

Add a test asserting toggle off + on triggers the mutation with `"reviewed"` / `"unreviewed"`.

- [ ] **Step 21.3: Run checks**

```bash
pnpm lint:eslint && pnpm lint:types && pnpm test:unit -- FileReviewPanel
```

Expected: PASS.

- [ ] **Step 21.4: Commit**

```bash
git add app/components/library/
git commit -m "[Frontend] Add review panel to FileEditDialog"
```

---

### Task 22: "Needs review" badge on book cards

**Files:**
- Modify: `app/components/library/BookItem.tsx` (and/or wherever the cover card is composed in `Gallery`)

- [ ] **Step 22.1: Compute aggregate state**

Add a helper `isBookNeedsReview(book: Book): boolean` to `app/utilities/book.ts` (or similar):

```ts
export const isBookNeedsReview = (book: Book): boolean => {
  const mains = (book.files ?? []).filter((f) => f.file_role === "main");
  if (mains.length === 0) return false;
  return mains.some((f) => f.reviewed !== true);
};
```

- [ ] **Step 22.2: Render the badge**

In `BookItem.tsx`, when `isBookNeedsReview(book)` is true, render a small `Badge variant="secondary"` overlaid on the cover (bottom-right corner) saying "Needs review".

Use the existing badge styles. Cover positioning matches existing overlays (e.g., the file-count badge if one exists).

- [ ] **Step 22.3: Test**

Add a test in `BookItem.test.tsx` (or sibling) asserting the badge appears for a book with a `reviewed=false` file and is absent for an all-reviewed book.

- [ ] **Step 22.4: Commit**

```bash
git add app/
git commit -m "[Frontend] Add Needs review badge to book cards"
```

---

### Task 23: Filter sheet entry

**Files:**
- Modify: `app/components/library/FilterSheet.tsx`
- Modify: `app/components/library/ActiveFilterChips.tsx`

- [ ] **Step 23.1: Add the section**

In `FilterSheet.tsx`, add a new section "Review state" with three radio buttons:

```tsx
<section>
  <h3>Review state</h3>
  <RadioGroup value={reviewedFilter} onValueChange={setReviewedFilter}>
    <Radio value="all">All</Radio>
    <Radio value="needs_review">Needs review</Radio>
    <Radio value="reviewed">Reviewed</Radio>
  </RadioGroup>
</section>
```

`reviewedFilter` is plumbed through the same mechanism as existing filters (typically a context, query string, or prop).

- [ ] **Step 23.2: Add chip**

In `ActiveFilterChips.tsx`, render a chip for the active value when it's not "all". Closing the chip resets to "all".

- [ ] **Step 23.3: Wire filter into the gallery query**

In whichever component drives the gallery query (typically `Library.tsx` or similar), pass `reviewed_filter` to `useBooks(...)`.

- [ ] **Step 23.4: Run checks**

```bash
pnpm lint:eslint && pnpm lint:types && pnpm test:unit -- FilterSheet
```

Expected: PASS.

- [ ] **Step 23.5: Commit**

```bash
git add app/
git commit -m "[Frontend] Add review state filter to FilterSheet"
```

---

### Task 24: Bulk "More" overflow popover in `SelectionToolbar`

**Files:**
- Modify: `app/components/library/SelectionToolbar.tsx`

- [ ] **Step 24.1: Add the popover**

Add a new button next to the existing actions, before "Clear":

```tsx
<Popover>
  <PopoverTrigger asChild>
    <Button size="sm" variant="default">
      <MoreHorizontal className="h-4 w-4" />
      More
    </Button>
  </PopoverTrigger>
  <PopoverContent align="center" className="w-56 p-1" side="top">
    <button
      className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-accent text-left w-full text-sm cursor-pointer"
      onClick={() => bulkSetReviewMutation.mutate({ bookIds: selectedBookIds, override: "reviewed" })}
    >
      <CheckCircle className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span className="truncate">Mark reviewed</span>
    </button>
    <button
      className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-accent text-left w-full text-sm cursor-pointer"
      onClick={() => bulkSetReviewMutation.mutate({ bookIds: selectedBookIds, override: "unreviewed" })}
    >
      <Circle className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span className="truncate">Mark needs review</span>
    </button>
  </PopoverContent>
</Popover>
```

After mutation success, `exitSelectionMode()` and show a toast.

- [ ] **Step 24.2: Test**

Extend the existing toolbar test to assert the new actions appear and trigger the right mutation.

- [ ] **Step 24.3: Run checks**

```bash
pnpm lint:eslint && pnpm lint:types && pnpm test:unit -- SelectionToolbar
```

Expected: PASS.

- [ ] **Step 24.4: Commit**

```bash
git add app/components/library/
git commit -m "[Frontend] Add bulk review actions to selection toolbar"
```

---

### Task 25: Review Criteria section in `AdminSettings`

**Files:**
- Modify: `app/components/pages/AdminSettings.tsx`
- Create: `app/components/pages/ReviewCriteriaSection.tsx`

- [ ] **Step 25.1: Build the section component**

`ReviewCriteriaSection.tsx`:

- Loads `useReviewCriteria()`.
- Renders two checkbox groups: "Required for all books" (UniversalCandidates) and "Required for audiobooks (additional)" (AudioCandidates). Each checkbox is checked iff the field is in the active criteria.
- Tracks dirty state. Save button is disabled until changed.
- On Save: if `override_count > 0`, opens a `ConfirmDialog` showing the count: "Recompute review state? Auto-detected reviewed status will refresh based on the new criteria. You currently have **{override_count} reviewed-overrides set out of {main_file_count} total main files**." with a "Also clear manual overrides" checkbox. On confirm, calls `useUpdateReviewCriteria()` with `clear_overrides` from the checkbox.
- If `override_count === 0`, Save fires `useUpdateReviewCriteria({clear_overrides: false})` directly without the dialog.
- Show toast on success: "Review state recompute queued."
- Below the form, render a "Recompute review state now" button that queues a `recompute_review` job via the jobs API (POST `/api/jobs`). Same confirm dialog when overrides exist.

Use the project's existing `useUnsavedChanges` pattern (see `app/CLAUDE.md`) since this is an admin page with editable form state.

- [ ] **Step 25.2: Place in AdminSettings**

In `AdminSettings.tsx`, render `<ReviewCriteriaSection />` as a new top-level card section between existing cards.

- [ ] **Step 25.3: Test**

Add `ReviewCriteriaSection.test.tsx` covering:
- Default values render with the right checkboxes checked.
- Toggling a checkbox and saving fires the mutation with the new arrays.
- Save with `override_count > 0` opens the confirmation dialog.
- "Clear manual overrides" checkbox flows through to the mutation payload.

- [ ] **Step 25.4: Run checks**

```bash
pnpm lint:eslint && pnpm lint:types && pnpm test:unit -- ReviewCriteriaSection
```

Expected: PASS.

- [ ] **Step 25.5: Commit**

```bash
git add app/
git commit -m "[Frontend] Add Review Criteria section to AdminSettings"
```

---

### Task 26: Review panel placement in `BookDetail`

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

- [ ] **Step 26.1: Render `<ReviewPanel />`**

Render the same component used in `BookEditDialog` near the top of the page (below the actions row, above the description). Hidden when `book.files` is empty.

- [ ] **Step 26.2: Run checks**

```bash
pnpm lint:eslint && pnpm lint:types && pnpm test:unit
```

Expected: PASS.

- [ ] **Step 26.3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "[Frontend] Show review panel on BookDetail"
```

---

## Phase 11 — Documentation

### Task 27: Add review state docs page

**Files:**
- Create: `website/docs/review-state.md`
- Modify: `website/docs/metadata.md` (cross-link)
- Modify: `website/sidebars.ts` (or equivalent — add to nav)

- [ ] **Step 27.1: Write the page**

`website/docs/review-state.md`:

```markdown
---
sidebar_position: 9
---

# Review state

Each book and file in your library has a **Reviewed** state that helps you track which books still need your attention. The library gallery offers a "Needs review" filter so you can work through books one at a time without losing track.

## Auto-flip

Files automatically flip to **Reviewed** when they have all the metadata fields you've marked as required. The defaults are:

- **All books:** authors, description, cover, genres
- **Audiobooks (additional):** narrators

If a file is missing any of these, it stays in your "Needs review" queue.

When you fill in a missing field — through the edit dialog, a plugin enrichment, or the identify dialog — the flag flips automatically. If you delete a field, the file returns to the queue (unless you've manually marked it).

## Manual override

If you want to keep a book in the queue regardless of completeness, or sign off on a book that will never have certain fields, use the toggle on the book or file edit page. Manual choices are sticky in both directions and persist until you change them again.

## Filter and badge

The library gallery shows a small **Needs review** badge on books that have at least one main file outstanding. Open the **Filter** sheet to switch between "All", "Needs review", and "Reviewed".

## Bulk actions

When multi-selecting books in the gallery, use the **More** menu in the action bar to mark all selected books as reviewed (or needs review) at once.

## Configuring required fields

Admins can change the required-field set in **Server Settings → Review Criteria**. Saving updated criteria triggers a background recompute of every main file. If you have manual overrides, you'll see a prompt asking whether to clear them — pick what fits your situation.

## Cross-reference

- [Metadata](./metadata.md) — what each field means.
- [Plugins](./plugins/) — plugins fill in metadata that drives review state.
```

- [ ] **Step 27.2: Add cross-links**

In `website/docs/metadata.md`, add a "See also" link to `review-state.md` near the top.

- [ ] **Step 27.3: Add to sidebar**

In `website/sidebars.ts` (or the auto-loaded equivalent), insert `"review-state"` in the appropriate position.

- [ ] **Step 27.4: Build the docs site to verify**

```bash
mise docs &
# Open http://localhost:3000 and confirm the page renders and is linked
```

- [ ] **Step 27.5: Commit**

```bash
git add website/
git commit -m "[Docs] Add review-state docs page and cross-links"
```

---

## Phase 12 — End-to-end test

### Task 28: E2E spec

**Files:**
- Create: `e2e/review-flag.spec.ts`

- [ ] **Step 28.1: Write the test**

```ts
import { test, expect } from "./fixtures";

test.describe("Review flag", () => {
  test("book missing required fields shows up in Needs review filter and drops out when filled", async ({
    page,
    seedLibrary,
  }) => {
    // Seed a book with NO description, cover, genres, narrators
    const book = await seedLibrary.book({ title: "Incomplete", description: null });
    await page.goto("/library/" + seedLibrary.id);

    // Open filter sheet, select Needs review
    await page.getByRole("button", { name: /filter/i }).click();
    await page.getByLabel(/needs review/i).check();
    await page.getByRole("button", { name: /apply/i }).click();

    // Book should appear
    await expect(page.getByText("Incomplete")).toBeVisible();

    // Open the book, fill in required fields
    await page.getByText("Incomplete").click();
    await page.getByRole("button", { name: /edit/i }).click();
    await page.getByLabel("Description").fill("A complete description");
    // ...fill in genres, attach a cover, etc...
    await page.getByRole("button", { name: /save/i }).click();

    // Go back to the library, filter should no longer include this book
    await page.goto("/library/" + seedLibrary.id + "?reviewed_filter=needs_review");
    await expect(page.getByText("Incomplete")).not.toBeVisible();
  });

  test("manual mark needs review keeps complete book in queue", async ({
    page,
    seedLibrary,
  }) => {
    const book = await seedLibrary.book({
      title: "Complete",
      description: "ok",
      genres: ["Fiction"],
      coverPath: "fixtures/cover.jpg",
      authors: ["Author"],
    });
    await page.goto("/library/" + seedLibrary.id + "/books/" + book.id);
    // Toggle "Reviewed" off
    await page.getByLabel("Reviewed").click();
    // Filter should now show this book
    await page.goto("/library/" + seedLibrary.id + "?reviewed_filter=needs_review");
    await expect(page.getByText("Complete")).toBeVisible();
  });

  test("bulk mark reviewed cascades to all selected books", async ({
    page,
    seedLibrary,
  }) => {
    await seedLibrary.book({ title: "Bulk1" });
    await seedLibrary.book({ title: "Bulk2" });
    await page.goto("/library/" + seedLibrary.id + "?reviewed_filter=needs_review");

    await page.getByText("Bulk1").click({ modifiers: ["Shift"] });
    await page.getByText("Bulk2").click({ modifiers: ["Shift"] });
    await page.getByRole("button", { name: /more/i }).click();
    await page.getByRole("menuitem", { name: /mark reviewed/i }).click();

    await page.goto("/library/" + seedLibrary.id + "?reviewed_filter=needs_review");
    await expect(page.getByText("Bulk1")).not.toBeVisible();
    await expect(page.getByText("Bulk2")).not.toBeVisible();
  });
});
```

(Replace `seedLibrary.book` etc. with the actual fixture API used in `e2e/`. Look at neighboring specs for the right shape.)

- [ ] **Step 28.2: Run E2E**

```bash
mise test:e2e
```

Expected: PASS.

- [ ] **Step 28.3: Commit**

```bash
git add e2e/
git commit -m "[E2E] Add review-flag end-to-end test"
```

---

## Phase 13 — Final integration check

### Task 29: Full check & manual smoke test

- [ ] **Step 29.1: Run full check suite**

```bash
mise check:quiet
```

Expected: PASS.

- [ ] **Step 29.2: Manual smoke (dev server)**

```bash
mise start
```

Walk through:

1. Drop a known-incomplete book into the library path. Confirm it appears with "Needs review" badge.
2. Filter to "Needs review" — book is visible. Filter to "Reviewed" — book is not visible.
3. Open book detail, fill in missing fields, save. Refresh. Badge gone, book moves out of needs-review filter.
4. Toggle "Reviewed" off manually. Book reappears in needs-review.
5. Multi-select two books. Use the "More" → "Mark reviewed". Both drop out.
6. Open Server Settings → Review Criteria. Toggle "Tags" on. Save → confirm dialog if overrides exist. Confirm. Verify some books with no tags now appear in needs-review.
7. Click "Recompute review state now". Watch the job in the jobs page.

- [ ] **Step 29.3: No commit needed (verification only)**

The plan is complete.

---

## Summary

This plan adds a per-file `Reviewed` state with admin-configurable required fields, sticky manual overrides, automatic recompute on metadata changes, and the corresponding UI surfaces (filter, badge, panels, settings page, bulk action). It introduces a new `app_settings` runtime-mutable settings table, a new `recompute_review` background job, and a `pkg/books/review` package that owns the completeness rule. Migrations are designed to populate the new state via the existing job system rather than synchronously, keeping deploys fast.
