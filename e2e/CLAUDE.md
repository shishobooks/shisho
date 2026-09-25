# E2E Testing

This file documents E2E testing patterns and conventions for Shisho.

## Stack

- **Playwright** for browser automation
- **Multiple browsers** (Chromium, Firefox) with per-browser database isolation
- **Test-only API endpoints** enabled via `ENVIRONMENT=test`

## Architecture: Per-Browser Isolation

Each browser project runs against its own isolated environment:

```
Browser A (chromium)          Browser B (firefox)
    ↓                              ↓
API Server (port A)           API Server (port B)
    ↓                              ↓
SQLite DB A                   SQLite DB B
```

**Execution model:**
- Tests within a browser run sequentially (`workers: 1`) to avoid database race conditions
- Different browsers run in parallel via mise (`mise test:e2e` runs chromium and firefox simultaneously)

### Adding New Browsers

Add to the `BROWSERS` array in `playwright.config.ts`:

```typescript
const BROWSERS = ["chromium", "firefox", "webkit"] as const;
```

The config automatically allocates ports, creates isolated databases, and starts servers.

## Core Principle: Test Independence

**Each test file must set up its own preconditions.** Never rely on test ordering or state from other test files.

```typescript
import { expect, getApiBaseURL, request, test } from "./fixtures";

test.describe("Login Flow", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    await apiContext.delete("/api/test/users");
    await apiContext.post("/api/test/users", {
      data: { username: "testadmin", password: "password123" },
    });
    await apiContext.dispose();
  });

  test("logs in successfully", async ({ page }) => {
    // Test can run independently
  });
});
```

## Test Fixtures

Import from `./fixtures` instead of `@playwright/test`:

```typescript
import { expect, getApiBaseURL, request, test } from "./fixtures";
```

### Available Exports

| Export | Purpose |
|--------|---------|
| `test` | Extended Playwright test with `apiContext` fixture |
| `expect` | Standard Playwright expect |
| `request` | Playwright request for creating API contexts |
| `getApiBaseURL(browserName)` | Get API URL for a browser (use in `beforeAll`) |

### Using `apiContext` Fixture

For individual tests, use the `apiContext` fixture directly:

```typescript
test("creates item via API", async ({ page, apiContext }) => {
  await apiContext.post("/api/items", { data: { name: "Test" } });
  // ...
});
```

For `beforeAll` hooks, use `getApiBaseURL` with `request.newContext()` since fixtures aren't available there.

`getApiBaseURL` and the API fixtures target the backend origin directly. Every API request must include `/api` explicitly, including setup and cleanup requests such as `/api/test/users`. Adding `/api` to `baseURL` does not fix leading-slash request paths: URL resolution replaces the base path. Device routes stay at `/ereader`, `/opds`, `/kobo`, and `/e`; browser navigation to SPA routes such as `/login` and `/libraries` also stays at the root.

## Test-Only API Endpoints

Test endpoints are only registered when `ENVIRONMENT=test`.

### Available Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/test/users` | POST | Create a test user with admin role |
| `/api/test/users` | DELETE | Delete all users |
| `/api/test/ereader` | DELETE | Wipe all eReader test data |
| `/api/test/plugins` | POST | Seed a plugin (disk + DB) |
| `/api/test/plugins` | DELETE | Wipe all plugin state (add `?include_official=true` to also wipe official repos) |
| `/api/test/plugins/fixture.zip` | GET | Fixture plugin zipped for install flows |
| `/api/test/plugins/fixture-info` | GET | `{scope, id, version, download_url, sha256}` for the fixture |
| `/api/test/libraries`, `/api/test/books`, `/api/test/persons`, `/api/test/series` | POST | Seed library data directly in the database |

Seeded books and series are added to their FTS tables so search-driven UI (global search, the merge combobox) can find them. Seeded persons are not indexed. A test that searches for a seeded row must wait for the search response before acting on results: the merge combobox debounces input and replaces its list when results arrive, so a click that races the debounce can land on an item that is about to be removed (see the series merge test in `alias.spec.ts`).

The fixture plugin (`pkg/testutils/plugin_fixture.go`) is an EPUB metadata enricher that always proposes one result, `Fixture Title` with `abridged: false`, so a seeded plugin plus a seeded EPUB book is enough to drive the Identify dialog end to end (see `identify.spec.ts`). The Identify dialog searches on open; results are buttons named by their title, and the book page's actions menu trigger is labeled `Book actions`. Seed a fresh book per test: once a proposal is applied, its rows are unchanged and hidden by the Changed filter.

### Backend Pattern

```go
// pkg/config/config.go
func (c *Config) IsTestMode() bool {
    return c.Environment == "test"
}

// pkg/server/server.go
if cfg.IsTestMode() && !cfg.DemoMode {
    testutils.RegisterRoutes(e.Group("/api"), db, pm, plugins.NewInstaller(cfg.PluginDir))
}
```

## Playwright Configuration

The config (`playwright.config.ts`):

1. Defines `BROWSERS` array - add browsers here
2. Allocates unique ports for each browser's API and frontend
3. Creates isolated temp directories with separate SQLite databases
4. Stores browser configs in `E2E_BROWSER_CONFIGS` env var
5. Uses `workers: 1` to run tests sequentially within each browser
6. Detects `--project` flag to start only needed servers (2 instead of 4)
7. Uses `reuseExistingServer: true` for concurrent browser runs

## Common Pitfalls

### 1. Using Wrong Import

**Problem:** Importing from `@playwright/test` instead of `./fixtures`.

**Solution:** Always import from `./fixtures`:

```typescript
import { expect, getApiBaseURL, request, test } from "./fixtures";
```

### 2. Toast Assertions on Navigation

**Problem:** Toast disappears when navigating to a different route.

**Solution:** Assert on stable UI elements instead:

```typescript
// ❌ BAD: Toast may disappear
await expect(page.getByText("Account created!")).toBeVisible();

// ✅ GOOD: Assert on stable state
await expect(page).toHaveURL("/settings/libraries");
```

### 3. Wrong Redirect Expectations

**Problem:** App redirects to different pages based on state.

**Solution:** Understand app routing logic and test actual behavior.

### 4. SPA Navigation Under Load

**Problem:** `page.goto()` defaults to waiting for the full `"load"` event, which can be slower than the UI becoming usable when the dev server is busy during `mise check:quiet`.

**Solution:** For SPA routes, prefer `waitUntil: "domcontentloaded"` and then assert on the stable UI the test actually needs:

```typescript
await page.goto("/setup", { waitUntil: "domcontentloaded" });
await expect(page.getByRole("heading", { name: "Welcome!" })).toBeVisible();
```

### 5. `getByRole(role, { name })` Matches Names as Substrings

**Problem:** `getByRole(role, { name })` performs a case-insensitive substring match by default. If two elements with the same role have names where one is a prefix/substring of the other (e.g., a "Select" toolbar button and a "Select Library" dropdown trigger on the same page), the locator resolves to multiple elements and Playwright's strict mode fails it with `strict mode violation`.

**Solution:** Pass `exact: true` whenever the visible name is — or could plausibly become — a substring of another element's name on the same page:

```typescript
// ❌ BAD: also matches "Select Library", "Select All", etc.
await expect(page.getByRole("button", { name: "Select" })).toBeVisible();

// ✅ GOOD: matches only the exact "Select" button
await expect(
  page.getByRole("button", { name: "Select", exact: true }),
).toBeVisible();
```

For more flexibility (e.g., "starts with X" or a specific casing), pass a regex instead: `name: /^Select /` to allow any "Select …" name, or `name: /select/i` for explicit case-insensitive matching.

This is especially insidious because the test passes until someone adds an unrelated button whose name happens to contain the same substring — the failure shows up in CI on a PR that didn't touch the test.

## Test File Structure

```typescript
import { expect, getApiBaseURL, request, test } from "./fixtures";

test.describe("Feature Name", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const apiContext = await request.newContext({ baseURL: apiBaseURL });
    // Set up state
    await apiContext.dispose();
  });

  test("does something", async ({ page }) => {
    // ...
  });
});
```

## Running Tests

```bash
mise test:e2e                              # Run all browsers in parallel (~10-12s)
mise e2e:chromium                          # Run only Chromium
mise e2e:firefox                           # Run only Firefox
playwright test --project=chromium         # Alternative: direct Playwright call
playwright test e2e/login.spec.ts          # Run specific test file
```

Playwright auto-starts servers via `webServer` config. When using `--project`, only that browser's servers start.

### webServer API command: inline `go build … && exec …`, never a wrapper

The API webServer command (`playwright.config.ts`) inlines `go build -o ./build/api/api-e2e-<browser> ./cmd/api && exec ./build/api/api-e2e-<browser>`. Do NOT route it through `mise start:api` (= `go run ./cmd/api`) or a `mise` task wrapper.

Why: Playwright spawns the webServer with `detached: true` (its own process group) and, on teardown, force-kills it with `process.kill(-pid, "SIGKILL")` — a process-GROUP kill. For that to reap the API, the API must be in the process group led by the PID Playwright spawned. Two wrappers break that:

- `go run` spawns the api binary as a child and does not forward signals to it, so the binary is orphaned.
- `mise <task>` (a Go program) puts the task subprocess in a separate process group on Linux, so the binary escapes the group kill even if the task execs it. (This passes on macOS, where mise keeps the same group, which is why it must be tested against CI, not just locally.)

When the API is orphaned it keeps the webServer's stdout pipe open, so Playwright hangs after the tests finish until the CI step times out. Inlining `sh -c "go build … && exec …"` fixes it: `exec` replaces the shell with the api binary, so the binary IS the process-group leader Playwright tracks, and the group SIGKILL reaches it. The api binary's own graceful shutdown lives in `cmd/api/main.go`. Binaries are built per-browser to `./build/api/api-e2e-<browser>` (gitignored) to avoid a concurrent-build race when chromium and firefox build in parallel under `mise test:e2e`.

## Key Files

| Purpose | Location |
|---------|----------|
| Playwright config | `playwright.config.ts` |
| Test fixtures | `e2e/fixtures.ts` |
| E2E tests | `e2e/*.spec.ts` |
| Test-only routes | `pkg/testutils/routes.go` |
| Test-only handlers | `pkg/testutils/handlers.go` |
