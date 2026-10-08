# Frontend review standards

Judgement rules a reviewer applies to a frontend diff, on top of `app/AGENTS.md`. Mechanical rules that ESLint or a test already enforces are not repeated here. Cite the rule's heading in a finding. When a rule and the dominant pattern in the codebase disagree, flag it rather than picking one silently.

## Design tokens

- Colors use semantic tokens (`bg-muted`, `text-muted-foreground`, `text-destructive`), whose CSS variables already handle dark mode. Palette classes for neutrals or the primary color drift from the theme. The theme has no success, warning, or info tokens, so status colors legitimately use a palette class with a `dark:` variant.
- Type scale, spacing, selected states, and warning boxes copy the nearest sibling that renders the same element rather than inventing classes. Flag a diff whose classes differ from its siblings for the same element.
- Page components use `rounded-md`, not `rounded-lg`.
- Responsive spacing scales up at `md:`, where the desktop sidebar appears.

## Page layout

- **Page headers with actions stack on mobile and sit side by side from a breakpoint**, with the title allowed to wrap and the action group not shrinking; copy `AdminUsers`.
- Header buttons with text hide the label on phones and keep the icon. An `aria-label` says what the control does ("Remove path"), or repeats the visible text when phones hide it.
- **List pages** have the header, a search input, a "Showing X-Y of Z" line only when `total > 0`, `LoadingSpinner` while loading, `QueryError` where results go, and shadcn `Pagination`; book and series grids use `Gallery`. Empty states distinguish "no results matching your search" from "nothing here yet".
- Searchable lists search server-side. List endpoints cap at 50 items, so client-side filtering hides everything past the first page.
- **Detail pages** put the mutating buttons in the header for roles that can write.

## Every page

- Every page sets a browser title with `usePageTitle`: plural noun for list pages, the entity's name for detail pages (`undefined` while loading), a descriptive name for settings pages. A static title is set before any early return.
- Tabbed views are deep-linked: an optional `/:tab?` route param validated against the allowed values, the first tab as default with a clean URL, controlled `value`, and `navigate()` in `onValueChange`. Tab lists scroll horizontally on narrow screens.

## Long text and overflow

- Nothing user-supplied causes horizontal scroll on a phone. Rows of stats and actions wrap. The shared `LibraryLayout` `<main>` has no `overflow-x-hidden`, so a non-wrapping row scrolls the whole page. Prefer per-element fixes (`break-words`, `min-w-0`, `minmax(0,1fr)` tracks) over a blanket `overflow-x-hidden`, which masks bugs and clips popovers and focus rings.
- `truncate` inside flex works only when the parent has `min-w-0`. Truncated text carries `title={text}`. Dialog titles wrap rather than truncate.

## Controls

- Overriding a `Button` variant's height, padding, or layout with `className` means a different size or variant fits better. Every clickable element shows `cursor-pointer`.
- Every clickable element is a `Button` (`variant="unstyled"` for rows, cards, and thumbnails), a link, or, where no `Button` fits (a slider), the matching ARIA role with `tabIndex` and key handling, so the keyboard can reach it. A toggle announces its state (`aria-pressed`, `aria-expanded`). Lint does not catch an `onClick` on a `div` or `span`.
- A region waiting for its content (page, section, dialog body, popover list) shows `LoadingSpinner`, not "Loading..." text, a skeleton, or a bare `Loader2`. A pending action inside a control (a Save button, a search input, a refetch over results already shown) uses `Loader2`.
- Durations, sizes, counts, and dates use the formatters in `@/utils/format` rather than inline math. A date-only value says "today" or "yesterday" for the last two days and is relative past that.

## Forms

- Unsaved changes protection ("Forms" in `app/AGENTS.md`) covers every form with a Save step: create and edit dialogs, settings pages, child editors with their own Save, and tab switches away from inline editing. Not needed: action dialogs that run immediately (merge, move, delete confirmations), quick actions with no pending state, view-only pages, settings that apply on change.
- Metadata fields are first-class: no helper text implying a field is secondary or derived ("Leave empty to use the title from file metadata").
- Clearing a metadata field saves the cleared value; it does not revert to a default. The next scan repopulates from the file if needed.

## Query cache

- A mutation invalidates every query that displays what it changed. Editing, merging, or deleting a genre, tag, series, person, or publisher also invalidates the book list and book detail queries, since books show that metadata.
