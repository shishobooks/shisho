# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Important Notes

**When `make tygo` says "Nothing to be done for \`tygo'", this is NORMAL.** It means the generated types are already up-to-date. Do not treat this as an error. Do not try to run `tygo` directly outside of make - always use `make tygo`. The user often has `make start` running in another session which runs tygo automatically, but you should still run `make tygo` yourself (especially in worktrees where `make start` may not be running).

**Keep CLAUDE.md files up to date.** Subdirectory `CLAUDE.md` files document patterns, conventions, and gotchas for each area of the codebase. When you make changes that affect what's documented — adding new patterns, changing APIs, renaming fields, adding new conventions — update the relevant `CLAUDE.md` to reflect the new state. Outdated documentation is worse than no documentation.

- **Domain-specific** (patterns, gotchas, conventions for a specific area) → Update or add to the relevant `CLAUDE.md` in the subdirectory (e.g., `pkg/epub/CLAUDE.md`)
- **Project-wide** (general conventions, critical gotchas, workflow rules) → Update or add to this file (CLAUDE.md)

Examples of things to record: discovered gotchas, naming conventions, architectural decisions, common mistakes, integration patterns, edge cases.

## Subdirectory CLAUDE.md Files

Project-specific conventions are documented in `CLAUDE.md` files within each subdirectory. These are automatically loaded when working on files in that directory.

| Location | Covers |
|----------|--------|
| `pkg/CLAUDE.md` | Go backend: Echo handlers, Bun ORM, workers, metadata sync checklist |
| `app/CLAUDE.md` | React frontend: Tanstack Query, components, UI patterns |
| `pkg/plugins/CLAUDE.md` | Plugin system: Goja runtime, hooks, host APIs, manifests |
| `pkg/epub/CLAUDE.md` | EPUB format: OPF, Dublin Core, parsing/generation |
| `pkg/cbz/CLAUDE.md` | CBZ format: ComicInfo.xml, creator roles, chapter detection |
| `pkg/kepub/CLAUDE.md` | KePub format: koboSpan wrapping, CBZ-to-KePub conversion |
| `pkg/mp4/CLAUDE.md` | M4B format: iTunes atoms, chapters, narrator fallback |
| `website/CLAUDE.md` | Docs site: Docusaurus, versioning, deployment |
| `e2e/CLAUDE.md` | E2E testing: Playwright, per-browser isolation, fixtures |

## Utility Skills

These workflow-based skills (in `.claude/skills/`) are invoked on demand:

| Skill | Invoke When |
|-------|-------------|
| `favicon` | Creating or updating favicon, app icons, PWA icons |
| `splash` | Creating or updating the README splash image |

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

**CoverImagePath stores filename only** — `file.CoverImagePath` stores just the filename (e.g., `book.cbz.cover.jpg`), NOT the full path. The full path is constructed at runtime. Always use `filepath.Base()` when updating:
```go
// ❌ WRONG - stores full path, breaks cover serving
file.CoverImagePath = &fullPath

// ✅ CORRECT - stores filename only
filename := filepath.Base(fullPath)
file.CoverImagePath = &filename
```

**JSON field naming is snake_case** — All JSON request/response payloads use `snake_case` (e.g., `created_at`, not `createdAt`). Go struct tags: `json:"snake_case_name"`.

**Self password reset route must not require users permissions** — `/users/:id/reset-password` should only require authentication. The handler enforces that self-reset is allowed and resetting another user requires `users:write`. Adding `users:read`/`users:write` middleware to the route breaks self-service password changes for roles like Viewer, including forced password reset flows.

### Frontend

**Cover images need cache busting** — All cover image URLs must include a `?t=` parameter to ensure updated covers display without caching issues:
```tsx
const coverUrl = `/api/books/${id}/cover?t=${query.dataUpdatedAt}`;
```

### Plugins

**SDK must stay in sync with Go** — When modifying plugin-related Go types (`pkg/plugins/`, `pkg/mediafile/mediafile.go`), the TypeScript SDK in `packages/plugin-types/` MUST be updated to match. Breaking changes to the SDK should be avoided.

## Development Commands

### Backend (Go)
- `make setup` - Install dependencies and build tools (run this when setting up a new worktree)
- `make build` - Build production API binary
- `make start` - Start development environment with Hivemind (both API and web)
- `make start:air` - Start API with hot reload via Air
- `make check` - Run all validation checks in parallel (tests, Go lint, JS lint)
- `make lint` - Run Go linting with golangci-lint
- `make test` - Run all Go tests with race detection and coverage

### Frontend (React/TypeScript)
- `yarn start` - Start Vite dev server
- `yarn build` - Build production frontend
- `yarn lint` - Run ESLint, TypeScript checks, and Prettier
- `yarn lint:eslint` - ESLint only
- `yarn lint:types` - TypeScript type checking only
- `yarn lint:prettier` - Prettier formatting check only

**Dependency Structure:** The `dependencies` vs `devDependencies` split in `package.json` is optimized for Docker builds, not traditional Node.js semantics:
- `dependencies`: Everything needed for `yarn build` (React, UI libs, vite, typescript, @types/*)
- `devDependencies`: Only test/lint tools (eslint, prettier, vitest, playwright, testing-library)

This allows the Dockerfile to use `yarn install --production` to skip installing test/lint tools, reducing build time and image layer size. When adding new packages, put build-time dependencies in `dependencies` and test/lint tools in `devDependencies`.

### Database
- `make db:migrate` - Run all pending migrations
- `make db:rollback` - Rollback last migration
- `make db:migrate:create name=migration_name` - Create new migration

### Type Generation
- `make tygo` - Generate TypeScript types from Go structs (see note at top about "Nothing to be done" message)
- Types are generated into `app/types/generated/` from Go packages via `tygo.yaml`
- **IMPORTANT**: The `app/types/generated/` directory is gitignored - these files are auto-generated and cannot be `git add`ed. If you need to update types, modify the Go source structs and run `make tygo`

## Architecture Overview

### Stack
- **Backend**: Go with Echo web framework, Bun ORM, SQLite database
- **Frontend**: React 19 with TypeScript, TailwindCSS, Tanstack Query, Vite
- **Development**: Air for Go hot reload, Hivemind for process management

For detailed architecture information, see:
- **Backend details**: `pkg/CLAUDE.md`
- **Frontend details**: `app/CLAUDE.md`

## Development Workflow

- Use `make start` to run both API and frontend in development (this also runs `make tygo` automatically)
- Database is SQLite file at `tmp/data.sqlite`
- Sample library files in `tmp/library/` for testing
- All Go files are formatted with `goimports` so all changes should continue that formatting
- Always run `make check:quiet` before committing — it suppresses output on success and only shows output for failing steps, printing a one-line pass/fail summary. Use `make check` only when you need full verbose output for debugging.
- **Don't run checks multiple times** — `make check:quiet` gives a clear pass/fail summary in ~6 lines. Run it once.
- **Keep docs up to date.** When making any user-facing change — new feature, changed behavior, new/changed config option, new API endpoint, modified UI — the corresponding page in `website/docs/` MUST be updated or created. If unsure which page, check the sidebar structure in `website/docs/`. This includes but is not limited to:
  - New or changed config options → `website/docs/configuration.md`
  - Plugin system changes → `website/docs/plugins/`
  - Metadata, resource, or relationship changes → `website/docs/metadata.md`
  - User/role/permission changes → `website/docs/users-and-permissions.md`
  - Sidecar format changes → `website/docs/sidecar-files.md`
  - Supplement discovery changes → `website/docs/supplement-files.md`
  - Format support changes → `website/docs/supported-formats.md`
  - New pages should cross-link to related pages (and vice versa)
- **If a new field is added to `pkg/config/config.go`**, both `shisho.example.yaml` AND `website/docs/configuration.md` MUST be updated with the new field. These files must always be a complete reference of all server config options. Exception: `environment` is test-only and should not be included.

## Node.js Version

When updating the Node.js version, update **all** of these locations:
- `.node-version` - Used by version managers (fnm, nodenv, etc.)
- `Dockerfile` - The `node:X.X.X-alpine` image in the frontend-builder stage
- `.github/workflows/ci.yml` - `node-version` in lint-js and test-js jobs
- `.github/workflows/release.yml` - `node-version` in test and npm jobs
- `package.json` - `@types/node` version (run `yarn install` after)

## Testing Strategy

- Go tests use standard testing package with testify assertions
- Tests should use `TZ=America/Chicago CI=true` environment
- **Always add `t.Parallel()` to new Go tests** to enable concurrent execution. Place it as the first line in each test function. Exception: tests in `pkg/plugins` and `pkg/config` have shared global state and cannot be parallelized.
- Frontend uses the same linting rules as backend for consistency
- Database migrations tested via `make db:rollback && make db:migrate`
- Tests should be added for any major pieces of functionality like workers or file parsers. If handler logic is also complex, it should be extracted out and tested separately.
- Whenever fixing a bug, test-driven development should be employed: write a test for the bug, confirm that it fails, fix the bug, and confirm that it passes.

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

- Use `make release tag=0.2.0` to create a release
- This runs `scripts/release.sh` which:
  1. Generates changelog from commits since the last tag
  2. Updates `CHANGELOG.md`, `package.json`, and `packages/plugin-types/package.json`
  3. Creates a commit `[Release] v0.2.0`
  4. Tags and pushes to trigger GitHub Actions

## Worktree Setup

- Worktrees should be created in `~/.worktrees/shisho/`
- After creating a new worktree, run `make setup` to install dependencies and build tools
- Example: `git worktree add ~/.worktrees/shisho/my-feature -b feature/my-feature && cd ~/.worktrees/shisho/my-feature && make setup`

## Database Best Practices

- **Always consider indexes** when modifying database schema or query patterns
- For deletion queries, ensure indexes exist on the WHERE clause columns
- For foreign key relationships, index the referencing column (e.g., `job_id` in `job_logs`)
- Composite indexes should match query patterns (column order matters)
- **The table for authors/narrators is named `persons`, NOT `people`.** This is a common mistake in raw SQL queries. The Go package is `pkg/people` and the model is `models.Person`, but the database table is `persons`.
