# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Backend (Go)
- `make build` - Build production API binary
- `make start` - Start development environment with Hivemind (both API and web)
- `make start:air` - Start API with hot reload via Air
- `make lint` - Run Go linting with golangci-lint
- `make test` - Run Go tests with race detection and coverage
- `TZ=America/Chicago ENVIRONMENT=test CI=true go test -race ./pkg/... -coverprofile coverage.out` - Run all tests

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
- `make tygo` - Generate TypeScript types from Go structs
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
- Individual file covers: `{filename}_cover.{ext}`
- Canonical covers: `cover.{ext}` (book priority) or `audiobook_cover.{ext}` (fallback)
- Book model has `ResolveCoverImage()` method that finds covers dynamically
- API endpoints: `/books/{id}/cover` (canonical) and `/files/{id}/cover` (individual)

**Data Source Priority System**:
- Metadata sources ranked: Manual > EPUB > M4B > Filepath
- Lower priority number = higher precedence
- Used to determine which metadata to keep when conflicts occur

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

- Use `make start` to run both API and frontend in development
- Database is SQLite file at `tmp/data.sqlite`
- Sample library files in `tmp/library/` for testing
- Run `make tygo` after changing Go structs to update TypeScript types
- Always run `make lint` and `yarn lint` before committing

### Testing Strategy

- Go tests use standard testing package with testify assertions
- Tests should use `TZ=America/Chicago ENVIRONMENT=test CI=true` environment
- Frontend uses the same linting rules as backend for consistency
- Database migrations tested via `make db:rollback && make db:migrate`
- To generate types, run `make tygo`. If it says that ``make: Nothing to be done for `tygo'.``, that means the types are already up-to-date.
- Add shadcn components using `npx shadcn@latest add`.
