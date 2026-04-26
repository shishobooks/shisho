# Identify Review Form: Input Parity with Edit Forms

## Problem

The identify review form (`app/components/library/IdentifyReviewForm.tsx`) uses
weaker inputs than the book and file edit dialogs. Authors, narrators, series,
genres, tags, publisher, imprint, release date, and identifiers all fall back to
plain text inputs or basic chip lists, while `BookEditDialog` and
`FileEditDialog` provide server-backed comboboxes with select-or-create,
sortable lists, role selectors, identifier CRUD with validation, and a date
picker. Identifiers in the review form are read-only.

The review form should let users edit the same fields with the same
affordances as the edit forms, while preserving the review-specific diff/badge
model that distinguishes new, changed, and unchanged values from the incoming
plugin search result.

## Goals

- Bring `IdentifyReviewForm` to input-level parity with `BookEditDialog` and
  `FileEditDialog`.
- Keep the new/changed/unchanged badge model. Per-row badges for multi-value
  fields.
- Add identifier CRUD inside the review form, matching `FileEditDialog`'s
  editor and validation rules.
- Auto-match incoming plugin entity names against the local DB on form load and
  visually distinguish "exists" from "will be created" chips.
- Avoid future drift between the review form and the edit forms by sharing
  underlying input components.

## Non-Goals

- **Cover handling.** The current "current vs incoming" toggle (with page-pick
  badges for page-based files) stays as-is. No upload, no page picker added.
- **Narrators on non-M4B formats.** Hidden in review and dropped on apply,
  matching the edit-form storage rule that narrators only live on M4B files.
- **Backend changes.** All required search/create endpoints, validation, and
  identifier type-uniqueness already exist; this work consumes them.
- **Per-entity sort names** (sort name on a person or series). These remain in
  `MetadataEditDialog` and are not surfaced inside the review form. The
  book-level sort title *is* in scope.
- **Bulk identify.** The UI does not expose a bulk identify flow; this work
  only touches the single-book review.

## Approach

Extract the inline `Popover + Command` entity-search patterns out of
`BookEditDialog` and `FileEditDialog` into reusable components in
`app/components/common/`. The same components are then composed inside
`IdentifyReviewForm`. This is the only path that keeps the two forms aligned
long-term — copy-paste alternatives leave the same review-form complaint open
the next time a new field is added.

### New shared components

#### `EntityCombobox<T>`

Generic server-backed search-or-create combobox built on `Popover` + `Command`.

Props:

- `hook: (query: string) => { data: T[]; isLoading: boolean }` — e.g.
  `usePersonSearch`, `useSeriesSearch`, `usePublisherSearch`, `useImprintSearch`.
- `label: string` — used in placeholder, "Create '…'" CTA, and a11y.
- `value: T | null`
- `onChange: (next: T | { __create: string }) => void`
- `canCreate?: boolean` — default `true`. When `false`, hides the create CTA.
- `exclude?: (item: T) => boolean` — hides already-chosen items from the
  dropdown (e.g., authors already in the list).
- `status?: "new" | "changed" | "unchanged"` — optional badge slot used by the
  review form. Edit dialogs omit this prop.

The `{ __create: string }` payload signals the parent to instantiate a pending
entity; the parent reconciles to a real entity with an `id` after the create
mutation succeeds on apply.

#### `SortableEntityList<T>`

A list of `EntityCombobox` rows with drag-reorder, remove, and an append row.

Props:

- `items: T[]`
- `onReorder: (next: T[]) => void`
- `onRemove: (index: number) => void`
- `onAppend: (entity: T | { __create: string }) => void`
- `comboboxProps`: forwarded to each row's `EntityCombobox` (label, hook,
  `canCreate`, `exclude`).
- `renderExtras?: (item: T, index: number) => ReactNode` — slot for per-row
  extras: author role select, series number input.
- `status?: (item: T, index: number) => "new" | "changed" | "unchanged" | undefined`
  — per-row badge resolver for the review form.

Used for: authors, narrators, series.

Drag-and-drop is implemented on top of the existing `DraggableBookList`
infrastructure (`@dnd-kit/core` + `@dnd-kit/sortable`), reusing the
`DragHandleProps` shape already exported from that file. Don't introduce a
second drag library.

#### `IdentifierEditor`

Extracted from `FileEditDialog`. Behavior unchanged:

- Type dropdown auto-excludes types already present in the list (uniqueness
  enforced backend-side as of `b51c142`).
- Value input with `validateIdentifier` pattern matching; invalid values block
  add and show inline validation.
- Per-row delete and clear-all.
- Optional `status?: (identifier) => "new" | "changed" | "unchanged" | undefined`
  for review-form per-row badges.

### Existing shared components reused as-is

`LanguageCombobox`, `SortNameInput`, `DatePicker`. No changes needed.

### `MultiSelectCombobox` extension

Genres and tags are stored as plain strings (`values: string[]`), not entity
objects. The current `MultiSelectCombobox` (`app/components/ui/MultiSelectCombobox.tsx`)
already supports server-side search, select-or-create, and chip removal — but
it has no per-chip status slot.

We add an optional prop:

```
status?: (value: string) => "new" | "changed" | "unchanged" | "removed" | undefined
```

When set, the chip rendered for each value picks up the corresponding badge
treatment. Edit dialogs continue to call `MultiSelectCombobox` without this
prop — behavior unchanged. Removed values (present in current, absent in
final) render as strikethrough chips with an undo button, same pattern as the
multi-entity lists.

### Edit dialog refactor

`BookEditDialog` and `FileEditDialog` are refactored to consume the three new
components. The diff is removal + a few imports — not a rewrite. No behavior
changes; existing tests must continue to pass, plus new tests are added (see
Testing).

## IdentifyReviewForm field changes

| Field | New input | Notes |
|---|---|---|
| Title | `Input` (unchanged) + `SortNameInput` for sort title | Sort title auto-generates from title with manual override, matching `BookEditDialog`. |
| Subtitle | `Input` (unchanged) | — |
| Authors | `SortableEntityList<Person>` over `EntityCombobox(usePersonSearch)` | Per-row role `Select` rendered only when the selected file is CBZ, matching `BookEditDialog`'s conditional. |
| Narrators | `SortableEntityList<Person>` over `EntityCombobox(usePersonSearch)` | Visible only when selected file is M4B. Hidden and dropped on apply otherwise. |
| Series | `SortableEntityList<Series>` over `EntityCombobox(useSeriesSearch)` | Series number input is the per-row extras slot. |
| Genres | `MultiSelectCombobox` (existing) | — |
| Tags | `MultiSelectCombobox` (existing) | — |
| Description | `Textarea` (unchanged) | — |
| Publisher | `EntityCombobox(usePublisherSearch)` | Single-value. |
| Imprint | `EntityCombobox(useImprintSearch)` | Single-value. |
| Release date | `DatePicker` (existing) | Replaces the manual `YYYY-MM-DD` text input. |
| URL | `Input` + external link button (unchanged) | — |
| Language | `LanguageCombobox` (unchanged) | — |
| Abridged | tri-state checkbox (unchanged) | — |
| Identifiers | `IdentifierEditor` (new shared) | Initial list is current ∪ incoming, deduped by `(type, value)`. Per-identifier badges. |
| Cover | unchanged | Out of scope. |

The legacy `TagInput`, `AuthorTagInput`, and `IdentifierTagInput` functions
defined inline inside `IdentifyReviewForm.tsx` are unused outside this file and
will be deleted as part of the refactor.

## Auto-match: "exists" vs "will be created"

When the review form receives a `selectedResult`, a new hook
`useAutoMatchEntities(result)` runs once and resolves named-entity fields
(authors, narrators, series, publisher, imprint, genres, tags) against the
local DB.

- Matches resolve to existing DB entities (full object with `id`).
- Non-matches stay as `{ name, __create: true }` and render with a distinct
  pending-create marker (e.g., dashed outline or `+` prefix) on the chip, with
  a "Will be created on apply" tooltip. This marker is visually different from
  the green new/changed/unchanged status badge and can co-occur with it.
- If the user opens the combobox and picks an existing entity instead, the
  pending-create marker disappears.
- Typing an unmatched name in the combobox surfaces the same "Create '…'" CTA
  the edit forms already provide; the resulting chip carries the
  pending-create marker.

While the auto-match is in flight, fields render in a brief skeleton/disabled
state with a "matching…" indicator to avoid flashing pending-create markers
that immediately resolve to existing entities.

### Apply path

When the form submits:

1. Pending-create entities are created via the existing creation endpoints (the
   same code path edit forms use today for "Create new…").
2. The resulting IDs are passed to the book/file update mutation.
3. On any creation failure, the apply aborts and a toast surfaces — same
   pattern as the edit forms; no partial apply.

## Status badges with the new inputs

The review form's diff model continues to drive what users see, but the badge
location moves for multi-value fields.

**Single-value fields** (title, subtitle, publisher, imprint, language,
release date, description, URL, abridged, sort title): badge logic unchanged,
derived from `(currentValue, finalValue)`.

**Multi-value fields** (authors, narrators, series, genres, tags,
identifiers):

- Per-row badge derived by comparing the row's identity against the current
  set:
  - **`unchanged`** — entity exists in current set with same identity and same
    per-row extras (role, series number).
  - **`changed`** — entity exists in current set but a per-row extra differs.
  - **`new`** — entity not present in current set.
- Removed entities (in current, absent in final) render below the active list
  as strikethrough chips with an undo affordance.
- Field-level header badge collapses per-row statuses: shows "changed" if any
  row is new/changed/removed, otherwise "unchanged."

The pending-create marker is orthogonal to status — both can appear on the
same chip.

### Reset / use-incoming controls

Each field keeps its existing per-field "reset to current" / "use incoming"
controls. For multi-value fields:

- **Reset to current** restores the field to the current value.
- **Use incoming** replaces the field with the incoming value (deduped against
  existing rows where applicable).

## Unsaved-changes guard

Already wired. `IdentifyBookDialog` wraps the review step in `FormDialog` with
`hasChanges={step === "review" && reviewHasChanges}`. The review form
continues to call `useHasChangesChange` as new fields are edited.

## Testing strategy

Three layers must all pass `mise check:quiet`.

### Layer 1 — Unit tests for extracted components

Each new component gets a dedicated test file alongside it.

`EntityCombobox.test.tsx`:

- Selecting an existing match emits the entity.
- Typing an unmatched name surfaces "Create '…'" and `onChange` is called with
  `{ __create }` payload.
- `exclude` predicate hides items from the dropdown.
- `status` prop renders the corresponding badge.
- `isLoading` from the hook renders skeleton.
- `canCreate={false}` hides the create CTA.

`SortableEntityList.test.tsx`:

- Renders N items.
- Reorder via drag fires `onReorder` with the new order.
- Remove fires `onRemove`.
- Append via embedded `EntityCombobox` fires `onAppend`.
- `renderExtras` is called per row and the returned element receives the
  correct row context.
- Per-row `status` resolves to per-row badge.

`IdentifierEditor.test.tsx`:

- Type dropdown excludes already-present types.
- Invalid value blocks add and shows validation message.
- Valid add appends to list.
- Per-row delete works.
- Clear-all empties list.
- Per-row `status` badge renders.

### Layer 2 — Edit dialog regression tests

Existing `BookEditDialog.test.tsx` and `FileEditDialog.test.tsx` files stay; we
add cases that exercise the now-shared inputs end-to-end through each dialog
to confirm the refactor preserved behavior.

`BookEditDialog`:

- Adding, removing, and reordering authors works.
- CBZ file shows author role select; non-CBZ hides it.
- Series number editable per row.
- Series search and "Create new series" both work.
- Genre/tag multi-select unchanged.

`FileEditDialog`:

- Adding/removing identifiers, including type-uniqueness exclusion.
- Narrator combobox visible for M4B.
- Publisher/imprint single-value combobox with select-or-create.
- Release date picker.
- Cover handling unchanged.

### Layer 3 — IdentifyReviewForm parity tests

Existing `IdentifyReviewForm.test.ts` is expanded (renamed to `.test.tsx` if
component-level rendering is needed):

- **Auto-match flow**: given a `PluginSearchResult` whose author/publisher/
  series names exist in the DB, the form initializes with existing-entity
  chips (no pending-create marker); names that don't match render with the
  marker.
- **Per-row status badges**: incoming author already present → `unchanged`;
  incoming author with different role → `changed`; incoming author not in
  current → `new`; current author absent from incoming → strikethrough removed
  chip with undo.
- **Identifier merge**: initial list is current ∪ incoming deduped by
  `(type, value)`; per-identifier badges correct.
- **Narrator visibility**: only when the selected file is M4B; toggling the
  file selector hides/shows the row.
- **Apply path**: submitting with a pending-create entity triggers the create
  call, then the update call with the new ID.
- **Apply failure**: a creation error aborts the update and surfaces a toast;
  no partial apply.
- **Unsaved-changes guard**: editing any new field marks `hasChanges`;
  closing the dialog triggers `UnsavedChangesDialog`.

### Coverage gate

Every public prop of each new shared component is exercised in at least one
test. New behaviors in `IdentifyReviewForm` (auto-match, identifier editing,
per-row status, narrator gating) all have test coverage.

## Conventions to follow

The frontend `CLAUDE.md` rules apply throughout. Specifically relevant here:

- **`cn()` for dynamic classNames** — no template literals.
- **`cursor-pointer` on every clickable element** — including chip remove
  buttons, undo buttons, drag handles, create-CTA rows.
- **`forwardRef` on any custom component used as `asChild`** for a Radix
  trigger — applies if any new component (e.g., a custom chip-row trigger)
  wraps `Popover`/`DropdownMenu`/`Tooltip`.
- **Cross-resource Tanstack Query invalidation** — when the apply path
  creates a new entity (person, series, publisher, imprint, genre, tag), it
  must invalidate both the entity's own query and the book queries
  (`ListBooks`, `RetrieveBook`) that display it. The existing edit-form
  create paths already do this; the auto-match apply must reuse those same
  mutations rather than calling the API directly.
- **`userEvent.setup({ advanceTimers: vi.advanceTimersByTime })`** in tests
  that interact via clicks/typing, since fake timers are enabled globally.
- **First-class metadata fields** — no helper text like "leave empty to use
  the title from file metadata" on the new inputs.

## Documentation

If user-visible behavior changes (likely a sentence about the "will be
created" pill in the review flow), update the corresponding page in
`website/docs/`. No new doc page expected.

## Risks

- **Edit dialog regressions during extraction.** Mitigated by Layer-2
  regression tests and by keeping the refactor mechanical (lift-and-shift,
  not redesign).
- **Auto-match performance** for results with many genres/tags. Mitigated by
  batching lookups in a single call per entity type rather than one per name.
- **Visual collision** between the green "new" status badge and the
  pending-create marker. Mitigated by using visually distinct treatments
  (color vs. shape/outline) and verifying with a designer-style spot check
  before landing.
