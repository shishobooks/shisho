# Identify Review Form: Input Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring `IdentifyReviewForm` to input-level parity with `BookEditDialog` and `FileEditDialog` by extracting their inline entity-search and identifier-editing patterns into shared components, then reusing those components inside the review form with per-row diff badges and an "exists vs will be created" auto-match indicator.

**Architecture:**
- Extract three new shared components into `app/components/common/`: `EntityCombobox<T>` (server-backed search-or-create), `SortableEntityList<T>` (sortable rows of comboboxes with per-row extras), and `IdentifierEditor` (lifted from `FileEditDialog`).
- Extend `MultiSelectCombobox` with an optional `status` prop for per-chip badge support; existing call sites unaffected.
- Refactor `BookEditDialog` and `FileEditDialog` to consume the new components — mechanical lift-and-shift, no behavior change. Existing tests must continue to pass; new regression tests are added.
- Refactor `IdentifyReviewForm` to consume the same components and add the missing fields (sort title, publisher, imprint, release date with `DatePicker`, identifiers CRUD, narrators on M4B only).
- Backend update endpoints already accept entity name strings and create-on-the-fly during update (`UpdateBookPayload.authors: AuthorInput[]`, `UpdateFilePayload.publisher?: string`, etc.). The "will be created" pill is a *display-only* signal — apply path is unchanged: send names, server resolves/creates.
- A new `useAutoMatchEntities` hook resolves incoming plugin entity names against the local DB on form mount; matches render as plain chips, non-matches render with a "will be created" marker.
- `useUpdateBook` and `useUpdateFile` extended to invalidate entity-list queries (persons, series, publishers, imprints, genres, tags) so newly-created entities show up in list pages.

**Tech Stack:** React 19 + TypeScript, Tanstack Query, Radix UI primitives (Popover, Command), `@dnd-kit/core` + `@dnd-kit/sortable`, TailwindCSS, Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-04-25-identify-review-form-parity-design.md`

---

## Sequencing notes

- **Tasks 1–4** extract the new shared components and extend `MultiSelectCombobox`. Each component is built and unit-tested before any consumer changes — that keeps the failure surface small and gives us building blocks to compose.
- **Tasks 5–6** refactor the existing edit dialogs to consume the new components, with regression tests added alongside. After Task 6, the *only* code paths exercising the new components are the (now-refactored) edit dialogs — both forms must still pass their existing test suites plus the new regression cases.
- **Task 7** adds `useUpdateBook` / `useUpdateFile` invalidation for entity-list queries. Standalone, lands here so subsequent identify-form testing benefits from it.
- **Task 8** adds `useAutoMatchEntities`, the new lookup hook for "exists vs will be created" markers.
- **Tasks 9–17** refactor `IdentifyReviewForm` field by field. Each field gets its own task with TDD: test the new field renders correctly with the new input + status badges, then swap implementation, then verify.
- **Task 18** removes the dead `TagInput` / `AuthorTagInput` / `IdentifierTagInput` functions now unused.
- **Task 19** updates `website/docs/` if user-visible behavior changed (likely the "will be created" pill).
- **Task 20** runs `mise check:quiet` and addresses anything that surfaces.

---

## Task 1: Extract `EntityCombobox<T>` (new shared component)

**Files:**
- Create: `app/components/common/EntityCombobox.tsx`
- Create: `app/components/common/EntityCombobox.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// app/components/common/EntityCombobox.test.tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { EntityCombobox } from "./EntityCombobox";

interface Person { id: number; name: string }

function makeHook(items: Person[], isLoading = false) {
  return (_q: string) => ({ data: items, isLoading });
}

describe("EntityCombobox", () => {
  it("calls onChange with an existing match when the user selects it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    const hook = makeHook([{ id: 1, name: "Tor Books" }]);

    render(
      <EntityCombobox<Person>
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Publisher"
        onChange={onChange}
        value={null}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Tor Books"));

    expect(onChange).toHaveBeenCalledWith({ id: 1, name: "Tor Books" });
  });

  it("offers Create when typed value has no match and emits __create payload", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    const hook = makeHook([]);

    render(
      <EntityCombobox<Person>
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Publisher"
        onChange={onChange}
        value={null}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText(/Search publisher/i), "Penguin");
    await user.click(screen.getByText(/Create "Penguin"/));

    expect(onChange).toHaveBeenCalledWith({ __create: "Penguin" });
  });

  it("hides items returned by exclude predicate", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const hook = makeHook([
      { id: 1, name: "A" },
      { id: 2, name: "B" },
    ]);

    render(
      <EntityCombobox<Person>
        exclude={(p) => p.name === "B"}
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Person"
        onChange={vi.fn()}
        value={null}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    expect(screen.getByText("A")).toBeInTheDocument();
    expect(screen.queryByText("B")).not.toBeInTheDocument();
  });

  it("hides Create CTA when canCreate=false", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const hook = makeHook([]);

    render(
      <EntityCombobox<Person>
        canCreate={false}
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Person"
        onChange={vi.fn()}
        value={null}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText(/Search person/i), "X");

    expect(screen.queryByText(/Create "X"/)).not.toBeInTheDocument();
  });

  it("renders status badge when status prop set", () => {
    const hook = makeHook([]);

    render(
      <EntityCombobox<Person>
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Person"
        onChange={vi.fn()}
        status="new"
        value={{ id: 1, name: "X" }}
      />,
    );

    expect(screen.getByTestId("entity-status-badge")).toHaveTextContent(/new/i);
  });

  it("shows loading state when hook reports isLoading", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const hook = makeHook([], true);

    render(
      <EntityCombobox<Person>
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Person"
        onChange={vi.fn()}
        value={null}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    expect(screen.getByText(/Loading/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm vitest run app/components/common/EntityCombobox.test.tsx`
Expected: FAIL — module `./EntityCombobox` does not exist.

- [ ] **Step 3: Write the component**

```tsx
// app/components/common/EntityCombobox.tsx
import { Check, ChevronsUpDown, Plus } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/libraries/utils";

export type EntityStatus = "new" | "changed" | "unchanged";

export interface EntityComboboxProps<T> {
  hook: (query: string) => { data?: T[]; isLoading: boolean };
  label: string;
  value: T | { __create: string } | null;
  onChange: (next: T | { __create: string }) => void;
  getOptionLabel: (item: T) => string;
  getOptionKey?: (item: T) => string | number;
  canCreate?: boolean;
  exclude?: (item: T) => boolean;
  status?: EntityStatus;
  pendingCreate?: boolean;
  placeholder?: string;
}

export function EntityCombobox<T>({
  hook,
  label,
  value,
  onChange,
  getOptionLabel,
  getOptionKey,
  canCreate = true,
  exclude,
  status,
  pendingCreate,
  placeholder,
}: EntityComboboxProps<T>) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const { data: items = [], isLoading } = hook(search);

  const filtered = exclude ? items.filter((i) => !exclude(i)) : items;
  const trimmed = search.trim();
  const showCreate =
    canCreate &&
    !!trimmed &&
    !filtered.some((i) => getOptionLabel(i).toLowerCase() === trimmed.toLowerCase());

  const triggerLabel =
    value == null
      ? placeholder ?? `Add ${label.toLowerCase()}...`
      : "__create" in value
        ? value.__create
        : getOptionLabel(value);

  return (
    <div className="flex items-center gap-2">
      <Popover modal onOpenChange={setOpen} open={open}>
        <PopoverTrigger asChild>
          <Button
            aria-expanded={open}
            className={cn(
              "w-full justify-between cursor-pointer",
              pendingCreate && "border-dashed",
            )}
            role="combobox"
            variant="outline"
          >
            <span className="truncate">{triggerLabel}</span>
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-full p-0">
          <Command shouldFilter={false}>
            <CommandInput
              onValueChange={setSearch}
              placeholder={`Search ${label.toLowerCase()}...`}
              value={search}
            />
            <CommandList>
              {isLoading && (
                <div className="p-4 text-center text-sm text-muted-foreground">
                  Loading...
                </div>
              )}
              {!isLoading && filtered.length === 0 && !showCreate && (
                <div className="p-4 text-center text-sm text-muted-foreground">
                  {trimmed
                    ? `No matching ${label.toLowerCase()}.`
                    : `No ${label.toLowerCase()} available.${canCreate ? " Type to create one." : ""}`}
                </div>
              )}
              {!isLoading && (
                <CommandGroup>
                  {filtered.map((item) => {
                    const key = getOptionKey
                      ? getOptionKey(item)
                      : getOptionLabel(item);
                    return (
                      <CommandItem
                        className="cursor-pointer"
                        key={key}
                        onSelect={() => {
                          onChange(item);
                          setOpen(false);
                          setSearch("");
                        }}
                        value={getOptionLabel(item)}
                      >
                        <Check className="mr-2 h-4 w-4 shrink-0 opacity-0" />
                        <span className="truncate">{getOptionLabel(item)}</span>
                      </CommandItem>
                    );
                  })}
                  {showCreate && (
                    <CommandItem
                      className="cursor-pointer"
                      onSelect={() => {
                        onChange({ __create: trimmed });
                        setOpen(false);
                        setSearch("");
                      }}
                      value={`create-${trimmed}`}
                    >
                      <Plus className="mr-2 h-4 w-4 shrink-0" />
                      <span className="truncate">Create "{trimmed}"</span>
                    </CommandItem>
                  )}
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
      {status && (
        <Badge
          className={cn(
            status === "new" && "bg-green-600",
            status === "changed" && "bg-amber-600",
            status === "unchanged" && "bg-muted text-muted-foreground",
          )}
          data-testid="entity-status-badge"
          variant="default"
        >
          {status}
        </Badge>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm vitest run app/components/common/EntityCombobox.test.tsx`
Expected: PASS — all 6 cases.

- [ ] **Step 5: Commit**

```bash
git add app/components/common/EntityCombobox.tsx app/components/common/EntityCombobox.test.tsx
git commit -m "[Frontend] Add shared EntityCombobox component"
```

---

## Task 2: Extract `SortableEntityList<T>` (new shared component)

`SortableEntityList` wraps a list of `EntityCombobox` rows with drag-reorder, remove, and a per-row "extras" slot (used for author roles and series numbers). Built on the existing `@dnd-kit`-based `DraggableBookList` infrastructure — reuse its primitives rather than rolling drag-and-drop again.

**Files:**
- Create: `app/components/common/SortableEntityList.tsx`
- Create: `app/components/common/SortableEntityList.test.tsx`

- [ ] **Step 1: Read the existing dnd primitives**

Read: `app/components/library/DraggableBookList.tsx` (full file).
Identify the exported `DragHandleProps` type and the `SortableList` shape (or equivalent) used by `BookEditDialog.tsx:686` and `FileEditDialog.tsx:962`.

- [ ] **Step 2: Write the failing test**

```tsx
// app/components/common/SortableEntityList.test.tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SortableEntityList } from "./SortableEntityList";

interface Person { id: number; name: string }

const HOOK = (_q: string) => ({ data: [] as Person[], isLoading: false });

describe("SortableEntityList", () => {
  it("renders one row per item with the entity label", () => {
    render(
      <SortableEntityList<Person>
        comboboxProps={{
          getOptionLabel: (p) => p.name,
          hook: HOOK,
          label: "Person",
        }}
        items={[
          { id: 1, name: "Alice" },
          { id: 2, name: "Bob" },
        ]}
        onAppend={vi.fn()}
        onRemove={vi.fn()}
        onReorder={vi.fn()}
      />,
    );

    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
  });

  it("calls onRemove with the row index when remove button clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onRemove = vi.fn();
    render(
      <SortableEntityList<Person>
        comboboxProps={{
          getOptionLabel: (p) => p.name,
          hook: HOOK,
          label: "Person",
        }}
        items={[{ id: 1, name: "Alice" }]}
        onAppend={vi.fn()}
        onRemove={onRemove}
        onReorder={vi.fn()}
      />,
    );

    await user.click(screen.getByLabelText(/Remove Alice/i));
    expect(onRemove).toHaveBeenCalledWith(0);
  });

  it("renders renderExtras output adjacent to each row", () => {
    render(
      <SortableEntityList<Person>
        comboboxProps={{
          getOptionLabel: (p) => p.name,
          hook: HOOK,
          label: "Person",
        }}
        items={[{ id: 1, name: "Alice" }]}
        onAppend={vi.fn()}
        onRemove={vi.fn()}
        onReorder={vi.fn()}
        renderExtras={(item, idx) => (
          <span data-testid="extras">extra-{item.name}-{idx}</span>
        )}
      />,
    );

    expect(screen.getByTestId("extras")).toHaveTextContent("extra-Alice-0");
  });

  it("resolves per-row status via the status prop", () => {
    render(
      <SortableEntityList<Person>
        comboboxProps={{
          getOptionLabel: (p) => p.name,
          hook: HOOK,
          label: "Person",
        }}
        items={[
          { id: 1, name: "A" },
          { id: 2, name: "B" },
        ]}
        onAppend={vi.fn()}
        onRemove={vi.fn()}
        onReorder={vi.fn()}
        status={(_, idx) => (idx === 0 ? "new" : "unchanged")}
      />,
    );

    const badges = screen.getAllByTestId("entity-status-badge");
    expect(badges[0]).toHaveTextContent(/new/i);
    expect(badges[1]).toHaveTextContent(/unchanged/i);
  });

  it("forwards onAppend when the embedded combobox emits a value", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onAppend = vi.fn();
    const hook = (_q: string) => ({
      data: [{ id: 9, name: "Carol" }],
      isLoading: false,
    });

    render(
      <SortableEntityList<Person>
        comboboxProps={{ getOptionLabel: (p) => p.name, hook, label: "Person" }}
        items={[]}
        onAppend={onAppend}
        onRemove={vi.fn()}
        onReorder={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Carol"));

    expect(onAppend).toHaveBeenCalledWith({ id: 9, name: "Carol" });
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `pnpm vitest run app/components/common/SortableEntityList.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 4: Write the component**

```tsx
// app/components/common/SortableEntityList.tsx
import { GripVertical, X } from "lucide-react";

import {
  EntityCombobox,
  type EntityComboboxProps,
  type EntityStatus,
} from "@/components/common/EntityCombobox";
import {
  type DragHandleProps,
  SortableList,
} from "@/components/library/DraggableBookList";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/libraries/utils";

interface SortableEntityListProps<T> {
  items: T[];
  onReorder: (next: T[]) => void;
  onRemove: (index: number) => void;
  onAppend: (next: T | { __create: string }) => void;
  comboboxProps: Pick<
    EntityComboboxProps<T>,
    "hook" | "label" | "getOptionLabel" | "getOptionKey" | "canCreate"
  >;
  renderExtras?: (item: T, index: number) => React.ReactNode;
  status?: (item: T, index: number) => EntityStatus | undefined;
  pendingCreate?: (item: T) => boolean;
  getItemId?: (item: T, index: number) => string;
}

export function SortableEntityList<T>({
  items,
  onReorder,
  onRemove,
  onAppend,
  comboboxProps,
  renderExtras,
  status,
  pendingCreate,
  getItemId,
}: SortableEntityListProps<T>) {
  const itemId = (item: T, index: number) =>
    getItemId
      ? getItemId(item, index)
      : `${comboboxProps.getOptionLabel(item)}-${index}`;

  const excludeAlreadyChosen = (candidate: T) =>
    items.some(
      (existing) =>
        comboboxProps.getOptionLabel(existing).toLowerCase() ===
        comboboxProps.getOptionLabel(candidate).toLowerCase(),
    );

  return (
    <div className="space-y-2">
      <SortableList
        getItemId={itemId}
        items={items}
        onReorder={onReorder}
        renderItem={(item: T, index: number, drag: DragHandleProps) => {
          const rowStatus = status ? status(item, index) : undefined;
          const isPending = pendingCreate ? pendingCreate(item) : false;
          const label = comboboxProps.getOptionLabel(item);

          return (
            <div className="flex items-center gap-2">
              <button
                aria-label={`Drag ${label}`}
                className="cursor-grab touch-none text-muted-foreground hover:text-foreground"
                type="button"
                {...drag.attributes}
                {...drag.listeners}
              >
                <GripVertical className="h-4 w-4" />
              </button>
              <div
                className={cn(
                  "flex-1 truncate rounded-md border px-3 py-2",
                  isPending && "border-dashed",
                )}
                title={label}
              >
                {label}
              </div>
              {renderExtras?.(item, index)}
              {rowStatus && (
                <Badge
                  className={cn(
                    rowStatus === "new" && "bg-green-600",
                    rowStatus === "changed" && "bg-amber-600",
                    rowStatus === "unchanged" && "bg-muted text-muted-foreground",
                  )}
                  data-testid="entity-status-badge"
                  variant="default"
                >
                  {rowStatus}
                </Badge>
              )}
              <Button
                aria-label={`Remove ${label}`}
                className="cursor-pointer"
                onClick={() => onRemove(index)}
                size="icon"
                type="button"
                variant="ghost"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          );
        }}
      />
      <EntityCombobox<T>
        {...comboboxProps}
        exclude={excludeAlreadyChosen}
        onChange={onAppend}
        value={null}
      />
    </div>
  );
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `pnpm vitest run app/components/common/SortableEntityList.test.tsx`
Expected: PASS — all 5 cases.

- [ ] **Step 6: Commit**

```bash
git add app/components/common/SortableEntityList.tsx app/components/common/SortableEntityList.test.tsx
git commit -m "[Frontend] Add shared SortableEntityList component"
```

---

## Task 3: Extract `IdentifierEditor` (new shared component)

**Files:**
- Create: `app/components/common/IdentifierEditor.tsx`
- Create: `app/components/common/IdentifierEditor.test.tsx`

- [ ] **Step 1: Read the existing identifier UI in FileEditDialog**

Read: `app/components/library/FileEditDialog.tsx` from line 110 through line ~1433 (or grep for `IdentifierPayload`, `validateIdentifier`, `pluginIdentifierTypes`). Capture the exact behavior:
- Type dropdown excludes already-present types via `disabled`
- Value input + Add button gated by `validateIdentifier(type, value)` returning a non-error
- Per-row delete and clear-all
- Tooltip on disabled type entries

Don't change behavior — lift it as-is.

- [ ] **Step 2: Write the failing test**

```tsx
// app/components/common/IdentifierEditor.test.tsx
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import {
  IdentifierEditor,
  type IdentifierRow,
} from "./IdentifierEditor";

const TYPES = [
  { type: "isbn", label: "ISBN" },
  { type: "asin", label: "ASIN" },
];

describe("IdentifierEditor", () => {
  it("excludes already-present types from the type dropdown", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={vi.fn()}
        value={[{ type: "isbn", value: "9780000000001" }]}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: /Type/i }));
    const isbn = screen.getByRole("option", { name: /ISBN/i });
    expect(isbn).toHaveAttribute("aria-disabled", "true");
  });

  it("blocks Add when validateIdentifier rejects the value", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={onChange}
        value={[]}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: /Type/i }));
    await user.click(screen.getByRole("option", { name: /ISBN/i }));
    await user.type(screen.getByPlaceholderText(/value/i), "not-an-isbn");
    await user.click(screen.getByRole("button", { name: /Add/i }));

    expect(onChange).not.toHaveBeenCalled();
    expect(
      screen.getByText(/invalid|format|valid/i),
    ).toBeInTheDocument();
  });

  it("appends a valid identifier on Add", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={onChange}
        value={[]}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: /Type/i }));
    await user.click(screen.getByRole("option", { name: /ISBN/i }));
    await user.type(screen.getByPlaceholderText(/value/i), "9780306406157");
    await user.click(screen.getByRole("button", { name: /Add/i }));

    expect(onChange).toHaveBeenCalledWith([
      { type: "isbn", value: "9780306406157" },
    ]);
  });

  it("removes a row when its delete button is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={onChange}
        value={[{ type: "isbn", value: "9780306406157" }]}
      />,
    );

    await user.click(screen.getByLabelText(/Remove isbn/i));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("clears all rows when Clear all clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    const rows: IdentifierRow[] = [
      { type: "isbn", value: "9780306406157" },
      { type: "asin", value: "B0BHRJYNHV" },
    ];

    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={onChange}
        value={rows}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Clear all/i }));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("renders per-row status badge when status resolver provided", () => {
    render(
      <IdentifierEditor
        identifierTypes={TYPES}
        onChange={vi.fn()}
        status={(row) => (row.type === "isbn" ? "new" : "unchanged")}
        value={[
          { type: "isbn", value: "9780306406157" },
          { type: "asin", value: "B0BHRJYNHV" },
        ]}
      />,
    );

    const isbnRow = screen.getByText("9780306406157").closest("[data-testid='identifier-row']");
    expect(within(isbnRow as HTMLElement).getByTestId("identifier-status-badge"))
      .toHaveTextContent(/new/i);
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `pnpm vitest run app/components/common/IdentifierEditor.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 4: Write the component (lifted from FileEditDialog)**

```tsx
// app/components/common/IdentifierEditor.tsx
import { Plus, X } from "lucide-react";
import { useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/libraries/utils";
// validateIdentifier already exists in the codebase; locate its current home and import.
// In FileEditDialog it's imported from "@/utils/identifiers" (or similar).
// If the import path differs, update accordingly when extracting.
import { validateIdentifier } from "@/utils/identifiers";

export interface IdentifierRow {
  type: string;
  value: string;
}

interface IdentifierTypeOption {
  type: string;
  label: string;
}

interface IdentifierEditorProps {
  value: IdentifierRow[];
  onChange: (next: IdentifierRow[]) => void;
  identifierTypes: IdentifierTypeOption[];
  status?: (row: IdentifierRow) => "new" | "changed" | "unchanged" | undefined;
}

export function IdentifierEditor({
  value,
  onChange,
  identifierTypes,
  status,
}: IdentifierEditorProps) {
  const [newType, setNewType] = useState<string>("");
  const [newValue, setNewValue] = useState<string>("");
  const [validationError, setValidationError] = useState<string | null>(null);

  const presentTypes = useMemo(
    () => new Set(value.map((r) => r.type)),
    [value],
  );

  const handleAdd = () => {
    const type = newType;
    const val = newValue.trim();
    if (!type || !val) return;
    const error = validateIdentifier(type, val);
    if (error) {
      setValidationError(error);
      return;
    }
    onChange([...value, { type, value: val }]);
    setNewType("");
    setNewValue("");
    setValidationError(null);
  };

  const handleRemove = (idx: number) => {
    onChange(value.filter((_, i) => i !== idx));
  };

  const handleClearAll = () => {
    onChange([]);
  };

  return (
    <div className="space-y-2">
      {value.length > 0 && (
        <div className="space-y-1">
          {value.map((row, idx) => {
            const rowStatus = status?.(row);
            return (
              <div
                className="flex items-center gap-2"
                data-testid="identifier-row"
                key={`${row.type}-${idx}`}
              >
                <Badge variant="outline">{row.type}</Badge>
                <span className="flex-1 font-mono text-sm">{row.value}</span>
                {rowStatus && (
                  <Badge
                    className={cn(
                      rowStatus === "new" && "bg-green-600",
                      rowStatus === "changed" && "bg-amber-600",
                      rowStatus === "unchanged" && "bg-muted text-muted-foreground",
                    )}
                    data-testid="identifier-status-badge"
                  >
                    {rowStatus}
                  </Badge>
                )}
                <Button
                  aria-label={`Remove ${row.type}`}
                  className="cursor-pointer"
                  onClick={() => handleRemove(idx)}
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            );
          })}
          {value.length > 1 && (
            <button
              className="text-xs text-muted-foreground hover:text-destructive cursor-pointer"
              onClick={handleClearAll}
              type="button"
            >
              Clear all
            </button>
          )}
        </div>
      )}
      <div className="flex items-start gap-2">
        <Select onValueChange={setNewType} value={newType}>
          <SelectTrigger
            aria-label="Type"
            className="w-32 cursor-pointer"
          >
            <SelectValue placeholder="Type" />
          </SelectTrigger>
          <SelectContent>
            <TooltipProvider>
              {identifierTypes.map((opt) => {
                const disabled = presentTypes.has(opt.type);
                const item = (
                  <SelectItem
                    aria-disabled={disabled}
                    className={cn(
                      "cursor-pointer",
                      disabled && "opacity-50 pointer-events-none",
                    )}
                    disabled={disabled}
                    key={opt.type}
                    value={opt.type}
                  >
                    {opt.label}
                  </SelectItem>
                );
                return disabled ? (
                  <Tooltip key={opt.type}>
                    <TooltipTrigger asChild>
                      <div>{item}</div>
                    </TooltipTrigger>
                    <TooltipContent>
                      Already added — only one identifier per type is allowed.
                    </TooltipContent>
                  </Tooltip>
                ) : (
                  item
                );
              })}
            </TooltipProvider>
          </SelectContent>
        </Select>
        <Input
          className="flex-1"
          onChange={(e) => {
            setNewValue(e.target.value);
            if (validationError) setValidationError(null);
          }}
          placeholder="Identifier value"
          value={newValue}
        />
        <Button
          className="cursor-pointer"
          disabled={!newType || !newValue.trim()}
          onClick={handleAdd}
          type="button"
        >
          <Plus className="mr-2 h-4 w-4" />
          Add
        </Button>
      </div>
      {validationError && (
        <p className="text-xs text-destructive">{validationError}</p>
      )}
    </div>
  );
}
```

- [ ] **Step 5: Verify `validateIdentifier` import path**

Run: `grep -rn "export function validateIdentifier\|export const validateIdentifier" /Users/robinjoseph/.worktrees/shisho/identify-form/app/`
Expected: A single export. Update the `import { validateIdentifier } from "@/utils/identifiers"` path in `IdentifierEditor.tsx` to match the actual location.

- [ ] **Step 6: Run tests to verify they pass**

Run: `pnpm vitest run app/components/common/IdentifierEditor.test.tsx`
Expected: PASS — all 6 cases.

- [ ] **Step 7: Commit**

```bash
git add app/components/common/IdentifierEditor.tsx app/components/common/IdentifierEditor.test.tsx
git commit -m "[Frontend] Add shared IdentifierEditor component"
```

---

## Task 4: Extend `MultiSelectCombobox` with optional `status` prop

**Files:**
- Modify: `app/components/ui/MultiSelectCombobox.tsx`
- Create: `app/components/ui/MultiSelectCombobox.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// app/components/ui/MultiSelectCombobox.test.tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { MultiSelectCombobox } from "./MultiSelectCombobox";

describe("MultiSelectCombobox", () => {
  it("renders chip status badges when status prop provided", () => {
    render(
      <MultiSelectCombobox
        isLoading={false}
        label="Genre"
        onChange={vi.fn()}
        onSearch={vi.fn()}
        options={[]}
        searchValue=""
        status={(v) => (v === "Sci-Fi" ? "new" : "unchanged")}
        values={["Sci-Fi", "Fantasy"]}
      />,
    );

    const sci = screen.getByText("Sci-Fi").closest("[data-testid='ms-chip']");
    expect(sci?.querySelector("[data-testid='ms-status-badge']"))
      .toHaveTextContent(/new/i);
  });

  it("renders no status badges when status prop omitted (existing behavior)", () => {
    render(
      <MultiSelectCombobox
        isLoading={false}
        label="Genre"
        onChange={vi.fn()}
        onSearch={vi.fn()}
        options={[]}
        searchValue=""
        values={["Sci-Fi"]}
      />,
    );

    expect(screen.queryByTestId("ms-status-badge")).not.toBeInTheDocument();
  });

  it("renders removed entries (in removed[] but not in values[]) as strikethrough with undo", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <MultiSelectCombobox
        isLoading={false}
        label="Genre"
        onChange={onChange}
        onSearch={vi.fn()}
        options={[]}
        removed={["Horror"]}
        searchValue=""
        values={["Sci-Fi"]}
      />,
    );

    expect(screen.getByText("Horror")).toHaveClass(/line-through/);
    await user.click(screen.getByLabelText(/Restore Horror/i));
    expect(onChange).toHaveBeenCalledWith(["Sci-Fi", "Horror"]);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm vitest run app/components/ui/MultiSelectCombobox.test.tsx`
Expected: FAIL — `status` prop not in component, `removed` prop not in component, `data-testid` attributes missing.

- [ ] **Step 3: Modify `MultiSelectCombobox` to accept `status` and `removed`**

Update `app/components/ui/MultiSelectCombobox.tsx`:

```tsx
// Add to MultiSelectComboboxProps:
//   status?: (value: string) => "new" | "changed" | "unchanged" | undefined;
//   removed?: string[];
//
// In the chip render block (around line 81-95), wrap each chip in a container with
// data-testid="ms-chip" and append a Badge with data-testid="ms-status-badge"
// when status(value) is defined.
//
// Below the active chip row, render removed entries with line-through and an
// undo button (aria-label={`Restore ${value}`}) that calls onChange([...values, value]).

interface MultiSelectComboboxProps {
  values: string[];
  onChange: (values: string[]) => void;
  options: string[];
  onSearch: (query: string) => void;
  searchValue: string;
  placeholder?: string;
  isLoading?: boolean;
  label: string;
  status?: (value: string) => "new" | "changed" | "unchanged" | undefined;
  removed?: string[];
}

// Inside the values-mapping JSX:
{values.map((value) => {
  const s = status?.(value);
  return (
    <div className="inline-flex" data-testid="ms-chip" key={value}>
      <Badge className="flex items-center gap-1" variant="secondary">
        {value}
        <button
          aria-label={`Remove ${value}`}
          className="ml-1 cursor-pointer hover:text-destructive"
          onClick={() => handleRemove(value)}
          type="button"
        >
          <X className="h-3 w-3" />
        </button>
      </Badge>
      {s && (
        <Badge
          className={cn(
            "ml-1",
            s === "new" && "bg-green-600",
            s === "changed" && "bg-amber-600",
            s === "unchanged" && "bg-muted text-muted-foreground",
          )}
          data-testid="ms-status-badge"
        >
          {s}
        </Badge>
      )}
    </div>
  );
})}

// After the active row, before Combobox:
{removed && removed.length > 0 && (
  <div className="flex flex-wrap gap-2">
    {removed.map((value) => (
      <Badge
        className="line-through text-muted-foreground"
        key={`removed-${value}`}
        variant="outline"
      >
        {value}
        <button
          aria-label={`Restore ${value}`}
          className="ml-1 cursor-pointer hover:text-foreground"
          onClick={() => onChange([...values, value])}
          type="button"
        >
          <Undo2 className="h-3 w-3" />
        </button>
      </Badge>
    ))}
  </div>
)}
```

(Add the `Undo2` icon import from `lucide-react` and the `cn` import from `@/libraries/utils`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm vitest run app/components/ui/MultiSelectCombobox.test.tsx`
Expected: PASS — all 3 cases.

- [ ] **Step 5: Run the full test suite to confirm no regressions in existing consumers**

Run: `pnpm vitest run app/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/components/ui/MultiSelectCombobox.tsx app/components/ui/MultiSelectCombobox.test.tsx
git commit -m "[Frontend] Add status + removed props to MultiSelectCombobox"
```

---

## Task 5: Refactor `BookEditDialog` to use shared components

**Files:**
- Modify: `app/components/library/BookEditDialog.tsx`
- Modify: `app/components/library/BookEditDialog.test.tsx` (add regression cases)

- [ ] **Step 1: Add the regression-test cases first**

Edit `app/components/library/BookEditDialog.test.tsx` and add at the end of the existing `describe`:

```tsx
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

// (Existing imports / setup unchanged)

describe("BookEditDialog (post-refactor regression)", () => {
  it("CBZ files render the per-author role select", async () => {
    // Mount BookEditDialog with a CBZ file via the existing test fixture,
    // add an author via the embedded EntityCombobox, and assert that a
    // role <Select> is rendered for that row.
    // Use the existing test scaffold (mocked queries, etc.).
  });

  it("non-CBZ files do not render the per-author role select", async () => {
    // Same shape, with an EPUB file. Assert no role select renders.
  });

  it("series rows expose a number input that updates state", async () => {
    // Add a series via the combobox; type a number; assert the value is set.
  });

  it("genres MultiSelectCombobox supports select-or-create", async () => {
    // Existing case; verify still works after refactor.
  });
});
```

(Replace the placeholder comments with concrete RTL setup using the existing fixtures in the test file. If the file doesn't already mock queries, mirror what `IdentifyReviewForm.test.ts` or `FileEditDialog.test.tsx` do.)

- [ ] **Step 2: Run test to verify failures (regression cases will pass against the un-refactored code; that's fine — they pin behavior so the refactor doesn't break it)**

Run: `pnpm vitest run app/components/library/BookEditDialog.test.tsx`
Expected: PASS for all the new cases. If any fail, fix the test setup before proceeding.

- [ ] **Step 3: Refactor — replace the inline author Popover blocks (lines ~617–680 and ~719–790)**

Replace the inline CBZ author combobox + create flow with `EntityCombobox`:

```tsx
import { EntityCombobox } from "@/components/common/EntityCombobox";
import { SortableEntityList } from "@/components/common/SortableEntityList";

// CBZ branch — replace the inline Popover with:
<EntityCombobox<PersonWithCounts>
  exclude={(p) =>
    authors.some((a) => a.name.toLowerCase() === p.name.toLowerCase())
  }
  getOptionKey={(p) => p.id}
  getOptionLabel={(p) => p.name}
  hook={(q) => {
    const { data, isLoading } = usePeopleList({ search: q, library_id: libraryId });
    return { data: data?.people, isLoading };
  }}
  label="Author"
  onChange={(next) => {
    const name = "__create" in next ? next.__create : next.name;
    if (!authors.some((a) => a.name === name)) {
      setAuthors([...authors, { name, role: AuthorRoleWriter }]);
    }
  }}
  value={null}
/>

// Non-CBZ branch — replace the inline SortableList block with:
<SortableEntityList<AuthorInput>
  comboboxProps={{
    getOptionKey: (p: PersonWithCounts) => p.id,
    getOptionLabel: (p: PersonWithCounts) => p.name,
    hook: (q: string) => {
      const { data, isLoading } = usePeopleList({ search: q, library_id: libraryId });
      return { data: data?.people, isLoading };
    },
    label: "Author",
  }}
  items={authors}
  onAppend={(next) => {
    const name = "__create" in next ? next.__create : (next as PersonWithCounts).name;
    if (!authors.some((a) => a.name === name)) {
      setAuthors([...authors, { name }]);
    }
  }}
  onRemove={handleRemoveAuthor}
  onReorder={setAuthors}
/>
```

Note: the embedded `EntityCombobox` inside `SortableEntityList` is generic over the entity type. The `onAppend` adapter unwraps `{ __create }` into a plain `AuthorInput` since the existing `authors` state stores `AuthorInput[]`.

- [ ] **Step 4: Refactor — replace the inline series Popover block (around lines ~865–910) with `SortableEntityList`**

```tsx
<SortableEntityList<SeriesInput>
  comboboxProps={{
    getOptionKey: (s: SeriesWithCount) => s.id,
    getOptionLabel: (s: SeriesWithCount) => s.name,
    hook: (q: string) => {
      const { data, isLoading } = useSeriesList({ search: q, library_id: libraryId });
      return { data: data?.series, isLoading };
    },
    label: "Series",
  }}
  items={seriesEntries}
  onAppend={(next) => {
    const name = "__create" in next ? next.__create : (next as SeriesWithCount).name;
    if (!seriesEntries.find((s) => s.name === name)) {
      setSeriesEntries([...seriesEntries, { name, number: undefined }]);
    }
  }}
  onRemove={handleRemoveSeries}
  onReorder={setSeriesEntries}
  renderExtras={(entry, idx) => (
    <Input
      className="w-24"
      onChange={(e) => handleSeriesNumberChange(idx, e.target.value)}
      placeholder="#"
      type="number"
      value={entry.number ?? ""}
    />
  )}
/>
```

- [ ] **Step 5: Run all `BookEditDialog` tests**

Run: `pnpm vitest run app/components/library/BookEditDialog.test.tsx`
Expected: PASS (existing + new regression cases).

- [ ] **Step 6: Run typecheck**

Run: `pnpm lint:types`
Expected: No type errors.

- [ ] **Step 7: Commit**

```bash
git add app/components/library/BookEditDialog.tsx app/components/library/BookEditDialog.test.tsx
git commit -m "[Frontend] Refactor BookEditDialog to use shared entity inputs"
```

---

## Task 6: Refactor `FileEditDialog` to use shared components

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx`
- Modify: `app/components/library/FileEditDialog.test.tsx` (add regression cases)

- [ ] **Step 1: Add regression tests first**

```tsx
// In FileEditDialog.test.tsx, add cases for:
// - Adding identifiers via the now-shared IdentifierEditor
// - Type-uniqueness exclusion in the type dropdown
// - Adding/removing narrators on M4B via SortableEntityList
// - Publisher/imprint single-value EntityCombobox select-or-create
// - Release date picker still works
// - Cover handling unchanged (no edits expected here)
```

Concrete cases (replace the comments with actual RTL code mirroring the existing test patterns in this file):

```tsx
it("adds an identifier via IdentifierEditor and persists on save", async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  // mount with an existing M4B file with no identifiers
  // pick ISBN type, type a valid value, click Add
  // click Save
  // assert useUpdateFile was called with payload.identifiers including the new entry
});

it("hides already-present identifier types in the type dropdown", async () => {
  // mount with an existing file that already has an isbn
  // open type dropdown
  // assert the ISBN option is disabled
});

it("adds a narrator via the shared SortableEntityList for M4B files", async () => {
  // mount with an M4B file
  // append a narrator via the embedded combobox
  // click Save
  // assert payload.narrators contains the new name
});

it("publisher EntityCombobox supports select-or-create", async () => {
  // mount with a non-M4B file with no publisher
  // open the publisher combobox, type a new name, click Create
  // click Save
  // assert payload.publisher = "<the typed name>"
});
```

- [ ] **Step 2: Run tests; cases pin current behavior, must pass against un-refactored code**

Run: `pnpm vitest run app/components/library/FileEditDialog.test.tsx`
Expected: PASS.

- [ ] **Step 3: Refactor — replace the inline narrator Popover block with `SortableEntityList`**

Pattern mirrors Task 5 Step 3. Replace the block around lines ~990–1070.

- [ ] **Step 4: Refactor — replace the publisher inline Popover block with `EntityCombobox` (single-value)**

Single-value entity fields (publisher, imprint) store only the name string in
state. The cleanest fit is to instantiate `EntityCombobox<string>` and let
`getOptionLabel` be identity. The hook adapter maps the entity-list response
down to names:

Around lines ~1150–1210:

```tsx
<EntityCombobox<string>
  getOptionLabel={(name) => name}
  hook={(q) => {
    const { data, isLoading } = usePublishersList({ search: q, library_id: libraryId });
    return {
      data: (data?.publishers ?? []).map((p) => p.name),
      isLoading,
    };
  }}
  label="Publisher"
  onChange={(next) => {
    setPublisher("__create" in next ? next.__create : next);
  }}
  value={publisher || null}
/>
```

- [ ] **Step 5: Refactor — same shape for imprint (around lines ~1250–1310)**

- [ ] **Step 6: Refactor — replace the inline identifier UI with `IdentifierEditor`**

Locate the identifier section (search for `pluginIdentifierTypes` and `setIdentifiers`); replace with:

```tsx
<IdentifierEditor
  identifierTypes={(pluginIdentifierTypes ?? []).map((t) => ({
    type: t.type,
    label: t.label,
  }))}
  onChange={setIdentifiers}
  value={identifiers}
/>
```

- [ ] **Step 7: Run all `FileEditDialog` tests**

Run: `pnpm vitest run app/components/library/FileEditDialog.test.tsx`
Expected: PASS.

- [ ] **Step 8: Run typecheck**

Run: `pnpm lint:types`
Expected: No type errors.

- [ ] **Step 9: Commit**

```bash
git add app/components/library/FileEditDialog.tsx app/components/library/FileEditDialog.test.tsx
git commit -m "[Frontend] Refactor FileEditDialog to use shared entity inputs"
```

---

## Task 7: Extend `useUpdateBook` and `useUpdateFile` to invalidate entity-list queries

When the update endpoint creates a new entity by name (e.g., a new author or publisher passed in the payload), the local cache for the entity list must be invalidated so the new entity appears in admin pages and future combobox results.

**Files:**
- Modify: `app/hooks/queries/books.ts`
- Modify: `app/hooks/queries/books.test.ts` (or create if missing)

- [ ] **Step 1: Write the failing test**

```tsx
// app/hooks/queries/books.test.ts (add to existing file or create)
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { useUpdateBook, useUpdateFile } from "./books";
import { QueryKey as PeopleQueryKey } from "./people";
import { QueryKey as PublishersQueryKey } from "./publishers";

vi.mock("@/libraries/api", () => ({
  default: {
    request: vi.fn().mockResolvedValue({ id: 1, title: "x" }),
  },
}));

const wrap = (qc: QueryClient) => ({ children }: { children: React.ReactNode }) =>
  <QueryClientProvider client={qc}>{children}</QueryClientProvider>;

describe("update mutations entity-list invalidation", () => {
  it("useUpdateBook invalidates ListPeople, ListSeries, ListGenres, ListTags", async () => {
    const qc = new QueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");

    const { result } = renderHook(() => useUpdateBook(), { wrapper: wrap(qc) });
    await result.current.mutateAsync({ id: "1", payload: { title: "x" } });

    const calls = spy.mock.calls.map((c) => (c[0] as any).queryKey?.[0]);
    expect(calls).toContain(PeopleQueryKey.ListPeople);
    // assert series/genres/tags too
  });

  it("useUpdateFile invalidates ListPeople, ListPublishers, ListImprints", async () => {
    const qc = new QueryClient();
    const spy = vi.spyOn(qc, "invalidateQueries");

    const { result } = renderHook(() => useUpdateFile(), { wrapper: wrap(qc) });
    await result.current.mutateAsync({ id: 1, payload: { publisher: "X" } });

    const calls = spy.mock.calls.map((c) => (c[0] as any).queryKey?.[0]);
    expect(calls).toContain(PeopleQueryKey.ListPeople);
    expect(calls).toContain(PublishersQueryKey.ListPublishers);
    // assert imprints too
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm vitest run app/hooks/queries/books.test.ts`
Expected: FAIL — invalidateQueries not called for those keys.

- [ ] **Step 3: Update both mutations**

Edit `app/hooks/queries/books.ts`:

```typescript
import { QueryKey as GenresQueryKey } from "./genres";
import { QueryKey as ImprintsQueryKey } from "./imprints";
import { QueryKey as PeopleQueryKey } from "./people";
import { QueryKey as PublishersQueryKey } from "./publishers";
import { QueryKey as SeriesQueryKey } from "./series";
import { QueryKey as TagsQueryKey } from "./tags";

// In useUpdateBook onSuccess:
queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
queryClient.setQueryData([QueryKey.RetrieveBook, String(data.id)], data);
queryClient.invalidateQueries({ queryKey: [PeopleQueryKey.ListPeople] });
queryClient.invalidateQueries({ queryKey: [SeriesQueryKey.ListSeries] });
queryClient.invalidateQueries({ queryKey: [GenresQueryKey.ListGenres] });
queryClient.invalidateQueries({ queryKey: [TagsQueryKey.ListTags] });

// In useUpdateFile onSuccess (alongside existing invalidations):
queryClient.invalidateQueries({ queryKey: [PeopleQueryKey.ListPeople] });
queryClient.invalidateQueries({ queryKey: [PublishersQueryKey.ListPublishers] });
queryClient.invalidateQueries({ queryKey: [ImprintsQueryKey.ListImprints] });
```

Verify the actual `QueryKey` enum names by reading each `app/hooks/queries/<entity>.ts` file before importing — adjust if names differ.

- [ ] **Step 4: Run tests**

Run: `pnpm vitest run app/hooks/queries/books.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/hooks/queries/books.ts app/hooks/queries/books.test.ts
git commit -m "[Frontend] Invalidate entity lists on book/file update"
```

---

## Task 8: Add `useAutoMatchEntities` hook

Resolves a `PluginSearchResult`'s entity names against the local DB so the review form can render existing-entity vs pending-create chips.

**Files:**
- Create: `app/hooks/useAutoMatchEntities.ts`
- Create: `app/hooks/useAutoMatchEntities.test.ts`

- [ ] **Step 1: Write the failing test**

```tsx
// app/hooks/useAutoMatchEntities.test.ts
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { useAutoMatchEntities } from "./useAutoMatchEntities";

// Mock the underlying list hooks; simplest is to mock the API request layer.
vi.mock("@/libraries/api", () => ({
  default: {
    request: vi.fn(async (_method, path, _body, query) => {
      if (path === "/people") {
        return { people: [{ id: 1, name: "Alice" }], total: 1 };
      }
      if (path === "/publishers") {
        return { publishers: [], total: 0 };
      }
      // ...handle other paths to return matching/non-matching data
      return { items: [], total: 0 };
    }),
  },
}));

describe("useAutoMatchEntities", () => {
  it("returns existing-entity matches and pending-create entries", async () => {
    const qc = new QueryClient();
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>;

    const { result } = renderHook(
      () =>
        useAutoMatchEntities({
          authors: ["Alice", "Carol"],
          narrators: [],
          series: [],
          publisher: "NewPress",
          imprint: undefined,
          genres: [],
          tags: [],
        }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.matches.authors).toEqual([
      { name: "Alice", existing: { id: 1, name: "Alice" } },
      { name: "Carol", existing: null },
    ]);
    expect(result.current.matches.publisher).toEqual({
      name: "NewPress",
      existing: null,
    });
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm vitest run app/hooks/useAutoMatchEntities.test.ts`
Expected: FAIL — hook does not exist.

- [ ] **Step 3: Implement the hook**

```tsx
// app/hooks/useAutoMatchEntities.ts
import { useQueries } from "@tanstack/react-query";

import API from "@/libraries/api";
import { QueryKey as GenresQueryKey } from "./queries/genres";
import { QueryKey as ImprintsQueryKey } from "./queries/imprints";
import { QueryKey as PeopleQueryKey } from "./queries/people";
import { QueryKey as PublishersQueryKey } from "./queries/publishers";
import { QueryKey as SeriesQueryKey } from "./queries/series";
import { QueryKey as TagsQueryKey } from "./queries/tags";

export interface AutoMatchInput {
  authors: string[];
  narrators: string[];
  series: string[];
  publisher: string | undefined;
  imprint: string | undefined;
  genres: string[];
  tags: string[];
}

export interface MatchedEntity<T> {
  name: string;
  existing: T | null;
}

export interface AutoMatchResult {
  isLoading: boolean;
  matches: {
    authors: MatchedEntity<{ id: number; name: string }>[];
    narrators: MatchedEntity<{ id: number; name: string }>[];
    series: MatchedEntity<{ id: number; name: string }>[];
    publisher: MatchedEntity<{ id: number; name: string }> | null;
    imprint: MatchedEntity<{ id: number; name: string }> | null;
    genres: MatchedEntity<{ id: number; name: string }>[];
    tags: MatchedEntity<{ id: number; name: string }>[];
  };
}

const eqLower = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

function matchByName<T extends { name: string }>(
  names: string[],
  pool: T[],
): MatchedEntity<T>[] {
  return names.map((name) => ({
    name,
    existing: pool.find((p) => eqLower(p.name, name)) ?? null,
  }));
}

export function useAutoMatchEntities(input: AutoMatchInput): AutoMatchResult {
  // Collect all names we need to look up. For each entity type, issue a single
  // list call with no search filter (the list returns up to 50; for review-form
  // contexts where typically only a handful of names need matching, this is
  // sufficient. If more precision is needed later we can issue per-name searches.)
  // For correctness on libraries with >50 of any entity type, fall back to
  // per-name searches.
  const queries = useQueries({
    queries: [
      {
        queryKey: [PeopleQueryKey.ListPeople, { search: "" }],
        queryFn: () => API.request("GET", "/people", null, { limit: 50 }, undefined),
        enabled:
          input.authors.length > 0 || input.narrators.length > 0,
      },
      {
        queryKey: [SeriesQueryKey.ListSeries, { search: "" }],
        queryFn: () => API.request("GET", "/series", null, { limit: 50 }, undefined),
        enabled: input.series.length > 0,
      },
      {
        queryKey: [PublishersQueryKey.ListPublishers, { search: "" }],
        queryFn: () => API.request("GET", "/publishers", null, { limit: 50 }, undefined),
        enabled: !!input.publisher,
      },
      {
        queryKey: [ImprintsQueryKey.ListImprints, { search: "" }],
        queryFn: () => API.request("GET", "/imprints", null, { limit: 50 }, undefined),
        enabled: !!input.imprint,
      },
      {
        queryKey: [GenresQueryKey.ListGenres, { search: "" }],
        queryFn: () => API.request("GET", "/genres", null, { limit: 50 }, undefined),
        enabled: input.genres.length > 0,
      },
      {
        queryKey: [TagsQueryKey.ListTags, { search: "" }],
        queryFn: () => API.request("GET", "/tags", null, { limit: 50 }, undefined),
        enabled: input.tags.length > 0,
      },
    ],
  });

  const [people, series, publishers, imprints, genres, tags] = queries;
  const isLoading = queries.some((q) => q.isLoading);

  const peoplePool = (people.data as any)?.people ?? [];
  const seriesPool = (series.data as any)?.series ?? [];
  const publishersPool = (publishers.data as any)?.publishers ?? [];
  const imprintsPool = (imprints.data as any)?.imprints ?? [];
  const genresPool = (genres.data as any)?.genres ?? [];
  const tagsPool = (tags.data as any)?.tags ?? [];

  return {
    isLoading,
    matches: {
      authors: matchByName(input.authors, peoplePool),
      narrators: matchByName(input.narrators, peoplePool),
      series: matchByName(input.series, seriesPool),
      publisher: input.publisher
        ? matchByName([input.publisher], publishersPool)[0]
        : null,
      imprint: input.imprint
        ? matchByName([input.imprint], imprintsPool)[0]
        : null,
      genres: matchByName(input.genres, genresPool),
      tags: matchByName(input.tags, tagsPool),
    },
  };
}
```

Note: list endpoints cap at 50 per the frontend CLAUDE.md. For libraries with >50 of any entity type, an exact-name list-call won't include every entity. For Phase 1, accept that limitation; this is a *display-only* signal and a missed match just means a chip wears the "will be created" pill and gets resolved server-side on apply. If precision becomes important later, a per-name `?search=<exact>` mode can be layered on.

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm vitest run app/hooks/useAutoMatchEntities.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/hooks/useAutoMatchEntities.ts app/hooks/useAutoMatchEntities.test.ts
git commit -m "[Frontend] Add useAutoMatchEntities hook"
```

---

## Common derivations used in Tasks 9–17

Several field tasks reference `currentAuthors`, `currentNarrators`,
`currentSeries`, `currentGenres`, `currentTags`, `currentIdentifiers`,
`currentPublisher`, `currentImprint`, `currentReleaseDate`, etc. These represent
the values **currently stored on the book/file** (before any review-form
edits) and are the basis for the diff/status badges. Derive them once near
the top of `IdentifyReviewForm`:

```tsx
const currentAuthors: AuthorInput[] = (book.authors ?? []).map((a) => ({
  name: a.person.name,
  role: a.role ?? undefined,
}));
const currentSeries: SeriesInput[] = (book.series ?? []).map((s) => ({
  name: s.series.name,
  number: s.number ?? undefined,
}));
const currentGenres: string[] = (book.genres ?? []).map((g) => g.name);
const currentTags: string[] = (book.tags ?? []).map((t) => t.name);
const currentNarrators: string[] = (selectedFile?.narrators ?? []).map(
  (n) => n.person.name,
);
const currentPublisher: string | undefined = selectedFile?.publisher?.name;
const currentImprint: string | undefined = selectedFile?.imprint?.name;
const currentReleaseDate: string | undefined = selectedFile?.release_date;
const currentIdentifiers: IdentifierRow[] = (selectedFile?.identifiers ?? []).map(
  (i) => ({ type: i.type, value: i.value }),
);
```

Verify the exact field shapes against `app/types/generated/books.ts` (`Book`,
`File`, `BookAuthor`, etc.) — adjust the field paths accordingly. These derivations replace the per-task duplication.

---

## Task 9: IdentifyReviewForm — wire authors with shared `SortableEntityList`

`IdentifyReviewForm.tsx` is large; do it field by field with TDD. Each field-task swaps one input and adds tests for that input's behavior + per-row status.

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/IdentifyReviewForm.test.ts` → rename to `IdentifyReviewForm.test.tsx` if not already (component-level tests need JSX).

- [ ] **Step 1: Rename test file if needed and add author-field tests**

```bash
git mv app/components/library/IdentifyReviewForm.test.ts app/components/library/IdentifyReviewForm.test.tsx
```

Add cases:

```tsx
describe("IdentifyReviewForm — authors", () => {
  it("renders one row per incoming author with per-row status badges", async () => {
    // mount with a result whose authors include one match and one non-match
    // assert status badges render correctly per row
  });

  it("CBZ files render a per-row role select (defaults to Writer)", async () => {
    // mount with selectedFile.file_type === 'cbz'
    // assert role select is visible per author row
  });

  it("non-CBZ files do not render the role select", async () => {
    // mount with selectedFile.file_type === 'epub'
    // assert no role select
  });

  it("appending a new author updates the list and marks the row 'new'", async () => {
    // append via combobox; assert new row + status='new'
  });

  it("removed authors (in current but not in final) render strikethrough with undo", async () => {
    // current book has author Alice; incoming has only Bob
    // assert Alice renders as strikethrough with restore button
    // click restore, assert Alice is back in the list
  });
});
```

- [ ] **Step 2: Run tests to verify failures (the new fields don't exist yet)**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx`
Expected: FAIL on the new cases.

- [ ] **Step 3: Replace the inline `AuthorTagInput` with `SortableEntityList<AuthorInput>`**

In `IdentifyReviewForm.tsx`, locate the `<AuthorTagInput ... />` usage (around line 939) and replace with:

```tsx
<SortableEntityList<AuthorInput>
  comboboxProps={{
    getOptionKey: (p: PersonWithCounts) => p.id,
    getOptionLabel: (p: PersonWithCounts) => p.name,
    hook: (q: string) => {
      const { data, isLoading } = usePeopleList({
        search: q,
        library_id: book.library_id,
      });
      return { data: data?.people, isLoading };
    },
    label: "Author",
  }}
  items={authors}
  onAppend={(next) => {
    const name = "__create" in next ? next.__create : (next as PersonWithCounts).name;
    if (!authors.some((a) => a.name === name)) {
      const role = isCbz ? AuthorRoleWriter : undefined;
      setAuthors([...authors, { name, role }]);
    }
  }}
  onRemove={(idx) => setAuthors(authors.filter((_, i) => i !== idx))}
  onReorder={setAuthors}
  pendingCreate={(a) => autoMatch.matches.authors.find((m) => m.name === a.name)?.existing == null}
  renderExtras={isCbz
    ? (author, idx) => (
        <Select
          onValueChange={(role) => updateAuthorRole(idx, role)}
          value={author.role ?? AuthorRoleWriter}
        >
          <SelectTrigger className="w-32 cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CBZ_AUTHOR_ROLES.map((r) => (
              <SelectItem className="cursor-pointer" key={r} value={r}>
                {r}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )
    : undefined}
  status={(author) => computeAuthorStatus(author, currentAuthors)}
/>
```

Add helper `computeAuthorStatus(author, currentAuthors)` near the top of the file:

```tsx
function computeAuthorStatus(
  author: AuthorInput,
  current: AuthorInput[],
): "new" | "changed" | "unchanged" {
  const match = current.find(
    (c) => c.name.toLowerCase() === author.name.toLowerCase(),
  );
  if (!match) return "new";
  if ((match.role ?? "") !== (author.role ?? "")) return "changed";
  return "unchanged";
}
```

Add the removed-row strip below the `SortableEntityList`:

```tsx
{removedAuthors.length > 0 && (
  <div className="flex flex-wrap gap-2 mt-2">
    {removedAuthors.map((a) => (
      <Badge
        className="line-through text-muted-foreground"
        key={`removed-author-${a.name}`}
        variant="outline"
      >
        {a.name}
        <button
          aria-label={`Restore ${a.name}`}
          className="ml-1 cursor-pointer hover:text-foreground"
          onClick={() => setAuthors([...authors, a])}
          type="button"
        >
          <Undo2 className="h-3 w-3" />
        </button>
      </Badge>
    ))}
  </div>
)}
```

Where `removedAuthors = currentAuthors.filter(c => !authors.some(a => a.name === c.name))`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/components/library/IdentifyReviewForm.tsx app/components/library/IdentifyReviewForm.test.tsx
git commit -m "[Frontend] Wire IdentifyReviewForm authors to shared list"
```

---

## Task 10: IdentifyReviewForm — wire narrators (M4B only)

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/IdentifyReviewForm.test.tsx`

- [ ] **Step 1: Write tests**

```tsx
describe("IdentifyReviewForm — narrators", () => {
  it("renders the narrators list only when selected file is M4B", async () => {
    // mount with non-M4B; assert no narrators section
    // mount with M4B; assert narrators section visible
  });

  it("appending a narrator updates state with status='new'", async () => {
    // ...
  });

  it("removed narrators render strikethrough with undo", async () => {
    // ...
  });
});
```

- [ ] **Step 2: Run tests; expect failures**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx -t narrators`
Expected: FAIL.

- [ ] **Step 3: Replace inline narrator `TagInput` with conditional `SortableEntityList<{ name: string }>`**

```tsx
{isM4b && (
  <SortableEntityList<{ name: string }>
    comboboxProps={{
      getOptionKey: (p: PersonWithCounts) => p.id,
      getOptionLabel: (p: PersonWithCounts) => p.name,
      hook: (q: string) => {
        const { data, isLoading } = usePeopleList({
          search: q,
          library_id: book.library_id,
        });
        return { data: data?.people, isLoading };
      },
      label: "Narrator",
    }}
    items={narrators.map((name) => ({ name }))}
    onAppend={(next) => {
      const name = "__create" in next ? next.__create : (next as PersonWithCounts).name;
      if (!narrators.includes(name)) setNarrators([...narrators, name]);
    }}
    onRemove={(idx) => setNarrators(narrators.filter((_, i) => i !== idx))}
    onReorder={(next) => setNarrators(next.map((n) => n.name))}
    pendingCreate={(n) => autoMatch.matches.narrators.find((m) => m.name === n.name)?.existing == null}
    status={(n) => (currentNarrators.includes(n.name) ? "unchanged" : "new")}
  />
)}
```

Add the removed-row strip pattern from Task 9.

- [ ] **Step 4: Run tests**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx -t narrators`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A app/components/library/
git commit -m "[Frontend] Wire IdentifyReviewForm narrators (M4B only)"
```

---

## Task 11: IdentifyReviewForm — wire series with `SortableEntityList`

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/IdentifyReviewForm.test.tsx`

- [ ] **Step 1: Write tests**

```tsx
describe("IdentifyReviewForm — series", () => {
  it("replaces the two text inputs with a SortableEntityList", async () => {
    // assert the combobox + per-row number input render
  });

  it("series number input updates row's number value", async () => {
    // type into the number input, assert state updated
  });

  it("'changed' status when number differs from current", async () => {
    // current has series 'Foo' #1; incoming has 'Foo' #2; assert status = 'changed'
  });

  it("'new' status when series not in current", async () => {
    // ...
  });
});
```

- [ ] **Step 2: Run tests; expect failures**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx -t series`
Expected: FAIL.

- [ ] **Step 3: Replace the inline series `<Input>` pair with `SortableEntityList<SeriesInput>`**

```tsx
<SortableEntityList<SeriesInput>
  comboboxProps={{
    getOptionKey: (s: SeriesWithCount) => s.id,
    getOptionLabel: (s: SeriesWithCount) => s.name,
    hook: (q: string) => {
      const { data, isLoading } = useSeriesList({
        search: q,
        library_id: book.library_id,
      });
      return { data: data?.series, isLoading };
    },
    label: "Series",
  }}
  items={seriesEntries}
  onAppend={(next) => {
    const name = "__create" in next ? next.__create : (next as SeriesWithCount).name;
    if (!seriesEntries.find((s) => s.name === name)) {
      setSeriesEntries([...seriesEntries, { name, number: undefined }]);
    }
  }}
  onRemove={(idx) => setSeriesEntries(seriesEntries.filter((_, i) => i !== idx))}
  onReorder={setSeriesEntries}
  pendingCreate={(s) => autoMatch.matches.series.find((m) => m.name === s.name)?.existing == null}
  renderExtras={(entry, idx) => (
    <Input
      className="w-24"
      onChange={(e) =>
        setSeriesEntries(seriesEntries.map((s, i) =>
          i === idx ? { ...s, number: e.target.value === "" ? undefined : Number(e.target.value) } : s,
        ))
      }
      placeholder="#"
      type="number"
      value={entry.number ?? ""}
    />
  )}
  status={(s) => computeSeriesStatus(s, currentSeries)}
/>
```

Add helper:

```tsx
function computeSeriesStatus(
  s: SeriesInput,
  current: SeriesInput[],
): "new" | "changed" | "unchanged" {
  const match = current.find(
    (c) => c.name.toLowerCase() === s.name.toLowerCase(),
  );
  if (!match) return "new";
  if ((match.number ?? null) !== (s.number ?? null)) return "changed";
  return "unchanged";
}
```

Add removed-series strip mirroring Task 9.

- [ ] **Step 4: Run tests**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx -t series`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A app/components/library/
git commit -m "[Frontend] Wire IdentifyReviewForm series to shared list"
```

---

## Task 12: IdentifyReviewForm — wire genres + tags with `MultiSelectCombobox` and per-chip status

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/IdentifyReviewForm.test.tsx`

- [ ] **Step 1: Write tests**

```tsx
describe("IdentifyReviewForm — genres/tags", () => {
  it("renders MultiSelectCombobox with per-chip status badges for genres", async () => {
    // mount; assert status badges per chip
  });

  it("renders removed genres as strikethrough with undo", async () => {
    // ...
  });

  it("supports select-or-create for tags", async () => {
    // type a new tag name, click Create, assert it's added with status='new'
  });
});
```

- [ ] **Step 2: Run tests; expect failures**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx -t "genres/tags"`
Expected: FAIL.

- [ ] **Step 3: Replace the inline `TagInput` for genres and tags**

Locate the two `<TagInput ... />` usages (around lines 957 and 1017/1035) and replace each with:

```tsx
{/* Genres */}
<MultiSelectCombobox
  isLoading={isLoadingGenres}
  label="Genre"
  onChange={setGenres}
  onSearch={setGenreSearch}
  options={(genresData?.genres ?? []).map((g) => g.name)}
  removed={currentGenres.filter((g) => !genres.includes(g))}
  searchValue={genreSearch}
  status={(v) => (currentGenres.includes(v) ? "unchanged" : "new")}
  values={genres}
/>

{/* Tags — same pattern with `tags` state */}
```

Wire `useGenresList` / `useTagsList` queries (mirroring the pattern in `BookEditDialog.tsx:154-170`) and `genreSearch` / `tagSearch` state.

- [ ] **Step 4: Run tests**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx -t "genres/tags"`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A app/components/library/
git commit -m "[Frontend] Wire IdentifyReviewForm genres + tags to MultiSelectCombobox"
```

---

## Task 13: IdentifyReviewForm — wire publisher and imprint with `EntityCombobox`

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/IdentifyReviewForm.test.tsx`

- [ ] **Step 1: Write tests**

```tsx
describe("IdentifyReviewForm — publisher/imprint", () => {
  it("publisher field is an EntityCombobox with select-or-create", async () => {
    // ...
  });

  it("imprint field is an EntityCombobox with select-or-create", async () => {
    // ...
  });

  it("publisher chip wears the pending-create marker for non-matched names", async () => {
    // ...
  });

  it("status badge reflects (current vs final) comparison", async () => {
    // current publisher = X; incoming = Y; assert status='changed'
  });
});
```

- [ ] **Step 2: Run tests; expect failures**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx -t "publisher/imprint"`
Expected: FAIL.

- [ ] **Step 3: Replace the inline `<Input>` for publisher and imprint with `EntityCombobox`**

Pattern from FileEditDialog refactor (Task 6 Step 4 / 5). Use `usePublishersList` and `useImprintsList`. Pass `pendingCreate` and `status` accordingly.

- [ ] **Step 4: Run tests**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx -t "publisher/imprint"`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A app/components/library/
git commit -m "[Frontend] Wire IdentifyReviewForm publisher + imprint to EntityCombobox"
```

---

## Task 14: IdentifyReviewForm — replace release date text input with `DatePicker`

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/IdentifyReviewForm.test.tsx`

- [ ] **Step 1: Write test**

```tsx
it("release date renders a DatePicker, not a plain text input", async () => {
  // assert the DatePicker trigger button is present (not <input type='text' placeholder='YYYY-MM-DD'>)
});

it("DatePicker selection updates the release_date state in ISO format", async () => {
  // open picker, select a date, assert state set to YYYY-MM-DD
});
```

- [ ] **Step 2: Run tests; expect failures**

- [ ] **Step 3: Locate and replace the `<Input ... placeholder="YYYY-MM-DD" />` for release date**

Use the existing `DatePicker` component (locate via `grep -rn "DatePicker" app/components/`). Mirror its usage in `FileEditDialog.tsx`.

- [ ] **Step 4: Run tests**

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A app/components/library/
git commit -m "[Frontend] Replace release-date text input with DatePicker in identify review"
```

---

## Task 15: IdentifyReviewForm — add sort title with `SortNameInput`

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/IdentifyReviewForm.test.tsx`

- [ ] **Step 1: Write tests**

```tsx
it("renders a SortNameInput for the book sort title beneath the title input", async () => {
  // ...
});

it("auto-generates sort title from title; manual override persists", async () => {
  // mirror SortNameInput behavior
});
```

- [ ] **Step 2: Run tests; expect failures**

- [ ] **Step 3: Add `SortNameInput` after the title `<Input>`**

Locate the title field and add directly below:

```tsx
<SortNameInput
  autoGenerateFrom={title}
  onChange={setSortTitle}
  value={sortTitle}
/>
```

Add `sortTitle` / `setSortTitle` state, initialized from `book.sort_title`. Include `sort_title` in the apply payload to `useUpdateBook`.

- [ ] **Step 4: Run tests**

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A app/components/library/
git commit -m "[Frontend] Add SortNameInput for book sort title in identify review"
```

---

## Task 16: IdentifyReviewForm — replace inline `IdentifierTagInput` with `IdentifierEditor`

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/IdentifyReviewForm.test.tsx`

- [ ] **Step 1: Write tests**

```tsx
describe("IdentifyReviewForm — identifiers", () => {
  it("initial list is current ∪ incoming, deduped by (type, value)", async () => {
    // current: [{ isbn, X }]; incoming: [{ isbn, X }, { asin, Y }]
    // assert: list contains exactly two rows
  });

  it("type dropdown excludes types already present", async () => {
    // ...
  });

  it("adding a new identifier updates state with status='new'", async () => {
    // ...
  });

  it("per-row delete and clear all work", async () => {
    // ...
  });

  it("apply payload contains the merged identifier list", async () => {
    // mock useUpdateFile; click apply; assert payload.identifiers
  });
});
```

- [ ] **Step 2: Run tests; expect failures**

- [ ] **Step 3: Replace `<IdentifierTagInput ... />` with `<IdentifierEditor ... />`**

Locate the read-only block (around line 1248) and replace:

```tsx
<IdentifierEditor
  identifierTypes={(pluginIdentifierTypes ?? []).map((t) => ({
    type: t.type,
    label: t.label,
  }))}
  onChange={setIdentifiers}
  status={(row) => computeIdentifierStatus(row, currentIdentifiers)}
  value={identifiers}
/>
```

Initialize `identifiers` state from `dedupeBy((row) => `${row.type}-${row.value}`, [...currentIdentifiers, ...(selectedResult?.identifiers ?? [])])`.

Add helper:

```tsx
function computeIdentifierStatus(
  row: IdentifierRow,
  current: IdentifierRow[],
): "new" | "unchanged" {
  return current.some((c) => c.type === row.type && c.value === row.value)
    ? "unchanged"
    : "new";
}
```

- [ ] **Step 4: Run tests**

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A app/components/library/
git commit -m "[Frontend] Wire IdentifyReviewForm identifiers to shared editor"
```

---

## Task 17: IdentifyReviewForm — wire `useAutoMatchEntities` for pending-create markers and loading state

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`
- Modify: `app/components/library/IdentifyReviewForm.test.tsx`

By this point all the field-task code already references `autoMatch.matches.<entity>` for `pendingCreate`. Now wire the hook.

- [ ] **Step 1: Write tests**

```tsx
describe("IdentifyReviewForm — auto-match", () => {
  it("renders a 'matching…' loading state while auto-match queries are in flight", async () => {
    // ...
  });

  it("after auto-match resolves, matched entities render plain (no pending-create marker)", async () => {
    // ...
  });

  it("non-matched entities render with the pending-create marker", async () => {
    // assert dashed-outline class on the chip / button
  });
});
```

- [ ] **Step 2: Run tests; expect failures**

- [ ] **Step 3: Call `useAutoMatchEntities` and gate the form on loading**

Near the top of `IdentifyReviewForm`:

```tsx
const autoMatch = useAutoMatchEntities({
  authors: (selectedResult?.authors ?? []).map((a) => a.name ?? a),
  narrators: selectedResult?.narrators ?? [],
  series: (selectedResult?.series ?? []).map((s) => s.name),
  publisher: selectedResult?.publisher,
  imprint: selectedResult?.imprint,
  genres: selectedResult?.genres ?? [],
  tags: selectedResult?.tags ?? [],
});

if (autoMatch.isLoading) {
  return (
    <div className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
      <LoadingSpinner /> Matching incoming entities…
    </div>
  );
}
```

(Adjust shape of `selectedResult` field accesses to match the actual `PluginSearchResult` definition.)

- [ ] **Step 4: Run tests**

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A app/components/library/
git commit -m "[Frontend] Wire useAutoMatchEntities into IdentifyReviewForm"
```

---

## Task 18: Remove dead helpers and unused imports

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx`

- [ ] **Step 1: Verify the helpers have no remaining call sites**

Run:
```bash
grep -n "TagInput\|AuthorTagInput\|IdentifierTagInput" app/components/library/IdentifyReviewForm.tsx
```

Expected: only the function *definitions* match — no `<TagInput ... />` JSX usages.

If any usages remain, finish Tasks 9–16 first.

- [ ] **Step 2: Delete the three function definitions and any imports they're the only consumer of**

Remove:
- `function TagInput(...) { ... }` (around line 323)
- `function AuthorTagInput(...) { ... }` (around line 392)
- `function IdentifierTagInput(...) { ... }` (around line 1293)

After deletion, run `pnpm lint:eslint app/components/library/IdentifyReviewForm.tsx` and remove any newly unused imports it flags.

- [ ] **Step 3: Run all identify-form tests + typecheck**

Run: `pnpm vitest run app/components/library/IdentifyReviewForm.test.tsx && pnpm lint:types`
Expected: PASS, no type errors.

- [ ] **Step 4: Commit**

```bash
git add app/components/library/IdentifyReviewForm.tsx
git commit -m "[Frontend] Remove dead inline helpers from IdentifyReviewForm"
```

---

## Task 19: Update website docs

**Files:**
- Modify: relevant `website/docs/*.md` page covering identify behavior (likely `metadata.md` or a dedicated identify page)

- [ ] **Step 1: Locate the page that describes the identify flow**

Run:
```bash
grep -rn "identify\|Identify" website/docs/
```

- [ ] **Step 2: Add a short paragraph (1–3 sentences) describing the new "will be created" pill on entity chips and that authors/series/publisher/imprint/narrators/genres/tags now use the same rich combobox UI as the edit forms**

Suggested wording:

> When you review identify results, fields like authors, series, publisher, and genres now use the same combobox inputs as the edit forms. Entities that already exist in your library show as plain chips; entities that don't show with a dashed outline and a "will be created" tooltip — they'll be created automatically when you apply the changes.

- [ ] **Step 3: Commit**

```bash
git add website/docs/
git commit -m "[Docs] Document identify review form input parity"
```

---

## Task 20: Final verification

- [ ] **Step 1: Run the full check suite**

Run: `mise check:quiet`
Expected: PASS — all tests green, lint clean, types clean.

If anything fails:
- Read the failing output.
- Fix the underlying issue (do NOT skip / xfail tests).
- Re-run.

- [ ] **Step 2: Manual smoke test**

Run `mise start`. Log in with `robin / password123`. From a book detail page:
1. Click Identify → run a search → pick a result.
2. Verify each field renders the new input.
3. Verify per-row status badges appear (new/changed/unchanged) on multi-value fields.
4. Verify the "will be created" pill shows on entities not in your library.
5. Edit a few fields, confirm the unsaved-changes dialog appears on close.
6. Apply changes; verify the book updates and any new entities show up in their respective list pages (admin → people, series, etc.).

- [ ] **Step 3: Final commit if any smoke-test fixes were needed**

Otherwise no commit.

---

## Done

After Task 20, the identify review form has full input parity with the edit forms, with per-row diff badges and an auto-match indicator, and the underlying input components are shared across all three dialogs.
