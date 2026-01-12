# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## ⚠️ IMPORTANT: Read This First

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

## Architecture Overview

### Stack
- **Backend**: Go with Echo web framework, Bun ORM, SQLite database
- **Frontend**: React 19 with TypeScript, TailwindCSS, Tanstack Query, Vite
- **Development**: Air for Go hot reload, Hivemind for process management

### Backend Architecture

**Entry Point**: `cmd/api/main.go` starts both HTTP server and background worker

**Core Services Pattern**:
- Each domain (books, jobs, libraries) has: `handlers.go`, `routes.go`, `service.go`, `validators.go`
- Services contain business logic and database operations
- Handlers contain HTTP request/response logic
- Routes register HTTP endpoints
- Validators define request/response schemas

**Database Models** (`pkg/models/`):
- Use Bun ORM with struct tags for database mapping
- Models include JSON tags for API serialization
- TypeScript types auto-generated via tygo from Go structs

**Background Worker** (`pkg/worker/`):
- Processes jobs from database queue
- Main job type: scan job that processes ebook/audiobook files
- Extracts metadata from EPUB (via `pkg/epub/`) and M4B files (via `pkg/mp4/`)
- Generates cover images with filename-based storage strategy

**Cover Image System**:
- Individual file covers: `{filename}.cover.{ext}`
- Canonical covers: `cover.{ext}` (book priority) or `audiobook_cover.{ext}` (fallback)
- Book model has `ResolveCoverImage()` method that finds covers dynamically
- API endpoints: `/books/{id}/cover` (canonical) and `/files/{id}/cover` (individual)

**Data Source Priority System**:
- Metadata sources ranked: Manual > EPUB > M4B > Filepath
- Lower priority number = higher precedence
- Used to determine which metadata to keep when conflicts occur

**OPDS**:
- There is an OPDS v1.2 server hosted in the application
- As new functionality is added, it's important to keep the OPDS server up-to-date with the new features

**Authentication**:
- RBAC is used throughout the app
- Authn and authz needs to be considered for all pieces of functionality
- Both frontend and backend checks need to be made so that everything is protected on all fronts

**Config**:
- This is a self-hosted app, so all configs are defined by a config file
- Each config field is also configurable by environment variables as well
- If a new field is added to `config.Config` in `pkg/config/config.go`, `shisho.example.yaml` should also be updated to reflect the new config field

**Sidecars**:
- We keep sidecar metadata files for every file we parse into the system
- We don't want to keep non-modifiable intrinsic properies of the file in the sidecar (e.g. bitrate, duration, etc.)
- Source fields (e.g. title\_source, name\_source, etc.) shouldn't be saved into the sidecar

**Metadata Sync Checklist**:
When adding or modifying book/file metadata fields, ensure these files are updated:
1. **Sidecar types** (`pkg/sidecar/types.go`) - Add field to `BookSidecar` or `FileSidecar` struct for persistence
2. **Sidecar conversion** (`pkg/sidecar/sidecar.go`) - Update `BookSidecarFromModel()` or `FileSidecarFromModel()` to include the new field
3. **Download fingerprint** (`pkg/downloadcache/fingerprint.go`) - Add field to `Fingerprint` struct and `ComputeFingerprint()` so cache invalidates when metadata changes
4. **File parsers** - Update to extract the new field:
   - EPUB: `pkg/epub/opf.go`
   - CBZ: `pkg/cbz/cbz.go`
   - M4B: `pkg/mp4/metadata.go`
5. **File generators** - Update to write the field back:
   - EPUB: `pkg/filegen/epub.go`
   - CBZ: `pkg/filegen/cbz.go`
   - M4B: `pkg/filegen/m4b.go`
   - KePub: `pkg/kepub/cbz.go` (for CBZ-to-KePub conversion)
6. **Scanner** (`pkg/worker/scan.go`) - Handle the new field during scanning
7. **ParsedMetadata** (`pkg/mediafile/mediafile.go`) - Add field if it's parsed from files
8. **API relations** (`pkg/books/service.go`) - If adding a relation to File (like Publisher, Imprint), add `.Relation("Files.NewRelation")` to all book query methods: `RetrieveBook`, `RetrieveBookByFilePath`, and `listBooksWithTotal`
9. **UI display** (`app/components/pages/BookDetail.tsx`) - Display the new field in the book detail view

**Adding New Entity Types** (like Publisher, Imprint, Genre, Tag):
When adding a new entity type that files or books reference:
1. Create model in `pkg/models/` with appropriate fields and Bun struct tags
2. Create service in `pkg/{entity}/service.go` following the pattern from `pkg/genres/service.go`:
   - Include `FindOrCreate{Entity}()` method for scanner to use
   - Include `Retrieve{Entity}()` and `List{Entity}s()` methods
3. Add service to worker (`pkg/worker/worker.go`) and initialize in `New()`
4. Update scanner to use the new service for entity creation

### Frontend Architecture

**React Router** (`app/router.tsx`):
- Single page app with client-side routing
- Main route loads Home page with book gallery

**State Management**:
- Tanstack Query for server state (books, jobs, libraries)
- React Context for theme management
- No global client state management library

**API Integration**:
- `app/libraries/api.ts` contains HTTP client functions
- Query hooks in `app/hooks/queries/` wrap API calls with Tanstack Query
- TypeScript types auto-imported from `app/types/generated/`
- **Default list limit is 50** - All list endpoints have a max limit of 50 items per request
- **Always use server-side search** - Never rely on client-side filtering for searchable lists; always pass search queries to the API. This ensures users can find items beyond the initial 50 loaded.

**UI Components**:
- Custom components in `app/components/` using Radix UI primitives
- Tailwind CSS for styling with dark/light theme support
- Components follow shadcn/ui patterns

### File Processing Flow

1. **Scan Job Creation**: User triggers scan via API
2. **File Discovery**: Worker scans library paths for `.epub`, `.m4b`, `.cbz` files
3. **Metadata Extraction**: Parse files to extract title, authors, cover images
4. **Database Storage**: Create/update Book and File records
5. **Cover Generation**: Save individual covers + generate canonical covers
6. **Priority Resolution**: Use data source priority to resolve metadata conflicts

### Development Workflow

- Use `make start` to run both API and frontend in development (this also runs `make tygo` automatically)
- Database is SQLite file at `tmp/data.sqlite`
- Sample library files in `tmp/library/` for testing
- All Go files are formatted with `goimports` so all changes should continue that formatting
- Always run `make check` before committing
- If a piece of code that is documented in `docs/` gets updated, the corresponding doc file should be updated as well

### Testing Strategy

- Go tests use standard testing package with testify assertions
- Tests should use `TZ=America/Chicago CI=true` environment
- Frontend uses the same linting rules as backend for consistency
- Database migrations tested via `make db:rollback && make db:migrate`
- Add shadcn components using `npx shadcn@latest add`.
- Tests should be added for any major pieces of functionality like workers or file parsers. If handler logic is also complex, it should be extracted out and tested separately.
- Whenever fixing a bug, test-driven development should be employed: write a test for the bug, confirm that it fails, fix the bug, and confirm that it passes.

### Git Conventions

- Each commit should be in the format of `[{Category}] {Change description}`
- Always develop in a git worktree and the squash the changes back into master
- This repo currently doesn't utilize pull requests, so instead of creating a PR, it should create a squash commit back into master

### Worktree Setup

- Worktrees should be created in `~/.worktrees/shisho/`
- After creating a new worktree, run `make setup` to install dependencies and build tools
- Example: `git worktree add ~/.worktrees/shisho/my-feature -b feature/my-feature && cd ~/.worktrees/shisho/my-feature && make setup`
