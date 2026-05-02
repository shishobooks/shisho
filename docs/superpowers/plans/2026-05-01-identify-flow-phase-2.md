# Identify flow redesign — Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the identify-review dialog around per-field opt-in (checkboxes), Book/File sections with sticky banners, smart defaults that respect primary-vs-non-primary file scope, and surface `file.Name` as a real per-field decision.

**Architecture:**
- Extend the existing `Checkbox` component to support a third `"indeterminate"` state (used as section-level and global "Apply all" mixed-state checkbox).
- New `StickySection` primitive wrapping the section banner (chevron + label + hint slot + count + section checkbox) and managing collapse state. Sticky CSS handled by caller container.
- Rewrite `IdentifyReviewForm` to organize fields into Book / File sections, each row carrying its own apply-toggle. Smart defaults choose initial check state. Submit emits only checked fields and includes `file_name` / `file_name_source` in the apply payload.
- File selection in `IdentifyBookDialog` now prefers a non-reviewed file, then primary, then first.
- Re-use the existing `EntityCombobox` / `MultiSelectCombobox` / `SortableEntityList` / `IdentifierEditor` for typeahead and composite rows. The Phase 3 "ComboboxTypeahead" unification is out of scope here — primitives stay parallel for this phase, but the new dialog will adopt them via thin wrappers / shared classnames.

**Tech Stack:**
- React 19 + TypeScript
- Tailwind CSS 4 (utility classes, `cn()` helper)
- Radix UI primitives (Checkbox already supports `indeterminate` via the `checked={"indeterminate"}` prop)
- `lucide-react` icons (`ChevronDown`, `ChevronRight`, `RefreshCcw`, `ArrowLeft`)
- Vitest + React Testing Library for tests

**Out of scope for Phase 2:**
- Plain-text date input (Phase 4).
- Migrating BookEditDialog / FileEditDialog onto the new primitives (Phase 3).
- A standalone `ComboboxTypeahead` shared primitive — keep using `EntityCombobox` / `MultiSelectCombobox`.
- Source attribution per-field beyond `file.NameSource` — Phase 1 backend already accepts `file_name`/`file_name_source`; per-field source overrides for book metadata are deferred.

---

## File Structure

**New files:**
- `app/components/library/IdentifySectionBanner.tsx` — sticky banner with chevron, label, hint slot, count, section-level checkbox.
- `app/components/library/IdentifySectionBanner.test.tsx` — banner tests.
- `app/components/library/identify-decisions.ts` — pure helpers for default-checkbox computation (primary vs non-primary), aggregation, mixed-state computation.
- `app/components/library/identify-decisions.test.ts` — pure unit tests for the helpers.

**Modified files:**
- `app/components/ui/checkbox.tsx` — add support for `"indeterminate"` value, with a horizontal-bar visual.
- `app/components/library/IdentifyBookDialog.tsx` — use `file.reviewed`-aware file selection.
- `app/components/library/IdentifyReviewForm.tsx` — full rewrite around sections + per-field decisions + Name field.
- `app/components/library/IdentifyReviewForm.test.tsx` — adapt existing assertions, add new tests for sections / decisions.
- `app/components/library/identify-utils.ts` — add `pickInitialFile` helper.
- `app/components/library/identify-utils.test.ts` — tests for `pickInitialFile`.
- `pkg/plugins/handler_apply_metadata.go` (only if necessary — verify with current tests it already accepts the omission semantics we need).

**Documentation:**
- `website/docs/identify.md` (or update existing identify-related doc) — describe the new per-field opt-in, Book vs File scoping, and sticky sections.

---

## Task 1: Extend Checkbox to support indeterminate

**Files:**
- Modify: `app/components/ui/checkbox.tsx`
- Add: `app/components/ui/checkbox.test.tsx`

Radix's `CheckboxPrimitive.Root` already supports `checked === "indeterminate"`. We need to render a horizontal bar in that state.

- [ ] **Step 1: Write failing test for indeterminate render**

```tsx
// app/components/ui/checkbox.test.tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Checkbox } from "./checkbox";

describe("Checkbox", () => {
  it("renders indeterminate state with horizontal bar", () => {
    render(<Checkbox checked="indeterminate" aria-label="mixed" />);
    const checkbox = screen.getByRole("checkbox", { name: "mixed" });
    expect(checkbox).toHaveAttribute("data-state", "indeterminate");
    expect(checkbox.querySelector('[data-slot="checkbox-indicator"]')).not.toBeNull();
  });

  it("renders unchecked state", () => {
    render(<Checkbox checked={false} aria-label="off" />);
    expect(screen.getByRole("checkbox", { name: "off" })).toHaveAttribute(
      "data-state",
      "unchecked",
    );
  });

  it("renders checked state with check icon", () => {
    render(<Checkbox checked={true} aria-label="on" />);
    expect(screen.getByRole("checkbox", { name: "on" })).toHaveAttribute(
      "data-state",
      "checked",
    );
  });
});
```

- [ ] **Step 2: Run test, expect FAIL**

```bash
cd /Users/robinjoseph/.worktrees/shisho/second-identify && pnpm vitest run app/components/ui/checkbox.test.tsx
```

- [ ] **Step 3: Update Checkbox component to render bar in indeterminate**

Replace `app/components/ui/checkbox.tsx` with:

```tsx
import * as CheckboxPrimitive from "@radix-ui/react-checkbox";
import { CheckIcon } from "lucide-react";
import * as React from "react";

import { cn } from "@/libraries/utils";

function Checkbox({
  className,
  ...props
}: React.ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        "peer border-input bg-background data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground data-[state=indeterminate]:bg-primary data-[state=indeterminate]:text-primary-foreground data-[state=checked]:border-primary data-[state=indeterminate]:border-primary focus-visible:border-ring focus-visible:ring-ring/50 aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive dark:bg-input/30 size-4 shrink-0 rounded-[4px] border shadow-xs transition-shadow outline-none focus-visible:ring-[3px] cursor-pointer disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        data-slot="checkbox-indicator"
        className="flex items-center justify-center text-current transition-none"
      >
        {props.checked === "indeterminate" ? (
          <span className="block h-[2px] w-2.5 rounded-sm bg-current" aria-hidden />
        ) : (
          <CheckIcon className="size-3.5" />
        )}
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  );
}

export { Checkbox };
```

- [ ] **Step 4: Run test, expect PASS**

```bash
pnpm vitest run app/components/ui/checkbox.test.tsx
```

- [ ] **Step 5: Commit**

```bash
git add app/components/ui/checkbox.tsx app/components/ui/checkbox.test.tsx
git commit -m "[Frontend] Add indeterminate state to Checkbox component"
```

---

## Task 2: identify-decisions helpers (pure logic)

**Files:**
- Create: `app/components/library/identify-decisions.ts`
- Create: `app/components/library/identify-decisions.test.ts`

Pure helpers for: (a) computing the initial `decision` (`true` | `false`) for every field given its scope, status, and primary-status of the file; (b) aggregating child decisions into a section-level state (`true` | `false` | `"indeterminate"`); (c) mapping the dialog's row decisions onto the apply payload.

- [ ] **Step 1: Write failing tests for `defaultDecision`**

Create `app/components/library/identify-decisions.test.ts`:

```ts
import { describe, expect, it } from "vitest";

import {
  type FieldDecisionInput,
  aggregateDecisions,
  defaultDecision,
} from "./identify-decisions";

describe("defaultDecision", () => {
  it("returns true for new book-level fields", () => {
    expect(
      defaultDecision({ scope: "book", status: "new", isPrimaryFile: false }),
    ).toBe(true);
    expect(
      defaultDecision({ scope: "book", status: "new", isPrimaryFile: true }),
    ).toBe(true);
  });

  it("returns true for changed book-level fields on primary file", () => {
    expect(
      defaultDecision({ scope: "book", status: "changed", isPrimaryFile: true }),
    ).toBe(true);
  });

  it("returns false for changed book-level fields on non-primary file", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "changed",
        isPrimaryFile: false,
      }),
    ).toBe(false);
  });

  it("returns true for any file-level field state", () => {
    expect(
      defaultDecision({ scope: "file", status: "new", isPrimaryFile: false }),
    ).toBe(true);
    expect(
      defaultDecision({ scope: "file", status: "changed", isPrimaryFile: false }),
    ).toBe(true);
  });

  it("returns false for unchanged fields regardless of scope", () => {
    expect(
      defaultDecision({
        scope: "book",
        status: "unchanged",
        isPrimaryFile: true,
      }),
    ).toBe(false);
    expect(
      defaultDecision({
        scope: "file",
        status: "unchanged",
        isPrimaryFile: false,
      }),
    ).toBe(false);
  });
});

describe("aggregateDecisions", () => {
  it("returns false when all decisions are false", () => {
    expect(aggregateDecisions([false, false, false])).toBe(false);
  });
  it("returns true when all decisions are true", () => {
    expect(aggregateDecisions([true, true])).toBe(true);
  });
  it("returns indeterminate for mixed", () => {
    expect(aggregateDecisions([true, false])).toBe("indeterminate");
  });
  it("returns false for empty", () => {
    expect(aggregateDecisions([])).toBe(false);
  });
});
```

- [ ] **Step 2: Run, expect FAIL**

```bash
pnpm vitest run app/components/library/identify-decisions.test.ts
```

- [ ] **Step 3: Implement helpers**

Create `app/components/library/identify-decisions.ts`:

```ts
import type { FieldStatus } from "./identify-utils";

export type FieldScope = "book" | "file";

export interface FieldDecisionInput {
  scope: FieldScope;
  status: FieldStatus;
  isPrimaryFile: boolean;
}

/** Default per-field checkbox state at dialog open.
 * - File-level fields: ON whenever there's something to apply.
 * - Book-level new: ON.
 * - Book-level changed: ON only on the primary file.
 * - Unchanged: OFF.
 * See spec "Smart defaults" section. */
export function defaultDecision({
  scope,
  status,
  isPrimaryFile,
}: FieldDecisionInput): boolean {
  if (status === "unchanged") return false;
  if (scope === "file") return true;
  // scope === "book"
  if (status === "new") return true;
  // status === "changed"
  return isPrimaryFile;
}

/** Combine child decisions into a section/global indeterminate state. */
export function aggregateDecisions(
  decisions: boolean[],
): boolean | "indeterminate" {
  if (decisions.length === 0) return false;
  const allTrue = decisions.every(Boolean);
  if (allTrue) return true;
  const anyTrue = decisions.some(Boolean);
  return anyTrue ? "indeterminate" : false;
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
pnpm vitest run app/components/library/identify-decisions.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add app/components/library/identify-decisions.ts app/components/library/identify-decisions.test.ts
git commit -m "[Frontend] Add identify decision helpers for per-field defaults"
```

---

## Task 3: pickInitialFile helper

**Files:**
- Modify: `app/components/library/identify-utils.ts`
- Modify: `app/components/library/identify-utils.test.ts`

The dialog must pick the right file to identify on open: prefer a non-reviewed main file, then the primary file, then the first file.

- [ ] **Step 1: Write failing tests**

Append to `app/components/library/identify-utils.test.ts`:

```ts
import { pickInitialFile } from "./identify-utils";
import type { Book, File } from "@/types";

describe("pickInitialFile", () => {
  function file(id: number, opts: Partial<File> = {}): File {
    return {
      id,
      file_role: "main",
      reviewed: undefined,
      ...opts,
    } as File;
  }

  it("returns undefined when there are no main files", () => {
    expect(
      pickInitialFile({
        files: [file(1, { file_role: "supplement" })],
        primary_file_id: undefined,
      } as unknown as Book),
    ).toBeUndefined();
  });

  it("prefers a non-reviewed file when some are reviewed", () => {
    const files = [
      file(1, { reviewed: true }),
      file(2, { reviewed: false }),
      file(3, { reviewed: true }),
    ];
    expect(
      pickInitialFile({ files, primary_file_id: 1 } as unknown as Book)?.id,
    ).toBe(2);
  });

  it("treats reviewed=undefined as non-reviewed", () => {
    const files = [
      file(1, { reviewed: true }),
      file(2, { reviewed: undefined }),
    ];
    expect(
      pickInitialFile({ files, primary_file_id: 1 } as unknown as Book)?.id,
    ).toBe(2);
  });

  it("when all reviewed equal, prefers primary", () => {
    const files = [file(1), file(2), file(3)];
    expect(
      pickInitialFile({ files, primary_file_id: 2 } as unknown as Book)?.id,
    ).toBe(2);
  });

  it("falls back to first when primary not set", () => {
    const files = [
      file(10, { reviewed: true }),
      file(20, { reviewed: true }),
    ];
    expect(
      pickInitialFile({ files, primary_file_id: undefined } as unknown as Book)
        ?.id,
    ).toBe(10);
  });
});
```

- [ ] **Step 2: Run, expect FAIL**

```bash
pnpm vitest run app/components/library/identify-utils.test.ts
```

- [ ] **Step 3: Implement `pickInitialFile`**

Append to `app/components/library/identify-utils.ts`:

```ts
import type { Book, File } from "@/types";

/** Choose which file to identify when the dialog opens.
 *
 * 1. Prefer a non-reviewed main file (reviewed !== true).
 * 2. If all share the same reviewed status, prefer book.primary_file_id.
 * 3. Otherwise, the first main file.
 *
 * Returns undefined when there are no main files. */
export function pickInitialFile(book: Book): File | undefined {
  const mains = (book.files ?? []).filter((f) => f.file_role === "main");
  if (mains.length === 0) return undefined;
  const nonReviewed = mains.find((f) => f.reviewed !== true);
  if (nonReviewed && mains.some((f) => f.reviewed === true)) return nonReviewed;
  if (book.primary_file_id != null) {
    const primary = mains.find((f) => f.id === book.primary_file_id);
    if (primary) return primary;
  }
  return mains[0];
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
pnpm vitest run app/components/library/identify-utils.test.ts
```

- [ ] **Step 5: Commit**

```bash
git add app/components/library/identify-utils.ts app/components/library/identify-utils.test.ts
git commit -m "[Frontend] Add pickInitialFile helper for identify dialog"
```

---

## Task 4: IdentifySectionBanner component

**Files:**
- Create: `app/components/library/IdentifySectionBanner.tsx`
- Create: `app/components/library/IdentifySectionBanner.test.tsx`

Sticky banner: chevron (rotates when collapsed) + label + slot + count + section checkbox. Click on banner toggles collapse. Click on checkbox stops propagation.

- [ ] **Step 1: Write failing tests**

```tsx
// app/components/library/IdentifySectionBanner.test.tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { IdentifySectionBanner } from "./IdentifySectionBanner";

describe("IdentifySectionBanner", () => {
  function renderBanner(props?: Partial<React.ComponentProps<typeof IdentifySectionBanner>>) {
    const handlers = {
      onToggleCollapse: vi.fn(),
      onCheckedChange: vi.fn(),
    };
    render(
      <IdentifySectionBanner
        label="BOOK"
        hint="applies to all files"
        selectedCount={2}
        totalCount={5}
        collapsed={false}
        checkboxState={true}
        {...handlers}
        {...props}
      />,
    );
    return handlers;
  }

  it("shows label, hint, and count", () => {
    renderBanner();
    expect(screen.getByText("BOOK")).toBeInTheDocument();
    expect(screen.getByText("applies to all files")).toBeInTheDocument();
    expect(screen.getByText(/2.*of.*5.*selected/i)).toBeInTheDocument();
  });

  it("toggles collapse when banner is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onToggleCollapse } = renderBanner();
    await user.click(screen.getByRole("button", { name: /toggle book section/i }));
    expect(onToggleCollapse).toHaveBeenCalledTimes(1);
  });

  it("does not collapse when checkbox is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onToggleCollapse, onCheckedChange } = renderBanner();
    await user.click(screen.getByRole("checkbox"));
    expect(onCheckedChange).toHaveBeenCalled();
    expect(onToggleCollapse).not.toHaveBeenCalled();
  });

  it("renders indeterminate checkbox state", () => {
    renderBanner({ checkboxState: "indeterminate" });
    expect(screen.getByRole("checkbox")).toHaveAttribute(
      "data-state",
      "indeterminate",
    );
  });
});
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement component**

```tsx
// app/components/library/IdentifySectionBanner.tsx
import { ChevronDown } from "lucide-react";
import type * as React from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "@/libraries/utils";

interface Props {
  label: string;
  hint?: React.ReactNode;
  selectedCount: number;
  totalCount: number;
  collapsed: boolean;
  checkboxState: boolean | "indeterminate";
  onToggleCollapse: () => void;
  onCheckedChange: (checked: boolean) => void;
  className?: string;
}

export function IdentifySectionBanner({
  label,
  hint,
  selectedCount,
  totalCount,
  collapsed,
  checkboxState,
  onToggleCollapse,
  onCheckedChange,
  className,
}: Props) {
  return (
    <div
      className={cn(
        "sticky z-[2] grid grid-cols-[24px_minmax(0,1fr)_auto_auto] items-center gap-3.5 border-b bg-muted/40 px-5 py-3",
        className,
      )}
    >
      <button
        aria-label={`Toggle ${label} section`}
        aria-expanded={!collapsed}
        type="button"
        onClick={onToggleCollapse}
        className="col-span-3 grid grid-cols-subgrid items-center gap-3.5 text-left cursor-pointer"
      >
        <ChevronDown
          aria-hidden
          className={cn(
            "size-3 text-muted-foreground transition-transform",
            collapsed && "-rotate-90",
          )}
        />
        <div className="flex min-w-0 items-center gap-3 truncate">
          <span className="text-[11px] font-bold uppercase tracking-[0.14em] text-foreground/90">
            {label}
          </span>
          {hint != null && (
            <span className="truncate text-[11.5px] text-muted-foreground">
              {hint}
            </span>
          )}
        </div>
        <span className="text-[11.5px] tabular-nums text-muted-foreground">
          <span className="font-semibold text-foreground">{selectedCount}</span>{" "}
          of {totalCount} selected
        </span>
      </button>
      <Checkbox
        aria-label={`Apply all ${label.toLowerCase()} fields`}
        checked={checkboxState}
        onCheckedChange={(v) => onCheckedChange(v === true)}
        // Stop click bubbling so the parent button's onClick doesn't toggle
        // collapse when the checkbox is clicked.
        onClick={(e) => e.stopPropagation()}
      />
    </div>
  );
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
pnpm vitest run app/components/library/IdentifySectionBanner.test.tsx
```

- [ ] **Step 5: Commit**

```bash
git add app/components/library/IdentifySectionBanner.tsx app/components/library/IdentifySectionBanner.test.tsx
git commit -m "[Frontend] Add IdentifySectionBanner sticky section component"
```

---

## Task 5: IdentifyBookDialog uses pickInitialFile

**Files:**
- Modify: `app/components/library/IdentifyBookDialog.tsx`

- [ ] **Step 1: Update initial file selection**

In `IdentifyBookDialog.tsx`, replace the logic that picks the initial file with `pickInitialFile(book)`. Specifically:

```ts
// Inside the open useEffect:
const initialFile = pickInitialFile(book);
const initialFileId = initialFile?.id;
const initialQuery = initialFile?.name || initialFile ? (initialFile?.name ?? book.title) : book.title;
const initialIds = (initialFile?.identifiers ?? []).map((id) => ({
  type: id.type,
  value: id.value,
}));
```

Also update the `selectedFile` derivation:

```ts
const selectedFile = selectedFileId
  ? mainFiles.find((f) => f.id === selectedFileId)
  : pickInitialFile(book);
```

Add the import:

```ts
import { computeIdentifyEmptyState, pickInitialFile } from "./identify-utils";
```

- [ ] **Step 2: Run lint + typecheck**

```bash
pnpm lint:eslint && pnpm lint:types
```

- [ ] **Step 3: Run dialog-related tests**

```bash
pnpm vitest run app/components/library
```

- [ ] **Step 4: Commit**

```bash
git add app/components/library/IdentifyBookDialog.tsx
git commit -m "[Frontend] Pick non-reviewed/primary/first file when opening identify"
```

---

## Task 6: Rewrite IdentifyReviewForm — section + per-field decisions

This is the largest task. The form gains:
- Top sticky select-all bar
- Two sticky section banners (Book / File) wrapping their fields
- Each row has a checkbox; checked = field is included in apply payload
- Smart defaults via `defaultDecision`
- A new `Name` (file.Name) row with "Copy from book title" inline button
- Footer: "Restore suggestions" (left) + per-section counts + Cancel/Apply
- Apply submit only includes checked fields; emits `file_name` and `file_name_source` when Name is checked

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/IdentifyReviewForm.test.tsx`

Approach: write the complete component, then update tests. The component is large but largely a reorganization of existing code.

- [ ] **Step 1: Sketch the component skeleton**

Open `IdentifyReviewForm.tsx`. Add new types/state at the top of the component:

```ts
type FieldKey =
  // book-level
  | "title"
  | "subtitle"
  | "authors"
  | "series"
  | "genres"
  | "tags"
  | "description"
  // file-level
  | "cover"
  | "name"
  | "narrators"
  | "publisher"
  | "imprint"
  | "language"
  | "release_date"
  | "url"
  | "identifiers"
  | "abridged";

const BOOK_FIELDS: FieldKey[] = [
  "title", "subtitle", "authors", "series", "genres", "tags", "description",
];
const FILE_FIELDS: FieldKey[] = [
  "cover", "name", "narrators", "publisher", "imprint",
  "language", "release_date", "url", "identifiers", "abridged",
];
```

For each field, compute `status: FieldStatus` (already done by `defaults`), then compute the default decision via `defaultDecision({ scope, status, isPrimaryFile })`. The scope per field:

- Book scope: `title, subtitle, authors, series, genres, tags, description`.
- File scope: `cover, name, narrators, publisher, imprint, language, release_date, url, identifiers, abridged`.

Derive `isPrimaryFile = book.primary_file_id === fileId` (treat undefined `primary_file_id` and the lone file as primary).

- [ ] **Step 2: Add decision state + handlers**

```ts
// In component:
const isPrimaryFile = useMemo(() => {
  if (book.primary_file_id == null) {
    // Single-file books default to primary
    return (book.files?.length ?? 0) <= 1;
  }
  return book.primary_file_id === fileId;
}, [book.primary_file_id, book.files?.length, fileId]);

const fieldScope = (k: FieldKey): "book" | "file" =>
  BOOK_FIELDS.includes(k) ? "book" : "file";

const fieldStatus = useMemo<Record<FieldKey, FieldStatus>>(() => ({
  title: defaults.title.status,
  subtitle: defaults.subtitle.status,
  authors: defaults.authors.status,
  series:
    defaults.series.status === "changed" ||
    defaults.seriesNumber.status === "changed" ||
    defaults.seriesNumberUnit.status === "changed"
      ? "changed"
      : defaults.series.status === "new" ||
          defaults.seriesNumber.status === "new" ||
          defaults.seriesNumberUnit.status === "new"
        ? "new"
        : "unchanged",
  genres: defaults.genres.status,
  tags: defaults.tags.status,
  description: defaults.description.status,
  cover: hasCoverChoice && !preferCurrentCover ? (currentCoverUrl ? "changed" : "new") : "unchanged",
  name:
    (file?.name ?? "") === (result.title ?? "")
      ? "unchanged"
      : (file?.name ?? "")
        ? "changed"
        : "new",
  narrators: defaults.narrators.status,
  publisher: defaults.publisher.status,
  imprint: defaults.imprint.status,
  language: defaults.language.status,
  release_date: defaults.releaseDate.status,
  url: defaults.url.status,
  identifiers: defaults.identifiers.status,
  abridged: defaults.abridged.status,
}), [defaults, hasCoverChoice, preferCurrentCover, currentCoverUrl, file?.name, result.title]);

const initialDecisions = useMemo<Record<FieldKey, boolean>>(() => {
  const out = {} as Record<FieldKey, boolean>;
  ([...BOOK_FIELDS, ...FILE_FIELDS] as FieldKey[]).forEach((k) => {
    if (disabledFields.has(k)) {
      out[k] = false;
      return;
    }
    out[k] = defaultDecision({
      scope: fieldScope(k),
      status: fieldStatus[k],
      isPrimaryFile,
    });
  });
  return out;
}, [disabledFields, fieldStatus, isPrimaryFile]);

const [decisions, setDecisions] = useState<Record<FieldKey, boolean>>(
  initialDecisions,
);

const setDecision = (k: FieldKey, v: boolean) =>
  setDecisions((prev) => ({ ...prev, [k]: v }));

const setSectionDecisions = (keys: FieldKey[], v: boolean) =>
  setDecisions((prev) => {
    const next = { ...prev };
    keys.forEach((k) => {
      if (!disabledFields.has(k)) next[k] = v;
    });
    return next;
  });
```

Also add the Name field state + initial value:

```ts
const initialName = useMemo(() => {
  // Default name is the plugin's proposed title; that's what `persistMetadata`
  // historically copied. Surfacing it makes the override explicit.
  return result.title ?? "";
}, [result.title]);
const [name, setName] = useState(initialName);
const nameSource: "plugin" | "user" = name === initialName ? "plugin" : "user";
```

- [ ] **Step 3: Add Restore suggestions**

```ts
const restoreSuggestions = () => {
  setDecisions(initialDecisions);
  setTitle(defaults.title.value);
  setSubtitle(defaults.subtitle.value);
  setDescription(defaults.description.value);
  setAuthors(defaults.authors.value);
  setNarrators(defaults.narrators.value);
  setSeries(defaults.series.value);
  setSeriesNumber(defaults.seriesNumber.value);
  setSeriesNumberUnit(defaults.seriesNumberUnit.value);
  setGenres(defaults.genres.value);
  setTags(defaults.tags.value);
  setPublisher(defaults.publisher.value);
  setImprint(defaults.imprint.value);
  setReleaseDate(defaults.releaseDate.value);
  setUrl(defaults.url.value);
  setLanguage(defaults.language.value);
  setAbridged(defaults.abridged.value);
  setIdentifiers(defaults.identifiers.value);
  setUserCoverSelection(null);
  setName(initialName);
};
```

- [ ] **Step 4: Update submit to honor decisions**

Replace the current `handleSubmit` body so the `fields` map only contains keys for which `decisions[k]` is true. For example:

```ts
const fields: Record<string, unknown> = {};
if (decisions.title) fields.title = title;
if (decisions.subtitle) fields.subtitle = subtitle;
if (decisions.authors) fields.authors = authors.map((a) => ({ name: a.name, role: a.role }));
if (decisions.series) {
  fields.series = series;
  fields.series_number = seriesNumber !== "" ? parseFloat(seriesNumber) : undefined;
  fields.series_number_unit = seriesNumberUnit !== "" ? seriesNumberUnit : undefined;
}
if (decisions.genres) fields.genres = genres;
if (decisions.tags) fields.tags = tags;
if (decisions.description) fields.description = description;
if (decisions.narrators) fields.narrators = narrators;
if (decisions.publisher) fields.publisher = publisher;
if (decisions.imprint) fields.imprint = imprint;
if (decisions.language) fields.language = language;
if (decisions.release_date) fields.release_date = releaseDate;
if (decisions.url) fields.url = url;
if (decisions.identifiers)
  fields.identifiers = identifiers.map((id) => ({ type: id.type, value: id.value }));
if (decisions.abridged && abridged !== null) fields.abridged = abridged;
if (decisions.cover && coverSelection === "new") {
  if (newCoverUrl) fields.cover_url = newCoverUrl;
  else if (newCoverPage != null) fields.cover_page = newCoverPage;
}

const payload: Parameters<typeof applyMutation.mutateAsync>[0] = {
  book_id: book.id,
  file_id: fileId,
  fields,
  plugin_scope: result.plugin_scope,
  plugin_id: result.plugin_id,
};
if (decisions.name) {
  (payload as { file_name?: string; file_name_source?: string }).file_name = name;
  (payload as { file_name?: string; file_name_source?: string }).file_name_source = nameSource;
}
```

- [ ] **Step 5: Wire up section-level state and the JSX**

Replace the body of the form's `return` with the new layout. High-level structure:

```tsx
<div className="flex flex-col h-full">
  {/* Header */}
  <div className="flex items-center gap-2 px-5 py-3 border-b">
    <Button variant="ghost" size="sm" onClick={onBack}>
      <ArrowLeft className="h-4 w-4" />
    </Button>
    <div className="min-w-0">
      <h3 className="truncate text-sm font-semibold">Review changes</h3>
      <p className="truncate text-xs text-muted-foreground">
        {summaryLine}
      </p>
    </div>
  </div>
  {/* Scroll body */}
  <div className="flex-1 overflow-y-auto">
    {/* Sticky select-all bar */}
    <div className="sticky top-0 z-[3] flex items-center gap-3.5 border-b bg-background/95 px-5 py-2.5 backdrop-blur">
      <Checkbox
        aria-label="Apply all"
        checked={globalCheckboxState}
        onCheckedChange={(v) =>
          setSectionDecisions(
            [...BOOK_FIELDS, ...FILE_FIELDS],
            v === true,
          )
        }
      />
      <span className="text-xs font-medium">Apply all</span>
      <span className="ml-auto text-[11.5px] tabular-nums text-muted-foreground">
        <span className="font-semibold text-foreground">{totalSelected}</span> of {totalApplicable} selected
      </span>
    </div>

    {/* Book section */}
    <IdentifySectionBanner
      label="BOOK"
      hint="applies to all files"
      selectedCount={bookSelectedCount}
      totalCount={bookApplicableCount}
      collapsed={bookCollapsed}
      checkboxState={bookCheckboxState}
      onToggleCollapse={() => setBookCollapsed((c) => !c)}
      onCheckedChange={(v) => setSectionDecisions(BOOK_FIELDS, v)}
      className="top-[41px]"
    />
    {!bookCollapsed && (
      <div>
        {/* Each row uses <FieldRow> wrapper documented below */}
      </div>
    )}

    {/* File section */}
    <IdentifySectionBanner
      label="FILE"
      hint={fileSectionHint}
      selectedCount={fileSelectedCount}
      totalCount={fileApplicableCount}
      collapsed={fileCollapsed}
      checkboxState={fileCheckboxState}
      onToggleCollapse={() => setFileCollapsed((c) => !c)}
      onCheckedChange={(v) => setSectionDecisions(FILE_FIELDS, v)}
      className="top-[41px]"
    />
    {!fileCollapsed && (
      <div>
        {/* file rows */}
      </div>
    )}
  </div>

  {/* Footer */}
  <div className="flex items-center justify-between border-t px-5 py-3">
    <Button variant="ghost" size="sm" onClick={restoreSuggestions}>
      <RefreshCcw className="h-3.5 w-3.5 mr-1.5" /> Restore suggestions
    </Button>
    <div className="flex items-center gap-3">
      <span className="hidden sm:block text-xs text-muted-foreground">
        <strong className="text-foreground font-semibold">{bookSelectedCount} book changes</strong> · <strong className="text-foreground font-semibold">{fileSelectedCount} file changes</strong> selected
      </span>
      <Button variant="outline" onClick={onClose} disabled={applyMutation.isPending}>Cancel</Button>
      <Button onClick={handleSubmit} disabled={applyMutation.isPending || totalSelected === 0}>
        {applyMutation.isPending ? <><Loader2 className="h-4 w-4 animate-spin mr-2" />Applying…</> : `Apply ${totalSelected} change${totalSelected === 1 ? "" : "s"}`}
      </Button>
    </div>
  </div>
</div>
```

Each field row uses a small `FieldRow` helper that wraps:

```tsx
function FieldRow({
  fieldKey,
  status,
  label,
  decision,
  setDecision,
  currentValue,
  disabled,
  children,
  inlineAction,
}: {
  fieldKey: FieldKey;
  status: FieldStatus;
  label: string;
  decision: boolean;
  setDecision: (v: boolean) => void;
  currentValue?: React.ReactNode;
  disabled?: boolean;
  children: React.ReactNode;
  inlineAction?: React.ReactNode;
}) {
  return (
    <div
      className={cn(
        "grid grid-cols-[24px_minmax(0,1fr)] gap-3.5 px-5 py-4 border-b last:border-b-0",
        status === "unchanged" && "opacity-60 hover:opacity-100 transition-opacity",
        disabled && "opacity-50",
      )}
    >
      <Checkbox
        aria-label={`Apply ${label}`}
        checked={decision && !disabled}
        disabled={disabled}
        onCheckedChange={(v) => setDecision(v === true)}
      />
      <div className="space-y-2 min-w-0">
        <div className="flex items-center gap-2">
          <Label className="text-sm font-semibold">{label}</Label>
          <StatusBadge status={status} />
          {inlineAction}
        </div>
        {children}
        {currentValue != null && status !== "unchanged" && (
          <p className="text-xs text-muted-foreground">
            <span className="font-medium">Currently:</span> {currentValue}
          </p>
        )}
      </div>
    </div>
  );
}
```

Compute the section-level state via `aggregateDecisions(bookKeys.filter(k => !disabledFields.has(k)).map(k => decisions[k]))`. Same for File. `totalSelected` = number of `true` decisions. `totalApplicable` = number of fields not disabled.

`fileSectionHint` is something like:

```ts
const fileSectionHint = useMemo(() => {
  if (!file) return null;
  const parts: string[] = [];
  parts.push(file.file_type.toUpperCase());
  if (file.name) parts.push(file.name);
  if (file.audiobook_duration_seconds) parts.push(formatDuration(file.audiobook_duration_seconds));
  if (file.audiobook_bitrate_bps) parts.push(`${Math.round(file.audiobook_bitrate_bps / 1000)} kbps`);
  if (file.page_count) parts.push(`${file.page_count} pages`);
  return parts.join(" · ");
}, [file]);
```

Default-collapsed: `bookCollapsed = bookSelectedCount === 0 && bookApplicableCount > 0`. Same for file. Compute initial collapse from initial decisions.

- [ ] **Step 6: Wire all field controls into the new structure**

For each existing field control (Title, Subtitle, Authors, Series, Genres, Tags, Description, Cover, Narrators, Publisher, Imprint, Language, Release date, URL, Identifiers, Abridged), move it into a `FieldRow`. Reuse the existing controls (EntityCombobox / SortableEntityList / MultiSelectCombobox / IdentifierEditor / Textarea / DatePicker / etc.).

Add the **Name** row (file scope):

```tsx
<FieldRow
  fieldKey="name"
  label="Name"
  status={fieldStatus.name}
  decision={decisions.name}
  setDecision={(v) => setDecision("name", v)}
  currentValue={file?.name || undefined}
  disabled={disabledFields.has("name")}
  inlineAction={
    title.trim() && title !== name ? (
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="ml-auto h-6 px-2 text-xs"
        onClick={() => setName(title)}
      >
        Copy from book title
      </Button>
    ) : null
  }
>
  <Input value={name} onChange={(e) => setName(e.target.value)} />
</FieldRow>
```

The Title field gains its `Extract subtitle` button via the existing component, kept as `inlineAction` on the title row.

- [ ] **Step 7: Update `hasChanges`**

Form has unsaved changes when:
- Any field value differs from defaults, OR
- Any decision differs from `initialDecisions`, OR
- The Name differs from `initialName`.

```ts
const hasChanges = useMemo(() => {
  if (name !== initialName) return true;
  for (const k of [...BOOK_FIELDS, ...FILE_FIELDS]) {
    if (decisions[k] !== initialDecisions[k]) return true;
  }
  // existing value comparisons
  return (
    title !== defaults.title.value ||
    subtitle !== defaults.subtitle.value ||
    description !== defaults.description.value ||
    !equal(authors, defaults.authors.value) ||
    !equal(narrators, defaults.narrators.value) ||
    series !== defaults.series.value ||
    seriesNumber !== defaults.seriesNumber.value ||
    seriesNumberUnit !== defaults.seriesNumberUnit.value ||
    !equal(genres, defaults.genres.value) ||
    !equal(tags, defaults.tags.value) ||
    publisher !== defaults.publisher.value ||
    imprint !== defaults.imprint.value ||
    releaseDate !== defaults.releaseDate.value ||
    url !== defaults.url.value ||
    language !== defaults.language.value ||
    abridged !== defaults.abridged.value ||
    !equal(identifiers, defaults.identifiers.value) ||
    coverSelection !== defaultCoverSelection
  );
}, [
  /* … all deps … */
]);
```

- [ ] **Step 8: Run typecheck + lint**

```bash
pnpm lint:types && pnpm lint:eslint
```

Resolve any errors before moving on.

- [ ] **Step 9: Run identify tests**

```bash
pnpm vitest run app/components/library
```

Existing tests will likely fail because of the structural change — update them in the next task before continuing.

- [ ] **Step 10: Commit**

```bash
git add app/components/library/IdentifyReviewForm.tsx
git commit -m "[Frontend] Rewrite IdentifyReviewForm with per-field decisions and sections"
```

---

## Task 7: Update / add IdentifyReviewForm tests

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.test.tsx`

The existing test file relies on the previous component shape (presence of "Use current" buttons, etc.). Update assertions to match the new shape and add coverage for:

1. Smart defaults: book-changed defaults OFF on a non-primary file.
2. Smart defaults: book-changed defaults ON on the primary file.
3. Unchecking a field omits it from the apply payload.
4. Section-level checkbox toggles all child rows.
5. Name field renders + applies with `file_name_source: "plugin"` when unedited and `"user"` when edited.
6. "Restore suggestions" resets the form.

- [ ] **Step 1: Inspect the existing test file**

```bash
$EDITOR app/components/library/IdentifyReviewForm.test.tsx
```

- [ ] **Step 2: Update the existing assertions**

Replace test selectors that look for "Use current" buttons (no longer present) with checkbox interactions. Where tests previously asserted on field values, assert on the apply mutation payload instead. Use `vi.mock` to spy on `usePluginApply`.

A representative new test:

```tsx
it("omits unchecked fields from the apply payload", async () => {
  const mutateAsync = vi.fn().mockResolvedValue(undefined);
  vi.mocked(usePluginApply).mockReturnValue({
    mutateAsync,
    isPending: false,
  } as unknown as ReturnType<typeof usePluginApply>);

  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  render(<IdentifyReviewForm /* primary book, single file, plugin proposes new title + new authors */ />);

  // Uncheck title
  await user.click(screen.getByRole("checkbox", { name: /apply title/i }));

  // Apply
  await user.click(screen.getByRole("button", { name: /apply.*changes?/i }));

  expect(mutateAsync).toHaveBeenCalledWith(
    expect.objectContaining({
      fields: expect.not.objectContaining({ title: expect.anything() }),
    }),
  );
});
```

- [ ] **Step 3: Run tests**

```bash
pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx
```

Iterate until all assertions pass.

- [ ] **Step 4: Commit**

```bash
git add app/components/library/IdentifyReviewForm.test.tsx
git commit -m "[Frontend] Update IdentifyReviewForm tests for per-field decisions"
```

---

## Task 8: Documentation

**Files:**
- Modify: `website/docs/identify.md` (create if missing; add to sidebar)
- Or: append a section to an existing identify-related page in `website/docs/`.

- [ ] **Step 1: Find the existing docs page**

```bash
grep -l -r "identify" website/docs/ | head -5
```

- [ ] **Step 2: Update or create the page**

Document:
- The new per-field opt-in checkboxes.
- Book vs File scope and why book-changed defaults differ on non-primary files.
- The Name field as a real, editable per-file value (and its independence from book.title).
- Restore suggestions button.

Keep it short — one short section is fine.

- [ ] **Step 3: Cross-link from related pages**

Add a link from `website/docs/metadata.md` (if it exists) and the relevant sidebar.

- [ ] **Step 4: Commit**

```bash
git add website/docs
git commit -m "[Docs] Document new identify per-field opt-in flow"
```

---

## Task 9: Final validation

- [ ] **Step 1: Run focused JS checks**

```bash
mise lint:js test:unit
```

- [ ] **Step 2: Mark Phase 2 complete in the spec**

In `docs/superpowers/specs/2026-05-01-identify-flow-design.md`, add `✅ shipped` to the Phase 2 header and a one-line summary.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/specs/2026-05-01-identify-flow-design.md
git commit -m "[Docs] Mark identify flow Phase 2 as complete"
```

- [ ] **Step 4: Ship via /ship-it**

Use the ship-it skill to push the branch, open a PR, and request review.
