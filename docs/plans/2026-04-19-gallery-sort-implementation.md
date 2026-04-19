# Gallery Sort Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Multi-level sort for the library gallery — URL-addressable (`?sort=…`), with per-(user × library) saved defaults stored in a new `user_library_settings` table and applied consistently across the gallery API, OPDS, and eReader surfaces.

**Architecture:** A new `pkg/sortspec` package owns sort grammar, validation, SQL expression building, and a preference-resolver that every book-list handler calls. A generic `user_library_settings` table parallels `user_settings`. Frontend mirrors existing `FilterSheet` / `ActiveFilterChips` components with `@dnd-kit/sortable` for drag-reorder.

**Tech Stack:** Go (Echo, Bun, SQLite with FTS5), React 19 + TypeScript, TanStack Query, `@dnd-kit/sortable`, TailwindCSS, Playwright E2E.

**Spec:** [`docs/plans/2026-04-19-gallery-sort-design.md`](./2026-04-19-gallery-sort-design.md)

---

## File Structure

### New files (Go)

- `pkg/migrations/20260419000000_add_user_library_settings.go` — creates table + unique index.
- `pkg/models/user_library_settings.go` — Bun model.
- `pkg/sortspec/spec.go` — `SortLevel`, `Direction`, `Parse`, `Serialize`.
- `pkg/sortspec/whitelist.go` — canonical field constants + `IsValidField`.
- `pkg/sortspec/sql.go` — `OrderClauses` (SQL expression builder).
- `pkg/sortspec/resolve.go` — `LibrarySettingsReader` interface + `ResolveForLibrary`.
- `pkg/sortspec/spec_test.go`, `whitelist_test.go`, `sql_test.go`, `resolve_test.go` — unit tests.
- `pkg/settings/library_service.go` — `GetLibrarySettings`, `UpsertLibrarySort`. (Split from `service.go` to keep files focused.)
- `pkg/settings/library_handlers.go` — `getLibrarySettings`, `updateLibrarySettings`.
- `pkg/settings/library_service_test.go`, `library_handlers_test.go` — tests.
- `pkg/books/handlers_list_test.go` — handler test for sort routing.

### Modified files (Go)

- `pkg/settings/routes.go` — register 2 new routes, wire library handlers.
- `pkg/settings/service.go` — no code change, but `Service` gains methods via the new `library_service.go` file in the same package.
- `pkg/settings/validators.go` — add `UpdateLibrarySettingsPayload`, `LibrarySettingsResponse`.
- `pkg/books/validators.go` — add `Sort` field to `ListBooksQuery`.
- `pkg/books/service.go` — add `Sort` to `ListBooksOptions`, apply in `listBooksWithTotal`.
- `pkg/books/handlers.go` — `list` handler parses sort, calls `ResolveForLibrary`.
- `pkg/books/routes.go` — inject settings service into handler struct.
- `pkg/opds/service.go` — add `sort []sortspec.SortLevel` parameter to 7 feed builders.
- `pkg/opds/handlers.go` — each of 7 feed handlers resolves user and preference before calling service.
- `pkg/opds/routes.go` — inject settings service into handler struct.
- `pkg/ereader/handlers.go` — 4 handlers resolve preference using `apiKey.UserID`.
- `pkg/ereader/routes.go` — inject settings service into handler.
- `pkg/server/server.go` — pass settings service into `opds.RegisterRoutes` and `ereader.RegisterRoutes` (or construct the service once and share).

### New files (Frontend)

- `app/lib/sortSpec.ts` — TS parser, whitelist, builtin default.
- `app/lib/sortSpec.test.ts` — unit tests.
- `app/hooks/queries/librarySettings.ts` — TanStack Query hooks.
- `app/components/library/SortSheet.tsx` — multi-level sort sheet/drawer.
- `app/components/library/SortSheet.test.tsx` — interaction tests.
- `app/components/library/SortedByChips.tsx` — the "Sorted by" chip row.
- `app/components/library/SortedByChips.test.tsx` — interaction tests.
- `e2e/gallery-sort.spec.ts` — Playwright E2E.

### Modified files (Frontend)

- `app/components/pages/Home.tsx` — add sort state, Sort button, chip row, block on library-settings load.

---

## Phase 1: Foundation (migration + model)

### Task 1: Database migration

**Files:**
- Create: `pkg/migrations/20260419000000_add_user_library_settings.go`

- [ ] **Step 1: Write the migration file**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE user_library_settings (
				id          INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				user_id     INTEGER NOT NULL REFERENCES users     (id) ON DELETE CASCADE,
				library_id  INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
				sort_spec   TEXT
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE UNIQUE INDEX ux_user_library_settings ON user_library_settings (user_id, library_id)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP INDEX IF EXISTS ux_user_library_settings")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS user_library_settings")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 2: Verify migration runs cleanly**

Run: `mise db:migrate`
Expected: completes without error, no migrations pending after.

- [ ] **Step 3: Verify rollback**

Run: `mise db:rollback && mise db:migrate`
Expected: both succeed; table is recreated identically.

- [ ] **Step 4: Commit**

```bash
git add pkg/migrations/20260419000000_add_user_library_settings.go
git commit -m "[Backend] Add user_library_settings migration"
```

---

### Task 2: Bun model

**Files:**
- Create: `pkg/models/user_library_settings.go`

- [ ] **Step 1: Write the model**

```go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

type UserLibrarySettings struct {
	bun.BaseModel `bun:"table:user_library_settings,alias:uls"`

	ID        int       `bun:",pk,autoincrement"                            json:"id"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
	UserID    int       `bun:",notnull"                                     json:"user_id"`
	LibraryID int       `bun:",notnull"                                     json:"library_id"`
	SortSpec  *string   `bun:",nullzero"                                    json:"sort_spec"`
}
```

- [ ] **Step 2: Build to verify**

Run: `go build ./pkg/models/...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/models/user_library_settings.go
git commit -m "[Backend] Add UserLibrarySettings model"
```

---

## Phase 2: sortspec package

All four subtasks live in a new `pkg/sortspec/` package. The package has no runtime dependencies on other domain packages except `pkg/models` (for the `UserLibrarySettings` pointer in the resolver).

### Task 3: SortLevel, Direction, parser

**Files:**
- Create: `pkg/sortspec/spec.go`
- Create: `pkg/sortspec/spec_test.go`

- [ ] **Step 1: Write failing tests for Parse**

```go
package sortspec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []SortLevel
	}{
		{
			name:  "single level asc",
			input: "title:asc",
			expected: []SortLevel{
				{Field: FieldTitle, Direction: DirAsc},
			},
		},
		{
			name:  "single level desc",
			input: "date_added:desc",
			expected: []SortLevel{
				{Field: FieldDateAdded, Direction: DirDesc},
			},
		},
		{
			name:  "multi level",
			input: "author:asc,series:asc,title:asc",
			expected: []SortLevel{
				{Field: FieldAuthor, Direction: DirAsc},
				{Field: FieldSeries, Direction: DirAsc},
				{Field: FieldTitle, Direction: DirAsc},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParse_Invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"missing direction", "title"},
		{"unknown field", "bogus:asc"},
		{"bad direction", "title:sideways"},
		{"duplicate field", "title:asc,title:desc"},
		{"trailing comma", "title:asc,"},
		{"leading comma", ",title:asc"},
		{"empty pair", "title:asc,,series:asc"},
		{"whitespace around pair", " title:asc "},
		{"too many levels", "title:asc,author:asc,series:asc,date_added:asc,date_released:asc,page_count:asc,duration:asc,title:desc,author:desc,series:desc,date_added:desc"}, // 11 levels, 10 is the max
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestSerialize(t *testing.T) {
	t.Parallel()

	levels := []SortLevel{
		{Field: FieldAuthor, Direction: DirAsc},
		{Field: FieldSeries, Direction: DirAsc},
	}
	assert.Equal(t, "author:asc,series:asc", Serialize(levels))
	assert.Equal(t, "", Serialize(nil))
}

func TestParseSerialize_RoundTrip(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"title:asc",
		"date_added:desc",
		"author:asc,series:asc,title:asc",
		"page_count:desc,duration:asc",
	}

	for _, input := range inputs {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			parsed, err := Parse(input)
			require.NoError(t, err)
			assert.Equal(t, input, Serialize(parsed))
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sortspec/... -run TestParse -v`
Expected: FAIL with "undefined: Parse" or similar.

- [ ] **Step 3: Write minimal implementation**

`pkg/sortspec/spec.go`:

```go
// Package sortspec parses, validates, builds SQL for, and resolves
// multi-level book sort specifications (e.g. "author:asc,series:asc").
package sortspec

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

// Direction is "asc" or "desc".
type Direction string

const (
	DirAsc  Direction = "asc"
	DirDesc Direction = "desc"
)

// MaxLevels is the hard cap on how many levels a spec may contain.
const MaxLevels = 10

// SortLevel is one field+direction pair in a spec.
type SortLevel struct {
	Field     string
	Direction Direction
}

// Parse reads a serialized spec string (e.g. "author:asc,series:desc") into
// a slice of SortLevel. It rejects unknown fields, bad directions, duplicates,
// empty pairs, stray whitespace, and specs longer than MaxLevels.
func Parse(s string) ([]SortLevel, error) {
	if s == "" {
		return nil, errors.New("sort spec is empty")
	}
	// Whitespace is not allowed anywhere — this is a machine-readable URL
	// param, not human prose. Rejecting early keeps the grammar strict.
	if strings.ContainsAny(s, " \t\n\r") {
		return nil, errors.New("sort spec must not contain whitespace")
	}

	parts := strings.Split(s, ",")
	if len(parts) > MaxLevels {
		return nil, errors.Errorf("sort spec has %d levels, max is %d", len(parts), MaxLevels)
	}

	seen := make(map[string]struct{}, len(parts))
	levels := make([]SortLevel, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			return nil, errors.New("sort spec contains an empty pair")
		}

		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			return nil, errors.Errorf("sort level %q missing direction", part)
		}

		field, dir := kv[0], kv[1]
		if !IsValidField(field) {
			return nil, errors.Errorf("unknown sort field %q", field)
		}
		if dir != string(DirAsc) && dir != string(DirDesc) {
			return nil, errors.Errorf("invalid direction %q (want asc or desc)", dir)
		}
		if _, dup := seen[field]; dup {
			return nil, errors.Errorf("duplicate sort field %q", field)
		}
		seen[field] = struct{}{}

		levels = append(levels, SortLevel{Field: field, Direction: Direction(dir)})
	}

	return levels, nil
}

// Serialize renders a level slice back into the URL-param form.
// The zero/nil slice serializes to the empty string.
func Serialize(levels []SortLevel) string {
	if len(levels) == 0 {
		return ""
	}
	parts := make([]string, len(levels))
	for i, l := range levels {
		parts[i] = fmt.Sprintf("%s:%s", l.Field, l.Direction)
	}
	return strings.Join(parts, ",")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sortspec/... -run "TestParse|TestSerialize" -v`
Expected: All PASS. (Note: `TestParse_Invalid`'s "unknown field" case depends on `IsValidField` from Task 4 — if that's not yet written, temporarily stub `IsValidField` to `return field == "title" || field == "author" || ...` inline or defer running that subtest. Cleaner approach: do Task 4 in the same commit.)

- [ ] **Step 5: Commit (deferred — combine with Task 4)**

Defer commit until Task 4 is written so the package ships coherently. Move on to Task 4 now.

---

### Task 4: Whitelist + field metadata

**Files:**
- Create: `pkg/sortspec/whitelist.go`
- Create: `pkg/sortspec/whitelist_test.go`

- [ ] **Step 1: Write failing tests**

```go
package sortspec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidField(t *testing.T) {
	t.Parallel()

	valid := []string{
		FieldTitle, FieldAuthor, FieldSeries,
		FieldDateAdded, FieldDateReleased,
		FieldPageCount, FieldDuration,
	}
	for _, f := range valid {
		f := f
		t.Run("valid/"+f, func(t *testing.T) {
			t.Parallel()
			assert.True(t, IsValidField(f))
		})
	}

	invalid := []string{"", "bogus", "TITLE", "date_published", "Title"}
	for _, f := range invalid {
		f := f
		t.Run("invalid/"+f, func(t *testing.T) {
			t.Parallel()
			assert.False(t, IsValidField(f))
		})
	}
}

func TestAllFields_Stable(t *testing.T) {
	t.Parallel()
	// AllFields is documented/consumed by the frontend via tygo; if this
	// list changes, the TS whitelist must be updated in lockstep. This
	// test just pins the expected order so additions are explicit.
	expected := []string{
		"title", "author", "series",
		"date_added", "date_released",
		"page_count", "duration",
	}
	assert.Equal(t, expected, AllFields())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sortspec/ -run "TestIsValidField|TestAllFields" -v`
Expected: FAIL with "undefined: FieldTitle" / "undefined: IsValidField".

- [ ] **Step 3: Write implementation**

`pkg/sortspec/whitelist.go`:

```go
package sortspec

// Canonical sort field tokens accepted by Parse.
//
// When adding a new field here, you MUST also:
//   - Update AllFields() below (order matters — it's pinned in tests).
//   - Add an SQL case to OrderClauses in sql.go.
//   - Update the TS whitelist in app/lib/sortSpec.ts (it mirrors this file).
//   - Document semantics in docs/plans/2026-04-19-gallery-sort-design.md.
const (
	FieldTitle        = "title"
	FieldAuthor       = "author"
	FieldSeries       = "series"
	FieldDateAdded    = "date_added"
	FieldDateReleased = "date_released"
	FieldPageCount    = "page_count"
	FieldDuration     = "duration"
)

// AllFields returns the canonical field list in UI display order.
func AllFields() []string {
	return []string{
		FieldTitle,
		FieldAuthor,
		FieldSeries,
		FieldDateAdded,
		FieldDateReleased,
		FieldPageCount,
		FieldDuration,
	}
}

// IsValidField returns true if s is a whitelisted sort field token.
func IsValidField(s string) bool {
	switch s {
	case FieldTitle, FieldAuthor, FieldSeries,
		FieldDateAdded, FieldDateReleased,
		FieldPageCount, FieldDuration:
		return true
	}
	return false
}
```

- [ ] **Step 4: Run all sortspec tests**

Run: `go test ./pkg/sortspec/... -v`
Expected: All PASS (Task 3's Parse tests now link against the real `IsValidField`).

- [ ] **Step 5: Commit**

```bash
git add pkg/sortspec/spec.go pkg/sortspec/spec_test.go pkg/sortspec/whitelist.go pkg/sortspec/whitelist_test.go
git commit -m "[Backend] Add pkg/sortspec parser and whitelist"
```

---

### Task 5: SQL expression builder

**Files:**
- Create: `pkg/sortspec/sql.go`
- Create: `pkg/sortspec/sql_test.go`

- [ ] **Step 1: Write failing tests**

```go
package sortspec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderClauses_SingleLevel(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldTitle, Direction: DirAsc},
	})

	// Title maps directly to b.sort_title. NULLS LAST is emulated with a
	// leading "IS NULL" term.
	assert.Equal(t, 1, len(got))
	assert.Equal(t, "b.sort_title IS NULL, b.sort_title ASC", got[0].Expression)
	assert.Nil(t, got[0].Args)
}

func TestOrderClauses_DescDirection(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldDateAdded, Direction: DirDesc},
	})
	assert.Equal(t, "b.created_at IS NULL, b.created_at DESC", got[0].Expression)
}

func TestOrderClauses_SeriesExpandsToTwo(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldSeries, Direction: DirDesc},
	})

	// Series expands to (series name, then series number). The number
	// clause is always ASC regardless of the user's chosen direction —
	// "Stormlight #1 before #2" is not a user preference.
	assert.Equal(t, 2, len(got))
	assert.Contains(t, got[0].Expression, "series") // sort name expression
	assert.Contains(t, got[0].Expression, "DESC")
	assert.Contains(t, got[1].Expression, "series_number")
	assert.Contains(t, got[1].Expression, "ASC")
}

func TestOrderClauses_PrimaryFileFallback(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldPageCount, Direction: DirAsc},
	})

	// page_count uses the COALESCE(primary file, any file with value)
	// pattern. The generated SQL must reference b.primary_file_id and
	// b.id as correlated subquery columns.
	assert.Equal(t, 1, len(got))
	assert.Contains(t, got[0].Expression, "COALESCE")
	assert.Contains(t, got[0].Expression, "b.primary_file_id")
	assert.Contains(t, got[0].Expression, "b.id")
	assert.Contains(t, got[0].Expression, "page_count")
	assert.Contains(t, got[0].Expression, "ASC")
}

func TestOrderClauses_MultiLevel(t *testing.T) {
	t.Parallel()

	got := OrderClauses([]SortLevel{
		{Field: FieldAuthor, Direction: DirAsc},
		{Field: FieldTitle, Direction: DirAsc},
	})
	assert.Equal(t, 2, len(got))
	assert.Contains(t, got[0].Expression, "sort_name") // author
	assert.Contains(t, got[1].Expression, "sort_title")
}

func TestOrderClauses_Empty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, OrderClauses(nil))
	assert.Empty(t, OrderClauses([]SortLevel{}))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sortspec/ -run TestOrderClauses -v`
Expected: FAIL — `undefined: OrderClauses`.

- [ ] **Step 3: Write implementation**

`pkg/sortspec/sql.go`:

```go
package sortspec

import (
	"fmt"
	"strings"
)

// OrderClause is one unit of ordering for a Bun query, ready to pass
// to q.OrderExpr(clause.Expression, clause.Args...).
//
// Each user-visible sort level produces ONE OrderClause, except the
// series level which expands to two (series sort name, then series
// number ASC).
type OrderClause struct {
	Expression string
	Args       []any
}

// OrderClauses maps a parsed sort spec to the SQL ORDER BY clauses
// that implement it on the `books` table (aliased `b` per pkg/CLAUDE.md).
//
// Every clause includes a NULLS-LAST indicator (`<expr> IS NULL`) so
// books missing the sort key always sit at the end regardless of
// direction. SQLite has no native NULLS LAST.
func OrderClauses(levels []SortLevel) []OrderClause {
	if len(levels) == 0 {
		return nil
	}

	out := make([]OrderClause, 0, len(levels)+1) // +1 for the series expansion
	for _, l := range levels {
		switch l.Field {
		case FieldTitle:
			out = append(out, nullsLast("b.sort_title", l.Direction))

		case FieldAuthor:
			// Primary author = authors row for this book with lowest
			// sort_order, tie-broken by authors.id ASC. Books with zero
			// authors sort last via NULLS LAST.
			expr := `(SELECT p.sort_name
                      FROM authors a
                      JOIN persons p ON p.id = a.person_id
                      WHERE a.book_id = b.id
                      ORDER BY a.sort_order ASC, a.id ASC
                      LIMIT 1)`
			out = append(out, nullsLast(expr, l.Direction))

		case FieldSeries:
			// Primary series sort name, then series number (always ASC
			// within a series). Books with no series row sort last.
			nameExpr := `(SELECT s.sort_name
                          FROM book_series bs
                          JOIN series s ON s.id = bs.series_id
                          WHERE bs.book_id = b.id
                          ORDER BY bs.series_number ASC, bs.id ASC
                          LIMIT 1)`
			numExpr := `(SELECT bs.series_number
                         FROM book_series bs
                         WHERE bs.book_id = b.id
                         ORDER BY bs.series_number ASC, bs.id ASC
                         LIMIT 1)`
			out = append(out,
				nullsLast(nameExpr, l.Direction),
				nullsLast(numExpr, DirAsc),
			)

		case FieldDateAdded:
			out = append(out, nullsLast("b.created_at", l.Direction))

		case FieldDateReleased:
			out = append(out, nullsLast(primaryFileCoalesce("release_date"), l.Direction))

		case FieldPageCount:
			out = append(out, nullsLast(primaryFileCoalesce("page_count"), l.Direction))

		case FieldDuration:
			out = append(out, nullsLast(primaryFileCoalesce("audiobook_duration_seconds"), l.Direction))
		}
	}
	return out
}

// nullsLast wraps a column/expression with both the NULLS-LAST
// indicator and the chosen direction, producing a single ORDER BY
// fragment like "<expr> IS NULL, <expr> ASC".
//
// The expression is embedded verbatim — callers must ensure it is
// safe (it always is in this package because expressions come from
// whitelisted field branches above).
func nullsLast(expr string, dir Direction) OrderClause {
	return OrderClause{
		Expression: fmt.Sprintf("%s IS NULL, %s %s", expr, expr, strings.ToUpper(string(dir))),
	}
}

// primaryFileCoalesce returns an SQL snippet that reads `field` from
// the book's primary file, falling back to any file on the book with
// a non-NULL value for that field.
//
//   COALESCE(
//     (SELECT f.<field> FROM files f WHERE f.id = b.primary_file_id),
//     (SELECT f.<field> FROM files f WHERE f.book_id = b.id AND f.<field> IS NOT NULL ORDER BY f.id LIMIT 1)
//   )
func primaryFileCoalesce(field string) string {
	return fmt.Sprintf(
		`COALESCE(
            (SELECT f.%[1]s FROM files f WHERE f.id = b.primary_file_id),
            (SELECT f.%[1]s FROM files f WHERE f.book_id = b.id AND f.%[1]s IS NOT NULL ORDER BY f.id LIMIT 1)
        )`, field)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sortspec/ -run TestOrderClauses -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sortspec/sql.go pkg/sortspec/sql_test.go
git commit -m "[Backend] Add pkg/sortspec SQL expression builder"
```

---

### Task 6: Preference resolver

**Files:**
- Create: `pkg/sortspec/resolve.go`
- Create: `pkg/sortspec/resolve_test.go`

- [ ] **Step 1: Write failing tests**

```go
package sortspec

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
)

// fakeReader is a test double for LibrarySettingsReader.
type fakeReader struct {
	settings *models.UserLibrarySettings
	err      error
}

func (f *fakeReader) GetLibrarySettings(_ context.Context, _ int, _ int) (*models.UserLibrarySettings, error) {
	return f.settings, f.err
}

func TestResolveForLibrary_ExplicitWins(t *testing.T) {
	t.Parallel()

	storedSpec := "title:asc"
	reader := &fakeReader{
		settings: &models.UserLibrarySettings{SortSpec: &storedSpec},
	}

	explicit := []SortLevel{{Field: FieldDateAdded, Direction: DirDesc}}
	got := ResolveForLibrary(context.Background(), reader, 1, 2, explicit)

	assert.Equal(t, explicit, got)
}

func TestResolveForLibrary_StoredUsedWhenNoExplicit(t *testing.T) {
	t.Parallel()

	storedSpec := "author:asc,series:asc"
	reader := &fakeReader{
		settings: &models.UserLibrarySettings{SortSpec: &storedSpec},
	}

	got := ResolveForLibrary(context.Background(), reader, 1, 2, nil)

	assert.Equal(t, []SortLevel{
		{Field: FieldAuthor, Direction: DirAsc},
		{Field: FieldSeries, Direction: DirAsc},
	}, got)
}

func TestResolveForLibrary_ReturnsNilWhenNoRow(t *testing.T) {
	t.Parallel()

	// sql.ErrNoRows or nil settings both mean "no preference saved".
	reader := &fakeReader{settings: nil, err: sql.ErrNoRows}
	got := ResolveForLibrary(context.Background(), reader, 1, 2, nil)

	assert.Nil(t, got)
}

func TestResolveForLibrary_ReturnsNilWhenSortSpecNull(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{
		settings: &models.UserLibrarySettings{SortSpec: nil},
	}
	got := ResolveForLibrary(context.Background(), reader, 1, 2, nil)

	assert.Nil(t, got)
}

func TestResolveForLibrary_InvalidStoredSpecFallsThrough(t *testing.T) {
	t.Parallel()

	// A stored spec that fails to parse (e.g. whitelist drift between
	// releases) should not crash — return nil and let the caller use
	// its hard-coded default.
	bad := "garbage_field:asc"
	reader := &fakeReader{
		settings: &models.UserLibrarySettings{SortSpec: &bad},
	}
	got := ResolveForLibrary(context.Background(), reader, 1, 2, nil)

	assert.Nil(t, got)
}

func TestResolveForLibrary_ReaderErrorFallsThrough(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{err: assert.AnError}
	got := ResolveForLibrary(context.Background(), reader, 1, 2, nil)

	// Unexpected DB errors are swallowed; sort is best-effort.
	assert.Nil(t, got)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sortspec/ -run TestResolveForLibrary -v`
Expected: FAIL — `undefined: ResolveForLibrary`, `undefined: LibrarySettingsReader`.

- [ ] **Step 3: Write implementation**

`pkg/sortspec/resolve.go`:

```go
package sortspec

import (
	"context"

	"github.com/shishobooks/shisho/pkg/models"
)

// LibrarySettingsReader is the narrow read interface this package
// needs from the settings service. Declared here (not in pkg/settings)
// so ResolveForLibrary can be called without importing settings —
// avoiding an import cycle because pkg/settings imports pkg/sortspec
// to validate sort_spec at write time.
type LibrarySettingsReader interface {
	GetLibrarySettings(ctx context.Context, userID, libraryID int) (*models.UserLibrarySettings, error)
}

// ResolveForLibrary picks the sort levels to apply for a given caller.
//
// Priority:
//  1. explicit — if non-empty (caller passed an explicit URL param),
//     it wins.
//  2. stored — look up user_library_settings for (userID, libraryID);
//     if a row exists with a parseable sort_spec, use it.
//  3. nil — caller should fall back to whatever hard-coded default
//     it was using before this feature shipped.
//
// Errors from the reader are swallowed: sort is a non-critical UX
// affordance and should never fail a request. An invalid stored spec
// is treated the same as no spec (returns nil).
func ResolveForLibrary(
	ctx context.Context,
	reader LibrarySettingsReader,
	userID, libraryID int,
	explicit []SortLevel,
) []SortLevel {
	if len(explicit) > 0 {
		return explicit
	}

	settings, err := reader.GetLibrarySettings(ctx, userID, libraryID)
	if err != nil || settings == nil || settings.SortSpec == nil {
		return nil
	}

	levels, err := Parse(*settings.SortSpec)
	if err != nil {
		return nil
	}
	return levels
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sortspec/... -v`
Expected: All tests in the package PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/sortspec/resolve.go pkg/sortspec/resolve_test.go
git commit -m "[Backend] Add pkg/sortspec ResolveForLibrary"
```

---

## Phase 3: Settings extensions

### Task 7: Settings validators for library settings

**Files:**
- Modify: `pkg/settings/validators.go`

- [ ] **Step 1: Append the new payload/response types**

Open `pkg/settings/validators.go` and append at the bottom of the file:

```go
// UpdateLibrarySettingsPayload is the request body for PUT /settings/libraries/:library_id.
//
// SortSpec is a pointer so the client can distinguish "unset" (omit field
// from JSON) from "clear the saved default" (send null). A null body
// clears the saved sort; omitting the field leaves it untouched.
type UpdateLibrarySettingsPayload struct {
	SortSpec *string `json:"sort_spec"`
}

// LibrarySettingsResponse is the response for GET/PUT /settings/libraries/:library_id.
type LibrarySettingsResponse struct {
	SortSpec *string `json:"sort_spec"`
}
```

- [ ] **Step 2: Build to verify**

Run: `go build ./pkg/settings/...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/settings/validators.go
git commit -m "[Backend] Add library settings request/response types"
```

---

### Task 8: Settings service — library methods

**Files:**
- Create: `pkg/settings/library_service.go`
- Create: `pkg/settings/library_service_test.go`

- [ ] **Step 1: Write failing tests**

```go
package settings

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() { db.Close() })
	return db
}

func createTestUser(t *testing.T, db *bun.DB, username string) *models.User {
	t.Helper()
	u := &models.User{
		Username:     username,
		PasswordHash: "x",
		RoleID:       1,
		IsActive:     true,
	}
	_, err := db.NewInsert().Model(u).Exec(context.Background())
	require.NoError(t, err)
	return u
}

func createTestLibrary(t *testing.T, db *bun.DB, name string) *models.Library {
	t.Helper()
	l := &models.Library{
		Name:                     name,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(l).Exec(context.Background())
	require.NoError(t, err)
	return l
}

func TestGetLibrarySettings_ReturnsNilWhenMissing(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")

	got, err := svc.GetLibrarySettings(context.Background(), user.ID, lib.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestUpsertLibrarySort_InsertsThenUpdates(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")

	spec := "title:asc"
	row, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &spec)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, user.ID, row.UserID)
	assert.Equal(t, lib.ID, row.LibraryID)
	require.NotNil(t, row.SortSpec)
	assert.Equal(t, "title:asc", *row.SortSpec)

	// Update — same (user, library), new spec. Should overwrite, not duplicate.
	newSpec := "author:asc,series:asc"
	row2, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &newSpec)
	require.NoError(t, err)
	assert.Equal(t, "author:asc,series:asc", *row2.SortSpec)

	// Only one row should exist for this pair.
	var count int
	count, err = db.NewSelect().
		Model((*models.UserLibrarySettings)(nil)).
		Where("user_id = ? AND library_id = ?", user.ID, lib.ID).
		Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestUpsertLibrarySort_ClearsWithNil(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")

	spec := "title:asc"
	_, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &spec)
	require.NoError(t, err)

	row, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, row.SortSpec)
}

func TestUserDelete_CascadesSettings(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")

	spec := "title:asc"
	_, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &spec)
	require.NoError(t, err)

	_, err = db.NewDelete().Model((*models.User)(nil)).Where("id = ?", user.ID).Exec(context.Background())
	require.NoError(t, err)

	var count int
	count, err = db.NewSelect().
		Model((*models.UserLibrarySettings)(nil)).
		Where("user_id = ?", user.ID).
		Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, count, "settings should cascade on user delete")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/settings/ -run "TestGetLibrarySettings|TestUpsertLibrarySort|TestUserDelete" -v`
Expected: FAIL — `svc.GetLibrarySettings undefined`, `svc.UpsertLibrarySort undefined`.

- [ ] **Step 3: Write implementation**

`pkg/settings/library_service.go`:

```go
package settings

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
)

// GetLibrarySettings returns the (user, library) settings row, or nil
// when no row exists. The nil-return form (rather than a zero-valued
// struct) makes callers' "no preference" checks explicit.
func (svc *Service) GetLibrarySettings(ctx context.Context, userID, libraryID int) (*models.UserLibrarySettings, error) {
	row := &models.UserLibrarySettings{}
	err := svc.db.NewSelect().
		Model(row).
		Where("user_id = ? AND library_id = ?", userID, libraryID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return row, nil
}

// UpsertLibrarySort writes just the sort_spec column for (userID,
// libraryID). sortSpec may be nil to clear the saved default. Other
// columns on the row (when they exist in a future version) are left
// untouched by the ON CONFLICT update.
func (svc *Service) UpsertLibrarySort(ctx context.Context, userID, libraryID int, sortSpec *string) (*models.UserLibrarySettings, error) {
	now := time.Now()

	row := &models.UserLibrarySettings{
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    userID,
		LibraryID: libraryID,
		SortSpec:  sortSpec,
	}

	_, err := svc.db.NewInsert().
		Model(row).
		On("CONFLICT (user_id, library_id) DO UPDATE").
		Set("updated_at = EXCLUDED.updated_at").
		Set("sort_spec = EXCLUDED.sort_spec").
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return row, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/settings/ -run "TestGetLibrarySettings|TestUpsertLibrarySort|TestUserDelete" -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/settings/library_service.go pkg/settings/library_service_test.go
git commit -m "[Backend] Add settings service library methods"
```

---

### Task 9: Settings handlers + routes for library settings

**Files:**
- Create: `pkg/settings/library_handlers.go`
- Create: `pkg/settings/library_handlers_test.go`
- Modify: `pkg/settings/routes.go`

- [ ] **Step 1: Write failing tests**

```go
package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func newTestEcho() *echo.Echo {
	e := echo.New()
	e.Binder = binder.New()
	return e
}

// buildGetRequest builds an Echo context for GET /settings/libraries/:library_id.
func buildGetRequest(t *testing.T, e *echo.Echo, user *models.User, libraryID int) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/settings/libraries/"+strconv.Itoa(libraryID), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("library_id")
	c.SetParamValues(strconv.Itoa(libraryID))
	c.Set("user", user)
	return c, rec
}

func buildPutRequest(t *testing.T, e *echo.Echo, user *models.User, libraryID int, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/settings/libraries/"+strconv.Itoa(libraryID), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("library_id")
	c.SetParamValues(strconv.Itoa(libraryID))
	c.Set("user", user)
	return c, rec
}

func seedLibraryAccess(t *testing.T, db *bun.DB, user *models.User, libraryID int) {
	t.Helper()
	// Grant access by giving user a role that can access all libraries,
	// OR by inserting into user_libraries. Simplest: set user.RoleID to
	// admin if one exists in the seeded schema; if not, insert the
	// user_libraries row explicitly.
	_, err := db.NewInsert().Model(&models.UserLibrary{
		UserID:    user.ID,
		LibraryID: libraryID,
	}).Exec(context.Background())
	require.NoError(t, err)
}

func TestGetLibrarySettings_NoRow(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	h := &libraryHandler{settingsService: svc}

	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")
	seedLibraryAccess(t, db, user, lib.ID)

	e := newTestEcho()
	c, rec := buildGetRequest(t, e, user, lib.ID)

	require.NoError(t, h.getLibrarySettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body LibrarySettingsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Nil(t, body.SortSpec)
}

func TestGetLibrarySettings_Forbidden(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	h := &libraryHandler{settingsService: svc}

	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books") // intentionally no seedLibraryAccess
	_ = lib

	e := newTestEcho()
	c, _ := buildGetRequest(t, e, user, lib.ID)

	err := h.getLibrarySettings(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Forbidden")
}

func TestUpdateLibrarySettings_PersistsSpec(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	h := &libraryHandler{settingsService: svc}

	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")
	seedLibraryAccess(t, db, user, lib.ID)

	e := newTestEcho()
	c, rec := buildPutRequest(t, e, user, lib.ID, `{"sort_spec":"title:asc"}`)

	require.NoError(t, h.updateLibrarySettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	stored, err := svc.GetLibrarySettings(context.Background(), user.ID, lib.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.NotNil(t, stored.SortSpec)
	assert.Equal(t, "title:asc", *stored.SortSpec)
}

func TestUpdateLibrarySettings_RejectsInvalidSpec(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	h := &libraryHandler{settingsService: svc}

	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")
	seedLibraryAccess(t, db, user, lib.ID)

	e := newTestEcho()
	c, _ := buildPutRequest(t, e, user, lib.ID, `{"sort_spec":"bogus_field:asc"}`)

	err := h.updateLibrarySettings(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown sort field")
}

func TestUpdateLibrarySettings_AcceptsNullClear(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	h := &libraryHandler{settingsService: svc}

	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")
	seedLibraryAccess(t, db, user, lib.ID)

	// Seed a spec first.
	spec := "title:asc"
	_, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &spec)
	require.NoError(t, err)

	e := newTestEcho()
	c, rec := buildPutRequest(t, e, user, lib.ID, `{"sort_spec":null}`)

	require.NoError(t, h.updateLibrarySettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	stored, err := svc.GetLibrarySettings(context.Background(), user.ID, lib.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Nil(t, stored.SortSpec)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/settings/ -run "TestGetLibrarySettings_NoRow|TestGetLibrarySettings_Forbidden|TestUpdateLibrarySettings" -v`
Expected: FAIL — `undefined: libraryHandler`.

- [ ] **Step 3: Write handler implementation**

`pkg/settings/library_handlers.go`:

```go
package settings

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
)

// libraryHandler handles per-(user × library) settings endpoints.
type libraryHandler struct {
	settingsService *Service
}

func (h *libraryHandler) getLibrarySettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	libraryID, err := strconv.Atoi(c.Param("library_id"))
	if err != nil || libraryID < 1 {
		return errcodes.ValidationError("invalid library_id")
	}

	if !user.HasLibraryAccess(libraryID) {
		return errcodes.Forbidden("You don't have access to this library")
	}

	row, err := h.settingsService.GetLibrarySettings(ctx, user.ID, libraryID)
	if err != nil {
		return errors.WithStack(err)
	}

	resp := LibrarySettingsResponse{}
	if row != nil {
		resp.SortSpec = row.SortSpec
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *libraryHandler) updateLibrarySettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	libraryID, err := strconv.Atoi(c.Param("library_id"))
	if err != nil || libraryID < 1 {
		return errcodes.ValidationError("invalid library_id")
	}

	if !user.HasLibraryAccess(libraryID) {
		return errcodes.Forbidden("You don't have access to this library")
	}

	var payload UpdateLibrarySettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	// Validate sort_spec if present (nil means "clear").
	if payload.SortSpec != nil && *payload.SortSpec != "" {
		if _, err := sortspec.Parse(*payload.SortSpec); err != nil {
			return errcodes.ValidationError(err.Error())
		}
	}
	// Treat empty string as equivalent to null (clear).
	var toStore *string
	if payload.SortSpec != nil && *payload.SortSpec != "" {
		toStore = payload.SortSpec
	}

	row, err := h.settingsService.UpsertLibrarySort(ctx, user.ID, libraryID, toStore)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, LibrarySettingsResponse{SortSpec: row.SortSpec})
}
```

- [ ] **Step 4: Register the new routes**

Replace the contents of `pkg/settings/routes.go` with:

```go
package settings

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Echo, db *bun.DB, authMiddleware *auth.Middleware) {
	svc := NewService(db)

	viewerH := &handler{settingsService: svc}
	libraryH := &libraryHandler{settingsService: svc}

	g := e.Group("/settings")
	g.Use(authMiddleware.Authenticate)

	g.GET("/viewer", viewerH.getViewerSettings)
	g.PUT("/viewer", viewerH.updateViewerSettings)

	g.GET("/libraries/:library_id", libraryH.getLibrarySettings)
	g.PUT("/libraries/:library_id", libraryH.updateLibrarySettings)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/settings/... -v`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/settings/library_handlers.go pkg/settings/library_handlers_test.go pkg/settings/routes.go
git commit -m "[Backend] Add library settings endpoints"
```

---

## Phase 4: Books integration

### Task 10: Add Sort to ListBooksOptions and apply in query

**Files:**
- Modify: `pkg/books/service.go`
- Create/Modify: `pkg/books/service_sort_test.go`

- [ ] **Step 1: Write a failing integration test that depends on Sort**

Create `pkg/books/service_sort_test.go`:

```go
package books

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupBooksTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() { db.Close() })
	return db
}

func seedLibrary(t *testing.T, db *bun.DB, name string) *models.Library {
	t.Helper()
	l := &models.Library{
		Name:                     name,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(l).Exec(context.Background())
	require.NoError(t, err)
	return l
}

func seedBook(t *testing.T, db *bun.DB, lib *models.Library, title, sortTitle string, createdAt time.Time) *models.Book {
	t.Helper()
	b := &models.Book{
		LibraryID:    lib.ID,
		Title:        title,
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    sortTitle,
		AuthorSource: models.DataSourceFilepath,
		Filepath:     "/test/" + title + ".epub",
		CreatedAt:    createdAt,
	}
	_, err := db.NewInsert().Model(b).Exec(context.Background())
	require.NoError(t, err)
	return b
}

// TestListBooks_SortByTitleAsc confirms an explicit Sort overrides the
// default sort_title ASC ordering.
func TestListBooks_SortByTitleAsc(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	svc := NewService(db)
	lib := seedLibrary(t, db, "Books")

	now := time.Now()
	// Titles intentionally in non-alphabetic insertion order.
	cheese := seedBook(t, db, lib, "Cheese", "Cheese", now.Add(-2*time.Hour))
	apple := seedBook(t, db, lib, "Apple", "Apple", now.Add(-time.Hour))
	banana := seedBook(t, db, lib, "Banana", "Banana", now)

	got, _, err := svc.ListBooksWithTotal(context.Background(), ListBooksOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldTitle, Direction: sortspec.DirAsc}},
	})
	require.NoError(t, err)
	require.Equal(t, 3, len(got))
	assert.Equal(t, apple.ID, got[0].ID)
	assert.Equal(t, banana.ID, got[1].ID)
	assert.Equal(t, cheese.ID, got[2].ID)
}

// TestListBooks_SortByDateAddedDesc confirms the primary use case for the
// frontend's builtin default.
func TestListBooks_SortByDateAddedDesc(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	svc := NewService(db)
	lib := seedLibrary(t, db, "Books")

	now := time.Now()
	oldest := seedBook(t, db, lib, "Oldest", "Oldest", now.Add(-3*time.Hour))
	middle := seedBook(t, db, lib, "Middle", "Middle", now.Add(-time.Hour))
	newest := seedBook(t, db, lib, "Newest", "Newest", now)

	got, _, err := svc.ListBooksWithTotal(context.Background(), ListBooksOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldDateAdded, Direction: sortspec.DirDesc}},
	})
	require.NoError(t, err)
	require.Equal(t, 3, len(got))
	assert.Equal(t, newest.ID, got[0].ID)
	assert.Equal(t, middle.ID, got[1].ID)
	assert.Equal(t, oldest.ID, got[2].ID)
}

// TestListBooks_NilSortUsesDefault preserves backward compatibility.
func TestListBooks_NilSortUsesDefault(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	svc := NewService(db)
	lib := seedLibrary(t, db, "Books")

	now := time.Now()
	cheese := seedBook(t, db, lib, "Cheese", "Cheese", now)
	apple := seedBook(t, db, lib, "Apple", "Apple", now.Add(-time.Hour))

	got, _, err := svc.ListBooksWithTotal(context.Background(), ListBooksOptions{
		LibraryID: &lib.ID,
		Sort:      nil,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(got))
	// Default is sort_title ASC → Apple before Cheese.
	assert.Equal(t, apple.ID, got[0].ID)
	assert.Equal(t, cheese.ID, got[1].ID)
}
```

- [ ] **Step 2: Run the failing test**

Run: `go test ./pkg/books/ -run TestListBooks_Sort -v`
Expected: FAIL — `unknown field Sort in struct literal of type books.ListBooksOptions`.

- [ ] **Step 3: Add the field to ListBooksOptions**

Open `pkg/books/service.go`. Locate the `ListBooksOptions` struct (around line 29). Add one field at the bottom (before `includeTotal`):

```go
type ListBooksOptions struct {
	Limit      *int
	Offset     *int
	LibraryID  *int
	LibraryIDs []int // Filter by multiple library IDs (for access control)
	SeriesID   *int
	FileTypes  []string // Filter by file types (e.g., ["epub", "cbz"])
	GenreIDs   []int    // Filter by genre IDs
	TagIDs     []int    // Filter by tag IDs
	Language   *string  // Filter by language tag (matches exact tag and subtag variants, e.g. "en" matches "en-US")
	IDs        []int    // Filter by specific book IDs
	Search     *string  // Search query for title/author

	// Sort overrides the default ordering. When nil, the legacy default
	// (sort_title ASC, or series_number ASC + sort_title ASC when
	// filtering by series) is preserved.
	Sort []sortspec.SortLevel

	includeTotal  bool
	orderByRecent bool // Order by updated_at DESC instead of created_at ASC
}
```

Add the import at the top of the file if not already present:

```go
"github.com/shishobooks/shisho/pkg/sortspec"
```

- [ ] **Step 4: Apply Sort in listBooksWithTotal**

Locate the ordering block in `pkg/books/service.go` (around line 305) and replace it:

```go
// Apply ordering.
// Precedence: orderByRecent (internal flag) > explicit Sort > legacy default.
switch {
case opts.orderByRecent:
	q = q.Order("b.updated_at DESC")

case len(opts.Sort) > 0:
	for _, clause := range sortspec.OrderClauses(opts.Sort) {
		q = q.OrderExpr(clause.Expression, clause.Args...)
	}

case opts.SeriesID != nil:
	// When filtering by series, order by series_number then sort_title
	q = q.Order("bs_filter.series_number ASC", "b.sort_title ASC")

default:
	q = q.Order("b.sort_title ASC")
}
```

- [ ] **Step 5: Run the new tests**

Run: `go test ./pkg/books/ -run TestListBooks_Sort -v`
Expected: All PASS.

- [ ] **Step 6: Run the full books package tests to ensure no regression**

Run: `go test ./pkg/books/... -v`
Expected: All PASS (existing tests continue to see nil Sort and hit the default branch).

- [ ] **Step 7: Commit**

```bash
git add pkg/books/service.go pkg/books/service_sort_test.go
git commit -m "[Backend] Wire sortspec into ListBooksOptions"
```

---

### Task 11: Add sort query param and handler resolution

**Files:**
- Modify: `pkg/books/validators.go`
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`
- Create: `pkg/books/handlers_list_test.go`

- [ ] **Step 1: Add the `Sort` field to the query validator**

Open `pkg/books/validators.go`. Add one field to `ListBooksQuery`:

```go
type ListBooksQuery struct {
	Limit     int      `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset    int      `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *int     `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	SeriesID  *int     `query:"series_id" json:"series_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search    *string  `query:"search" json:"search,omitempty" validate:"omitempty,max=100" tstype:"string"`
	FileTypes []string `query:"file_types" json:"file_types,omitempty"`
	GenreIDs  []int    `query:"genre_ids" json:"genre_ids,omitempty"`
	TagIDs    []int    `query:"tag_ids" json:"tag_ids,omitempty"`
	Language  *string  `query:"language" json:"language,omitempty" validate:"omitempty,max=35" tstype:"string"`
	IDs       []int    `query:"ids" json:"ids,omitempty"`
	Sort      string   `query:"sort" json:"sort,omitempty" validate:"omitempty,max=200"`
}
```

- [ ] **Step 2: Wire the settings service into the books handler**

Open `pkg/books/handlers.go` and add a `settingsService` field to the handler struct. Locate the struct declaration (search for `type handler struct` in that file) and add:

```go
type handler struct {
	// ... existing fields ...
	settingsService *settings.Service
}
```

Add the import if not present: `"github.com/shishobooks/shisho/pkg/settings"`.

Then update `pkg/books/routes.go` to construct the service and pass it in. Find `func RegisterRoutes(...)` and update the handler construction to include `settingsService: settings.NewService(db)` (add the import).

NOTE: Each downstream package (books, opds, ereader) constructs its own `settings.NewService(db)` instance. The service is stateless (it wraps `*bun.DB` and has no in-memory caches), so the duplication is cheap and avoids threading a shared instance through `RegisterRoutes` signatures.

- [ ] **Step 3: Write a failing handler test**

Create `pkg/books/handlers_list_test.go`:

```go
package books

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func newTestEchoBooks() *echo.Echo {
	e := echo.New()
	e.Binder = binder.New()
	return e
}

func seedUserWithLibAccess(t *testing.T, db *bun.DB, username string, lib *models.Library) *models.User {
	t.Helper()
	u := &models.User{
		Username:     username,
		PasswordHash: "x",
		RoleID:       1,
		IsActive:     true,
	}
	_, err := db.NewInsert().Model(u).Exec(context.Background())
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.UserLibrary{UserID: u.ID, LibraryID: lib.ID}).Exec(context.Background())
	require.NoError(t, err)

	// Reload so u.Libraries is populated for HasLibraryAccess checks.
	reloaded := &models.User{}
	err = db.NewSelect().Model(reloaded).Relation("Libraries").Where("u.id = ?", u.ID).Scan(context.Background())
	require.NoError(t, err)
	return reloaded
}

// TestListHandler_ExplicitSortWins verifies the sort query param is parsed
// and passed through, overriding any stored preference.
func TestListHandler_ExplicitSortWins(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "Books")
	user := seedUserWithLibAccess(t, db, "alice", lib)

	now := time.Now()
	cheese := seedBook(t, db, lib, "Cheese", "Cheese", now)
	apple := seedBook(t, db, lib, "Apple", "Apple", now.Add(-time.Hour))

	// Stored preference is date_added:desc
	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
	_, err := settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	h := &handler{bookService: NewService(db), settingsService: settingsSvc}

	e := newTestEchoBooks()
	// Explicit sort in URL overrides stored default.
	req := httptest.NewRequest(http.MethodGet, "/books?library_id="+strconv.Itoa(lib.ID)+"&sort=title:asc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.list(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Books []*models.Book `json:"books"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 2, len(resp.Books))
	// title:asc → Apple before Cheese
	assert.Equal(t, apple.ID, resp.Books[0].ID)
	assert.Equal(t, cheese.ID, resp.Books[1].ID)
}

// TestListHandler_StoredPreferenceUsed verifies that when no URL sort is
// provided, the stored preference drives ordering.
func TestListHandler_StoredPreferenceUsed(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "Books")
	user := seedUserWithLibAccess(t, db, "alice", lib)

	now := time.Now()
	oldest := seedBook(t, db, lib, "Oldest", "Oldest", now.Add(-2*time.Hour))
	newest := seedBook(t, db, lib, "Newest", "Newest", now)

	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
	_, err := settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	h := &handler{bookService: NewService(db), settingsService: settingsSvc}

	e := newTestEchoBooks()
	req := httptest.NewRequest(http.MethodGet, "/books?library_id="+strconv.Itoa(lib.ID), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.list(c))

	var resp struct {
		Books []*models.Book `json:"books"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 2, len(resp.Books))
	assert.Equal(t, newest.ID, resp.Books[0].ID)
	assert.Equal(t, oldest.ID, resp.Books[1].ID)
}

// TestListHandler_InvalidSortReturns400 verifies validation.
func TestListHandler_InvalidSortReturns400(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "Books")
	user := seedUserWithLibAccess(t, db, "alice", lib)

	h := &handler{bookService: NewService(db), settingsService: settings.NewService(db)}

	e := newTestEchoBooks()
	req := httptest.NewRequest(
		http.MethodGet,
		"/books?library_id="+strconv.Itoa(lib.ID)+"&sort="+url.QueryEscape("bogus_field:asc"),
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	err := h.list(c)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unknown sort field")
}

// TestListHandler_NoLibraryIDSkipsStoredLookup verifies the resolver only
// engages when scoped to a single library.
func TestListHandler_NoLibraryIDSkipsStoredLookup(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "Books")
	user := seedUserWithLibAccess(t, db, "alice", lib)

	now := time.Now()
	cheese := seedBook(t, db, lib, "Cheese", "Cheese", now)
	apple := seedBook(t, db, lib, "Apple", "Apple", now.Add(-time.Hour))

	// Stored preference would produce date_added:desc → cheese first,
	// but without library_id the resolver must be skipped and the
	// default sort_title ASC → apple first applies.
	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
	_, err := settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	h := &handler{bookService: NewService(db), settingsService: settingsSvc}

	e := newTestEchoBooks()
	req := httptest.NewRequest(http.MethodGet, "/books", nil) // no library_id
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.list(c))

	var resp struct {
		Books []*models.Book `json:"books"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 2, len(resp.Books))
	assert.Equal(t, apple.ID, resp.Books[0].ID) // alphabetical default
	assert.Equal(t, cheese.ID, resp.Books[1].ID)
}
```

- [ ] **Step 4: Run the failing tests**

Run: `go test ./pkg/books/ -run TestListHandler -v`
Expected: FAIL — handler doesn't yet parse `Sort` or call `ResolveForLibrary`.

- [ ] **Step 5: Update the list handler**

Open `pkg/books/handlers.go`. Locate the `list` function (near line 120). Replace the `opts := ListBooksOptions{...}` block and everything up to the `books, total, err := h.bookService.ListBooksWithTotal(ctx, opts)` call with:

```go
	opts := ListBooksOptions{
		Limit:     &params.Limit,
		Offset:    &params.Offset,
		LibraryID: params.LibraryID,
		SeriesID:  params.SeriesID,
		Search:    params.Search,
		FileTypes: params.FileTypes,
		GenreIDs:  params.GenreIDs,
		TagIDs:    params.TagIDs,
		Language:  languageFilter,
		IDs:       params.IDs,
	}

	// Parse an explicit sort param if present.
	var explicitSort []sortspec.SortLevel
	if params.Sort != "" {
		levels, err := sortspec.Parse(params.Sort)
		if err != nil {
			return errcodes.ValidationError(err.Error())
		}
		explicitSort = levels
	}

	// Filter by user's library access if user is in context.
	var user *models.User
	if u, ok := c.Get("user").(*models.User); ok {
		user = u
		libraryIDs := user.GetAccessibleLibraryIDs()
		if libraryIDs != nil {
			opts.LibraryIDs = libraryIDs
		}
	}

	// Resolve stored preference only when scoped to a single library.
	// Without a library_id filter we have no single (user, library) pair
	// to look up, so explicit wins or we fall through to the default.
	if user != nil && params.LibraryID != nil {
		opts.Sort = sortspec.ResolveForLibrary(ctx, h.settingsService, user.ID, *params.LibraryID, explicitSort)
	} else {
		opts.Sort = explicitSort
	}

	books, total, err := h.bookService.ListBooksWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}
```

Add imports to the top of `pkg/books/handlers.go` if not present:

```go
"github.com/shishobooks/shisho/pkg/sortspec"
```

- [ ] **Step 6: Run the tests**

Run: `go test ./pkg/books/... -v`
Expected: All PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/books/validators.go pkg/books/handlers.go pkg/books/routes.go pkg/books/handlers_list_test.go
git commit -m "[Backend] Add sort query param to GET /books"
```

---

## Phase 5: OPDS integration

### Task 12: Thread Sort through OPDS service signatures

**Files:**
- Modify: `pkg/opds/service.go`

- [ ] **Step 1: Add a `sort` parameter to each library-scoped feed builder**

There are 7 OPDS service methods that call `bookService.ListBooksWithTotal` (or `ListBooks`). Each gets a new trailing parameter `sort []sortspec.SortLevel` and passes it through via `ListBooksOptions.Sort`.

Add the import at the top of `pkg/opds/service.go`:

```go
"github.com/shishobooks/shisho/pkg/sortspec"
```

Update each of the following signatures (file: `pkg/opds/service.go`) and the corresponding call sites. Apply the same change pattern to each.

**Method 1 (~line 120): `BuildLibraryAllBooksFeed`**

Old:
```go
func (svc *Service) BuildLibraryAllBooksFeed(ctx context.Context, baseURL, fileTypes string, libraryID, limit, offset int) (*Feed, error) {
```

New:
```go
func (svc *Service) BuildLibraryAllBooksFeed(ctx context.Context, baseURL, fileTypes string, libraryID, limit, offset int, sort []sortspec.SortLevel) (*Feed, error) {
```

Inside, locate the `ListBooksWithTotal` call and add `Sort: sort,` to the options struct:

```go
booksResult, total, err := svc.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
    Limit:     &limit,
    Offset:    &offset,
    LibraryID: &libraryID,
    FileTypes: types,
    Sort:      sort,
})
```

**Method 2 (~line 165): `BuildLibraryAllBooksFeedKepub`** — same shape as method 1.

**Method 3 (~line 260): `BuildLibrarySeriesBooksFeed`** — signature gains `sort []sortspec.SortLevel`, call site adds `Sort: sort,`.

**Method 4 (~line 312): `BuildLibrarySeriesBooksFeedKepub`** — same.

**Method 5 (~line 430): `BuildLibraryAuthorBooksFeed`** — this one calls `ListBooksByAuthor` which calls `ListBooks`. Pass `sort` into `ListBooksByAuthor` and from there into the `ListBooks` call.

**Method 6 (~line 590): `BuildLibrarySearchFeed`** — same as method 1.

**Method 7 (~line 640): `BuildLibrarySearchFeedKepub`** — same.

Also update the `ListBooksByAuthor` helper around line 475 that `BuildLibraryAuthorBooksFeed` delegates to:

```go
func (svc *Service) ListBooksByAuthor(ctx context.Context, libraryID int, authorName string, fileTypes []string, limit, offset int, sort []sortspec.SortLevel) ([]*models.Book, int, error) {
    // ...
    booksResult, err := svc.bookService.ListBooks(ctx, books.ListBooksOptions{
        LibraryID: &libraryID,
        FileTypes: fileTypes,
        Sort:      sort,
    })
    // ...
}
```

And if there's an author-KePub variant, apply the same change.

- [ ] **Step 2: Build to confirm signatures compile**

Run: `go build ./pkg/opds/...`
Expected: fails — callers (the OPDS handlers) now pass the wrong number of arguments. This is expected; handlers are updated in Task 13.

To isolate: temporarily pass `nil` at each handler call site just to make the build pass:
```bash
# Find all handler call sites and add a trailing `, nil` argument.
```

Actually — skip this step. Move directly to Task 13 in the same commit so the build is never broken on `main`.

- [ ] **Step 3: Do NOT commit yet — proceed to Task 13.**

---

### Task 13: Wire OPDS handlers to resolve sort preference

**Files:**
- Modify: `pkg/opds/handlers.go`
- Modify: `pkg/opds/routes.go`

- [ ] **Step 1: Add settings service dependency to the OPDS handler**

Open `pkg/opds/handlers.go`. Locate the `handler` struct (search for `type handler struct`). Add one field:

```go
type handler struct {
	opdsService     *Service
	bookService     *books.Service
	downloadCache   *downloadcache.Cache
	settingsService *settings.Service
}
```

Import `"github.com/shishobooks/shisho/pkg/settings"` if not present.

- [ ] **Step 2: Update OPDS routes to construct and inject the settings service**

Open `pkg/opds/routes.go`. Update the handler construction in `RegisterRoutes`:

```go
h := &handler{
	opdsService:     opdsService,
	bookService:     bookService,
	downloadCache:   cache,
	settingsService: settings.NewService(db),
}
```

Add the import for `settings`.

- [ ] **Step 3: Write a small helper inside `pkg/opds/handlers.go`**

At the top of the file (after imports, before the handler struct), add:

```go
// resolveSort resolves the stored user-library sort preference for the
// current request, or returns nil so the service's legacy default
// ordering applies.
//
// OPDS is read-only; no explicit `?sort=` input is parsed here (that's
// a v2+ feature). The caller's user is taken from c.Get("user"),
// populated by the BasicAuth middleware.
func (h *handler) resolveSort(c echo.Context, libraryID int) []sortspec.SortLevel {
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return nil
	}
	return sortspec.ResolveForLibrary(c.Request().Context(), h.settingsService, user.ID, libraryID, nil)
}
```

Add imports: `"github.com/shishobooks/shisho/pkg/sortspec"` and `"github.com/shishobooks/shisho/pkg/models"`.

- [ ] **Step 4: Update each feed handler call site to pass the resolved sort**

For each of the 7 OPDS handler functions (search by calling function name in `pkg/opds/handlers.go`), add one line above the service call:

```go
sort := h.resolveSort(c, libraryID)
```

And pass `sort` as the new trailing argument to the service call. Example for `libraryAllBooks`:

```go
func (h *handler) libraryAllBooks(c echo.Context) error {
    // ... existing param parsing up to `libraryID` ...
    sort := h.resolveSort(c, libraryID)

    feed, err := h.opdsService.BuildLibraryAllBooksFeed(
        c.Request().Context(), baseURL, fileTypes, libraryID, limit, offset, sort,
    )
    if err != nil {
        return errors.WithStack(err)
    }
    // ... existing rendering ...
}
```

Handlers that need updating (one call per handler, 7 total):

1. `libraryAllBooks`
2. `libraryAllBooksKepub`
3. `librarySeriesBooks`
4. `librarySeriesBooksKepub`
5. `libraryAuthorBooks` (and `libraryAuthorBooksKepub` if present — 8th case)
6. `librarySearch`
7. `librarySearchKepub`

If `libraryAuthorBooksKepub` exists as a separate handler, update it too.

- [ ] **Step 5: Build and run all OPDS tests**

Run: `go build ./pkg/opds/...`
Expected: PASS.

Run: `go test ./pkg/opds/... -v`
Expected: existing tests still PASS. (Changing the default sort only matters when a stored preference exists; existing tests seed fresh DBs with no preferences, so their ordering is unchanged.)

- [ ] **Step 6: Add an integration test for preference application**

Create a new test file `pkg/opds/handlers_sort_test.go` (or append to an existing `*_test.go`):

```go
package opds

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupOPDSDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// TestLibraryAllBooksFeed_HonorsStoredSort confirms the OPDS "all books"
// feed applies the user's stored library sort preference.
func TestLibraryAllBooksFeed_HonorsStoredSort(t *testing.T) {
	t.Parallel()

	db := setupOPDSDB(t)

	// Seed data: two books with distinct created_at and sort_title.
	lib := &models.Library{Name: "Books", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)

	now := time.Now()
	apple := &models.Book{LibraryID: lib.ID, Title: "Apple", SortTitle: "Apple", Filepath: "/a", CreatedAt: now.Add(-2 * time.Hour), TitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath}
	cheese := &models.Book{LibraryID: lib.ID, Title: "Cheese", SortTitle: "Cheese", Filepath: "/c", CreatedAt: now, TitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath}
	_, err = db.NewInsert().Model(apple).Exec(context.Background())
	require.NoError(t, err)
	_, err = db.NewInsert().Model(cheese).Exec(context.Background())
	require.NoError(t, err)

	user := &models.User{Username: "alice", PasswordHash: "x", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(context.Background())
	require.NoError(t, err)

	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
	_, err = settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	opdsSvc := NewService(db)

	// Resolve the sort the way the handler would.
	resolved, err := settingsSvc.GetLibrarySettings(context.Background(), user.ID, lib.ID)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.NotNil(t, resolved.SortSpec)

	// Exercise the service directly with the resolved preference.
	feed, err := opdsSvc.BuildLibraryAllBooksFeed(context.Background(), "http://x", "epub", lib.ID, 10, 0, mustParse(t, *resolved.SortSpec))
	require.NoError(t, err)
	require.Len(t, feed.Entries, 2)
	// date_added:desc → cheese first, apple second.
	assert.Contains(t, feed.Entries[0].Title, "Cheese")
	assert.Contains(t, feed.Entries[1].Title, "Apple")
}
```

(Replace the `mustParse(t, *resolved.SortSpec)` call with `func mustParse(t *testing.T, s string) []sortspec.SortLevel { t.Helper(); levels, err := sortspec.Parse(s); require.NoError(t, err); return levels }` defined at the bottom of the test file, plus the `sortspec` import.)

- [ ] **Step 7: Run the new test**

Run: `go test ./pkg/opds/ -run TestLibraryAllBooksFeed_HonorsStoredSort -v`
Expected: PASS.

- [ ] **Step 8: Commit Tasks 12 + 13 together**

```bash
git add pkg/opds/service.go pkg/opds/handlers.go pkg/opds/routes.go pkg/opds/handlers_sort_test.go
git commit -m "[Backend] Apply stored library sort to OPDS feeds"
```

---

## Phase 6: eReader integration

### Task 14: Apply sort preference to eReader handlers

**Files:**
- Modify: `pkg/ereader/handlers.go`
- Modify: `pkg/ereader/routes.go`

- [ ] **Step 1: Add settings service to the eReader handler**

Open `pkg/ereader/handlers.go`. Locate the handler struct declaration (search `type handler struct` or `func newHandler`). Add one field:

```go
type handler struct {
	// ... existing fields ...
	settingsService *settings.Service
}
```

Add import `"github.com/shishobooks/shisho/pkg/settings"`.

- [ ] **Step 2: Update `newHandler` and `RegisterRoutes` to inject it**

In `pkg/ereader/handlers.go`, update `newHandler` signature to accept a `*settings.Service` and assign it.

In `pkg/ereader/routes.go`, update the call:

```go
h := newHandler(db, libraryService, bookService, seriesService, peopleService, downloadCache, settings.NewService(db))
```

Add the `settings` import.

- [ ] **Step 3: Add a sort-resolution helper**

At the top of `pkg/ereader/handlers.go` (after imports), add:

```go
// resolveSort returns the user's stored library sort preference for the
// current API-key call, or nil if none is set or resolution fails.
func (h *handler) resolveSort(ctx context.Context, apiKey *apikeys.APIKey, libraryID int) []sortspec.SortLevel {
	if apiKey == nil {
		return nil
	}
	return sortspec.ResolveForLibrary(ctx, h.settingsService, apiKey.UserID, libraryID, nil)
}
```

Add imports: `"github.com/shishobooks/shisho/pkg/sortspec"` and `"github.com/shishobooks/shisho/pkg/apikeys"` if not present.

- [ ] **Step 4: Update each of the 4 book-list handlers**

Locate each handler and add a `sort := h.resolveSort(...)` line before the `h.bookService.ListBooksWithTotal` call, then set `Sort: sort` on the options struct.

**Handler 1: `LibraryAllBooks` (line ~128)**

Find both branches (the `if typesFilter != ""` and `else`) and update each `ListBooksOptions` literal:

```go
sort := h.resolveSort(ctx, apiKey, libraryIDInt)

if typesFilter != "" && typesFilter != "all" {
    allBooks, _, err := h.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
        LibraryID: &libraryIDInt,
        Sort:      sort,
    })
    // ...
} else {
    offset := (page - 1) * defaultPageSize
    booksResult, total, err = h.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
        Limit:     intPtr(defaultPageSize),
        Offset:    intPtr(offset),
        LibraryID: &libraryIDInt,
        Sort:      sort,
    })
    // ...
}
```

**Handler 2: `SeriesBooks` (line ~263)**

```go
sort := h.resolveSort(ctx, apiKey, libraryIDInt)

booksResult, _, err := h.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
    LibraryID: &libraryIDInt,
    SeriesID:  &seriesIDInt,
    Sort:      sort,
})
```

**Handler 3: `AuthorBooks` (line ~188)**

Locate the `ListBooksWithTotal` call inside `AuthorBooks` and add the same pattern.

**Handler 4: `LibrarySearch` (line ~461)**

```go
sort := h.resolveSort(ctx, apiKey, libraryIDInt)

booksResult, _, err := h.bookService.ListBooksWithTotal(ctx, books.ListBooksOptions{
    LibraryID: &libraryIDInt,
    Search:    &query,
    Limit:     intPtr(defaultPageSize),
    Sort:      sort,
})
```

- [ ] **Step 5: Build and verify**

Run: `go build ./pkg/ereader/...`
Expected: PASS.

- [ ] **Step 6: Add a handler integration test**

Create `pkg/ereader/handlers_sort_test.go`:

```go
package ereader

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupEReaderDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// TestEReaderResolveSort_AppliesStoredPreference confirms the eReader
// sort-resolution helper picks up a stored `(user, library)` preference.
//
// We exercise the resolution path at the service boundary (ListBooksWithTotal
// with the resolved Sort), which is what the handler does internally. This
// keeps the test independent of the HTML template layer and of whichever API
// key middleware wrapper is in use.
func TestEReaderResolveSort_AppliesStoredPreference(t *testing.T) {
	t.Parallel()

	db := setupEReaderDB(t)

	// Seed a library with two books distinguishable by created_at.
	lib := &models.Library{
		Name:                     "Books",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)

	now := time.Now()
	apple := &models.Book{
		LibraryID: lib.ID, Title: "Apple", SortTitle: "Apple", Filepath: "/a",
		CreatedAt: now.Add(-2 * time.Hour),
		TitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath,
	}
	cheese := &models.Book{
		LibraryID: lib.ID, Title: "Cheese", SortTitle: "Cheese", Filepath: "/c",
		CreatedAt: now,
		TitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(apple).Exec(context.Background())
	require.NoError(t, err)
	_, err = db.NewInsert().Model(cheese).Exec(context.Background())
	require.NoError(t, err)

	// Seed a user and save a stored preference of date_added:desc.
	user := &models.User{Username: "alice", PasswordHash: "x", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(context.Background())
	require.NoError(t, err)

	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
	_, err = settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	// The API key struct the eReader middleware would install.
	apiKey := &apikeys.APIKey{UserID: user.ID}

	// Resolve via the same helper the handler uses.
	resolved := sortspec.ResolveForLibrary(
		context.Background(),
		settingsSvc,
		apiKey.UserID,
		lib.ID,
		nil, // eReader never carries an explicit URL sort
	)
	require.Equal(t, []sortspec.SortLevel{
		{Field: sortspec.FieldDateAdded, Direction: sortspec.DirDesc},
	}, resolved)

	// Feed it into the books service exactly as the handler does.
	bookSvc := books.NewService(db)
	got, _, err := bookSvc.ListBooksWithTotal(context.Background(), books.ListBooksOptions{
		LibraryID: &lib.ID,
		Sort:      resolved,
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	// date_added:desc → cheese (now) before apple (now-2h).
	assert.Equal(t, cheese.ID, got[0].ID)
	assert.Equal(t, apple.ID, got[1].ID)
}

// TestEReaderResolveSort_NoPreferenceReturnsNil confirms that without a
// stored preference, ResolveForLibrary returns nil and the handler falls
// through to its existing default ordering.
func TestEReaderResolveSort_NoPreferenceReturnsNil(t *testing.T) {
	t.Parallel()

	db := setupEReaderDB(t)
	lib := &models.Library{
		Name:                     "Books",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)

	user := &models.User{Username: "alice", PasswordHash: "x", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(context.Background())
	require.NoError(t, err)

	settingsSvc := settings.NewService(db)
	resolved := sortspec.ResolveForLibrary(
		context.Background(), settingsSvc, user.ID, lib.ID, nil,
	)
	assert.Nil(t, resolved, "no stored preference → nil (handler uses its default)")
}
```

Rationale for testing at the resolver seam rather than the handler: the eReader handlers render HTML templates and authenticate via middleware that installs `*apikeys.APIKey` into the Echo context. Unit-testing the full HTTP path couples this test to template structure and middleware wiring that don't add signal over the resolver check. The two tests above verify both branches of the resolver with real seeded data and confirm the `books` service returns the expected order; that's the sort behavior we care about.

- [ ] **Step 7: Run all eReader tests**

Run: `go test ./pkg/ereader/... -v`
Expected: All PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/ereader/handlers.go pkg/ereader/routes.go pkg/ereader/handlers_sort_test.go
git commit -m "[Backend] Apply stored library sort to eReader handlers"
```

---

## Phase 7: Tygo regen

### Task 15: Regenerate TypeScript types

- [ ] **Step 1: Run tygo**

Run: `mise tygo`
Expected: Either "skipping, outputs are up-to-date" (if the generated types haven't materially changed — possible, since `UserLibrarySettings` has `tstype:"-"` equivalent isn't on it so it will be emitted) or a successful regeneration.

- [ ] **Step 2: Confirm the generated types include UserLibrarySettings, LibrarySettingsResponse, UpdateLibrarySettingsPayload, and the new `Sort` field on `ListBooksQuery`**

Run: `grep -n "UserLibrarySettings\|LibrarySettingsResponse\|UpdateLibrarySettingsPayload\|Sort" app/types/generated/*.ts`
Expected: the above symbols appear in the generated output. If any are missing, check the relevant Go struct's `tstype` tag — `UserLibrarySettings` should be emitted (no `tstype:"-"` on the struct-level BaseModel tag). `LibrarySettingsResponse` and `UpdateLibrarySettingsPayload` should naturally emit from `pkg/settings/validators.go`.

- [ ] **Step 3: Commit**

Note: `app/types/generated/` is gitignored; no commit needed for the generated output. If the `tygo.yaml` or a Go struct needed adjustment, commit those source changes.

---

## Phase 8: Frontend

### Task 16: TypeScript sortSpec parser

**Files:**
- Create: `app/lib/sortSpec.ts`
- Create: `app/lib/sortSpec.test.ts`

- [ ] **Step 1: Write failing tests**

```typescript
import { describe, expect, it } from "vitest";

import {
  BUILTIN_DEFAULT_SORT,
  parseSortSpec,
  serializeSortSpec,
  SORT_FIELDS,
  type SortLevel,
} from "./sortSpec";

describe("parseSortSpec", () => {
  it("parses single level", () => {
    expect(parseSortSpec("title:asc")).toEqual([
      { field: "title", direction: "asc" },
    ]);
  });

  it("parses multi-level", () => {
    expect(parseSortSpec("author:asc,series:asc,title:asc")).toEqual([
      { field: "author", direction: "asc" },
      { field: "series", direction: "asc" },
      { field: "title", direction: "asc" },
    ]);
  });

  it("returns null for unknown field", () => {
    expect(parseSortSpec("bogus:asc")).toBeNull();
  });

  it("returns null for bad direction", () => {
    expect(parseSortSpec("title:sideways")).toBeNull();
  });

  it("returns null for duplicate field", () => {
    expect(parseSortSpec("title:asc,title:desc")).toBeNull();
  });

  it("returns null for empty string", () => {
    expect(parseSortSpec("")).toBeNull();
  });
});

describe("serializeSortSpec", () => {
  it("serializes a single level", () => {
    expect(
      serializeSortSpec([{ field: "title", direction: "asc" }]),
    ).toBe("title:asc");
  });

  it("returns empty string for empty array", () => {
    expect(serializeSortSpec([])).toBe("");
  });

  it("round-trips", () => {
    const input = "author:asc,date_added:desc";
    const parsed = parseSortSpec(input);
    expect(parsed).not.toBeNull();
    expect(serializeSortSpec(parsed!)).toBe(input);
  });
});

describe("SORT_FIELDS", () => {
  it("matches the Go whitelist", () => {
    // If this test fails, the Go side (pkg/sortspec/whitelist.go)
    // and the TS side have drifted. Update both in lockstep.
    expect(SORT_FIELDS).toEqual([
      "title",
      "author",
      "series",
      "date_added",
      "date_released",
      "page_count",
      "duration",
    ]);
  });
});

describe("BUILTIN_DEFAULT_SORT", () => {
  it("is date_added:desc", () => {
    expect(BUILTIN_DEFAULT_SORT).toEqual([
      { field: "date_added", direction: "desc" },
    ]);
    expect(serializeSortSpec(BUILTIN_DEFAULT_SORT)).toBe("date_added:desc");
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test -- app/lib/sortSpec.test.ts`
Expected: FAIL — file not found.

- [ ] **Step 3: Write the implementation**

`app/lib/sortSpec.ts`:

```typescript
/**
 * Parses, validates, and serializes gallery sort specs.
 * Mirrors pkg/sortspec (Go). Keep SORT_FIELDS and the grammar in sync
 * with the Go whitelist — a matching test in the Go side pins the list.
 */

export type SortDirection = "asc" | "desc";

export type SortField =
  | "title"
  | "author"
  | "series"
  | "date_added"
  | "date_released"
  | "page_count"
  | "duration";

export interface SortLevel {
  field: SortField;
  direction: SortDirection;
}

/** Canonical list of sort fields in UI display order. */
export const SORT_FIELDS: readonly SortField[] = [
  "title",
  "author",
  "series",
  "date_added",
  "date_released",
  "page_count",
  "duration",
] as const;

/** Human-readable labels for each field. */
export const SORT_FIELD_LABELS: Record<SortField, string> = {
  title: "Title",
  author: "Author",
  series: "Series",
  date_added: "Date added",
  date_released: "Date released",
  page_count: "Page count",
  duration: "Duration",
};

/** Hard cap matching pkg/sortspec.MaxLevels. */
export const MAX_SORT_LEVELS = 10;

/** Builtin default when the URL has no sort and the DB has no saved default. */
export const BUILTIN_DEFAULT_SORT: readonly SortLevel[] = [
  { field: "date_added", direction: "desc" },
];

const FIELD_SET = new Set<string>(SORT_FIELDS);

function isSortField(s: string): s is SortField {
  return FIELD_SET.has(s);
}

function isSortDirection(s: string): s is SortDirection {
  return s === "asc" || s === "desc";
}

/**
 * Parse a serialized sort spec. Returns null for any invalid input —
 * callers should treat that as "no sort specified" and fall back.
 */
export function parseSortSpec(s: string): SortLevel[] | null {
  if (!s) return null;
  if (/\s/.test(s)) return null;

  const parts = s.split(",");
  if (parts.length > MAX_SORT_LEVELS) return null;

  const levels: SortLevel[] = [];
  const seen = new Set<string>();

  for (const part of parts) {
    if (!part) return null;
    const [field, direction] = part.split(":", 2);
    if (!field || !direction) return null;
    if (!isSortField(field)) return null;
    if (!isSortDirection(direction)) return null;
    if (seen.has(field)) return null;
    seen.add(field);
    levels.push({ field, direction });
  }

  return levels;
}

/** Serialize a spec back into the URL-param form. */
export function serializeSortSpec(levels: readonly SortLevel[]): string {
  return levels.map((l) => `${l.field}:${l.direction}`).join(",");
}

/** Deep equality for two specs (order matters). */
export function sortSpecsEqual(
  a: readonly SortLevel[] | null | undefined,
  b: readonly SortLevel[] | null | undefined,
): boolean {
  if (!a || !b) return !a && !b;
  if (a.length !== b.length) return false;
  return a.every((l, i) => l.field === b[i].field && l.direction === b[i].direction);
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm test -- app/lib/sortSpec.test.ts`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add app/lib/sortSpec.ts app/lib/sortSpec.test.ts
git commit -m "[Frontend] Add TS sortSpec parser mirroring Go package"
```

---

### Task 17: TanStack Query hook for library settings

**Files:**
- Create: `app/hooks/queries/librarySettings.ts`

- [ ] **Step 1: Write the hooks**

```typescript
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { API, ShishoAPIError } from "@/libraries/api";

export interface LibrarySettings {
  sort_spec: string | null;
}

export interface UpdateLibrarySettingsPayload {
  sort_spec: string | null;
}

export enum QueryKey {
  LibrarySettings = "LibrarySettings",
}

export const useLibrarySettings = (
  libraryId: number,
  options: Omit<
    UseQueryOptions<LibrarySettings, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<LibrarySettings, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.LibrarySettings, libraryId],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/settings/libraries/${libraryId}`,
        null,
        null,
        signal,
      );
    },
  });
};

export const useUpdateLibrarySettings = (libraryId: number) => {
  const queryClient = useQueryClient();

  return useMutation<
    LibrarySettings,
    ShishoAPIError,
    UpdateLibrarySettingsPayload
  >({
    mutationFn: (payload) => {
      return API.request(
        "PUT",
        `/settings/libraries/${libraryId}`,
        payload,
        null,
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.LibrarySettings, libraryId], data);
      // The gallery ordering may change; invalidate the books cache.
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
    },
  });
};
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `pnpm lint:types`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add app/hooks/queries/librarySettings.ts
git commit -m "[Frontend] Add library settings query hooks"
```

---

### Task 18: SortedByChips component

**Files:**
- Create: `app/components/library/SortedByChips.tsx`
- Create: `app/components/library/SortedByChips.test.tsx`

- [ ] **Step 1: Write failing tests**

```typescript
import SortedByChips from "./SortedByChips";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { SortLevel } from "@/lib/sortSpec";

describe("SortedByChips", () => {
  it("renders a chip per level", () => {
    const levels: SortLevel[] = [
      { field: "author", direction: "asc" },
      { field: "series", direction: "asc" },
    ];
    render(
      <SortedByChips
        levels={levels}
        onRemoveLevel={vi.fn()}
        onReset={vi.fn()}
      />,
    );
    expect(screen.getByText(/Author/)).toBeInTheDocument();
    expect(screen.getByText(/Series/)).toBeInTheDocument();
  });

  it("renders ascending arrow for asc and descending for desc", () => {
    render(
      <SortedByChips
        levels={[
          { field: "title", direction: "asc" },
          { field: "date_added", direction: "desc" },
        ]}
        onRemoveLevel={vi.fn()}
        onReset={vi.fn()}
      />,
    );
    // At minimum, the arrow characters should be on the page.
    expect(screen.getByText(/↑/)).toBeInTheDocument();
    expect(screen.getByText(/↓/)).toBeInTheDocument();
  });

  it("clicking a chip removes its level", async () => {
    const user = userEvent.setup();
    const onRemoveLevel = vi.fn();
    render(
      <SortedByChips
        levels={[
          { field: "title", direction: "asc" },
          { field: "author", direction: "asc" },
        ]}
        onRemoveLevel={onRemoveLevel}
        onReset={vi.fn()}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Title/i }));
    expect(onRemoveLevel).toHaveBeenCalledWith(0);
  });

  it("clicking reset fires onReset", async () => {
    const user = userEvent.setup();
    const onReset = vi.fn();
    render(
      <SortedByChips
        levels={[{ field: "title", direction: "asc" }]}
        onRemoveLevel={vi.fn()}
        onReset={onReset}
      />,
    );
    await user.click(screen.getByRole("button", { name: /reset to default/i }));
    expect(onReset).toHaveBeenCalled();
  });

  it("renders nothing when levels is empty", () => {
    const { container } = render(
      <SortedByChips
        levels={[]}
        onRemoveLevel={vi.fn()}
        onReset={vi.fn()}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm test -- app/components/library/SortedByChips.test.tsx`
Expected: FAIL — component not found.

- [ ] **Step 3: Write the component**

`app/components/library/SortedByChips.tsx`:

```typescript
import { ArrowDown, ArrowUp, X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import {
  SORT_FIELD_LABELS,
  type SortLevel,
} from "@/lib/sortSpec";

interface SortedByChipsProps {
  levels: readonly SortLevel[];
  onRemoveLevel: (index: number) => void;
  onReset: () => void;
}

const SortedByChips = ({
  levels,
  onRemoveLevel,
  onReset,
}: SortedByChipsProps) => {
  if (levels.length === 0) {
    return null;
  }

  return (
    <div className="flex flex-wrap items-center gap-2 py-2">
      <span className="text-sm text-muted-foreground">Sorted by:</span>

      {levels.map((level, index) => {
        const label = SORT_FIELD_LABELS[level.field];
        const Arrow = level.direction === "asc" ? ArrowUp : ArrowDown;
        return (
          <button
            key={level.field}
            type="button"
            onClick={() => onRemoveLevel(index)}
            aria-label={`${label} ${level.direction === "asc" ? "ascending" : "descending"} — click to remove`}
            className="group"
          >
            <Badge
              variant="secondary"
              className="gap-1 cursor-pointer hover:bg-destructive/20 transition-colors"
            >
              <span>{label}</span>
              <Arrow className="h-3 w-3" />
              <X className="h-3 w-3 opacity-60 group-hover:opacity-100" />
            </Badge>
          </button>
        );
      })}

      <button
        type="button"
        onClick={onReset}
        className="text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground"
      >
        reset to default
      </button>
    </div>
  );
};

export default SortedByChips;
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm test -- app/components/library/SortedByChips.test.tsx`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add app/components/library/SortedByChips.tsx app/components/library/SortedByChips.test.tsx
git commit -m "[Frontend] Add SortedByChips component"
```

---

### Task 19: SortSheet component

**Files:**
- Create: `app/components/library/SortSheet.tsx`
- Create: `app/components/library/SortSheet.test.tsx`

- [ ] **Step 1: Write the component**

This is the largest single component in the plan. It uses `@dnd-kit/sortable` for drag-reorder. Because `@dnd-kit` is non-trivial to test end-to-end, tests focus on the observable behavior (add level, remove level, toggle direction, save-as-default) via the callbacks.

`app/components/library/SortSheet.tsx`:

```typescript
import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  ArrowDown,
  ArrowDownUp,
  ArrowUp,
  GripVertical,
  Plus,
  Save,
  X,
} from "lucide-react";
import { useMediaQuery } from "@uidotdev/usehooks";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@/components/ui/drawer";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  SORT_FIELD_LABELS,
  SORT_FIELDS,
  type SortField,
  type SortLevel,
} from "@/lib/sortSpec";

interface SortSheetProps {
  levels: readonly SortLevel[];
  onChange: (levels: SortLevel[]) => void;
  onSaveAsDefault: () => void;
  isDirty: boolean;
  isSaving: boolean;
}

export const SortButton = ({
  isDirty,
  onClick,
}: {
  isDirty: boolean;
  onClick?: () => void;
}) => (
  <Button variant="outline" size="sm" onClick={onClick} className="relative gap-2">
    <ArrowDownUp className="h-4 w-4" />
    <span>Sort</span>
    {isDirty && (
      <span
        className="absolute -right-1 -top-1 h-2 w-2 rounded-full bg-primary"
        aria-label="Sort differs from default"
      />
    )}
  </Button>
);

const SortLevelRow = ({
  level,
  index,
  usedFields,
  onChangeField,
  onToggleDirection,
  onRemove,
}: {
  level: SortLevel;
  index: number;
  usedFields: ReadonlySet<SortField>;
  onChangeField: (field: SortField) => void;
  onToggleDirection: () => void;
  onRemove: () => void;
}) => {
  const { attributes, listeners, setNodeRef, transform, transition } =
    useSortable({ id: level.field });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  const Arrow = level.direction === "asc" ? ArrowUp : ArrowDown;

  // Fields available in the dropdown = this row's current field + any unused field.
  const available = SORT_FIELDS.filter(
    (f) => f === level.field || !usedFields.has(f),
  );

  return (
    <div
      ref={setNodeRef}
      style={style}
      className="flex items-center gap-2 rounded-md border p-2"
    >
      <button
        type="button"
        aria-label={`Reorder sort level ${index + 1}`}
        {...attributes}
        {...listeners}
        className="cursor-grab text-muted-foreground hover:text-foreground"
      >
        <GripVertical className="h-4 w-4" />
      </button>

      <Select value={level.field} onValueChange={(v) => onChangeField(v as SortField)}>
        <SelectTrigger className="flex-1">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {available.map((f) => (
            <SelectItem key={f} value={f}>
              {SORT_FIELD_LABELS[f]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Button
        variant="outline"
        size="icon"
        onClick={onToggleDirection}
        aria-label={`Direction: ${level.direction === "asc" ? "ascending" : "descending"}. Click to toggle.`}
      >
        <Arrow className="h-4 w-4" />
      </Button>

      <Button
        variant="ghost"
        size="icon"
        onClick={onRemove}
        aria-label={`Remove ${SORT_FIELD_LABELS[level.field]} sort level`}
      >
        <X className="h-4 w-4" />
      </Button>
    </div>
  );
};

const SortSheetContent = ({
  levels,
  onChange,
  onSaveAsDefault,
  isDirty,
  isSaving,
}: SortSheetProps) => {
  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const usedFields = new Set(levels.map((l) => l.field));
  const unused = SORT_FIELDS.filter((f) => !usedFields.has(f));

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = levels.findIndex((l) => l.field === active.id);
    const newIndex = levels.findIndex((l) => l.field === over.id);
    onChange(arrayMove([...levels], oldIndex, newIndex));
  };

  const addLevel = (field: SortField) => {
    onChange([...levels, { field, direction: "asc" }]);
  };

  const changeField = (index: number, field: SortField) => {
    const next = [...levels];
    next[index] = { ...next[index], field };
    onChange(next);
  };

  const toggleDirection = (index: number) => {
    const next = [...levels];
    next[index] = {
      ...next[index],
      direction: next[index].direction === "asc" ? "desc" : "asc",
    };
    onChange(next);
  };

  const removeLevel = (index: number) => {
    onChange(levels.filter((_, i) => i !== index));
  };

  return (
    <div className="space-y-4">
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={handleDragEnd}
      >
        <SortableContext
          items={levels.map((l) => l.field)}
          strategy={verticalListSortingStrategy}
        >
          <div className="space-y-2">
            {levels.map((level, index) => (
              <SortLevelRow
                key={level.field}
                level={level}
                index={index}
                usedFields={usedFields}
                onChangeField={(f) => changeField(index, f)}
                onToggleDirection={() => toggleDirection(index)}
                onRemove={() => removeLevel(index)}
              />
            ))}
          </div>
        </SortableContext>
      </DndContext>

      {unused.length > 0 && (
        <div>
          <p className="mb-2 text-sm text-muted-foreground">
            {levels.length === 0 ? "Sort by…" : "Then by…"}
          </p>
          <div className="flex flex-wrap gap-2">
            {unused.map((f) => (
              <Button
                key={f}
                variant="outline"
                size="sm"
                onClick={() => addLevel(f)}
                className="gap-1"
              >
                <Plus className="h-3 w-3" />
                {SORT_FIELD_LABELS[f]}
              </Button>
            ))}
          </div>
        </div>
      )}

      {isDirty && (
        <div className="rounded-md border border-dashed p-3">
          <p className="mb-2 text-sm">
            Save this as the default for this library?
          </p>
          <Button
            onClick={onSaveAsDefault}
            disabled={isSaving}
            size="sm"
            className="gap-1"
          >
            <Save className="h-3 w-3" />
            {isSaving ? "Saving…" : "Save as default"}
          </Button>
        </div>
      )}
    </div>
  );
};

const SortSheet = ({
  trigger,
  ...props
}: SortSheetProps & { trigger: React.ReactNode }) => {
  const [open, setOpen] = useState(false);
  const isDesktop = useMediaQuery("(min-width: 768px)");

  if (isDesktop) {
    return (
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger asChild>{trigger}</SheetTrigger>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>Sort</SheetTitle>
          </SheetHeader>
          <div className="mt-4">
            <SortSheetContent {...props} />
          </div>
        </SheetContent>
      </Sheet>
    );
  }

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>{trigger}</DrawerTrigger>
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>Sort</DrawerTitle>
        </DrawerHeader>
        <div className="px-4 pb-4">
          <SortSheetContent {...props} />
        </div>
      </DrawerContent>
    </Drawer>
  );
};

export default SortSheet;
```

- [ ] **Step 2: Write tests**

`app/components/library/SortSheet.test.tsx`:

```typescript
import SortSheet from "./SortSheet";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Button } from "@/components/ui/button";
import type { SortLevel } from "@/lib/sortSpec";

const Trigger = () => <Button>Open Sort</Button>;

describe("SortSheet", () => {
  it("adds a sort level when 'Then by' button clicked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <SortSheet
        trigger={<Trigger />}
        levels={[]}
        onChange={onChange}
        onSaveAsDefault={vi.fn()}
        isDirty={false}
        isSaving={false}
      />,
    );

    await user.click(screen.getByText("Open Sort"));
    await user.click(screen.getByRole("button", { name: /Title/i }));

    expect(onChange).toHaveBeenCalledWith([
      { field: "title", direction: "asc" },
    ]);
  });

  it("toggles direction when arrow button clicked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const levels: SortLevel[] = [{ field: "title", direction: "asc" }];

    render(
      <SortSheet
        trigger={<Trigger />}
        levels={levels}
        onChange={onChange}
        onSaveAsDefault={vi.fn()}
        isDirty={true}
        isSaving={false}
      />,
    );

    await user.click(screen.getByText("Open Sort"));
    await user.click(screen.getByLabelText(/Direction: ascending/i));

    expect(onChange).toHaveBeenCalledWith([
      { field: "title", direction: "desc" },
    ]);
  });

  it("removes a level when X clicked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const levels: SortLevel[] = [
      { field: "title", direction: "asc" },
      { field: "author", direction: "asc" },
    ];

    render(
      <SortSheet
        trigger={<Trigger />}
        levels={levels}
        onChange={onChange}
        onSaveAsDefault={vi.fn()}
        isDirty={true}
        isSaving={false}
      />,
    );

    await user.click(screen.getByText("Open Sort"));
    await user.click(screen.getByLabelText(/Remove Title sort level/i));

    expect(onChange).toHaveBeenCalledWith([
      { field: "author", direction: "asc" },
    ]);
  });

  it("fires onSaveAsDefault when Save clicked", async () => {
    const user = userEvent.setup();
    const onSaveAsDefault = vi.fn();

    render(
      <SortSheet
        trigger={<Trigger />}
        levels={[{ field: "title", direction: "asc" }]}
        onChange={vi.fn()}
        onSaveAsDefault={onSaveAsDefault}
        isDirty={true}
        isSaving={false}
      />,
    );

    await user.click(screen.getByText("Open Sort"));
    await user.click(screen.getByRole("button", { name: /Save as default/i }));

    expect(onSaveAsDefault).toHaveBeenCalled();
  });

  it("hides Save as default when not dirty", async () => {
    const user = userEvent.setup();

    render(
      <SortSheet
        trigger={<Trigger />}
        levels={[{ field: "title", direction: "asc" }]}
        onChange={vi.fn()}
        onSaveAsDefault={vi.fn()}
        isDirty={false}
        isSaving={false}
      />,
    );

    await user.click(screen.getByText("Open Sort"));

    expect(
      screen.queryByRole("button", { name: /Save as default/i }),
    ).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run tests**

Run: `pnpm test -- app/components/library/SortSheet.test.tsx`
Expected: All PASS. (If `@uidotdev/usehooks` isn't already installed, check whether `FilterSheet.tsx` uses the same media-query hook; if it does, reuse that import. If not, fall back to `window.matchMedia` directly.)

- [ ] **Step 4: Commit**

```bash
git add app/components/library/SortSheet.tsx app/components/library/SortSheet.test.tsx
git commit -m "[Frontend] Add SortSheet component"
```

---

### Task 20: Integrate sort state into Home page

**Files:**
- Modify: `app/components/pages/Home.tsx`

- [ ] **Step 1: Read the current Home.tsx to understand existing imports and state shape**

Run: `wc -l app/components/pages/Home.tsx` and read the file. Identify:
- The top-level `HomeContent` component boundary.
- Where `searchParams` is already read.
- Where `useBooks(...)` is called.
- Where existing filter chips (`ActiveFilterChips`) are rendered — the SortedByChips row should sit right next to or below them.
- Where the Filters button is rendered — the Sort button goes next to it.

- [ ] **Step 2: Add sort state derivation**

Near the top of `HomeContent`, after the existing `searchParams` reads, add:

```typescript
import SortedByChips from "@/components/library/SortedByChips";
import SortSheet, { SortButton } from "@/components/library/SortSheet";
import { useLibrarySettings, useUpdateLibrarySettings } from "@/hooks/queries/librarySettings";
import {
  BUILTIN_DEFAULT_SORT,
  parseSortSpec,
  serializeSortSpec,
  sortSpecsEqual,
  type SortLevel,
} from "@/lib/sortSpec";
```

Inside `HomeContent`:

```typescript
const sortParam = searchParams.get("sort") ?? "";
const urlSort = sortParam ? parseSortSpec(sortParam) : null;

// Fetch stored per-library preference. Keep previousData to avoid a
// flicker on param changes.
const librarySettingsQuery = useLibrarySettings(libraryIdNum, {
  enabled: libraryIdNum != null,
});

// Resolve the effective sort:
//  1. URL wins if valid.
//  2. Else stored preference if set and valid.
//  3. Else builtin default.
const storedSort: SortLevel[] | null = librarySettingsQuery.data?.sort_spec
  ? parseSortSpec(librarySettingsQuery.data.sort_spec)
  : null;
const defaultSort: readonly SortLevel[] =
  storedSort && storedSort.length > 0 ? storedSort : BUILTIN_DEFAULT_SORT;
const effectiveSort: readonly SortLevel[] = urlSort && urlSort.length > 0 ? urlSort : defaultSort;

// "Dirty" = URL sort differs from the resolved default.
const isSortDirty = urlSort !== null && !sortSpecsEqual(urlSort, defaultSort);

// Block the initial gallery render until the library-settings query
// has resolved, so we don't flash a wrong default order.
const settingsResolved =
  libraryIdNum == null ||
  librarySettingsQuery.isSuccess ||
  librarySettingsQuery.isError;
```

- [ ] **Step 3: Pass `sort` into useBooks**

Locate the existing `useBooks({ ... })` call. Add:

```typescript
const booksQuery = useBooks(
  {
    // ... existing fields ...
    sort: serializeSortSpec(effectiveSort) || undefined,
  },
  {
    // Gate the query on settingsResolved so we don't fire until the
    // stored preference is known. This is the "block gallery render"
    // requirement from the spec.
    enabled: settingsResolved && /* existing enabled conditions */ true,
  },
);
```

- [ ] **Step 4: Add Sort button to the toolbar**

Locate the existing `FilterButton` (or whatever renders the Filters UI) in the toolbar and add next to it:

```tsx
<SortSheet
  trigger={<SortButton isDirty={isSortDirty} />}
  levels={effectiveSort}
  onChange={(next) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      const serialized = serializeSortSpec(next);
      if (serialized && !sortSpecsEqual(next, defaultSort)) {
        params.set("sort", serialized);
      } else {
        params.delete("sort");
      }
      params.set("page", "1");
      return params;
    });
  }}
  onSaveAsDefault={() => {
    const serialized = serializeSortSpec(effectiveSort);
    updateSettings.mutate(
      { sort_spec: serialized || null },
      {
        onSuccess: () => {
          setSearchParams((prev) => {
            const params = new URLSearchParams(prev);
            params.delete("sort");
            return params;
          });
        },
      },
    );
  }}
  isDirty={isSortDirty}
  isSaving={updateSettings.isPending}
/>
```

where `updateSettings` is:

```typescript
const updateSettings = useUpdateLibrarySettings(libraryIdNum ?? 0);
```

(Guard: only call `mutate` when `libraryIdNum` is non-null.)

- [ ] **Step 5: Add SortedByChips row**

Next to or below `ActiveFilterChips`:

```tsx
<SortedByChips
  levels={isSortDirty ? effectiveSort : []}
  onRemoveLevel={(index) => {
    const next = effectiveSort.filter((_, i) => i !== index);
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      const serialized = serializeSortSpec(next);
      if (serialized && !sortSpecsEqual(next, defaultSort)) {
        params.set("sort", serialized);
      } else {
        params.delete("sort");
      }
      params.set("page", "1");
      return params;
    });
  }}
  onReset={() => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      params.delete("sort");
      return params;
    });
  }}
/>
```

- [ ] **Step 6: Render nothing from the gallery until `settingsResolved` is true**

Locate where the book grid currently renders (probably inside `{booksQuery.data && ...}`). Gate the gallery on `settingsResolved`:

```tsx
{!settingsResolved ? (
  <div className="flex min-h-[300px] items-center justify-center">
    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
  </div>
) : (
  /* existing gallery render */
)}
```

- [ ] **Step 7: Type-check and build**

Run: `pnpm lint:types && pnpm build`
Expected: Both pass.

- [ ] **Step 8: Smoke-test locally**

Run: `mise start` (API + Vite). Open the gallery at `/libraries/:id` and verify:
1. No sort param → gallery renders in `date_added:desc` order (the builtin default), no dot, no chip row.
2. Click Sort → sheet opens → add level → URL updates with `?sort=…`, chip row appears, dot on button.
3. Click a chip → chip disappears.
4. Click "Save as default" → URL clears, dot disappears, reload → new default sticks.
5. Click "reset to default" → URL clears, builtin/stored default order returns.

- [ ] **Step 9: Commit**

```bash
git add app/components/pages/Home.tsx
git commit -m "[Frontend] Wire sort sheet, chips, and URL state into Home"
```

---

## Phase 9: E2E test

### Task 21: Playwright E2E coverage

**Files:**
- Create: `e2e/gallery-sort.spec.ts`

- [ ] **Step 1: Read existing E2E patterns**

Glance at an existing E2E spec under `e2e/` to mirror the fixture-setup pattern (login flow, library creation, book seeding). E.g. look at `e2e/gallery-filter.spec.ts` if it exists, or `e2e/books.spec.ts`.

- [ ] **Step 2: Write the E2E spec**

```typescript
import { expect, test } from "@playwright/test";

test.describe("Gallery sort", () => {
  // Assumes an existing fixture with: one logged-in user, one library
  // with ≥3 books differing in title and created_at. See the repo's
  // existing E2E fixtures for the exact setup helpers.

  test("default load shows builtin sort, no chip row, no dot", async ({
    page,
  }) => {
    await page.goto("/libraries/1");
    await expect(page.getByText(/Sorted by:/)).not.toBeVisible();
    // No dot on the sort button.
    await expect(
      page.getByRole("button", { name: /^Sort$/ }),
    ).not.toHaveClass(/relative/); // dot is inside; this is a coarse check
  });

  test("selecting a sort updates URL and shows dot + chip row", async ({
    page,
  }) => {
    await page.goto("/libraries/1");
    await page.getByRole("button", { name: /^Sort$/ }).click();
    await page.getByRole("button", { name: /^Title$/ }).click();
    await expect(page).toHaveURL(/[?&]sort=title:asc/);
    await expect(page.getByText(/Sorted by:/)).toBeVisible();
    await expect(page.getByRole("button", { name: /^Title/ })).toBeVisible();
  });

  test("clicking a Sorted-by chip removes that level", async ({ page }) => {
    await page.goto("/libraries/1?sort=title:asc");
    await expect(page.getByText(/Sorted by:/)).toBeVisible();
    await page.getByRole("button", { name: /^Title/ }).click();
    await expect(page).not.toHaveURL(/[?&]sort=/);
    await expect(page.getByText(/Sorted by:/)).not.toBeVisible();
  });

  test("save as default persists and clears URL", async ({ page }) => {
    await page.goto("/libraries/1?sort=title:asc");
    await page.getByRole("button", { name: /^Sort$/ }).click();
    await page.getByRole("button", { name: /Save as default/ }).click();

    await expect(page).not.toHaveURL(/[?&]sort=/);
    await expect(page.getByText(/Sorted by:/)).not.toBeVisible();

    // Reload — the saved preference should render books in title order
    // without a `?sort=` param.
    await page.reload();
    await expect(page).not.toHaveURL(/[?&]sort=/);
    await expect(page.getByText(/Sorted by:/)).not.toBeVisible();
  });

  test("reset to default clears URL", async ({ page }) => {
    await page.goto("/libraries/1?sort=title:desc");
    await expect(page.getByText(/Sorted by:/)).toBeVisible();
    await page.getByRole("button", { name: /reset to default/i }).click();
    await expect(page).not.toHaveURL(/[?&]sort=/);
  });
});
```

- [ ] **Step 3: Run the E2E**

Run: `mise test:e2e -- gallery-sort.spec.ts`
Expected: All PASS in both Chromium and Firefox.

Note: if existing E2E fixtures don't match the assumptions above (e.g. no library "1" or no pre-seeded books), adapt the `beforeAll` / `beforeEach` setup to match what this repo's helpers provide.

- [ ] **Step 4: Commit**

```bash
git add e2e/gallery-sort.spec.ts
git commit -m "[E2E] Add gallery sort coverage"
```

---

## Phase 10: Docs

### Task 22: User-facing documentation

Per `CLAUDE.md` (root) — user-facing changes MUST update or create the corresponding page in `website/docs/`. This feature ships a new gallery UI (Sort button + sheet + chip row), a new settings endpoint, and new behavior on OPDS / eReader feeds, so three docs touchpoints apply.

**Files:**
- Create: `website/docs/gallery-sort.md`
- Modify: `website/docs/opds.md`
- Modify: `website/docs/ereader-browser.md`

- [ ] **Step 1: Create `website/docs/gallery-sort.md`**

Place it at the end of the sidebar (`sidebar_position: 20`) since it's a secondary UI feature. Exact content:

```markdown
---
sidebar_position: 20
---

# Gallery sort

The library gallery can be sorted by one or more fields — author, series, date added, and more — with each level ascending or descending. You can share a sorted view as a URL, save it as your default for that library, or reset back to the builtin default.

## Using the Sort sheet

Click **Sort** in the gallery toolbar to open the sort sheet. The sheet shows the current sort levels in priority order (leftmost = primary). From the sheet you can:

- **Add a level** — click any "Then by…" button at the bottom of the sheet.
- **Remove a level** — click the × on its row.
- **Change direction** — click the ↑ / ↓ button on its row to flip ascending/descending.
- **Reorder levels** — drag the grip handle on the left edge of each row.

A dot on the Sort button means your current sort differs from your saved default.

## Available sort fields

| Field | Sorts by |
|-------|----------|
| Title | Book title |
| Author | Primary author's name (books with no author sort to the end) |
| Series | Primary series name, then series number within each series |
| Date added | When the book was first scanned into the library |
| Date released | Release date from the primary file's metadata |
| Page count | Page count from the primary file |
| Duration | Audiobook duration from the primary file |

For Date released, Page count, and Duration: if the primary file doesn't have a value, Shisho falls back to any other file on the book that does. Books with no value on any of their files sort to the end, regardless of ascending/descending.

## URL-addressable sorts

Non-default sorts live in the URL as `?sort=field:dir,field:dir`. You can share or bookmark a sorted view and it reloads in the same order.

Examples:

```
?sort=author:asc
?sort=author:asc,series:asc
?sort=date_added:desc
```

When the URL has no `sort` parameter, the gallery uses your saved default for that library (or the builtin default — **Date added, newest first** — if you haven't saved one).

## Saving a default for this library

When your current sort differs from your saved default, the sort sheet shows a **Save as default** button. Clicking it:

1. Saves your current sort as the new default for this library.
2. Clears the `?sort=` parameter from the URL — you're now viewing the default.

Defaults are per-user per-library, so each library can have its own saved sort and each user's saved sorts are their own.

## Resetting to the default

When a non-default sort is active, a **reset to default** link appears next to the sort chip row. Clicking it clears the `?sort=` parameter and returns the gallery to your saved default.

## How this affects other surfaces

Your saved library sort also applies to the OPDS feeds and the eReader browser for that library. See [OPDS](./opds.md) and [eReader browser](./ereader-browser.md) for details.
```

- [ ] **Step 2: Add a section to `website/docs/opds.md`**

At the end of the file (above any closing pages), append:

```markdown
## Sort order

Book-listing feeds (library, series, author, genre, tag, all-books, recently-added) apply the authenticated user's saved default sort for the relevant library. See [Gallery sort](./gallery-sort.md) for how to set a default.

When a feed is not scoped to a single library (for example, the all-books root feed across a user's libraries), Shisho falls back to its builtin default order — `user_library_settings` is per-library by design.
```

- [ ] **Step 3: Add a section to `website/docs/ereader-browser.md`**

Append at the end:

```markdown
## Sort order

The eReader browser's book listings (library index, author, series, genre pages) apply the authenticated user's saved default sort for the relevant library. See [Gallery sort](./gallery-sort.md) for how to set a default.
```

- [ ] **Step 4: Preview the docs locally**

Run: `mise docs`
Expected: dev server starts, visit `http://localhost:3000/docs/gallery-sort` and confirm the page renders with working cross-links to OPDS and eReader pages.

- [ ] **Step 5: Commit**

```bash
git add website/docs/gallery-sort.md website/docs/opds.md website/docs/ereader-browser.md
git commit -m "[Docs] Document gallery sort feature"
```

---

## Phase 11: Final check

### Task 23: Full validation

- [ ] **Step 1: Run the full check suite**

Run: `mise check:quiet`
Expected: PASS. If anything fails, fix the underlying issue (don't rerun blindly — the output tells you what's broken).

- [ ] **Step 2: Manual smoke test**

Start the app: `mise start`. Walk through the flow one more time with fresh eyes:

1. Gallery loads with `date_added:desc` order when no preference saved.
2. Opening Sort sheet and picking a single field updates URL.
3. Adding a second level works.
4. Drag-reorder changes URL ordering.
5. Toggling direction flips the arrow and re-sorts.
6. Saving as default clears URL and persists across reload.
7. OPDS feed (browse at `/opds/v1/epub/catalog` in a new incognito, Basic-Auth as the same user): confirms saved preference is reflected in the `<atom:entry>` order.
8. eReader browser at `/ereader/key/:apiKey/libraries/:id/all`: same — saved preference reflected.

- [ ] **Step 3: Final commit (if any polish changes came out of smoke test)**

If the smoke test surfaces no changes, this task is complete. Otherwise, fix inline and commit.

---

## Cross-cutting reminders

- **Every Go test gets `t.Parallel()` as the first line** per `CLAUDE.md`. The helper functions (`setupTestDB`, etc.) should use `t.Helper()`.
- **Bun query aliases**: in any WHERE/ORDER clause, use `b` for books, `f` for files, `s` for series, `p` for persons, `a` for authors — never the full table name.
- **JSON field names are `snake_case`**. Go struct tags use `json:"snake_case"`.
- **Request bodies bind to structs**, never slices.
- **Generated types** (`app/types/generated/`) are gitignored — don't try to commit them.
- **Docs update**: handled in Task 22 — new `website/docs/gallery-sort.md` page plus short additions to `opds.md` and `ereader-browser.md` so users know their saved sort applies on those surfaces.
