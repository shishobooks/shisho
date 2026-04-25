# Identifier Uniqueness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce "at most one identifier per type per file" coherently from the API down through the manual-edit UI and identify-merge flow, replacing today's silent-drop behavior.

**Architecture:** The DB already has `UNIQUE(file_id, type)`. This plan adds an explicit duplicate-type rejection at the `updateFile` API, defensive type-dedupe + warn logging in `BulkCreateFileIdentifiers`, source preservation for unchanged rows, type-keyed merging in the frontend identify flow, and a non-selectable dropdown for already-present types in the manual edit dialog. The plugin metadata persistence path is refactored to use the same bulk insert so it inherits the dedupe defense, and the now-unused `CreateFileIdentifier` is deleted.

**Tech Stack:** Go 1.x with Bun ORM and Echo framework, React 19 + TypeScript with Tanstack Query and Vitest, Radix UI Select/Tooltip primitives.

**Background reading:** `docs/superpowers/specs/2026-04-25-identifier-uniqueness-design.md`

---

## File Map

**Modify (backend):**
- `pkg/books/service.go` — `BulkCreateFileIdentifiers` adds dedupe + warn log; `CreateFileIdentifier` deleted.
- `pkg/books/handlers.go` — `updateFile` adds duplicate-type validation, switches to bulk insert, preserves source for unchanged rows.
- `pkg/plugins/handler.go` — `identifierStore` interface gets `BulkCreateFileIdentifiers`, drops `CreateFileIdentifier`.
- `pkg/plugins/handler_persist_metadata.go` — replaces per-item create loop with single bulk insert.
- `pkg/server/server.go` — `bookUpdaterAdapter` exposes `BulkCreateFileIdentifiers`, drops `CreateFileIdentifier`.

**Create (backend tests):**
- `pkg/books/service_identifiers_test.go` — `BulkCreateFileIdentifiers` dedupe + warn coverage.

**Modify (backend tests):**
- `pkg/books/handlers_test.go` (or appropriate file) — `updateFile` duplicate-type rejection + source preservation.
- `pkg/plugins/` tests covering `persistMetadata` — duplicate-type input does not crash.

**Modify (frontend):**
- `app/components/library/identify-utils.ts` — `resolveIdentifiers` keys by type, incoming wins, intra-incoming dedupe.
- `app/components/library/identify-utils.test.ts` — replace smoke test with full coverage.
- `app/components/library/IdentifyReviewForm.test.ts` — update tests for new merge behavior.
- `app/components/library/FileEditDialog.tsx` — disable already-present types in the add-identifier `Select`, with `Tooltip`.
- `app/components/library/FileEditDialog.test.tsx` — assert disabled-and-tooltip behavior.

**Modify (docs):**
- `website/docs/metadata.md` — type-uniqueness rule and identify-replaces-on-conflict note.

---

## Task 1: Frontend `resolveIdentifiers` — dedupe by type, incoming wins

**Files:**
- Modify: `app/components/library/identify-utils.ts`
- Test: `app/components/library/identify-utils.test.ts`
- Test (other): `app/components/library/IdentifyReviewForm.test.ts`

- [ ] **Step 1: Replace the existing tests with new coverage.**

In `app/components/library/identify-utils.test.ts`, replace the entire `describe("resolveIdentifiers", ...)` block (lines 132-140) with:

```typescript
describe("resolveIdentifiers", () => {
  it("returns unchanged when current and incoming are identical by type and value", () => {
    const current = [
      { type: "isbn_13", value: "9780316769488" },
      { type: "asin", value: "B01ABC1234" },
    ];
    const incoming = [
      { type: "asin", value: "B01ABC1234" },
      { type: "isbn_13", value: "9780316769488" },
    ];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("unchanged");
  });

  it("returns new with deduped value when current is empty", () => {
    const incoming = [
      { type: "asin", value: "B01ABC1234" },
      { type: "asin", value: "B02DEF5678" }, // intra-incoming duplicate type
      { type: "isbn_13", value: "9780316769488" },
    ];
    const result = resolveIdentifiers([], incoming);
    expect(result.status).toBe("new");
    expect(result.value).toEqual([
      { type: "asin", value: "B02DEF5678" }, // last-wins
      { type: "isbn_13", value: "9780316769488" },
    ]);
  });

  it("returns unchanged when incoming is empty", () => {
    const current = [{ type: "asin", value: "B01ABC1234" }];
    const result = resolveIdentifiers(current, []);
    expect(result.status).toBe("unchanged");
    expect(result.value).toEqual(current);
  });

  it("returns changed and replaces existing value when incoming has same type with different value", () => {
    const current = [{ type: "asin", value: "B01ABC1234" }];
    const incoming = [{ type: "asin", value: "B02DEF5678" }];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([{ type: "asin", value: "B02DEF5678" }]);
  });

  it("returns changed and adds incoming type when current has different types", () => {
    const current = [{ type: "isbn_13", value: "9780316769488" }];
    const incoming = [{ type: "asin", value: "B01ABC1234" }];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([
      { type: "isbn_13", value: "9780316769488" },
      { type: "asin", value: "B01ABC1234" },
    ]);
  });

  it("merges by type with incoming winning on conflict and current order preserved", () => {
    const current = [
      { type: "isbn_13", value: "9780316769488" },
      { type: "asin", value: "B01ABC1234" },
    ];
    const incoming = [
      { type: "asin", value: "B02DEF5678" }, // replaces
      { type: "goodreads", value: "12345" }, // new
    ];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([
      { type: "isbn_13", value: "9780316769488" }, // unchanged, current order
      { type: "asin", value: "B02DEF5678" }, // replaced, current order
      { type: "goodreads", value: "12345" }, // new, appended
    ]);
  });

  it("dedupes intra-incoming duplicates with last-wins before merging", () => {
    const current: { type: string; value: string }[] = [];
    const incoming = [
      { type: "asin", value: "OLD" },
      { type: "asin", value: "NEW" },
    ];
    const result = resolveIdentifiers(current, incoming);
    expect(result.value).toEqual([{ type: "asin", value: "NEW" }]);
  });

  it("returns unchanged when incoming is a subset of current with matching values", () => {
    const current = [
      { type: "isbn_13", value: "9780316769488" },
      { type: "asin", value: "B01ABC1234" },
    ];
    const incoming = [{ type: "isbn_13", value: "9780316769488" }];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("unchanged");
    expect(result.value).toEqual(current);
  });
});
```

In `app/components/library/IdentifyReviewForm.test.ts`, replace the existing `describe("resolveIdentifiers", ...)` block (lines 5-58) with this — the block was a duplicate of identify-utils.test.ts and we want to keep it focused on IdentifyReviewForm-specific concerns:

```typescript
import { describe, expect, it } from "vitest";

import { resolveIdentifiers } from "./identify-utils";

describe("resolveIdentifiers (incoming wins on type conflict)", () => {
  it("replaces an existing identifier when incoming has the same type with a different value", () => {
    const current = [{ type: "asin", value: "B01ABC1234" }];
    const incoming = [{ type: "asin", value: "B02DEF5678" }];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([{ type: "asin", value: "B02DEF5678" }]);
  });
});
```

- [ ] **Step 2: Run the tests and confirm they fail.**

Run: `pnpm vitest run app/components/library/identify-utils.test.ts app/components/library/IdentifyReviewForm.test.ts`
Expected: failures, mostly on the same-type-different-value cases (current implementation keeps both entries) and on the intra-incoming dedupe case.

- [ ] **Step 3: Implement the new `resolveIdentifiers`.**

Replace lines 68-93 of `app/components/library/identify-utils.ts` with:

```typescript
export function resolveIdentifiers(
  current: IdentifierEntry[],
  incoming: IdentifierEntry[],
): { value: IdentifierEntry[]; status: FieldStatus } {
  // Dedupe incoming by type (last-wins) so a misbehaving plugin can't propagate
  // a duplicate-type set forward. The DB invariant is one identifier per type
  // per file; "incoming wins on conflict" extends naturally to "the last
  // incoming entry wins" within the same payload.
  const dedupedIncoming: IdentifierEntry[] = [];
  const incomingByType = new Map<string, number>();
  for (const entry of incoming) {
    const existingIdx = incomingByType.get(entry.type);
    if (existingIdx === undefined) {
      incomingByType.set(entry.type, dedupedIncoming.length);
      dedupedIncoming.push(entry);
    } else {
      dedupedIncoming[existingIdx] = entry;
    }
  }

  if (current.length === 0 && dedupedIncoming.length === 0) {
    return { value: [], status: "unchanged" };
  }
  if (current.length === 0) {
    return { value: dedupedIncoming, status: "new" };
  }
  if (dedupedIncoming.length === 0) {
    return { value: current, status: "unchanged" };
  }

  // Merge: keep current's order; for each current entry, replace value with
  // incoming's value if the type matches. Append new types from incoming
  // (in incoming order) at the end.
  const incomingMap = new Map(dedupedIncoming.map((id) => [id.type, id.value]));
  let changed = false;
  const merged: IdentifierEntry[] = current.map((id) => {
    const incomingValue = incomingMap.get(id.type);
    if (incomingValue !== undefined && incomingValue !== id.value) {
      changed = true;
      return { type: id.type, value: incomingValue };
    }
    return id;
  });
  const currentTypes = new Set(current.map((id) => id.type));
  for (const entry of dedupedIncoming) {
    if (!currentTypes.has(entry.type)) {
      merged.push(entry);
      changed = true;
    }
  }

  return { value: merged, status: changed ? "changed" : "unchanged" };
}
```

- [ ] **Step 4: Run the tests and confirm they pass.**

Run: `pnpm vitest run app/components/library/identify-utils.test.ts app/components/library/IdentifyReviewForm.test.ts`
Expected: all pass.

- [ ] **Step 5: Run typecheck and lint.**

Run: `pnpm lint:types && pnpm lint:eslint app/components/library/identify-utils.ts app/components/library/identify-utils.test.ts app/components/library/IdentifyReviewForm.test.ts`
Expected: no errors.

- [ ] **Step 6: Commit.**

```bash
git add app/components/library/identify-utils.ts app/components/library/identify-utils.test.ts app/components/library/IdentifyReviewForm.test.ts
git commit -m "[Frontend] Dedupe identifiers by type with incoming-wins in identify merge"
```

---

## Task 2: Frontend `FileEditDialog` — disable already-present identifier types

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx`
- Test: `app/components/library/FileEditDialog.test.tsx`

- [ ] **Step 1: Locate the type Select in `FileEditDialog.tsx`.**

Open `app/components/library/FileEditDialog.tsx` and find the add-identifier form, starting around line 1292:

```tsx
<div className="flex gap-2">
  <Select
    onValueChange={setNewIdentifierType}
    value={newIdentifierType}
  >
    <SelectTrigger className="w-auto min-w-32 shrink-0 gap-2">
      <SelectValue />
    </SelectTrigger>
```

Continue reading until you find the `SelectContent` block enumerating identifier type options (built-ins + `pluginIdentifierTypes`). Note the exact JSX shape and how each `SelectItem` is rendered today — the rest of this task assumes you've inspected it.

- [ ] **Step 2: Add a failing test for the disabled-type behavior.**

In `app/components/library/FileEditDialog.test.tsx`, add a new test case (placement: append to the existing `describe` block; if no `describe` block covers identifier-editing, add one named `describe("identifier add form", ...)`):

```tsx
it("disables identifier types that are already present in the form", async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

  // Render with a file that already has an ASIN identifier.
  // (Use the existing test harness/fixture builder; pattern after other tests
  // in this file that pass a file with non-default identifiers.)
  renderFileEditDialogWithIdentifiers([
    { type: "asin", value: "B01ABC1234" },
  ]);

  await user.click(screen.getByRole("combobox", { name: /identifier type/i }));

  // Radix Select uses role="option"; the disabled state is exposed via aria-disabled.
  const asinOption = await screen.findByRole("option", { name: /asin/i });
  expect(asinOption).toHaveAttribute("aria-disabled", "true");

  // Hover surfaces the explanatory tooltip.
  await user.hover(asinOption);
  expect(
    await screen.findByText(
      /this file already has an asin identifier\. remove it first/i,
    ),
  ).toBeInTheDocument();
});

it("re-enables a previously-disabled type after the existing identifier is removed", async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  renderFileEditDialogWithIdentifiers([
    { type: "asin", value: "B01ABC1234" },
  ]);

  // Remove the existing ASIN badge.
  const removeButtons = screen.getAllByRole("button", { name: /remove/i });
  await user.click(removeButtons[0]);

  await user.click(screen.getByRole("combobox", { name: /identifier type/i }));
  const asinOption = await screen.findByRole("option", { name: /asin/i });
  expect(asinOption).not.toHaveAttribute("aria-disabled", "true");
});
```

If `renderFileEditDialogWithIdentifiers` does not already exist in this file, define a small helper at the top of the test file that calls the existing render-the-dialog helper and pre-populates the file's identifiers (mirror the existing pattern; do not introduce new MSW handlers if the test file already mocks the queries). The two new tests should follow the same imports and harness as the existing FileEditDialog tests.

- [ ] **Step 3: Run the tests and confirm they fail.**

Run: `pnpm vitest run app/components/library/FileEditDialog.test.tsx`
Expected: the two new tests fail (the option will be selectable today).

- [ ] **Step 4: Implement the disabled-type behavior.**

In `app/components/library/FileEditDialog.tsx`:

1. Compute the set of types already present in the form. Add this `useMemo` near the other identifier-related state:

```tsx
const presentIdentifierTypes = useMemo(
  () => new Set(identifiers.map((id) => id.type)),
  [identifiers],
);
```

2. In the `SelectContent` block that renders identifier-type `SelectItem`s, wrap each item so that when its type is in `presentIdentifierTypes`, the `SelectItem` receives `disabled` and is wrapped in a `Tooltip` with explanatory copy. The Radix Select disabled item won't fire `onSelect`, and the `Tooltip` shows on hover.

The exact JSX shape depends on what's already there, but the pattern is:

```tsx
{availableTypes.map((typeDef) => {
  const isPresent = presentIdentifierTypes.has(typeDef.id);
  const item = (
    <SelectItem
      key={typeDef.id}
      value={typeDef.id}
      disabled={isPresent}
    >
      {formatIdentifierType(typeDef.id, pluginIdentifierTypes)}
    </SelectItem>
  );
  if (!isPresent) return item;
  return (
    <Tooltip key={typeDef.id}>
      <TooltipTrigger asChild>
        <span className="block">{item}</span>
      </TooltipTrigger>
      <TooltipContent>
        This file already has{" "}
        {/^[aeiou]/i.test(formatIdentifierType(typeDef.id, pluginIdentifierTypes))
          ? "an"
          : "a"}{" "}
        {formatIdentifierType(typeDef.id, pluginIdentifierTypes)} identifier.
        Remove it first to add a different value.
      </TooltipContent>
    </Tooltip>
  );
})}
```

(Adapt to the actual variable names and rendering pattern in the file. If types are sourced from a constant array of built-ins plus `pluginIdentifierTypes`, treat both the same.)

If the import of `Tooltip`, `TooltipTrigger`, and `TooltipContent` isn't already in the file, add it:

```tsx
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
```

3. The "Add" button (separate from the type Select) must continue to work for non-disabled types. Don't change its behavior beyond what's required; in particular, do not remove existing client-side "ignore add when type/value is empty" guards.

- [ ] **Step 5: Run the tests and confirm they pass.**

Run: `pnpm vitest run app/components/library/FileEditDialog.test.tsx`
Expected: all pass, including the two new tests.

- [ ] **Step 6: Run typecheck and lint.**

Run: `pnpm lint:types && pnpm lint:eslint app/components/library/FileEditDialog.tsx app/components/library/FileEditDialog.test.tsx`
Expected: no errors.

- [ ] **Step 7: Manual smoke check.**

Start the dev server if not running (`mise start`), open a book in the library that has at least one identifier (e.g., an ASIN), open the file edit dialog, click the identifier type Select, and confirm the type matching the existing identifier is greyed out with the explanatory tooltip on hover. Remove the existing badge — the type should re-enable.

- [ ] **Step 8: Commit.**

```bash
git add app/components/library/FileEditDialog.tsx app/components/library/FileEditDialog.test.tsx
git commit -m "[Frontend] Disable already-present identifier types in FileEditDialog with tooltip"
```

---

## Task 3: Backend `BulkCreateFileIdentifiers` — defensive type-dedupe + warn log

**Files:**
- Modify: `pkg/books/service.go:1408-1420`
- Create: `pkg/books/service_identifiers_test.go`

- [ ] **Step 1: Write a failing test for the dedupe behavior.**

Create `pkg/books/service_identifiers_test.go`:

```go
package books

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_BulkCreateFileIdentifiers_DedupesByType(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	_, book := setupTestLibraryAndBook(t, db)
	file := setupTestFile(t, db, book.ID)

	// Same type appears twice; last-wins.
	identifiers := []*models.FileIdentifier{
		{FileID: file.ID, Type: "asin", Value: "B01ABC1234", Source: models.DataSourcePlugin("shisho", "audnexus")},
		{FileID: file.ID, Type: "isbn_13", Value: "9780316769488", Source: models.DataSourceEPUBMetadata},
		{FileID: file.ID, Type: "asin", Value: "B02DEF5678", Source: models.DataSourceManual},
	}

	err := svc.BulkCreateFileIdentifiers(ctx, identifiers)
	require.NoError(t, err)

	var stored []*models.FileIdentifier
	err = db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Order("type ASC").Scan(ctx)
	require.NoError(t, err)

	require.Len(t, stored, 2)
	asin := stored[0]
	isbn := stored[1]
	assert.Equal(t, "asin", asin.Type)
	assert.Equal(t, "B02DEF5678", asin.Value, "expected last-wins for the asin type")
	assert.Equal(t, models.DataSourceManual, asin.Source)
	assert.Equal(t, "isbn_13", isbn.Type)
	assert.Equal(t, "9780316769488", isbn.Value)
}

func TestService_BulkCreateFileIdentifiers_NoDuplicates(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	_, book := setupTestLibraryAndBook(t, db)
	file := setupTestFile(t, db, book.ID)

	identifiers := []*models.FileIdentifier{
		{FileID: file.ID, Type: "asin", Value: "B01ABC1234", Source: models.DataSourceManual},
		{FileID: file.ID, Type: "isbn_13", Value: "9780316769488", Source: models.DataSourceEPUBMetadata},
	}

	err := svc.BulkCreateFileIdentifiers(ctx, identifiers)
	require.NoError(t, err)

	var stored []*models.FileIdentifier
	err = db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Order("type ASC").Scan(ctx)
	require.NoError(t, err)
	require.Len(t, stored, 2)
}

func TestService_BulkCreateFileIdentifiers_EmptySliceIsNoop(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	err := svc.BulkCreateFileIdentifiers(ctx, nil)
	assert.NoError(t, err)

	err = svc.BulkCreateFileIdentifiers(ctx, []*models.FileIdentifier{})
	assert.NoError(t, err)
}
```

If `setupTestFile` does not exist, check `pkg/books/service_test.go` and `pkg/books/service_bulk_test.go` for the helper used to insert a file row in tests; reuse whichever helper is already in use, or inline the creation following the pattern from existing `*_test.go` files in this package. Do not invent a helper — match the existing style.

If `models.DataSourcePlugin` is the wrong constant/function name for plugin-source values, look at `pkg/models/data_source.go` (or wherever data sources are defined) for the correct one. The CLAUDE.md says plugin sources use `plugin:scope/id` format and there's a `models.PluginDataSource(scope, id)` helper — adjust the test accordingly.

- [ ] **Step 2: Run the test and confirm it fails.**

Run: `mise test ./pkg/books/ -run TestService_BulkCreateFileIdentifiers`
Expected: `TestService_BulkCreateFileIdentifiers_DedupesByType` either fails because the bulk insert errored on the unique constraint, or because (depending on driver behavior) the second `asin` insert silently bypassed and the test sees `B01ABC1234` instead of `B02DEF5678`. Either failure is correct.

- [ ] **Step 3: Implement the dedupe + warn log.**

Replace the body of `BulkCreateFileIdentifiers` in `pkg/books/service.go` (lines 1408-1420) with:

```go
// BulkCreateFileIdentifiers creates multiple file identifier records in a
// single query. Identifier values are canonicalized via
// identifiers.NormalizeValue before insert. Defensively dedupes by type
// (last-wins) so a misbehaving caller never trips the UNIQUE(file_id, type)
// constraint; each dropped duplicate is logged at warn level so upstream
// bugs are visible. Returns nil if the slice is empty after dedupe.
func (svc *Service) BulkCreateFileIdentifiers(ctx context.Context, fileIdentifiers []*models.FileIdentifier) error {
	if len(fileIdentifiers) == 0 {
		return nil
	}
	type key struct {
		FileID int
		Type   string
	}
	indexByKey := make(map[key]int, len(fileIdentifiers))
	deduped := make([]*models.FileIdentifier, 0, len(fileIdentifiers))
	for _, fi := range fileIdentifiers {
		clone := *fi
		clone.Value = identifiers.NormalizeValue(fi.Type, fi.Value)
		k := key{FileID: clone.FileID, Type: clone.Type}
		if existingIdx, ok := indexByKey[k]; ok {
			dropped := deduped[existingIdx]
			log.Warn("dropping duplicate file identifier of same type", logger.Data{
				"file_id":        clone.FileID,
				"type":           clone.Type,
				"dropped_value":  dropped.Value,
				"dropped_source": dropped.Source,
				"kept_value":     clone.Value,
				"kept_source":    clone.Source,
			})
			deduped[existingIdx] = &clone
			continue
		}
		indexByKey[k] = len(deduped)
		deduped = append(deduped, &clone)
	}
	if len(deduped) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&deduped).Exec(ctx)
	return errors.WithStack(err)
}
```

If `log` (from `github.com/robinjoseph08/golib/logger`) is not yet imported in `pkg/books/service.go`, add it. Check the existing import for `logger.Data` (line 12) — `log` is the logger instance. If the package uses a different logger, mirror its existing call sites in the file (e.g. lines 480, 828, 838, 862, 896 use `log.Error`/`log.Info`/`log.Warn` with `logger.Data{...}`). Reuse the same pattern.

- [ ] **Step 4: Run the test and confirm it passes.**

Run: `mise test ./pkg/books/ -run TestService_BulkCreateFileIdentifiers`
Expected: all three tests pass.

- [ ] **Step 5: Commit.**

```bash
git add pkg/books/service.go pkg/books/service_identifiers_test.go
git commit -m "[Backend] Dedupe by type in BulkCreateFileIdentifiers with warn log"
```

---

## Task 4: Backend `updateFile` — reject duplicate-type payloads with 422

**Files:**
- Modify: `pkg/books/handlers.go:1112-1132`
- Test: `pkg/books/handlers_test.go` (or `pkg/books/handlers_files_test.go` if that's the convention — check existing tests for `updateFile`).

- [ ] **Step 1: Locate the existing `updateFile` test patterns.**

Read existing tests in `pkg/books/handlers_test.go` (or whichever file holds `updateFile` tests) to confirm the test harness pattern: how the request is built, how the auth/middleware is set up, and how DB state is asserted. Use that as the template for the new test.

- [ ] **Step 2: Write a failing test for duplicate-type rejection.**

Append to the appropriate handlers test file (the same one that holds existing `updateFile` tests):

```go
func TestUpdateFile_RejectsDuplicateIdentifierTypes(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t) // or whatever the existing test harness function is
	defer tc.cleanup()

	_, book := setupTestLibraryAndBook(t, tc.db)
	file := setupTestFile(t, tc.db, book.ID)

	// Seed an existing identifier so we can assert it's preserved when the
	// 422 short-circuits the handler.
	existing := &models.FileIdentifier{
		FileID: file.ID,
		Type:   "asin",
		Value:  "B01ORIGINAL",
		Source: models.DataSourceManual,
	}
	require.NoError(t, tc.svc.CreateFileIdentifier(tc.ctx, existing))

	body := `{"identifiers":[{"type":"asin","value":"B01AAA"},{"type":"asin","value":"B02BBB"}]}`
	resp := tc.do(t, http.MethodPut, "/books/files/"+strconv.Itoa(file.ID), body)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
	assert.Contains(t, resp.Body.String(), "asin")

	// Existing identifier untouched.
	var stored []*models.FileIdentifier
	err := tc.db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Scan(tc.ctx)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "B01ORIGINAL", stored[0].Value)
}
```

Note: `tc.svc.CreateFileIdentifier` is used here for test setup only, before Task 7 deletes it. After Task 7 lands, replace this seed with `tc.svc.BulkCreateFileIdentifiers(tc.ctx, []*models.FileIdentifier{existing})`. (See Task 7's note about updating tests.)

If the existing test harness uses different names — e.g., `setupTestServer`, `req()`, etc. — match those exactly. Look at the test directly above your insertion point and pattern after it.

- [ ] **Step 3: Run the test and confirm it fails.**

Run: `mise test ./pkg/books/ -run TestUpdateFile_RejectsDuplicateIdentifierTypes`
Expected: fails — the current handler accepts the payload, attempts the duplicate insert, logs a warning, and returns 200 with one of the values silently dropped.

- [ ] **Step 4: Implement the validation.**

In `pkg/books/handlers.go`, modify the `if params.Identifiers != nil { ... }` block (lines 1112-1132). Insert this validation at the top of the block, before `DeleteFileIdentifiers`:

```go
if params.Identifiers != nil {
	// Reject duplicate types before any DB mutation. The DB enforces
	// UNIQUE(file_id, type); surface the contract violation explicitly
	// instead of silently dropping the second insert.
	seen := make(map[string]struct{}, len(*params.Identifiers))
	for _, id := range *params.Identifiers {
		if _, dup := seen[id.Type]; dup {
			return errcodes.ValidationError("duplicate identifier type: " + id.Type)
		}
		seen[id.Type] = struct{}{}
	}

	// (... existing delete-then-create code follows in step 5 below)
```

Confirm `errcodes` is already imported at the top of `handlers.go` (it is — used at lines 93, 106, 141, etc.). No import change needed.

- [ ] **Step 5: Run the test and confirm it passes.**

Run: `mise test ./pkg/books/ -run TestUpdateFile_RejectsDuplicateIdentifierTypes`
Expected: pass.

- [ ] **Step 6: Run all books-package tests to ensure no regressions.**

Run: `mise test ./pkg/books/`
Expected: pass.

- [ ] **Step 7: Commit.**

```bash
git add pkg/books/handlers.go pkg/books/handlers_test.go
git commit -m "[Backend] Reject duplicate identifier types in updateFile with 422"
```

---

## Task 5: Backend `updateFile` — bulk insert + source preservation

**Files:**
- Modify: `pkg/books/handlers.go:1112-1132`
- Test: same handlers test file as Task 4.

- [ ] **Step 1: Write a failing test for source preservation.**

Append to the handlers test file:

```go
func TestUpdateFile_PreservesSourceForUnchangedIdentifiers(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	defer tc.cleanup()

	_, book := setupTestLibraryAndBook(t, tc.db)
	file := setupTestFile(t, tc.db, book.ID)

	// Seed identifiers with two distinct sources.
	pluginSource := models.PluginDataSource("shisho", "audnexus")
	require.NoError(t, tc.svc.BulkCreateFileIdentifiers(tc.ctx, []*models.FileIdentifier{
		{FileID: file.ID, Type: "asin", Value: "B01ABC1234", Source: pluginSource},
		{FileID: file.ID, Type: "isbn_13", Value: "9780316769488", Source: models.DataSourceEPUBMetadata},
	}))

	// Re-submit ASIN with the same value, ISBN unchanged. Add a new goodreads.
	body := `{"identifiers":[
		{"type":"asin","value":"B01ABC1234"},
		{"type":"isbn_13","value":"9780316769488"},
		{"type":"goodreads","value":"12345"}
	]}`
	resp := tc.do(t, http.MethodPut, "/books/files/"+strconv.Itoa(file.ID), body)
	require.Equal(t, http.StatusOK, resp.Code)

	var stored []*models.FileIdentifier
	err := tc.db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Order("type ASC").Scan(tc.ctx)
	require.NoError(t, err)
	require.Len(t, stored, 3)

	byType := map[string]*models.FileIdentifier{}
	for _, id := range stored {
		byType[id.Type] = id
	}
	assert.Equal(t, pluginSource, byType["asin"].Source, "asin source preserved (unchanged value)")
	assert.Equal(t, models.DataSourceEPUBMetadata, byType["isbn_13"].Source, "isbn_13 source preserved (unchanged value)")
	assert.Equal(t, models.DataSourceManual, byType["goodreads"].Source, "new identifier gets manual source")
}

func TestUpdateFile_AssignsManualSourceWhenValueChanges(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	defer tc.cleanup()

	_, book := setupTestLibraryAndBook(t, tc.db)
	file := setupTestFile(t, tc.db, book.ID)

	pluginSource := models.PluginDataSource("shisho", "audnexus")
	require.NoError(t, tc.svc.BulkCreateFileIdentifiers(tc.ctx, []*models.FileIdentifier{
		{FileID: file.ID, Type: "asin", Value: "B01ORIG", Source: pluginSource},
	}))

	body := `{"identifiers":[{"type":"asin","value":"B02NEW"}]}`
	resp := tc.do(t, http.MethodPut, "/books/files/"+strconv.Itoa(file.ID), body)
	require.Equal(t, http.StatusOK, resp.Code)

	var stored []*models.FileIdentifier
	require.NoError(t, tc.db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Scan(tc.ctx))
	require.Len(t, stored, 1)
	assert.Equal(t, "B02NEW", stored[0].Value)
	assert.Equal(t, models.DataSourceManual, stored[0].Source, "value-changed entry gets manual source")
}
```

If `models.PluginDataSource` is wrong, look at `pkg/models/` for the right helper or constant — same as in Task 3.

- [ ] **Step 2: Run the tests and confirm they fail.**

Run: `mise test ./pkg/books/ -run "TestUpdateFile_PreservesSourceForUnchangedIdentifiers|TestUpdateFile_AssignsManualSourceWhenValueChanges"`
Expected: both fail — current handler hardcodes `models.DataSourceManual` for every row.

- [ ] **Step 3: Implement source preservation and bulk insert.**

In `pkg/books/handlers.go`, replace the entire identifiers block in `updateFile` (the `if params.Identifiers != nil { ... }` block — including the validation added in Task 4) with:

```go
if params.Identifiers != nil {
	// Reject duplicate types before any DB mutation.
	seen := make(map[string]struct{}, len(*params.Identifiers))
	for _, id := range *params.Identifiers {
		if _, dup := seen[id.Type]; dup {
			return errcodes.ValidationError("duplicate identifier type: " + id.Type)
		}
		seen[id.Type] = struct{}{}
	}

	// Read existing identifiers so we can preserve `source` when an entry's
	// (type, normalized value) is unchanged from what's already stored.
	// Replacements and net-new entries get DataSourceManual since the user
	// explicitly applied them via this endpoint.
	existing, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &file.ID})
	if err != nil {
		return errors.WithStack(err)
	}
	type sourceKey struct {
		Type           string
		NormalizedValue string
	}
	existingSources := make(map[sourceKey]string, len(existing.Identifiers))
	for _, ex := range existing.Identifiers {
		existingSources[sourceKey{Type: ex.Type, NormalizedValue: ex.Value}] = ex.Source
	}

	if err := h.bookService.DeleteFileIdentifiers(ctx, file.ID); err != nil {
		return errors.WithStack(err)
	}

	toInsert := make([]*models.FileIdentifier, 0, len(*params.Identifiers))
	for _, id := range *params.Identifiers {
		source := models.DataSourceManual
		normValue := identifiers.NormalizeValue(id.Type, id.Value)
		if prev, ok := existingSources[sourceKey{Type: id.Type, NormalizedValue: normValue}]; ok {
			source = prev
		}
		toInsert = append(toInsert, &models.FileIdentifier{
			FileID: file.ID,
			Type:   id.Type,
			Value:  id.Value,
			Source: source,
		})
	}
	if err := h.bookService.BulkCreateFileIdentifiers(ctx, toInsert); err != nil {
		return errors.WithStack(err)
	}
	file.IdentifierSource = strPtr(models.DataSourceManual)
	opts.Columns = append(opts.Columns, "identifier_source")
}
```

Imports needed (verify they're present, add if not):
- `github.com/shishobooks/shisho/pkg/identifiers` — for `NormalizeValue`. Check the existing imports in `handlers.go`.
- `github.com/shishobooks/shisho/pkg/books` is the current package, so `books.RetrieveFileOptions` is just `RetrieveFileOptions{}` from inside the same package. **Correct the snippet** to remove the `books.` prefix when in-package — pattern after the existing call to `RetrieveBook` in the same handler at line 1151.

If `RetrieveFile` does not include `Identifiers` relation by default, check `service.go:558-590`. Looking at the codebase (per `pkg/CLAUDE.md`), `RetrieveFile` already includes the `Identifiers` relation, so `existing.Identifiers` will be populated.

- [ ] **Step 4: Run the new tests and confirm they pass.**

Run: `mise test ./pkg/books/ -run "TestUpdateFile_PreservesSourceForUnchangedIdentifiers|TestUpdateFile_AssignsManualSourceWhenValueChanges"`
Expected: pass.

- [ ] **Step 5: Run all books-package tests to ensure no regressions.**

Run: `mise test ./pkg/books/`
Expected: pass. The earlier `TestUpdateFile_RejectsDuplicateIdentifierTypes` from Task 4 must still pass.

- [ ] **Step 6: Commit.**

```bash
git add pkg/books/handlers.go pkg/books/handlers_test.go
git commit -m "[Backend] Preserve identifier source for unchanged rows in updateFile"
```

---

## Task 6: Backend plugin metadata — refactor to bulk insert

**Files:**
- Modify: `pkg/plugins/handler.go` (interface)
- Modify: `pkg/plugins/handler_persist_metadata.go:263-281`
- Modify: `pkg/server/server.go:302-308`
- Test: existing plugin tests covering `persistMetadata` (find via `grep -rn "persistMetadata\|applyMetadata" pkg/plugins/*_test.go` and pattern after the closest match).

- [ ] **Step 1: Add a failing test for duplicate-type robustness from a plugin payload.**

Locate the existing test that exercises `persistMetadata` with identifiers. If none exists, the closest analog is the `applyMetadata` integration test (search via `grep -n "applyMetadata\|Identifiers:" pkg/plugins/*_test.go`). Add a new test in the same file:

```go
func TestPersistMetadata_DedupesDuplicateIdentifierTypesFromPlugin(t *testing.T) {
	t.Parallel()
	// Set up the test handler context (mirror existing plugin test setup).
	tc := newPluginTestContext(t) // or whatever the existing helper is named
	defer tc.cleanup()

	file := tc.seedFile(t, "test.epub")

	// Plugin payload deliberately contains two of the same type.
	md := &mediafile.ParsedMetadata{
		Identifiers: []mediafile.ParsedIdentifier{
			{Type: "asin", Value: "B01ORIG"},
			{Type: "asin", Value: "B02NEW"}, // duplicate type, last-wins
			{Type: "isbn_13", Value: "9780316769488"},
		},
	}

	err := tc.handler.persistMetadataForFile(tc.ctx, file, md, "shisho", "audnexus")
	require.NoError(t, err, "duplicate types from plugin must not error")

	var stored []*models.FileIdentifier
	require.NoError(t, tc.db.NewSelect().Model(&stored).Where("file_id = ?", file.ID).Order("type ASC").Scan(tc.ctx))
	require.Len(t, stored, 2)
	byType := map[string]*models.FileIdentifier{}
	for _, id := range stored {
		byType[id.Type] = id
	}
	assert.Equal(t, "B02NEW", byType["asin"].Value, "last-wins on intra-payload duplicate")
}
```

Adapt the test harness names (`newPluginTestContext`, `tc.handler.persistMetadataForFile`, `tc.seedFile`) to whatever's already in use. If the persist function is exposed differently, find its existing test and mirror that exactly.

- [ ] **Step 2: Run the test and confirm it fails.**

Run: `mise test ./pkg/plugins/ -run TestPersistMetadata_DedupesDuplicateIdentifierTypesFromPlugin`
Expected: fail — the current per-item create loop calls `CreateFileIdentifier`, which trips the unique constraint on the second `asin` and logs a warning. Either the assertion on count or value will fail.

- [ ] **Step 3: Update the `identifierStore` interface.**

In `pkg/plugins/handler.go`, replace lines 50-54 with:

```go
// identifierStore provides file identifier CRUD operations.
type identifierStore interface {
	DeleteIdentifiersForFile(ctx context.Context, fileID int) (int, error)
	BulkCreateFileIdentifiers(ctx context.Context, fileIdentifiers []*models.FileIdentifier) error
}
```

- [ ] **Step 4: Update the adapter.**

In `pkg/server/server.go`, replace lines 302-308 (the `DeleteIdentifiersForFile` and `CreateFileIdentifier` adapter methods) with:

```go
func (a *bookUpdaterAdapter) DeleteIdentifiersForFile(ctx context.Context, fileID int) (int, error) {
	return a.svc.DeleteIdentifiersForFile(ctx, fileID)
}

func (a *bookUpdaterAdapter) BulkCreateFileIdentifiers(ctx context.Context, fileIdentifiers []*models.FileIdentifier) error {
	return a.svc.BulkCreateFileIdentifiers(ctx, fileIdentifiers)
}
```

- [ ] **Step 5: Refactor `handler_persist_metadata.go` to use bulk insert.**

In `pkg/plugins/handler_persist_metadata.go`, replace lines 263-281 (the `// Identifiers (file-level, applied to target file)` block) with:

```go
// Identifiers (file-level, applied to target file). Filter out blanks the
// plugin may have emitted, then bulk-insert. The bulk helper dedupes by
// type with last-wins and warns, so a misbehaving plugin never trips the
// UNIQUE(file_id, type) constraint.
if len(md.Identifiers) > 0 && targetFile != nil {
	if _, err := h.enrich.identStore.DeleteIdentifiersForFile(ctx, targetFile.ID); err != nil {
		return errors.Wrap(err, "failed to delete identifiers")
	}
	toInsert := make([]*models.FileIdentifier, 0, len(md.Identifiers))
	for _, ident := range md.Identifiers {
		if ident.Type == "" || ident.Value == "" {
			continue
		}
		toInsert = append(toInsert, &models.FileIdentifier{
			FileID: targetFile.ID,
			Type:   ident.Type,
			Value:  ident.Value,
			Source: pluginSource,
		})
	}
	if err := h.enrich.identStore.BulkCreateFileIdentifiers(ctx, toInsert); err != nil {
		return errors.Wrap(err, "failed to bulk-create identifiers")
	}
}
```

- [ ] **Step 6: Run the new test and confirm it passes.**

Run: `mise test ./pkg/plugins/ -run TestPersistMetadata_DedupesDuplicateIdentifierTypesFromPlugin`
Expected: pass.

- [ ] **Step 7: Run all plugins-package tests.**

Run: `mise test ./pkg/plugins/`
Expected: pass.

- [ ] **Step 8: Commit.**

```bash
git add pkg/plugins/handler.go pkg/plugins/handler_persist_metadata.go pkg/server/server.go pkg/plugins/*_test.go
git commit -m "[Backend] Refactor plugin metadata persistence to use BulkCreateFileIdentifiers"
```

---

## Task 7: Backend cleanup — delete `CreateFileIdentifier`

**Files:**
- Modify: `pkg/books/service.go:534-542` (delete the function)
- Modify: any tests that called `tc.svc.CreateFileIdentifier` for setup (replace with `BulkCreateFileIdentifiers`)
- Modify: `pkg/server/server.go` if any residual reference remains

- [ ] **Step 1: Confirm no callers remain.**

Run: `grep -rn "CreateFileIdentifier\b" pkg --include="*.go"`
Expected output (anything else means a caller was missed in earlier tasks):
- `pkg/books/service.go:535` (the function definition itself).

If any other matches appear that aren't tests, fix the caller first to use `BulkCreateFileIdentifiers` before continuing.

- [ ] **Step 2: Update test setup helpers that still use `CreateFileIdentifier`.**

In whichever test files used `tc.svc.CreateFileIdentifier` (notably `TestUpdateFile_RejectsDuplicateIdentifierTypes` from Task 4 — search via `grep -rn "CreateFileIdentifier" pkg --include="*_test.go"`), replace each call:

```go
require.NoError(t, tc.svc.CreateFileIdentifier(tc.ctx, existing))
```

with:

```go
require.NoError(t, tc.svc.BulkCreateFileIdentifiers(tc.ctx, []*models.FileIdentifier{existing}))
```

- [ ] **Step 3: Delete `CreateFileIdentifier` from `pkg/books/service.go`.**

Remove lines 534-542 (the entire `CreateFileIdentifier` function, including the doc comment).

- [ ] **Step 4: Run the full test suite to confirm nothing broke.**

Run: `mise test`
Expected: pass.

- [ ] **Step 5: Confirm the function is gone.**

Run: `grep -rn "CreateFileIdentifier\b" pkg --include="*.go"`
Expected: empty output.

- [ ] **Step 6: Commit.**

```bash
git add pkg/books/service.go pkg/books/handlers_test.go
git commit -m "[Backend] Delete unused CreateFileIdentifier"
```

---

## Task 8: Docs — update `metadata.md`

**Files:**
- Modify: `website/docs/metadata.md:41` (and surrounding paragraph)

- [ ] **Step 1: Read the existing identifiers section.**

Open `website/docs/metadata.md` and read the section starting around line 41 (the existing line: "Identifiers (ISBN, ASIN, etc.) are also file-level. Each file can have multiple identifiers of different types..."). Note the surrounding paragraph and any cross-links.

- [ ] **Step 2: Update the section.**

Replace the existing identifier description with the following (preserving any neighboring content; only the identifier-specific paragraphs change):

```markdown
Identifiers (ISBN, ASIN, etc.) are also file-level. Each file can have multiple identifiers of **different** types: `isbn_10`, `isbn_13`, `asin`, `uuid`, `goodreads`, `google`, and custom types registered by [plugins](./plugins/overview).

A file has at most **one identifier per type**. You can have an ISBN-13 and an ASIN on the same file, but you cannot have two ASINs. The file edit dialog enforces this in the type dropdown — types already in use are greyed out until you remove the existing entry.

When you confirm an identify match (via the Identify dialog) and the match brings in an identifier whose type already exists on the file, the incoming value **replaces** the existing one. Identifiers of types not in the match are kept untouched.
```

Do not delete the canonicalization paragraph (about hyphens, `urn:uuid:`, etc.) that follows; keep it in place after the new content. The "Searches by identifier accept any of the cosmetic variants above..." line continues to apply.

- [ ] **Step 3: Confirm docs build.**

If `mise docs` is available and easy to run, start the docs dev server and visually confirm the section renders. Otherwise inspect the raw Markdown for syntax issues.

- [ ] **Step 4: Commit.**

```bash
git add website/docs/metadata.md
git commit -m "[Docs] Document file identifier type-uniqueness and identify-replaces-on-conflict"
```

---

## Task 9: Final verification

- [ ] **Step 1: Run the full check suite.**

Run: `mise check:quiet`
Expected: pass with all green.

- [ ] **Step 2: Manual smoke test of the identify flow.**

Start the dev server (`mise start`). Open a book, run identify, pick a result that has a different ASIN than the file currently has, confirm the merged identifiers in the review form show the incoming ASIN replacing the existing one, save, reload the file detail page, confirm the ASIN persists with the new value.

- [ ] **Step 3: Manual smoke test of the manual edit flow.**

Open the file edit dialog for a book that has at least one identifier. Verify:

- The type whose identifier is already present is greyed out in the dropdown.
- Hovering shows the "remove it first" tooltip.
- Removing the existing badge re-enables the type.
- Saving with no changes preserves the original `source` (verify by inspecting `tmp/data.sqlite` if you want to be thorough — `select type, value, source from file_identifiers where file_id = <id>`).

---

## Self-review

- **Spec coverage:**
  - Backend `updateFile` 422 on duplicate types: Task 4. ✓
  - Backend `BulkCreateFileIdentifiers` dedupe + warn: Task 3. ✓
  - Backend plugin metadata refactor to bulk: Task 6. ✓
  - Source preservation for unchanged rows: Task 5. ✓
  - `CreateFileIdentifier` deletion: Task 7. ✓
  - Frontend `resolveIdentifiers` type-keyed merge: Task 1. ✓
  - Frontend `FileEditDialog` disabled types + tooltip: Task 2. ✓
  - Docs update: Task 8. ✓
  - Out-of-scope items (no migration, no SDK changes, no sidecar rewriting, no global uniqueness) explicitly omitted. ✓

- **Type consistency:** `BulkCreateFileIdentifiers` signature is consistent across Task 3 (definition), Task 5 (call site), Task 6 (interface + adapter + call site), Task 7 (cleanup). `resolveIdentifiers` keyed-by-type semantics are consistent across Task 1's tests and implementation. `IdentifierEntry` shape unchanged.

- **Placeholder scan:** Tasks reference real file paths and line numbers from current HEAD; code blocks are complete; commands are runnable; expected outputs are specific. The two places where I deferred to "match existing test harness names" (Tasks 4, 5, 6) are necessary because the harness names vary across files in this codebase and reading the nearest existing test is the right move — but the surrounding test bodies are spelled out concretely.
