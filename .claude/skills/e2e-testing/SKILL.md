---
name: e2e-testing
description: You MUST use this before writing or debugging E2E tests. Covers test independence, test-only API endpoints, Playwright configuration, and common pitfalls like shared database race conditions and toast assertions.
user-invocable: false
---

# E2E Testing

This skill documents E2E testing patterns and conventions for Shisho.

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
- Different browsers run in parallel via `concurrently` (`yarn test:e2e` runs chromium and firefox simultaneously)

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
    await apiContext.delete("/test/users");
    await apiContext.post("/test/users", {
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
  await apiContext.post("/items", { data: { name: "Test" } });
  // ...
});
```

For `beforeAll` hooks, use `getApiBaseURL` with `request.newContext()` since fixtures aren't available there.

## Test-Only API Endpoints

Test endpoints are only registered when `ENVIRONMENT=test`.

### Available Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/test/users` | POST | Create a test user with admin role |
| `/test/users` | DELETE | Delete all users |

### Backend Pattern

```go
// pkg/config/config.go
func (c *Config) IsTestMode() bool {
    return c.Environment == "test"
}

// pkg/server/server.go
if cfg.IsTestMode() {
    testutils.RegisterRoutes(e, db)
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
yarn test:e2e                              # Run all browsers in parallel (~10-12s)
yarn test:e2e:chromium                     # Run only Chromium
yarn test:e2e:firefox                      # Run only Firefox
playwright test --project=chromium         # Alternative: direct Playwright call
playwright test e2e/login.spec.ts          # Run specific test file
```

Playwright auto-starts servers via `webServer` config. When using `--project`, only that browser's servers start.

## Key Files

| Purpose | Location |
|---------|----------|
| Playwright config | `playwright.config.ts` |
| Test fixtures | `e2e/fixtures.ts` |
| E2E tests | `e2e/*.spec.ts` |
| Test-only routes | `pkg/testutils/routes.go` |
| Test-only handlers | `pkg/testutils/handlers.go` |
