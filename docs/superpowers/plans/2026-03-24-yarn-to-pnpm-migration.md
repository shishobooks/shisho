# Yarn to pnpm Migration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Yarn 1 (Classic) with pnpm as the package manager across the entire project — root app, website, CI, Docker, and all scripts.

**Architecture:** The project has three independent `package.json` files (root, `website/`, `packages/plugin-types/`). They are NOT formal workspaces — each is managed independently. The migration converts each `yarn.lock` → `pnpm-lock.yaml`, updates all scripts/config that reference yarn, and adds a `.npmrc` for pnpm settings. `resolutions` in `package.json` become `pnpm.overrides`.

**Tech Stack:** pnpm 10, Node.js 24, GitHub Actions with `pnpm/action-setup`

---

### Task 1: Generate pnpm lock files and configure pnpm

**Files:**
- Create: `.npmrc`
- Create: `pnpm-lock.yaml` (generated)
- Create: `website/pnpm-lock.yaml` (generated)
- Delete: `yarn.lock`
- Delete: `website/yarn.lock`
- Modify: `package.json` (root)
- Modify: `website/package.json`

- [ ] **Step 1: Install pnpm globally (if not already available)**

```bash
corepack enable && corepack prepare pnpm@latest --activate
```

Verify: `pnpm --version` outputs 10.x.

- [ ] **Step 2: Create `.npmrc` in project root**

```ini
# Use a flat node_modules structure for compatibility
shamefully-hoist=true
# Extended timeout for Docker arm64 builds which run through slow QEMU emulation
fetch-timeout=600000
```

`shamefully-hoist=true` gives a flat `node_modules` like yarn/npm, preventing breakage from phantom dependencies. This is the conservative choice — it can be removed later once compatibility is confirmed. `fetch-timeout=600000` replaces the old `yarn config set network-timeout 600000` needed for arm64 Docker builds via QEMU.

- [ ] **Step 3: Update `packageManager` field in root `package.json`**

Change line 7 from:
```json
"packageManager": "yarn@1.22.22+sha1.ac34549e6aa8e7ead463a7407e1c7390f61a6610"
```
to the pnpm equivalent. **IMPORTANT:** Do NOT use a hardcoded hash. Run this command to get the exact value:
```bash
corepack prepare pnpm@latest --activate && corepack info pnpm
```
Or simply set `"packageManager": "pnpm@10.12.1"` (without hash) — corepack will still work, it just won't verify integrity. If you want the hash, copy it from the corepack output verbatim.

- [ ] **Step 4: Run `pnpm import` BEFORE modifying resolutions**

`pnpm import` reads the existing `yarn.lock` to generate `pnpm-lock.yaml`. Run it while `resolutions` is still in the original format so yarn.lock can be read correctly:

```bash
rm -rf node_modules
pnpm import
```

Do the same for website:
```bash
cd website && rm -rf node_modules && pnpm import && cd ..
```

- [ ] **Step 5: Convert `resolutions` to `pnpm.overrides` in root `package.json`**

Replace:
```json
"resolutions": {
  "minimatch": ">=3.1.4",
  "rollup": ">=4.59.0",
  "ajv": "~6.14.0"
}
```

With:
```json
"pnpm": {
  "overrides": {
    "minimatch": ">=3.1.4",
    "rollup": ">=4.59.0",
    "ajv": "~6.14.0"
  }
}
```

- [ ] **Step 6: Convert `resolutions` to `pnpm.overrides` in `website/package.json`**

Replace:
```json
"resolutions": {
  "minimatch": ">=3.1.4",
  "qs": ">=6.14.2",
  "svgo": ">=3.3.3",
  "serialize-javascript": ">=7.0.3"
}
```

With:
```json
"pnpm": {
  "overrides": {
    "minimatch": ">=3.1.4",
    "qs": ">=6.14.2",
    "svgo": ">=3.3.3",
    "serialize-javascript": ">=7.0.3"
  }
}
```

- [ ] **Step 7: Install dependencies with pnpm**

```bash
pnpm install                              # root
cd website && pnpm install && cd ..       # website
```

- [ ] **Step 8: Delete old yarn.lock files**

```bash
rm yarn.lock website/yarn.lock
```

- [ ] **Step 9: Verify both installs work cleanly**

```bash
pnpm install          # root — should be a no-op
cd website && pnpm install && cd ..
```

Expected: clean installs with no errors.

- [ ] **Step 10: Commit**

```bash
git add .npmrc pnpm-lock.yaml website/pnpm-lock.yaml package.json website/package.json
git rm yarn.lock website/yarn.lock
git commit -m "[Backend] Migrate from Yarn to pnpm: lock files and config"
```

---

### Task 2: Update package.json scripts

**Files:**
- Modify: `package.json` (root — lines 10-22)

The `concurrently` package (v9) supports `pnpm:` prefix for script grouping. The `yarn --cwd` syntax becomes `pnpm -C` (or `pnpm --dir`).

- [ ] **Step 1: Update scripts in root `package.json`**

Change the scripts section from:
```json
"scripts": {
  "build": "tsc -b && vite build",
  "lint": "concurrently --kill-others-on-fail --group \"yarn:lint:*\"",
  "lint:eslint": "eslint --max-warnings 0 .",
  "lint:prettier": "prettier --check .",
  "lint:types": "tsc -b --noEmit && tsc -p tsconfig.test.json --noEmit && yarn --cwd website typecheck",
  "preview": "vite preview",
  "start": "vite",
  "test": "concurrently --kill-others-on-fail --group \"yarn:test:*\"",
  "test:unit": "vitest run",
  "test:e2e": "mkdir -p ./tmp && concurrently --kill-others-on-fail --group \"yarn:e2e:*\"",
  "e2e:chromium": "playwright test --project=chromium",
  "e2e:firefox": "playwright test --project=firefox"
}
```

To:
```json
"scripts": {
  "build": "tsc -b && vite build",
  "lint": "concurrently --kill-others-on-fail --group \"pnpm:lint:*\"",
  "lint:eslint": "eslint --max-warnings 0 .",
  "lint:prettier": "prettier --check .",
  "lint:types": "tsc -b --noEmit && tsc -p tsconfig.test.json --noEmit && pnpm -C website typecheck",
  "preview": "vite preview",
  "start": "vite",
  "test": "concurrently --kill-others-on-fail --group \"pnpm:test:*\"",
  "test:unit": "vitest run",
  "test:e2e": "mkdir -p ./tmp && concurrently --kill-others-on-fail --group \"pnpm:e2e:*\"",
  "e2e:chromium": "playwright test --project=chromium",
  "e2e:firefox": "playwright test --project=firefox"
}
```

Changes:
- `yarn:lint:*` → `pnpm:lint:*` (3 places)
- `yarn:test:*` → `pnpm:test:*`
- `yarn:e2e:*` → `pnpm:e2e:*`
- `yarn --cwd website typecheck` → `pnpm -C website typecheck`

- [ ] **Step 2: Verify scripts work**

```bash
pnpm lint:eslint   # quick smoke test for a single script
pnpm lint:types    # tests the -C website cross-project invocation
```

Expected: both pass (or show only pre-existing lint issues, not pnpm errors).

- [ ] **Step 3: Commit**

```bash
git add package.json
git commit -m "[Backend] Update package.json scripts from yarn to pnpm"
```

---

### Task 3: Update Makefile

**Files:**
- Modify: `Makefile` (lines 82-83, 86-87, 102-103, 111)

- [ ] **Step 1: Replace yarn references in Makefile**

Line 83: `yarn lint` → `pnpm lint`
Line 87: `yarn test` → `pnpm test`
Line 102: `yarn` → `pnpm install`
Line 103: `cd website && yarn` → `cd website && pnpm install`
Line 111: `cd website && yarn start` → `cd website && pnpm start`

Updated targets:
```makefile
lint\:js:
	pnpm lint

test\:js:
	pnpm test

setup: $(BUILD_DIR)/golangci-lint tygo
	pnpm install
	cd website && pnpm install

docs:
	cd website && pnpm start
```

- [ ] **Step 2: Verify Makefile targets**

```bash
make lint:js    # should run pnpm lint
make test:js    # should run pnpm test
```

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "[Backend] Update Makefile from yarn to pnpm"
```

---

### Task 4: Update Dockerfile

**Files:**
- Modify: `Dockerfile` (lines 34-36, 47)

- [ ] **Step 1: Update frontend-builder stage in Dockerfile**

Replace lines 32-36:
```dockerfile
# Install production dependencies only (build tools, not test/lint tools)
# Note: Extended timeout for arm64 builds which run through slow QEMU emulation
COPY package.json yarn.lock ./
RUN yarn config set network-timeout 600000 && \
    yarn install --production --frozen-lockfile
```

With:
```dockerfile
# Install pnpm via corepack (reads version from packageManager field in package.json)
COPY package.json pnpm-lock.yaml .npmrc ./
RUN corepack enable && corepack install

# Install production dependencies only (build tools, not test/lint tools)
# Note: fetch-timeout for arm64/QEMU is configured in .npmrc
RUN pnpm install --prod --frozen-lockfile
```

Note: `corepack install` reads the `packageManager` field from the already-copied `package.json`, ensuring the exact pinned pnpm version is used (reproducible builds). The fetch timeout for slow arm64/QEMU builds is handled in `.npmrc` (`fetch-timeout=600000`).

Replace line 47:
```dockerfile
RUN NODE_ENV=production yarn build
```

With:
```dockerfile
RUN NODE_ENV=production pnpm build
```

- [ ] **Step 2: Test Docker build locally**

```bash
docker build -t shisho:pnpm-test .
```

Expected: successful build. The frontend-builder stage should install deps and build without errors.

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "[Backend] Update Dockerfile from yarn to pnpm"
```

---

### Task 5: Update CI workflows

**Files:**
- Modify: `.github/workflows/ci.yml` (lines 60, 66, 70, 76, 93, 99, 127, 133)
- Modify: `.github/workflows/release.yml` (lines 156, 163)
- Modify: `.github/workflows/docs.yml` (lines 33-34, 38, 44)

GitHub Actions `setup-node` supports `cache: "pnpm"` but requires pnpm to be installed first. Use `pnpm/action-setup` before `setup-node`.

- [ ] **Step 1: Update `.github/workflows/ci.yml`**

In the `lint-js` job, add a pnpm install step before `setup-node` and update references:

```yaml
  lint-js:
    name: JS Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.25"

      - name: Install pnpm
        uses: pnpm/action-setup@v4

      - name: Set up Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "24"
          cache: "pnpm"

      - name: Install yq
        uses: mikefarah/yq@v4

      - name: Install dependencies
        run: pnpm install --frozen-lockfile

      - name: Install docs dependencies
        working-directory: ./website
        run: pnpm install --frozen-lockfile

      - name: Generate TypeScript types
        run: make tygo

      - name: Run lint
        run: pnpm lint
```

In the `test-js` job, same pattern:

```yaml
  test-js:
    name: JS Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.25"

      - name: Install pnpm
        uses: pnpm/action-setup@v4

      - name: Set up Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "24"
          cache: "pnpm"

      - name: Install yq
        uses: mikefarah/yq@v4

      - name: Install dependencies
        run: pnpm install --frozen-lockfile

      # ... playwright steps stay the same (they use npx) ...

      - name: Run unit tests
        run: pnpm test:unit

      - name: Run E2E tests
        run: pnpm test:e2e
```

Note: `pnpm/action-setup@v4` automatically reads the `packageManager` field from `package.json` to determine the pnpm version, so no `version` input is needed.

- [ ] **Step 2: Update `.github/workflows/release.yml`**

In the `npm` job, add pnpm setup and update references:

```yaml
      - name: Install pnpm
        uses: pnpm/action-setup@v4

      - name: Set up Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "24"
          cache: "pnpm"
          registry-url: "https://registry.npmjs.org"

      - name: Install dependencies
        run: pnpm install --frozen-lockfile
```

- [ ] **Step 3: Update `.github/workflows/docs.yml`**

```yaml
      - name: Install pnpm
        uses: pnpm/action-setup@v4

      - name: Set up Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "24"
          cache: "pnpm"
          cache-dependency-path: website/pnpm-lock.yaml

      - name: Install dependencies
        working-directory: ./website
        run: pnpm install --frozen-lockfile

      - name: Build
        working-directory: ./website
        env:
          GITHUB_ACTIONS: "true"
        run: pnpm build
```

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml .github/workflows/release.yml .github/workflows/docs.yml
git commit -m "[CI] Update GitHub Actions workflows from yarn to pnpm"
```

---

### Task 6: Update Procfile, playwright config, release script, and remaining files

**Files:**
- Modify: `Procfile` (line 2)
- Modify: `playwright.config.ts` (lines 119, 137)
- Modify: `scripts/release.sh` (lines 197-198)
- Modify: `.claude/settings.json` (line 10 — hook command)
- Modify: `.cursor/worktrees.json` (line 3)
- Modify: `website/README.md` (lines 9-10, 17-18, 27)
- Modify: `e2e/setup.spec.ts` (lines 8-9 — JSDoc comments)
- Modify: `e2e/login.spec.ts` (lines 7-8 — JSDoc comments)
- Modify: `e2e/ereader.spec.ts` (lines 8-9 — JSDoc comments)

- [ ] **Step 1: Update Procfile**

Change line 2 from:
```
web: until nc -z localhost 3689 2>/dev/null; do sleep 0.5; done && yarn start
```

To:
```
web: until nc -z localhost 3689 2>/dev/null; do sleep 0.5; done && pnpm start
```

- [ ] **Step 2: Update playwright.config.ts**

Change line 119 comment from:
```typescript
// (yarn test:e2e runs chromium and firefox in parallel, both try to start all servers)
```

To:
```typescript
// (pnpm test:e2e runs chromium and firefox in parallel, both try to start all servers)
```

Change line 137 from:
```typescript
command: `yarn start --port ${config.frontendPort}`,
```

To:
```typescript
command: `pnpm start --port ${config.frontendPort}`,
```

- [ ] **Step 3: Update scripts/release.sh**

Change lines 197-198 from:
```bash
yarn install --frozen-lockfile
yarn docs:version "$VERSION"
```

To:
```bash
pnpm install --frozen-lockfile
pnpm docs:version "$VERSION"
```

- [ ] **Step 4: Update `.claude/settings.json`**

Change the hook command (line 10) from:
```
yarn prettier --write "$file_path"
```
To:
```
pnpm exec prettier --write "$file_path"
```

Use `pnpm exec` (not just `pnpm prettier`) because this is a direct binary invocation in a hook, not a package.json script.

- [ ] **Step 5: Update `.cursor/worktrees.json`**

Change line 3 from `"yarn"` to `"pnpm install"`.

- [ ] **Step 6: Update `website/README.md`**

Replace all yarn references:
- `yarn install` → `pnpm install`
- `yarn start` → `pnpm start`
- `yarn build` → `pnpm build`
- `yarn serve` → `pnpm serve`
- `yarn docs:version` → `pnpm docs:version`

- [ ] **Step 7: Update E2E test JSDoc comments**

In `e2e/setup.spec.ts`, `e2e/login.spec.ts`, and `e2e/ereader.spec.ts`, replace `yarn test:e2e` with `pnpm test:e2e` in the JSDoc comment blocks.

- [ ] **Step 8: Commit**

```bash
git add Procfile playwright.config.ts scripts/release.sh .claude/settings.json .cursor/worktrees.json website/README.md e2e/setup.spec.ts e2e/login.spec.ts e2e/ereader.spec.ts
git commit -m "[Backend] Update Procfile, Playwright config, release script, and remaining files from yarn to pnpm"
```

---

### Task 7: Update .gitignore

**Files:**
- Modify: `.gitignore` (lines 182-183, 250, 301-306)

- [ ] **Step 1: Update .gitignore**

The existing yarn-related entries can stay (harmless). But we should ensure pnpm-specific patterns are present. The `.pnpm-debug.log*` entry already exists on line 185.

No changes strictly needed. The existing `.gitignore` already has:
- `.pnpm-debug.log*` (line 185)
- `node_modules/` (line 219)

The yarn entries (`yarn-debug.log*`, `yarn-error.log*`, `.yarn-integrity`, `.yarn/*`, `.pnp.*`) are harmless to leave — they prevent accidental commits if someone still has yarn artifacts locally.

- [ ] **Step 2: Skip commit — no changes needed**

---

### Task 8: Update documentation (CLAUDE.md files)

**Files:**
- Modify: `CLAUDE.md` (root — lines 98-110, 155-159)
- Modify: `website/CLAUDE.md` (lines 14, 94, 96-97, 107, 136)
- Modify: `app/CLAUDE.md` (lines 456-459)
- Modify: `e2e/CLAUDE.md` (lines 25, 182-186)

- [ ] **Step 1: Update root `CLAUDE.md`**

Replace all `yarn` command references with `pnpm`:

Frontend commands section (~lines 98-110):
```markdown
- `pnpm start` - Start Vite dev server
- `pnpm build` - Build production frontend
- `pnpm lint` - Run ESLint, TypeScript checks, and Prettier
- `pnpm lint:eslint` - ESLint only
- `pnpm lint:types` - TypeScript type checking only
- `pnpm lint:prettier` - Prettier formatting check only
```

Dependency structure section: Replace references to `yarn build`, `yarn install --production` with `pnpm build`, `pnpm install --prod`.

Node.js version section (~line 159): Replace `run `yarn install` after` with `run `pnpm install` after`.

- [ ] **Step 2: Update `website/CLAUDE.md`**

Replace all `yarn` references with `pnpm`:
- `yarn build` → `pnpm build`
- `cd website && yarn` → `cd website && pnpm install`
- `yarn docs:version` → `pnpm docs:version`
- `yarn --cwd website typecheck` → `pnpm -C website typecheck`
- `yarn install --frozen-lockfile` → `pnpm install --frozen-lockfile`
- `cache: "yarn"` → `cache: "pnpm"`
- `cache-dependency-path: website/yarn.lock` → `cache-dependency-path: website/pnpm-lock.yaml`

- [ ] **Step 3: Update `app/CLAUDE.md`**

Replace test command references (~lines 456-459):
```markdown
pnpm test           # Run all tests (unit + E2E via concurrently)
pnpm test:unit      # Run Vitest unit/component tests with coverage
pnpm test:e2e       # Run Playwright E2E tests
make test:js        # Run tests via Makefile (used in `make check`)
```

- [ ] **Step 4: Update `e2e/CLAUDE.md`**

Replace yarn references:
- `yarn test:e2e` → `pnpm test:e2e`
- `yarn test:e2e:chromium` → `pnpm test:e2e:chromium` (note: this script doesn't exist, but update the doc reference)
- Any other `yarn` references → `pnpm`

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md website/CLAUDE.md app/CLAUDE.md e2e/CLAUDE.md
git commit -m "[Docs] Update CLAUDE.md files from yarn to pnpm"
```

---

### Task 9: Smoke test the full build

This is the final verification pass. No code changes — just running existing checks.

- [ ] **Step 1: Run full check suite**

```bash
make check:quiet
```

Expected: all steps PASS (lint, test, test:js, lint:js).

- [ ] **Step 2: Verify `make start` works**

```bash
make start
```

Expected: both API and frontend dev servers start. Frontend served via `pnpm start` (visible in Hivemind output).

- [ ] **Step 3: Verify `make docs` works**

```bash
make docs
```

Expected: Docusaurus dev server starts.

- [ ] **Step 4: Verify Docker build works**

```bash
make docker
```

Expected: successful multi-stage Docker build using pnpm.
