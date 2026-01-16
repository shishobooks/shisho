# Frontend Testing Design

## Overview

Add a frontend testing foundation with three levels:
1. **Unit tests** - Pure function testing (utils, helpers)
2. **Component tests** - React component testing in isolation
3. **E2E tests** - Browser-based user flow testing

## Testing Stack

| Level | Framework | Purpose |
|-------|-----------|---------|
| Unit + Component | Vitest + React Testing Library | Fast, native Vite integration |
| E2E | Playwright | Browser automation (Chromium + Firefox) |

## Directory Structure

```
app/
├── utils/
│   ├── identifiers.ts
│   └── identifiers.test.ts        # Unit tests (colocated)
├── components/
│   └── library/
│       ├── CoverPlaceholder.tsx
│       └── CoverPlaceholder.test.tsx  # Component tests (colocated)
e2e/                                # E2E tests (separate directory)
├── setup.spec.ts
└── login.spec.ts
coverage/                           # Coverage reports (gitignored)
```

## Dynamic Port Handling

To support multiple `make start` instances across worktrees:

### Backend Changes (`cmd/api/main.go`)
- Use `net.Listen` instead of `srv.ListenAndServe()`
- Write actual bound port to `tmp/api.port`
- Log actual port (not config port)

### Vite Changes (`vite.config.ts`)
- Priority: `API_PORT` env var → `tmp/api.port` file → default 3689
- Use `server.strictPort: false` to auto-increment frontend port

### Flow
- **`make start`**: Backend writes port file, Vite reads it
- **E2E tests**: Use explicit `API_PORT` env var for isolation

## Configuration

### Vitest (`vitest.config.ts`)
```typescript
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./app") },
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

### Playwright (`playwright.config.ts`)
```typescript
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

## Scripts

```json
{
  "test": "concurrently --kill-others-on-fail --group \"yarn:test:*\"",
  "test:unit": "vitest run",
  "test:e2e": "playwright test"
}
```

## Makefile Integration

```makefile
.PHONY: check
check:
	$(MAKE) -j4 test test\:js lint lint\:js

.PHONY: test\:js
test\:js:
	yarn test
```

## Initial Tests

### Unit Tests (`app/utils/identifiers.test.ts`)
- `validateISBN10` - valid, invalid, X check digit
- `validateISBN13` - valid, invalid checksums
- `validateASIN` - format validation
- `validateUUID` - format validation
- `validateIdentifier` - dispatch to correct validator

### Component Tests (`app/components/library/CoverPlaceholder.test.tsx`)
- Renders correct viewBox for book vs audiobook variant
- Applies custom className

### E2E Tests
- `e2e/setup.spec.ts` - Admin account creation flow
- `e2e/login.spec.ts` - Login success and failure flows

## Coverage

- Always collected with unit tests (V8 provider, minimal overhead)
- Reports: text (console), lcov, HTML
- Not enforced, tracked for visibility
- Covers TypeScript source files (not generated JS)

## New Dependencies

```
devDependencies:
  vitest
  @vitest/coverage-v8
  jsdom
  @testing-library/react
  @testing-library/dom
  @playwright/test
```

## Files to Modify

| File | Action |
|------|--------|
| `vite.config.ts` | Modify - dynamic API port |
| `vitest.config.ts` | Create |
| `vitest.setup.ts` | Create |
| `playwright.config.ts` | Create |
| `package.json` | Modify - deps & scripts |
| `cmd/api/main.go` | Modify - net.Listener, port file |
| `Makefile` | Modify - add test:js |
| `app/utils/identifiers.test.ts` | Create |
| `app/components/library/CoverPlaceholder.test.tsx` | Create |
| `e2e/setup.spec.ts` | Create |
| `e2e/login.spec.ts` | Create |
| `.claude/skills/frontend/SKILL.md` | Modify - testing section |
