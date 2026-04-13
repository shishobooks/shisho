# Identify Dialog Author Roles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display author role labels (e.g., `(Writer)`, `(Penciller)`) in the identify search result row, "Use current" summary, and editable author chips so same-named authors with different roles stop looking like duplicates.

**Architecture:** Extract the existing `getRoleLabel` helper out of `BookDetail.tsx` into a shared `app/utils/authorRoles.ts` module (renamed `getAuthorRoleLabel` to avoid collision with user-permission `roles.ts`). Then update three render sites in the identify flow to consume it: the search result row in `IdentifyBookDialog.tsx`, the Authors field summary in `IdentifyReviewForm.tsx`, and a new role-aware `AuthorTagInput` that replaces the generic `TagInput` for the authors field only.

**Tech Stack:** React 19 + TypeScript, Vitest for unit tests, TailwindCSS for styling. Uses existing `Badge` component and `AuthorEntry { name, role }` shape already present in `IdentifyReviewForm.tsx`.

**Spec:** `docs/superpowers/specs/2026-04-13-identify-author-roles-design.md`

---

## Context for the implementing engineer

Read the spec first. A few things to know about this codebase before starting:

- **Test runner:** `pnpm test:unit` runs Vitest with coverage. To run a single file: `pnpm test:unit app/utils/authorRoles.test.ts`. To run in watch mode during TDD: `pnpm exec vitest app/utils/authorRoles.test.ts`.
- **Type generation:** Don't worry about `mise tygo` for this task — no Go types are changing.
- **Full check:** `mise check:quiet` runs Go tests, Go lint, JS tests, ESLint, Prettier, and TypeScript in parallel. Run it once before the final commit.
- **Path alias:** `@/` maps to `app/`. Import the helper as `from "@/utils/authorRoles"`.
- **`AuthorEntry` type** already exists inside `IdentifyReviewForm.tsx` (lines 53–56). Keep it; don't move it out.
- **`TagInput` component** is inside `IdentifyReviewForm.tsx` (lines 320–387) and is used for narrators/genres/tags too — leave it alone. You're adding a sibling component `AuthorTagInput`, not modifying `TagInput`.
- **Commit style:** `[Frontend] <description>` for frontend changes, `[Test] <description>` for test-only commits. See `CLAUDE.md` Git Conventions.
- **TDD is required** for this task per `CLAUDE.md`: Red → Green → Refactor. Write the failing test first, verify it fails, then implement.

---

## Task 1: Create `getAuthorRoleLabel` helper with failing test

**Files:**
- Create: `app/utils/authorRoles.ts`
- Create: `app/utils/authorRoles.test.ts`

- [ ] **Step 1: Write the failing test**

Create `app/utils/authorRoles.test.ts` with the following content:

```ts
import { describe, expect, it } from "vitest";

import { getAuthorRoleLabel } from "./authorRoles";

describe("getAuthorRoleLabel", () => {
  it("maps known canonical roles to capitalized labels", () => {
    expect(getAuthorRoleLabel("writer")).toBe("Writer");
    expect(getAuthorRoleLabel("penciller")).toBe("Penciller");
    expect(getAuthorRoleLabel("inker")).toBe("Inker");
    expect(getAuthorRoleLabel("colorist")).toBe("Colorist");
    expect(getAuthorRoleLabel("letterer")).toBe("Letterer");
    expect(getAuthorRoleLabel("cover_artist")).toBe("Cover Artist");
    expect(getAuthorRoleLabel("editor")).toBe("Editor");
    expect(getAuthorRoleLabel("translator")).toBe("Translator");
  });

  it("falls back to the raw role string for unknown values", () => {
    expect(getAuthorRoleLabel("illustrator")).toBe("illustrator");
  });

  it("returns null for undefined, null, and empty string", () => {
    expect(getAuthorRoleLabel(undefined)).toBeNull();
    expect(getAuthorRoleLabel(null)).toBeNull();
    expect(getAuthorRoleLabel("")).toBeNull();
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `pnpm exec vitest run app/utils/authorRoles.test.ts`

Expected: FAIL with a module-not-found error for `./authorRoles`.

- [ ] **Step 3: Write the minimal implementation**

Create `app/utils/authorRoles.ts`:

```ts
/**
 * Maps a canonical author role string (as stored on the Author model, e.g.
 * sourced from CBZ ComicInfo.xml creator fields) to its display label.
 *
 * Returns null when the role is missing, so callers can conditionally render.
 */
export function getAuthorRoleLabel(
  role: string | undefined | null,
): string | null {
  if (!role) return null;
  const roleLabels: Record<string, string> = {
    writer: "Writer",
    penciller: "Penciller",
    inker: "Inker",
    colorist: "Colorist",
    letterer: "Letterer",
    cover_artist: "Cover Artist",
    editor: "Editor",
    translator: "Translator",
  };
  return roleLabels[role] || role;
}
```

- [ ] **Step 4: Run the test and verify it passes**

Run: `pnpm exec vitest run app/utils/authorRoles.test.ts`

Expected: PASS, all 3 test cases green.

- [ ] **Step 5: Commit**

```bash
git add app/utils/authorRoles.ts app/utils/authorRoles.test.ts
git commit -m "[Frontend] Add getAuthorRoleLabel helper"
```

---

## Task 2: Migrate `BookDetail.tsx` to use the shared helper

**Files:**
- Modify: `app/components/pages/BookDetail.tsx` (lines 122–135 to delete, line 1218 to update)

No new test is needed — this is a pure rename refactor of an internal helper already covered by existing (manual) BookDetail rendering.

- [ ] **Step 1: Add the import**

At the top of `app/components/pages/BookDetail.tsx`, add the import alongside the other `@/utils/...` imports (keep imports sorted to satisfy ESLint):

```ts
import { getAuthorRoleLabel } from "@/utils/authorRoles";
```

- [ ] **Step 2: Delete the inline `getRoleLabel` function**

Remove the whole block at lines 122–135:

```ts
const getRoleLabel = (role: string | undefined): string | null => {
  if (!role) return null;
  const roleLabels: Record<string, string> = {
    writer: "Writer",
    penciller: "Penciller",
    inker: "Inker",
    colorist: "Colorist",
    letterer: "Letterer",
    cover_artist: "Cover Artist",
    editor: "Editor",
    translator: "Translator",
  };
  return roleLabels[role] || role;
};
```

- [ ] **Step 3: Update the call site**

At line 1218, change:

```tsx
const roleLabel = getRoleLabel(author.role);
```

to:

```tsx
const roleLabel = getAuthorRoleLabel(author.role);
```

- [ ] **Step 4: Run TypeScript and ESLint to verify no regressions**

Run: `pnpm exec tsc --noEmit && pnpm exec eslint app/components/pages/BookDetail.tsx`

Expected: both commands exit 0 with no output.

- [ ] **Step 5: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "[Frontend] Use shared getAuthorRoleLabel helper in BookDetail"
```

---

## Task 3: Update identify search result row

**Files:**
- Modify: `app/components/library/IdentifyBookDialog.tsx` (lines 154–158 and line 449)

No unit test — `IdentifyBookDialog` has no component tests today and this is a narrow display change best verified manually. Manual verification is in Task 5.

- [ ] **Step 1: Add the import**

At the top of `app/components/library/IdentifyBookDialog.tsx`, add next to the other `@/utils` / `@/libraries` imports (keep sorted):

```ts
import { getAuthorRoleLabel } from "@/utils/authorRoles";
```

- [ ] **Step 2: Update `resolveAuthors` to return a formatted string**

Replace the current definition at lines 154–158:

```tsx
const resolveAuthors = (result: PluginSearchResult): string[] | undefined => {
  if (result.authors && result.authors.length > 0)
    return result.authors.map((a) => a.name);
  return undefined;
};
```

with:

```tsx
const resolveAuthors = (result: PluginSearchResult): string | undefined => {
  if (!result.authors || result.authors.length === 0) return undefined;
  return result.authors
    .map((a) => {
      const label = getAuthorRoleLabel(a.role);
      return label ? `${a.name} (${label})` : a.name;
    })
    .join(", ");
};
```

- [ ] **Step 3: Update the render site**

At roughly line 449 in the Authors section of the result row, change:

```tsx
<p className="text-sm text-muted-foreground">
  {authors.join(", ")}
</p>
```

to:

```tsx
<p className="text-sm text-muted-foreground">
  {authors}
</p>
```

The existing `hasAuthors = authors && authors.length > 0` check on the line above still works correctly with the new `string` shape — do not modify it.

- [ ] **Step 4: Run TypeScript and ESLint on the file**

Run: `pnpm exec tsc --noEmit && pnpm exec eslint app/components/library/IdentifyBookDialog.tsx`

Expected: both commands exit 0 with no output.

- [ ] **Step 5: Commit**

```bash
git add app/components/library/IdentifyBookDialog.tsx
git commit -m "[Frontend] Show author roles in identify search result row"
```

---

## Task 4: Add role-aware author chip input and update the review form

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx` (add `AuthorTagInput` near `TagInput` ~line 320; update Authors `FieldWrapper` ~line 790)

No new unit test — `IdentifyReviewForm` has no component tests today and the logic is visual. Manual verification is in Task 5.

- [ ] **Step 1: Add the import**

At the top of `app/components/library/IdentifyReviewForm.tsx`, add next to the other `@/` imports:

```ts
import { getAuthorRoleLabel } from "@/utils/authorRoles";
```

- [ ] **Step 2: Add the `AuthorTagInput` component**

Immediately after the closing `}` of the `TagInput` function (around line 387), add a new component. The existing `TagInput` is a generic `string[]` editor; this is its role-aware sibling that operates on `AuthorEntry[]` and keys chips by index so same-name-different-role entries can be removed independently.

```tsx
function AuthorTagInput({
  authors,
  onChange,
  disabled,
  placeholder,
}: {
  authors: AuthorEntry[];
  onChange: (authors: AuthorEntry[]) => void;
  disabled?: boolean;
  placeholder?: string;
}) {
  const [input, setInput] = useState("");

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && input.trim()) {
      e.preventDefault();
      onChange([...authors, { name: input.trim(), role: undefined }]);
      setInput("");
    }
    if (e.key === "Backspace" && !input && authors.length > 0) {
      onChange(authors.slice(0, -1));
    }
  };

  return (
    <div
      className={cn(
        "flex flex-wrap gap-1.5 rounded-md border border-input bg-transparent p-2 min-h-[36px]",
        disabled && "opacity-50 cursor-not-allowed",
      )}
    >
      {authors.map((author, i) => {
        const label = getAuthorRoleLabel(author.role);
        return (
          <Badge
            className="max-w-full gap-1 pr-1"
            key={i}
            variant="secondary"
          >
            <span
              className="truncate"
              title={label ? `${author.name} (${label})` : author.name}
            >
              {author.name}
              {label && (
                <span className="text-muted-foreground ml-1">({label})</span>
              )}
            </span>
            {!disabled && (
              <button
                className="shrink-0 rounded-sm hover:bg-muted-foreground/20 p-0.5 cursor-pointer"
                onClick={() =>
                  onChange(authors.filter((_, j) => j !== i))
                }
                type="button"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </Badge>
        );
      })}
      {!disabled && (
        <input
          className="flex-1 min-w-[80px] bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={
            authors.length === 0 ? (placeholder ?? "Type and press Enter") : ""
          }
          type="text"
          value={input}
        />
      )}
    </div>
  );
}
```

Note on the `key={i}` — React typically warns against index keys, but the identify form's author list is a short, locally-managed, linearly-edited array where we specifically *need* positional identity to distinguish duplicates. The ESLint rule `react/no-array-index-key` is not enabled in this repo (verified against existing index-keyed patterns), so no disable comment is required. If ESLint complains at verification time, add `// eslint-disable-next-line react/no-array-index-key` above the `key={i}` line.

- [ ] **Step 3: Update the Authors `FieldWrapper`**

Find the Authors field block (around lines 790–815). Replace the entire block:

```tsx
{/* Authors */}
<FieldWrapper
  currentValue={
    currentAuthors.length > 0
      ? currentAuthors.map((a) => a.name).join(", ")
      : undefined
  }
  disabled={isDisabled("authors")}
  field="authors"
  onUseCurrent={() => setAuthors(currentAuthors)}
  status={defaults.authors.status}
>
  <TagInput
    disabled={isDisabled("authors")}
    onChange={(names) =>
      setAuthors(
        names.map((name) => {
          const existing = authors.find((a) => a.name === name);
          return existing ?? { name, role: undefined };
        }),
      )
    }
    placeholder="Add author..."
    tags={authors.map((a) => a.name)}
  />
</FieldWrapper>
```

with:

```tsx
{/* Authors */}
<FieldWrapper
  currentValue={
    currentAuthors.length > 0
      ? currentAuthors
          .map((a) => {
            const label = getAuthorRoleLabel(a.role);
            return label ? `${a.name} (${label})` : a.name;
          })
          .join(", ")
      : undefined
  }
  disabled={isDisabled("authors")}
  field="authors"
  onUseCurrent={() => setAuthors(currentAuthors)}
  status={defaults.authors.status}
>
  <AuthorTagInput
    authors={authors}
    disabled={isDisabled("authors")}
    onChange={setAuthors}
    placeholder="Add author..."
  />
</FieldWrapper>
```

Two changes to be aware of:

1. The `currentValue` summary now suffixes each author with `(Role)` when present, matching the search result row in Task 3.
2. The chip editor is now `AuthorTagInput`, which passes the full `AuthorEntry[]` through directly — no more lossy `string[]` round-trip via `find((a) => a.name === name)`.

- [ ] **Step 4: Run TypeScript and ESLint on the file**

Run: `pnpm exec tsc --noEmit && pnpm exec eslint app/components/library/IdentifyReviewForm.tsx`

Expected: both commands exit 0 with no output. If ESLint complains about `key={i}`, add a `// eslint-disable-next-line react/no-array-index-key` comment above the `key={i}` line as noted in Step 2, then re-run.

- [ ] **Step 5: Commit**

```bash
git add app/components/library/IdentifyReviewForm.tsx
git commit -m "[Frontend] Show author roles in identify review form"
```

---

## Task 5: Full validation and manual verification

**Files:** none modified — verification only.

- [ ] **Step 1: Run the full check suite**

Run: `mise check:quiet`

Expected: pass. The output is one-line per step; if any step fails, the verbose output appears inline. Fix any issues before proceeding.

- [ ] **Step 2: Start the dev server and manually verify the three touchpoints**

Start: `mise start`

Navigate to a library that contains a CBZ manga with a person assigned to multiple author roles (e.g., Demon Slayer: Kimetsu no Yaiba v001 with `Gotouge Koyoharu` as both Writer and Penciller). If none exists, add one via the existing book edit dialog or use the sample library in `tmp/library/`.

Open the Identify dialog for that book and verify:

1. **Search result row** — the author line reads `Gotouge Koyoharu (Writer), Gotouge Koyoharu (Penciller)` instead of `Gotouge Koyoharu, Gotouge Koyoharu`.
2. **"Use current" summary** — click into a result to enter review mode; the muted Current bar above the Authors field shows the same role-suffixed string.
3. **Editable chips** — the Authors field displays two distinct chips: `Gotouge Koyoharu (Writer) ×` and `Gotouge Koyoharu (Penciller) ×`. Clicking the `×` on one chip removes only that chip; the other remains. Typing a new author name and pressing Enter adds it as a bare `Gotouge Koyoharu` chip with no role suffix (confirming new entries default to `role: undefined`).
4. **Non-comic book smoke test** — open Identify on an EPUB or M4B book with ordinary (role-less) authors. Chips should render with just names, no trailing `()`.

- [ ] **Step 3: Stop the dev server**

Ctrl-C the `mise start` process.

- [ ] **Step 4: (Optional) Squash-review commit history**

You should now have four commits:

1. `[Frontend] Add getAuthorRoleLabel helper`
2. `[Frontend] Use shared getAuthorRoleLabel helper in BookDetail`
3. `[Frontend] Show author roles in identify search result row`
4. `[Frontend] Show author roles in identify review form`

Leave them as-is. They land cleanly as a sequence in a squash-merged PR and make review easier than one monolithic commit.

---

## Self-review checklist

- **Spec coverage:** All four changes listed in the spec's "Files touched" table are covered — Task 1 (helper + test), Task 2 (BookDetail), Task 3 (IdentifyBookDialog), Task 4 (IdentifyReviewForm). Manual verification in Task 5 covers the three identify touchpoints from the spec's Problem section.
- **Types:** `getAuthorRoleLabel(role: string | undefined | null): string | null` is consistent across Tasks 1–4. `resolveAuthors(result): string | undefined` is consistent across Task 3. `AuthorEntry` reused from its existing local definition in Task 4.
- **Naming:** Helper is `getAuthorRoleLabel` everywhere (distinct from the unrelated `sortRoles` in `app/utils/roles.ts`). Chip component is `AuthorTagInput`.
- **Docs:** Spec correctly notes no docs changes. No docs tasks in this plan.
