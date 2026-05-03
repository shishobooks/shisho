# Coding Standards

## Style

### Go (Backend)

- **Formatting**: All Go files formatted with `goimports` (auto-managed by golangci-lint formatter)
- **Naming**: PascalCase for exported identifiers, camelCase for unexported
- **JSON fields**: Always `snake_case` in struct tags (`json:"created_at"`)
- **Error wrapping**: Use `github.com/pkg/errors` for stack traces (`errors.WithStack`, `errors.Wrap`)
- **Imports**: Grouped as stdlib, external, internal (enforced by goimports)
- **Comments**: Minimal — only when the "why" isn't obvious. No function-level doc comments unless exported API
- **Constants**: Group related constants with `const ( ... )` blocks; use `//tygo:emit` for TypeScript union type generation

### TypeScript / React (Frontend)

- **Formatting**: Prettier with trailing commas (`"always-multiline"`)
- **Import order**: Builtins → third-party → `@/` aliases → relative (enforced by `@ianvs/prettier-plugin-sort-imports`)
- **JSX props**: Sorted alphabetically (`react/jsx-sort-props` ESLint rule)
- **Naming**: PascalCase for components and types, camelCase for functions and variables
- **Class composition**: Always use `cn()` from `@/libraries/utils` — never template literals for dynamic classNames
- **Exports**: Prefer named exports; `allowConstantExport` is enabled for react-refresh
- **No default exports** except for page components used in router lazy-loading
- **Type imports**: Use `import type { ... }` for type-only imports

### Shared

- **No comments explaining "what"** — code should be self-documenting via naming
- **No task/ticket references in code** — those belong in commit messages and PRs
- **Trailing commas** in multiline structures (enforced in both Go and TS)

## Testing

### Go Tests

- **Framework**: Standard `testing` package with `testify/assert` and `testify/require`
- **Parallelism**: Always add `t.Parallel()` as the first line in new test functions, except when tests share global state (config, shared plugin managers)
- **Environment**: Tests run with `TZ=America/Chicago CI=true`
- **TDD required**: Red-Green-Refactor — write the failing test first, then implement
- **Test files**: Named `*_test.go` colocated with source

### Frontend Tests

- **Unit/Component**: Vitest + React Testing Library, colocated as `*.test.ts(x)`
- **Fake timers**: Enabled globally; use `userEvent.setup({ advanceTimers: vi.advanceTimersByTime })`
- **E2E**: Playwright with per-browser database isolation (Chromium + Firefox)
- **Test independence**: Each E2E test file sets up its own preconditions via `beforeAll`

### General Testing Principles

- Tests for all major functionality (workers, file parsers, complex handlers)
- No mocking of databases in integration tests — use real SQLite
- E2E tests use test-only API endpoints (`ENVIRONMENT=test`) for setup/teardown

## Architecture

### Backend Structure

Each domain package (`pkg/{domain}/`) contains:

- `handlers.go` — HTTP request/response, binds params, calls service
- `routes.go` — Echo route registration and middleware wiring
- `service.go` — Business logic and database operations
- `validators.go` — Request/response payload structs with validation tags

### Key Conventions

- **Request binding must use structs** — never bind directly to slices (the custom binder uses mold/validator which require struct targets)
- **Bun table aliases in queries** — use single-letter aliases (`b` for books, `f` for files) not full table names
- **Context propagation** — pass `context.Context` through all service calls; check `ctx.Err()` before expensive operations
- **Error handling** — use `pkg/errcodes` for typed HTTP errors (`NotFound`, `Forbidden`, `BadRequest`, `Conflict`)
- **Permissions on all routes** — every route must consider RBAC and library access checks

### Frontend Structure

- **Pages**: `app/components/pages/` — top-level route components
- **Components**: `app/components/` — reusable UI components
- **Hooks**: `app/hooks/queries/` — Tanstack Query wrappers per domain
- **API client**: `app/libraries/api.ts` — single `ShishoAPI` class with typed methods
- **Types**: Auto-generated from Go via tygo into `app/types/generated/`

### Key Conventions

- **Semantic color tokens only** — never hardcoded Tailwind colors; CSS variables handle dark mode
- **`cursor-pointer` on all interactive elements** — checkboxes, selects, tabs, dropdowns, buttons
- **Unsaved changes protection** — all create/edit forms must use `FormDialog` or `useUnsavedChanges`
- **Tabbed navigation must be URL-synced** — tabs are deep-linked via route params, never local state alone
- **Page titles required** — all pages use `usePageTitle` hook for browser tab titles
- **Server-side search** — never rely on client-side filtering (list endpoints max at 50 items)
- **Cover image cache-busting** — append `?v=${cacheKey}` using `query.dataUpdatedAt`
- **Cross-resource invalidation** — mutations on metadata entities must also invalidate book queries

### Database

- **ORM**: Bun with SQLite
- **Table names**: Always plural (`books`, `persons`, `files`)
- **Foreign keys**: Always specify `ON DELETE CASCADE` or `ON DELETE SET NULL` — never leave implicit
- **Indexes**: Required on WHERE clause columns and FK referencing columns
- **FTS cleanup**: CASCADE doesn't remove FTS entries — callers must explicitly clean up search indexes
- **Migrations**: Created via `mise db:migrate:create <name>`, rolled back via `mise db:rollback`

### API Design

- **All payloads use `snake_case`** field names
- **List endpoints**: Max 50 items per request, support `limit`/`offset`/`search` params
- **Validation**: Struct tags (`validate:"required,min=1,max=200"`) with trim via `mod:"trim"`
- **Error responses**: `{ "error": { "message": "...", "code": "snake_case_code" } }` with appropriate HTTP status

## Code Quality

### Linting (Go)

Enforced linters include: `errcheck`, `govet`, `staticcheck`, `gosec`, `ineffassign`, `unused`, `revive`, `exhaustive`, `bodyclose`, `noctx`, `perfsprint`. Full list in `.golangci.yml`.

### Linting (Frontend)

- ESLint with TypeScript strict rules (`@typescript-eslint/recommended`)
- React hooks rules (exhaustive deps enforced, `set-state-in-effect` disabled)
- Prettier for formatting (with Tailwind class sorting plugin)
- Zero warnings policy (`--max-warnings 0`)

### Git Conventions

- **Commit format**: `[Category] Description` (e.g., `[Fix] Resolve race condition in job worker`)
- **Categories**: `[Frontend]`, `[Backend]`, `[Feature]`/`[Feat]`, `[Fix]`, `[Docs]`/`[Doc]`, `[Test]`/`[E2E]`, `[CI]`/`[CD]`
- **Commit messages**: Focus on "why", not "what"
- **One concern per commit** — don't mix features with unrelated cleanup

### Security

- No command injection, XSS, SQL injection (OWASP top 10 awareness)
- Session-based auth for frontend API routes, Basic Auth for OPDS, API key auth for eReader
- Never store secrets in code or commit `.env` files
- `gosec` linter catches common Go security issues
