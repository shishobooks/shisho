# Plugins UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> Check the project's root CLAUDE.md and any relevant subdirectory CLAUDE.md files for rules that apply to your work. These contain critical project conventions, gotchas, and requirements (e.g., docs update requirements, testing conventions, naming rules). Violations of these rules are review failures.

**Goal:** Collapse the plugins settings page to two tabs (Installed / Discover) with a gear-icon Advanced dialog for Order + Repositories, add a plugin detail page at `/settings/plugins/:scope/:id` with hero, versioned changelogs, permissions, config, and uninstall, apply consistent logo treatment everywhere, validate `releaseDate` on the repo manifest, and render changelogs as sanitized markdown.

**Architecture:** The detail page reuses the existing `usePluginsInstalled()` / `usePluginsAvailable()` list queries and filters client-side — no new per-plugin detail endpoints. One small new backend endpoint (`GET /plugins/installed/:scope/:id/manifest`) powers the "View manifest" icon action. A shared `PluginLogo` component handles the square-backdrop treatment + initials fallback. A shared `PluginRow` component is used by both tabs. `PluginConfigForm` is extracted from `PluginConfigDialog` so the detail page can mount it inline (with page-level `useUnsavedChanges` protection) and the old dialog is deleted. `react-markdown` + `rehype-sanitize` are added for the changelog render.

**Tech Stack:** Go (Echo, Bun), React 19 + TypeScript, Tanstack Query, React Router v7, shadcn/ui, Tailwind, Vitest + Testing Library, Playwright.

**Spec:** `docs/superpowers/specs/2026-04-18-plugins-ui-redesign-design.md`

---

### Task 1: Backend — Validate `releaseDate` on Repository Manifest

**Why:** Spec §6 says `releaseDate` must accept RFC3339 or date-only and reject garbage. Today `PluginVersion.ReleaseDate` is stored as a raw string with no validation (see `pkg/plugins/repository.go:44`). We validate at fetch/parse time so downstream UI can trust the value.

**Files:**
- Modify: `pkg/plugins/repository.go` (add validator + call site in parse path)
- Create: `pkg/plugins/repository_release_date_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/plugins/repository_release_date_test.go`:

```go
package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateReleaseDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty is allowed", "", false},
		{"RFC3339 UTC", "2026-04-14T00:00:00Z", false},
		{"RFC3339 offset", "2026-04-14T09:30:00-05:00", false},
		{"date only", "2026-04-14", false},
		{"random garbage", "not-a-date", true},
		{"partial date", "2026-04", true},
		{"wrong separator", "2026/04/14", true},
		{"empty space", "   ", true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateReleaseDate(tc.input)
			if tc.wantErr {
				assert.Error(t, err, "expected error for %q", tc.input)
			} else {
				assert.NoError(t, err, "unexpected error for %q", tc.input)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/plugins/ -run TestValidateReleaseDate -v`
Expected: FAIL with `validateReleaseDate` undefined.

- [ ] **Step 3: Implement `validateReleaseDate`**

In `pkg/plugins/repository.go`, add near the top-level helpers (after the `AllowedFetchHosts` block):

```go
// validateReleaseDate returns nil for an empty string, RFC3339, or date-only
// (YYYY-MM-DD) values. Any other non-empty value returns an error.
func validateReleaseDate(s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return nil
	}
	return errors.Errorf("invalid releaseDate %q: expected RFC3339 or YYYY-MM-DD", s)
}
```

(`errors` and `time` are already imported in this file.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/plugins/ -run TestValidateReleaseDate -v`
Expected: PASS for all cases.

- [ ] **Step 5: Wire validator into `FetchRepository`**

In `pkg/plugins/repository.go`, after the existing JSON unmarshal (look for the block that parses the manifest body into `RepositoryManifest`), iterate plugins and versions and skip any version with an invalid `ReleaseDate`, logging a warning. Show the exact edit after reading the file:

```go
// After: if err := json.Unmarshal(body, &manifest); err != nil { ... }
// Add version-level validation to drop malformed releaseDate entries:
for i := range manifest.Plugins {
	p := &manifest.Plugins[i]
	filtered := p.Versions[:0]
	for _, v := range p.Versions {
		if err := validateReleaseDate(v.ReleaseDate); err != nil {
			logger.Default().Warn("skipping plugin version with invalid releaseDate", logger.Data{
				"scope":        manifest.Scope,
				"plugin":       p.ID,
				"version":      v.Version,
				"release_date": v.ReleaseDate,
				"error":        err.Error(),
			})
			continue
		}
		filtered = append(filtered, v)
	}
	p.Versions = filtered
}
```

If `logger` isn't already imported in this file, import `"github.com/shishobooks/shisho/pkg/logger"` (use the same import path already used elsewhere in the package — check `pkg/plugins/handler.go` for the exact path).

- [ ] **Step 6: Add a repository-level integration test**

Extend `repository_release_date_test.go` with a test that exercises the filtering path (write one that calls the same helper loop against a fixture manifest struct — no HTTP needed):

```go
func TestFetchRepository_FiltersInvalidReleaseDates(t *testing.T) {
	t.Parallel()

	manifest := &RepositoryManifest{
		Scope: "test",
		Plugins: []AvailablePlugin{{
			ID: "p",
			Versions: []PluginVersion{
				{Version: "1.0.0", ReleaseDate: "2026-04-14"},
				{Version: "1.1.0", ReleaseDate: "garbage"},
				{Version: "1.2.0", ReleaseDate: ""},
			},
		}},
	}

	filterInvalidReleaseDates(manifest) // extract the for-loop above into this helper

	assert.Len(t, manifest.Plugins[0].Versions, 2)
	assert.Equal(t, "1.0.0", manifest.Plugins[0].Versions[0].Version)
	assert.Equal(t, "1.2.0", manifest.Plugins[0].Versions[1].Version)
}
```

Refactor the loop in Step 5 into an exported-within-package helper `filterInvalidReleaseDates(manifest *RepositoryManifest)` so the test hits it directly.

- [ ] **Step 7: Run all plugin tests**

Run: `go test ./pkg/plugins/... -race`
Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add pkg/plugins/repository.go pkg/plugins/repository_release_date_test.go
git commit -m "[Backend] Validate releaseDate on repository manifest"
```

---

### Task 2: Backend — Manifest Dialog Endpoint + Frontend Query

**Why:** Spec §6 "Manifest dialog endpoint" and §4 "View manifest" icon action. The endpoint reads the raw `manifest.json` from disk and returns it so the UI can display it in a dialog without rehydrating the installed-plugin list payload.

**Files:**
- Modify: `pkg/plugins/routes.go` (new route)
- Modify: `pkg/plugins/handler.go` (new handler method)
- Create: `pkg/plugins/handler_manifest_test.go`
- Modify: `app/hooks/queries/plugins.ts` (new query)

- [ ] **Step 1: Write the failing test**

Create `pkg/plugins/handler_manifest_test.go`:

```go
package plugins

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_GetManifest_ReturnsFileContents(t *testing.T) {
	t.Parallel()

	h, cleanup := newTestHandler(t) // existing helper in this package; check other handler tests
	defer cleanup()

	// Create a plugin on disk and register it in the DB
	scope, id := "test", "echo"
	pluginPath := filepath.Join(h.installer.PluginDir(), scope, id)
	require.NoError(t, os.MkdirAll(pluginPath, 0o755))
	manifestJSON := `{"manifestVersion":1,"id":"echo","name":"Echo","version":"1.0.0"}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginPath, "manifest.json"), []byte(manifestJSON), 0o644))
	installTestPluginRow(t, h.service, scope, id, "1.0.0")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/plugins/installed/test/echo/manifest", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues(scope, id)

	require.NoError(t, h.getManifest(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, manifestJSON, rec.Body.String())
}

func TestHandler_GetManifest_Returns404WhenPluginNotInDB(t *testing.T) {
	t.Parallel()

	h, cleanup := newTestHandler(t)
	defer cleanup()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/plugins/installed/nope/nope/manifest", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues("nope", "nope")

	err := h.getManifest(c)
	assert.True(t, errcodes.IsNotFound(err))
}

func TestHandler_GetManifest_Returns404WhenFileMissing(t *testing.T) {
	t.Parallel()

	h, cleanup := newTestHandler(t)
	defer cleanup()

	// Register plugin in DB but don't create manifest.json on disk
	installTestPluginRow(t, h.service, "test", "ghost", "1.0.0")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/plugins/installed/test/ghost/manifest", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues("test", "ghost")

	err := h.getManifest(c)
	assert.True(t, errcodes.IsNotFound(err))
}
```

(Check `pkg/plugins/handler_*_test.go` for the actual helper name — `newTestHandler` or equivalent — and for an `installTestPluginRow` helper that inserts a `models.Plugin` row. If neither exists, add minimal helpers at the top of this test file using the existing DB helpers.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/plugins/ -run TestHandler_GetManifest -v`
Expected: FAIL with `h.getManifest undefined`.

- [ ] **Step 3: Add the handler**

In `pkg/plugins/handler.go`, add (place near the other installed-plugin handlers, e.g., after `getConfig`):

```go
// getManifest returns the raw manifest.json for an installed plugin.
func (h *handler) getManifest(c echo.Context) error {
	ctx := c.Request().Context()
	scope := c.Param("scope")
	id := c.Param("id")

	if _, err := h.service.RetrievePlugin(ctx, scope, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("Plugin")
		}
		return errors.WithStack(err)
	}

	manifestPath := filepath.Join(h.installer.PluginDir(), scope, id, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errcodes.NotFound("Manifest")
		}
		return errors.WithStack(err)
	}

	return c.Blob(http.StatusOK, "application/json", data)
}
```

(Imports: `"os"`, `"path/filepath"`. `sql`, `errors`, `errcodes`, `http` are already imported.)

- [ ] **Step 4: Register the route**

In `pkg/plugins/routes.go`, in the existing installed-plugin route block (around line 42–52), add:

```go
g.GET("/installed/:scope/:id/manifest", h.getManifest)
```

Place it near the other `/installed/:scope/:id/...` GET routes so it's grouped logically.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/plugins/ -run TestHandler_GetManifest -v`
Expected: PASS.

- [ ] **Step 6: Add the frontend query hook**

In `app/hooks/queries/plugins.ts`, after the existing `usePluginConfig` hook, add:

```ts
export const usePluginManifest = (
  scope: string | undefined,
  id: string | undefined,
  options: { enabled?: boolean } = {},
) => {
  return useQuery({
    queryKey: ["plugins", "manifest", scope, id],
    enabled: !!scope && !!id && options.enabled !== false,
    queryFn: async ({ signal }) => {
      return API.request<unknown>(
        "GET",
        `/plugins/installed/${scope}/${id}/manifest`,
        null,
        null,
        signal,
      );
    },
  });
};
```

- [ ] **Step 7: Type-check**

Run: `mise tygo && pnpm -C . lint:types`
Expected: passes.

- [ ] **Step 8: Commit**

```bash
git add pkg/plugins/routes.go pkg/plugins/handler.go pkg/plugins/handler_manifest_test.go app/hooks/queries/plugins.ts
git commit -m "[Backend] Add GET /plugins/installed/:scope/:id/manifest endpoint"
```

---

### Task 3: `PluginLogo` Component + Initials Fallback

**Why:** Spec §5. Consistent square/radius/backdrop treatment with deterministic initials fallback for plugins without `imageUrl` or when images fail to load.

**Files:**
- Create: `app/components/plugins/PluginLogo.tsx`
- Create: `app/components/plugins/PluginLogo.test.tsx`
- Create: `app/components/plugins/logoColor.ts` (pure helper for the palette)
- Create: `app/components/plugins/logoColor.test.ts`

- [ ] **Step 1: Write the failing initials/color helper tests**

Create `app/components/plugins/logoColor.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { derivePluginInitials, getPluginFallbackColor } from "./logoColor";

describe("derivePluginInitials", () => {
  it("uses first letter + letter after first hyphen when id has a hyphen", () => {
    expect(derivePluginInitials("google-books")).toBe("GB");
    expect(derivePluginInitials("shisho-local-tagger")).toBe("SL");
  });

  it("uses first two letters for single-word ids ≥ 2 chars", () => {
    expect(derivePluginInitials("calibre")).toBe("CA");
    expect(derivePluginInitials("audible")).toBe("AU");
  });

  it("uses the single letter uppercased for 1-char ids", () => {
    expect(derivePluginInitials("c")).toBe("C");
  });

  it("returns uppercase", () => {
    expect(derivePluginInitials("abc-def")).toBe("AD");
  });
});

describe("getPluginFallbackColor", () => {
  it("returns a deterministic palette entry for a given (scope, id)", () => {
    const first = getPluginFallbackColor("shisho", "google-books");
    const second = getPluginFallbackColor("shisho", "google-books");
    expect(first).toBe(second);
  });

  it("returns different colors for different (scope, id) pairs (most of the time)", () => {
    // With 10 palette entries, most adjacent ids should not collide.
    const a = getPluginFallbackColor("shisho", "a");
    const b = getPluginFallbackColor("shisho", "b");
    const c = getPluginFallbackColor("shisho", "c");
    const d = getPluginFallbackColor("shisho", "d");
    const unique = new Set([a, b, c, d]);
    expect(unique.size).toBeGreaterThan(1);
  });

  it("returns a value from the declared palette", () => {
    const color = getPluginFallbackColor("shisho", "google-books");
    expect(color).toMatch(/^#([0-9a-f]{6})$/i);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm vitest run app/components/plugins/logoColor.test.ts`
Expected: FAIL with module-not-found.

- [ ] **Step 3: Implement the helpers**

Create `app/components/plugins/logoColor.ts`:

```ts
// 10-entry palette of saturated but not-harsh hues. Hex values chosen to render
// legibly on the muted 5%-white backdrop used by <PluginLogo />.
export const PLUGIN_LOGO_PALETTE = [
  "#7a5cff", // violet
  "#2ea4ff", // blue
  "#13b981", // green
  "#e2a42c", // amber
  "#e36a2c", // orange
  "#e8487f", // pink
  "#9f4dd8", // purple
  "#2ac3a2", // teal
  "#5a6bff", // indigo
  "#d85555", // red
] as const;

export const derivePluginInitials = (id: string): string => {
  if (!id) return "?";
  const hyphenIdx = id.indexOf("-");
  if (hyphenIdx > 0 && hyphenIdx < id.length - 1) {
    return (id[0] + id[hyphenIdx + 1]).toUpperCase();
  }
  if (id.length >= 2) {
    return id.slice(0, 2).toUpperCase();
  }
  return id.toUpperCase();
};

const hashString = (s: string): number => {
  // djb2-style hash, deterministic, no dependencies
  let h = 5381;
  for (let i = 0; i < s.length; i++) {
    h = (h * 33) ^ s.charCodeAt(i);
  }
  return Math.abs(h);
};

export const getPluginFallbackColor = (scope: string, id: string): string => {
  const idx = hashString(`${scope}/${id}`) % PLUGIN_LOGO_PALETTE.length;
  return PLUGIN_LOGO_PALETTE[idx];
};
```

- [ ] **Step 4: Run helper tests to verify they pass**

Run: `pnpm vitest run app/components/plugins/logoColor.test.ts`
Expected: PASS.

- [ ] **Step 5: Write the failing component test**

Create `app/components/plugins/PluginLogo.test.tsx`:

```tsx
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PluginLogo } from "./PluginLogo";

describe("PluginLogo", () => {
  it("renders an <img> when imageUrl is provided", () => {
    render(<PluginLogo scope="shisho" id="google-books" imageUrl="https://example/g.png" size={40} />);
    const img = screen.getByRole("img");
    expect(img).toHaveAttribute("src", "https://example/g.png");
    expect(img).toHaveAttribute("alt", "google-books logo");
  });

  it("falls back to initials when imageUrl is missing", () => {
    render(<PluginLogo scope="shisho" id="google-books" size={40} />);
    expect(screen.getByText("GB")).toBeInTheDocument();
    expect(screen.queryByRole("img")).toBeNull();
  });

  it("swaps to initials when the <img> onError fires", () => {
    render(<PluginLogo scope="shisho" id="google-books" imageUrl="https://broken" size={40} />);
    const img = screen.getByRole("img");
    fireEvent.error(img);
    expect(screen.getByText("GB")).toBeInTheDocument();
    expect(screen.queryByRole("img")).toBeNull();
  });

  it("sizes the container to the size prop", () => {
    const { container } = render(
      <PluginLogo scope="shisho" id="google-books" size={64} />,
    );
    const wrapper = container.firstChild as HTMLElement;
    expect(wrapper).toHaveStyle({ width: "64px", height: "64px" });
  });
});
```

- [ ] **Step 6: Run test to verify it fails**

Run: `pnpm vitest run app/components/plugins/PluginLogo.test.tsx`
Expected: FAIL — module missing.

- [ ] **Step 7: Implement `PluginLogo`**

Create `app/components/plugins/PluginLogo.tsx`:

```tsx
import { useState } from "react";
import { cn } from "@/lib/utils";
import { derivePluginInitials, getPluginFallbackColor } from "./logoColor";

export interface PluginLogoProps {
  scope: string;
  id: string;
  imageUrl?: string | null;
  size: 24 | 40 | 56 | 64;
  className?: string;
}

const RADIUS_BY_SIZE: Record<PluginLogoProps["size"], number> = {
  24: 4,
  40: 6,
  56: 10,
  64: 12,
};

const PADDING_BY_SIZE: Record<PluginLogoProps["size"], number> = {
  24: 3,
  40: 6,
  56: 8,
  64: 10,
};

export const PluginLogo = ({ scope, id, imageUrl, size, className }: PluginLogoProps) => {
  const [hasError, setHasError] = useState(false);
  const showImage = !!imageUrl && !hasError;
  const radius = RADIUS_BY_SIZE[size];
  const padding = PADDING_BY_SIZE[size];

  return (
    <div
      className={cn(
        "inline-flex shrink-0 items-center justify-center overflow-hidden",
        className,
      )}
      style={{
        width: size,
        height: size,
        aspectRatio: "1 / 1",
        borderRadius: radius,
        backgroundColor: showImage
          ? "oklch(1 0 0 / 5%)"
          : getPluginFallbackColor(scope, id),
      }}
    >
      {showImage ? (
        <img
          src={imageUrl!}
          alt={`${id} logo`}
          onError={() => setHasError(true)}
          style={{
            width: "100%",
            height: "100%",
            padding,
            objectFit: "contain",
          }}
        />
      ) : (
        <span
          className="font-semibold text-white"
          style={{ fontSize: size * 0.4 }}
        >
          {derivePluginInitials(id)}
        </span>
      )}
    </div>
  );
};
```

- [ ] **Step 8: Run component tests to verify they pass**

Run: `pnpm vitest run app/components/plugins/PluginLogo.test.tsx`
Expected: PASS.

- [ ] **Step 9: Run full lint/typecheck**

Run: `mise lint:js`
Expected: passes.

- [ ] **Step 10: Commit**

```bash
git add app/components/plugins/PluginLogo.tsx app/components/plugins/PluginLogo.test.tsx app/components/plugins/logoColor.ts app/components/plugins/logoColor.test.ts
git commit -m "[Frontend] Add PluginLogo with initials fallback"
```

---

### Task 4: Plugin Detail Page — Route + Read-Only Hero Scaffold

**Why:** Spec §4. Establish the new `/settings/plugins/:scope/:id` route and the polymorphic container (Installed / Available-only / Neither) before wiring individual sections. This task lands the navigation target; subsequent tasks fill in the sections.

**Files:**
- Create: `app/components/pages/PluginDetail.tsx`
- Create: `app/components/plugins/PluginDetailHero.tsx`
- Modify: `app/router.tsx` (add `/settings/plugins/:scope/:id` route)

- [ ] **Step 1: Add the route**

In `app/router.tsx`, in the settings section (around the existing `plugins/:tab?` block at lines ~140–149), add the detail route below it. Keep the existing `:tab?` route for now; we'll clean it up in Task 14.

```tsx
{
  path: "plugins/:scope/:id",
  element: (
    <ProtectedRoute
      requiredPermission={{ resource: "config", operation: "read" }}
    >
      <PluginDetail />
    </ProtectedRoute>
  ),
},
```

Add the import at the top of the file: `import { PluginDetail } from "@/components/pages/PluginDetail";`

- [ ] **Step 2: Scaffold `PluginDetail`**

Create `app/components/pages/PluginDetail.tsx`:

```tsx
import { useParams, useNavigate } from "react-router";
import { ChevronLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { usePluginsInstalled, usePluginsAvailable } from "@/hooks/queries/plugins";
import { PluginDetailHero } from "@/components/plugins/PluginDetailHero";

export const PluginDetail = () => {
  const { scope, id } = useParams<{ scope: string; id: string }>();
  const navigate = useNavigate();
  const installedQuery = usePluginsInstalled();
  const availableQuery = usePluginsAvailable();

  const installed = installedQuery.data?.find(
    (p) => p.scope === scope && p.id === id,
  );
  const available = availableQuery.data?.find(
    (p) => p.scope === scope && p.id === id,
  );

  const isLoading = installedQuery.isLoading || availableQuery.isLoading;
  const notFound =
    !isLoading &&
    installedQuery.isSuccess &&
    availableQuery.isSuccess &&
    !installed &&
    !available;

  return (
    <div className="flex flex-col gap-6 p-6">
      <div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => navigate("/settings/plugins")}
        >
          <ChevronLeft className="mr-1 h-4 w-4" />
          Plugins
        </Button>
      </div>

      {isLoading && <PluginDetailSkeleton />}

      {notFound && (
        <div className="rounded-md border border-border p-8 text-center text-muted-foreground">
          <p className="text-lg">Plugin not found</p>
          <p className="mt-1 text-sm">
            No installed or available plugin matches <code>{scope}/{id}</code>.
          </p>
        </div>
      )}

      {!isLoading && !notFound && (
        <PluginDetailHero
          scope={scope!}
          id={id!}
          installed={installed}
          available={available}
        />
      )}
    </div>
  );
};

const PluginDetailSkeleton = () => (
  <div className="rounded-md border border-border p-6">
    <div className="flex gap-4">
      <div className="h-16 w-16 animate-pulse rounded-xl bg-muted" />
      <div className="flex-1 space-y-2">
        <div className="h-5 w-1/3 animate-pulse rounded bg-muted" />
        <div className="h-4 w-1/2 animate-pulse rounded bg-muted" />
        <div className="h-4 w-3/4 animate-pulse rounded bg-muted" />
      </div>
    </div>
  </div>
);
```

- [ ] **Step 3: Scaffold `PluginDetailHero` (read-only)**

Create `app/components/plugins/PluginDetailHero.tsx`:

```tsx
import type { Plugin } from "@/types/generated/models";
import type { AvailablePlugin } from "@/hooks/queries/plugins";
import { PluginLogo } from "./PluginLogo";
import { Badge } from "@/components/ui/badge";
import { ExternalLink } from "lucide-react";

export interface PluginDetailHeroProps {
  scope: string;
  id: string;
  installed?: Plugin;
  available?: AvailablePlugin;
}

export const PluginDetailHero = ({
  scope,
  id,
  installed,
  available,
}: PluginDetailHeroProps) => {
  const name = installed?.name ?? available?.name ?? id;
  const description = installed?.description ?? available?.description ?? "";
  const author = installed?.author ?? available?.author;
  const homepage = installed?.homepage ?? available?.homepage;
  const imageUrl = available?.imageUrl ?? undefined;
  const version = installed?.version;
  const updateAvailable = installed?.update_available_version ?? undefined;

  return (
    <div className="flex gap-4 rounded-md border border-border p-6">
      <PluginLogo scope={scope} id={id} imageUrl={imageUrl} size={64} />

      <div className="flex-1 space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold">{name}</h1>
          {updateAvailable && (
            <Badge variant="default">Update available — {updateAvailable}</Badge>
          )}
          {!installed && available && (
            <Badge variant="secondary">Not installed</Badge>
          )}
        </div>

        <p className="text-sm text-muted-foreground">
          {version && <span>v{version}</span>}
          {version && author && <span> · by {author}</span>}
          {!version && author && <span>by {author}</span>}
          {homepage && (
            <>
              {" · "}
              <a
                href={homepage}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 underline"
              >
                homepage <ExternalLink className="h-3 w-3" />
              </a>
            </>
          )}
        </p>

        {description && (
          <p className="text-sm text-muted-foreground">{description}</p>
        )}
      </div>
    </div>
  );
};
```

- [ ] **Step 4: Wire the `imageUrl` field on `AvailablePlugin` in the frontend type**

Check `app/hooks/queries/plugins.ts` — if `AvailablePlugin` (lines 56–67) doesn't already have `imageUrl: string`, add it. The Go struct has `ImageURL string \`json:"imageUrl"\`` so the wire format is correct; the TS type just needs the field.

- [ ] **Step 5: Verify the route navigates and renders the three states**

Run: `mise start` (if not already running) and visit:
- `/settings/plugins/shisho/google-books` (installed plugin) → hero renders
- `/settings/plugins/shisho/nonexistent` → "Plugin not found"
- An available-but-not-installed plugin (pick one from Discover) → hero renders "Not installed" badge

- [ ] **Step 6: Run lint/type checks**

Run: `mise lint:js`
Expected: passes.

- [ ] **Step 7: Commit**

```bash
git add app/components/pages/PluginDetail.tsx app/components/plugins/PluginDetailHero.tsx app/hooks/queries/plugins.ts app/router.tsx
git commit -m "[Frontend] Scaffold plugin detail page with read-only hero"
```

---

### Task 5: Extract `PluginConfigForm` + Mount in Detail Page

**Why:** Spec §4 "Configuration section" and §7 "New components". Current `PluginConfigDialog.tsx` (395 lines) both owns the dialog chrome and the form. We extract the form so the detail page can mount it inline under its own "Configuration" section, with page-level unsaved-changes protection via `useUnsavedChanges` (not dialog-level via `FormDialog`).

**Files:**
- Create: `app/components/plugins/PluginConfigForm.tsx`
- Modify: `app/components/plugins/PluginConfigDialog.tsx` (becomes a thin wrapper around the new form until we delete it in Task 16)
- Modify: `app/components/pages/PluginDetail.tsx` (mount the form)

- [ ] **Step 1: Write a test for `PluginConfigForm` rendering and save**

Create `app/components/plugins/PluginConfigForm.test.tsx` modeled on `app/components/library/RoleDialog.test.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { PluginConfigForm } from "./PluginConfigForm";

const mockSaveConfig = vi.fn();
const mockSaveFields = vi.fn();

vi.mock("@/hooks/queries/plugins", () => ({
  usePluginConfig: () => ({
    data: {
      schema: {
        apiKey: { type: "string", label: "API Key", required: true },
        maxResults: { type: "number", label: "Max Results", min: 1, max: 100 },
      },
      values: { apiKey: "", maxResults: 10 },
      declaredFields: [],
      fieldSettings: {},
      confidence_threshold: null,
    },
    isLoading: false,
  }),
  useSavePluginConfig: () => ({ mutateAsync: mockSaveConfig, isPending: false }),
  useSavePluginFieldSettings: () => ({ mutateAsync: mockSaveFields, isPending: false }),
}));

const wrap = (ui: React.ReactNode) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {ui}
  </QueryClientProvider>
);

describe("PluginConfigForm", () => {
  it("renders the declared schema fields", () => {
    render(wrap(<PluginConfigForm scope="shisho" id="test" />));
    expect(screen.getByLabelText(/api key/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/max results/i)).toBeInTheDocument();
  });

  it("calls save with updated values when the user clicks Save", async () => {
    render(wrap(<PluginConfigForm scope="shisho" id="test" />));
    fireEvent.change(screen.getByLabelText(/api key/i), { target: { value: "sk-123" } });
    fireEvent.click(screen.getByRole("button", { name: /save configuration/i }));
    await waitFor(() => {
      expect(mockSaveConfig).toHaveBeenCalledWith(
        expect.objectContaining({
          scope: "shisho",
          id: "test",
          values: expect.objectContaining({ apiKey: "sk-123" }),
        }),
      );
    });
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm vitest run app/components/plugins/PluginConfigForm.test.tsx`
Expected: FAIL — module missing.

- [ ] **Step 3: Extract the form from `PluginConfigDialog.tsx`**

Read `app/components/plugins/PluginConfigDialog.tsx`. Move:
- All config-rendering logic (`renderField`, form state, save handler, field toggles, confidence threshold input)
- The `onDirtyChange` prop pattern

…into a new `app/components/plugins/PluginConfigForm.tsx` that exposes:

```tsx
export interface PluginConfigFormProps {
  scope: string;
  id: string;
  onDirtyChange?: (isDirty: boolean) => void;
  onSaved?: () => void;
}

export const PluginConfigForm = (props: PluginConfigFormProps) => {
  // … form state, renderField, save handler …
  // Exposes:
  // - <h2>Configuration</h2>
  // - fields rendered
  // - field toggles (enrichers)
  // - confidence threshold
  // - Save configuration + Reset to defaults buttons
};
```

Then reduce `PluginConfigDialog.tsx` to a thin wrapper:

```tsx
export const PluginConfigDialog = ({ plugin, open, onOpenChange }: PluginConfigDialogProps) => {
  const [dirty, setDirty] = useState(false);
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      hasChanges={dirty}
      title={plugin ? `Configure ${plugin.name}` : ""}
    >
      {plugin && (
        <PluginConfigForm
          scope={plugin.scope}
          id={plugin.id}
          onDirtyChange={setDirty}
          onSaved={() => onOpenChange(false)}
        />
      )}
    </FormDialog>
  );
};
```

(Keep this wrapper for now — we delete it in Task 16 after confirming the detail page subsumes it.)

- [ ] **Step 4: Run `PluginConfigForm` tests**

Run: `pnpm vitest run app/components/plugins/PluginConfigForm.test.tsx`
Expected: PASS.

- [ ] **Step 5: Mount `PluginConfigForm` in `PluginDetail.tsx` (installed state only)**

In `app/components/pages/PluginDetail.tsx`, add a Configuration section rendered only when the plugin is installed. Also wire page-level unsaved-changes protection:

```tsx
import { useState } from "react";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import { PluginConfigForm } from "@/components/plugins/PluginConfigForm";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";

// Inside PluginDetail component, once `installed` is known:
const [configDirty, setConfigDirty] = useState(false);
const { showBlockerDialog, proceedNavigation, cancelNavigation } = useUnsavedChanges(configDirty);

// …then render, when installed is truthy:
<section className="space-y-3">
  <PluginConfigForm
    scope={scope!}
    id={id!}
    onDirtyChange={setConfigDirty}
  />
</section>
<UnsavedChangesDialog
  open={showBlockerDialog}
  onConfirm={proceedNavigation}
  onCancel={cancelNavigation}
/>
```

(Exact names — `UnsavedChangesDialog`, `useUnsavedChanges`'s returned shape — may differ. Read `app/hooks/useUnsavedChanges.ts` and the companion dialog before adapting.)

- [ ] **Step 6: Verify in the app**

- Visit `/settings/plugins/shisho/<installed-plugin>` → Configuration section renders.
- Edit a field, try to navigate away via the sidebar — confirm dialog appears.
- Save changes — dialog no longer appears on navigation.

- [ ] **Step 7: Run full lint/tests**

Run: `mise lint:js && pnpm vitest run app/components/plugins/PluginConfigForm.test.tsx`
Expected: passes.

- [ ] **Step 8: Commit**

```bash
git add app/components/plugins/PluginConfigForm.tsx app/components/plugins/PluginConfigForm.test.tsx app/components/plugins/PluginConfigDialog.tsx app/components/pages/PluginDetail.tsx
git commit -m "[Frontend] Extract PluginConfigForm and mount in detail page"
```

---

### Task 6: Version History + Markdown Changelog

**Why:** Spec §4 "Version history section" and the markdown risk in §11. The detail page lists every version from the available-plugin entry with installed/available-now/plain variants, with changelog rendered as sanitized markdown.

**Files:**
- Modify: `package.json` (add `react-markdown`, `rehype-sanitize`)
- Create: `app/components/plugins/ChangelogMarkdown.tsx`
- Create: `app/components/plugins/ChangelogMarkdown.test.tsx`
- Create: `app/components/plugins/PluginVersionCard.tsx`
- Create: `app/components/plugins/PluginVersionHistory.tsx`
- Modify: `app/components/pages/PluginDetail.tsx` (mount history)

- [ ] **Step 1: Add markdown deps**

Run:
```bash
pnpm add react-markdown rehype-sanitize
```

Verify `package.json` `dependencies` (not `devDependencies`) received both entries (per root CLAUDE.md note on Docker build layout).

- [ ] **Step 2: Write the `ChangelogMarkdown` test first**

Create `app/components/plugins/ChangelogMarkdown.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ChangelogMarkdown } from "./ChangelogMarkdown";

describe("ChangelogMarkdown", () => {
  it("renders h2/h3 headings and paragraphs", () => {
    render(
      <ChangelogMarkdown>
        {"## What's New\n\nAdded thing.\n\n### Details\n\nMore info."}
      </ChangelogMarkdown>,
    );
    expect(screen.getByRole("heading", { level: 2, name: /what's new/i })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 3, name: /details/i })).toBeInTheDocument();
  });

  it("renders lists, inline code, and code blocks", () => {
    render(
      <ChangelogMarkdown>
        {"- item 1\n- item 2\n\nUse `foo()` or:\n\n```js\nconsole.log(1)\n```"}
      </ChangelogMarkdown>,
    );
    expect(screen.getByText("item 1")).toBeInTheDocument();
    expect(screen.getByText("foo()")).toBeInTheDocument();
    expect(screen.getByText("console.log(1)")).toBeInTheDocument();
  });

  it("opens links in a new tab with rel=noopener noreferrer", () => {
    render(<ChangelogMarkdown>{"[link](https://example.com)"}</ChangelogMarkdown>);
    const link = screen.getByRole("link", { name: /link/i });
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", expect.stringContaining("noopener"));
  });

  it("strips images, iframes, and raw html", () => {
    render(
      <ChangelogMarkdown>
        {"![img](x.png)\n\n<iframe src='x'></iframe>\n\n<script>alert(1)</script>"}
      </ChangelogMarkdown>,
    );
    expect(document.querySelector("img")).toBeNull();
    expect(document.querySelector("iframe")).toBeNull();
    expect(document.querySelector("script")).toBeNull();
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `pnpm vitest run app/components/plugins/ChangelogMarkdown.test.tsx`
Expected: FAIL — module missing.

- [ ] **Step 4: Implement `ChangelogMarkdown`**

Create `app/components/plugins/ChangelogMarkdown.tsx`:

```tsx
import ReactMarkdown from "react-markdown";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import type { Components } from "react-markdown";
import { cn } from "@/lib/utils";

const schema = {
  ...defaultSchema,
  tagNames: ["h2", "h3", "p", "ul", "ol", "li", "code", "pre", "a", "strong", "em"],
  attributes: {
    a: ["href", "title"],
    code: [],
    pre: [],
  },
};

const components: Components = {
  a: ({ node, ...props }) => (
    <a {...props} target="_blank" rel="noopener noreferrer" />
  ),
};

export interface ChangelogMarkdownProps {
  children: string;
  className?: string;
}

export const ChangelogMarkdown = ({ children, className }: ChangelogMarkdownProps) => (
  <div className={cn("prose prose-sm prose-invert max-w-none", className)}>
    <ReactMarkdown
      rehypePlugins={[[rehypeSanitize, schema]]}
      components={components}
    >
      {children}
    </ReactMarkdown>
  </div>
);
```

(If the existing Tailwind config doesn't have the typography plugin, drop the `prose*` classes and style headings/lists manually. Confirm first by grepping for `@tailwindcss/typography` in `tailwind.config` and `package.json`.)

- [ ] **Step 5: Run test to verify it passes**

Run: `pnpm vitest run app/components/plugins/ChangelogMarkdown.test.tsx`
Expected: PASS.

- [ ] **Step 6: Implement `PluginVersionCard`**

Create `app/components/plugins/PluginVersionCard.tsx`:

```tsx
import type { PluginVersion } from "@/hooks/queries/plugins";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ExternalLink } from "lucide-react";
import { cn } from "@/lib/utils";
import { ChangelogMarkdown } from "./ChangelogMarkdown";

export interface PluginVersionCardProps {
  version: PluginVersion;
  state: "installed" | "available" | "latest" | "older";
  gitHubDiffUrl?: string;
  onUpdate?: () => void;
  isUpdating?: boolean;
}

const formatReleaseDate = (raw: string): { absolute: string; relative: string } | null => {
  if (!raw) return null;
  const d = new Date(raw);
  if (isNaN(d.getTime())) return null;
  const absolute = d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
  const diffDays = Math.floor((Date.now() - d.getTime()) / (1000 * 60 * 60 * 24));
  const relative =
    diffDays < 1 ? "today" :
    diffDays === 1 ? "yesterday" :
    diffDays < 30 ? `${diffDays} days ago` :
    diffDays < 365 ? `${Math.floor(diffDays / 30)} months ago` :
    `${Math.floor(diffDays / 365)} years ago`;
  return { absolute, relative };
};

export const PluginVersionCard = ({
  version,
  state,
  gitHubDiffUrl,
  onUpdate,
  isUpdating,
}: PluginVersionCardProps) => {
  const date = formatReleaseDate(version.releaseDate);
  return (
    <div
      className={cn(
        "space-y-3 rounded-md border p-4",
        state === "available" && "border-primary/50 bg-primary/5",
        state === "latest" && "border-primary/50 bg-primary/5",
      )}
    >
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="font-medium">v{version.version}</span>
          {state === "available" && <Badge>Available now</Badge>}
          {state === "latest" && <Badge>Latest</Badge>}
          {state === "installed" && <Badge variant="secondary">Installed</Badge>}
        </div>
        {date && (
          <span className="text-xs text-muted-foreground">
            Released {date.absolute} · {date.relative}
          </span>
        )}
      </div>

      {version.changelog && <ChangelogMarkdown>{version.changelog}</ChangelogMarkdown>}

      {(onUpdate || gitHubDiffUrl) && (
        <div className="flex items-center gap-2">
          {onUpdate && (
            <Button size="sm" onClick={onUpdate} disabled={isUpdating}>
              {isUpdating ? "Updating…" : "Update now"}
            </Button>
          )}
          {gitHubDiffUrl && (
            <a
              href={gitHubDiffUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-xs underline"
            >
              View full diff on GitHub <ExternalLink className="h-3 w-3" />
            </a>
          )}
        </div>
      )}
    </div>
  );
};
```

- [ ] **Step 7: Implement `PluginVersionHistory`**

Create `app/components/plugins/PluginVersionHistory.tsx`:

```tsx
import { useMemo, useState } from "react";
import type { Plugin } from "@/types/generated/models";
import type { AvailablePlugin } from "@/hooks/queries/plugins";
import { useUpdatePluginVersion } from "@/hooks/queries/plugins";
import { PluginVersionCard } from "./PluginVersionCard";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";

const INITIAL_VISIBLE = 3;

export interface PluginVersionHistoryProps {
  installed?: Plugin;
  available?: AvailablePlugin;
}

export const PluginVersionHistory = ({ installed, available }: PluginVersionHistoryProps) => {
  const versions = available?.versions ?? [];
  const installedVersion = installed?.version;
  const updateVersion = useUpdatePluginVersion();
  const [expanded, setExpanded] = useState(false);

  const [newerVersions, olderVersions] = useMemo(() => {
    if (!installedVersion) {
      // Available-only: highest compatible first, then rest
      const compatible = versions.filter((v) => v.compatible !== false);
      return [compatible.slice(0, 1), compatible.slice(1)];
    }
    const installedIdx = versions.findIndex((v) => v.version === installedVersion);
    const newer = installedIdx > 0 ? versions.slice(0, installedIdx) : [];
    const rest = installedIdx >= 0 ? versions.slice(installedIdx) : versions;
    return [newer, rest];
  }, [versions, installedVersion]);

  const gitHubDiffUrl = (homepage: string | undefined | null, version: string) => {
    if (!homepage || !homepage.includes("github.com")) return undefined;
    return `${homepage.replace(/\/$/, "")}/releases/tag/v${version}`;
  };

  const handleUpdate = async (version: string) => {
    if (!installed) return;
    try {
      await updateVersion.mutateAsync({
        scope: installed.scope,
        id: installed.id,
        version,
      });
      toast.success(`Updated to v${version}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Update failed");
    }
  };

  if (versions.length === 0) return null;
  const visibleOlder = expanded ? olderVersions : olderVersions.slice(0, INITIAL_VISIBLE);
  const hiddenCount = olderVersions.length - visibleOlder.length;

  return (
    <section className="space-y-4">
      <h2 className="text-lg font-semibold">Version history</h2>

      {newerVersions.map((v) => (
        <PluginVersionCard
          key={v.version}
          version={v}
          state={installedVersion ? "available" : "latest"}
          gitHubDiffUrl={gitHubDiffUrl(installed?.homepage ?? available?.homepage, v.version)}
          onUpdate={installedVersion ? () => handleUpdate(v.version) : undefined}
          isUpdating={updateVersion.isPending}
        />
      ))}

      {visibleOlder.map((v) => (
        <PluginVersionCard
          key={v.version}
          version={v}
          state={v.version === installedVersion ? "installed" : "older"}
          gitHubDiffUrl={gitHubDiffUrl(installed?.homepage ?? available?.homepage, v.version)}
        />
      ))}

      {hiddenCount > 0 && (
        <Button variant="ghost" size="sm" onClick={() => setExpanded(true)}>
          Show {hiddenCount} older version{hiddenCount === 1 ? "" : "s"}
        </Button>
      )}
    </section>
  );
};
```

- [ ] **Step 8: Mount history in `PluginDetail.tsx`**

Between the hero and the configuration section:

```tsx
<PluginVersionHistory installed={installed} available={available} />
```

- [ ] **Step 9: Verify in the app**

Navigate to an installed plugin detail page. Confirm version cards render, the installed version has a green "Installed" badge, newer versions have "Available now" with an Update button that works, and changelogs render with markdown formatting.

- [ ] **Step 10: Run tests and lint**

Run: `pnpm vitest run app/components/plugins/ChangelogMarkdown.test.tsx && mise lint:js`
Expected: passes.

- [ ] **Step 11: Commit**

```bash
git add package.json pnpm-lock.yaml app/components/plugins/ChangelogMarkdown.tsx app/components/plugins/ChangelogMarkdown.test.tsx app/components/plugins/PluginVersionCard.tsx app/components/plugins/PluginVersionHistory.tsx app/components/pages/PluginDetail.tsx
git commit -m "[Frontend] Add plugin version history with sanitized markdown changelog"
```

---

### Task 7: Permissions Section

**Why:** Spec §4 "Permissions section". Read-only derivation from the plugin's declared capabilities.

**Files:**
- Create: `app/components/plugins/PluginPermissions.tsx`
- Modify: `app/components/pages/PluginDetail.tsx`

- [ ] **Step 1: Implement the component**

Create `app/components/plugins/PluginPermissions.tsx`:

```tsx
import type { Plugin } from "@/types/generated/models";
import type { AvailablePlugin, PluginCapabilities } from "@/hooks/queries/plugins";
import { Globe, FolderOpen, Film, Terminal } from "lucide-react";

interface PluginPermissionsProps {
  installed?: Plugin;
  available?: AvailablePlugin;
}

type Row = { icon: typeof Globe; label: string; detail: string };

const buildRows = (caps?: PluginCapabilities | null): Row[] => {
  if (!caps) return [];
  const rows: Row[] = [];
  if (caps.httpAccess?.domains?.length) {
    rows.push({
      icon: Globe,
      label: "Network access",
      detail: caps.httpAccess.domains.join(", "),
    });
  }
  if (caps.fileAccess?.level) {
    rows.push({
      icon: FolderOpen,
      label: "Filesystem access",
      detail: caps.fileAccess.level,
    });
  }
  if (caps.ffmpegAccess) {
    rows.push({ icon: Film, label: "FFmpeg access", detail: "" });
  }
  if (caps.shellAccess?.commands?.length) {
    rows.push({
      icon: Terminal,
      label: "Shell commands",
      detail: caps.shellAccess.commands.join(", "),
    });
  }
  return rows;
};

export const PluginPermissions = ({ installed, available }: PluginPermissionsProps) => {
  // Prefer installed (authoritative from manifest.json) over available (repo-declared)
  const caps = installed?.capabilities ?? available?.versions?.[0]?.capabilities ?? null;
  const rows = buildRows(caps);
  if (rows.length === 0) return null;

  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold">Permissions</h2>
      <div className="space-y-2">
        {rows.map(({ icon: Icon, label, detail }) => (
          <div key={label} className="flex items-start gap-3 rounded-md border p-3">
            <Icon className="mt-0.5 h-4 w-4 text-muted-foreground" />
            <div className="flex-1">
              <div className="text-sm font-medium">{label}</div>
              {detail && <div className="text-xs text-muted-foreground">{detail}</div>}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
};
```

(Verify the `Plugin` model has a `capabilities` field. If not, the component falls back to `available?.versions?.[0]?.capabilities` — that's why the fallback chain is there.)

- [ ] **Step 2: Mount in `PluginDetail.tsx`**

Add after the version history (before Configuration):

```tsx
<PluginPermissions installed={installed} available={available} />
```

- [ ] **Step 3: Verify**

Visit a plugin with `httpAccess`, `fileAccess`, etc. declared — rows render. Visit one with no capabilities — section hides entirely.

- [ ] **Step 4: Commit**

```bash
git add app/components/plugins/PluginPermissions.tsx app/components/pages/PluginDetail.tsx
git commit -m "[Frontend] Add permissions section to plugin detail page"
```

---

### Task 8: Danger Zone (Uninstall)

**Why:** Spec §4 "Danger zone". The confirm dialog is an explicit exception to the no-confirms rule because uninstall is destructive.

**Files:**
- Create: `app/components/plugins/PluginDangerZone.tsx`
- Modify: `app/components/pages/PluginDetail.tsx`

- [ ] **Step 1: Implement `PluginDangerZone`**

Create `app/components/plugins/PluginDangerZone.tsx`:

```tsx
import { useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { useUninstallPlugin } from "@/hooks/queries/plugins";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import type { Plugin } from "@/types/generated/models";

export interface PluginDangerZoneProps {
  plugin: Plugin;
}

export const PluginDangerZone = ({ plugin }: PluginDangerZoneProps) => {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const navigate = useNavigate();
  const uninstall = useUninstallPlugin();

  const handleUninstall = async () => {
    try {
      await uninstall.mutateAsync({ scope: plugin.scope, id: plugin.id });
      toast.success(`${plugin.name} uninstalled`);
      navigate("/settings/plugins");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Uninstall failed");
    }
  };

  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold text-destructive">Danger zone</h2>
      <div className="flex items-center justify-between gap-4 rounded-md border border-destructive/40 p-4">
        <div>
          <div className="text-sm font-medium">Uninstall plugin</div>
          <div className="text-xs text-muted-foreground">
            Removes the plugin and its files. Plugin configuration will be discarded.
          </div>
        </div>
        <Button variant="destructive" onClick={() => setConfirmOpen(true)}>
          Uninstall
        </Button>
      </div>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Uninstall plugin"
        description={`Are you sure you want to uninstall "${plugin.name}"? This cannot be undone.`}
        confirmLabel="Uninstall"
        variant="destructive"
        isPending={uninstall.isPending}
        onConfirm={handleUninstall}
      />
    </section>
  );
};
```

- [ ] **Step 2: Mount in `PluginDetail.tsx`**

At the bottom, gated on `installed`:

```tsx
{installed && <PluginDangerZone plugin={installed} />}
```

- [ ] **Step 3: Verify**

Install a test plugin → visit detail → click Uninstall → confirm → land on `/settings/plugins`, plugin gone.

- [ ] **Step 4: Commit**

```bash
git add app/components/plugins/PluginDangerZone.tsx app/components/pages/PluginDetail.tsx
git commit -m "[Frontend] Add danger zone / uninstall to plugin detail page"
```

---

### Task 9: Icon Actions — Reload + View Manifest

**Why:** Spec §4 hero right-rail icon actions. Reload is local-only and uses the existing endpoint. View Manifest opens a dialog that fetches from the endpoint added in Task 2.

**Files:**
- Create: `app/components/plugins/PluginManifestDialog.tsx`
- Create: `app/components/plugins/PluginHeroActions.tsx`
- Modify: `app/components/plugins/PluginDetailHero.tsx`

- [ ] **Step 1: Implement `PluginManifestDialog`**

Create `app/components/plugins/PluginManifestDialog.tsx`:

```tsx
import { usePluginManifest } from "@/hooks/queries/plugins";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";

export interface PluginManifestDialogProps {
  scope: string;
  id: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const PluginManifestDialog = ({ scope, id, open, onOpenChange }: PluginManifestDialogProps) => {
  const { data, isLoading, error } = usePluginManifest(scope, id, { enabled: open });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80vh] max-w-2xl overflow-hidden">
        <DialogHeader>
          <DialogTitle>Plugin manifest</DialogTitle>
        </DialogHeader>
        <div className="overflow-auto rounded-md border bg-muted/30 p-4">
          {isLoading && <div className="text-sm text-muted-foreground">Loading…</div>}
          {error && <div className="text-sm text-destructive">{error.message}</div>}
          {data && (
            <pre className="text-xs">
              <code>{JSON.stringify(data, null, 2)}</code>
            </pre>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
};
```

(Plain `<pre>` with `JSON.stringify` — no syntax-highlighter dependency. The spec's reference to "already in the app" was incorrect; adding one is YAGNI for this dialog.)

- [ ] **Step 2: Implement `PluginHeroActions`**

Create `app/components/plugins/PluginHeroActions.tsx`:

```tsx
import { useState } from "react";
import { toast } from "sonner";
import { RotateCw, FileText } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useReloadPlugin } from "@/hooks/queries/plugins";
import type { Plugin } from "@/types/generated/models";
import { PluginManifestDialog } from "./PluginManifestDialog";

export interface PluginHeroActionsProps {
  plugin: Plugin;
}

export const PluginHeroActions = ({ plugin }: PluginHeroActionsProps) => {
  const [manifestOpen, setManifestOpen] = useState(false);
  const reload = useReloadPlugin();

  const handleReload = async () => {
    try {
      await reload.mutateAsync({ scope: plugin.scope, id: plugin.id });
      toast.success(`${plugin.name} reloaded from disk`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Reload failed");
    }
  };

  return (
    <>
      <div className="flex items-center gap-1">
        {plugin.scope === "local" && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                onClick={handleReload}
                disabled={reload.isPending}
              >
                <RotateCw className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Reload plugin from disk</TooltipContent>
          </Tooltip>
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" onClick={() => setManifestOpen(true)}>
              <FileText className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>View manifest</TooltipContent>
        </Tooltip>
      </div>
      <PluginManifestDialog
        scope={plugin.scope}
        id={plugin.id}
        open={manifestOpen}
        onOpenChange={setManifestOpen}
      />
    </>
  );
};
```

- [ ] **Step 3: Wire actions + enable/disable toggle into the hero**

Modify `app/components/plugins/PluginDetailHero.tsx` to accept an `actions` slot in the right rail, and render the enable/disable toggle + `PluginHeroActions` + update button:

```tsx
// Inside PluginDetailHero, add right rail actions block:
{installed && (
  <div className="flex flex-col items-end gap-3">
    {updateAvailable && (
      <Button onClick={onUpdate} disabled={isUpdating}>
        {isUpdating ? "Updating…" : `Update to ${updateAvailable}`}
      </Button>
    )}
    <div className="flex items-center gap-2">
      <Label htmlFor="plugin-enabled">Enabled</Label>
      <Switch
        id="plugin-enabled"
        checked={installed.status === "active"}
        onCheckedChange={onToggleEnabled}
      />
    </div>
    <PluginHeroActions plugin={installed} />
  </div>
)}
```

Pass `onUpdate`, `onToggleEnabled`, `isUpdating`, etc. as props from `PluginDetail.tsx`. Wire with existing `useUpdatePlugin()` and `useUpdatePluginVersion()` mutations. The enable/disable toggle uses `useUpdatePlugin` with `{ enabled }`.

- [ ] **Step 4: Verify**

- Visit a local-scope plugin → Reload icon visible; click reloads successfully.
- Visit a repo-installed plugin → only View manifest icon shown.
- Click View manifest → dialog shows indented JSON.
- Toggle Enabled → plugin enables/disables; state reflected on list page after nav back.
- Click Update to X.Y.Z → in-place update, badge removed.

- [ ] **Step 5: Commit**

```bash
git add app/components/plugins/PluginManifestDialog.tsx app/components/plugins/PluginHeroActions.tsx app/components/plugins/PluginDetailHero.tsx app/components/pages/PluginDetail.tsx
git commit -m "[Frontend] Add reload, view manifest, and enable/update controls to plugin detail hero"
```

---

### Task 10: Shared `PluginRow` + Installed-Tab Refactor

**Why:** Spec §2 and §7. Both tabs share a row component. The Installed tab loses its inline enable/disable switch (moved to detail page), gains row-as-link behavior, and uses `PluginLogo`. Capabilities become inline badges on the meta line.

**Files:**
- Create: `app/components/plugins/PluginRow.tsx`
- Create: `app/components/plugins/PluginRow.test.tsx`
- Create: `app/components/plugins/InstalledTab.tsx` (extract from AdminPlugins)
- Modify: `app/components/pages/AdminPlugins.tsx` (remove inline InstalledTab, mount new one)

- [ ] **Step 1: Write the `PluginRow` test**

Create `app/components/plugins/PluginRow.test.tsx`:

```tsx
import { MemoryRouter } from "react-router";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PluginRow } from "./PluginRow";

const base = {
  scope: "shisho",
  id: "test",
  name: "Test",
  version: "1.0.0",
  author: "Me",
  description: "A test plugin.",
  imageUrl: undefined,
  capabilities: [],
  href: "/settings/plugins/shisho/test",
};

const wrap = (ui: React.ReactNode) => <MemoryRouter>{ui}</MemoryRouter>;

describe("PluginRow", () => {
  it("renders name, version, and author on meta line", () => {
    render(wrap(<PluginRow {...base} />));
    expect(screen.getByText("Test")).toBeInTheDocument();
    expect(screen.getByText(/v1\.0\.0/)).toBeInTheDocument();
    expect(screen.getByText(/Me/)).toBeInTheDocument();
  });

  it("renders the Disabled badge when disabled=true", () => {
    render(wrap(<PluginRow {...base} disabled />));
    expect(screen.getByText(/disabled/i)).toBeInTheDocument();
  });

  it("renders capability badges on meta line", () => {
    render(
      wrap(
        <PluginRow
          {...base}
          capabilities={["Metadata enricher", "File parser"]}
        />,
      ),
    );
    expect(screen.getByText("Metadata enricher")).toBeInTheDocument();
    expect(screen.getByText("File parser")).toBeInTheDocument();
  });

  it("renders the Update badge when updateAvailable is set", () => {
    render(wrap(<PluginRow {...base} updateAvailable="1.5.0" />));
    expect(screen.getByText(/update 1\.5\.0/i)).toBeInTheDocument();
  });

  it("links the whole row to href", () => {
    render(wrap(<PluginRow {...base} />));
    expect(screen.getByRole("link")).toHaveAttribute(
      "href",
      "/settings/plugins/shisho/test",
    );
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm vitest run app/components/plugins/PluginRow.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Implement `PluginRow`**

Create `app/components/plugins/PluginRow.tsx`:

```tsx
import { Link } from "react-router";
import { ChevronRight } from "lucide-react";
import { PluginLogo } from "./PluginLogo";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

export interface PluginRowProps {
  scope: string;
  id: string;
  name: string;
  version?: string;
  author?: string;
  description?: string;
  imageUrl?: string | null;
  capabilities: string[];
  href: string;
  disabled?: boolean;
  updateAvailable?: string;
  actions?: React.ReactNode;
}

export const PluginRow = ({
  scope,
  id,
  name,
  version,
  author,
  description,
  imageUrl,
  capabilities,
  href,
  disabled,
  updateAvailable,
  actions,
}: PluginRowProps) => {
  return (
    <Link
      to={href}
      className={cn(
        "group flex items-center gap-4 rounded-md border border-border p-4 transition-colors hover:bg-accent/30",
        disabled && "opacity-50 saturate-50",
      )}
    >
      <PluginLogo scope={scope} id={id} imageUrl={imageUrl ?? undefined} size={40} />

      <div className="flex-1 min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium truncate">{name}</span>
          {disabled && <Badge variant="secondary">Disabled</Badge>}
          {updateAvailable && <Badge>Update {updateAvailable}</Badge>}
        </div>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {version && <span>v{version}</span>}
          {capabilities.map((cap) => (
            <Badge key={cap} variant="outline">{cap}</Badge>
          ))}
          {author && <span>· {author}</span>}
        </div>
        {description && (
          <p className="text-xs text-muted-foreground line-clamp-2">{description}</p>
        )}
      </div>

      {actions && (
        <div
          className="flex items-center gap-2"
          onClick={(e) => e.stopPropagation()}
          onMouseDown={(e) => e.stopPropagation()}
        >
          {actions}
        </div>
      )}

      <ChevronRight className="h-4 w-4 flex-shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
    </Link>
  );
};
```

- [ ] **Step 4: Test passes**

Run: `pnpm vitest run app/components/plugins/PluginRow.test.tsx`
Expected: PASS.

- [ ] **Step 5: Extract `InstalledTab` into its own component**

Create `app/components/plugins/InstalledTab.tsx`. Move the installed-list logic from `AdminPlugins.tsx:66–318` into this file. Use `PluginRow` with an `actions` slot containing only the Update button (no switch, no reload, no settings, no uninstall — those move to detail page). Sort order: enabled first alphabetical, then a 1px separator, then disabled alphabetical.

Key structure:

```tsx
export const InstalledTab = ({ canWrite }: { canWrite: boolean }) => {
  const { data: plugins = [] } = usePluginsInstalled();
  const updatePluginVersion = useUpdatePluginVersion();

  const sorted = useMemo(() => {
    const enabled = plugins.filter((p) => p.status === "active").sort((a, b) => a.name.localeCompare(b.name));
    const disabled = plugins.filter((p) => p.status !== "active").sort((a, b) => a.name.localeCompare(b.name));
    return { enabled, disabled };
  }, [plugins]);

  const capabilityLabels = (p: Plugin): string[] => {
    // Derive badges from p.capabilities: "Metadata enricher", "File parser", etc.
    // See AdminPlugins.tsx for the existing logic; move it into a pluginCapabilities.ts helper.
  };

  const handleUpdate = async (p: Plugin) => {
    if (!p.update_available_version) return;
    try {
      await updatePluginVersion.mutateAsync({
        scope: p.scope,
        id: p.id,
        version: p.update_available_version,
      });
      toast.success(`Updated ${p.name} to ${p.update_available_version}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Update failed");
    }
  };

  return (
    <div className="space-y-2">
      {sorted.enabled.map((p) => (
        <PluginRow
          key={`${p.scope}/${p.id}`}
          scope={p.scope}
          id={p.id}
          name={p.name}
          version={p.version}
          author={p.author ?? undefined}
          description={p.description ?? undefined}
          imageUrl={/* derived via repo lookup helper — see Task 10.5 below */}
          capabilities={capabilityLabels(p)}
          href={`/settings/plugins/${p.scope}/${p.id}`}
          updateAvailable={p.update_available_version ?? undefined}
          actions={
            canWrite && p.update_available_version ? (
              <Button
                size="sm"
                onClick={() => handleUpdate(p)}
                disabled={updatePluginVersion.isPending}
              >
                Update
              </Button>
            ) : null
          }
        />
      ))}

      {sorted.disabled.length > 0 && sorted.enabled.length > 0 && (
        <div className="my-2 border-t border-border" />
      )}

      {sorted.disabled.map((p) => (
        <PluginRow
          key={`${p.scope}/${p.id}`}
          scope={p.scope}
          id={p.id}
          name={p.name}
          version={p.version}
          author={p.author ?? undefined}
          description={p.description ?? undefined}
          imageUrl={/* ... */}
          capabilities={capabilityLabels(p)}
          href={`/settings/plugins/${p.scope}/${p.id}`}
          disabled
        />
      ))}
    </div>
  );
};
```

- [ ] **Step 6: Create `pluginCapabilities.ts` helper**

Extract capability-label derivation into `app/components/plugins/pluginCapabilities.ts`:

```ts
import type { Plugin } from "@/types/generated/models";
import type { PluginCapabilities } from "@/hooks/queries/plugins";

export const deriveCapabilityLabels = (caps: PluginCapabilities | null | undefined): string[] => {
  if (!caps) return [];
  const labels: string[] = [];
  if (caps.metadataEnricher) labels.push("Metadata enricher");
  if (caps.inputConverter) labels.push("Input converter");
  if (caps.fileParser) labels.push("File parser");
  if (caps.outputGenerator) labels.push("Output generator");
  return labels;
};

export const deriveCapabilityLabelsFromPlugin = (p: Plugin): string[] =>
  deriveCapabilityLabels(p.capabilities ?? null);
```

(Add `capabilities` field to `Plugin` TS type if not present — regenerate via `mise tygo` if the Go struct has it; otherwise adapt to however capabilities are exposed on `Plugin` today.)

- [ ] **Step 7: Create image-URL lookup helper**

Create `app/components/plugins/useInstalledPluginImageUrl.ts` — a small hook that joins an installed plugin to its repo entry via `(scope, id)` to pull `imageUrl`:

```ts
import { usePluginsAvailable } from "@/hooks/queries/plugins";

export const useInstalledPluginImageUrl = () => {
  const { data: available = [] } = usePluginsAvailable();
  return (scope: string, id: string): string | undefined =>
    available.find((p) => p.scope === scope && p.id === id)?.imageUrl || undefined;
};
```

Used in `InstalledTab` to populate `PluginRow`'s `imageUrl` prop.

- [ ] **Step 8: Wire new `InstalledTab` into `AdminPlugins.tsx`**

Replace the inline `InstalledTab` component (lines 66–318) usage with an import from the new file. Leave the rest of `AdminPlugins.tsx` (BrowseTab, OrderTab, RepositoriesTab) untouched for now — those are Tasks 11 and 13.

- [ ] **Step 9: Verify**

Visit `/settings/plugins/installed`. Row layout matches spec: logo, name, version + capabilities on meta line, Update button when applicable, clickable entire row, disabled plugins dimmed and sorted below separator. No switch, no reload/settings/uninstall buttons.

- [ ] **Step 10: Run tests and lint**

Run: `pnpm vitest run app/components/plugins && mise lint:js`
Expected: passes.

- [ ] **Step 11: Commit**

```bash
git add app/components/plugins/PluginRow.tsx app/components/plugins/PluginRow.test.tsx app/components/plugins/InstalledTab.tsx app/components/plugins/pluginCapabilities.ts app/components/plugins/useInstalledPluginImageUrl.ts app/components/pages/AdminPlugins.tsx
git commit -m "[Frontend] Add shared PluginRow and refactor installed tab"
```

---

### Task 11: Rename Browse → Discover; Share `PluginRow`

**Why:** Spec §3. Discover tab shares the row layout with Installed, but its `actions` slot renders Install / Installed / Incompatible button states. Also adds the filter row (search + capability + source).

**Files:**
- Create: `app/components/plugins/DiscoverTab.tsx`
- Modify: `app/components/pages/AdminPlugins.tsx`

- [ ] **Step 1: Implement `DiscoverTab`**

Create `app/components/plugins/DiscoverTab.tsx`. Move `BrowseTab` logic (`AdminPlugins.tsx:322–458`) here and adapt:

```tsx
export const DiscoverTab = ({ canWrite }: { canWrite: boolean }) => {
  const { data: available = [] } = usePluginsAvailable();
  const { data: installed = [] } = usePluginsInstalled();
  const installPlugin = useInstallPlugin();

  const [search, setSearch] = useState("");
  const [capability, setCapability] = useState<string>("all");
  const [source, setSource] = useState<string>("all");

  const installedKeys = useMemo(
    () => new Set(installed.map((p) => `${p.scope}/${p.id}`)),
    [installed],
  );

  const sources = useMemo(() => {
    const set = new Set(available.map((p) => p.scope));
    return ["all", ...Array.from(set).sort()];
  }, [available]);

  const filtered = useMemo(() => {
    return available.filter((p) => {
      if (search && !p.name.toLowerCase().includes(search.toLowerCase()) &&
          !(p.description ?? "").toLowerCase().includes(search.toLowerCase())) {
        return false;
      }
      if (capability !== "all") {
        const caps = p.versions[0]?.capabilities;
        if (capability === "metadataEnricher" && !caps?.metadataEnricher) return false;
        if (capability === "inputConverter" && !caps?.inputConverter) return false;
        if (capability === "fileParser" && !caps?.fileParser) return false;
        if (capability === "outputGenerator" && !caps?.outputGenerator) return false;
      }
      if (source !== "all" && p.scope !== source) return false;
      return true;
    });
  }, [available, search, capability, source]);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          placeholder="Search plugins…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="max-w-xs"
        />
        <Select value={capability} onValueChange={setCapability}>
          <SelectTrigger className="w-48"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All capabilities</SelectItem>
            <SelectItem value="metadataEnricher">Metadata Enricher</SelectItem>
            <SelectItem value="inputConverter">Input Converter</SelectItem>
            <SelectItem value="fileParser">File Parser</SelectItem>
            <SelectItem value="outputGenerator">Output Generator</SelectItem>
          </SelectContent>
        </Select>
        <Select value={source} onValueChange={setSource}>
          <SelectTrigger className="w-48"><SelectValue /></SelectTrigger>
          <SelectContent>
            {sources.map((s) => (
              <SelectItem key={s} value={s}>{s === "all" ? "All sources" : s}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2">
        {filtered.map((p) => {
          const key = `${p.scope}/${p.id}`;
          const isInstalled = installedKeys.has(key);
          const incompatible = p.compatible === false;
          return (
            <PluginRow
              key={key}
              scope={p.scope}
              id={p.id}
              name={p.name}
              version={p.versions[0]?.version}
              author={p.author ?? undefined}
              description={p.description ?? undefined}
              imageUrl={p.imageUrl}
              capabilities={deriveCapabilityLabels(p.versions[0]?.capabilities)}
              href={`/settings/plugins/${p.scope}/${p.id}`}
              actions={
                canWrite ? (
                  isInstalled ? (
                    <Button size="sm" variant="outline" disabled>
                      <Check className="mr-1 h-3 w-3" />
                      Installed
                    </Button>
                  ) : incompatible ? (
                    <Button size="sm" variant="outline" disabled className="opacity-50">
                      Incompatible
                    </Button>
                  ) : (
                    <Button
                      size="sm"
                      onClick={() => installPlugin.mutate({ scope: p.scope, id: p.id })}
                      disabled={installPlugin.isPending}
                    >
                      Install
                    </Button>
                  )
                ) : null
              }
            />
          );
        })}
      </div>
    </div>
  );
};
```

(Adapt `Select` imports to match shadcn/ui's API in this repo; see `BrowseTab` for the current pattern.)

- [ ] **Step 2: Wire into `AdminPlugins.tsx`**

Replace the inline `BrowseTab` usage with `DiscoverTab`. Rename the tab trigger label in the Tabs list from "Browse" to "Discover". Update `validTabs` to include both `"discover"` and `"browse"` during the transition (Task 14 handles the redirect).

- [ ] **Step 3: Verify**

- Visit `/settings/plugins/browse` (old link still works during transition) → rows render with Install / Installed / Incompatible states.
- Filter by name, capability, source.
- Click Install → plugin installs, row shows "Installed".

- [ ] **Step 4: Commit**

```bash
git add app/components/plugins/DiscoverTab.tsx app/components/pages/AdminPlugins.tsx
git commit -m "[Frontend] Rename Browse to Discover and share PluginRow"
```

---

### Task 12: Tab Pill — Update Count + Tooltip

**Why:** Spec §1 "Tab-label pill". The Installed tab label shows a count of plugins with updates available; tooltip shows "{n} plugin{s} have an update available".

**Files:**
- Create: `app/components/plugins/TabUpdatePill.tsx`
- Create: `app/components/plugins/TabUpdatePill.test.tsx`
- Modify: `app/components/pages/AdminPlugins.tsx`

- [ ] **Step 1: Write failing test**

Create `app/components/plugins/TabUpdatePill.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TabUpdatePill } from "./TabUpdatePill";

describe("TabUpdatePill", () => {
  it("renders nothing when count is 0", () => {
    const { container } = render(<TabUpdatePill count={0} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders the count when > 0", () => {
    render(<TabUpdatePill count={3} />);
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("sets the plural tooltip for count > 1", () => {
    render(<TabUpdatePill count={3} />);
    expect(screen.getByLabelText(/3 plugins have an update available/i)).toBeInTheDocument();
  });

  it("sets the singular tooltip for count === 1", () => {
    render(<TabUpdatePill count={1} />);
    expect(screen.getByLabelText(/1 plugin has an update available/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run to verify fail; implement**

Create `app/components/plugins/TabUpdatePill.tsx`:

```tsx
import { Badge } from "@/components/ui/badge";

export const TabUpdatePill = ({ count }: { count: number }) => {
  if (count <= 0) return null;
  const label =
    count === 1
      ? "1 plugin has an update available"
      : `${count} plugins have an update available`;
  return (
    <Badge
      aria-label={label}
      className="ml-2 h-5 min-w-5 bg-primary/20 text-primary border-primary/40"
    >
      {count}
    </Badge>
  );
};
```

- [ ] **Step 3: Test passes; wire into `AdminPlugins.tsx`**

In the Tabs list, pass the derived count to the pill:

```tsx
const { data: plugins = [] } = usePluginsInstalled();
const updateCount = useMemo(
  () => plugins.filter((p) => !!p.update_available_version).length,
  [plugins],
);

// Inside TabsList:
<TabsTrigger value="installed">
  Installed <TabUpdatePill count={updateCount} />
</TabsTrigger>
```

- [ ] **Step 4: Verify**

Pill appears only when a plugin has `update_available_version`. Hovering shows the pluralized tooltip. Clicking Update on a row decrements.

- [ ] **Step 5: Commit**

```bash
git add app/components/plugins/TabUpdatePill.tsx app/components/plugins/TabUpdatePill.test.tsx app/components/pages/AdminPlugins.tsx
git commit -m "[Frontend] Add update-count pill to Installed tab label"
```

---

### Task 13: Advanced Dialog (Order + Repositories)

**Why:** Spec §1 and §2. Order + Repositories move behind a gear-icon button. The dialog accepts a `defaultSection` prop so legacy `?advanced=order|repositories` redirects (Task 14) can open the right section.

**Files:**
- Create: `app/components/plugins/AdvancedPluginsDialog.tsx`
- Create: `app/components/plugins/AdvancedOrderSection.tsx`
- Create: `app/components/plugins/AdvancedRepositoriesSection.tsx`
- Modify: `app/components/pages/AdminPlugins.tsx`

- [ ] **Step 1: Extract `AdvancedOrderSection`**

Move the Order tab logic (`AdminPlugins.tsx:462–678`) into `AdvancedOrderSection.tsx`. Keep all state (localOrder, hasOrderChanged), mutations, UI. Export:

```tsx
export const AdvancedOrderSection = () => {
  // identical logic to OrderTab
};
```

- [ ] **Step 2: Extract `AdvancedRepositoriesSection`**

Move Repositories tab (`AdminPlugins.tsx:682–849`) into `AdvancedRepositoriesSection.tsx`. Same export pattern.

- [ ] **Step 3: Implement `AdvancedPluginsDialog`**

Create `app/components/plugins/AdvancedPluginsDialog.tsx`:

```tsx
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AdvancedOrderSection } from "./AdvancedOrderSection";
import { AdvancedRepositoriesSection } from "./AdvancedRepositoriesSection";

export interface AdvancedPluginsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultSection?: "order" | "repositories";
}

export const AdvancedPluginsDialog = ({
  open,
  onOpenChange,
  defaultSection = "order",
}: AdvancedPluginsDialogProps) => {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-3xl overflow-hidden">
        <DialogHeader>
          <DialogTitle>Advanced plugin settings</DialogTitle>
        </DialogHeader>
        <Tabs defaultValue={defaultSection} className="flex flex-col overflow-hidden">
          <TabsList>
            <TabsTrigger value="order">Order</TabsTrigger>
            <TabsTrigger value="repositories">Repositories</TabsTrigger>
          </TabsList>
          <div className="overflow-auto">
            <TabsContent value="order" className="mt-4"><AdvancedOrderSection /></TabsContent>
            <TabsContent value="repositories" className="mt-4"><AdvancedRepositoriesSection /></TabsContent>
          </div>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
};
```

- [ ] **Step 4: Add gear button + wire dialog in `AdminPlugins.tsx`**

In the page header of `AdminPlugins.tsx`, add a Scan local button (existing) and a gear icon next to it. Wire the gear to open `AdvancedPluginsDialog`. Read `?advanced=order|repositories` from search params on mount:

```tsx
const [advancedOpen, setAdvancedOpen] = useState(false);
const [advancedDefault, setAdvancedDefault] = useState<"order" | "repositories">("order");
const [searchParams, setSearchParams] = useSearchParams();

useEffect(() => {
  const adv = searchParams.get("advanced");
  if (adv === "order" || adv === "repositories") {
    setAdvancedDefault(adv);
    setAdvancedOpen(true);
    // Clean the URL so reload doesn't reopen it
    const next = new URLSearchParams(searchParams);
    next.delete("advanced");
    setSearchParams(next, { replace: true });
  }
}, []); // intentionally empty; only on mount

// In header:
<Button variant="ghost" size="icon" onClick={() => setAdvancedOpen(true)}>
  <Settings className="h-4 w-4" />
</Button>

<AdvancedPluginsDialog
  open={advancedOpen}
  onOpenChange={setAdvancedOpen}
  defaultSection={advancedDefault}
/>
```

- [ ] **Step 5: Remove old Order + Repositories tab triggers + contents**

From the Tabs list, remove the Order and Repositories TabsTrigger elements and their TabsContent. Update `validTabs` to only accept `"installed"` and `"discover"`.

- [ ] **Step 6: Verify**

- Click gear → dialog opens on Order section.
- Switch to Repositories inside dialog → everything works.
- Close dialog → no stale state.

- [ ] **Step 7: Commit**

```bash
git add app/components/plugins/AdvancedPluginsDialog.tsx app/components/plugins/AdvancedOrderSection.tsx app/components/plugins/AdvancedRepositoriesSection.tsx app/components/pages/AdminPlugins.tsx
git commit -m "[Frontend] Move Order and Repositories into Advanced dialog"
```

---

### Task 14: Router — Explicit Routes + Legacy Redirects

**Why:** Spec §1 "Route layout". Replace `:tab?` with explicit `/installed`, `/discover`, and redirect legacy `/browse`, `/order`, `/repositories` paths.

**Files:**
- Modify: `app/router.tsx`
- Modify: `app/components/pages/AdminPlugins.tsx` (remove `:tab?` client-side logic)

- [ ] **Step 1: Update routes**

In `app/router.tsx`, replace the existing `plugins/:tab?` block with:

```tsx
{
  path: "plugins",
  element: (
    <ProtectedRoute requiredPermission={{ resource: "config", operation: "read" }}>
      <AdminPlugins />
    </ProtectedRoute>
  ),
},
{
  path: "plugins/installed",
  element: (
    <ProtectedRoute requiredPermission={{ resource: "config", operation: "read" }}>
      <AdminPlugins />
    </ProtectedRoute>
  ),
},
{
  path: "plugins/discover",
  element: (
    <ProtectedRoute requiredPermission={{ resource: "config", operation: "read" }}>
      <AdminPlugins />
    </ProtectedRoute>
  ),
},
{ path: "plugins/browse", element: <Navigate to="/settings/plugins/discover" replace /> },
{ path: "plugins/order", element: <Navigate to="/settings/plugins?advanced=order" replace /> },
{ path: "plugins/repositories", element: <Navigate to="/settings/plugins?advanced=repositories" replace /> },
{
  path: "plugins/:scope/:id",
  element: (
    <ProtectedRoute requiredPermission={{ resource: "config", operation: "read" }}>
      <PluginDetail />
    </ProtectedRoute>
  ),
},
```

Import `Navigate` from `react-router`.

- [ ] **Step 2: Simplify `AdminPlugins.tsx` tab routing**

Since routes are now explicit, simplify tab-derivation logic. Use `useLocation` or pick off the last path segment:

```tsx
const location = useLocation();
const activeTab = location.pathname.endsWith("/discover") ? "discover" : "installed";
const navigate = useNavigate();
const handleTabChange = (value: string) => {
  navigate(`/settings/plugins${value === "discover" ? "/discover" : ""}`);
};
```

Delete the old `:tab?` param parsing / `validTabs` array.

- [ ] **Step 3: Verify each route**

- `/settings/plugins` → Installed tab.
- `/settings/plugins/installed` → Installed tab.
- `/settings/plugins/discover` → Discover tab.
- `/settings/plugins/browse` → 301-style SPA redirect to `/settings/plugins/discover`.
- `/settings/plugins/order` → redirect to `/settings/plugins?advanced=order` which auto-opens Advanced on Order.
- `/settings/plugins/repositories` → same for repositories.
- `/settings/plugins/shisho/google-books` → detail page.

- [ ] **Step 4: Commit**

```bash
git add app/router.tsx app/components/pages/AdminPlugins.tsx
git commit -m "[Frontend] Split plugins routes; redirect legacy browse/order/repositories paths"
```

---

### Task 15: Docs — Field-Level Notes in `repositories.md`

**Why:** Spec §8. Field-level notes for `imageUrl`, `releaseDate`, `changelog` in the existing Repository Manifest Format section. No new sections; no screenshots.

**Files:**
- Modify: `website/docs/plugins/repositories.md`

- [ ] **Step 1: Extend the Repository Manifest Format section**

Open `website/docs/plugins/repositories.md`. After the existing JSON example (around line 85), and before the "Key Rules" section, add or merge into an expanded field reference. Locate the appropriate spot to describe each field. Add three field-level notes:

```markdown
### Field notes

- **`imageUrl`** (on each plugin entry): Plugin logo URL. Recommended 256×256 PNG or SVG (SVG preferred), 1:1 aspect ratio, centered mark with ≥10% safe area; any HTTPS URL works (GitHub raw is fine). Shisho renders it on a muted square backdrop with a rounded radius that scales with display size, so transparent artwork shows the backdrop through. When `imageUrl` is missing or fails to load, Shisho falls back to hashed-color initials derived from `scope/id`.
- **`releaseDate`** (on each version entry): Optional. Accepts RFC3339 (`2026-04-14T00:00:00Z`) or date-only (`2026-04-14`). When omitted, the "Released" line is hidden on the version card. Repository manifests are validated at fetch time; versions with an invalid `releaseDate` are skipped (and a warning is logged server-side).
- **`changelog`** (on each version entry): Rendered as sanitized markdown on the plugin detail page. Supported subset: headings (`##`, `###`), paragraphs, lists, inline code, fenced code blocks, links (open in a new tab), bold, italic. Raw HTML, images, and iframes are stripped — author content accordingly. The "View full diff on GitHub" link in the UI is inferred from the plugin's `homepage` when it points to a GitHub repo; no additional manifest field is read for it.
```

- [ ] **Step 2: Add `imageUrl` to the example JSON**

Edit the `repository.json` code block near lines 51–85 so the plugin entry includes an `imageUrl` line next to `homepage`:

```json
"homepage": "https://github.com/my-org/my-plugin",
"imageUrl": "https://raw.githubusercontent.com/my-org/my-plugin/main/logo.png",
```

- [ ] **Step 3: Verify docs build**

Run: `cd website && pnpm build`
Expected: success, no broken links.

- [ ] **Step 4: Commit**

```bash
git add website/docs/plugins/repositories.md
git commit -m "[Docs] Document imageUrl, releaseDate, and changelog fields on repo manifest"
```

---

### Task 16: Cleanup

**Why:** Spec §9 step 13. Remove the now-unused `PluginConfigDialog` wrapper (the form lives in the detail page), remove any top-level "Update available" banner, and delete dead code from the old InstalledTab row (reload/settings/uninstall inline button handlers).

**Files:**
- Delete: `app/components/plugins/PluginConfigDialog.tsx`
- Modify: `app/components/pages/AdminPlugins.tsx` (remove any leftover banner, leftover modal state)
- Grep-and-clean any references

- [ ] **Step 1: Confirm no remaining importers of `PluginConfigDialog`**

Run: `grep -r "PluginConfigDialog" app/`
Expected: only the file itself and possibly stale test imports. Remove all importers.

- [ ] **Step 2: Delete the file**

```bash
git rm app/components/plugins/PluginConfigDialog.tsx
```

- [ ] **Step 3: Scan `AdminPlugins.tsx` for dead code**

Look for:
- `useDisclosure` or `useState<Plugin | null>` for `configTarget` — remove if only the old dialog used it.
- Any page-level "Update available" banner JSX (check for strings like "Update available" outside the row code). Remove.
- Orphaned imports (`FormDialog`, `Switch` in the row code, etc.).

- [ ] **Step 4: Run full check**

```bash
mise check:quiet
```

Expected: pass. Fix anything that breaks.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "[Frontend] Remove unused PluginConfigDialog and dead banner code"
```

---

### Task 17: E2E Tests

**Why:** Spec §10 E2E section. Lock in the major flows with Playwright so future refactors don't regress them.

**Files:**
- Create: `e2e/tests/plugins.spec.ts` (or extend an existing plugins e2e file if one exists — check `e2e/CLAUDE.md`)

- [ ] **Step 1: Read `e2e/CLAUDE.md` for fixture conventions**

Take note of:
- How per-browser isolation works
- How to seed a plugin repo / install a plugin in a test
- Which fixtures provide auth'd pages

- [ ] **Step 2: Write the test suite**

Create `e2e/tests/plugins.spec.ts` with the five flows from spec §10:

```ts
import { test, expect } from "../fixtures";

test.describe("Plugins redesigned UI", () => {
  test("install plugin from Discover → appears in Installed", async ({ adminPage }) => {
    await adminPage.goto("/settings/plugins/discover");
    const row = adminPage.getByRole("link", { name: /test-plugin/i }).first();
    await row.locator("button", { hasText: "Install" }).click();
    await expect(row.locator("button", { hasText: "Installed" })).toBeVisible();

    await adminPage.goto("/settings/plugins/installed");
    await expect(adminPage.getByRole("link", { name: /test-plugin/i })).toBeVisible();
  });

  test("click Update on installed row → in-place version bump + pill decrement", async ({ adminPage }) => {
    await adminPage.goto("/settings/plugins/installed");
    // Assumes fixture seeds a plugin with an available update
    const row = adminPage.getByRole("link", { name: /outdated-plugin/i });
    const updateBadge = row.locator("text=/update \\d/i");
    await expect(updateBadge).toBeVisible();
    await row.locator("button", { hasText: "Update" }).click();
    await expect(updateBadge).not.toBeVisible();
  });

  test("detail page: toggle enabled, reflects on list after back-nav", async ({ adminPage }) => {
    await adminPage.goto("/settings/plugins/shisho/test-plugin");
    const toggle = adminPage.getByRole("switch", { name: /enabled/i });
    await toggle.click();
    await adminPage.goto("/settings/plugins/installed");
    // Disabled rows get the dimmed class + Disabled badge
    await expect(adminPage.getByText(/disabled/i)).toBeVisible();
  });

  test("uninstall from detail page → returns to list, plugin gone", async ({ adminPage }) => {
    await adminPage.goto("/settings/plugins/shisho/test-plugin");
    await adminPage.getByRole("button", { name: /^uninstall$/i }).click();
    await adminPage.getByRole("button", { name: /^uninstall$/i }).click(); // confirm dialog
    await expect(adminPage).toHaveURL(/\/settings\/plugins$/);
    await expect(adminPage.getByRole("link", { name: /test-plugin/i })).toHaveCount(0);
  });

  test("local-scope plugin shows reload icon; repo-installed plugin does not", async ({ adminPage }) => {
    await adminPage.goto("/settings/plugins/local/dev-plugin");
    await expect(adminPage.getByRole("button", { name: /reload plugin from disk/i })).toBeVisible();

    await adminPage.goto("/settings/plugins/shisho/test-plugin");
    await expect(adminPage.getByRole("button", { name: /reload plugin from disk/i })).toHaveCount(0);
  });
});
```

(Adapt selectors and fixtures to match this repo's patterns — see `e2e/CLAUDE.md` and any existing `plugins` or `admin` specs.)

- [ ] **Step 3: Run the E2E suite**

Run: `mise test:e2e -- plugins.spec.ts`
Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add e2e/tests/plugins.spec.ts
git commit -m "[E2E] Add plugins UI redesign flows"
```

---

### Final Checklist (before merge)

- [ ] Run `mise check:quiet` one final time. Expected: pass.
- [ ] Manually verify each of the following flows in a running dev server:
  - [ ] Visit `/settings/plugins` → Installed tab shows, gear icon opens Advanced dialog.
  - [ ] Gear → Order → reorder and save works.
  - [ ] Gear → Repositories → add, sync, remove works.
  - [ ] Tab pill renders when a plugin has an update; disappears when updated.
  - [ ] Click a row → detail page renders with hero, version history, permissions, config, danger zone.
  - [ ] Edit config field, navigate away → unsaved-changes dialog appears.
  - [ ] Save config → dialog doesn't appear on next navigation.
  - [ ] View manifest → dialog shows JSON.
  - [ ] Reload (local plugin only) → toast confirms.
  - [ ] Uninstall → confirm → lands on list, plugin gone.
  - [ ] Discover tab: filter, install, installed/incompatible button states.
  - [ ] Legacy redirects: `/browse`, `/order`, `/repositories` all land somewhere sensible.
- [ ] Confirm no TypeScript errors, no unused imports, no dead CSS.
