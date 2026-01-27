# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.0.8] - 2026-01-26

### Features
- Add audiobook codec detection for M4B files
- Add book merge/split functionality with file selection mode
- Strip HTML tags from book descriptions
- Add dynamic page titles to all pages
- Add e-reader image optimization to CBZ downloads
- Add keyboard navigation to global search


## [0.0.7] - 2026-01-25


## [0.0.6] - 2026-01-25

### Bug Fixes
- Book sidecar written to wrong location for root-level files with OrganizeFileStructure enabled


## [0.0.5] - 2026-01-25

### Bug Fixes
- Remove 'api' from gitignore
- Skip writing port file when tmp directory doesn't exist


## [0.0.4] - 2026-01-25


## [0.0.3] - 2026-01-25


## [0.0.2] - 2026-01-25

### Features
- Add mobile responsiveness across the application

### Bug Fixes
- Use correct table name 'persons' instead of 'people' in SQL queries


## [0.0.1] - 2026-01-25

### Features
- Add version tracking and release automation
- Fix UI/UX consistency across list pages and add cursor-pointer
- Add CBZ cover page selection
- Add editor role and improve permission enforcement
- Add bulk selection and list management improvements
- Add cross-library book lists with sharing
- Add chapters frontend and improve sidecar/download efficiency
- Add CBZ reader with page viewing and settings
- Add chapter parsing and storage for CBZ, M4B, and EPUB files
- Add autogenerate sort name UI with live preview
- Add Name field to files for custom titles
- Add favicon, splash image, and shelf icon to logo
- Add rescan metadata on every sync
- Add searchable author/narrator dropdowns
- Add cancel button to downloads
- Add print-mp4-atoms debug script
- Add subtitle parsing for EPUB and M4B formats
- Refresh books list when resync completes on home page
- Add supplementary files support
- Add placeholder covers for books without cover images
- Add file identifier support (ISBN, ASIN, GTIN)

### Bug Fixes
- Use pure bash for changelog insertion in release script
- Release script Bash 3.x compatibility and add dry-run option
- Display file covers in edit dialog when cover_mime_type is null
- Add unmount cleanup for Dialog pointer-events bug
- Prevent sharing lists with users who already have access
- Wait for API before starting Vite in dev
- Fix flaky make check due to test-results directory race
- Handle sort_order and default_sort when switching list ordering
- Merge root-level files with same title/author into single book
- Add cache-busting to cover image URLs

### Documentation
- Add comprehensive README for open-source release
- Create FUNDING.yml
- Add MIME type validation and FFmpeg API to plugin system design
- Resolve contradictions and flesh out plugin system design
- Resolve contradictions and flesh out plugin system design
- Resolve contradictions and flesh out plugin system design
- Add inputConverter hook to plugin system design
- Add plugin system design document
- Add design for CBZ cover page selection feature
- Add remove sidecar data source design
- Add autogenerate sort name design plan
- Add file name field design plan
- Add file UI redesign plan
- Add rescan metadata design
- Correct date for placeholder cover plans
- Add cancellable downloads design and context propagation guidance
- Add author/narrator searchable dropdown design
- Add placeholder covers design document
- Add supplementary files design document
- Add file identifiers design document
- Add resync button design document
- Add job logs design document
- Add metadata fields design
- Add metadata entity editing design
- Add worktree setup instructions to CLAUDE.md

### Testing
- Add retry for flaky tests in CI
- Run tygo before make check
- Add frontend testing infrastructure with Vitest and Playwright

### CI/CD
- Use native arm64 runners for Docker builds
- Upgrade Node.js from 22 to 24 LTS
- Remove author info from GoReleaser changelog
- Optimize Docker build and fix npm publish
- Add release automation with goreleaser and GitHub Actions
- Add GitHub Actions workflow and parallelize Go tests
- Add Playwright browser installation for E2E tests
- Add make install target and use in test-js job
- Update golangci-lint-action to v7 for v2.x support
- Add GitHub Actions workflow with parallel jobs

### Other
- Update Go and JS dependencies
- Add empty directory cleanup and remove canonical cover handling
- Redesign CBZ page picker with large preview and thumbnail strip
- Fix autogenerate sort name not working for persons/series
- Fix series books not refreshing after merge
- Fix chapter reorder not triggering from arrow buttons
- Fix duplicate books when person has multiple roles
- Reorganize Security Settings into separate eReader and Kobo sections
- Add complete plugin system with JavaScript runtime and per-library configuration
- Add wireless book sync for Kobo eReaders
- Pin Go tool versions by removing go get from install step
- Add login redirect to return users to intended destination
- Replace native date input with custom DatePicker component
- Add drag-and-drop reordering for authors, series, and narrators
- Move library navigation from header to collapsible sidebar
- Add Lists option to user dropdown menu
- Add ConfirmDialog component and remove drag handle icon
- Redesign library/list picker with consistent visual language
- Adjust popover padding for consistency
- Add Kobo sync design document
- Disable unnecessary claude plugins
- Restore sidecar data source priority above file metadata
- Add single book/file resync with unified scan architecture
- Add Lists feature design document
- Disable superpowers plugin
- Add file chapters frontend design document
- Add eReader browser support with API key authentication
- Add single book/file resync design document
- Add CBZ viewer design document
- Add eReader browser support design document
- Redesign file display on book detail page
- Remove DataSourceSidecar, unify file metadata priorities
- Add chapter parsing and storage design document
- Add defense-in-depth check to setup endpoint
- Add merge conflict resolution skill
- Ignore .auto-claude directory
- Clarify clean up for squash-merge-worktree skill
- Add Toasters to Login and Setup
- Add squash-merge-worktree skill
- Refine ./scripts/task-start
- Add frontmatter to Claude skills
- Add library-specific resync button to TopNav
- Add job logging system with UI viewer
- Refine skills and slim down CLAUDE.md
- Improve handling of long text in dialogs and page headers
- Add metadata entity editing feature
- Add description to books and URL/publisher/imprint/release_date to files
- Fix gitignore and add pkg/tags
- Support genres and tags for books
- Add Claude plugins
- Add task scripts
- Store page count
- Split fields by ; in addition to ,
- Add support for download kepubs
- Add generated file downloads
- Make downloads more efficient
- Add a loading spinner for file downloads
- Sort by series number when filtering by series
- Fix series dropdown
- Preserve all mp4 tags when generating a download
- Add generated file downloads
- Add epub file generation
- Parse out duration and bitrate
- Sync search input with query param
- Order jobs by create_at desc
- Handle enter press for global search
- Parse series from CBZs if there's a volume number
- Update indexes when nested resources are updated
- Fix series covers
- Implement sort name
- Organize non-root files too
- Don't let data get cleared out
- Fix organization ordering bug
- Add create default library to admin libraries page
- Fix retry logic
- Organize files during a scan
- Add retries for SQLITE_BUSY
- Implement scan job concurrency limit
- Simplify covers and add aspect ratio setting
- Add UI to edit books and files
- Implement global search
- Add ability to customize roles
- Refine navigation
- Fix empty directory bug
- Make light mode better and default to dark mode
- Fix narrator parsing from filename
- Update CONFIG_FILE references in Makefile
- Fix toSnakeCase for acronyms
- Make MP4 parsing more robust
- Handle bare volume numbers
- Implement user authentication
- Allow books to have multiple series
- Support parsing narrators
- Make authors and narrators more robust
- Add support for OPDS v1.2
- Fix empty title bugs
- Add typegen stage
- Make adding a library path easier
- Add Dockerfile
- Pull config from file and envs
- Add extension to cover file names
- Add support for sidecar metadata files
- Allow pre-existing cover images
- Update to v1.25.5
- Fix yarn build
- Refresh the UI
- Make series its own model
- Add test foundation and tests for scan worker
- Implement organize_file_structure
- Add library settings
- Handle root-level books better
- Add support for user config
- Add basic support for series
- Support root-level book files
- Add support for CBZ files
- Add pagination to the book gallery
- Add a book details page
- Add files for Claude
- Have canonical cover images for books
- Update IDs to ints instead of UUIDs
- Don't let a failing audiobook prevent parsing the rest of the files
- Clean up the book gallery UI
- Add foundation of scan worker for epub and mp4

