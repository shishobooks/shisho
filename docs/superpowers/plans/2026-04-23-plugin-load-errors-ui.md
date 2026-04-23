# Plugin Load Error UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface plugin load errors to users — return a proper error from the enable endpoint, and render status-specific badges + error text on the installed list and plugin detail page.

**Architecture:** Three-part change. (1) `pkg/plugins/handler.go:update` persists the malfunctioned/not-supported state and then returns `errcodes.ValidationError(loadErr.Error())` so the frontend mutation's `catch` branch fires with the real message. (2) `PluginRow` accepts a `status` prop and renders one of three badges (Disabled / Error / Incompatible). (3) `PluginDetailHero` renders an inline alert block when `installed.load_error` is set or status is Malfunctioned/NotSupported.

**Tech Stack:** Go (Echo, Bun), React 19 + TypeScript, Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-04-23-plugin-load-errors-ui-design.md`

---

## File Map

| File | Change |
|------|--------|
| `pkg/plugins/handler.go` | Modify `update` to persist-then-error on load failure during enable |
| `pkg/plugins/handler_enable_test.go` | **Create** — Go test for the new error path |
| `app/components/plugins/PluginRow.tsx` | Replace `disabled` boolean with `status` prop; render status-specific badge |
| `app/components/plugins/PluginRow.test.tsx` | Update existing disabled case; add Malfunctioned and NotSupported cases |
| `app/components/plugins/InstalledTab.tsx` | Pass `plugin.status` (and `plugin.load_error`) to `PluginRow` |
| `app/components/plugins/PluginDetailHero.tsx` | Add the load-error alert block under the title row |
| `app/components/plugins/PluginDetailHero.test.tsx` | Add cases for load_error block and NotSupported without load_error |

No model changes, no migrations, no generated types to regenerate.

---

## Task 1: Backend — failing test for bad-enable returns error

**Files:**
- Create: `pkg/plugins/handler_enable_test.go`

- [ ] **Step 1: Write the failing test**

```go
package plugins

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdate_EnableLoadFailure_ReturnsError confirms that toggling a plugin's
// enable flag to true when the plugin cannot be loaded returns a 422 error and
// still persists the Malfunctioned status + load_error to the database so the
// UI can render it after a refetch.
func TestUpdate_EnableLoadFailure_ReturnsError(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	pluginDir := t.TempDir()

	// Plugin with a manifest declaring an enricher field that doesn't exist.
	// LoadPlugin will reject this with an "invalid metadata field" error.
	scope := "test"
	id := "broken-enricher"
	destDir := filepath.Join(pluginDir, scope, id)
	require.NoError(t, os.MkdirAll(destDir, 0755))
	manifest := `{
  "manifestVersion": 1,
  "id": "broken-enricher",
  "name": "Broken Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "fileTypes": ["epub"],
      "fields": ["nonsenseField"]
    }
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifest), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(destDir, "main.js"),
		[]byte(`var plugin=(function(){return{metadataEnricher:{search:function(){return{results:[]}}}};})();`), 0644))

	ctx := context.Background()
	// Pre-install as Disabled so the PATCH path exercises the enable branch.
	require.NoError(t, svc.InstallPlugin(ctx, &models.Plugin{
		Scope:       scope,
		ID:          id,
		Name:        "Broken Enricher",
		Version:     "1.0.0",
		Status:      models.PluginStatusDisabled,
		InstalledAt: time.Now(),
	}))

	mgr := NewManager(svc, pluginDir, "")
	h := NewHandler(svc, mgr, nil)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"enabled": true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope", "id")
	c.SetParamValues(scope, id)

	err := h.update(c)
	// The handler must surface a 422 error via the errcodes pipeline.
	require.Error(t, err)
	var ec *errcodes.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, http.StatusUnprocessableEntity, ec.HTTPCode)
	assert.Contains(t, ec.Message, "nonsenseField")

	// And the plugin row must be persisted as Malfunctioned with the load_error.
	retrieved, err := svc.RetrievePlugin(ctx, scope, id)
	require.NoError(t, err)
	assert.Equal(t, models.PluginStatusMalfunctioned, retrieved.Status)
	require.NotNil(t, retrieved.LoadError)
	assert.Contains(t, *retrieved.LoadError, "nonsenseField")
}
```

- [ ] **Step 2: Run test — expect fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && go test ./pkg/plugins/ -run TestUpdate_EnableLoadFailure_ReturnsError -v`

Expected: FAIL. Current handler returns `nil` from `c.JSON(http.StatusOK, plugin)`, so `require.Error(t, err)` fails.

- [ ] **Step 3: Commit the red test**

```bash
git add pkg/plugins/handler_enable_test.go
git commit -m "[Test] Add failing test for plugin enable load-failure error response"
```

---

## Task 2: Backend — return error when enable-triggered load fails

**Files:**
- Modify: `pkg/plugins/handler.go` (the `update` method, currently around lines 369-455)

- [ ] **Step 1: Implement the change**

In `handler.update`, locate the enable block:

```go
		if *payload.Enabled && !wasActive {
			// Enabling: set Active and load the plugin
			plugin.Status = models.PluginStatusActive
			if err := h.manager.LoadPlugin(ctx, scope, id); err != nil {
				errMsg := err.Error()
				plugin.LoadError = &errMsg
				if isVersionIncompatible(err) {
					plugin.Status = models.PluginStatusNotSupported
				} else {
					plugin.Status = models.PluginStatusMalfunctioned
				}
				h.manager.emitEvent(PluginEventMalfunctioned, scope, id, nil)
			} else {
				plugin.LoadError = nil
				var hooks []string
				if rt := h.manager.GetRuntime(scope, id); rt != nil {
					hooks = rt.HookTypes()
				}
				h.manager.emitEvent(PluginEventEnabled, scope, id, hooks)
			}
		} else if !*payload.Enabled && wasActive {
```

Introduce a `loadErr` variable captured from `LoadPlugin` so we can return it **after** persisting. Replace the block with:

```go
		var loadErr error
		if *payload.Enabled && !wasActive {
			// Enabling: set Active and load the plugin
			plugin.Status = models.PluginStatusActive
			if err := h.manager.LoadPlugin(ctx, scope, id); err != nil {
				loadErr = err
				errMsg := err.Error()
				plugin.LoadError = &errMsg
				if isVersionIncompatible(err) {
					plugin.Status = models.PluginStatusNotSupported
				} else {
					plugin.Status = models.PluginStatusMalfunctioned
				}
				h.manager.emitEvent(PluginEventMalfunctioned, scope, id, nil)
			} else {
				plugin.LoadError = nil
				var hooks []string
				if rt := h.manager.GetRuntime(scope, id); rt != nil {
					hooks = rt.HookTypes()
				}
				h.manager.emitEvent(PluginEventEnabled, scope, id, hooks)
			}
		} else if !*payload.Enabled && wasActive {
```

Then, after `h.service.UpdatePlugin(ctx, plugin)` succeeds (and before the config/confidence-threshold branches), add the error return. The final portion of the method changes from:

```go
	if err := h.service.UpdatePlugin(ctx, plugin); err != nil {
		return errors.WithStack(err)
	}

	if payload.Config != nil {
		// ...
	}

	// ... confidence-threshold branches ...

	return errors.WithStack(c.JSON(http.StatusOK, plugin))
}
```

to:

```go
	if err := h.service.UpdatePlugin(ctx, plugin); err != nil {
		return errors.WithStack(err)
	}

	// Surface enable-time load failures as a 422 so the frontend toast shows
	// the real reason. The Malfunctioned/NotSupported status is already
	// persisted above, so a follow-up GET reflects the broken state.
	if loadErr != nil {
		return errcodes.ValidationError(loadErr.Error())
	}

	if payload.Config != nil {
		// ...
	}

	// ... confidence-threshold branches ...

	return errors.WithStack(c.JSON(http.StatusOK, plugin))
}
```

- [ ] **Step 2: Run the new test — expect pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && go test ./pkg/plugins/ -run TestUpdate_EnableLoadFailure_ReturnsError -v`

Expected: PASS.

- [ ] **Step 3: Run the full plugins package — expect pass (no regressions)**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && go test ./pkg/plugins/ -count=1`

Expected: PASS. If anything fails, the most likely cause is a test that PATCHes `enabled: true` with a load-failing plugin and used to assert `err == nil` — update it to assert the new 422 behavior.

- [ ] **Step 4: Commit**

```bash
git add pkg/plugins/handler.go
git commit -m "[Backend] Return 422 when enabling a plugin fails to load"
```

---

## Task 3: Frontend — PluginRow status-aware badges (failing tests)

**Files:**
- Modify: `app/components/plugins/PluginRow.test.tsx`

- [ ] **Step 1: Update the existing "disabled" test and add three new tests**

Replace the `it("renders the Disabled badge when disabled=true", ...)` block with the following, and add an import for the status constants at the top of the file next to the existing imports:

```tsx
import {
  PluginStatusActive,
  PluginStatusDisabled,
  PluginStatusMalfunctioned,
  PluginStatusNotSupported,
} from "@/types/generated/models";
```

Replace the existing disabled test and add new ones:

```tsx
  it("renders no status badge when status is Active", () => {
    render(wrap(<PluginRow {...base} status={PluginStatusActive} />));
    expect(screen.queryByText(/disabled/i)).toBeNull();
    expect(screen.queryByText(/error/i)).toBeNull();
    expect(screen.queryByText(/incompatible/i)).toBeNull();
  });

  it("renders the Disabled badge when status is Disabled", () => {
    render(wrap(<PluginRow {...base} status={PluginStatusDisabled} />));
    expect(screen.getByText(/disabled/i)).toBeInTheDocument();
  });

  it("renders the Error badge with load_error as title when status is Malfunctioned", () => {
    render(
      wrap(
        <PluginRow
          {...base}
          loadError="failed to load plugin: invalid field"
          status={PluginStatusMalfunctioned}
        />,
      ),
    );
    const badge = screen.getByText(/error/i);
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveAttribute(
      "title",
      "failed to load plugin: invalid field",
    );
  });

  it("renders the Incompatible badge when status is NotSupported", () => {
    render(wrap(<PluginRow {...base} status={PluginStatusNotSupported} />));
    expect(screen.getByText(/incompatible/i)).toBeInTheDocument();
  });
```

- [ ] **Step 2: Run tests — expect fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && pnpm vitest run app/components/plugins/PluginRow.test.tsx`

Expected: FAIL. `status` prop doesn't exist yet; the component only accepts `disabled`.

- [ ] **Step 3: Commit the red tests**

```bash
git add app/components/plugins/PluginRow.test.tsx
git commit -m "[Test] Update PluginRow tests for status-aware badges"
```

---

## Task 4: Frontend — implement PluginRow status prop

**Files:**
- Modify: `app/components/plugins/PluginRow.tsx`

- [ ] **Step 1: Replace the `disabled` prop with `status` + optional `loadError`**

The full new file contents:

```tsx
import { BadgeCheck, ChevronRight } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/libraries/utils";
import {
  PluginStatusActive,
  PluginStatusDisabled,
  PluginStatusMalfunctioned,
  PluginStatusNotSupported,
  type PluginStatus,
} from "@/types/generated/models";

import { PluginLogo } from "./PluginLogo";

export interface PluginRowProps {
  actions?: ReactNode;
  capabilities: string[];
  description?: string;
  href: string;
  id: string;
  imageUrl?: string | null;
  isOfficial?: boolean;
  loadError?: string;
  name: string;
  repoName?: string;
  scope: string;
  status?: PluginStatus;
  updateAvailable?: string;
  version?: string;
}

const renderStatusBadge = (status: PluginStatus | undefined, loadError?: string) => {
  if (status === undefined || status === PluginStatusActive) return null;
  if (status === PluginStatusMalfunctioned) {
    return (
      <Badge title={loadError} variant="destructive">
        Error
      </Badge>
    );
  }
  if (status === PluginStatusNotSupported) {
    return <Badge variant="outline">Incompatible</Badge>;
  }
  // PluginStatusDisabled and any future non-active value fall back to Disabled.
  return <Badge variant="secondary">Disabled</Badge>;
};

export const PluginRow = ({
  actions,
  capabilities,
  description,
  href,
  id,
  imageUrl,
  isOfficial,
  loadError,
  name,
  repoName,
  scope,
  status,
  updateAvailable,
  version,
}: PluginRowProps) => {
  const isInactive = status !== undefined && status !== PluginStatusActive;
  return (
    <Link
      className={cn(
        "group flex items-center gap-4 rounded-md border border-border px-4 py-3 transition-colors hover:bg-accent/30",
        isInactive && "opacity-50 saturate-50",
      )}
      to={href}
    >
      <PluginLogo
        id={id}
        imageUrl={imageUrl ?? undefined}
        scope={scope}
        size={40}
      />

      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate font-medium">{name}</span>
          {renderStatusBadge(status, loadError)}
          {updateAvailable && <Badge>Update {updateAvailable}</Badge>}
        </div>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {version && <span>v{version}</span>}
          {capabilities.map((cap) => (
            <Badge key={cap} variant="outline">
              {cap}
            </Badge>
          ))}
          {repoName && (
            <>
              <span aria-hidden="true" className="text-muted-foreground/50">
                ·
              </span>
              <span className="inline-flex items-center gap-1">
                {isOfficial && (
                  <BadgeCheck
                    aria-label="Official plugin"
                    className="h-3.5 w-3.5 text-primary"
                  />
                )}
                {repoName}
              </span>
            </>
          )}
        </div>
        {description && (
          <p className="line-clamp-2 text-xs text-muted-foreground">
            {description}
          </p>
        )}
      </div>

      {actions && (
        <div
          className="flex items-center gap-2"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
          }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          {actions}
        </div>
      )}

      <ChevronRight
        aria-hidden="true"
        className="h-4 w-4 flex-shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
      />
    </Link>
  );
};
```

- [ ] **Step 2: Update the existing disabled test that still uses the old prop name**

The test `it("renders the Disabled badge when disabled=true", ...)` was replaced in Task 3, but the earlier test at `PluginRow.test.tsx:23` (`renders name and version on meta line`) and the other tests use `base` which doesn't include `disabled`. Scan the file for any remaining `disabled` prop usage and change to `status={PluginStatusDisabled}`. At the time of writing only the test removed in Task 3 used it; verify with:

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && grep -n 'disabled' app/components/plugins/PluginRow.test.tsx`

Expected: matches should only be `status={PluginStatusDisabled}` / `/disabled/i` regexes from Task 3.

- [ ] **Step 3: Run PluginRow tests — expect pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && pnpm vitest run app/components/plugins/PluginRow.test.tsx`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add app/components/plugins/PluginRow.tsx
git commit -m "[Frontend] PluginRow renders status-specific badges"
```

---

## Task 5: Frontend — InstalledTab passes status through

**Files:**
- Modify: `app/components/plugins/InstalledTab.tsx` (the `renderRow` function around lines 53-113)

- [ ] **Step 1: Remove `disabled` computation and forward `status` + `loadError`**

In `renderRow`, delete the line `const isDisabled = plugin.status !== PluginStatusActive;` (it's around line 61) and change the `PluginRow` invocation from:

```tsx
      <PluginRow
        actions={...}
        capabilities={capabilityLabels}
        description={plugin.description}
        disabled={isDisabled}
        href={`/settings/plugins/${plugin.scope}/${plugin.id}`}
        ...
      />
```

to:

```tsx
      <PluginRow
        actions={...}
        capabilities={capabilityLabels}
        description={plugin.description}
        href={`/settings/plugins/${plugin.scope}/${plugin.id}`}
        id={plugin.id}
        imageUrl={imageUrl}
        isOfficial={isOfficial}
        key={`${plugin.scope}/${plugin.id}`}
        loadError={plugin.load_error}
        name={plugin.name}
        repoName={repoNameByScope.get(plugin.scope)}
        scope={plugin.scope}
        status={plugin.status}
        updateAvailable={plugin.update_available_version ?? undefined}
        version={plugin.version}
      />
```

Only the `disabled` → `status` + `loadError` lines are new; the rest is unchanged. Keep `actions` (the Update button wiring) exactly as it was.

- [ ] **Step 2: Type-check and run frontend tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && pnpm lint:types`

Expected: PASS (no TS errors).

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && pnpm vitest run app/components/plugins/`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add app/components/plugins/InstalledTab.tsx
git commit -m "[Frontend] Installed plugins list distinguishes Error and Incompatible states"
```

---

## Task 6: Frontend — PluginDetailHero error alert (failing tests)

**Files:**
- Modify: `app/components/plugins/PluginDetailHero.test.tsx`

- [ ] **Step 1: Add three new tests covering the error block**

Add to the existing `describe("PluginDetailHero", ...)`, after the existing two tests. First, add imports at the top:

```tsx
import type { Plugin } from "@/types/generated/models";
import {
  PluginStatusActive,
  PluginStatusMalfunctioned,
  PluginStatusNotSupported,
} from "@/types/generated/models";
```

Add a `baseInstalled` helper next to `baseAvailable`:

```tsx
const baseInstalled: Plugin = {
  auto_update: false,
  id: "p",
  installed_at: "2026-01-01T00:00:00Z",
  name: "Plugin",
  scope: "shisho",
  status: PluginStatusActive,
  version: "1.0.0",
};
```

Then add the tests:

```tsx
  it("renders the load-error alert when status is Malfunctioned", () => {
    render(
      <PluginDetailHero
        canWrite={false}
        id="p"
        installed={{
          ...baseInstalled,
          load_error: "failed to load plugin: invalid field",
          status: PluginStatusMalfunctioned,
        }}
        scope="shisho"
      />,
    );
    expect(screen.getByText(/plugin failed to load/i)).toBeInTheDocument();
    expect(
      screen.getByText(/failed to load plugin: invalid field/i),
    ).toBeInTheDocument();
  });

  it("renders the Incompatible alert when status is NotSupported without a load_error", () => {
    render(
      <PluginDetailHero
        canWrite={false}
        id="p"
        installed={{
          ...baseInstalled,
          status: PluginStatusNotSupported,
        }}
        scope="shisho"
      />,
    );
    expect(
      screen.getByText(/not compatible with this shisho version/i),
    ).toBeInTheDocument();
  });

  it("does not render the alert for an Active plugin", () => {
    render(
      <PluginDetailHero
        canWrite={false}
        id="p"
        installed={baseInstalled}
        scope="shisho"
      />,
    );
    expect(screen.queryByText(/failed to load/i)).toBeNull();
    expect(screen.queryByText(/not compatible/i)).toBeNull();
  });
```

- [ ] **Step 2: Run tests — expect fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && pnpm vitest run app/components/plugins/PluginDetailHero.test.tsx`

Expected: FAIL. The alert block does not exist yet.

- [ ] **Step 3: Commit the red tests**

```bash
git add app/components/plugins/PluginDetailHero.test.tsx
git commit -m "[Test] Add PluginDetailHero tests for load-error alert"
```

---

## Task 7: Frontend — implement PluginDetailHero alert block

**Files:**
- Modify: `app/components/plugins/PluginDetailHero.tsx`

- [ ] **Step 1: Import the extra status constants**

Change the existing import:

```tsx
import { PluginStatusActive, type Plugin } from "@/types/generated/models";
```

to:

```tsx
import {
  PluginStatusActive,
  PluginStatusMalfunctioned,
  PluginStatusNotSupported,
  type Plugin,
} from "@/types/generated/models";
```

- [ ] **Step 2: Insert the alert block inside the info column**

Inside `PluginDetailHero`, the middle column contains the title row, `metaParts`, and `description`. Insert a new block between the title row `<div className="flex flex-wrap items-center gap-2">...</div>` and the `metaParts` block. The info column becomes:

```tsx
      <div className="flex-1 space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold">{name}</h1>
          {updateAvailable && (
            <Badge variant="default">
              Update available — {updateAvailable}
            </Badge>
          )}
          {!installed && available && (
            <Badge variant="secondary">Not installed</Badge>
          )}
        </div>

        {installed &&
          (installed.status === PluginStatusMalfunctioned ||
            installed.status === PluginStatusNotSupported ||
            installed.load_error) && (
            <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm">
              <p className="font-medium text-destructive">
                {installed.status === PluginStatusNotSupported
                  ? "Plugin is not compatible with this Shisho version"
                  : "Plugin failed to load"}
              </p>
              {installed.load_error && (
                <p className="mt-1 break-words font-mono text-xs text-muted-foreground">
                  {installed.load_error}
                </p>
              )}
            </div>
          )}

        {metaParts.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            {metaParts.map((part, i) => (
              <Fragment key={i}>
                {i > 0 && (
                  <span aria-hidden="true" className="text-muted-foreground/50">
                    ·
                  </span>
                )}
                {part}
              </Fragment>
            ))}
          </div>
        )}

        {description && (
          <p className="text-sm text-muted-foreground">{description}</p>
        )}
      </div>
```

Everything outside the info column (the logo, the right-side actions column) stays unchanged.

- [ ] **Step 3: Run PluginDetailHero tests — expect pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && pnpm vitest run app/components/plugins/PluginDetailHero.test.tsx`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add app/components/plugins/PluginDetailHero.tsx
git commit -m "[Frontend] Show load error on plugin detail page"
```

---

## Task 8: Final verification

- [ ] **Step 1: Run the full check suite**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-errors && mise check:quiet`

Expected: one-line PASS summary. If anything fails, fix the underlying cause and re-run (do not `--no-verify`).

- [ ] **Step 2: Manual smoke test (if the dev server is available)**

With `mise start` running:

1. Visit `/settings/plugins`. A plugin with `status: -2` should render a red `Error` badge; hovering reveals the `load_error` message.
2. Click into that plugin. The detail page should render a red alert block under the title with "Plugin failed to load" and the error text in monospace.
3. Toggle the enable switch off, then on again — the failing re-enable produces a toast containing the real error message (not "Plugin enabled").
4. A plugin with `status: -3` (NotSupported) should show an outline `Incompatible` badge on the list and the "not compatible with this Shisho version" alert on the detail page.

If no UI session is available, explicitly say so rather than claiming success.

- [ ] **Step 3: Nothing to commit — verification only.**

---

## Self-Review Notes

- Every spec requirement (enable returns error, list shows distinct badges, detail shows error text) maps to a task: #1-#2, #3-#5, #6-#7 respectively.
- No placeholders or "similar to" — each step contains the exact code.
- Type names are consistent across tasks: `PluginStatus`, `PluginStatusActive`, `PluginStatusDisabled`, `PluginStatusMalfunctioned`, `PluginStatusNotSupported` (all re-exported from `@/types/generated/models`); the `loadError` prop name on `PluginRow` matches the `load_error` field on `Plugin` (snake_case on the wire, camelCase at the React prop boundary).
- No docs update — matches the spec's "No user-facing doc changes" note.
