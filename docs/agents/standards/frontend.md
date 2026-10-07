# Frontend review standards

Judgement rules a reviewer applies to a frontend diff. Mechanical rules that ESLint or a test already enforces are listed in `app/AGENTS.md` and are not repeated here. Cite the rule's heading in a finding. When a rule and the dominant pattern in the codebase disagree, flag it rather than picking one silently.

## Design tokens

- Colors use semantic tokens (`bg-muted`, `text-muted-foreground`, `border-primary`, `text-destructive`), whose CSS variables already handle dark mode. Hardcoded palette classes for neutrals or the primary color (`text-gray-*`, `bg-neutral-*`, `dark:bg-neutral-*`, `dark:text-violet-*`) drift from the theme. The theme has no success, warning, or info tokens, so status colors (green, amber, blue) legitimately use a palette class with a `dark:` variant.
- Page titles: `text-2xl font-semibold`. Dialog titles: `text-sm font-semibold`. Section headings inside a page: `text-base md:text-lg font-semibold` in cards, `text-xl font-semibold mb-4` for full-width page sections.
- Page header margin `mb-6 md:mb-8`; card and section padding `p-4 md:p-6`; dialog body `space-y-6`.
- Page components use `rounded-md`, not `rounded-lg`. Hover backgrounds use `hover:bg-muted/50`.
- Selected card: `border-primary bg-primary/5`, with `border-transparent` when unselected. Selected toggle chip: `border-primary bg-primary/5 text-primary`.
- Inset inline warning: `rounded-md bg-destructive/10 border border-destructive/20 p-3`.
- Full-width dialog error banner above the footer: `shrink-0 border-t border-destructive/20 bg-destructive/10 px-5 py-3 text-sm text-destructive`, square edges, no side borders.
- Danger zone: `space-y-3 rounded-md border border-destructive/40 p-4 md:p-6` with a `text-lg font-semibold text-destructive` title (`PluginDangerZone.tsx`).
- Muted status badge: `bg-muted text-muted-foreground`.
- Responsive spacing scales up at `md:` (`gap-4 md:gap-8`, `space-y-4 md:space-y-6`, `py-3 md:py-4 px-4 md:px-6`). Breakpoints: `sm:` 640px, `md:` 768px (desktop sidebar appears), `lg:` 1024px.

## Page layout

- **Page headers with actions stack on mobile and sit side by side from a breakpoint.** The container is `flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between` (`gap-4 mb-6 md:mb-8` on list and settings pages such as `AdminUsers`, `ListsIndex`, `AdminPlugins`; `md:` instead of `sm:` on Book and File detail, whose titles run long). The title gets `min-w-0 break-words`, the action group `shrink-0`. A description goes under the title as `text-sm md:text-base text-muted-foreground`. `ResourceDetail` and `SeriesDetail` keep one row at every width and `ListDetail` always stacks; neither is the model for new pages.
- Header action buttons with text use an icon plus `<span className="hidden sm:inline">` label (`sm:mr-2` on the icon), so the row fits a phone. Icon-only buttons need an `aria-label`; disclosure toggles also need `aria-expanded` and a label that says what they do ("Show file details").
- **List pages** have the header (title and subtitle), a search input with `max-w-xs`, a "Showing X-Y of Z items" line shown only when `total > 0`, `<LoadingSpinner />` while loading, `QueryError` where results go on failure, and shadcn `Pagination`. Grids of books or series use `Gallery`. Empty states distinguish "no results matching your search" from "nothing here yet".
- Searchable lists search server-side. List endpoints cap at 50 items, so client-side filtering hides everything past the first page.
- Cover galleries take their size and paging from `useGallerySizeParam`, and gate their query on its `settingsResolved` so the page does not fetch once with the default size and again with the saved one.
- **Detail pages** put the mutating buttons in the header for roles that can write; content sections are `<section className="mb-10">` with an `h2`; an empty section reads `text-center py-8 text-muted-foreground`.
- The sidebar is hidden below `md:` (`hidden md:block`); mobile navigation goes in `MobileDrawer`, opened from the header hamburger through `MobileNavContext` (`app/contexts/MobileNav`).
- Breadcrumbs wrap (`flex-wrap`), shrink text on mobile (`text-xs sm:text-sm`), keep separators `shrink-0`, and truncate long middle crumbs (`truncate max-w-[120px] sm:max-w-none`).
- Cover images center and constrain on mobile: `w-48 sm:w-64 lg:w-full mx-auto lg:mx-0`.
- Label/value settings rows stack on mobile: `flex flex-col sm:flex-row sm:justify-between sm:items-center gap-1 sm:gap-4`, long values `font-mono break-all sm:break-normal`.

## Every page

- Every page sets a browser title with `usePageTitle` (`{Title} - Shisho`): plural noun for list pages, the entity's name for detail pages (pass `undefined` while loading), a descriptive name for settings pages. A static title is set before any early return.
- Tabbed views are deep-linked: `/:tab?` in `app/router.tsx`, `useParams()` validated against the allowed values with the first tab as default (and a clean URL for it), controlled `value`, and `navigate()` in `onValueChange`. Tabs that are distinct routes (`AdminPlugins`) derive the active tab from `useLocation()`.
- Tab lists scroll on narrow screens: `TabsList className="w-full justify-start overflow-x-auto"`, triggers `text-xs sm:text-sm`.

## Long text and overflow

- `DialogContent` with user content gets `overflow-x-hidden`; inner containers avoid `overflow-hidden`, which clips focus rings.
- `DialogHeader` gets `pr-8` to clear the close button; dialog titles wrap rather than truncate.
- `truncate` inside flex works only when the parent has `min-w-0`. Truncated text carries `title={text}`.
- Badges with long text: `max-w-full` on the badge, `truncate` on the text span, `shrink-0` on any remove button. Command and menu items: icon `shrink-0`, text `truncate`.
- **Rows of stats and actions wrap**: `flex flex-wrap items-center gap-x-2 gap-y-1 min-w-0`. The shared `LibraryLayout` `<main>` has no `overflow-x-hidden`, so a non-wrapping row turns into page-level horizontal scroll on a phone. Prefer per-element fixes (`break-words`, `break-all`, `min-w-0`, grid `minmax(0,1fr)` tracks) over a blanket `overflow-x-hidden`, which masks bugs and clips popovers.
- Inline stats are separated by a faded middle dot: `<span className="text-muted-foreground/50">·</span>`.
- A dropdown inside an `overflow-hidden` container (a collapsible section) switches to fixed positioning on mobile (`fixed left-4 right-4 top-28`) instead of `absolute`.
- Full-width mobile inputs drop the focus glow: `focus-visible:ring-0 focus-visible:border-border`.

## Controls

- **No raw `<button>` outside `app/components/ui`.** Use `Button` and pick the closest fit: a standard variant; `size="icon-sm"` (28px) or `"icon-xs"` (20px) for small icon buttons; `variant="link"` for an inline text link; `BadgeRemoveButton` for the X inside a `Badge`. `variant="unstyled"` (cursor, focus ring, and disabled styles only) is the last resort, for rows, cards, and tap zones the caller lays out entirely. Overriding a variant's height, padding, or layout with `className` means a different variant fits better. Every clickable element shows `cursor-pointer`; a UI primitive that is clickable carries it in its base classes with `disabled:cursor-not-allowed`.
- A region waiting for its content (a page, section, dialog body, popover list, reader overlay) shows `<LoadingSpinner />` from `@/components/library/LoadingSpinner`, not "Loading..." text, a skeleton, or a bare `Loader2`. It carries `role="status"`, so tests find it with `getByRole("status")`.
- A pending action inside a control (a Save button, a row being added, a search input, a refetch indicator over results already shown) uses `Loader2`. `LoadingSpinner`'s `className` can tighten its default `py-8` (`py-3` in a popover menu). Combobox dropdowns keep their inline "Loading..." row.
- A date-only value (no time of day) says "today" or "yesterday" for the last two days and uses `addSuffix` past that (`PluginVersionCard`). Durations use `formatElapsed`, byte counts `formatFileSize`, page counts `formatPageCount`; other counts pluralize with a `count === 1` check.
- File labels render `fileLabel(file)`, never a label rebuilt from `file.name` or the path.

## Forms

- Every form that creates or updates data with a Save step has unsaved changes protection (`docs/agents/frontend/forms.md`): create and edit dialogs, settings pages, child editors with their own Save, and tab switches away from inline editing. Not needed: action dialogs that run immediately (merge, move, delete confirmations), quick actions with no pending state (add to list popover), view-only pages, settings that apply on change (theme).
- Metadata fields are first-class: no helper text implying a field is secondary or derived ("Leave empty to use the title from file metadata").
- Clearing a metadata field saves the cleared value; it does not revert to a default. The next scan repopulates from the file if needed.

## Query cache

- A mutation invalidates every query that displays what it changed. Editing, merging, or deleting a genre, tag, series, person, or publisher also invalidates `ListBooks` and `RetrieveBook`, since books show that metadata.
- Reparenting a publisher (`parent_id` edits, set child) invalidates the whole `RetrievePublisher` and `PublisherFiles` families: descendant-inclusive data changes for both sides of the move and every cached ancestor.
- Cover and page image rules: `docs/agents/frontend/image-urls.md`.
