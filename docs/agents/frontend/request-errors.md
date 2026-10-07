# Request errors, retries, and mutation handling

Read this before writing code that reports a failed request, renders a failed query, calls `mutate`/`mutateAsync`, or fetches outside `API.request`. The one-line rules live in `app/AGENTS.md`; this is the detail behind them.

## What `checkStatus` returns

`ShishoAPI.checkStatus` (`app/libraries/api.ts`) always rejects with a `ShishoAPIError`, never a JSON `SyntaxError`. It reads the body as text and then tries `JSON.parse`, because a non-JSON body can only come from a reverse proxy in front of the server.

- A 2xx with an empty or whitespace-only body (including `204`) resolves to `undefined`.
- A 2xx with a non-JSON body rejects: every endpoint returns JSON or nothing.
- A non-2xx without the Go `{ error: { code, message } }` body rejects with a status message such as `Request failed with status 504 (Gateway Timeout)`, and `ShishoAPIError.code` is `undefined`. Code that reads `code` handles `undefined`.
- `app/libraries/api.test.ts` pins the exact messages.

A direct `fetch` to a JSON endpoint (a `FormData` upload, which `API.request` would JSON-encode) passes the response to `API.checkStatus` rather than calling `response.json()`. `useUploadFileCover` is the example.

## Retries

The shared QueryClient does not retry a `ShishoAPIError` with status `401`, `403`, `404`, or `422`; other query failures keep the three-retry limit. Permission failures, Demo Mode rejections included, stay out of the retry path.

## Toasts and inline messages

`toastRequestError(error, fallback)` shows `requestErrorMessage(error, fallback)`: the server's message from the Go error body, otherwise `fallback`, which names the action (`"Failed to delete book"`). These show the fallback, because their text does not say which action failed:

- a `ShishoAPIError` without a Shisho body (`code` is `undefined`),
- the server's generic `internal_server_error`,
- any other rejection (a network `TypeError`, an abort, a client-side `Error`).

Inline error UI calls `requestErrorMessage` directly. Every failure goes through these two functions so it reads the same everywhere; hand-built wording (`error.message || ...`, a `"Failed to X: ${error.message}"` prefix) drifts. Plain `toast.error` is only for client-side validation that never reached the server. Both functions stay silent on Demo Mode rejections; see `docs/agents/frontend/demo-mode.md`.

## Failed queries: `QueryError`

`<QueryError query={q} fallback="Failed to load plugins" />` (`@/components/library/QueryError`) renders the inset destructive box with `role="alert"`, the message, and a Retry button disabled while `q.isFetching`. A disabled query gets no Retry, since `refetch()` would bypass the hook's permission gate. Pass the whole query result.

- Render it when `q.error && !q.data`, so a failed background refetch keeps loaded content on screen.
- A query with neither data nor an error is waiting on its `enabled` gate: show the spinner or nothing, never `QueryError`.
- For several queries, `firstFailedQuery(...queries)` from `@/libraries/api` picks the one to report.
- A list that tracks a "confirmed" search or filter behind its shown results (`confirmedSearch` in `ResourceList`, `confirmedFilterKey` in `Home`) confirms it when the query settles with an error too, or a failed search spins forever.
- A failed query never shows an empty state, nothing, `error.message`, or a full-page "Error Loading X" heading.

One shape per page type:

- **List pages** keep the header, search, and filters and render `QueryError` where the results go (`Gallery` and `ResourceList` do this for you).
- **Detail pages** keep their Not Found page for a 404 and render `QueryError` inside the layout otherwise. Branch with `isLoadFailure(q)` (a non-404 error with no data, built on `isNotFoundError`). `ResourceDetail` takes the entity query as `query` and makes this choice itself.
- **Dialogs and popovers** render `QueryError` in the body where the content goes. A dialog that saves over what failed to load (`AddToListDialog`, whose Save replaces the book's lists) hides the selection and disables Save.
- **Settings pages and tabs** keep the page header and put `QueryError` inline below it.

Exceptions with their own error UI: `FetchChaptersDialog` (maps Audible error codes), `SharedBook` (recipient-facing unavailable and Try again pages), `IdentifyBookDialog` (per-plugin errors inside a successful response), and `EPUBReader` (shows foliate's client-side `loadError`).

## Mutations

ESLint rejects a `mutate()` without `onError`, and the local rule `eslint-rules/mutate-async-handled.js` rejects an unhandled `mutateAsync` promise. The rule's header lists the shapes it accepts, and `app/eslint-rules.test.ts` pins both rules and their known bypasses and false positives. What the rules cannot check:

- **A returned promise makes the caller responsible.** The metadata detail pages return their merge, set-child, delete, and edit promises from named handlers passed to callback props (`onSave`, `onMerge`, `onDelete`); `MetadataMergeDialog`, `MetadataDeleteDialog`, `MetadataEditDialog`, and `PublisherEditDialog` catch them, report the failure, and stay open. The rule cannot see whether the caller really catches.
- When the call itself is returned, a follow-up that waits for success (closing a dialog, navigating) goes in the call's `onSuccess`; a caller that awaits the returned promise can follow up after it instead (`ResourceDetail` closes its dialogs this way).
- An event handler never catches, so returning from a function bound to `onClick`/`onSubmit`/etc. does not count as handing off.
- Run mutations concurrently with `await Promise.all([a.mutateAsync(x), b.mutateAsync(y)])` inside the `try`; a promise stored and awaited later is flagged.
- A dialog that treats a resolved `onCreate`/`onUpdate` promise as success (`CreateListDialog`) needs its parent callbacks to rethrow after toasting, so the dialog stays open with its unsaved-changes protection. Test these flows through the callers: a caller that swallows the rejection is the bug.
- A failed save keeps the user's draft and reports inline or by toast (`BookEditDialog` shows save errors inline; the top-nav `ResyncButton` toasts).

## Testing failures

- `rejectingMutate()` from `app/testing/mutations.ts` calls the caller's `onError` with a `ShishoAPIError`; assert the toast.
- `watchUnhandledRejections()` from the same file proves a click handler left no unhandled rejection. Reject from a plain async function rather than a `vi.fn()` mock, whose spy handles the promise and hides the leak.
