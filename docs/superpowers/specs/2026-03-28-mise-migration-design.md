# Migrate from Make/Hivemind/nodenv/goenv to mise

**Date:** 2026-03-28
**Scope:** Dev and CI tooling only. Dockerfile unchanged.

## Summary

Consolidate five separate tools (Make, Hivemind, concurrently, nodenv, goenv) into mise for version management and task running. Air stays as the Go hot-reload tool, invoked through mise. This is a big-bang migration — one PR replaces everything.

## Decisions

- **mise for dev and CI only** — Dockerfile untouched, `docs.yml` workflow untouched
- **Keep air** — purpose-built Go hot-reload with graceful shutdown and port management that `mise watch` can't replicate
- **All tasks inline in `.mise.toml`** — no file-based tasks in `.mise/tasks/`
- **`mise <task>` syntax** — not `mise run <task>`
- **Exact version pinning** — every tool pinned to a specific version, no ranges
- **`ubi` backend for Go CLI tools** — pre-built binaries, fast installs, no compilation
- **Big-bang migration** — Makefile deleted, not kept alongside

## `.mise.toml` — Tools

Single source of truth for all tool versions. Replaces `.node-version`, `.go-version`, Makefile install logic, and CI version declarations.

```toml
[tools]
go = "1.25.5"
node = "24.13.0"
"npm:pnpm" = "10.33.0"
"ubi:air-verse/air" = "1.64.2"
"ubi:gzuidhof/tygo" = "0.2.20"
"ubi:golangci/golangci-lint" = "2.7.2"
```

The `packageManager` field stays in `package.json` because the Dockerfile uses corepack. It won't conflict — mise's pnpm takes precedence on PATH in dev.

## `.mise.toml` — Tasks

### Build

```toml
[tasks.build]
description = "Build production API binary"
run = """
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
CGO_ENABLED=0 go build -o ./build/api/api -installsuffix cgo \
  -ldflags "-w -s -X github.com/shishobooks/shisho/pkg/version.Version=$VERSION" \
  ./cmd/api
"""

[tasks."build:air"]
description = "Build development API binary (used by air)"
run = "go build -o ./build/api/api-air ./cmd/api"
```

### Dev server

Replaces Hivemind + Procfile. mise runs `depends` in parallel by default. `start:web` polls for the API port before launching Vite.

```toml
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
```

### Type generation

Uses mise's `sources`/`outputs` for incremental builds — skips generation if outputs are newer than sources.

```toml
[tasks.tygo]
description = "Generate TypeScript types from Go structs"
run = "tygo generate"
sources = ["tygo.yaml", "pkg/**/validators.go", "pkg/**/config.go", "pkg/**/model.go", "pkg/mediafile/mediafile.go"]
outputs = ["app/types/generated/**/*.ts"]
```

### Linting and testing

`lint:js` and `test:js` use mise sub-tasks for parallelism (replacing concurrently). Both depend on `tygo` since type checking and tests require generated TypeScript types.

```toml
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
```

### Check tasks

`check` uses mise's native parallel `depends`. `check:quiet` keeps custom shell logic for the "suppress output on success" behavior that mise can't express natively.

```toml
[tasks.check]
description = "Run all validation checks"
depends = ["lint", "lint:js", "test", "test:js"]

[tasks."check:quiet"]
description = "Run all checks, suppress output on success"
depends = ["tygo"]
run = """
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
"""
```

### Database

```toml
[tasks."db:migrate"]
description = "Run all pending migrations"
env = { CONFIG_FILE = "./shisho.dev.yaml" }
run = """
go run ./cmd/migrations init
go run ./cmd/migrations migrate
"""

[tasks."db:migrate:create"]
description = "Create a new migration"
env = { CONFIG_FILE = "./shisho.dev.yaml" }
run = "go run ./cmd/migrations create {{arg(i=0, name='name')}}"

[tasks."db:rollback"]
description = "Rollback last migration"
env = { CONFIG_FILE = "./shisho.dev.yaml" }
run = "go run ./cmd/migrations rollback"
```

Usage: `mise db:migrate:create add_column`.

### Other

```toml
[tasks.docker]
description = "Build Docker image"
run = "docker build -t shisho:${DOCKER_TAG:-latest} ."

[tasks.setup]
description = "Install dependencies (run after mise install)"
depends = ["tygo"]
run = """
pnpm install
cd website && pnpm install
"""

[tasks.release]
description = "Create a release"
run = "./scripts/release.sh {{arg(i=0, name='tag')}} {{flag(name='dry-run')}}"
```

Usage: `mise release 0.2.0` or `mise release 0.2.0 --dry-run`.

## `.air.toml` changes

Decouple from Make. Since air and tygo are on PATH via mise, delegate back to mise tasks:

```diff
- cmd = "make build:air"
+ cmd = "mise build:air"

- pre_cmd = ["make tygo"]
+ pre_cmd = ["mise tygo"]
```

Everything else in `.air.toml` stays the same.

## `package.json` changes

Remove `concurrently` from `devDependencies`. Scripts become simple leaf commands — all orchestration and parallelism moves to mise:

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

The `lint`, `test`, and `test:e2e` orchestration scripts are removed — use `mise lint:js`, `mise test:js` instead. Leaf commands (`lint:eslint`, `test:unit`, `e2e:chromium`, etc.) stay for mise to call. The `concurrently` package is removed from `devDependencies`.

## CI: `.github/workflows/ci.yml`

Replace `setup-go`, `setup-node`, `pnpm/action-setup`, `yq`, and `golangci-lint-action` with `jdx/mise-action@v2`. Tool versions come from `.mise.toml`.

**Tradeoff:** Losing `golangci-lint-action`'s inline PR annotations. Lint errors still show in the job log.

### lint job

```yaml
lint:
  name: Go Lint
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: jdx/mise-action@v2
    - run: mise lint
```

### test job

```yaml
test:
  name: Go Test
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: jdx/mise-action@v2
    - run: mise test:race
```

### lint-js job

```yaml
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
```

### test-js job

```yaml
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

## CI: `.github/workflows/release.yml`

Only the `release` job and `npm` job change. Docker jobs and manifest job are untouched.

### release job

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

### npm job

Keep `setup-node` here — it handles the OIDC-based npm provenance flow that mise can't replicate. Use mise only for Go tooling (tygo).

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

Note: `setup-node` is kept specifically for OIDC npm provenance. `pnpm/action-setup` is also kept since `setup-node` expects it for caching. `mise-action` is still used for Go + tygo. The `node-version-file` points to `.mise.toml` — `setup-node` v4 supports reading versions from mise config files, keeping `.mise.toml` as the single source of truth.

## CI: `.github/workflows/docs.yml`

**No changes.** This workflow only builds the Docusaurus site with pnpm. Adding mise for just Node + pnpm adds overhead for no benefit.

## Files deleted

| File | Replaced by |
|------|-------------|
| `Makefile` | `.mise.toml` tasks |
| `Procfile` | `mise start` task with depends |
| `.node-version` | `.mise.toml` `[tools]` |
| `.go-version` | `.mise.toml` `[tools]` |

## Files modified

| File | Change |
|------|--------|
| `.air.toml` | `cmd` and `pre_cmd` use `mise` instead of `make` |
| `package.json` | Remove `concurrently`, simplify `lint`/`test`/`test:e2e` scripts |
| `.github/workflows/ci.yml` | Replace setup actions with `jdx/mise-action`, `make` → `mise` |
| `.github/workflows/release.yml` | Update `release` and `npm` jobs |
| `go.mod` / `go.sum` | Remove `github.com/DarthSim/hivemind` dependency |
| `CLAUDE.md` | Update development commands, worktree setup, Node.js version locations |

## Files unchanged

| File | Why |
|------|-----|
| `Dockerfile` | Dev/CI only migration |
| `.github/workflows/docs.yml` | No benefit from mise |
| `tygo.yaml` | Config file, not a runner |
| `.golangci.yml` | Config file, not a runner |
| `.prettierrc` | Config file, not a runner |
| `shisho.example.yaml` | App config, unrelated |

## CLAUDE.md updates

The "Development Commands" section needs a full rewrite. All `make` references become `mise`. The "Worktree Setup" section changes from `make setup` to `mise install && mise setup`. The "Node.js Version" section drops `.node-version` and `.go-version`, adds `.mise.toml`.

## Command mapping reference

| Before | After |
|--------|-------|
| `make setup` | `mise install && mise setup` |
| `make start` | `mise start` |
| `make start:air` | `mise start:air` |
| `make build` | `mise build` |
| `make test` | `mise test` |
| `make test:race` | `mise test:race` |
| `make lint` | `mise lint` |
| `make lint:js` | `mise lint:js` |
| `make test:js` | `mise test:js` |
| `make check` | `mise check` |
| `make check:quiet` | `mise check:quiet` |
| `make tygo` | `mise tygo` |
| `make db:migrate` | `mise db:migrate` |
| `make db:rollback` | `mise db:rollback` |
| `make db:migrate:create name=X` | `mise db:migrate:create X` |
| `make docker` | `mise docker` |
| `make release tag=X` | `mise release X` |
| `make docs` | `mise docs` |
