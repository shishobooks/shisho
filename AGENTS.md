# AGENTS.md

Guidance for coding agents (Claude Code, Codex, Pi, etc.) working in this repository.

## How the instructions are organized

- **This file**: rules every task needs.
- **Subdirectory `AGENTS.md` files**: rules most tasks in that directory need. Some harnesses load them automatically and some do not, so before editing files in a directory listed below, read its `AGENTS.md` yourself.
- **Topic docs in `docs/agents/`**: reference for one area. Each is reached by a pointer line that says when to read it; read it when the trigger matches your change.
- **`CODING_STANDARDS.md`**: review-time judgement rules. The code-review Standards reviewer reads it and the files it points to.
- **Lint and tests**: mechanical rules are enforced by golangci-lint, ESLint, `scripts/check-emdash.sh`, and invariant tests. Their messages say what to do instead, so these docs only name them.

| Location | Covers |
|----------|--------|
| `pkg/AGENTS.md` | Go backend: Echo handlers, Bun ORM, workers |
| `app/AGENTS.md` | React frontend: Tanstack Query, components, UI patterns |
| `app/components/layout/AGENTS.md` | Shared layout primitives: Sidebar, UserMenu, top-nav class constants |
| `pkg/plugins/AGENTS.md` | Plugin system: Goja runtime, hooks, host APIs, manifests |
| `pkg/epub/AGENTS.md` | EPUB format: OPF, Dublin Core, parsing/generation |
| `pkg/cbz/AGENTS.md` | CBZ format: ComicInfo.xml, creator roles, chapter detection |
| `pkg/covers/AGENTS.md` | Cover selection, lazy thumbnails, resize bounds, cache invalidation |
| `pkg/kepub/AGENTS.md` | KePub format: koboSpan wrapping, CBZ-to-KePub conversion |
| `pkg/mp4/AGENTS.md` | M4B format: reading, rewriting, chapters |
| `pkg/pdf/AGENTS.md` | PDF format: info dict metadata, pdfcpu thread safety |
| `pkg/pdfpages/AGENTS.md` | PDF page cache: rendered JPEG pages, render key, thread safety |
| `pkg/events/AGENTS.md` | SSE: event broker, streaming handler, event types |
| `pkg/audnexus/AGENTS.md` | Audnexus chapter lookup: cached HTTP client, typed error codes, the M4B chapter route |
| `website/AGENTS.md` | Docs site: Docusaurus, versioning, deployment |
| `e2e/AGENTS.md` | E2E testing: Playwright, per-browser isolation, fixtures |
| `tools/gotestsplit/AGENTS.md` | Timing-aware Go test sharding: design rules, recalibration playbook |

Repo-wide topic docs:

- Read `docs/agents/docker-and-deploy.md` before changing the `Dockerfile`, image smoke tests, the server listener or frontend serving, or anything under `demo/`.
- Read `docs/agents/dev-servers.md` before adding or changing a `mise start*` or `mise docs` task, or anything that picks a dev port or names the session cookie.
- Read `docs/agents/releases.md` before writing a breaking change's upgrade notes, cutting a release, or changing `scripts/`.

## Adding to these files

Instruction files hold two kinds of content, and nothing else:

- **How and why things are done, when it applies across the codebase.** A rule about one function or file goes in a comment at that code, where the agent editing it will see it. A doc earns a rule when the trap fires while writing new code elsewhere (a new delete endpoint must reindex search; a new route family must be classified for Demo Mode), so no comment would be read in time.
- **Guidance that keeps agents out of traps and rabbit holes**: the wrong turn and the right one.

Never copy what the code or config already holds: no lists of files, components, endpoints, fields, ports, or defaults, and no values that change when the code does. Where agents are known to look in the wrong place, name the right lookup (the file, command, or test), not its result. If a rule is mechanical, enforce it with a lint rule or test and keep at most a one-line pointer. Judgement rules for reviewers go in `CODING_STANDARDS.md` or the files it points to; bug history (what used to happen, PR numbers) goes in the commit message. `scripts/check-docrefs.sh` fails when a doc names a path or identifier that no longer exists; when a change makes a line wrong, fix or delete it in the same change.

## Subagent instructions

When dispatching subagents (implementation, review, or anything else), include this in the prompt:

> Read the root AGENTS.md, the AGENTS.md of every directory you will touch, and each topic doc whose pointer trigger matches your change. Reviewers also read CODING_STANDARDS.md and the standards files it points to for the touched areas. Violations of these rules are review failures.

## Critical gotchas

- **`mise tygo` printing "skipping, outputs are up-to-date" is normal**: mise compares source and output timestamps. Still run `mise tygo` yourself, especially in worktrees where `mise start` is not running.
- **Request binding must use structs**, never a slice or array: the custom binder runs mold and validator, which only handle structs.
- **API types are generated from Go via tygo**: Go is the single source of truth for every request and response shape, so there are no anonymous responses and no hand-written TS types. Rules are under "API Conventions" in `pkg/AGENTS.md`; rationale in ADR 0004.
- **JSON fields are `snake_case`**, except the plugin manifest and repository-index passthrough fields.
- **The self password reset route must not require users permissions.** `/users/:id/reset-password` requires authentication only; the handler allows self-reset and requires `users:write` to reset another user. Adding `users:read`/`users:write` middleware breaks self-service and forced password changes for roles like Viewer.
- **Cover, page, download, and stream URLs come from helpers in `app/utils`**, because image endpoints are cached as immutable and rely on a `?v=` key, and server covers render through `CoverImage`, which picks a thumbnail size. ESLint rejects literal URLs; `docs/agents/frontend/image-urls.md` explains the keys.
- **The plugin SDK in `packages/plugin-sdk/` must stay in sync with Go plugin types** (`pkg/plugins/`, `pkg/mediafile/mediafile.go`) without breaking changes. `pkg/plugins/sdk_sync_test.go` checks host APIs, hook names, the manifest, and metadata fields; the rest is under "Plugin SDK" in `pkg/plugins/AGENTS.md`.

## Development commands

All tasks are in `mise.toml` (`mise tasks` lists them). The ones with non-obvious behavior:

- `mise setup` installs tools and dependencies and generates types. Run it after creating a worktree.
- `mise start` runs the API with hot reload plus Vite; air runs `mise tygo` before each rebuild. Several worktrees can run it at once, so each server may land on a different port than usual: read the URL it prints. Never hardcode a dev port in a task; reserve it through `internal/devtool`.
- `mise build` generates types, builds the frontend, copies it into `pkg/frontend/dist`, and compiles the binary. `pnpm build` alone does not refresh the embedded files. Keep the tracked `pkg/frontend/dist/placeholder.html` (it lets plain `go build` and `go test ./...` work without a frontend build); never overwrite it with build output.
- `mise check:quiet` runs every check, skips Firefox e2e (CI runs it), and serializes across worktrees via `flock` (`brew install flock` on macOS). Use it instead of `mise check`.
- `app/types/generated/` is gitignored output of `mise tygo`; change the Go structs, never the generated files.

While iterating, run only the checks for what you touched. Separate mise tasks with `:::` (`mise lint test` passes `test` to the linter and silently skips the tests):

- Go → `mise run lint ::: test`
- Frontend → `mise run lint:js ::: test:unit` (includes tygo, eslint, prettier, tsc, and the SDK build)
- Scripts under `scripts/` → `mise test:scripts`
- Migrations → also `mise db:rollback && mise db:migrate`
- App E2E flows you touched → `mise e2e:chromium`; documentation theme flows → `mise e2e:docs`

Run `mise check:quiet` once when the work is done, before pushing or opening a PR.

**`package.json` dependency split is for Docker, not Node semantics**: `dependencies` holds everything `pnpm build` needs (React, UI libs, vite, typescript, `@types/*`), `devDependencies` only test and lint tools, so the Dockerfile can `pnpm install --prod`. Place new packages by that rule.

## Architecture

Go (Echo, Bun ORM, SQLite) backend in `pkg/`; React 19 + TypeScript + Tailwind + Tanstack Query + Vite frontend in `app/`; mise for tools and tasks, Air for hot reload. Development data lives in `tmp/data.sqlite` and sample files in `tmp/library/`. The production image is a single Go binary that serves `/api` and the embedded SPA; see `docs/agents/docker-and-deploy.md`.

## Docs and config must move with behavior

- **Any user-facing change updates `website/docs/`** in the same change: new or changed features, behavior, config options, API endpoints, or UI. A new or changed API endpoint counts even when its UI ships later. Plans for user-facing changes include a docs task. Common targets: `configuration.md`, `plugins/`, `metadata.md`, `users-and-permissions.md`, `sidecar-files.md`, `supplement-files.md`, `supported-formats.md`. New pages cross-link to related pages and vice versa.
- **A new field in `config.Config`** (`pkg/config/config.go`) also updates `shisho.example.yaml` (field, env var, default, description), `website/docs/configuration.md`, and the Server Settings page in `app/components/pages/AdminSettings.tsx`. The yaml file and the docs page stay complete references. Two exceptions: the test-only `shisho_test_mode` (env `SHISHO_TEST_MODE`) stays out of the yaml and docs (the Server Settings page still shows it); it mounts the unauthenticated `/api/test/*` routes, so its name must stay one no other tool sets (never a generic key like `ENVIRONMENT=test`). The development-only `shisho_cookie_namespace` (env `SHISHO_COOKIE_NAMESPACE`), set by `mise start`, stays out of all three and is tagged `json:"-"`. Validation and env parsing are under "Config" in `pkg/AGENTS.md`.

## Tool versions

`mise.toml` is the source of truth for Go, Node, pnpm, air, tygo, and golangci-lint. When bumping one, also update:

- `Dockerfile`: the `golang:X.X.X-alpine` and `node:X.X.X-alpine` images and the tygo `go install` version (Docker doesn't use mise)
- `package.json`: `@types/node` (then `pnpm install`) and the `packageManager` pnpm version (Docker uses it via corepack)

## Testing

- Go tests get their database from `testdb.New(t)` (`pkg/testutils/testdb`): in-memory, migrated to the latest schema, foreign keys on, one pinned connection, closed at test end. Don't copy a `setupTestDB` into a package. Migration tests that need an older schema, and worker tests that share a cache-mode database across goroutines, open their own.
- Run Go tests with `TZ=America/Chicago CI=true`.
- New Go tests call `t.Parallel()` as their first line, unless they touch shared global state: `pkg/config` tests mutate global config, and `pkg/plugins/AGENTS.md` lists the plugin tests that can't run in parallel.
- Add tests for major functionality such as workers and file parsers; extract complex handler logic and test it separately.
- **Bug fixes and features follow Red-Green-Refactor**, in sequence: write the test and watch it fail, implement until it passes, then clean up with the tests still passing. A test never seen failing proves nothing.

## Git conventions

Commit subjects and PR titles use `[{Category}] {Change description}`. Categories drive the changelog: `[Frontend]`, `[Backend]`, `[Feature]`, `[Feat]` → Features; `[Fix]` → Bug Fixes; `[Docs]`, `[Doc]` → Documentation; `[Test]`, `[E2E]` → Testing; `[CI]`, `[CD]` → CI/CD; anything else → Other.

**Breaking changes carry two markers**: `!` after the category in the PR title (`[Fix]! Replace ENVIRONMENT=test with SHISHO_TEST_MODE`), and a `## BREAKING CHANGES` section in the PR body with one upgrade-note bullet per change. A change is breaking when an operator must act before or after upgrading: a renamed or removed config key or env var, a changed default, a removed route or response field, new startup validation that can refuse an existing config, or a changed on-disk layout. The PR body is the only place to write the notes; `docs/agents/releases.md` covers how they reach the changelog. Reviewers treat a breaking change missing either marker as a review failure.

## Worktrees

Create worktrees in `~/.worktrees/shisho/` and run `mise setup` in each new one:
`git worktree add ~/.worktrees/shisho/my-feature -b feature/my-feature && cd ~/.worktrees/shisho/my-feature && mise setup`

## Database

Enforced by tests in `pkg/migrations/schema_invariants_test.go`: every FK has an explicit ON DELETE action (CASCADE for children meaningless without the parent, SET NULL for nullable references that should survive), every FK column leads an index on the child table, table names are plural, and case-insensitive name lookups use `name = ? COLLATE NOCASE` rather than `LOWER(name)` (only the former can use the `(name COLLATE NOCASE, library_id)` unique indexes; keep `library_id = ?` on library-scoped lookups). Migrators are built with `migrations.NewMigrator` (forbidigo enforces it) so a migration is marked applied only after it succeeds.

Not enforced, so read these:

- **The authors/narrators table is `persons`, not `people`**, even though the package is `pkg/people` and the model is `models.Person`.
- **Column `DEFAULT`s never apply when Bun inserts a zero `time.Time` into a field without `nullzero`**: Bun writes `0001-01-01 00:00:00`. Set `CreatedAt`/`UpdatedAt` explicitly on insert (`now := time.Now()`, as in `CreateSeries` in `pkg/series/service.go`) or tag the field `bun:",nullzero,notnull,default:current_timestamp"`. Updates that list `updated_at` in `Column(...)` write whatever the struct holds, so set `UpdatedAt = time.Now()` first.
- **`nullzero` on a non-pointer field writes NULL for its zero value**, which a `NOT NULL` column rejects. Use it only where zero is invalid (IDs, required names, 1-based sort orders), not where zero is real, like `File.FilesizeBytes` for an empty file.
- **SQLite table rebuilds use the helpers in `pkg/migrations/rebuild.go`**: with `PRAGMA foreign_keys=ON`, `DROP TABLE` on a parent cascades deletes into every child, and the pragma is a no-op inside a transaction. `withForeignKeysOff` pins a connection and toggles the pragma outside the transaction; `rebuildTableInTx` copies rows by explicit column list, recreates every index and trigger from `sqlite_master`, and restores the `sqlite_sequence` high-water mark. Finish with `checkForeignKeys` before commit. `20260928110000_rebuild_files_users_library_paths.go` shows the usage; don't copy `recreateTable` from `20260406100000`.
- **Connection pragmas apply per connection**, and `database/sql` replaces a connection whose query is canceled, so `pkg/sqliteconn.NewConnector` runs `foreign_keys` and `busy_timeout` on every new connection. `database.New` and `testdb.New(t)` use it; a test that opens its own database must too (or pin one connection and enable the pragma).
- **CASCADE does not clean up FTS indexes or the links that say which rows copied a deleted entity.** Collect affected ids with `searchService.CollectAffected` before the delete and `defer searchService.ReindexAffected` after it. FTS rows are keyed by `rowid` equal to the entity id; see `docs/agents/backend/search-fts.md`.
- Index the columns in deletion `WHERE` clauses, and order composite index columns to match the query.

## Agent skills

- **Issue tracker**: GitHub Issues on `shishobooks/shisho`. See `docs/agents/issue-tracker.md`.
- **Triage labels**: default vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.
- **Domain docs**: single-context layout. See `docs/agents/domain.md`.
- **Repo skills** in `.claude/skills/`: `favicon` (favicons, app and PWA icons), `splash` (README splash image), `metadata-field` (adding, removing, or significantly changing a metadata field on books or files).
