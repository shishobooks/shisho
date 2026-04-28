# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Subagent Instructions

**When dispatching subagents (for implementation, code review, spec review, or any other task), always include this instruction in the prompt:**

> Check the project's root CLAUDE.md and any relevant subdirectory CLAUDE.md files for rules that apply to your work. These contain critical project conventions, gotchas, and requirements (e.g., docs update requirements, testing conventions, naming rules). Violations of these rules are review failures.

Subdirectory CLAUDE.md files are loaded automatically when working on files in that directory, but cross-cutting rules (like "update website docs when changing user-facing behavior") live in this root file and are easy to overlook if not explicitly checked.

## Important Notes

**When `mise tygo` prints "skipping, outputs are up-to-date", this is NORMAL.** It means the generated types are already up-to-date (mise checks source/output timestamps). Do not treat this as an error. The user often has `mise start` running in another session which runs tygo automatically via air, but you should still run `mise tygo` yourself (especially in worktrees where `mise start` may not be running).

**Keep CLAUDE.md files up to date.** Subdirectory `CLAUDE.md` files document patterns, conventions, and gotchas for each area of the codebase. When you make changes that affect what's documented — adding new patterns, changing APIs, renaming fields, adding new conventions — update the relevant `CLAUDE.md` to reflect the new state. Outdated documentation is worse than no documentation.

- **Domain-specific** (patterns, gotchas, conventions for a specific area) → Update or add to the relevant `CLAUDE.md` in the subdirectory (e.g., `pkg/epub/CLAUDE.md`)
- **Project-wide** (general conventions, critical gotchas, workflow rules) → Update or add to this file (CLAUDE.md)

Examples of things to record: discovered gotchas, naming conventions, architectural decisions, common mistakes, integration patterns, edge cases.

## Subdirectory CLAUDE.md Files

Project-specific conventions are documented in `CLAUDE.md` files within each subdirectory. These are automatically loaded when working on files in that directory.

| Location | Covers |
|----------|--------|
| `pkg/CLAUDE.md` | Go backend: Echo handlers, Bun ORM, workers |
| `app/CLAUDE.md` | React frontend: Tanstack Query, components, UI patterns |
| `app/components/layout/CLAUDE.md` | Shared layout primitives: Sidebar, UserMenu, top-nav class constants |
| `pkg/plugins/CLAUDE.md` | Plugin system: Goja runtime, hooks, host APIs, manifests |
| `pkg/epub/CLAUDE.md` | EPUB format: OPF, Dublin Core, parsing/generation |
| `pkg/cbz/CLAUDE.md` | CBZ format: ComicInfo.xml, creator roles, chapter detection |
| `pkg/kepub/CLAUDE.md` | KePub format: koboSpan wrapping, CBZ-to-KePub conversion |
| `pkg/mp4/CLAUDE.md` | M4B format: iTunes atoms, chapters, narrator fallback |
| `pkg/pdf/CLAUDE.md` | PDF format: info dict metadata, pdfcpu thread safety |
| `pkg/pdfpages/CLAUDE.md` | PDF page cache: render/cache PDF pages as JPEG, thread safety, config |
| `pkg/events/CLAUDE.md` | SSE: event broker, streaming handler, event types |
| `website/CLAUDE.md` | Docs site: Docusaurus, versioning, deployment |
| `e2e/CLAUDE.md` | E2E testing: Playwright, per-browser isolation, fixtures |
| `tools/gotestsplit/CLAUDE.md` | Timing-aware Go test sharding: cache strategy, picking shard count, recalibration playbook |

## Utility Skills

These workflow-based skills (in `.claude/skills/`) are invoked on demand:

| Skill | Invoke When |
|-------|-------------|
| `favicon` | Creating or updating favicon, app icons, PWA icons |
| `splash` | Creating or updating the README splash image |
| `metadata-field` | Adding, removing, or significantly modifying a metadata field on books or files |

## Critical Gotchas

These are common mistakes that cause bugs. They're documented in detail in the skills but are easy to miss.

### Backend

**Request binding must use structs** — The custom binder (`pkg/binder/`) uses mold and validator, which only work with structs. Never bind directly to a slice/array:
```go
// ❌ WRONG - causes nil pointer crash
var entries []orderEntry
c.Bind(&entries)

// ✅ CORRECT - wrap in struct
type payload struct { Order []orderEntry `json:"order"` }
var p payload
c.Bind(&p)
```

**CoverImageFilename stores filename only** — `file.CoverImageFilename` stores just the filename (e.g., `book.cbz.cover.jpg`), NOT the full path. The full path is constructed at runtime. Always use `filepath.Base()` when updating:
```go
// ❌ WRONG - stores full path, breaks cover serving
file.CoverImageFilename = &fullPath

// ✅ CORRECT - stores filename only
filename := filepath.Base(fullPath)
file.CoverImageFilename = &filename
```

**JSON field naming is snake_case** — All JSON request/response payloads use `snake_case` (e.g., `created_at`, not `createdAt`). Go struct tags: `json:"snake_case_name"`.

**Self password reset route must not require users permissions** — `/users/:id/reset-password` should only require authentication. The handler enforces that self-reset is allowed and resetting another user requires `users:write`. Adding `users:read`/`users:write` middleware to the route breaks self-service password changes for roles like Viewer, including forced password reset flows.

### Frontend

**Cover images require both URL cache-busting AND HTTP revalidation** — Browsers maintain an in-memory image cache independent from the HTTP cache, so `Cache-Control: no-cache` alone doesn't force a fresh fetch on `<img>` remount with the same src. Cover URLs must include `?v=${cacheKey}` where `cacheKey` reliably bumps on data refetches (e.g., `bookQuery.dataUpdatedAt`). See `app/CLAUDE.md` for details.

```tsx
<img
  key={bookQuery.dataUpdatedAt}
  src={`/api/books/${id}/cover?v=${bookQuery.dataUpdatedAt}`}
/>
```

### Plugins

**SDK must stay in sync with Go** — When modifying plugin-related Go types (`pkg/plugins/`, `pkg/mediafile/mediafile.go`), the TypeScript SDK in `packages/plugin-sdk/` MUST be updated to match. Breaking changes to the SDK should be avoided.

## Development Commands

### Setup
- `mise setup` - Install all tools, JS dependencies, and generate types (one-command setup)

### Dev Server
- `mise start` - Start development environment (API with hot reload + Vite frontend)
- `mise start:air` - Start API with hot reload via Air only
- `mise start:api` - Start API directly (no hot reload)
- `mise docs` - Start documentation dev server

### Build
- `mise build` - Build production API binary

### Linting
- `mise lint` - Run Go linting with golangci-lint
- `mise lint:js` - Run all JS/TS linting (ESLint, Prettier, TypeScript) in parallel
- `mise check` - Run all validation checks in parallel (tests, Go lint, JS lint)
- `mise check:quiet` - Same as check but quieter, skips Firefox e2e (which still runs in CI), and serializes concurrent runs across worktrees via `flock`. Prefer this over `mise check` locally.

### Testing
- `mise test` - Run all Go tests with coverage
- `mise test:race` - Run all Go tests with race detection and coverage (local; CI runs the same `-race` tests but sharded across parallel jobs in `.github/workflows/ci.yml`)
- `mise test:js` - Run all JS tests (unit + E2E) in parallel
- `mise test:js:fast` - Run JS tests with chromium e2e only (Firefox runs in CI); used by `mise check:quiet`
- `mise test:unit` - Run JS unit tests only
- `mise test:e2e` - Run E2E tests (Chromium + Firefox) in parallel

### Database
- `mise db:migrate` - Run all pending migrations
- `mise db:rollback` - Rollback last migration
- `mise db:migrate:create <name>` - Create new migration

### Type Generation
- `mise tygo` - Generate TypeScript types from Go structs (skips if outputs are up-to-date)
- Types are generated into `app/types/generated/` from Go packages via `tygo.yaml`
- **IMPORTANT**: The `app/types/generated/` directory is gitignored - these files are auto-generated and cannot be `git add`ed. If you need to update types, modify the Go source structs and run `mise tygo`

### Frontend (leaf commands, called by mise tasks)
- `pnpm start` - Start Vite dev server
- `pnpm build` - Build production frontend
- `pnpm lint:eslint` - ESLint only
- `pnpm lint:types` - TypeScript type checking only
- `pnpm lint:prettier` - Prettier formatting check only

**Dependency Structure:** The `dependencies` vs `devDependencies` split in `package.json` is optimized for Docker builds, not traditional Node.js semantics:
- `dependencies`: Everything needed for `pnpm build` (React, UI libs, vite, typescript, @types/*)
- `devDependencies`: Only test/lint tools (eslint, prettier, vitest, playwright, testing-library)

This allows the Dockerfile to use `pnpm install --prod` to skip installing test/lint tools, reducing build time and image layer size. When adding new packages, put build-time dependencies in `dependencies` and test/lint tools in `devDependencies`.

## Architecture Overview

### Stack
- **Backend**: Go with Echo web framework, Bun ORM, SQLite database
- **Frontend**: React 19 with TypeScript, TailwindCSS, Tanstack Query, Vite
- **Development**: mise for tool/version management and task running, Air for Go hot reload

For detailed architecture information, see:
- **Backend details**: `pkg/CLAUDE.md`
- **Frontend details**: `app/CLAUDE.md`

## Development Workflow

- Use `mise start` to run both API and frontend in development (air runs `mise tygo` automatically before each rebuild)
- Database is SQLite file at `tmp/data.sqlite`
- Sample library files in `tmp/library/` for testing
- All Go files are formatted with `goimports` so all changes should continue that formatting
- **While iterating, run only the targeted subset of checks relevant to what you changed.** `mise check:quiet` fans out four heavy parallel pipelines that peg CPU; running it between every iteration is wasteful when you only touched one stack. Subset cheat sheet:
  - Go-only edits → `mise lint test`
  - Frontend-only edits → `mise lint:js test:unit` (already runs `tygo`, eslint, prettier, tsc, and the SDK build)
  - Both → run both
  - Migrations → also `mise db:rollback && mise db:migrate`
  - E2E flows → `mise e2e:chromium` only when you actually touched a flow (CI runs Firefox)
- **Run the full `mise check:quiet` once when the feature/fix is done, before pushing or opening a PR.** Concurrent runs from different worktrees serialize automatically via `flock` (install with `brew install flock` on macOS; built in on Linux), so you don't need to coordinate with other agents — just kick it off and it'll wait its turn if another is in flight. Avoid plain `mise check` — its parallel verbose output is hard to follow and tempts you to re-run it.
- **Keep docs up to date.** When making any user-facing change — new feature, changed behavior, new/changed config option, new API endpoint, modified UI — the corresponding page in `website/docs/` MUST be updated or created. **This applies to implementation plans too** — if a plan changes user-facing behavior, it MUST include a task for updating docs. If unsure which page, check the sidebar structure in `website/docs/`. This includes but is not limited to:
  - New or changed config options → `website/docs/configuration.md`
  - Plugin system changes → `website/docs/plugins/`
  - Metadata, resource, or relationship changes → `website/docs/metadata.md`
  - User/role/permission changes → `website/docs/users-and-permissions.md`
  - Sidecar format changes → `website/docs/sidecar-files.md`
  - Supplement discovery changes → `website/docs/supplement-files.md`
  - Format support changes → `website/docs/supported-formats.md`
  - New pages should cross-link to related pages (and vice versa)
- **If a new field is added to `pkg/config/config.go`**, both `shisho.example.yaml` AND `website/docs/configuration.md` MUST be updated with the new field. These files must always be a complete reference of all server config options. Exception: `environment` is test-only and should not be included.

## Tool Versions

All tool versions are managed by mise via `.mise.toml`. When updating versions, update these locations:

- `.mise.toml` - Single source of truth for Go, Node, pnpm, air, tygo, golangci-lint
- `Dockerfile` - The `golang:X.X.X-alpine` and `node:X.X.X-alpine` images, and tygo version in `go install` (Docker doesn't use mise)
- `package.json` - `@types/node` version (run `pnpm install` after)
- `package.json` - `packageManager` field for pnpm (used by Docker via corepack)

## Testing Strategy

- Go tests use standard testing package with testify assertions
- Tests should use `TZ=America/Chicago CI=true` environment
- **Always add `t.Parallel()` to new Go tests** to enable concurrent execution. Place it as the first line in each test function. Exception: tests that use shared global state (e.g., shared database connections, global singletons) cannot be parallelized. In `pkg/plugins`, tests for pure functions (like `handler_convert_test.go`, `hooks_search_result_test.go`, `hostapi_url_test.go`) should use `t.Parallel()`, while tests that share a plugin manager or runtime instance should not. In `pkg/config`, tests mutate global config state and should not be parallelized.
- Frontend uses the same linting rules as backend for consistency
- Database migrations tested via `mise db:rollback && mise db:migrate`
- Tests should be added for any major pieces of functionality like workers or file parsers. If handler logic is also complex, it should be extracted out and tested separately.
- **Follow Red-Green-Refactor TDD for bug fixes and new features.** Do NOT write the implementation and test at the same time. The steps must be sequential:
  1. **Red:** Write the test first. Run it and confirm it **fails** (proving the test actually catches the bug or asserts the new behavior).
  2. **Green:** Write the minimal implementation to make the test pass. Run the test and confirm it **passes**.
  3. **Refactor:** Clean up the implementation if needed, re-running tests to ensure they still pass.

  Skipping the Red step means you can't be sure the test is valid — it might pass regardless of the fix.

## Git Conventions

### Commit Message Format

Each commit should be in the format of `[{Category}] {Change description}`

**Categories** (used for changelog generation):
- `[Frontend]`, `[Backend]`, `[Feature]`, `[Feat]` → Features section
- `[Fix]` → Bug Fixes section
- `[Docs]`, `[Doc]` → Documentation section
- `[Test]`, `[E2E]` → Testing section
- `[CI]`, `[CD]` → CI/CD section
- Any other category → Other section

**Examples:**
```
[Frontend] Add dark mode toggle to settings page
[Backend] Add batch delete endpoint for books
[Fix] Resolve race condition in job worker
[E2E] Add tests for user authentication flow
[CI] Add release automation with GitHub Actions
```

### Releases

- Use `mise release 0.2.0` to create a release
- This runs `scripts/release.sh` which:
  1. Generates changelog from commits since the last tag
  2. Updates `CHANGELOG.md`, `package.json`, and `packages/plugin-sdk/package.json`
  3. Creates a commit `[Release] v0.2.0`
  4. Tags and pushes to trigger GitHub Actions

## Worktree Setup

- Worktrees should be created in `~/.worktrees/shisho/`
- After creating a new worktree, run `mise setup` to install tools and dependencies
- Example: `git worktree add ~/.worktrees/shisho/my-feature -b feature/my-feature && cd ~/.worktrees/shisho/my-feature && mise setup`

## Database Best Practices

- **Always consider indexes** when modifying database schema or query patterns
- For deletion queries, ensure indexes exist on the WHERE clause columns
- For foreign key relationships, index the referencing column (e.g., `job_id` in `job_logs`)
- Composite indexes should match query patterns (column order matters)
- **The table for authors/narrators is named `persons`, NOT `people`.** This is a common mistake in raw SQL queries. The Go package is `pkg/people` and the model is `models.Person`, but the database table is `persons`.
- **Table names must be plural** — All database tables use plural names (e.g., `plugins`, `plugin_configs`, `plugin_hook_configs`). When creating new tables or referencing existing ones in raw SQL, always use the plural form.
- **Foreign key enforcement is enabled** — `PRAGMA foreign_keys=ON` is set in production. Test DB helpers must also enable this pragma.
- **All FK constraints must specify ON DELETE behavior** — Use `ON DELETE CASCADE` for child rows that have no meaning without the parent (e.g., `files.book_id`, `authors.book_id`). Use `ON DELETE SET NULL` for nullable references where the child should survive (e.g., `books.primary_file_id`, `jobs.library_id`, `files.publisher_id`). Never leave a FK without an explicit ON DELETE action.
- **CASCADE does not clean up FTS indexes** — When deleting books/series/persons/etc., their FTS entries (`books_fts`, `series_fts`, `persons_fts`) are NOT automatically removed by CASCADE. Callers that delete searchable entities must also remove from the FTS index via the search service.
