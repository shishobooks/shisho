# E2E Testing

Each browser project runs against its own API server and SQLite database, and tests within a browser run serially (`workers: 1`). The comments in `playwright.config.ts` explain the setup, including why the API webServer command must stay an inline `go build && exec`.

## Writing Tests

- **Each test file sets up its own preconditions** through the test-only API, typically in `beforeAll`. Never rely on test ordering or state left by another file.
- **Import `test`, `expect`, and `request` from `./fixtures`**, not `@playwright/test` (type-only imports are fine). The fixtures point each browser at its own API server; `e2e/fixtures.ts` documents which helper to use where.
- **API request paths include `/api` explicitly**, including `/api/test/*`. The API helpers target the backend origin, and adding `/api` to `baseURL` does not help because a leading-slash path replaces the base path. Device routes and SPA navigation stay at the root.
- **Contexts made with `browser.newContext()` inherit neither the config's `baseURL` nor cookies.** That is how to get a recipient with no session; pass `baseURL` when such a context navigates by relative path.

## Test-Only API

The routes in `pkg/testutils/routes.go` exist only when the server runs with `SHISHO_TEST_MODE=true`, which `playwright.config.ts` sets. Read the handlers for what each seed accepts. Traps:

- Seeded books have no file on disk, so their downloads fail. `withEpubOnDisk` writes a real EPUB for one (see `share-link.spec.ts`).
- Seeded books and series are indexed for search; seeded persons are not. A test that searches for a seeded row waits for the search response before clicking a result: debounced comboboxes replace their list when results arrive, so an early click can land on an item about to vanish (see the series merge test in `alias.spec.ts`).
- The plugin seed route accepts any scope and id, but only the id `fixture` loads; any other id gets an Active row with a `load_error` and no runtime.
- The fixture plugin (`pkg/testutils/plugin_fixture.go`) always proposes one result, so a seeded plugin plus a seeded EPUB book drives the Identify dialog end to end (see `identify.spec.ts`). Seed a fresh book per test: once a proposal is applied, its rows are unchanged and hidden by the Changed filter.

## Common Pitfalls

- **Toasts vanish on navigation.** After an action that navigates, assert on the destination (URL or stable UI), not the toast.
- **`page.goto()` waits for the full `load` event**, which can lag far behind a usable UI when the dev server is busy during `mise check:quiet`. For SPA routes, pass `waitUntil: "domcontentloaded"` and then assert on the UI the test needs.
- **`getByRole(role, { name })` matches names as case-insensitive substrings.** A name that is a substring of another element's name on the page (a "Select" button next to "Select Library") resolves to both and fails strict mode, often only after someone adds the second element in an unrelated PR. Pass `exact: true` whenever the name is or could become a substring of another, or use an anchored regex.
