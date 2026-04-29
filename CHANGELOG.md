# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.0.41] - 2026-04-28

### Features
- Convert filter sheet genre/tags to combobox with shared chip (#175)
- Auto-classify supplement-named PDFs on scan (#168)
- Make author names clickable in gallery cards (#169)
- Add dev-only quick admin button on setup page (#170)
- Add Filter label to filter button (#167)

### Bug Fixes
- Hide identify cover picker when plugin cover_url is broken or still loading (#176)
- Write genres and tags to book sidecar (#174)
- Index series/persons/genres/tags after identify-apply, resync, and monitor batches (#173)
- Promote root-level files into book folder during organize (#171)

### CI/CD
- Timing-aware Go test sharding (gotestsplit + 12 shards) (#172)


## [0.0.40] - 2026-04-26

### Features
- Hard-delete series + close FTS leak in orphan cleanup (#166)
- Consolidate cover selection and serving into pkg/covers (#164)
- Bring identify review form to parity with edit forms (#158)
- Consolidate book actions into the ... menu (#161)
- Chapter filename parsing for CBZ files (#157)

### Bug Fixes
- Bake selected file identity into book cover ETag (#165)
- Clean up book_series rows when soft-deleting a series (#163)
- Accept HEAD on OPDS download routes (#159)
- Serve OPDS cover images from the OPDS group (#160)

### Other
- Coordinate concurrent mise check:quiet across worktrees (#162)
- Bump the minor-and-patch group with 12 updates (#149)
- Bump github.com/pdfcpu/pdfcpu from 0.11.1 to 0.12.0 in the minor-and-patch group (#148)


## [0.0.39] - 2026-04-25

### Features
- Per-file Reviewed flag with admin-configurable criteria (#156)
- Remove back button from reader UI (#155)
- Enforce file identifier type-uniqueness across the stack (#153)
- Per-user gallery cover size (#152)
- Cache successful Basic Auth results to speed up OPDS (#150)

### Bug Fixes
- Encode non-ASCII filenames in Content-Disposition per RFC 5987 (#151)

### Testing
- Pin mold dive requirement for slice-of-struct payloads (#154)


## [0.0.38] - 2026-04-24

### Features
- Cache management admin page (#146)
- Compare cover pages by number in identify review (#141)

### Bug Fixes
- Discard stale sidecars on rescan so replaced files re-extract metadata (#147)
- Trust upstream proxy X-Forwarded-Proto in Caddy (#145)
- Reset cover error flag so failed thumbnails can recover (#143)
- Use ETag for series cover revalidation (#144)
- Coerce library ID to string in setQueryData cache writes (#142)


## [0.0.37] - 2026-04-23

### Features
- Harden Kobo sync flow and bust device cover cache (#139)
- Split pkg/plugins/handler.go into per-operation files (#138)
- Surface plugin load errors in the UI (#137)
- Remove unused search query scaffolding types (#134)
- Replace cover URL cache-busting with HTTP revalidation (#132)
- Add delete library action to admin settings (#133)
- Sync file.Name on manual book title edit (#130)
- Add Extract Subtitle affordance to book edit forms (#131)

### Bug Fixes
- Sort CBZ pages naturally across all numeric runs in kepub (#140)
- Serve covers and persist metadata for root-level library files (#135)
- Skip placeholder data when accumulating directory picker entries (#136)


## [0.0.36] - 2026-04-22

### Bug Fixes
- Stub foliate pdf.js so vite build resolves (#129)


## [0.0.35] - 2026-04-22

### Features
- In-app EPUB reader (#128)
- Add shisho.yaml host API and scoped filePath access for enrichers (#127)


## [0.0.34] - 2026-04-22

### Features
- Sync file name with book title on identify, align scan/identify consistency (#126)
- Fix M4B title showing as 'Unknown' in audiobook players (#124)

### Bug Fixes
- Refresh plugin update availability on startup and repo sync, and surface updates on Discover (#125)


## [0.0.33] - 2026-04-21

### Features
- Fetch M4B chapters from Audible via Audnexus (#123)
- Remove in-app back buttons (#122)
- Unify admin and library sidebars and top nav (#121)
- Let plugin logos fill their rounded tile (#120)
- Paginate eReader AuthorBooks, SeriesBooks, and LibrarySearch (#116)
- Initialize ListIdentifierTypes slice with make for JSON safety (#113)
- Sort relative imports last via prettier (#111)

### Bug Fixes
- Select newest plugin update via semver comparison (#118)
- Show "Author not found" in eReader AuthorBooks 404 (#117)
- Library-scope RetrievePerson in eReader AuthorBooks (#114)

### Documentation
- Title-case Gallery Sort and reorder before Advanced (#112)

### Testing
- Backend test infra + E2E specs for plugin install/update/uninstall flows (#119)
- Cover PluginRow stopPropagation on action click (#115)


## [0.0.32] - 2026-04-20

### Features
- Plugin admin UI polish pass (#110)
- Add gallery sort with persisted library preferences (#108)
- Let plugins set cover_page for CBZ/PDF files (#107)
- Add server log viewer with in-memory ring buffer (#102)
- Switch Vite to plugin-react (#101)
- Collapse library filters into a single sheet/drawer button (#99)
- Display series number prominently and polish detail page navigation (#97)

### Bug Fixes
- Align sort default across surfaces and fix per-author pagination (#109)
- Release port 3689 promptly on server shutdown (#103)
- Stabilize flaky JS test runs (#100)
- Prefer existing cover in identify dialog when plugin cover is lower resolution (#98)
- Avoid misleading skipped enricher message in identify dialog (#96)

### Other
- Bump the minor-and-patch group with 8 updates (#106)
- Bump github.com/klippa-app/go-pdfium from 1.18.1 to 1.19.2 in the minor-and-patch group (#104)
- Bump actions/upload-pages-artifact from 4 to 5 (#105)


## [0.0.31] - 2026-04-14

### Features
- Surface plugin search errors in identify dialog (#94)

### Bug Fixes
- Clean up stale book folder after root-level organize (#95)


## [0.0.30] - 2026-04-13

### Features
- Normalize identifier values on write (#93)


## [0.0.29] - 2026-04-13

### Features
- Show author roles in identify dialog (#92)
- Add sha256 file hashing with move/rename detection (#91)
- Polish identify workflow UI (#88)

### Bug Fixes
- Trigger file organization after identify/apply metadata (#90)
- Make reset-to-file-metadata fully wipe non-file metadata (#89)
- Stop release script from dropping oldest commit per release (#87)


## [0.0.28] - 2026-04-11

### CI/CD
- Cache build:sdk mise task with sources/outputs (#86)
- Gate plugin-sdk and docs builds on PRs via mise tasks (#85)


## [0.0.27] - 2026-04-11

### Features
- Wire plugin hook context to vm.Interrupt() for real cancellation (#80)
- Add shisho.sleep() host API for plugin retry backoff (#78)
- Consolidate duplicate edit-mode render blocks in FileChaptersTab (#77)
- Write edited PDF chapters back as bookmarks on download (#69)
- Add language and abridged fields to files (#64)

### Bug Fixes
- Close several cover recovery and display gaps (#84)
- Clean up DB rows when a book directory is removed or renamed (#83)
- Preload supplements into scan cache to avoid UNIQUE rescans (#82)
- Use ResolveCoverDirForWrite in plugin persistMetadata cover write (#81)
- Prefer file.Name over book.Title for PDF info-dict Title (#76)
- Restore Docusaurus 3.10 build and gate it in CI (#74)
- Resolve cover path before deleting on file downgrade (#67)
- Restore PDF cover_page from sidecar on rescan (#68)
- Add language and abridged to plugin SDK MetadataField union (#66)
- Improve identify workflow for audiobooks and multi-file books (#63)
- Fix plugin identifier type conflicts between local and published plugins (#61)
- Surface plugin install errors in the UI (#60)
- Fix flaky check:quiet when subshell is killed before writing exit code (#59)

### Documentation
- Add metadata-field skill for cross-stack field changes (#65)

### CI/CD
- Shard Go tests across 5 parallel jobs with gotesplit (#75)
- Remove Docker ecosystem from Dependabot config (#58)

### Other
- Consolidate file cover path resolution via fileutils.ResolveCoverPath (#79)
- Bump the minor-and-patch group with 17 updates (#72)
- Bump the minor-and-patch group with 5 updates (#71)
- Bump pnpm/action-setup from 5 to 6 (#70)
- Bump vite from 8.0.3 to 8.0.5 (#62)
- Bump lucide-react from 0.563.0 to 1.7.0 (#51)
- Bump typescript from 5.6.3 to 5.9.3 (#50)
- Bump the minor-and-patch group across 1 directory with 12 updates (#47)
- Bump jsdom from 27.4.0 to 29.0.1 (#52)
- Bump vite from 7.3.1 to 8.0.3 (#49)
- Bump the minor-and-patch group across 1 directory with 9 updates (#48)


## [0.0.26] - 2026-04-05

### Features
- Show dynamic capabilities in plugin install dialog (#57)
- Add three-state plugin mode (Enabled / Manual Only / Disabled) (#56)
- Improve plugin settings order tab UX (#55)
- Enforce plugin minShishoVersion with UI compatibility indicators (#54)

### Documentation
- Refine README a bit (#53)

### CI/CD
- Add Dependabot configuration for automated dependency updates (#44)

### Other
- Bump lodash from 4.17.23 to 4.18.1 (#43)


## [0.0.25] - 2026-04-05

### Bug Fixes
- Resolve false "Changed" badges in identify workflow (#42)

### Other
- Bump golang.org/x/image from 0.35.0 to 0.38.0 (#41)


## [0.0.24] - 2026-03-31

### Features
- Change Docker container port from 8080 to 5173 (#40)
- Unified rescan dialog with skip-plugins mode (#39)
- Add enricher cover resolution gate and page-based format protection (#38)

### Bug Fixes
- Remove ajv override that broke docs build after workspace merge (#37)


## [0.0.23] - 2026-03-29

### CI/CD
- Add pnpm workspace config to fix plugin-sdk npm publish (#36)


## [0.0.22] - 2026-03-29

### Features
- Refine plugin system: type unification, HTML API, search context, confidence scores (#35)
- Add PDF cover page selection (#34)
- Refine identify workflow with cover, date, identifier, and UI improvements (#33)

### Bug Fixes
- Fix chapter leaks in DeleteFile/DeleteBook and misleading orphan cleanup logs (#31)

### CI/CD
- Upgrade GitHub Actions to Node.js 24 versions (#32)


## [0.0.21] - 2026-03-28

### Features
- Add in-app PDF viewer with server-side rendering (#27)
- Redesign docs homepage, darken app theme, use Tailwind font-logo class (#28)
- Replace sequential orphan cleanup with batch operations (#25)
- Add interactive identify apply with field-by-field review (#24)
- Migrate from Yarn to pnpm (#22)
- Add bulk download for selected books (#19)
- Improve identify search result row layout and readability (#20)

### Bug Fixes
- Validate and download cover URLs using plugin httpAccess domains (#18)
- Clean up cover and sidecar files when deleting last file in a book (#17)

### Documentation
- Add note about AI usage to README (#30)

### CI/CD
- Update task-start script to use mise instead of make (#29)
- Migrate from Make/Hivemind/concurrently to mise (#26)
- Move race detection to CI-only, speed up local test runs 8x (#23)
- Add concurrency limits to cancel redundant CI runs (#21)


## [0.0.20] - 2026-03-15

### Features
- Replace job status polling with Server-Sent Events (#15)
- Add configurable session duration (#16)
- Add richer search results with new fields, coverUrl, and passthrough pattern (#14)
- Add first-class PDF support with metadata extraction and cover generation (#12)
- Add filesystem watcher with debounced rescans (#11)

### Bug Fixes
- Fix date/time display: UTC for dates, local timezone for timestamps (#13)


## [0.0.19] - 2026-03-10

### Features
- Add plugin search and identify dialog with full metadata enrichment (#10)

### Documentation
- Remove outdated git workflow section from CLAUDE.md (#8)

### CI/CD
- Cache Playwright browsers in CI to speed up JS tests (#9)


## [0.0.18] - 2026-03-09

### Other
- Fix remaining Dependabot alerts in website dependencies (#7)
- Fix flakey E2E tests by isolating Vite cache per browser (#6)
- Fix Dependabot security alerts for npm dependencies (#5)
- Add manual Identify Book dialog (#4)
- Move skills to subdirectory CLAUDE.md files for automatic context loading
- Plugin System v2: UX and architecture revamp (#3)
- Optimize scan performance with mod-time change detection (#2)


## [0.0.17] - 2026-02-07

### Bug Fixes
- Allow plugin-defined identifier types and fix truncated selector


## [0.0.16] - 2026-02-07

### Features
- Give metadata enrichers priority over file-parsed metadata
- Log version on server startup

### Documentation
- Document enricher priority over file metadata
- Fix file parser fields table in plugin development docs


## [0.0.15] - 2026-02-07

### Documentation
- Fix homepage gap and update site config


## [0.0.14] - 2026-02-07

### Bug Fixes
- Exclude versions.json from Prettier checks


## [0.0.13] - 2026-02-07

### Bug Fixes
- Exclude versioned docs from Prettier checks


## [0.0.12] - 2026-02-07

### Features
- Add forced password reset flow for temporary passwords
- Add primary file system for books
- Add delete books and files functionality

### Bug Fixes
- Delete book when scan detects missing main file with only supplements remaining
- Prevent SQLite database lock errors with single connection
- Deduplicate author names in global search results
- Show correct placeholder cover for audiobooks in global search
- Use correct Docker tag format in release notes

### Documentation
- Add metadata, plugins, users, sidecar, and supplement documentation pages
- Add Kobo sync, eReader browser, and OPDS documentation pages
- Add lists documentation page
- Add Docusaurus documentation website
- Add agent config symlinks and Codex environment setup
- Add subagent skill routing instructions to CLAUDE.md
- Add skills routing and critical gotchas to CLAUDE.md

### Testing
- Suppress Radix UI act() warnings in FileEditDialog tests

### CI/CD
- Reuse CI workflow for release validation


## [0.0.11] - 2026-02-01

### Features
- Add create new list option to select mode toolbar
- Add unsaved changes warning to forms and dialogs
- Parallelize library scan with worker pool and entity caching
- Add ScrollRestoration to scroll to top on navigation
- Add URL utilities and wildcard domain matching to plugin system
- Use replace navigation after merge to avoid 404 on back
- Add numeric HTML entity decoding to description sanitizer
- Add job logging to scan worker functions
- Add cache busting to file cover thumbnails


## [0.0.10] - 2026-01-27

### Features
- Add per-field configuration for metadata enricher plugins
- Compute dev library path dynamically

### Documentation
- Update plugin skill with FFmpeg and shell API changes


## [0.0.9] - 2026-01-27

### Features
- Add FFmpeg probe/version APIs and shell.exec()

### Documentation
- Add plugin API design for FFmpeg enhancements and shell exec


## [0.0.8] - 2026-01-26

### Features
- Add audiobook codec detection for M4B files
- Add book merge/split functionality with file selection mode
- Strip HTML tags from book descriptions
- Add dynamic page titles to all pages
- Add e-reader image optimization to CBZ downloads
- Add keyboard navigation to global search

### Documentation
- Add book merge and split design


## [0.0.7] - 2026-01-25

### Features
- Restore desktop layout for book/file detail pages


## [0.0.6] - 2026-01-25

### Bug Fixes
- Book sidecar written to wrong location for root-level files with OrganizeFileStructure enabled
- Include public directory in Docker build for favicons


## [0.0.5] - 2026-01-25

### Bug Fixes
- Remove 'api' from gitignore
- Skip writing port file when tmp directory doesn't exist
- Increase startup timeout for slow storage (NAS devices)


## [0.0.4] - 2026-01-25

### Bug Fixes
- Reuse existing groups when PGID is already taken in Docker entrypoint


## [0.0.3] - 2026-01-25

### Bug Fixes
- Fix PUID/PGID handling and health check timeout in Docker entrypoint


## [0.0.2] - 2026-01-25

### Features
- Add mobile responsiveness across the application

### Bug Fixes
- Use correct table name 'persons' instead of 'people' in SQL queries

### Documentation
- Improve Docker Compose example in README


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

