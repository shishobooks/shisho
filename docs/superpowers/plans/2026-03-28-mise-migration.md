# Mise Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Make, Hivemind, concurrently, nodenv, and goenv with mise for version management and task running.

**Architecture:** Single `.mise.toml` file defines all tool versions (Go, Node, pnpm, air, tygo, golangci-lint) and all dev/CI tasks. Air stays for Go hot-reload, invoked through mise. CI uses `jdx/mise-action` except the npm publish job which keeps `setup-node` for OIDC provenance.

**Tech Stack:** mise, TOML config, GitHub Actions, bash

**Spec:** `docs/superpowers/specs/2026-03-28-mise-migration-design.md`

---

### Task 1: Create `.mise.toml`

The foundation — all tool versions and task definitions in one file.

**Files:**
- Create: `.mise.toml`

- [ ] **Step 1: Create `.mise.toml` with tools and all tasks**

**Important:** All multiline `run` values use TOML literal strings (`'''`) — not basic strings (`"""`) — because shell scripts contain `\033` ANSI escapes and `\` line continuations that TOML basic strings would incorrectly process.

```toml
[tools]
go = "1.25.5"
node = "24.13.0"
"npm:pnpm" = "10.33.0"
"ubi:air-verse/air" = "1.64.2"
"ubi:gzuidhof/tygo" = "0.2.20"
"ubi:golangci/golangci-lint" = "2.7.2"

# === Build ===

[tasks.build]
description = "Build production API binary"
run = '''
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
CGO_ENABLED=0 go build -o ./build/api/api -installsuffix cgo \
  -ldflags "-w -s -X github.com/shishobooks/shisho/pkg/version.Version=$VERSION" \
  ./cmd/api
'''

[tasks."build:air"]
description = "Build development API binary (used by air)"
run = "go build -o ./build/api/api-air ./cmd/api"

# === Dev Server ===

[tasks.start]
description = "Start development environment (API + web)"
depends = ["start:air", "start:web"]

[tasks."start:air"]
description = "Start API with hot reload via Air"
run = "air"

[tasks."start:web"]
description = "Start Vite dev server (waits for API)"
run = "until nc -z localhost 3689 2>/dev/null; do sleep 0.5; done && pnpm start"

[tasks."start:api"]
description = "Start API directly (no hot reload)"
run = "go run ./cmd/api"

[tasks.docs]
description = "Start documentation dev server"
run = "cd website && pnpm start"

# === Type Generation ===

[tasks.tygo]
description = "Generate TypeScript types from Go structs"
run = "tygo generate"
sources = ["tygo.yaml", "pkg/**/validators.go", "pkg/**/config.go", "pkg/**/model.go", "pkg/mediafile/mediafile.go"]
outputs = ["app/types/generated/**/*.ts"]

# === Linting ===

[tasks.lint]
description = "Run Go linting"
run = "golangci-lint run"

[tasks."lint:js"]
description = "Run all JS/TS linting"
depends = ["tygo", "lint:eslint", "lint:prettier", "lint:types"]

[tasks."lint:eslint"]
description = "Run ESLint"
run = "pnpm lint:eslint"

[tasks."lint:prettier"]
description = "Run Prettier check"
run = "pnpm lint:prettier"

[tasks."lint:types"]
description = "Run TypeScript type checking"
depends = ["tygo"]
run = "pnpm lint:types"

# === Testing ===

[tasks.test]
description = "Run Go tests with coverage"
env = { TZ = "America/Chicago", CI = "true" }
run = "go test ./pkg/... -coverprofile coverage.out"

[tasks."test:race"]
description = "Run Go tests with race detection"
env = { TZ = "America/Chicago", CI = "true" }
run = "go test -race ./pkg/... -coverprofile coverage.out"

[tasks."test:js"]
description = "Run all JS tests"
depends = ["tygo", "test:unit", "test:e2e"]

[tasks."test:unit"]
description = "Run JS unit tests"
run = "pnpm test:unit"

[tasks."test:e2e"]
description = "Run E2E tests"
depends = ["e2e:chromium", "e2e:firefox"]

[tasks."e2e:chromium"]
description = "Run E2E tests in Chromium"
run = "mkdir -p ./tmp && pnpm e2e:chromium"

[tasks."e2e:firefox"]
description = "Run E2E tests in Firefox"
run = "mkdir -p ./tmp && pnpm e2e:firefox"

[tasks."test:cover"]
description = "Open coverage report in browser"
run = "go tool cover -html=coverage.out"

# === Check ===

[tasks.check]
description = "Run all validation checks"
depends = ["lint", "lint:js", "test", "test:js"]

[tasks."check:quiet"]
description = "Run all checks, suppress output on success"
depends = ["tygo"]
run = '''
tmpdir=$(mktemp -d)
for step in lint test test:js lint:js; do
  ( mise "$step" >"$tmpdir/$step.out" 2>&1; echo $? >"$tmpdir/$step.rc" ) &
done
wait
failed=0
for step in lint test test:js lint:js; do
  rc=$(cat "$tmpdir/$step.rc")
  if [ "$rc" -eq 0 ]; then
    printf '  \033[32mPASS\033[0m  %s\n' "$step"
  else
    printf '  \033[31mFAIL\033[0m  %s\n' "$step"
    cat "$tmpdir/$step.out"
    failed=1
  fi
done
rm -rf "$tmpdir"
if [ "$failed" -eq 1 ]; then
  printf '\n\033[31mcheck:quiet FAILED\033[0m\n'; exit 1
else
  printf '\n\033[32mcheck:quiet PASSED\033[0m\n'
fi
'''

# === Database ===

[tasks."db:migrate"]
description = "Run all pending migrations"
env = { CONFIG_FILE = "./shisho.dev.yaml" }
run = '''
go run ./cmd/migrations init
go run ./cmd/migrations migrate
'''

[tasks."db:migrate:create"]
description = "Create a new migration"
env = { CONFIG_FILE = "./shisho.dev.yaml" }
run = "go run ./cmd/migrations create {{arg(i=0, name='name')}}"

[tasks."db:rollback"]
description = "Rollback last migration"
env = { CONFIG_FILE = "./shisho.dev.yaml" }
run = "go run ./cmd/migrations rollback"

# === Other ===

[tasks.docker]
description = "Build Docker image"
run = "docker build -t shisho:${DOCKER_TAG:-latest} ."

[tasks.setup]
description = "Install dependencies (run after mise install)"
depends = ["tygo"]
run = '''
pnpm install
cd website && pnpm install
'''

[tasks.release]
description = "Create a release"
run = "./scripts/release.sh {{arg(i=0, name='tag')}} {{flag(name='dry-run')}}"
```

- [ ] **Step 2: Verify mise installs all tools**

Run: `mise install`
Expected: All tools download successfully (Go, Node, pnpm, air, tygo, golangci-lint). No compilation — all are pre-built binaries via ubi backend.

- [ ] **Step 3: Verify tools are on PATH**

Run: `mise exec -- go version && mise exec -- node --version && mise exec -- pnpm --version && mise exec -- air -v && mise exec -- tygo --version && mise exec -- golangci-lint version`
Expected: Each prints its version matching the pinned versions in `.mise.toml`.

- [ ] **Step 4: Commit**

```bash
git add .mise.toml
git commit -m "[CI] Add .mise.toml with tool versions and task definitions"
```

---

### Task 2: Update `.air.toml` to use mise

Decouple air from Make so it delegates to mise tasks instead.

**Files:**
- Modify: `.air.toml:8` (cmd line)
- Modify: `.air.toml:24` (pre_cmd line)

- [ ] **Step 1: Update build command**

In `.air.toml`, change line 8:

```
# old
cmd = "make build:air"

# new
cmd = "mise build:air"
```

- [ ] **Step 2: Update pre-command**

In `.air.toml`, change line 24:

```
# old
pre_cmd = ["make tygo"]

# new
pre_cmd = ["mise tygo"]
```

- [ ] **Step 3: Commit**

```bash
git add .air.toml
git commit -m "[CI] Update air config to use mise instead of make"
```

---

### Task 3: Update `package.json`

Remove concurrently and simplify scripts to leaf commands only — mise handles orchestration.

**Files:**
- Modify: `package.json` (scripts section, devDependencies)

- [ ] **Step 1: Replace scripts section**

In `package.json`, replace the entire `"scripts"` block with:

```json
"scripts": {
  "build": "tsc -b && vite build",
  "lint:eslint": "eslint --max-warnings 0 .",
  "lint:prettier": "prettier --check .",
  "lint:types": "tsc -b --noEmit && tsc -p tsconfig.test.json --noEmit && pnpm -C website typecheck",
  "preview": "vite preview",
  "start": "vite",
  "test:unit": "vitest run",
  "e2e:chromium": "playwright test --project=chromium",
  "e2e:firefox": "playwright test --project=firefox"
}
```

Removed scripts: `lint` (orchestration), `test` (orchestration), `test:e2e` (orchestration). These are now `mise lint:js`, `mise test:js`, `mise test:e2e`.

- [ ] **Step 2: Remove concurrently from devDependencies**

In `package.json`, remove the `"concurrently": "^9.1.2"` line from `devDependencies`.

- [ ] **Step 3: Run pnpm install to update lockfile**

Run: `pnpm install`
Expected: Lockfile updates, concurrently is removed from node_modules.

- [ ] **Step 4: Commit**

```bash
git add package.json pnpm-lock.yaml
git commit -m "[CI] Remove concurrently, simplify package.json scripts for mise"
```

---

### Task 4: Remove hivemind from Go dependencies

Hivemind is replaced by mise's parallel task execution.

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Remove hivemind from go.mod and tidy**

Run:
```bash
go mod edit -droprequire github.com/DarthSim/hivemind
go mod tidy
```

- [ ] **Step 2: Verify hivemind is gone**

Run: `grep hivemind go.mod`
Expected: No output (hivemind no longer in go.mod).

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "[CI] Remove hivemind dependency from go.mod"
```

---

### Task 5: Delete replaced files

Remove Makefile, Procfile, and version files that mise replaces.

**Files:**
- Delete: `Makefile`
- Delete: `Procfile`
- Delete: `.node-version`
- Delete: `.go-version`

- [ ] **Step 1: Delete all four files**

Run:
```bash
git rm Makefile Procfile .node-version .go-version
```

- [ ] **Step 2: Commit**

```bash
git commit -m "[CI] Remove Makefile, Procfile, .node-version, .go-version (replaced by mise)"
```

---

### Task 6: Verify local dev tasks work

Run key mise tasks to confirm the migration works before touching CI.

- [ ] **Step 1: Verify type generation**

Run: `mise tygo`
Expected: TypeScript types generated in `app/types/generated/`. No errors. Running a second time should skip (sources/outputs caching).

- [ ] **Step 2: Verify Go linting**

Run: `mise lint`
Expected: golangci-lint runs successfully with no errors.

- [ ] **Step 3: Verify Go tests**

Run: `mise test`
Expected: All Go tests pass with coverage output.

- [ ] **Step 4: Verify JS linting**

Run: `mise lint:js`
Expected: ESLint, Prettier, and TypeScript checks all pass. tygo runs first as a dependency.

- [ ] **Step 5: Verify JS unit tests**

Run: `mise test:unit`
Expected: Vitest unit tests pass.

- [ ] **Step 6: Verify build**

Run: `mise build`
Expected: Production binary built at `./build/api/api`.

- [ ] **Step 7: Verify check:quiet**

Run: `mise check:quiet`
Expected: All four steps (lint, test, test:js, lint:js) show PASS with green checkmarks, ending with "check:quiet PASSED".

---

### Task 7: Update CI workflow — `ci.yml`

Replace all setup actions with `jdx/mise-action@v2`.

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Rewrite ci.yml**

Replace the entire contents of `.github/workflows/ci.yml` with:

```yaml
name: CI

on:
  push:
    branches: [master]
  pull_request:
    branches: [master]
  workflow_call:

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}

jobs:
  lint:
    name: Go Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: jdx/mise-action@v2
      - run: mise lint

  test:
    name: Go Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: jdx/mise-action@v2
      - run: mise test:race

  lint-js:
    name: JS Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: jdx/mise-action@v2
      - run: pnpm install --frozen-lockfile
      - run: cd website && pnpm install --frozen-lockfile
      - run: mise tygo
      - run: mise lint:js

  test-js:
    name: JS Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: jdx/mise-action@v2
      - run: pnpm install --frozen-lockfile

      - name: Get Playwright version
        id: playwright-version
        run: echo "version=$(npx playwright --version | awk '{print $NF}')" >> $GITHUB_OUTPUT

      - name: Cache Playwright browsers
        id: playwright-cache
        uses: actions/cache@v4
        with:
          path: ~/.cache/ms-playwright
          key: playwright-${{ runner.os }}-${{ steps.playwright-version.outputs.version }}

      - name: Install Playwright browsers
        if: steps.playwright-cache.outputs.cache-hit != 'true'
        run: npx playwright install --with-deps chromium firefox

      - name: Install Playwright system dependencies
        if: steps.playwright-cache.outputs.cache-hit == 'true'
        run: npx playwright install-deps chromium firefox

      - run: go mod download
      - run: mise tygo
      - run: pnpm test:unit
      - run: mise build
      - run: pnpm test:e2e
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "[CI] Migrate ci.yml from setup-go/setup-node to mise-action"
```

---

### Task 8: Update CI workflow — `release.yml`

Update the `release` and `npm` jobs. Docker jobs stay untouched.

**Files:**
- Modify: `.github/workflows/release.yml`

- [ ] **Step 1: Update the `release` job**

In `.github/workflows/release.yml`, replace the `release` job (the `GitHub Release` job, not the whole file) with:

```yaml
  release:
    name: GitHub Release
    needs: validate
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: jdx/mise-action@v2
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 2: Update the `npm` job**

In `.github/workflows/release.yml`, replace the `npm` job with:

```yaml
  npm:
    name: npm Publish
    needs: validate
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: jdx/mise-action@v2

      - name: Install pnpm
        uses: pnpm/action-setup@v4

      - name: Set up Node.js
        uses: actions/setup-node@v4
        with:
          node-version-file: ".mise.toml"
          cache: "pnpm"
          registry-url: "https://registry.npmjs.org"

      - run: pnpm install --frozen-lockfile
      - run: mise tygo

      - name: Extract version from tag
        id: version
        run: echo "VERSION=${GITHUB_REF_NAME#v}" >> $GITHUB_OUTPUT

      - name: Update package version
        working-directory: ./packages/plugin-types
        run: npm version ${{ steps.version.outputs.VERSION }} --no-git-tag-version --allow-same-version

      - name: Publish to npm
        working-directory: ./packages/plugin-types
        run: npm publish --access public
```

Note: `setup-node` is kept here specifically for OIDC npm provenance. `pnpm/action-setup` is kept because `setup-node` needs it for caching. `mise-action` provides Go for tygo. `node-version-file: ".mise.toml"` reads the Node version from mise's config.

- [ ] **Step 3: Verify no other jobs reference make**

Run: `grep -n "make " .github/workflows/release.yml`
Expected: No output — all `make` references have been replaced.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "[CI] Migrate release.yml to use mise-action"
```

---

### Task 9: Update `CLAUDE.md`

Update all sections that reference make, hivemind, version files, or the old workflow.

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update the "Important Notes" tygo section**

Replace the paragraph starting with `**When \`make tygo\` says`:

```markdown
**When `mise tygo` prints "skipping, outputs are up-to-date", this is NORMAL.** It means the generated types are already up-to-date (mise checks source/output timestamps). Do not treat this as an error. The user often has `mise start` running in another session which runs tygo automatically via air, but you should still run `mise tygo` yourself (especially in worktrees where `mise start` may not be running).
```

- [ ] **Step 2: Update "Development Commands" section**

Replace the entire "Development Commands" section (from `## Development Commands` through the end of the "Type Generation" subsection) with:

```markdown
## Development Commands

### Setup
- `mise install` - Install all tools (Go, Node, pnpm, air, tygo, golangci-lint)
- `mise setup` - Install JS dependencies and generate types (run after `mise install`)

### Dev Server
- `mise start` - Start development environment (API with hot reload + Vite frontend)
- `mise start:air` - Start API with hot reload via Air only
- `mise start:api` - Start API directly (no hot reload)
- `mise docs` - Start documentation dev server

### Build
- `mise build` - Build production API binary

### Linting
- `mise lint` - Run Go linting with golangci-lint
- `mise lint:js` - Run all JS/TS linting (ESLint, Prettier, TypeScript) in parallel
- `mise check` - Run all validation checks in parallel (tests, Go lint, JS lint)
- `mise check:quiet` - Same as check but suppresses output on success, only shows failures

### Testing
- `mise test` - Run all Go tests with coverage
- `mise test:race` - Run all Go tests with race detection and coverage (used in CI)
- `mise test:js` - Run all JS tests (unit + E2E) in parallel
- `mise test:unit` - Run JS unit tests only
- `mise test:e2e` - Run E2E tests (Chromium + Firefox) in parallel

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
```

- [ ] **Step 3: Update "Dependency Structure" note**

Keep the existing `**Dependency Structure:**` paragraph as-is — it still applies. It should now appear right after the "Frontend (leaf commands)" subsection.

- [ ] **Step 4: Update "Architecture Overview" stack**

In the `### Stack` section, replace:

```markdown
- **Development**: Air for Go hot reload, Hivemind for process management
```

with:

```markdown
- **Development**: mise for tool/version management and task running, Air for Go hot reload
```

- [ ] **Step 5: Update "Development Workflow" section**

Replace these lines:
- `Use \`make start\`` → `Use \`mise start\``
- `this also runs \`make tygo\` automatically` → `air runs \`mise tygo\` automatically before each rebuild`
- `Always run \`make check:quiet\`` → `Always run \`mise check:quiet\``
- `Use \`make check\` only when` → `Use \`mise check\` only when`
- `\`make check:quiet\` gives` → `\`mise check:quiet\` gives`

- [ ] **Step 6: Update "Node.js Version" section**

Replace the entire "Node.js Version" section with:

```markdown
## Tool Versions

All tool versions are managed by mise via `.mise.toml`. When updating versions, update these locations:

- `.mise.toml` - Single source of truth for Go, Node, pnpm, air, tygo, golangci-lint
- `Dockerfile` - The `golang:X.X.X-alpine` and `node:X.X.X-alpine` images (Docker doesn't use mise)
- `package.json` - `@types/node` version (run `pnpm install` after)
- `package.json` - `packageManager` field for pnpm (used by Docker via corepack)
```

- [ ] **Step 7: Update "Releases" subsection**

Replace:

```markdown
- Use `make release tag=0.2.0` to create a release
```

with:

```markdown
- Use `mise release 0.2.0` to create a release
```

- [ ] **Step 8: Update "Worktree Setup" section**

Replace the entire "Worktree Setup" section with:

```markdown
## Worktree Setup

- Worktrees should be created in `~/.worktrees/shisho/`
- After creating a new worktree, run `mise install && mise setup` to install tools and dependencies
- Example: `git worktree add ~/.worktrees/shisho/my-feature -b feature/my-feature && cd ~/.worktrees/shisho/my-feature && mise install && mise setup`
```

- [ ] **Step 9: Update "Testing Strategy" database migrations line**

Replace:

```markdown
- Database migrations tested via `make db:rollback && make db:migrate`
```

with:

```markdown
- Database migrations tested via `mise db:rollback && mise db:migrate`
```

- [ ] **Step 10: Commit**

```bash
git add CLAUDE.md
git commit -m "[Docs] Update CLAUDE.md for mise migration"
```

---

### Task 10: Final verification

Run the full check suite to confirm everything works end-to-end.

- [ ] **Step 1: Run full check**

Run: `mise check:quiet`
Expected: All four steps pass:
```
  PASS  lint
  PASS  test
  PASS  test:js
  PASS  lint:js

check:quiet PASSED
```

- [ ] **Step 2: Verify no make references remain in tracked files**

Run: `grep -rn "make " CLAUDE.md .github/workflows/ .air.toml package.json | grep -v "node_modules" | grep -v ".git"`
Expected: No references to `make ` commands (there may be references to "make" as a word in documentation context, which is fine — look for actual command invocations like `make build`, `make test`, etc.).

- [ ] **Step 3: Verify no stale version file references**

Run: `grep -rn "\.node-version\|\.go-version\|nodenv\|goenv" CLAUDE.md .github/workflows/ | grep -v ".git"`
Expected: No references to deleted version files or replaced version managers.
