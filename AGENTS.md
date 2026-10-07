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
| `pkg/plugins/AGENTS.md` | Plugin system: Goja runtime, hooks, host APIs, manifests |
| `pkg/epub/AGENTS.md` | EPUB format: parsing and generation |
| `pkg/cbz/AGENTS.md` | CBZ format: ComicInfo.xml, page order |
| `pkg/covers/AGENTS.md` | Cover serving and the shared thumbnail cache |
| `pkg/kepub/AGENTS.md` | KePub conversion from EPUB and CBZ |
| `pkg/mp4/AGENTS.md` | M4B format: reading, rewriting, chapters |
| `pkg/pdf/AGENTS.md` | PDF format: pdfcpu and PDFium, page numbering |
| `pkg/events/AGENTS.md` | Server-sent events broker and stream |
| `website/AGENTS.md` | Docs site: Docusaurus, versioning, deployment |
| `e2e/AGENTS.md` | E2E testing: Playwright, per-browser isolation, fixtures |
| `tools/gotestsplit/AGENTS.md` | Timing-aware Go test sharding |

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
- **Cover, page, download, and stream URLs come from helpers in `app/utils`**: image endpoints are cached as immutable and rely on a `?v=` key. ESLint rejects literal URLs; "Image URLs" in `app/AGENTS.md` explains the keys.
- **The plugin SDK in `packages/plugin-sdk/` must stay in sync with Go plugin types without breaking changes.** `pkg/plugins/sdk_sync_test.go` checks most of it; the rest is under "Plugin SDK" in `pkg/plugins/AGENTS.md`.

## Development commands

All tasks are in `mise.toml` (`mise tasks` lists them). The ones with non-obvious behavior:

- `mise setup` installs tools and dependencies and generates types. Run it after creating a worktree.
- `mise start` runs the API with hot reload plus Vite. Several worktrees can run it at once, so each server may land on a different port than usual: read the URL it prints.
- `mise build` is the only way to refresh the frontend embedded in the binary; `pnpm build` alone does not. Never overwrite the tracked `pkg/frontend/dist/placeholder.html` with build output: it lets plain `go build` and `go test ./...` work without a frontend build.
- `mise check:quiet` runs every check except Firefox e2e (CI runs it) and serializes across worktrees via `flock` (`brew install flock` on macOS). Use it instead of `mise check`.
- `app/types/generated/` is gitignored output of `mise tygo`; change the Go structs, never the generated files.

While iterating, run only the checks for what you touched. Separate mise tasks with `:::` (`mise lint test` passes `test` to the linter and silently skips the tests):

- Go → `mise run lint ::: test`
- Frontend → `mise run lint:js ::: test:unit`
- Scripts under `scripts/` → `mise test:scripts`
- Migrations → also `mise db:rollback && mise db:migrate`
- App E2E flows you touched → `mise e2e:chromium`; documentation theme flows → `mise e2e:docs`

Run `mise check:quiet` once when the work is done, before pushing or opening a PR.

**`package.json` dependency split is for Docker, not Node semantics**: `dependencies` holds everything `pnpm build` needs, including build tools and `@types/*`; `devDependencies` holds only test and lint tools, so the Dockerfile can `pnpm install --prod`. Place new packages by that rule.

## Architecture

Go (Echo, Bun ORM, SQLite) backend in `pkg/`; React, TypeScript, Tailwind, Tanstack Query, and Vite frontend in `app/`; mise for tools and tasks, Air for hot reload. The production image is a single Go binary that serves `/api` and the embedded SPA; see `docs/agents/docker-and-deploy.md`.

## Docs and config must move with behavior

- **`website/docs/` holds only what an operator of Shisho needs to know**: how to deploy, configure, use, troubleshoot, or extend it through the supported plugin contract. A change that alters what an operator sees or does updates the page that owns that fact, in the same change, and plans for such changes include a docs task. How the feature works inside (endpoints the UI calls, the data model, package layout, internal behavior) stays out, however large the change. Read `website/AGENTS.md` before editing the site; its inclusion test decides what belongs.
- **A new field in `config.Config`** (`pkg/config/config.go`) also updates `shisho.example.yaml` (field, env var, default, description), `website/docs/configuration.md`, and the Server Settings page in `app/components/pages/AdminSettings.tsx`. The yaml file and the docs page stay complete references; the few exempt fields say so in their comments. Validation and env parsing are under "Config" in `pkg/AGENTS.md`.

## Tool versions

`mise.toml` is the source of truth for tool versions, but Docker does not use mise, so a bump also updates the copies in the `Dockerfile` and `package.json`; `mise lint` (`tools/checkversions`) names any that disagree.

## Testing

- Go tests get their database from `testdb.New(t)` (`pkg/testutils/testdb`); don't copy a `setupTestDB` into a package. Migration tests that need an older schema, and worker tests that share a cache-mode database across goroutines, open their own.
- Run Go tests through `mise test`, or set `TZ=America/Chicago CI=true` as it does when calling `go test` directly.
- New Go tests call `t.Parallel()` as their first line, unless they touch shared global state: `pkg/config` tests mutate global config, and `pkg/plugins/AGENTS.md` lists the plugin tests that can't run in parallel.
- Add tests for major functionality such as workers and file parsers; extract complex handler logic and test it separately.
- **Bug fixes and features follow Red-Green-Refactor**, in sequence: write the test and watch it fail, implement until it passes, then clean up with the tests still passing. A test never seen failing proves nothing.

## Git conventions

Commit subjects and PR titles use `[{Category}] {Change description}`. Categories drive the changelog: `[Frontend]`, `[Backend]`, `[Feature]`, `[Feat]` → Features; `[Fix]` → Bug Fixes; `[Docs]`, `[Doc]` → Documentation; `[Test]`, `[E2E]` → Testing; `[CI]`, `[CD]` → CI/CD; anything else → Other.

**Watch CI by run id** (`gh run view $RUN_ID --json jobs`), not by PR: `gh pr checks` follows the latest commit, and `gh run view` reports a run as in progress until the whole workflow finishes.

**Breaking changes carry two markers**: `!` after the category in the PR title (`[Fix]! Replace ENVIRONMENT=test with SHISHO_TEST_MODE`), and a `## BREAKING CHANGES` section in the PR body with one upgrade-note bullet per change. A change is breaking when an operator must act before or after upgrading: a renamed or removed config key or env var, a changed default, a removed route or response field, new startup validation that can refuse an existing config, or a changed on-disk layout. The PR body is the only place to write the notes; `docs/agents/releases.md` covers how they reach the changelog. Reviewers treat a breaking change missing either marker as a review failure.

## Database

Schema rules (foreign key actions and indexes, table naming, case-insensitive lookups) are enforced by `pkg/migrations/schema_invariants_test.go`, whose failures say what to do.

Not enforced, so read these:

- **The authors/narrators table is `persons`, not `people`**, even though the package is `pkg/people` and the model is `models.Person`.
- **Column `DEFAULT`s never apply when Bun inserts a zero `time.Time` into a field without `nullzero`**: Bun writes `0001-01-01 00:00:00`. Set `CreatedAt`/`UpdatedAt` explicitly on insert or tag the field `bun:",nullzero,notnull,default:current_timestamp"`. Updates that list `updated_at` in `Column(...)` write whatever the struct holds, so set `UpdatedAt = time.Now()` first.
- **`nullzero` on a non-pointer field writes NULL for its zero value**, which a `NOT NULL` column rejects. Use it only where zero is invalid (IDs, required names, 1-based sort orders), not where zero is real, like the size of an empty file.
- **SQLite table rebuilds use the helpers in `pkg/migrations/rebuild.go`**, never a hand-rolled copy like the `recreateTable` inside an older migration: with `PRAGMA foreign_keys=ON`, `DROP TABLE` on a parent cascades deletes into every child, and the pragma is a no-op inside a transaction.
- **Connection pragmas apply per connection**, and `database/sql` replaces a connection whose query is canceled, so a test that opens its own database uses `pkg/sqliteconn.NewConnector` as `database.New` and `testdb.New(t)` do (or pins one connection and enables the pragma).
- **CASCADE does not clean up FTS indexes or the links that say which rows copied a deleted entity.** Collect affected ids with `searchService.CollectAffected` before the delete and `defer searchService.ReindexAffected` after it; see `docs/agents/backend/search-fts.md`.
- Index the columns in deletion `WHERE` clauses, and order composite index columns to match the query.

## Agent skills

- **Issue tracker**: GitHub Issues on `shishobooks/shisho`. See `docs/agents/issue-tracker.md`.
- **Triage labels**: see `docs/agents/triage-labels.md`.
- **Domain docs**: single-context layout. See `docs/agents/domain.md`.
- **Repo skills** in `.claude/skills/`: `favicon` (favicons, app and PWA icons), `splash` (README splash image), `metadata-field` (adding, removing, or significantly changing a metadata field on books or files).
