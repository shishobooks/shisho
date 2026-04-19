# Plugins UI Feedback Pass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Polish the redesigned plugins admin UI based on user feedback — tighten row padding, add an official-repo badge, fix a UTC date bug, replace the back button with breadcrumbs, fix a serialization bug that hides plugin logos, and replace the half-built `tag` field with a per-version `releaseUrl` that decouples plugin homepages from release URLs.

**Architecture:** Frontend-heavy work touching `app/components/plugins/` and `app/components/pages/PluginDetail.tsx`. Small backend response-struct change in `pkg/plugins/handler.go`. One schema change in `pkg/plugins/repository.go` (swap `Tag` → `ReleaseURL`) mirrored in TS types. Docs update in `website/docs/plugins/repositories.md`.

**Tech Stack:** Go + Echo + Bun (backend), React 19 + TypeScript + TailwindCSS (frontend), Vitest + RTL (frontend tests), testify (Go tests), lucide-react (icons).

---

## Pre-work: Worktree state check

This plan lives in a worktree with uncommitted changes from a prior iteration (`tag` field on PluginVersion). Several tasks explicitly replace that work. Read the spec at `docs/superpowers/specs/2026-04-19-plugins-ui-feedback-pass-design.md` before starting — it explains why each change exists.

Before starting, verify the worktree matches expectations:

```bash
git status
# Expected: on branch t3code/2b13113f with uncommitted changes in
# app/components/plugins/PluginVersionHistory.tsx, app/components/plugins/PluginVersionHistory.test.tsx,
# app/hooks/queries/plugins.ts, pkg/plugins/repository.go
```

The uncommitted changes add a `tag` field; Task 8 will overwrite them.

---

## Task 1: Backend — add `imageUrl` and `is_official` to available plugin response

**Files:**
- Modify: `pkg/plugins/handler.go:698-758`
- Test: `pkg/plugins/handler_list_available_test.go` (new)

- [ ] **Step 1: Write the failing test**

Create `pkg/plugins/handler_list_available_test.go`:

```go
package plugins

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAvailablePluginResponse_JSONFields locks the JSON wire format for the
// /plugins/available response. The frontend depends on these exact field names.
func TestAvailablePluginResponse_JSONFields(t *testing.T) {
	t.Parallel()

	resp := availablePluginResponse{
		Scope:       "shisho",
		ID:          "example",
		Name:        "Example",
		Overview:    "ov",
		Description: "desc",
		Author:      "Author",
		Homepage:    "https://example.com",
		ImageURL:    "https://example.com/logo.png",
		IsOfficial:  true,
		Versions:    nil,
		Compatible:  true,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "https://example.com/logo.png", decoded["imageUrl"], "imageUrl must serialize as camelCase key")
	assert.Equal(t, true, decoded["is_official"], "is_official must serialize as snake_case key")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestAvailablePluginResponse_JSONFields ./pkg/plugins/ -v`
Expected: FAIL with compile error `resp.ImageURL undefined` (or similar — the struct doesn't have those fields yet).

- [ ] **Step 3: Add the fields to the struct and handler**

Edit `pkg/plugins/handler.go` lines 698-709:

```go
// availablePluginResponse is the response format for available plugins.
type availablePluginResponse struct {
	Scope       string                   `json:"scope"`
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Overview    string                   `json:"overview"`
	Description string                   `json:"description"`
	Author      string                   `json:"author"`
	Homepage    string                   `json:"homepage"`
	ImageURL    string                   `json:"imageUrl"`
	IsOfficial  bool                     `json:"is_official"`
	Versions    []AnnotatedPluginVersion `json:"versions"`
	Compatible  bool                     `json:"compatible"`
}
```

Edit `pkg/plugins/handler.go` lines 747-757 (inside `listAvailable`) to populate the new fields:

```go
result = append(result, availablePluginResponse{
	Scope:       manifest.Scope,
	ID:          p.ID,
	Name:        p.Name,
	Overview:    p.Overview,
	Description: p.Description,
	Author:      p.Author,
	Homepage:    p.Homepage,
	ImageURL:    p.ImageURL,
	IsOfficial:  repo.IsOfficial,
	Versions:    annotated,
	Compatible:  hasCompatible,
})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestAvailablePluginResponse_JSONFields ./pkg/plugins/ -v`
Expected: PASS.

- [ ] **Step 5: Run the full package test suite to check for regressions**

Run: `go test ./pkg/plugins/ -count=1`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/handler_list_available_test.go
git commit -m "[Backend] Include imageUrl and is_official in available plugin response"
```

---

## Task 2: Frontend — add `is_official` to `AvailablePlugin` TS type

**Files:**
- Modify: `app/hooks/queries/plugins.ts`

- [ ] **Step 1: Add the field to the interface**

Open `app/hooks/queries/plugins.ts`. Find the `AvailablePlugin` interface (around line 60). Add `is_official: boolean;` after `imageUrl: string;`:

```ts
export interface AvailablePlugin {
  scope: string;
  id: string;
  name: string;
  overview: string;
  description: string;
  author: string;
  homepage: string;
  imageUrl: string;
  is_official: boolean;
  versions: PluginVersion[];
  compatible: boolean;
}
```

- [ ] **Step 2: Fix TypeScript compile errors in tests**

Tests that construct `AvailablePlugin` objects now need `is_official`. Run typecheck to find them:

Run: `pnpm lint:types`
Expected: errors pointing at `AvailablePlugin` literal usages missing `is_official`.

Fix each offending test by adding `is_official: false` to the `AvailablePlugin` literal. Known callsites (verify by running typecheck):

- `app/components/plugins/PluginVersionHistory.test.tsx` — the `makeAvailable` helper.
- `app/components/plugins/DiscoverTab.test.tsx` — fixture objects.
- `app/components/plugins/pluginCapabilities.test.ts` — fixture objects.

For each file, add `is_official: false,` to the literal alphabetically near `imageUrl` (tsconfig doesn't enforce order, but keeping consistent with existing style).

- [ ] **Step 3: Re-run typecheck**

Run: `pnpm lint:types`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add app/hooks/queries/plugins.ts app/components/plugins/PluginVersionHistory.test.tsx app/components/plugins/DiscoverTab.test.tsx app/components/plugins/pluginCapabilities.test.ts
git commit -m "[Frontend] Add is_official to AvailablePlugin type"
```

---

## Task 3: Frontend — reduce `PluginRow` vertical padding

**Files:**
- Modify: `app/components/plugins/PluginRow.tsx:39-44`

- [ ] **Step 1: Edit the className**

In `app/components/plugins/PluginRow.tsx`, find the `<Link>` root (line 39). Change `p-4` to `px-4 py-3`:

```tsx
<Link
  className={cn(
    "group flex items-center gap-4 rounded-md border border-border px-4 py-3 transition-colors hover:bg-accent/30",
    disabled && "opacity-50 saturate-50",
  )}
  to={href}
>
```

- [ ] **Step 2: Run the existing PluginRow test**

Run: `pnpm vitest run app/components/plugins/PluginRow.test.tsx`
Expected: PASS — no behavior change, only styling.

- [ ] **Step 3: Commit**

```bash
git add app/components/plugins/PluginRow.tsx
git commit -m "[Frontend] Reduce plugin row vertical padding"
```

---

## Task 4: Frontend — fix UTC date bug in `formatReleaseDate`

**Files:**
- Modify: `app/components/plugins/PluginVersionCard.tsx:23-52`
- Test: `app/components/plugins/PluginVersionCard.test.tsx` (new)

- [ ] **Step 1: Write the failing test**

Create `app/components/plugins/PluginVersionCard.test.tsx`:

```tsx
import { PluginVersionCard } from "./PluginVersionCard";
import { render, screen } from "@testing-library/react";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";

import type { PluginVersion } from "@/hooks/queries/plugins";

const makeVersion = (overrides: Partial<PluginVersion> = {}): PluginVersion => ({
  capabilities: undefined,
  changelog: "",
  compatible: true,
  downloadUrl: "",
  manifestVersion: 1,
  minShishoVersion: "0.0.0",
  releaseDate: "",
  sha256: "",
  version: "1.0.0",
  ...overrides,
});

describe("PluginVersionCard date formatting", () => {
  beforeAll(() => {
    // Pin to a known "now" so the "relative" string is deterministic.
    vi.setSystemTime(new Date("2026-04-20T12:00:00Z"));
  });
  afterAll(() => {
    vi.useRealTimers();
  });

  it("treats a date-only releaseDate as local, not UTC", () => {
    // "2026-04-14" must render as "Apr 14, 2026" in any timezone —
    // parsing it as UTC midnight and formatting with toLocaleDateString
    // would show "Apr 13, 2026" west of UTC.
    render(
      <PluginVersionCard
        state="latest"
        version={makeVersion({ releaseDate: "2026-04-14" })}
      />,
    );
    const released = screen.getByText(/released/i);
    expect(released.textContent).toMatch(/Apr 14, 2026/);
  });

  it("still handles RFC3339 timestamps", () => {
    render(
      <PluginVersionCard
        state="latest"
        version={makeVersion({ releaseDate: "2026-04-14T12:00:00Z" })}
      />,
    );
    const released = screen.getByText(/released/i);
    expect(released.textContent).toMatch(/Apr 14, 2026/);
  });

  it("omits the Released line when releaseDate is empty", () => {
    render(
      <PluginVersionCard state="latest" version={makeVersion({ releaseDate: "" })} />,
    );
    expect(screen.queryByText(/released/i)).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify the date-only case fails**

Run: `pnpm vitest run app/components/plugins/PluginVersionCard.test.tsx`
Expected: the "treats a date-only releaseDate as local" test FAILS with rendered text matching `Apr 13, 2026` (off-by-one due to UTC parse). RFC3339 and empty-date tests pass.

If you happen to be running in UTC (unlikely on dev machines but possible in CI), the failing test will render `Apr 14`. The fix is still required for US-timezone users.

- [ ] **Step 3: Fix the parser**

Edit `app/components/plugins/PluginVersionCard.tsx`. Replace lines 23-52 (the `formatReleaseDate` function) with:

```ts
const formatReleaseDate = (
  raw: string,
): { absolute: string; relative: string } | null => {
  if (!raw) return null;
  // Date-only strings like "2026-04-14" are parsed as midnight UTC by
  // `new Date()`, which renders as the previous day west of UTC. Detect the
  // format and construct a local-midnight Date instead.
  const dateOnly = raw.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  const d = dateOnly
    ? new Date(+dateOnly[1], +dateOnly[2] - 1, +dateOnly[3])
    : new Date(raw);
  if (Number.isNaN(d.getTime())) return null;
  const absolute = d.toLocaleDateString(undefined, {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
  const diffDays = Math.floor(
    (Date.now() - d.getTime()) / (1000 * 60 * 60 * 24),
  );
  let relative: string;
  if (diffDays < 1) {
    relative = "today";
  } else if (diffDays === 1) {
    relative = "yesterday";
  } else if (diffDays < 30) {
    relative = `${diffDays} days ago`;
  } else if (diffDays < 365) {
    const months = Math.floor(diffDays / 30);
    relative = `${months} month${months === 1 ? "" : "s"} ago`;
  } else {
    const years = Math.floor(diffDays / 365);
    relative = `${years} year${years === 1 ? "" : "s"} ago`;
  }
  return { absolute, relative };
};
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm vitest run app/components/plugins/PluginVersionCard.test.tsx`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add app/components/plugins/PluginVersionCard.tsx app/components/plugins/PluginVersionCard.test.tsx
git commit -m "[Fix] Parse date-only plugin releaseDate as local, not UTC"
```

---

## Task 5: Frontend — add official-repo badge to `PluginRow`

**Files:**
- Modify: `app/components/plugins/PluginRow.tsx`
- Modify: `app/components/plugins/PluginRow.test.tsx`

- [ ] **Step 1: Write the failing test**

Add to `app/components/plugins/PluginRow.test.tsx` inside the existing `describe("PluginRow", ...)` block:

```tsx
it("renders the official badge when isOfficial is true", () => {
  render(wrap(<PluginRow {...base} isOfficial />));
  // BadgeCheck from lucide renders with this aria-label when we label it.
  expect(screen.getByLabelText(/official plugin/i)).toBeInTheDocument();
});

it("does not render the official badge by default", () => {
  render(wrap(<PluginRow {...base} />));
  expect(screen.queryByLabelText(/official plugin/i)).toBeNull();
});
```

- [ ] **Step 2: Run tests to verify failure**

Run: `pnpm vitest run app/components/plugins/PluginRow.test.tsx`
Expected: the two new tests FAIL (no aria-label "official plugin" in output). Existing tests pass.

- [ ] **Step 3: Add the prop and render the badge**

Edit `app/components/plugins/PluginRow.tsx`. Add `BadgeCheck` to the lucide imports:

```tsx
import { BadgeCheck, ChevronRight } from "lucide-react";
```

Add `isOfficial?: boolean;` to `PluginRowProps` (alphabetically between `imageUrl` and `name`):

```ts
export interface PluginRowProps {
  actions?: ReactNode;
  author?: string;
  capabilities: string[];
  description?: string;
  disabled?: boolean;
  href: string;
  id: string;
  imageUrl?: string | null;
  isOfficial?: boolean;
  name: string;
  scope: string;
  updateAvailable?: string;
  version?: string;
}
```

Destructure `isOfficial` from props (keep alphabetical):

```tsx
export const PluginRow = ({
  actions,
  author,
  capabilities,
  description,
  disabled,
  href,
  id,
  imageUrl,
  isOfficial,
  name,
  scope,
  updateAvailable,
  version,
}: PluginRowProps) => {
```

Update the author span (currently line 66) to include the badge:

```tsx
{author && (
  <span className="inline-flex items-center gap-1">
    ·{" "}
    {isOfficial && (
      <BadgeCheck
        aria-label="Official plugin"
        className="h-3.5 w-3.5 text-primary"
      />
    )}
    {author}
  </span>
)}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm vitest run app/components/plugins/PluginRow.test.tsx`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add app/components/plugins/PluginRow.tsx app/components/plugins/PluginRow.test.tsx
git commit -m "[Frontend] Add official-plugin badge to PluginRow"
```

---

## Task 6: Frontend — add official-repo badge to `PluginDetailHero`

**Files:**
- Modify: `app/components/plugins/PluginDetailHero.tsx`
- Test: `app/components/plugins/PluginDetailHero.test.tsx` (new)

- [ ] **Step 1: Write the failing test**

Create `app/components/plugins/PluginDetailHero.test.tsx`:

```tsx
import { PluginDetailHero } from "./PluginDetailHero";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { AvailablePlugin } from "@/hooks/queries/plugins";

const baseAvailable: AvailablePlugin = {
  author: "Shisho Team",
  compatible: true,
  description: "",
  homepage: "",
  id: "p",
  imageUrl: "",
  is_official: false,
  name: "Plugin",
  overview: "",
  scope: "shisho",
  versions: [],
};

describe("PluginDetailHero", () => {
  it("renders the official badge when the available plugin is official", () => {
    render(
      <PluginDetailHero
        available={{ ...baseAvailable, is_official: true }}
        canWrite={false}
        id="p"
        scope="shisho"
      />,
    );
    expect(screen.getByLabelText(/official plugin/i)).toBeInTheDocument();
  });

  it("does not render the official badge for community plugins", () => {
    render(
      <PluginDetailHero
        available={baseAvailable}
        canWrite={false}
        id="p"
        scope="shisho"
      />,
    );
    expect(screen.queryByLabelText(/official plugin/i)).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify failure**

Run: `pnpm vitest run app/components/plugins/PluginDetailHero.test.tsx`
Expected: the "official" test FAILS (no badge rendered yet).

- [ ] **Step 3: Render the badge**

Edit `app/components/plugins/PluginDetailHero.tsx`.

Add `BadgeCheck` to the lucide import:

```tsx
import { BadgeCheck, ExternalLink } from "lucide-react";
```

Add `isOfficial` as a derived value near the other derivations (around line 40):

```tsx
const imageUrl = available?.imageUrl ?? undefined;
const isOfficial = available?.is_official ?? false;
```

Update the `author` meta part (line 46). Replace:

```tsx
if (author) metaParts.push(`by ${author}`);
```

with:

```tsx
if (author) {
  metaParts.push(
    <span className="inline-flex items-center gap-1">
      by{" "}
      {isOfficial && (
        <BadgeCheck
          aria-label="Official plugin"
          className="h-4 w-4 text-primary"
        />
      )}
      {author}
    </span>,
  );
}
```

Note: `metaParts` is typed as `ReactNode[]` — a JSX fragment in the array is fine.

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm vitest run app/components/plugins/PluginDetailHero.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/components/plugins/PluginDetailHero.tsx app/components/plugins/PluginDetailHero.test.tsx
git commit -m "[Frontend] Add official-plugin badge to PluginDetailHero"
```

---

## Task 7: Frontend — wire `isOfficial` through `DiscoverTab` and `InstalledTab`

**Files:**
- Modify: `app/components/plugins/DiscoverTab.tsx:156-191`
- Modify: `app/components/plugins/InstalledTab.tsx:46-96`

- [ ] **Step 1: Update `DiscoverTab`**

Open `app/components/plugins/DiscoverTab.tsx`. Find the `<PluginRow` call (around line 157). Add `isOfficial={p.is_official}` (alphabetically between `imageUrl` and `key`):

```tsx
<PluginRow
  actions={/* ... */}
  author={p.author || undefined}
  capabilities={caps}
  description={p.description || undefined}
  href={`/settings/plugins/${p.scope}/${p.id}`}
  id={p.id}
  imageUrl={p.imageUrl || null}
  isOfficial={p.is_official}
  key={key}
  name={p.name}
  scope={p.scope}
  version={p.versions[0]?.version}
/>
```

- [ ] **Step 2: Update `InstalledTab`**

Open `app/components/plugins/InstalledTab.tsx`. The installed-plugin row gets `isOfficial` from the matching available entry (already looked up at line 47-49).

Add a `isOfficial` derived value near `imageUrl` (line 52):

```tsx
const imageUrl = availableEntry?.imageUrl || undefined;
const isOfficial = availableEntry?.is_official ?? false;
```

Pass `isOfficial={isOfficial}` to the `<PluginRow` (alphabetical between `imageUrl` and `key`):

```tsx
<PluginRow
  actions={/* ... */}
  author={plugin.author}
  capabilities={capabilityLabels}
  description={plugin.description}
  disabled={isDisabled}
  href={`/settings/plugins/${plugin.scope}/${plugin.id}`}
  id={plugin.id}
  imageUrl={imageUrl}
  isOfficial={isOfficial}
  key={`${plugin.scope}/${plugin.id}`}
  name={plugin.name}
  scope={plugin.scope}
  updateAvailable={plugin.update_available_version ?? undefined}
  version={plugin.version}
/>
```

- [ ] **Step 3: Run the tabs' test suites**

Run: `pnpm vitest run app/components/plugins/DiscoverTab.test.tsx`
Expected: PASS.

(InstalledTab has no dedicated test file. Skip.)

- [ ] **Step 4: Commit**

```bash
git add app/components/plugins/DiscoverTab.tsx app/components/plugins/InstalledTab.tsx
git commit -m "[Frontend] Pass is_official through discover and installed tabs"
```

---

## Task 8: Backend + Frontend — replace per-version `tag` with `releaseUrl`

This task overwrites uncommitted work on this branch. Task 9 rewires the frontend to consume the new field; do them back-to-back.

**Files:**
- Modify: `pkg/plugins/repository.go:79-93`
- Modify: `app/hooks/queries/plugins.ts` (the `PluginVersion` interface)

- [ ] **Step 1: Update the Go struct**

Edit `pkg/plugins/repository.go`. Replace the `PluginVersion` struct (lines 79-93) with:

```go
// PluginVersion describes a specific version of an available plugin.
type PluginVersion struct {
	Version          string `json:"version"`
	MinShishoVersion string `json:"minShishoVersion"`
	ManifestVersion  int    `json:"manifestVersion"`
	ReleaseDate      string `json:"releaseDate"`
	Changelog        string `json:"changelog"`
	DownloadURL      string `json:"downloadUrl"`
	SHA256           string `json:"sha256"`
	// ReleaseURL is an optional explicit URL for this version's release page
	// (e.g. a GitHub release, a GitLab tag page). When present, the UI renders
	// a "View release" link on the version card. When absent, no link is shown.
	// Not validated — any string URL is accepted.
	ReleaseURL   string        `json:"releaseUrl,omitempty"`
	Capabilities *Capabilities `json:"capabilities,omitempty"`
}
```

- [ ] **Step 2: Update the TypeScript interface**

Edit `app/hooks/queries/plugins.ts`. Find the `PluginVersion` interface (around line 44). Replace the `tag?: string` field (and its comment) with `releaseUrl?: string`:

```ts
export interface PluginVersion {
  version: string;
  minShishoVersion: string;
  downloadUrl: string;
  compatible: boolean;
  changelog: string;
  sha256: string;
  manifestVersion: number;
  releaseDate: string;
  // Optional full URL to this version's release page (e.g. a GitHub release
  // URL). When unset, the version card does not show a release link.
  releaseUrl?: string;
  capabilities?: PluginCapabilities;
}
```

- [ ] **Step 3: Run the Go package tests**

Run: `go test ./pkg/plugins/ -count=1`
Expected: PASS. (The struct change is serialization-only; no behavioral tests depend on `tag`.)

- [ ] **Step 4: Run the frontend typecheck**

Run: `pnpm lint:types`
Expected: errors in `PluginVersionHistory.tsx` and `PluginVersionHistory.test.tsx` that reference `version.tag`. Leave those errors — they'll be fixed in Task 9.

- [ ] **Step 5: Commit**

```bash
git add pkg/plugins/repository.go app/hooks/queries/plugins.ts
git commit -m "[Backend] Replace per-version tag with releaseUrl"
```

---

## Task 9: Frontend — rewire `PluginVersionHistory` and `PluginVersionCard` to use `releaseUrl`

**Files:**
- Modify: `app/components/plugins/PluginVersionHistory.tsx`
- Modify: `app/components/plugins/PluginVersionHistory.test.tsx`
- Modify: `app/components/plugins/PluginVersionCard.tsx`

- [ ] **Step 1: Replace the test file**

Overwrite `app/components/plugins/PluginVersionHistory.test.tsx` entirely. The existing tests all assert the old `buildGitHubReleaseUrl` behavior — they need to be replaced, not augmented. New content:

```tsx
import { PluginVersionHistory } from "./PluginVersionHistory";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it } from "vitest";

import type { AvailablePlugin, PluginVersion } from "@/hooks/queries/plugins";

const makeVersion = (
  v: string,
  overrides: Partial<PluginVersion> = {},
): PluginVersion => ({
  capabilities: undefined,
  changelog: "",
  compatible: true,
  downloadUrl: "https://example.com/plugin.zip",
  manifestVersion: 1,
  minShishoVersion: "0.0.0",
  releaseDate: "",
  sha256: "deadbeef",
  version: v,
  ...overrides,
});

const makeAvailable = (versions: PluginVersion[]): AvailablePlugin => ({
  author: "",
  compatible: true,
  description: "",
  homepage: "",
  id: "p",
  imageUrl: "",
  is_official: false,
  name: "Plugin",
  overview: "",
  scope: "shisho",
  versions,
});

const renderWithClient = (ui: React.ReactElement) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
};

describe("PluginVersionHistory", () => {
  it("renders a View release link pointing at the version's releaseUrl", () => {
    const available = makeAvailable([
      makeVersion("1.0.0", {
        releaseUrl: "https://github.com/me/repo/releases/tag/v1.0.0",
      }),
    ]);
    renderWithClient(<PluginVersionHistory available={available} />);
    const link = screen.getByRole("link", { name: /view release/i });
    expect(link).toHaveAttribute(
      "href",
      "https://github.com/me/repo/releases/tag/v1.0.0",
    );
  });

  it("renders releaseUrl verbatim regardless of host", () => {
    const available = makeAvailable([
      makeVersion("1.0.0", {
        releaseUrl: "https://gitlab.example.com/me/repo/-/tags/v1.0.0",
      }),
    ]);
    renderWithClient(<PluginVersionHistory available={available} />);
    const link = screen.getByRole("link", { name: /view release/i });
    expect(link).toHaveAttribute(
      "href",
      "https://gitlab.example.com/me/repo/-/tags/v1.0.0",
    );
  });

  it("omits the release link when releaseUrl is absent", () => {
    const available = makeAvailable([makeVersion("1.0.0")]);
    renderWithClient(<PluginVersionHistory available={available} />);
    expect(screen.queryByRole("link", { name: /view release/i })).toBeNull();
  });
});
```

- [ ] **Step 2: Rewrite `PluginVersionHistory.tsx`**

Open `app/components/plugins/PluginVersionHistory.tsx`. Delete `buildGitHubReleaseUrl` entirely. Remove the `homepage` usage. Replace the `gitHubReleaseUrl` prop passed to each `PluginVersionCard` with `releaseUrl={v.releaseUrl}`.

Full updated file contents:

```tsx
import { PluginVersionCard } from "./PluginVersionCard";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  useUpdatePluginVersion,
  type AvailablePlugin,
} from "@/hooks/queries/plugins";
import type { Plugin } from "@/types/generated/models";

const INITIAL_VISIBLE_OLDER = 3;

export interface PluginVersionHistoryProps {
  available?: AvailablePlugin;
  installed?: Plugin;
}

export const PluginVersionHistory = ({
  available,
  installed,
}: PluginVersionHistoryProps) => {
  const versions = useMemo(
    () => available?.versions ?? [],
    [available?.versions],
  );
  const installedVersion = installed?.version;
  const updateTarget = installed?.update_available_version;
  const updateVersion = useUpdatePluginVersion();
  const [expanded, setExpanded] = useState(false);

  const [newerVersions, olderVersions] = useMemo<
    [typeof versions, typeof versions]
  >(() => {
    if (!installedVersion) {
      const compatible = versions.filter((v) => v.compatible !== false);
      return [compatible.slice(0, 1), compatible.slice(1)];
    }
    const installedIdx = versions.findIndex(
      (v) => v.version === installedVersion,
    );
    if (installedIdx === -1) {
      const compatible = versions.filter((v) => v.compatible !== false);
      return [compatible.slice(0, 1), compatible.slice(1)];
    }
    const newer = installedIdx > 0 ? versions.slice(0, installedIdx) : [];
    const rest = versions.slice(installedIdx);
    return [newer, rest];
  }, [versions, installedVersion]);

  const handleUpdate = () => {
    if (!installed) return;
    const targetLabel = updateTarget ?? newerVersions[0]?.version;
    updateVersion.mutate(
      { id: installed.id, scope: installed.scope },
      {
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : "Update failed");
        },
        onSuccess: () => {
          toast.success(
            targetLabel ? `Updated to v${targetLabel}` : "Plugin updated",
          );
        },
      },
    );
  };

  if (versions.length === 0) return null;

  const visibleOlder = expanded
    ? olderVersions
    : olderVersions.slice(0, INITIAL_VISIBLE_OLDER);
  const hiddenCount = olderVersions.length - visibleOlder.length;

  return (
    <section className="space-y-4">
      <h2 className="text-lg font-semibold">Version history</h2>

      {newerVersions.map((v, idx) => {
        const isUpdateTarget =
          installedVersion !== undefined &&
          (updateTarget ? v.version === updateTarget : idx === 0);
        return (
          <PluginVersionCard
            isUpdating={updateVersion.isPending}
            key={v.version}
            onUpdate={isUpdateTarget ? handleUpdate : undefined}
            releaseUrl={v.releaseUrl}
            state={installedVersion ? "available" : "latest"}
            version={v}
          />
        );
      })}

      {visibleOlder.map((v) => (
        <PluginVersionCard
          key={v.version}
          releaseUrl={v.releaseUrl}
          state={v.version === installedVersion ? "installed" : "older"}
          version={v}
        />
      ))}

      {hiddenCount > 0 && (
        <Button onClick={() => setExpanded(true)} size="sm" variant="ghost">
          Show {hiddenCount} older version{hiddenCount === 1 ? "" : "s"}
        </Button>
      )}
    </section>
  );
};
```

- [ ] **Step 3: Update `PluginVersionCard` to take `releaseUrl` and rename the link label**

Open `app/components/plugins/PluginVersionCard.tsx`. Rename the prop `gitHubReleaseUrl` to `releaseUrl` and change the link text from "View release on GitHub" to "View release".

Find the props interface (around line 15) and change:

```ts
export interface PluginVersionCardProps {
  gitHubReleaseUrl?: string;
  isUpdating?: boolean;
  onUpdate?: () => void;
  state: PluginVersionCardState;
  version: PluginVersion;
}
```

to:

```ts
export interface PluginVersionCardProps {
  releaseUrl?: string;
  isUpdating?: boolean;
  onUpdate?: () => void;
  state: PluginVersionCardState;
  version: PluginVersion;
}
```

Update the component destructure (around line 54):

```tsx
export const PluginVersionCard = ({
  releaseUrl,
  isUpdating,
  onUpdate,
  state,
  version,
}: PluginVersionCardProps) => {
```

Update the link rendering (around lines 90-106). Change the conditional from `gitHubReleaseUrl` to `releaseUrl` and the text:

```tsx
{(onUpdate || releaseUrl) && (
  <div className="flex items-center gap-2">
    {onUpdate && (
      <Button disabled={isUpdating} onClick={onUpdate} size="sm">
        {isUpdating ? "Updating…" : "Update now"}
      </Button>
    )}
    {releaseUrl && (
      <a
        className="inline-flex items-center gap-1 text-xs underline underline-offset-2"
        href={releaseUrl}
        rel="noopener noreferrer"
        target="_blank"
      >
        View release <ExternalLink className="h-3 w-3" />
      </a>
    )}
  </div>
)}
```

- [ ] **Step 4: Run the frontend test suite for affected files**

Run: `pnpm vitest run app/components/plugins/PluginVersionHistory.test.tsx app/components/plugins/PluginVersionCard.test.tsx`
Expected: all PASS.

- [ ] **Step 5: Run the full typecheck**

Run: `pnpm lint:types`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add app/components/plugins/PluginVersionHistory.tsx app/components/plugins/PluginVersionHistory.test.tsx app/components/plugins/PluginVersionCard.tsx
git commit -m "[Frontend] Use releaseUrl directly for version release link"
```

---

## Task 10: Frontend — replace `PluginDetail` back button with breadcrumb

**Files:**
- Modify: `app/components/pages/PluginDetail.tsx:85-96`
- Test: `app/components/pages/PluginDetail.test.tsx` (new)

Follow the existing `LibraryBreadcrumbs` pattern (see `app/components/library/LibraryBreadcrumbs.tsx`) but render inline since this is the only non-library breadcrumb so far.

- [ ] **Step 1: Write the failing test**

Create `app/components/pages/PluginDetail.test.tsx`:

```tsx
import { PluginDetail } from "./PluginDetail";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import React from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

// Mock the auth hook used by PluginDetail.
vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({ hasPermission: () => true }),
}));

// Mock query hooks so the component renders without network.
vi.mock("@/hooks/queries/plugins", () => ({
  usePluginsInstalled: () => ({ data: [], isLoading: false, isError: false }),
  usePluginsAvailable: () => ({
    data: [
      {
        author: "",
        compatible: true,
        description: "",
        homepage: "",
        id: "example",
        imageUrl: "",
        is_official: false,
        name: "Example Plugin",
        overview: "",
        scope: "shisho",
        versions: [],
      },
    ],
    isLoading: false,
    isError: false,
  }),
  useUpdatePlugin: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdatePluginVersion: () => ({ mutateAsync: vi.fn(), isPending: false, mutate: vi.fn() }),
  PluginStatusActive: "active",
}));

const renderAt = (path: string) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route element={<PluginDetail />} path="/settings/plugins/:scope/:id" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("PluginDetail breadcrumb", () => {
  it("renders Plugins as a link to /settings/plugins and the plugin name as current", () => {
    renderAt("/settings/plugins/shisho/example");
    const pluginsLink = screen.getByRole("link", { name: /^plugins$/i });
    expect(pluginsLink).toHaveAttribute("href", "/settings/plugins");
    // Plugin name appears in breadcrumb current position AND in the hero —
    // at least one of them; ensure no back-button "Plugins" button remains.
    expect(screen.queryByRole("button", { name: /^plugins$/i })).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify failure**

Run: `pnpm vitest run app/components/pages/PluginDetail.test.tsx`
Expected: FAIL — the "Plugins" button still exists or the link doesn't.

- [ ] **Step 3: Replace the back button with a breadcrumb**

Open `app/components/pages/PluginDetail.tsx`. Remove the `ChevronLeft` import (no longer needed). The `Button` import can also be removed if no other usage remains — check before removing.

Replace the back-button block (lines 87-96):

```tsx
<div>
  <Button
    onClick={() => navigate("/settings/plugins")}
    size="sm"
    variant="ghost"
  >
    <ChevronLeft className="mr-1 h-4 w-4" />
    Plugins
  </Button>
</div>
```

with a breadcrumb nav:

```tsx
<nav className="text-xs sm:text-sm text-muted-foreground overflow-hidden">
  <ol className="flex items-center gap-1 sm:gap-2 flex-wrap">
    <li className="shrink-0">
      <Link
        className="hover:text-foreground hover:underline"
        to="/settings/plugins"
      >
        Plugins
      </Link>
    </li>
    <li aria-hidden="true" className="shrink-0">
      ›
    </li>
    <li className="text-foreground truncate">{displayName}</li>
  </ol>
</nav>
```

Add `Link` to the `react-router-dom` imports at the top of the file:

```tsx
import { Link, useNavigate, useParams } from "react-router-dom";
```

Remove unused imports:
- `ChevronLeft` from lucide-react (if not used elsewhere in this file)
- `Button` from `@/components/ui/button` (if not used elsewhere in this file — check the skeleton and form sections)

Check whether `Button` or `ChevronLeft` are used anywhere else in the file before removing. Run `pnpm lint:eslint app/components/pages/PluginDetail.tsx` after to catch unused-import warnings.

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm vitest run app/components/pages/PluginDetail.test.tsx`
Expected: PASS.

- [ ] **Step 5: Run full frontend lint to catch unused imports**

Run: `pnpm lint:eslint app/components/pages/PluginDetail.tsx && pnpm lint:types`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add app/components/pages/PluginDetail.tsx app/components/pages/PluginDetail.test.tsx
git commit -m "[Frontend] Replace plugin detail back button with breadcrumb"
```

---

## Task 11: Docs — update `repositories.md` for schema changes

**Files:**
- Modify: `website/docs/plugins/repositories.md`

- [ ] **Step 1: Update the "Plugin Versions Include" summary list**

Open `website/docs/plugins/repositories.md`. Find the list around lines 32-38 ("Each plugin version in a repository includes:"). Insert a bullet for `releaseUrl` after the changelog bullet so the list reads:

```markdown
Each plugin version in a repository includes:

- A **download URL** pointing to a ZIP file on GitHub Releases
- A **SHA256 hash** for verifying the download integrity
- A **minimum Shisho version** for compatibility filtering
- A **changelog** describing what changed
- An optional **release URL** linking to the version's release page (shown as "View release" on the version card)
- An optional **capabilities** object declaring what the plugin can do (shown during install)
```

- [ ] **Step 2: Update the example JSON block**

Still in `repositories.md`. Find the JSON code block (around lines 50-87). Two edits:

1. Delete the `"tag": "v1.0.0",` line.
2. Add `"releaseUrl": "https://github.com/my-org/my-plugin/releases/tag/v1.0.0",` right after `"downloadUrl"`.

The version entry should read:

```json
{
  "version": "1.0.0",
  "minShishoVersion": "0.1.0",
  "manifestVersion": 1,
  "releaseDate": "2025-06-15",
  "changelog": "Initial release.",
  "downloadUrl": "https://github.com/my-org/my-plugin/releases/download/v1.0.0/my-plugin.zip",
  "releaseUrl": "https://github.com/my-org/my-plugin/releases/tag/v1.0.0",
  "sha256": "abc123...",
  "capabilities": {
    "metadataEnricher": {
      "fileTypes": ["epub", "m4b"],
      "fields": ["title", "authors", "description", "cover"]
    },
    "httpAccess": {
      "domains": ["*.example.com"]
    }
  }
}
```

- [ ] **Step 3: Update the "Field notes" section**

In the same file, find the "Field notes" section (starts around line 89). Make four edits:

**Edit 3a:** Delete the `tag` bullet entirely (it's the one starting `**`tag`** (on each version entry): Optional. The git tag used to build…`).

**Edit 3b:** Add a `releaseUrl` bullet immediately after the `releaseDate` bullet:

```markdown
- **`releaseUrl`** (on each version entry): Optional. Full URL to the release page for this version — any HTTPS URL works (GitHub release, GitLab tag, Codeberg release, etc.). When present, Shisho renders a "View release" link on the version card. When omitted, no link is shown. Shisho does not validate the host or path — it renders the URL verbatim.
```

**Edit 3c:** Add a `homepage` bullet (this field is currently not called out in Field notes). Add it before the `imageUrl` bullet so the order mirrors the JSON example (plugin-level first, version-level after):

```markdown
- **`homepage`** (on each plugin entry): Optional. The plugin's landing page — shown as the "homepage" link on the plugin detail page. For multi-plugin repositories, point this at the plugin's own page (e.g. `https://github.com/my-org/my-plugins/tree/main/plugins/my-plugin`) rather than the repository root. Shisho uses this field purely for display — release links come from `releaseUrl` on each version, not from `homepage`.
```

**Edit 3d:** Update the `changelog` bullet. Replace its last sentence — currently `The "View full diff on GitHub" link in the UI is inferred from the plugin's \`homepage\` when it points to a GitHub repo; no additional manifest field is read for it.` — with:

```
The "View release" link shown alongside the changelog is controlled by `releaseUrl` on the version, not inferred from `homepage`.
```

The final `changelog` bullet should read:

```markdown
- **`changelog`** (on each version entry): Rendered as sanitized markdown on the plugin detail page. Supported subset: headings (`##`, `###`), paragraphs, lists, inline code, fenced code blocks, links (open in a new tab), bold, italic. Raw HTML, images, and iframes are stripped — author content accordingly. The "View release" link shown alongside the changelog is controlled by `releaseUrl` on the version, not inferred from `homepage`.
```

- [ ] **Step 4: Preview the docs locally if desired**

(Optional — only if actively developing docs style.) Run `mise docs` and open http://localhost:3000 to eyeball the formatted page.

- [ ] **Step 5: Commit**

```bash
git add website/docs/plugins/repositories.md
git commit -m "[Docs] Document releaseUrl schema change and homepage clarification"
```

---

## Task 12: Final verification

- [ ] **Step 1: Run `mise check:quiet`**

Run: `mise check:quiet`
Expected: one-line pass summary. If anything fails, fix it before proceeding (do not squash failures).

- [ ] **Step 2: Manual smoke test**

Start the dev server (`mise start`) and verify:

- Discover tab: plugin rows are visibly shorter vertically than before; official plugins (from the `shishobooks` scope) show a purple `BadgeCheck` icon next to the author name; non-official plugins do not. Plugin logos load (no more OL/initial fallback for plugins that ship an imageUrl — subject to the repo.json containing imageUrls; the test plugin from `shishobooks/plugins` does).
- Plugin detail page (any plugin): breadcrumb reads `Plugins › {plugin name}`; clicking `Plugins` returns to `/settings/plugins`. No back button present.
- Plugin detail page, Version history: release dates match the `releaseDate` value in the repository.json (verify against https://raw.githubusercontent.com/shishobooks/plugins/master/repository.json for open-library-enricher: v0.3.1 should show "Apr 12, 2026", not "Apr 11").
- A "View release" link appears on each version card if and only if that version has a `releaseUrl` in the manifest. (The official repo does not yet set `releaseUrl`, so the Shisho team needs to deploy data updates separately — see "Out of scope" in the design doc. During local testing, verify by editing the cached repository response or pointing at a fork that includes `releaseUrl`.)

- [ ] **Step 3: Nothing else committed**

Run: `git status`
Expected: clean working tree.

---

## Out of scope (do not implement as part of this plan)

These are tracked separately on the Shisho Notion board:

1. Update `shishobooks/plugins` repository.json: per-plugin `homepage` (point at plugin subdirectory) and per-version `releaseUrl`.

Failing to do these doesn't break anything — the UI falls back to today's behavior (homepage at repo root; no release link when `releaseUrl` is absent).
