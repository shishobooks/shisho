# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Important Notes

**When `make tygo` says "Nothing to be done for \`tygo'", this is NORMAL.** It means the generated types are already up-to-date. Do not treat this as an error. Do not try to run `tygo` directly outside of make - always use `make tygo`. The user often has `make start` running in another session which runs tygo automatically, but you should still run `make tygo` yourself (especially in worktrees where `make start` may not be running).

**ALWAYS** use the `AskUserQuestion` tool when asking the user questions, in any context. If you have too many questions for the tool, split them up into multiple calls.

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

For detailed architecture information, see the skills:
- **Backend details**: `.claude/skills/backend/SKILL.md`
- **Frontend details**: `.claude/skills/frontend/SKILL.md`

## Development Workflow

- Use `make start` to run both API and frontend in development (this also runs `make tygo` automatically)
- Database is SQLite file at `tmp/data.sqlite`
- Sample library files in `tmp/library/` for testing
- All Go files are formatted with `goimports` so all changes should continue that formatting
- Always run `make check` before committing
- If a piece of code that is documented in `docs/` gets updated, the corresponding doc file should be updated as well
- **If a new field is added to `pkg/config/config.go`**, `shisho.example.yaml` MUST be updated with the new field (env var, default, description). This file must always be a complete reference of all server config options. Exception: `environment` is test-only and should not be included.

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

### Workflow

- Always develop in a git worktree and squash the changes back into master
- This repo currently doesn't utilize pull requests, so instead of creating a PR, create a squash commit back into master

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

## User Preferences

- **Plan execution:** Always use subagent-driven development (dispatch fresh subagent per task in current session) rather than parallel sessions
