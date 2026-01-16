# Frontend Testing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add frontend testing foundation with unit tests (Vitest), component tests (React Testing Library), and E2E tests (Playwright).

**Architecture:** Colocated unit/component tests (`*.test.ts(x)`) in `app/`, separate E2E tests in `e2e/`. Dynamic port handling allows multiple worktree instances to run simultaneously without port conflicts.

**Tech Stack:** Vitest + React Testing Library for unit/component tests, Playwright for E2E, V8 for coverage.

---

## Task 1: Install Test Dependencies

**Files:**
- Modify: `package.json:50-70` (devDependencies section)

**Step 1: Add test dependencies**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && yarn add -D vitest @vitest/coverage-v8 jsdom @testing-library/react @testing-library/dom @playwright/test
```

Expected: Dependencies added to package.json devDependencies

**Step 2: Install Playwright browsers**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && npx playwright install chromium firefox
```

Expected: Chromium and Firefox browsers downloaded

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add package.json yarn.lock && git commit -m "$(cat <<'EOF'
[Feature] Add frontend testing dependencies

Install Vitest, React Testing Library, and Playwright for
unit, component, and E2E testing.
EOF
)"
```

---

## Task 2: Create Vitest Configuration

**Files:**
- Create: `vitest.config.ts`
- Create: `vitest.setup.ts`

**Step 1: Create vitest.config.ts**

Create file `/Users/robinjoseph/.worktrees/shisho/frontend-testing/vitest.config.ts`:

```typescript
import path from "path";

import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./app"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    include: ["app/**/*.test.{ts,tsx}"],
    coverage: {
      enabled: true,
      provider: "v8",
      reporter: ["text", "lcov", "html"],
      reportsDirectory: "./coverage",
      include: ["app/**/*.{ts,tsx}"],
      exclude: ["app/**/*.test.{ts,tsx}", "app/types/generated/**"],
    },
  },
});
```

**Step 2: Create vitest.setup.ts**

Create file `/Users/robinjoseph/.worktrees/shisho/frontend-testing/vitest.setup.ts`:

```typescript
import "@testing-library/dom";
```

**Step 3: Verify config by running empty test suite**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && npx vitest run
```

Expected: "No test files found" (no error means config is valid)

**Step 4: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add vitest.config.ts vitest.setup.ts && git commit -m "$(cat <<'EOF'
[Feature] Add Vitest configuration

Configure Vitest with jsdom, React Testing Library setup,
path aliases matching vite.config.ts, and V8 coverage.
EOF
)"
```

---

## Task 3: Create Playwright Configuration

**Files:**
- Create: `playwright.config.ts`
- Create: `e2e/.gitkeep`

**Step 1: Create playwright.config.ts**

Create file `/Users/robinjoseph/.worktrees/shisho/frontend-testing/playwright.config.ts`:

```typescript
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: "http://localhost:5173",
    trace: "on-first-retry",
  },
  projects: [
    { name: "chromium", use: { browserName: "chromium" } },
    { name: "firefox", use: { browserName: "firefox" } },
  ],
});
```

**Step 2: Create e2e directory**

Run:
```bash
mkdir -p /Users/robinjoseph/.worktrees/shisho/frontend-testing/e2e && touch /Users/robinjoseph/.worktrees/shisho/frontend-testing/e2e/.gitkeep
```

**Step 3: Verify config**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && npx playwright test --list
```

Expected: "No tests found" (no error means config is valid)

**Step 4: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add playwright.config.ts e2e/.gitkeep && git commit -m "$(cat <<'EOF'
[Feature] Add Playwright configuration

Configure Playwright with Chromium and Firefox projects,
30s timeout, and e2e test directory.
EOF
)"
```

---

## Task 4: Add Test Scripts to package.json

**Files:**
- Modify: `package.json:10-18` (scripts section)

**Step 1: Add test scripts**

In `/Users/robinjoseph/.worktrees/shisho/frontend-testing/package.json`, replace the scripts section (lines 10-18):

```json
  "scripts": {
    "build": "tsc -b && vite build",
    "lint": "concurrently --kill-others-on-fail --group \"yarn:lint:*\"",
    "lint:eslint": "eslint --max-warnings 0 .",
    "lint:prettier": "prettier --check .",
    "lint:types": "tsc -b --noEmit",
    "preview": "vite preview",
    "start": "vite",
    "test": "concurrently --kill-others-on-fail --group \"yarn:test:*\"",
    "test:unit": "vitest run",
    "test:e2e": "playwright test"
  },
```

**Step 2: Verify scripts**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && yarn test:unit
```

Expected: "No test files found" (script works)

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add package.json && git commit -m "$(cat <<'EOF'
[Feature] Add test scripts to package.json

Add yarn test (runs all), yarn test:unit (Vitest),
and yarn test:e2e (Playwright) scripts.
EOF
)"
```

---

## Task 5: Add test:js to Makefile

**Files:**
- Modify: `Makefile:18-20` (check target)

**Step 1: Add test:js target and update check**

In `/Users/robinjoseph/.worktrees/shisho/frontend-testing/Makefile`:

After line 53 (after `lint\:js` target), add:

```makefile

.PHONY: test\:js
test\:js:
	yarn test
```

Update line 20 to include test:js in parallel:

```makefile
	$(MAKE) -j4 test test\:js lint lint\:js
```

**Step 2: Verify make check runs**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && make test:js
```

Expected: "No test files found" messages (but no errors)

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add Makefile && git commit -m "$(cat <<'EOF'
[Feature] Add test:js target to Makefile

Add make test:js target that runs yarn test.
Include in make check for parallel execution.
EOF
)"
```

---

## Task 6: Add .gitignore Entries

**Files:**
- Modify: `.gitignore`

**Step 1: Check current .gitignore**

Run:
```bash
cat /Users/robinjoseph/.worktrees/shisho/frontend-testing/.gitignore
```

**Step 2: Add coverage and playwright entries**

Add to `.gitignore` if not present:

```
# Test coverage
coverage/

# Playwright
playwright-report/
test-results/
```

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add .gitignore && git commit -m "$(cat <<'EOF'
[Chore] Add test output directories to .gitignore

Ignore coverage/, playwright-report/, and test-results/.
EOF
)"
```

---

## Task 7: Write Unit Tests for validateISBN10

**Files:**
- Create: `app/utils/identifiers.test.ts`

**Step 1: Write the failing test**

Create file `/Users/robinjoseph/.worktrees/shisho/frontend-testing/app/utils/identifiers.test.ts`:

```typescript
import { describe, expect, it } from "vitest";

import {
  validateASIN,
  validateIdentifier,
  validateISBN10,
  validateISBN13,
  validateUUID,
} from "./identifiers";

describe("validateISBN10", () => {
  it("validates correct ISBN-10", () => {
    // "The Great Gatsby" - valid ISBN-10
    expect(validateISBN10("0743273567")).toBe(true);
  });

  it("validates ISBN-10 with dashes", () => {
    expect(validateISBN10("0-7432-7356-7")).toBe(true);
  });

  it("validates ISBN-10 with spaces", () => {
    expect(validateISBN10("0 7432 7356 7")).toBe(true);
  });

  it("validates ISBN-10 with X check digit", () => {
    // "Introduction to Algorithms" - ends with X
    expect(validateISBN10("026203384X")).toBe(true);
  });

  it("validates lowercase x check digit", () => {
    expect(validateISBN10("026203384x")).toBe(true);
  });

  it("rejects invalid checksum", () => {
    expect(validateISBN10("0743273568")).toBe(false);
  });

  it("rejects wrong length", () => {
    expect(validateISBN10("074327356")).toBe(false);
    expect(validateISBN10("07432735677")).toBe(false);
  });

  it("rejects X in non-final position", () => {
    expect(validateISBN10("0X43273567")).toBe(false);
  });

  it("rejects non-numeric characters", () => {
    expect(validateISBN10("074327356A")).toBe(false);
  });
});
```

**Step 2: Run test to verify it passes**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && npx vitest run app/utils/identifiers.test.ts
```

Expected: All 9 tests PASS

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add app/utils/identifiers.test.ts && git commit -m "$(cat <<'EOF'
[Test] Add unit tests for validateISBN10

Test valid ISBN-10, dashes/spaces handling, X check digit,
invalid checksums, wrong lengths, and invalid characters.
EOF
)"
```

---

## Task 8: Add Unit Tests for validateISBN13

**Files:**
- Modify: `app/utils/identifiers.test.ts`

**Step 1: Add ISBN-13 tests**

Add to `/Users/robinjoseph/.worktrees/shisho/frontend-testing/app/utils/identifiers.test.ts` after the validateISBN10 describe block:

```typescript

describe("validateISBN13", () => {
  it("validates correct ISBN-13", () => {
    // "The Great Gatsby" - valid ISBN-13
    expect(validateISBN13("9780743273565")).toBe(true);
  });

  it("validates ISBN-13 with dashes", () => {
    expect(validateISBN13("978-0-7432-7356-5")).toBe(true);
  });

  it("validates ISBN-13 with spaces", () => {
    expect(validateISBN13("978 0 7432 7356 5")).toBe(true);
  });

  it("rejects invalid checksum", () => {
    expect(validateISBN13("9780743273566")).toBe(false);
  });

  it("rejects wrong length", () => {
    expect(validateISBN13("978074327356")).toBe(false);
    expect(validateISBN13("97807432735655")).toBe(false);
  });

  it("rejects non-numeric characters", () => {
    expect(validateISBN13("978074327356X")).toBe(false);
  });
});
```

**Step 2: Run tests**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && npx vitest run app/utils/identifiers.test.ts
```

Expected: All 15 tests PASS

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add app/utils/identifiers.test.ts && git commit -m "$(cat <<'EOF'
[Test] Add unit tests for validateISBN13

Test valid ISBN-13, dashes/spaces handling, invalid checksums,
wrong lengths, and non-numeric characters.
EOF
)"
```

---

## Task 9: Add Unit Tests for validateASIN and validateUUID

**Files:**
- Modify: `app/utils/identifiers.test.ts`

**Step 1: Add ASIN and UUID tests**

Add to `/Users/robinjoseph/.worktrees/shisho/frontend-testing/app/utils/identifiers.test.ts`:

```typescript

describe("validateASIN", () => {
  it("validates correct ASIN", () => {
    expect(validateASIN("B0CHVFQ31G")).toBe(true);
  });

  it("validates lowercase asin", () => {
    expect(validateASIN("b0chvfq31g")).toBe(true);
  });

  it("rejects ASIN not starting with B0", () => {
    expect(validateASIN("A0CHVFQ31G")).toBe(false);
    expect(validateASIN("B1CHVFQ31G")).toBe(false);
  });

  it("rejects wrong length", () => {
    expect(validateASIN("B0CHVFQ31")).toBe(false);
    expect(validateASIN("B0CHVFQ31GX")).toBe(false);
  });

  it("rejects invalid characters", () => {
    expect(validateASIN("B0CHVFQ31!")).toBe(false);
  });
});

describe("validateUUID", () => {
  it("validates correct UUID", () => {
    expect(validateUUID("550e8400-e29b-41d4-a716-446655440000")).toBe(true);
  });

  it("validates uppercase UUID", () => {
    expect(validateUUID("550E8400-E29B-41D4-A716-446655440000")).toBe(true);
  });

  it("validates UUID with urn:uuid: prefix", () => {
    expect(validateUUID("urn:uuid:550e8400-e29b-41d4-a716-446655440000")).toBe(
      true,
    );
  });

  it("rejects UUID without dashes", () => {
    expect(validateUUID("550e8400e29b41d4a716446655440000")).toBe(false);
  });

  it("rejects UUID with wrong segment lengths", () => {
    expect(validateUUID("550e840-e29b-41d4-a716-446655440000")).toBe(false);
  });

  it("rejects invalid characters", () => {
    expect(validateUUID("550e8400-e29b-41d4-a716-44665544000g")).toBe(false);
  });
});
```

**Step 2: Run tests**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && npx vitest run app/utils/identifiers.test.ts
```

Expected: All 26 tests PASS

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add app/utils/identifiers.test.ts && git commit -m "$(cat <<'EOF'
[Test] Add unit tests for validateASIN and validateUUID

Test ASIN format validation (B0 prefix, alphanumeric).
Test UUID format with/without urn:uuid: prefix.
EOF
)"
```

---

## Task 10: Add Unit Tests for validateIdentifier

**Files:**
- Modify: `app/utils/identifiers.test.ts`

**Step 1: Add validateIdentifier tests**

Add to `/Users/robinjoseph/.worktrees/shisho/frontend-testing/app/utils/identifiers.test.ts`:

```typescript

describe("validateIdentifier", () => {
  it("validates isbn_10 type", () => {
    expect(validateIdentifier("isbn_10", "0743273567")).toEqual({ valid: true });
    expect(validateIdentifier("isbn_10", "0743273568")).toEqual({
      valid: false,
      error: "Invalid ISBN-10 checksum",
    });
  });

  it("validates isbn_13 type", () => {
    expect(validateIdentifier("isbn_13", "9780743273565")).toEqual({
      valid: true,
    });
    expect(validateIdentifier("isbn_13", "9780743273566")).toEqual({
      valid: false,
      error: "Invalid ISBN-13 checksum",
    });
  });

  it("validates asin type", () => {
    expect(validateIdentifier("asin", "B0CHVFQ31G")).toEqual({ valid: true });
    expect(validateIdentifier("asin", "invalid")).toEqual({
      valid: false,
      error: "ASIN must be 10 alphanumeric characters starting with B0",
    });
  });

  it("validates uuid type", () => {
    expect(
      validateIdentifier("uuid", "550e8400-e29b-41d4-a716-446655440000"),
    ).toEqual({ valid: true });
    expect(validateIdentifier("uuid", "invalid")).toEqual({
      valid: false,
      error: "Invalid UUID format",
    });
  });

  it("returns valid for unknown types", () => {
    expect(validateIdentifier("unknown", "anything")).toEqual({ valid: true });
  });
});
```

**Step 2: Run tests**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && npx vitest run app/utils/identifiers.test.ts
```

Expected: All 31 tests PASS

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add app/utils/identifiers.test.ts && git commit -m "$(cat <<'EOF'
[Test] Add unit tests for validateIdentifier

Test dispatch function for all identifier types.
Verify error messages and unknown type handling.
EOF
)"
```

---

## Task 11: Write Component Test for CoverPlaceholder

**Files:**
- Create: `app/components/library/CoverPlaceholder.test.tsx`

**Step 1: Write the component test**

Create file `/Users/robinjoseph/.worktrees/shisho/frontend-testing/app/components/library/CoverPlaceholder.test.tsx`:

```typescript
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import CoverPlaceholder from "./CoverPlaceholder";

describe("CoverPlaceholder", () => {
  it("renders book variant with correct viewBox", () => {
    render(<CoverPlaceholder variant="book" />);

    const svgs = document.querySelectorAll("svg");
    expect(svgs).toHaveLength(2); // light and dark mode

    svgs.forEach((svg) => {
      expect(svg.getAttribute("viewBox")).toBe("0 0 200 300");
    });
  });

  it("renders audiobook variant with correct viewBox", () => {
    render(<CoverPlaceholder variant="audiobook" />);

    const svgs = document.querySelectorAll("svg");
    expect(svgs).toHaveLength(2);

    svgs.forEach((svg) => {
      expect(svg.getAttribute("viewBox")).toBe("0 0 300 300");
    });
  });

  it("applies custom className", () => {
    const { container } = render(
      <CoverPlaceholder className="custom-class" variant="book" />,
    );

    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper.classList.contains("custom-class")).toBe(true);
  });

  it("renders light mode SVG visible by default", () => {
    render(<CoverPlaceholder variant="book" />);

    const svgs = document.querySelectorAll("svg");
    // First SVG is light mode (no hidden class)
    expect(svgs[0].classList.contains("dark:hidden")).toBe(true);
    // Second SVG is dark mode (hidden by default)
    expect(svgs[1].classList.contains("hidden")).toBe(true);
  });
});
```

**Step 2: Run tests**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && npx vitest run app/components/library/CoverPlaceholder.test.tsx
```

Expected: All 4 tests PASS

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add app/components/library/CoverPlaceholder.test.tsx && git commit -m "$(cat <<'EOF'
[Test] Add component tests for CoverPlaceholder

Test viewBox for book vs audiobook variants,
custom className application, and dark/light mode SVGs.
EOF
)"
```

---

## Task 12: Modify Backend for Dynamic Port File

**Files:**
- Modify: `cmd/api/main.go:64-71`

**Step 1: Read current server code**

The server startup is at lines 64-71 in `cmd/api/main.go`. Currently uses `srv.ListenAndServe()` which doesn't expose the actual port when using port 0.

**Step 2: Modify server startup to use net.Listen**

Replace lines 64-71 in `/Users/robinjoseph/.worktrees/shisho/frontend-testing/cmd/api/main.go`:

```go
	go func() {
		addr := fmt.Sprintf(":%d", cfg.ServerPort)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.Err(err).Fatal("failed to bind port")
		}

		// Extract actual port (useful when ServerPort is 0)
		actualPort := listener.Addr().(*net.TCPAddr).Port
		log.Info("server started", logger.Data{"port": actualPort})

		// Write port file for Vite to read
		if err := writePortFile(actualPort); err != nil {
			log.Err(err).Error("failed to write port file")
		}

		err = srv.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Err(err).Fatal("server stopped")
		}
		log.Info("server stopped")
	}()
```

**Step 3: Add writePortFile function and imports**

Add imports at top of file (after line 8):

```go
	"fmt"
	"net"
```

Add function after `initDownloadCacheDir` (after line 115):

```go

// writePortFile writes the server's actual port to tmp/api.port for frontend dev server.
func writePortFile(port int) error {
	return os.WriteFile("tmp/api.port", []byte(fmt.Sprintf("%d", port)), 0644)
}
```

**Step 4: Verify Go code compiles**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && go build ./cmd/api
```

Expected: Compiles without errors

**Step 5: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add cmd/api/main.go && git commit -m "$(cat <<'EOF'
[Feature] Write API port to tmp/api.port on startup

Use net.Listen to bind, extract actual port (supports port 0),
and write to tmp/api.port for Vite proxy configuration.
EOF
)"
```

---

## Task 13: Modify Vite Config for Dynamic Port

**Files:**
- Modify: `vite.config.ts`

**Step 1: Update vite.config.ts with dynamic port reading**

Replace entire `/Users/robinjoseph/.worktrees/shisho/frontend-testing/vite.config.ts`:

```typescript
import fs from "fs";
import path from "path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";

// Read API port from: API_PORT env var → tmp/api.port file → default 3689
function getApiPort(): number {
  if (process.env.API_PORT) {
    return parseInt(process.env.API_PORT, 10);
  }

  const portFile = path.resolve(__dirname, "tmp/api.port");
  if (fs.existsSync(portFile)) {
    const port = parseInt(fs.readFileSync(portFile, "utf-8").trim(), 10);
    if (!isNaN(port)) {
      return port;
    }
  }

  return 3689;
}

// https://vite.dev/config/
export default defineConfig({
  server: {
    host: "0.0.0.0",
    strictPort: false, // Allow auto-increment if port is taken
    proxy: {
      "/api": {
        target: `http://localhost:${getApiPort()}`,
        rewrite: (path) => path.replace(/^\/api/, ""),
        headers: {
          "X-Forwarded-Prefix": "/api",
        },
      },
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./app"),
    },
  },
  build: {
    outDir: "./build/app",
    emptyOutDir: true, // also necessary
  },
  clearScreen: false,
  plugins: [react(), tailwindcss()],
});
```

**Step 2: Verify Vite starts**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && timeout 5 yarn start || true
```

Expected: Vite starts without errors (times out after 5s which is fine)

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add vite.config.ts && git commit -m "$(cat <<'EOF'
[Feature] Add dynamic API port detection to Vite config

Read API port from: API_PORT env → tmp/api.port → default 3689.
Use strictPort: false to allow port auto-increment.
EOF
)"
```

---

## Task 14: Add tmp/api.port to .gitignore

**Files:**
- Modify: `.gitignore`

**Step 1: Add tmp/api.port to .gitignore**

Add to `.gitignore`:

```
# Dynamic port file
tmp/api.port
```

**Step 2: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add .gitignore && git commit -m "$(cat <<'EOF'
[Chore] Add tmp/api.port to .gitignore

Port file is generated at runtime by the API server.
EOF
)"
```

---

## Task 15: Write E2E Test for Setup Flow

**Files:**
- Create: `e2e/setup.spec.ts`

**Step 1: Write the setup E2E test**

Create file `/Users/robinjoseph/.worktrees/shisho/frontend-testing/e2e/setup.spec.ts`:

```typescript
import { expect, test } from "@playwright/test";

test.describe("Setup flow", () => {
  // Note: This test requires a fresh database (no users)
  // Run with: API_PORT=<port> yarn test:e2e e2e/setup.spec.ts

  test("shows setup page on fresh install", async ({ page }) => {
    await page.goto("/");

    // Should redirect to /setup on fresh install
    await expect(page).toHaveURL(/\/setup/);

    // Check for setup form elements
    await expect(page.getByText("Welcome!")).toBeVisible();
    await expect(
      page.getByText("Create your admin account to get started"),
    ).toBeVisible();

    await expect(page.getByLabel("Username")).toBeVisible();
    await expect(page.getByLabel(/Email/)).toBeVisible();
    await expect(page.getByLabel("Password", { exact: true })).toBeVisible();
    await expect(page.getByLabel("Confirm Password")).toBeVisible();
  });

  test("validates required fields", async ({ page }) => {
    await page.goto("/setup");

    // Try to submit empty form
    await page.getByRole("button", { name: "Create Admin Account" }).click();

    // Should show error toast
    await expect(page.getByText("Username is required")).toBeVisible();
  });

  test("validates username length", async ({ page }) => {
    await page.goto("/setup");

    await page.getByLabel("Username").fill("ab");
    await page.getByRole("button", { name: "Create Admin Account" }).click();

    await expect(
      page.getByText("Username must be at least 3 characters"),
    ).toBeVisible();
  });

  test("validates password length", async ({ page }) => {
    await page.goto("/setup");

    await page.getByLabel("Username").fill("testadmin");
    await page.getByLabel("Password", { exact: true }).fill("short");
    await page.getByRole("button", { name: "Create Admin Account" }).click();

    await expect(
      page.getByText("Password must be at least 8 characters"),
    ).toBeVisible();
  });

  test("validates password match", async ({ page }) => {
    await page.goto("/setup");

    await page.getByLabel("Username").fill("testadmin");
    await page.getByLabel("Password", { exact: true }).fill("password123");
    await page.getByLabel("Confirm Password").fill("different123");
    await page.getByRole("button", { name: "Create Admin Account" }).click();

    await expect(page.getByText("Passwords do not match")).toBeVisible();
  });
});
```

**Step 2: Remove .gitkeep**

Run:
```bash
rm /Users/robinjoseph/.worktrees/shisho/frontend-testing/e2e/.gitkeep
```

**Step 3: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add e2e/setup.spec.ts && git rm e2e/.gitkeep && git commit -m "$(cat <<'EOF'
[Test] Add E2E tests for setup flow

Test setup page visibility, form validation for username,
password length, and password confirmation.
EOF
)"
```

---

## Task 16: Write E2E Test for Login Flow

**Files:**
- Create: `e2e/login.spec.ts`

**Step 1: Write the login E2E test**

Create file `/Users/robinjoseph/.worktrees/shisho/frontend-testing/e2e/login.spec.ts`:

```typescript
import { expect, test } from "@playwright/test";

test.describe("Login flow", () => {
  // Note: These tests require a user to exist in the database
  // Run with: API_PORT=<port> yarn test:e2e e2e/login.spec.ts

  test("shows login page when not authenticated", async ({ page }) => {
    // Assuming setup is complete, visiting / should redirect to /login
    await page.goto("/login");

    // Check for login form elements
    await expect(
      page.getByText("Sign in to access your library"),
    ).toBeVisible();
    await expect(page.getByLabel("Username")).toBeVisible();
    await expect(page.getByLabel("Password")).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });

  test("validates empty credentials", async ({ page }) => {
    await page.goto("/login");

    await page.getByRole("button", { name: "Sign in" }).click();

    await expect(
      page.getByText("Please enter both username and password"),
    ).toBeVisible();
  });

  test("shows error on invalid credentials", async ({ page }) => {
    await page.goto("/login");

    await page.getByLabel("Username").fill("wronguser");
    await page.getByLabel("Password").fill("wrongpassword");
    await page.getByRole("button", { name: "Sign in" }).click();

    // Wait for error response
    await expect(page.getByText(/Login failed|invalid/i)).toBeVisible({
      timeout: 10000,
    });
  });
});
```

**Step 2: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add e2e/login.spec.ts && git commit -m "$(cat <<'EOF'
[Test] Add E2E tests for login flow

Test login page visibility, empty credential validation,
and invalid credentials error handling.
EOF
)"
```

---

## Task 17: Update Frontend Skill Documentation

**Files:**
- Modify: `.claude/skills/frontend/SKILL.md`

**Step 1: Add testing section to frontend skill**

Add to end of `/Users/robinjoseph/.worktrees/shisho/frontend-testing/.claude/skills/frontend/SKILL.md`:

```markdown

## Testing

### Stack
- **Unit + Component**: Vitest + React Testing Library
- **E2E**: Playwright (Chromium + Firefox)

### Running Tests
```bash
yarn test          # All tests (unit + E2E)
yarn test:unit     # Vitest only
yarn test:e2e      # Playwright only
make test:js       # Same as yarn test
make check         # All checks including JS tests
```

### Test File Locations
- Unit/Component tests: Colocated as `*.test.ts(x)` next to source files
- E2E tests: `e2e/*.spec.ts`

### Writing Tests

**Unit tests** - Pure functions:
```typescript
import { describe, expect, it } from "vitest";
import { myFunction } from "./myModule";

describe("myFunction", () => {
  it("does something", () => {
    expect(myFunction(input)).toBe(expected);
  });
});
```

**Component tests** - React components:
```typescript
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import MyComponent from "./MyComponent";

describe("MyComponent", () => {
  it("renders correctly", () => {
    render(<MyComponent prop="value" />);
    expect(screen.getByText("expected text")).toBeInTheDocument();
  });
});
```

**E2E tests** - User flows:
```typescript
import { expect, test } from "@playwright/test";

test("user can complete flow", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: "Click me" }).click();
  await expect(page.getByText("Success")).toBeVisible();
});
```

### Coverage
- Auto-collected with unit tests (V8 provider)
- Reports in `coverage/` (text, lcov, HTML)
- View HTML report: `open coverage/index.html`
```

**Step 2: Commit**

```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && git add .claude/skills/frontend/SKILL.md && git commit -m "$(cat <<'EOF'
[Docs] Add testing section to frontend skill

Document test stack, commands, file locations, and patterns
for unit, component, and E2E tests.
EOF
)"
```

---

## Task 18: Run Full Test Suite and Verify

**Files:** None (verification only)

**Step 1: Run make check**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && make check
```

Expected: All Go tests pass, JS lint passes, unit tests pass

**Step 2: Run unit tests with coverage**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && yarn test:unit
```

Expected: 35 tests pass with coverage report

**Step 3: Verify E2E config (don't run full suite without backend)**

Run:
```bash
cd /Users/robinjoseph/.worktrees/shisho/frontend-testing && npx playwright test --list
```

Expected: Lists 8 E2E tests across 2 files

**Step 4: Final verification commit**

If all passes, no commit needed. If issues found, fix and commit.

---

## Summary

This plan creates a complete frontend testing foundation:

1. **Tasks 1-6**: Infrastructure setup (dependencies, configs, scripts, Makefile)
2. **Tasks 7-10**: Unit tests for `identifiers.ts` (31 tests)
3. **Task 11**: Component test for `CoverPlaceholder.tsx` (4 tests)
4. **Tasks 12-14**: Dynamic port handling for worktree isolation
5. **Tasks 15-16**: E2E tests for setup and login flows (8 tests)
6. **Task 17**: Documentation update
7. **Task 18**: Final verification

Total: 43 tests (35 unit/component + 8 E2E)
