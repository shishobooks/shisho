# Shisho Frontend Development

React 19, TypeScript, Tailwind, TanStack Query, Vite, and Radix UI with shadcn/ui patterns. Server state lives in TanStack Query; there is no global client state library. HTTP client functions live in `app/libraries/api.ts`; query and mutation hooks wrap them in `app/hooks/queries/`. Reviewers also read `docs/agents/standards/frontend.md`.

## Enforced by checks

ESLint (`eslint.config.js`, local rules in `eslint-rules/`) and `app/hooks/queries/permissions.test.tsx` enforce the mechanical frontend rules; their messages say how to comply. Among them: no raw `<button>` outside `app/components/ui`, so every control gets `Button`'s focus ring, cursor, and disabled state.

The rules have known bypasses, pinned in `app/eslint-rules.test.ts`: a passing lint is a tripwire, not a guarantee. Tab state in `useState` passes lint but is still wrong; tabs deep-link through the URL ("Every page" in `docs/agents/standards/frontend.md`).

## API types

Request and response shapes are generated from Go by tygo into `app/types/generated/` (re-exported from `@/types`). Never hand-define a type with a Go counterpart: fix the Go struct and run `mise tygo` (ADR 0004).

- A query hook types its return as the generated `{Entity}Response` or `List{Entities}Response`, not the bare model or `ResourceListResponse`, and reads a reshaped relation as typed, with no `as unknown as` cast. Older hooks are not the model; `app/hooks/queries/publishers.ts` is.

## Request errors

- Report a failed request with `toastRequestError(error, fallback)` from `@/libraries/api`, where `fallback` names the action (`"Failed to delete book"`); inline error text uses `requestErrorMessage(error, fallback)`. Never hand-build wording from `error.message`. Plain `toast.error` is only for client-side validation that never reached the server.
- Render a failed query with `<QueryError query={q} fallback="Failed to load X" />` only when `q.error && !q.data` (`firstFailedQuery` picks among several), so a failed background refetch keeps loaded content. A query with neither data nor error is waiting on its permission gate: show the spinner or nothing. A failed query never shows an empty state, nothing, or a full-page error heading.
- Where it goes: list pages keep header, search, and filters and put it where results go (`Gallery` and `ResourceList` already do); detail pages keep their Not Found page for a 404 and branch on `isLoadFailure(q)` otherwise; dialogs and popovers render it in the body, and one that would save over what failed to load hides the selection and disables Save; settings pages put it below the header.
- A direct `fetch` (a `FormData` upload) passes its response to `API.checkStatus`, never `response.json()`. `ShishoAPIError.code` is `undefined` when a proxy, not the server, answered.
- A failed save keeps the dialog open and the draft intact.

Mutation hand-offs the lint rules cannot verify:

- A handler that returns its mutation promise to a callback prop (`onSave`, `onMerge`) makes the caller responsible for catching it, reporting, and staying open. An event handler (`onClick`, `onSubmit`) never catches, so returning to one is not a hand-off.
- A dialog that treats a resolved `onCreate`/`onUpdate` promise as success needs its parent to rethrow after toasting. Test such flows through the caller: a caller that swallows the rejection is the bug.
- Run mutations concurrently with `await Promise.all([...mutateAsync(...)])` inside the `try`; a promise stored and awaited later is flagged.

## Permissions

Both layers take a typed `Requirement` (one permission, an array for all, or `anyOf(...)`):

- **Query hooks gate themselves** with `useRequires` (`app/hooks/queries/permissions.ts`). Callers pass `enabled` only for their own state (`enabled: open` for a dialog), never a restated permission. `refetch()` runs even a disabled query, so check the result's `isEnabled` first.
- **Components hide controls** with `useCan` (or `can` from `useAuth()` where a hook cannot run). Gate on the permission the backend route requires (find it in the handler package's `routes.go`), not on the page type.

Hide a control the role cannot use and skip mounting the dialogs behind it; do not disable it. The exception is a settings form a role can read but not save: disabled inputs, `<ReadOnlyNotice />`, no Save button. Action menus gate each entry on its own route's permission, not the whole menu.

- Never gated on a write permission: list membership (it follows the list's own `permission` field), selection mode, and downloads.
- A new route adds its requirement to `ROUTE_PERMISSIONS` (`@/utils/permissions`), which both the route guard and the nav hooks read.
- A link to another page (a series or person name) renders as plain text when the target route would deny the role. A page part backed by a route the role lacks is hidden or replaced by a notice.
- Reader pages read libraries through `useUserLibraries` and its siblings (`GET /user/libraries`), never the Libraries Read hooks `useLibrary`/`useLibraries`. User pickers outside admin pages use `useUserDirectory`.
- `X.permissions.test.tsx` renders the real query hooks with a spied `API.request` and asserts which requests fire for a role; assert that no request fires, not which `enabled` a caller passed.

## Demo Mode

Branch on `demoMode` from `useAuth()`. `checkStatus` already toasts a Demo Mode rejection, and `toastRequestError` stays silent on it; new inline error UI skips it with `isDemoModeError(error)`. Test that through the real `API` with a `demo_mode` 403 and a real `<Toaster />`: a mocked rejection never reaches `checkStatus`. Demo Mode hides only a few controls (downloads, admin and Security entries); everything else the role may use stays visible and relies on the backend rejection. `useCan` and `can` know nothing about Demo Mode. Backend side: `docs/agents/backend/demo-mode.md`.

## Forms

Every form that creates or updates data has unsaved changes protection. Keep an `initialValues` snapshot, compute `hasChanges` against it (`fast-deep-equal` for arrays and objects), and reset the snapshot to the saved values after a successful save.

- Dialogs use `FormDialog` (`@/components/ui/form-dialog`) and close after a save with `requestClose` from `useFormDialogClose`; calling `onOpenChange(false)` right after the reset prompts on the stale `hasChanges`. Initialize fields in an effect keyed on `open`. A confirmation launched from a form dialog renders beside the `FormDialog`, not inside it.
- Pages use `useUnsavedChanges` with `UnsavedChangesDialog`, and initialize once per entity behind an `isInitialized` flag reset when the id changes, so a background refetch cannot overwrite edits.
- A child editor with its own Save reports through `onHasChangesChange`, and the parent ORs it into its `hasChanges`. Tabs with inline editing intercept `onValueChange`: while `hasChanges`, hold the target tab and show `UnsavedChangesDialog`.

## Image URLs

Cover and page endpoints are cached `immutable`, and browsers also keep an in-memory image cache that reuses the bitmap for any `src` already rendered this session, so a changed image needs a changed URL. Every URL carries a `?v=` key built by the helpers in `app/utils` (`coverUrl.ts`, `pageUrl.ts`, `downloadUrl.ts`), which document their keys.

- Components take the model (book, series, file) and call the helper, never a bare id or key. Page URLs come from `useFilePageUrl()`, which adds the PDF render key.
- Render server covers with `CoverImage`, keyed `key={coverUrl}` where the cover can change while mounted; use a plain `<img>` for local previews, provider images, and CBZ/PDF pages. Tests rendering covers call `mockCoverDimensions()` (jsdom has no layout).
- A cover-changing mutation invalidates the query whose data drives the key.

## Testing

- Unit and component tests are colocated as `*.test.ts(x)` and run with Vitest and Testing Library (`mise test:unit`). E2E patterns live in `e2e/AGENTS.md`. Test-only helpers go in `app/testing/`, which the production build excludes.
- Anything that calls `useAuth()` throws outside `AuthProvider`. Never hand-write a `useAuth` mock: use `app/testing/auth.ts` (its header shows the `vi.mock` and `setAuth` lines).
- `vitest.setup.ts` turns on fake timers globally, so `userEvent.setup()` needs `{ advanceTimers: vi.advanceTimersByTime }` or clicks and typing stall under load.
- `testTimeout` is 15s because `mise check:quiet` runs the unit suite beside everything else. Keep it; make a slow test cheaper (render one card, not a page) rather than raising a per-test timeout.
- Mutation failure helpers are in `app/testing/mutations.ts`.
- jsdom loads no Tailwind, so a label hidden on phones (`hidden sm:inline`) still names its button in tests. Assert the phone-width accessible name under `emulatePhoneWidth()` from `app/testing/phoneWidth.ts`.

## Gotchas

- **Queries never go stale on their own** (the global `staleTime` in `app/libraries/query-client.ts`), so a new query shows old data until something invalidates it. Invalidate it from every mutation and every SSE event (`app/hooks/useSSE.ts`) that changes its data.
- **A custom component used as a Radix `asChild` trigger must `forwardRef` and spread props** onto the underlying `Button`, with `displayName` set. Without the ref, floating content anchors at `(0, 0)` off-screen and `Dialog`/`Sheet` cannot restore focus. jsdom runs no positioning, so unit tests pass; only a real browser shows it.
- **Sortable lists need stable client-side row keys** assigned when a row enters the list. Index and content keys change on reorder and break dnd-kit drag tracking; server ids work only if unsaved rows have one too.
- **CBZ and PDF page numbers are 0-indexed in storage and the API, 1-indexed on screen.** Convert at the display edge, never in what you send back.
- **A dialog opened from a dropdown item**: give the `DropdownMenuContent` `onCloseAutoFocus={(e) => e.preventDefault()}` so the two focus managers do not fight.
- **File labels come from the server**: render `fileLabel(file)` from `@/utils/format`, never a label rebuilt from the name or path.
- **`Button` defaults `type="button"`**, so a submit button says `type="submit"`. Before overriding its classes, use another of its sizes or variants (`app/components/ui/button.tsx`) or `BadgeRemoveButton`.
- **Full-height surfaces offset with `--demo-banner-height`**, never a fixed banner height; modals cover the banner instead. UI shared by library and admin pages (sidebar chrome, top-nav geometry, `UserMenu`) lives in `app/components/layout/` so the two cannot drift.
- **M4B audio can hang silently.** Outside WebKit, xHE-AAC files pass `canPlayType` and then hang on any seek with no error, so a test file that plays fine never shows the bug. New code driving an `<audio>` element reuses `app/utils/audioCodec.ts` and follows its header comment (no `play()` before `canplay`, a timeout on every wait).
- New shadcn components are added with `npx shadcn@latest add`.
