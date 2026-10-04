# AGENTS.md

This file provides guidance to coding agents (Claude Code, Codex, Pi, etc.) when working with code in this repository.

## Subagent Instructions

**When dispatching subagents (for implementation, code review, spec review, or any other task), always include this instruction in the prompt:**

> Check the project's root AGENTS.md and any relevant subdirectory AGENTS.md files for rules that apply to your work. These contain critical project conventions, gotchas, and requirements (e.g., docs update requirements, testing conventions, naming rules). Violations of these rules are review failures.

Subdirectory AGENTS.md files are loaded automatically when working on files in that directory, but cross-cutting rules (like "update website docs when changing user-facing behavior") live in this root file and are easy to overlook if not explicitly checked.

## Important Notes

**When `mise tygo` prints "skipping, outputs are up-to-date", this is NORMAL.** It means the generated types are already up-to-date (mise checks source/output timestamps). Do not treat this as an error. The user often has `mise start` running in another session which runs tygo automatically via air, but you should still run `mise tygo` yourself (especially in worktrees where `mise start` may not be running).

**Keep AGENTS.md files up to date.** Subdirectory `AGENTS.md` files document patterns, conventions, and gotchas for each area of the codebase. When you make changes that affect what's documented, such as adding new patterns, changing APIs, renaming fields, or adding new conventions, update the relevant `AGENTS.md` to reflect the new state. Outdated documentation is worse than no documentation.

- **Domain-specific** (patterns, gotchas, conventions for a specific area) → Update or add to the relevant `AGENTS.md` in the subdirectory (e.g., `pkg/epub/AGENTS.md`)
- **Project-wide** (general conventions, critical gotchas, workflow rules) → Update or add to this file (AGENTS.md)

Examples of things to record: discovered gotchas, naming conventions, architectural decisions, common mistakes, integration patterns, edge cases.

## Subdirectory AGENTS.md Files

Project-specific conventions are documented in `AGENTS.md` files within each subdirectory. These are automatically loaded when working on files in that directory.

| Location | Covers |
|----------|--------|
| `pkg/AGENTS.md` | Go backend: Echo handlers, Bun ORM, workers |
| `app/AGENTS.md` | React frontend: Tanstack Query, components, UI patterns |
| `app/components/layout/AGENTS.md` | Shared layout primitives: Sidebar, UserMenu, top-nav class constants |
| `pkg/plugins/AGENTS.md` | Plugin system: Goja runtime, hooks, host APIs, manifests |
| `pkg/epub/AGENTS.md` | EPUB format: OPF, Dublin Core, parsing/generation |
| `pkg/cbz/AGENTS.md` | CBZ format: ComicInfo.xml, creator roles, chapter detection |
| `pkg/kepub/AGENTS.md` | KePub format: koboSpan wrapping, CBZ-to-KePub conversion |
| `pkg/mp4/AGENTS.md` | M4B format: iTunes atoms, chapters, narrator fallback |
| `pkg/pdf/AGENTS.md` | PDF format: info dict metadata, pdfcpu thread safety |
| `pkg/pdfpages/AGENTS.md` | PDF page cache: render/cache PDF pages as JPEG, thread safety, config |
| `pkg/events/AGENTS.md` | SSE: event broker, streaming handler, event types |
| `pkg/audnexus/AGENTS.md` | Audnexus chapter lookup: cached HTTP client, typed error codes, the M4B chapter route |
| `website/AGENTS.md` | Docs site: Docusaurus, versioning, deployment |
| `e2e/AGENTS.md` | E2E testing: Playwright, per-browser isolation, fixtures |
| `tools/gotestsplit/AGENTS.md` | Timing-aware Go test sharding: cache strategy, picking shard count, recalibration playbook |

## Utility Skills

These workflow-based skills (in `.claude/skills/`) are invoked on demand:

| Skill | Invoke When |
|-------|-------------|
| `favicon` | Creating or updating favicon, app icons, PWA icons |
| `splash` | Creating or updating the README splash image |
| `metadata-field` | Adding, removing, or significantly modifying a metadata field on books or files |

## Critical Gotchas

These are common mistakes that cause bugs. Most are summarized here in a line and documented in detail in `pkg/AGENTS.md`, `app/AGENTS.md`, or `pkg/plugins/AGENTS.md`; the self password reset rule lives only here.

### Backend

**Request binding must use structs.** The custom binder uses mold and validator, which only work with structs, so never bind directly to a slice or array. See "Request binding must use structs" under API Conventions in `pkg/AGENTS.md`.

**`CoverImageFilename` stores the filename only**, never a full path; use `filepath.Base()` when updating it. See "Cover Image System" in `pkg/AGENTS.md`.

**JSON field naming is `snake_case`**, except the plugin manifest and repository-index passthrough fields. See "API Conventions" in `pkg/AGENTS.md`.

**API types are generated from Go via tygo: no anonymous responses and no hand-written TS.** Go is the single source of truth for every request and response shape. The rules (named structs in `types.go`, embedding with `tstype:",extends"`, `{Entity}Response`/`List{Entities}Response`/`{Entity}ListItem` naming, the bare-model and two-tier collection rules) live under API Conventions in `pkg/AGENTS.md`; frontend consumption is under API Integration in `app/AGENTS.md`. See ADR 0004 (`docs/adr/0004-tygo-generated-api-types.md`) for the rationale and its amendments.

**Self password reset route must not require users permissions.** `/users/:id/reset-password` should only require authentication. The handler enforces that self-reset is allowed and resetting another user requires `users:write`. Adding `users:read`/`users:write` middleware to the route breaks self-service password changes for roles like Viewer, including forced password reset flows.

### Frontend

**Cover and page images require URL-based cache busting.** API cover endpoints and the CBZ/PDF page endpoint use `Cache-Control: immutable` so browsers cache forever, so every URL carries a `?v=` key that changes only when the image does. Build cover URLs with `bookCoverUrl`, `seriesCoverUrl`, and `fileCoverUrl` from `app/utils/coverUrl.ts` (keyed on the backend `cover_cache_key` for books and series, and on `file.updated_at` in epoch milliseconds for files) and page URLs with the function `useFilePageUrl()` returns (it wraps `filePageUrl` from `app/utils/pageUrl.ts` and adds the PDF render key). Download and stream URLs come from `app/utils/downloadUrl.ts`. ESLint rejects a literal `/api/.../cover`, `/page/`, `/download`, or `/stream` URL anywhere outside `app/utils`. See `app/AGENTS.md` for details.

```tsx
const coverUrl = bookCoverUrl(book);
<img key={coverUrl} src={coverUrl} />;
```

### Plugins

**SDK must stay in sync with Go.** When modifying plugin-related Go types (`pkg/plugins/`, `pkg/mediafile/mediafile.go`), the TypeScript SDK in `packages/plugin-sdk/` MUST be updated to match. Breaking changes to the SDK should be avoided. See "Plugin SDK" in `pkg/plugins/AGENTS.md`.

## Development Commands

### Setup
- `mise setup` - Install all tools, JS dependencies, and generate types (one-command setup)

### Dev Server
- `mise start` - Start development environment (API with hot reload + Vite frontend)
- `mise start:air` - Start API with hot reload via Air only
- `mise start:api` - Start API directly (no hot reload)
- `mise docs` - Start documentation dev server

### Build
- `mise build` - Generate API types, build the frontend, copy it into `pkg/frontend/dist`, and compile a self-contained production binary

`pkg/frontend` embeds the SPA. Keep `pkg/frontend/dist/placeholder.html` checked in so plain `go build` and `go test ./...` work without Node or a frontend build. Generated `index.html` and assets are gitignored; the handler serves the placeholder's "frontend not built" page only when `index.html` is absent. Do not overwrite the tracked placeholder with build output. Use `mise build` for a complete local binary; `pnpm build` alone does not refresh the embedded files.

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
- `mise test:scripts` - Run the shell script tests (`scripts/changelog_test.sh`, the release changelog generator)
- `mise test:e2e` - Run app E2E tests (Chromium + Firefox) in parallel
- `mise e2e:docs` - Run documentation theme E2E tests in Chromium

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

### Public Demo

`demo/` holds the derived Public Demo image (`Dockerfile`), the Fly.io config (`fly.toml`), the corpus authoring Compose file, and `demo/README.md` with the authoring loop and operator setup. `.github/workflows/demo.yml` deploys when the Release workflow calls it after publishing the image, on manual dispatch, and on `repository_dispatch` from `shishobooks/demo-corpus`; it refuses tags older than the first Demo Mode release. Media and the prepared database live only in that corpus repository; `demo/corpus/` is a gitignored CI checkout. Demo Mode behavior itself is documented in `pkg/AGENTS.md`.

### Docker builds

The type-generation, frontend, and backend Docker stages run on `$BUILDPLATFORM`. The backend uses `CGO_ENABLED=0` and cross-compiles with `$TARGETOS`/`$TARGETARCH`; declare target and version arguments only after dependency installation so they do not invalidate dependency layers. Keep the final Alpine stage on the target platform. Multiarch builders still need QEMU for its package installation and user setup, not for Go or Node compilation.

CI builds one multiarch OCI archive and runs that same artifact on native AMD64 and ARM64 runners. Smoke tests check architecture, startup, the embedded frontend and its assets, version injection, custom `PUID`/`PGID`, and graceful shutdown. Do not rebuild the image in smoke jobs or use emulation there.

Only trusted `master` pushes export the shared GHA `image-smoke` cache. PRs and release tags may import it but must not export tag-local caches: exporting the intermediate layers can take longer than compiling, and the next release tag cannot reuse the previous tag's cache.

### Production serving

The Alpine image runs a single Go process through `su-exec` after resolving `PUID`/`PGID` and preparing `/config` ownership. It has no Caddy layer, startup health polling, or signal-forwarding shell. The image sets `SERVER_PORT=5173`; the application default stays `3689`. The listener honors `server_host`.

The Go server owns `/api` directly. Vite forwards `/api` unchanged, without injecting `X-Forwarded-Prefix`. Keep `/health`, `/opds`, `/kobo`, `/ereader`, and `/e` at the root. Forwarded-header trust, compression exclusions, security headers, and frontend cache behavior belong to the Go server, not a bundled proxy. See ADR 0007 (`docs/adr/0007-single-binary-image.md`) for the trade-offs.

For detailed architecture information, see:
- **Backend details**: `pkg/AGENTS.md`
- **Frontend details**: `app/AGENTS.md`

## Development Workflow

- Use `mise start` to run both API and frontend in development (air runs `mise tygo` automatically before each rebuild)
- Database is SQLite file at `tmp/data.sqlite`
- Sample library files in `tmp/library/` for testing
- All Go files are formatted with `goimports` so all changes should continue that formatting
- **While iterating, run only the targeted subset of checks relevant to what you changed.** `mise check:quiet` fans out four heavy parallel pipelines that peg CPU; running it between every iteration is wasteful when you only touched one stack. Subset cheat sheet:
  - Go-only edits → `mise run lint ::: test`
  - Frontend-only edits → `mise run lint:js ::: test:unit` (already runs `tygo`, eslint, prettier, tsc, and the SDK build)
  - Separate tasks with `:::`. `mise lint test` passes `test` to the linter as an argument and silently skips the tests.
  - Both → run both
  - Release or changelog scripts under `scripts/` → `mise test:scripts`
  - Migrations → also `mise db:rollback && mise db:migrate`
  - App E2E flows → `mise e2e:chromium` only when you actually touched a flow (CI runs Firefox)
  - Documentation theme flows → `mise e2e:docs`
- **Run the full `mise check:quiet` once when the feature/fix is done, before pushing or opening a PR.** Concurrent runs from different worktrees serialize automatically via `flock` (install with `brew install flock` on macOS; built in on Linux), so you don't need to coordinate with other agents. Just kick it off and it'll wait its turn if another is in flight. Avoid plain `mise check`: its parallel verbose output is hard to follow and tempts you to re-run it.
- **Keep docs up to date.** When making any user-facing change (new feature, changed behavior, new/changed config option, new API endpoint, modified UI), the corresponding page in `website/docs/` MUST be updated or created. **This applies to implementation plans too:** if a plan changes user-facing behavior, it MUST include a task for updating docs. If unsure which page, check the sidebar structure in `website/docs/`. This includes but is not limited to:
  - New or changed config options → `website/docs/configuration.md`
  - Plugin system changes → `website/docs/plugins/`
  - Metadata, resource, or relationship changes → `website/docs/metadata.md`
  - User/role/permission changes → `website/docs/users-and-permissions.md`
  - Sidecar format changes → `website/docs/sidecar-files.md`
  - Supplement discovery changes → `website/docs/supplement-files.md`
  - Format support changes → `website/docs/supported-formats.md`
  - New pages should cross-link to related pages (and vice versa)
- **If a new field is added to `config.Config` in `pkg/config/config.go`**, update all three of these in the same change:
  - `shisho.example.yaml`: the field, its env var name, default value, and a description.
  - `website/docs/configuration.md`: the same reference for users.
  - `app/components/pages/AdminSettings.tsx`: the Server Settings page shows every non-secret config field.

  The yaml file and the docs page must always be a complete reference of all server config options. Exception: `shisho_test_mode` (env `SHISHO_TEST_MODE`, field `TestMode`) is test-only, so it is left out of `shisho.example.yaml` and `configuration.md` (the Server Settings page still shows it as Test Mode). It mounts the unauthenticated `/api/test/*` routes, so its name must stay one no other tool sets; never go back to a generic key such as `ENVIRONMENT=test`.
  Validation errors name the config key, its env variable and the allowed range (`validationMessage` in `pkg/config/config.go`), so a new rule only needs a `validate` tag. List (`[]string`) fields split comma-separated env values automatically.

## Tool Versions

All tool versions are managed by mise via `mise.toml`. When updating versions, update these locations:

- `mise.toml` - Single source of truth for Go, Node, pnpm, air, tygo, golangci-lint
- `Dockerfile` - The `golang:X.X.X-alpine` and `node:X.X.X-alpine` images, and tygo version in `go install` (Docker doesn't use mise)
- `package.json` - `@types/node` version (run `pnpm install` after)
- `package.json` - `packageManager` field for pnpm (used by Docker via corepack)

## Testing Strategy

- Go tests use standard testing package with testify assertions
- **Tests get their database from `testdb.New(t)`** (`pkg/testutils/testdb`): an in-memory database migrated to the latest schema, with foreign keys on, pinned to one connection as in production, and closed when the test ends. Do not copy a `setupTestDB` into a package. It lives outside `pkg/testutils` because `pkg/testutils` imports auth, apikeys, plugins, and search, whose own tests could not import it. Migration tests that need an older schema, and the worker tests that share a cache-mode database across goroutines, open their own.
- Tests should use `TZ=America/Chicago CI=true` environment
- **Always add `t.Parallel()` to new Go tests** to enable concurrent execution. Place it as the first line in each test function. Exception: tests that use shared global state (e.g., shared database connections, global singletons) cannot be parallelized. `pkg/plugins/AGENTS.md` records which plugin tests can run in parallel. In `pkg/config`, tests mutate global config state and should not be parallelized.
- Frontend uses the same linting rules as backend for consistency
- Database migrations tested via `mise db:rollback && mise db:migrate`
- Tests should be added for any major pieces of functionality like workers or file parsers. If handler logic is also complex, it should be extracted out and tested separately.
- **Follow Red-Green-Refactor TDD for bug fixes and new features.** Do NOT write the implementation and test at the same time. The steps must be sequential:
  1. **Red:** Write the test first. Run it and confirm it **fails** (proving the test actually catches the bug or asserts the new behavior).
  2. **Green:** Write the minimal implementation to make the test pass. Run the test and confirm it **passes**.
  3. **Refactor:** Clean up the implementation if needed, re-running tests to ensure they still pass.

  Skipping the Red step means you can't be sure the test is valid: it might pass regardless of the fix.

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

### Breaking Changes

A change is breaking when an operator has to do something before or after upgrading: a renamed or removed config key or env var, a changed default, a removed route or response field, a new startup validation that can refuse an existing config, or a changed on-disk layout. Mark it in two places, both required:

1. **Title marker.** Put `!` right after the category in the PR title, which becomes the squash commit subject: `[Fix]! Replace ENVIRONMENT=test with SHISHO_TEST_MODE`. The category still decides the changelog section; the `!` flags the change to anyone reading PR titles or `git log`, and on its own it still puts the commit in the changelog's Breaking Changes list when the body section is missing.
2. **Upgrade notes.** Add a `## BREAKING CHANGES` section to the PR body with one bullet per change, written for an operator who is upgrading: what changed, what they must do, and what happens if they do not. Pull requests squash-merge with the PR body as the commit message, so these bullets end up in git history and `scripts/release.sh` copies them into the changelog under the commit's subject. List items (`-`, `*`, or `1.`, with their wrapped lines), plain paragraphs (each becomes a bullet), and fenced code blocks are copied; a `Closes #N` or `Fixes #N` line and trailers such as `Co-authored-by:` are dropped. The section ends at the next heading of the same level, and a deeper heading inside it becomes a bold bullet. The heading alone marks the commit as breaking even when the title has no `!`. A literal `{{` in the notes is escaped before it reaches GoReleaser's template renderer.

Reviewers treat a breaking change without both markers as a review failure. There is no way to add notes at release time: the changelog is generated from commit subjects and bodies, so the PR is the only place to write them.

### Releases

- Use `mise release 0.2.0` to create a release (`mise release 0.2.0 --dry-run` prints the changelog entry without changing anything)
- This runs `scripts/release.sh` which:
  1. Generates the changelog entry from commits since the last tag with `scripts/lib/changelog.sh`: a `### Breaking Changes` block first (commits marked with `!` or carrying a `## BREAKING CHANGES` body section, with their upgrade notes nested under the subject), then the category sections
  2. Updates `CHANGELOG.md`, `package.json`, and `packages/plugin-sdk/package.json`
  3. Creates a commit `[Release] v0.2.0`
  4. Tags and pushes to trigger GitHub Actions
- The release workflow runs `scripts/release-notes-header.sh` to build the GitHub release header from the install block plus that version's `### Breaking Changes` block in `CHANGELOG.md`, and passes it to GoReleaser with `--release-header-tmpl`. That header is the only Breaking Changes section on the release page; GoReleaser's own commit list below it groups by category only, because it reads subjects and would repeat the headline without the notes.
- `mise test:scripts` runs `scripts/changelog_test.sh`, which exercises the generator against a throwaway git repository. Run it after changing anything under `scripts/`.

## Worktree Setup

- Worktrees should be created in `~/.worktrees/shisho/`
- After creating a new worktree, run `mise setup` to install tools and dependencies
- Example: `git worktree add ~/.worktrees/shisho/my-feature -b feature/my-feature && cd ~/.worktrees/shisho/my-feature && mise setup`

## Database Best Practices

- **Migrations must only be marked applied after success.** Always construct Bun migrators through `pkg/migrations.NewMigrator`, not `migrate.NewMigrator` directly. The helper enables `migrate.WithMarkAppliedOnSuccess(true)`. Without it, Bun records a migration before running its body; a failed DDL/data migration can leave the DB half-mutated while future startups skip the migration as already applied.
- **Column `DEFAULT`s never apply when Bun inserts a zero `time.Time` into a field without `nullzero`.** Bun writes the zero value (`0001-01-01 00:00:00`) instead of omitting the column, so `DEFAULT CURRENT_TIMESTAMP` in the migration does nothing. Every insert must set `CreatedAt`/`UpdatedAt` explicitly (the convention is `now := time.Now()` in the service's create method, see `CreateSeries` in `pkg/series/service.go`), or the field must be tagged `bun:",nullzero,notnull,default:current_timestamp"`. The same applies to updates: listing `updated_at` in `Column(...)` writes whatever the struct holds, so set `UpdatedAt = time.Now()` first or it writes back the stale value.
- **Always consider indexes** when modifying database schema or query patterns
- **SQLite table-rebuild migrations must recreate all indexes.** When recreating a table to drop/change columns, list every existing index for that table and recreate it on the replacement table. Dropping the old table drops its indexes too.
- **Table rebuilds must turn foreign keys off first, outside the transaction.** With `PRAGMA foreign_keys=ON`, `DROP TABLE` on a parent runs an implicit DELETE that fires `ON DELETE CASCADE` into every child table and wipes their rows. The pragma is a no-op inside a transaction, so pin one connection, switch it off before BEGIN, and restore it afterwards. The helpers in `pkg/migrations/rebuild.go` do this (`20260928110000_rebuild_files_users_library_paths.go` shows the usage): `withForeignKeysOff` handles the pragma and transaction, and `rebuildTableInTx` copies rows by explicit column list, recreates every index and trigger read from `sqlite_master`, and restores the `sqlite_sequence` high-water mark so deleted AUTOINCREMENT ids are not reused. Reuse them for new rebuilds, finishing with `checkForeignKeys` on the rebuilt tables before commit. Do not copy `recreateTable` from `20260406100000`, which predates these rules.
- For deletion queries, ensure indexes exist on the WHERE clause columns
- For foreign key relationships, index the referencing column (e.g., `job_id` in `job_logs`). The index's leading column must be the foreign key: a composite index that leads with another column (like `ux_user_library_settings (user_id, library_id)` for `library_id`) does not count. Without one, every lookup of children by parent and every `ON DELETE` action on the parent scans the child table. This includes each `*_aliases` parent column, which the search reindex reads once per Book.
- **Case-insensitive name lookups use `name = ? COLLATE NOCASE`, never `LOWER(name) = LOWER(?)`.** Resource and alias tables carry a `(name COLLATE NOCASE, library_id)` unique index, and only a `COLLATE NOCASE` comparison can search it; wrapping the column in `LOWER()` scans the alias table or every row in the library. Both forms fold ASCII letters only, so matching is unchanged. Keep the `library_id = ?` condition where the lookup is library-scoped, so it uses both index columns. Lookups narrowed by parent id instead, like removing one resource's alias, correctly have none.
- Composite indexes should match query patterns (column order matters)
- **The table for authors/narrators is named `persons`, NOT `people`.** This is a common mistake in raw SQL queries. The Go package is `pkg/people` and the model is `models.Person`, but the database table is `persons`.
- **Table names must be plural.** All database tables use plural names (e.g., `plugins`, `plugin_configs`, `plugin_hook_configs`). When creating new tables or referencing existing ones in raw SQL, always use the plural form.
- **Foreign key enforcement is enabled.** `PRAGMA foreign_keys=ON` is set in production. Tests get their database from `testdb.New(t)` (`pkg/testutils/testdb`), which enables it; a test that opens its own database must enable it too.
- **All FK constraints must specify ON DELETE behavior.** Use `ON DELETE CASCADE` for child rows that have no meaning without the parent (e.g., `files.book_id`, `authors.book_id`). Use `ON DELETE SET NULL` for nullable references where the child should survive (e.g., `jobs.library_id`, `files.publisher_id`). Never leave a FK without an explicit ON DELETE action.
- **CASCADE does not clean up FTS indexes**: When deleting books/series/persons/etc., their FTS entries (`books_fts`, `series_fts`, `persons_fts`) are NOT automatically removed by CASCADE, and the CASCADE also drops the links that say which other rows copied the deleted entity (a deleted Book's `book_series` rows). Collect the affected ids with `searchService.CollectAffected` before the delete and `defer searchService.ReindexAffected` after it. FTS rows are keyed by `rowid` equal to the entity id, so never insert FTS rows outside the search service without setting `rowid` (see "Search Index (FTS)" in `pkg/AGENTS.md`).

## Agent skills

### Issue tracker

GitHub Issues on `shishobooks/shisho`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout. See `docs/agents/domain.md`.
