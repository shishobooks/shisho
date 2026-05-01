# Identify flow redesign

**Status:** Spec, ready for plan
**Date:** 2026-05-01
**Companion:** `docs/design/identify-mockups/index.html` (interactive mockup)

## Why

The current identify flow has three problems:

1. **Multi-file books are surprising.** Identify writes book-level fields (title, authors, description, etc.) and file-level fields (narrators, identifiers, cover, etc.) at the same time. When a user identifies a non-primary file, they don't realize it can overwrite the book's metadata that was set by the primary file's earlier identify.
2. **`file.Name` is clobbered.** `persistMetadata` always copies the new title into `file.Name`, which destroys legitimate edition-specific filenames like "Harry Potter and the Sorcerer's Stone (Full-Cast Edition)" when identifying against a generic "Harry Potter and the Sorcerer's Stone" plugin result.
3. **The review UI is busy and one-way.** Multi-value fields show every current value as strikethrough chips plus every new value as new chips, with a single "Use current" all-or-nothing escape hatch. Editing the proposed values is awkward, and there's no way to keep some current entries while accepting others.

This spec redesigns the review step of the identify dialog. Search step gets a light visual refresh only.

## Design principles

- **Identify is a per-file operation.** Plugins typically pick a specific edition (ASIN, ISBN). The dialog scopes its decisions accordingly.
- **Per-field opt-in.** The user decides per field whether to accept the plugin's proposed value. Atomic. Inspired by Audiobookshelf.
- **Cohesion.** Whatever input patterns this dialog adopts (combobox typeahead, plain-text dates, composite rows) port to the rest of the app — `BookEditDialog`, `FileEditDialog`, metadata edit forms, filter pickers — not just identify.
- **Smart defaults over micromanagement.** The dialog auto-checks the right boxes given context (primary vs non-primary file, changed vs new field). The user mostly clicks Apply.

## Scope

**In scope:** Review step of the identify dialog (`IdentifyReviewForm.tsx`). Per-field controls, scope sectioning, sticky headers, the cover/title/authors/series/genres/tags/description/narrators/publisher/imprint/language/release-date/identifiers/abridged rows. Footer actions. The same patterns ported into `BookEditDialog` and `FileEditDialog`.

**Out of scope (this spec):**
- Full search-step redesign. Search keeps its existing functionality and gets a light visual refresh — same dialog frame, back arrow, file switcher with dropdown, source pill — but no structural change to the result-picker grid.
- New plugin behaviors. The plugin SDK and `handler_apply_metadata.go` semantics stay the same except where this spec calls them out.
- New model fields. The "reviewed" concept used here is the existing `file.reviewed` / `ReviewOverride` system in `pkg/books/review/`.

## Decision model

- **Per-field checkbox** on every row. Checked = this field's proposed value will be written on Apply. Unchecked = skipped.
- **Section-level select-all** on each section banner. Toggles every checkbox in the section.
- **Global "Apply all"** at the top of the body toggles every checkbox in the dialog.
- **Three checkbox states**: `false`, `true`, `mixed`. Mixed renders as a filled background with a horizontal bar (not a check). Mixed appears on section and global toggles when some-but-not-all child rows are checked. Clicking a mixed checkbox sets it to `false` (matches browser indeterminate behavior).
- Hover affordance only on **unchecked** boxes; checked and mixed boxes brighten to a slightly lighter primary on hover, never to a "looks unchecked" treatment.

## Smart defaults

Every field starts with one of three states:

- **`changed`** — the current value differs from the proposed value (this field will overwrite).
- **`new`** — there is no current value; the proposed value would fill it in.
- **`unchanged`** — current and proposed match (or the plugin didn't propose anything). The row is editable, but the checkbox defaults off.

Default checkbox values:

- **File-level fields, any state**: default ON.
- **Book-level `new` fields**: default ON (no overwrite — there's nothing to lose).
- **Book-level `changed` fields, primary file**: default ON (primary is the canonical file; identifying it should propagate to the book).
- **Book-level `changed` fields, non-primary file**: default OFF (the "second-identify" case — don't overwrite shared book metadata from a non-canonical source).
- **Unchanged fields**: default OFF.

The `Restore suggestions` button (footer left) reverts every checkbox and field value to these defaults. Useful when a user has flipped many boxes and wants a clean slate without navigating away.

## Section organization

Two sections: **Book** and **File**. Each is collapsible.

- **Book section** ("applies to all files"): Title, Subtitle, Authors, Series, Genres, Tags, Description.
- **File section** (file metadata shown inline in banner — see below): Cover, Narrators, Publisher, Imprint, Language, Release date, Identifiers, Abridged.

### Section banner

A single sticky row per section. Contents:

- Chevron icon (rotates -90° when collapsed)
- Section label (`BOOK` or `FILE`) — uppercase, tracked, weighted
- For Book: hint text "applies to all files"
- For File: inline file metadata — type label + `file.Name` (no extension) + adapt-by-file-type metadata (duration + bitrate for audiobooks; page count for ebooks/comics/PDFs; etc.)
- Count: "X of Y selected" (always visible, including when collapsed)
- Section-level checkbox (supports mixed state)

Click anywhere on the banner (except the section's own checkbox) to toggle expanded/collapsed. Section checkbox stops propagation.

Banner alignment: the chevron sits in the same column as field-row checkboxes (24px wide), so the section label aligns with field labels at column 2 (60px from container left).

### Default collapsed state

A section is **collapsed by default iff its selected count is 0**.

This handles the cases naturally:
- **First identify** (single file, mostly `new` and `changed` fields, all default-on): Book and File both have non-zero counts → both expanded.
- **Second identify on non-primary** (book-level `changed` defaults off): if there are no `new` book fields, Book count is 0 → collapsed by default. If the second file's plugin has data the first file's plugin didn't (e.g., a Subtitle), that field is `new` → defaults on → count is 1 → section stays expanded.
- **Second identify on primary**: book-level `changed` defaults on → counts > 0 → both expanded.

## Sticky headers

The dialog body is a vertically scrolling container. The following stay visible while scrolling:

- **Dialog frame head** (back arrow, title, source pill): outside the scroll body entirely.
- **Global select-all bar**: `position: sticky; top: 0; z-index: 3` within the scroll body.
- **Section banners**: `position: sticky; top: 49px; z-index: 2` (offset by selectbar height). When the user scrolls into a section's rows, its banner pins below the selectbar; scrolling past the section lets the next banner take over.

Typeahead popovers use `z-index: 50` to clear sticky banners when an entity-input dropdown is open.

All sticky elements have solid backgrounds so content scrolling underneath is occluded cleanly.

## Field controls

Every row has the structure:

```
[checkbox]  [label]  [status badge]
            [field control]
            [Currently: value …  inline action (optional)]
```

Where `[checkbox]` is the per-field apply toggle. Status badge: `Changed` (primary soft) or `New` (success soft); no badge for unchanged. The "Currently:" reference shows the current value of that field (book or file scope as appropriate).

### Per-field rules

- **Title**: plain text input. If the title contains a colon, show an "Extract subtitle" button inline with the "Currently:" reference (right-aligned, same baseline). Clicking it removes everything after the colon from the title and writes that text into the Subtitle field. Mirrors `BookEditDialog.tsx:526` behavior — reuse the same component.
- **Subtitle**: plain text input.
- **Authors**: composite list — drag handle + name + role dropdown + remove. Roles: Author / Translator / Editor / No role. Add row is a combobox-style typeahead (see below). Order is preserved.
- **Narrators**: composite list — drag handle + name + remove. No role. Add row is a combobox typeahead.
- **Series**: composite list — drag handle + name + number input (tabular-nums, ~50px wide) + unit dropdown (— / Volume / Chapter) + remove. Add row is a combobox typeahead.
- **Genres / Tags**: chip group with always-visible × per chip. Chips are 28px tall, 999px radius. Add input is a chip-shaped combobox (typeahead).
- **Description**: `<textarea>`, min-height ~88px, vertical resize. The "Currently:" reference is line-clamped to 2 lines by default with a "Show full" toggle that expands to the full original; "Show less" collapses back. No markdown, no rich text.
- **Cover**: same row layout for all file types. Side-by-side comparison: current thumbnail → new thumbnail (highlighted with primary ring). Caption under each shows resolution (`1280×1920`) for image-based covers (M4B/EPUB) or `Page N` for page-based covers (CBZ/PDF). A trailing button switches by `file_type`:
  - **M4B/EPUB**: "Upload cover" — opens file picker.
  - **CBZ/PDF**: "Select page" — opens existing `PagePicker` dialog.
- **Publisher**: combobox single-value with caret + dedicated clear button.
- **Imprint**: same combobox. Even when "Unchanged" (no incoming value), input *and* checkbox stay enabled — user can manually fill and apply.
- **Language**: same. Same enabled-when-unchanged rule.
- **Release date**: **plain text input**, never a date picker. Placeholder `YYYY-MM-DD`. Supports old, partial, or unknown dates. The current `<DatePicker>` in `BookEditDialog` and `FileEditDialog` must also be swapped to plain text — site-wide change.
- **Identifiers**: chips render `[TYPE] value` with `TYPE` uppercase in primary color. Add row is horizontal: type-select (only available types listed; greyed when all types already added) + value input (monospace) + Enter on value commits.
- **Abridged**: outer row checkbox enabled regardless of state. Inner Abridged toggle is disabled until the outer is checked. When the outer flips on, the inner becomes interactive.

### Multi-value entity inputs (combobox typeahead)

Replaces today's `[+ Add tag]` buttons. Pill-shaped (for chip groups) or row-shaped (for composite lists), with a caret on the right.

- Click or focus opens a dropdown popover.
- Type to filter.
- Dropdown groups:
  - `IN YOUR LIBRARY` — matching existing entities, sorted by relevance, with usage counts ("12 books").
  - Separator.
  - `Create new {type} "text"` — escape hatch for novel values.
- Up/Down arrows navigate; **Enter** commits the highlighted suggestion (or the create-new option if no match is highlighted); **Esc** closes.
- On commit, the suggestion becomes a chip/composite row above the input; the input clears and stays focused so the user can type the next entry.
- No "↵ to add" hint label needed — the caret + dropdown carry the affordance.

Applies to: Authors, Narrators, Series, Genres, Tags.

## Two-step flow & navigation

### Step 1: search

User selects which file is being identified (file switcher with dropdown), enters a query, gets plugin results, picks one. Search step gets a **light visual refresh** — same dialog frame, back arrow → close, source pill, file switcher — but no structural change to its result-picker grid. (Full search redesign is out of scope; this spec covers the visual cohesion only.)

### Step 2: review

The form designed in this spec. File switcher here is **read-only** (just shows which file is being identified, no caret, no dropdown). Switching files happens on the search step — the file determines what searches against the plugin, so it makes no sense to switch on review.

### Default file selection on dialog open

Uses the existing `file.reviewed` flag (computed by `review.IsComplete(book, file, criteria)` against per-library required fields, with optional `ReviewOverride`):

1. If some files are reviewed and some aren't, pick a non-reviewed file (`reviewed != true` — covers both `false` and `NULL`).
2. If all files share the same reviewed status, prefer the primary file if set.
3. Otherwise pick the first file. Order doesn't matter much in this case.

No new model fields needed. Builds on the review system already in `pkg/books/review/`.

### Back navigation (review → search)

Search state (query, results, selected result) is **preserved**. Review state **resets on every forward navigation** — going back and forward is an intentional way to clear changes (matches today's behavior). The `Restore suggestions` button covers the in-place reset case.

### Apply behavior

Inline loading + close on success.

- Click Apply → button shows spinner and "Applying…" while the request is pending.
- On success: dialog closes, toast confirms ("Updated 6 fields").
- On error: dialog stays open with an inline error banner so the user can retry without losing checkbox state.
- File reorganization (renames triggered by title/author/series changes) is an implementation detail and is **not** surfaced in the toast.

### Multi-file post-apply

Dialog just closes. No "identify the other file?" prompt. User re-opens identify from the book detail page when they want to identify additional files.

### NameSource / source attribution

After Apply:

- Fields whose values come from the plugin's proposed result are saved with their respective `*Source` column set to `"plugin"`.
- Fields whose values were manually edited by the user in the review form before Apply are saved with source `"user"`.

This matters for subsequent scanner re-identify: user edits must survive a re-scan; plugin-set values can be overwritten by a re-scan that finds better data.

### Identify and reviewed flag

Identify does not directly set `reviewed`. After Apply, the existing recompute pipeline runs (`RecomputeReviewedForBook` / `RecomputeReviewedForFile`); if the applied changes brought the file to completeness, it becomes `reviewed = true` automatically. Users with an explicit `ReviewOverride` keep their override.

## Dialog frame

- **Width**: `max-w-3xl` (768px), one step wider than the existing edit dialog because each row carries current-vs-proposed.
- **Head**: back arrow (top-left, 24px button, 12px chevron-left icon) + title + subtitle (file count + N changes proposed) + source pill (right). No close X — back arrow returns to the search step.
- **Body**: `max-height: 70vh; overflow-y: auto`. Sticky select-all + sticky section banners as defined above.
- **Foot**: Restore suggestions button (ghost, left side) + counts split by section ("**1 book change** · **5 file changes** selected") in the middle + Cancel/Apply on the right. Primary action "Apply N changes" with N = total selected.

## Cohesion — across the whole app

Identify is one of several places these patterns surface. The implementation must avoid one-off treatments here — every foundational change adopted in this dialog also lands wherever it appears in Shisho.

- **Combobox typeahead** replaces existing `EntityCombobox` / `MultiSelectCombobox` / `IdentifierEditor` add affordances site-wide: `BookEditDialog`, `FileEditDialog`, metadata-entity edit forms (Genres, Tags, People, Series), filter pickers, and any other place chips/composite lists are entered.
- **Composite rows** (authors+role, series+number+unit) share the `SortableEntityList` component used by the edit dialogs. Don't build a parallel implementation.
- **Plain-text date input** replaces `<DatePicker>` wherever release dates, publish dates, or other "could be old" dates appear — including `FileEditDialog`, library scan filters, series metadata, etc.
- **Title's "Extract subtitle" button** is the same component used today in `BookEditDialog`; reused as-is.
- **Visual tokens** (radius, border alpha, primary, success, mixed-state checkbox style, status badges, chip styling, etc.) come from `app/index.css` and shared component CSS. If a token changes here, it changes everywhere it's used; no in-file overrides.
- **Checkbox states** (false / true / mixed) and their hover behavior are a global change — every `Checkbox` usage in the app must support the indeterminate state and never show the "looks unchecked on hover" bug for checked or mixed boxes.
- **Outer-controls-inner pattern** for fields like Abridged is reusable; if other forms gain a similar "apply this field" gate, they inherit the same disabled-until-applied behavior.

## Visual specifics

- **Surface stack** (darkest → lightest): body `oklch(0.18)` → dialog card `oklch(0.22)` → row container `oklch(0.22)` → selectbar `oklch(0.20)` → section banner `oklch(0.205)`.
- **Status badge palette**: `Changed` = primary soft; `New` = success soft. Unchanged rows get no badge.
- **Chips**: 28px tall, 999px radius, always-visible × on each chip (no hover-to-show). × hover state = danger-soft background.
- **Checkboxes**: 18×18, 5px radius. Primary fill when checked or mixed. Mixed visual = horizontal bar in primary-fg.
- **Atmosphere**: faint top-of-page radial gradient (`oklch(0.8 0.15 280 / 0.05)`); no other glow effects.
- **Icons**: every icon in the implementation uses `lucide-react` — chevron-down, chevron-left, x, plus, upload, refresh-ccw, etc. Match icon sizes to surrounding type (12–16px is the typical range).

## Open items intentionally deferred

- **Validation on inputs** (invalid ISBN/ASIN format) — match whatever `BookEditDialog` does today.
- **Cancel with unsaved edits** — leverage existing `UnsavedChangesDialog` pattern from `app/CLAUDE.md`.
- **Search step empty results** — search step concern; light visual refresh keeps current empty state.
- **Permission gating** — identify currently sits behind plugin permissions; spec inherits that.
- **Filter "Changed/All" toggle in selectbar** — count reflects total changes regardless of filter; filter only changes which rows render.

## Implementation order (suggested)

1. Backend: fix the `file.Name` clobber bug. `persistMetadata` no longer copies the new title into `file.Name`. Identify does not surface `file.Name` as a form field — users edit it via `FileEditDialog`. This eliminates the case where identifying a non-primary file's audiobook erases an edition-specific filename like "Harry Potter and the Sorcerer's Stone (Full-Cast Edition)".
2. Frontend: shared primitives — checkbox with `mixed` state, combobox typeahead component, composite-row primitive, sticky-section-banner primitive.
3. Port primitives into `BookEditDialog` / `FileEditDialog` first (cohesion). Verify no regressions.
4. Build the new `IdentifyReviewForm` against the new primitives.
5. Light visual refresh of the search step.
6. Plain-text date input swap site-wide.
7. End-to-end tests covering: first identify, second identify on primary, second identify on non-primary, restore suggestions, multi-file flow.

## Reference

- Mockup: `docs/design/identify-mockups/index.html`
- Current implementation: `app/components/library/IdentifyBookDialog.tsx`, `app/components/library/IdentifyReviewForm.tsx`, `pkg/plugins/handler_apply_metadata.go`, `pkg/plugins/handler_persist_metadata.go`
- Edit dialogs (cohesion target): `app/components/library/BookEditDialog.tsx`, `app/components/library/FileEditDialog.tsx`
- Review system: `pkg/books/review/`
- Visual tokens: `app/index.css`
