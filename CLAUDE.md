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
- **Backend details**: `.claude/skills/backend.md`
- **Frontend details**: `.claude/skills/frontend.md`

## Development Workflow

- Use `make start` to run both API and frontend in development (this also runs `make tygo` automatically)
- Database is SQLite file at `tmp/data.sqlite`
- Sample library files in `tmp/library/` for testing
- All Go files are formatted with `goimports` so all changes should continue that formatting
- Always run `make check` before committing
- If a piece of code that is documented in `docs/` gets updated, the corresponding doc file should be updated as well

## Testing Strategy

- Go tests use standard testing package with testify assertions
- Tests should use `TZ=America/Chicago CI=true` environment
- Frontend uses the same linting rules as backend for consistency
- Database migrations tested via `make db:rollback && make db:migrate`
- Tests should be added for any major pieces of functionality like workers or file parsers. If handler logic is also complex, it should be extracted out and tested separately.
- Whenever fixing a bug, test-driven development should be employed: write a test for the bug, confirm that it fails, fix the bug, and confirm that it passes.

## Git Conventions

- Each commit should be in the format of `[{Category}] {Change description}`
- Always develop in a git worktree and the squash the changes back into master
- This repo currently doesn't utilize pull requests, so instead of creating a PR, it should create a squash commit back into master

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
