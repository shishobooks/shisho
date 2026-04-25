# Gallery Size Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a per-user `gallery_size` preference (S/M/L/XL) that resizes book covers across every gallery page, with an in-toolbar popover for ad-hoc and saved-default control plus a User Settings entry. Bundle a copy fix to the existing sort save-as-default UI.

**Architecture:**
- Backend: add `gallery_size TEXT NOT NULL DEFAULT 'm'` to `user_settings`; rename the existing `/settings/viewer` endpoint to `/settings/user` (the row was always per-user, the endpoint name was a premature scope choice).
- Frontend: shared constants + pure utils, a single `SizeSelector` component reused by both the gallery toolbar (`SizePopover`) and the User Settings page; per-page wiring resolves effective size from URL `?size=` → saved → default `'m'`, mirroring how sort already works.
- Items-per-page scales with size (S=48, M=24, L=16, XL=12); switching size mid-page preserves the user's first-visible book via `floor(old_offset / new_limit) + 1`.

**Tech Stack:** Go (Echo, Bun, SQLite), React 19 + TypeScript, Tanstack Query, TailwindCSS, shadcn/ui, Vitest, Docusaurus.

**Spec:** `docs/superpowers/specs/2026-04-25-gallery-size-design.md`

---

## Sequencing notes

- Backend migration + model change land first (Tasks 1–2); without them, frontend can't be wired.
- The `/viewer` → `/user` rename (Task 3) is a coordinated rename across backend files, validator types, service methods, and route — all in one commit so the build is never broken at HEAD.
- Frontend hook rename (Task 8) similarly lands in one commit covering the hook, the EPUB viewer call sites, the query key, and the URL.
- Once backend + hook foundations are in place, the gallery feature itself stacks cleanly: constants → utils → UI components → page wiring → docs.
- Sort copy fix (Task 16) is a tiny standalone change but conceptually pairs with this feature; landed alongside.

---

## Task 1: Backend migration — add `gallery_size` column

**Files:**
- Create: `pkg/migrations/20260425000000_add_user_gallery_size.go`

- [ ] **Step 1: Create the migration file**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			ALTER TABLE user_settings ADD COLUMN gallery_size TEXT NOT NULL DEFAULT 'm'
		`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("ALTER TABLE user_settings DROP COLUMN gallery_size")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 2: Verify migration applies and rolls back**

Run: `mise db:migrate && mise db:rollback && mise db:migrate`
Expected: all three succeed; final `mise db:migrate` leaves the column present.

- [ ] **Step 3: Commit**

```bash
git add pkg/migrations/20260425000000_add_user_gallery_size.go
git commit -m "[Backend] Add gallery_size column to user_settings"
```

---

## Task 2: Backend model — `UserSettings.GallerySize`

**Files:**
- Modify: `pkg/models/user_settings.go`

- [ ] **Step 1: Add `GallerySize` constants and field**

Add this block above the existing `EpubFlow*` constants:

```go
const (
	//tygo:emit export type GallerySize = typeof GallerySizeSmall | typeof GallerySizeMedium | typeof GallerySizeLarge | typeof GallerySizeExtraLarge;
	GallerySizeSmall      = "s"
	GallerySizeMedium     = "m"
	GallerySizeLarge      = "l"
	GallerySizeExtraLarge = "xl"
)
```

Add `GallerySize string` to the struct with a default tag matching the migration:

```go
GallerySize        string    `bun:",notnull,default:'m'" json:"gallery_size" tstype:"GallerySize"`
```

Place it after `EpubFlow` to match the migration column order.

Update `DefaultUserSettings()` to set the new field:

```go
return &UserSettings{
	ViewerPreloadCount: 3,
	ViewerFitMode:      FitModeHeight,
	EpubFontSize:       100,
	EpubTheme:          EpubThemeLight,
	EpubFlow:           EpubFlowPaginated,
	GallerySize:        GallerySizeMedium,
}
```

- [ ] **Step 2: Regenerate TS types**

Run: `mise tygo`
Expected: either silent success or "skipping, outputs are up-to-date" (which is normal). Verify the change shows up in `app/types/generated/`:

Run: `grep -n "GallerySize" app/types/generated/*.ts`
Expected: matches in the generated settings file showing the new `GallerySize` literal type and `gallery_size` field.

`app/types/generated/` is gitignored — the regenerated TS files don't get committed. Tests and the dev server pick them up locally; CI regenerates from the Go source.

- [ ] **Step 3: Commit**

```bash
git add pkg/models/user_settings.go
git commit -m "[Backend] Add GallerySize field to UserSettings model"
```

---

## Task 3: Backend rename — `/settings/viewer` → `/settings/user`

This task does not add any new behavior — it renames the wrapper around the existing `user_settings` row so adding non-viewer fields (gallery_size in Task 4) has a sensible home.

**Files:**
- Rename: `pkg/settings/handlers.go` stays put but its types/methods rename
- Rename: `pkg/settings/viewer_handlers_test.go` → `pkg/settings/user_handlers_test.go`
- Rename: `pkg/settings/viewer_service_test.go` → `pkg/settings/user_service_test.go`
- Modify: `pkg/settings/handlers.go`
- Modify: `pkg/settings/service.go`
- Modify: `pkg/settings/validators.go`
- Modify: `pkg/settings/routes.go`

- [ ] **Step 1: Run existing tests to confirm baseline pass**

Run: `go test ./pkg/settings/...`
Expected: PASS.

- [ ] **Step 2: Rename test files on disk**

```bash
git mv pkg/settings/viewer_handlers_test.go pkg/settings/user_handlers_test.go
git mv pkg/settings/viewer_service_test.go pkg/settings/user_service_test.go
```

- [ ] **Step 3: Rename validator types**

In `pkg/settings/validators.go`, rename:
- `ViewerSettingsPayload` → `UserSettingsPayload`
- `ViewerSettingsResponse` → `UserSettingsResponse`

Update doc comments to say "user settings" instead of "viewer settings". Field names stay (they keep their `viewer_` prefix because those genuinely are viewer fields).

- [ ] **Step 4: Rename service method and update struct**

In `pkg/settings/service.go`:
- `GetViewerSettings` → `GetUserSettings`
- `UpdateViewerSettings` → `UpdateUserSettings`
- `ViewerSettingsUpdate` → `UserSettingsUpdate`
- Update doc comment block above `UpdateUserSettings` to say "user settings".

- [ ] **Step 5: Rename handler methods**

In `pkg/settings/handlers.go`:
- `getViewerSettings` → `getUserSettings`
- `updateViewerSettings` → `updateUserSettings`
- Update call sites to use the renamed service methods and validator types.

The handler struct stays named `handler` (it's package-scoped).

- [ ] **Step 6: Rename routes**

In `pkg/settings/routes.go`, change:

```go
viewerH := &handler{settingsService: svc}
// ...
g.GET("/viewer", viewerH.getViewerSettings)
g.PUT("/viewer", viewerH.updateViewerSettings)
```

to:

```go
userH := &handler{settingsService: svc}
// ...
g.GET("/user", userH.getUserSettings)
g.PUT("/user", userH.updateUserSettings)
```

- [ ] **Step 7: Update test files to use renamed identifiers**

In `pkg/settings/user_handlers_test.go` and `pkg/settings/user_service_test.go`:
- Replace all references to renamed types/methods (`ViewerSettingsPayload` → `UserSettingsPayload`, `getViewerSettings` → `getUserSettings`, `/settings/viewer` → `/settings/user`, etc.).

Use this command to find any stragglers:

```bash
grep -rn "ViewerSettings\|getViewerSettings\|updateViewerSettings\|/settings/viewer\|/viewer" pkg/settings/
```

Expected: zero matches after this step.

- [ ] **Step 8: Run tests**

Run: `go test ./pkg/settings/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add pkg/settings/
git commit -m "[Backend] Rename /settings/viewer to /settings/user

The user_settings row was always per-user; the endpoint name was a
premature scope choice from when only EPUB viewer fields lived there.
Rename in advance of adding gallery_size."
```

---

## Task 4: Backend — add `gallery_size` to validators, handler, service

**Files:**
- Modify: `pkg/settings/validators.go`
- Modify: `pkg/settings/handlers.go`
- Modify: `pkg/settings/service.go`

- [ ] **Step 1: Add validator helper**

In `pkg/settings/validators.go`, add at the bottom of the existing helpers:

```go
// IsValidGallerySize returns true if the size is a supported gallery size.
func IsValidGallerySize(size string) bool {
	switch size {
	case models.GallerySizeSmall, models.GallerySizeMedium,
		models.GallerySizeLarge, models.GallerySizeExtraLarge:
		return true
	}
	return false
}
```

- [ ] **Step 2: Add `GallerySize` to payload and response**

In `pkg/settings/validators.go`, extend `UserSettingsPayload`:

```go
type UserSettingsPayload struct {
	PreloadCount *int    `json:"preload_count,omitempty"`
	FitMode      *string `json:"fit_mode,omitempty"`
	EpubFontSize *int    `json:"viewer_epub_font_size,omitempty"`
	EpubTheme    *string `json:"viewer_epub_theme,omitempty"`
	EpubFlow     *string `json:"viewer_epub_flow,omitempty"`
	GallerySize  *string `json:"gallery_size,omitempty"`
}
```

And `UserSettingsResponse`:

```go
type UserSettingsResponse struct {
	PreloadCount int    `json:"preload_count"`
	FitMode      string `json:"fit_mode"`
	EpubFontSize int    `json:"viewer_epub_font_size"`
	EpubTheme    string `json:"viewer_epub_theme"`
	EpubFlow     string `json:"viewer_epub_flow"`
	GallerySize  string `json:"gallery_size"`
}
```

- [ ] **Step 3: Add `GallerySize` to `UserSettingsUpdate`**

In `pkg/settings/service.go`, append to the struct:

```go
type UserSettingsUpdate struct {
	PreloadCount *int
	FitMode      *string
	EpubFontSize *int
	EpubTheme    *string
	EpubFlow     *string
	GallerySize  *string
}
```

The handler casts `UserSettingsPayload` to `UserSettingsUpdate` directly (`UserSettingsUpdate(payload)`), so the field order MUST stay identical to the payload.

- [ ] **Step 4: Apply gallery_size in service merge + upsert**

In `pkg/settings/service.go`, inside `UpdateUserSettings`, add the merge:

```go
if update.GallerySize != nil {
	current.GallerySize = *update.GallerySize
}
```

Place it just below the `EpubFlow` block to match field order.

In the same function, add to the `INSERT ON CONFLICT` chain:

```go
Set("gallery_size = EXCLUDED.gallery_size").
```

Place it just before `Returning("*")`.

- [ ] **Step 5: Add validation and pass-through in handler**

In `pkg/settings/handlers.go` `updateUserSettings`, add the validation block alongside the other field validators:

```go
if payload.GallerySize != nil && !IsValidGallerySize(*payload.GallerySize) {
	return errcodes.ValidationError("gallery_size must be 's', 'm', 'l', or 'xl'")
}
```

Update both `getUserSettings` and `updateUserSettings` response builders to include the new field:

```go
return c.JSON(http.StatusOK, UserSettingsResponse{
	PreloadCount: settings.ViewerPreloadCount,
	FitMode:      settings.ViewerFitMode,
	EpubFontSize: settings.EpubFontSize,
	EpubTheme:    settings.EpubTheme,
	EpubFlow:     settings.EpubFlow,
	GallerySize:  settings.GallerySize,
})
```

- [ ] **Step 6: Write failing tests for gallery_size handling**

Add three new top-level test functions in `pkg/settings/user_handlers_test.go`, following the existing flat-function style (not subtests-of-one). The file uses helpers `setupTestDB`, `createTestUser`, `newTestEcho` — already proven by the existing `TestUpdateUserSettings_*` tests.

```go
func TestUpdateUserSettings_AcceptsValidGallerySize(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "gally-valid")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	body := `{"gallery_size": "l"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.updateUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, models.GallerySizeLarge, resp.GallerySize)
}

func TestUpdateUserSettings_RejectsInvalidGallerySize(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "gally-bad")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	body := `{"gallery_size": "huge"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	err := h.updateUserSettings(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gallery_size")
}

func TestGetUserSettings_DefaultsToMediumGallerySize(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "gally-default")

	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	req := httptest.NewRequest(http.MethodGet, "/settings/user", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.getUserSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, models.GallerySizeMedium, resp.GallerySize)
}
```

- [ ] **Step 7: Run tests to verify failures**

Run: `go test ./pkg/settings/... -run UserSettings -v`
Expected: the three new sub-tests FAIL (the validator handler isn't returning the right values yet — actually this depends on whether you completed Steps 4–5 first; if you did, they should already PASS, which is fine).

If the implementation steps were already done, the tests should PASS now. The intent of writing them is to lock the behavior in; either order is acceptable as long as both implementation and tests land together.

- [ ] **Step 8: Commit**

```bash
git add pkg/settings/
git commit -m "[Backend] Persist and validate user gallery_size"
```

---

## Task 5: Frontend — rename `useViewerSettings` → `useUserSettings`

This task just renames the existing hook + URL to match the backend rename in Task 3. No behavior change.

**Files:**
- Modify: `app/hooks/queries/settings.ts`
- Modify: `app/components/pages/PageReader.tsx`
- Modify: `app/components/pages/EPUBReader.tsx`
- Modify: `app/components/pages/EPUBReader.test.tsx`

- [ ] **Step 1: Run baseline tests**

Run: `mise check:quiet`
Expected: PASS (this is the pre-rename baseline).

- [ ] **Step 2: Rename in the hook file**

In `app/hooks/queries/settings.ts`:
- `interface ViewerSettings` → `interface UserSettings`
- `enum QueryKey { ViewerSettings = "ViewerSettings" }` → `enum QueryKey { UserSettings = "UserSettings" }`
- `useViewerSettings` → `useUserSettings`
- `useUpdateViewerSettings` → `useUpdateUserSettings`
- `UpdateViewerSettingsVariables` → `UpdateUserSettingsVariables`
- `UseQueryOptions<ViewerSettings, ...>` → `UseQueryOptions<UserSettings, ...>` etc.
- URLs `"/settings/viewer"` → `"/settings/user"`
- All internal `[QueryKey.ViewerSettings]` array references → `[QueryKey.UserSettings]`

- [ ] **Step 3: Update call-sites**

```bash
grep -rln "useViewerSettings\|useUpdateViewerSettings\|ViewerSettings" app/ --include="*.ts" --include="*.tsx"
```

Expected files (besides the hook itself and the regenerated `app/types/generated/settings.ts`):
- `app/components/pages/PageReader.tsx`
- `app/components/pages/EPUBReader.tsx`
- `app/components/pages/EPUBReader.test.tsx`
- `app/types/index.ts` (re-export)

In each file, replace:
- `useViewerSettings` → `useUserSettings`
- `useUpdateViewerSettings` → `useUpdateUserSettings`
- `ViewerSettings` (the interface) → `UserSettings`

In `app/types/index.ts`, update any re-export of `ViewerSettings` to `UserSettings`.

- [ ] **Step 4: Verify no stragglers**

Run: `grep -rn "ViewerSettings\|useViewerSettings\|/settings/viewer" app/ --include="*.ts" --include="*.tsx"`
Expected: zero matches.

- [ ] **Step 5: Run all checks**

Run: `mise check:quiet`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/
git commit -m "[Frontend] Rename useViewerSettings to useUserSettings

Mirrors the backend rename of /settings/viewer to /settings/user in
preparation for adding gallery_size and other future per-user prefs."
```

---

## Task 6: Frontend — extend `UserSettings` hook with `gallery_size`

**Files:**
- Modify: `app/hooks/queries/settings.ts`

- [ ] **Step 1: Add `gallery_size` to interface and update payload**

```ts
import type { EpubFlow, EpubTheme, FitMode, GallerySize } from "@/types";

export interface UserSettings {
  preload_count: number;
  fit_mode: FitMode;
  viewer_epub_font_size: number;
  viewer_epub_theme: EpubTheme;
  viewer_epub_flow: EpubFlow;
  gallery_size: GallerySize;
}

export interface UpdateUserSettingsVariables {
  preload_count?: number;
  fit_mode?: FitMode;
  viewer_epub_font_size?: number;
  viewer_epub_theme?: EpubTheme;
  viewer_epub_flow?: EpubFlow;
  gallery_size?: GallerySize;
}
```

If `GallerySize` isn't re-exported from `@/types`, add it to `app/types/index.ts` next to `EpubTheme`.

- [ ] **Step 2: Verify TS compiles**

Run: `pnpm lint:types`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add app/hooks/queries/settings.ts app/types/index.ts
git commit -m "[Frontend] Add gallery_size to UserSettings hook"
```

---

## Task 7: Frontend constants — `app/constants/gallerySize.ts`

**Files:**
- Create: `app/constants/gallerySize.ts`

- [ ] **Step 1: Write the constants file**

```ts
import type { GallerySize } from "@/types";

export const GALLERY_SIZES: readonly GallerySize[] = ["s", "m", "l", "xl"];

export const DEFAULT_GALLERY_SIZE: GallerySize = "m";

export const GALLERY_SIZE_LABELS: Record<GallerySize, string> = {
  s: "S",
  m: "M",
  l: "L",
  xl: "XL",
};

// sm: applies at >= 640px. Below sm:, BookItem uses w-[calc(50%-0.5rem)]
// (forced 2-col), so size has no visual effect on mobile.
export const COVER_WIDTH_CLASS: Record<GallerySize, string> = {
  s: "sm:w-24",
  m: "sm:w-32",
  l: "sm:w-44",
  xl: "sm:w-56",
};

// Backend max list limit is 50. If you raise any of these to >50 the API
// will silently clip and pagination will break.
export const ITEMS_PER_PAGE_BY_SIZE: Record<GallerySize, number> = {
  s: 48,
  m: 24,
  l: 16,
  xl: 12,
};
```

- [ ] **Step 2: Verify it compiles**

Run: `pnpm lint:types`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add app/constants/gallerySize.ts
git commit -m "[Frontend] Add gallery size constants"
```

---

## Task 8: Frontend pure utils — `app/libraries/gallerySize.ts` (TDD)

**Files:**
- Create: `app/libraries/gallerySize.test.ts`
- Create: `app/libraries/gallerySize.ts`

- [ ] **Step 1: Write the failing test file**

`app/libraries/gallerySize.test.ts`:

```ts
import { describe, expect, it } from "vitest";

import {
  pageForSizeChange,
  parseGallerySize,
} from "@/libraries/gallerySize";

describe("parseGallerySize", () => {
  it("accepts the four valid sizes", () => {
    expect(parseGallerySize("s")).toBe("s");
    expect(parseGallerySize("m")).toBe("m");
    expect(parseGallerySize("l")).toBe("l");
    expect(parseGallerySize("xl")).toBe("xl");
  });

  it("rejects nullish input", () => {
    expect(parseGallerySize(null)).toBeNull();
    expect(parseGallerySize("")).toBeNull();
  });

  it("is case-sensitive (matches sort behavior)", () => {
    expect(parseGallerySize("S")).toBeNull();
    expect(parseGallerySize("Medium")).toBeNull();
  });

  it("rejects unknown values", () => {
    expect(parseGallerySize("xxl")).toBeNull();
    expect(parseGallerySize("huge")).toBeNull();
  });
});

describe("pageForSizeChange", () => {
  it("preserves the first-visible item across size changes", () => {
    // M page 5 = offset 96. Switch to S (limit 48) -> page 3, which
    // covers offsets 96-143. First-visible book stays at top.
    expect(pageForSizeChange(96, 48)).toBe(3);
    // M page 5 = offset 96. Switch to L (limit 16) -> page 7, covers 96-111.
    expect(pageForSizeChange(96, 16)).toBe(7);
    // M page 5 = offset 96. Switch to XL (limit 12) -> page 9, covers 96-107.
    expect(pageForSizeChange(96, 12)).toBe(9);
  });

  it("errs backward when boundaries don't align", () => {
    // offset 99, new limit 12 -> floor(99/12)=8 -> page 9 covers 96-107.
    // User sees their old book #99 plus three earlier books — never skips ahead.
    expect(pageForSizeChange(99, 12)).toBe(9);
  });

  it("returns page 1 at offset 0", () => {
    expect(pageForSizeChange(0, 48)).toBe(1);
    expect(pageForSizeChange(0, 12)).toBe(1);
  });

  it("returns page 1 when offset < new_limit", () => {
    expect(pageForSizeChange(11, 12)).toBe(1);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm vitest run app/libraries/gallerySize.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the utils**

`app/libraries/gallerySize.ts`:

```ts
import { GALLERY_SIZES } from "@/constants/gallerySize";
import type { GallerySize } from "@/types";

export const parseGallerySize = (raw: string | null): GallerySize | null => {
  if (!raw) return null;
  return (GALLERY_SIZES as readonly string[]).includes(raw)
    ? (raw as GallerySize)
    : null;
};

export const pageForSizeChange = (
  oldOffset: number,
  newLimit: number,
): number => {
  return Math.floor(oldOffset / newLimit) + 1;
};
```

- [ ] **Step 4: Run tests to verify pass**

Run: `pnpm vitest run app/libraries/gallerySize.test.ts`
Expected: PASS, 8 tests passing.

- [ ] **Step 5: Commit**

```bash
git add app/libraries/gallerySize.ts app/libraries/gallerySize.test.ts
git commit -m "[Frontend] Add gallerySize parse and page-preservation utils"
```

---

## Task 9: Frontend — `SizeSelector` component (TDD)

The shared 4-button segmented control reused by `SizePopover` and `UserSettings.tsx`.

**Files:**
- Create: `app/components/library/SizeSelector.tsx`
- Create: `app/components/library/SizeSelector.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SizeSelector } from "@/components/library/SizeSelector";

describe("SizeSelector", () => {
  it("renders one button per size", () => {
    render(<SizeSelector value="m" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "S" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "M" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "L" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "XL" })).toBeInTheDocument();
  });

  it("marks the active size with aria-pressed", () => {
    render(<SizeSelector value="l" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "L" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "M" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("calls onChange with the clicked size", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    render(<SizeSelector value="m" onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: "L" }));
    expect(onChange).toHaveBeenCalledWith("l");
  });
});
```

- [ ] **Step 2: Run test to verify fail**

Run: `pnpm vitest run app/components/library/SizeSelector.test.tsx`
Expected: FAIL — component not found.

- [ ] **Step 3: Implement the component**

```tsx
import { Button } from "@/components/ui/button";
import {
  GALLERY_SIZE_LABELS,
  GALLERY_SIZES,
} from "@/constants/gallerySize";
import { cn } from "@/libraries/utils";
import type { GallerySize } from "@/types";

interface SizeSelectorProps {
  value: GallerySize;
  onChange: (size: GallerySize) => void;
  className?: string;
}

export const SizeSelector = ({
  value,
  onChange,
  className,
}: SizeSelectorProps) => {
  return (
    <div
      className={cn("inline-flex rounded-md border bg-background", className)}
      role="group"
    >
      {GALLERY_SIZES.map((size, index) => {
        const isActive = size === value;
        return (
          <Button
            aria-pressed={isActive}
            className={cn(
              "h-8 rounded-none px-3 text-xs font-semibold border-0",
              index > 0 && "border-l",
              isActive
                ? "bg-primary text-primary-foreground hover:bg-primary"
                : "bg-transparent",
            )}
            key={size}
            onClick={() => onChange(size)}
            type="button"
            variant="ghost"
          >
            {GALLERY_SIZE_LABELS[size]}
          </Button>
        );
      })}
    </div>
  );
};
```

- [ ] **Step 4: Run test to verify pass**

Run: `pnpm vitest run app/components/library/SizeSelector.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/components/library/SizeSelector.tsx app/components/library/SizeSelector.test.tsx
git commit -m "[Frontend] Add SizeSelector segmented-button component"
```

---

## Task 10: Frontend — `SizePopover` and `SizeButton` (TDD)

**Files:**
- Create: `app/components/library/SizePopover.tsx`
- Create: `app/components/library/SizePopover.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import {
  SizeButton,
  SizePopover,
} from "@/components/library/SizePopover";

describe("SizePopover", () => {
  const renderPopover = (overrides: Partial<{
    effectiveSize: "s" | "m" | "l" | "xl";
    savedSize: "s" | "m" | "l" | "xl";
    onChange: ReturnType<typeof vi.fn>;
    onSaveAsDefault: ReturnType<typeof vi.fn>;
    isSaving: boolean;
  }> = {}) => {
    const onChange = overrides.onChange ?? vi.fn();
    const onSaveAsDefault = overrides.onSaveAsDefault ?? vi.fn();
    render(
      <SizePopover
        effectiveSize={overrides.effectiveSize ?? "m"}
        savedSize={overrides.savedSize ?? "m"}
        isSaving={overrides.isSaving ?? false}
        onChange={onChange}
        onSaveAsDefault={onSaveAsDefault}
        trigger={<SizeButton isDirty={false} />}
      />,
    );
    return { onChange, onSaveAsDefault };
  };

  it("opens when the trigger is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPopover();
    await user.click(screen.getByRole("button", { name: /size/i }));
    expect(screen.getByRole("button", { name: "M" })).toBeInTheDocument();
  });

  it("hides the save-as-default card when effective size matches saved", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPopover({ effectiveSize: "m", savedSize: "m" });
    await user.click(screen.getByRole("button", { name: /size/i }));
    expect(
      screen.queryByRole("button", { name: /save as my default everywhere/i }),
    ).not.toBeInTheDocument();
  });

  it("shows the save-as-default card when sizes differ and calls handler", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onSaveAsDefault } = renderPopover({
      effectiveSize: "l",
      savedSize: "m",
    });
    await user.click(screen.getByRole("button", { name: /size/i }));
    const saveBtn = await screen.findByRole("button", {
      name: /save as my default everywhere/i,
    });
    expect(
      screen.getByText("Other users won't be affected."),
    ).toBeInTheDocument();
    await user.click(saveBtn);
    expect(onSaveAsDefault).toHaveBeenCalledTimes(1);
  });
});

describe("SizeButton", () => {
  it("shows a dirty dot when isDirty=true", () => {
    render(<SizeButton isDirty={true} />);
    expect(
      screen.getByLabelText("Size differs from default"),
    ).toBeInTheDocument();
  });

  it("hides the dirty dot when isDirty=false", () => {
    render(<SizeButton isDirty={false} />);
    expect(
      screen.queryByLabelText("Size differs from default"),
    ).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify fail**

Run: `pnpm vitest run app/components/library/SizePopover.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the components**

```tsx
import { Maximize2, Save } from "lucide-react";
import { useState } from "react";

import { SizeSelector } from "@/components/library/SizeSelector";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import type { GallerySize } from "@/types";

interface SizePopoverProps {
  effectiveSize: GallerySize;
  savedSize: GallerySize;
  isSaving: boolean;
  onChange: (size: GallerySize) => void;
  onSaveAsDefault: () => void;
  trigger: React.ReactNode;
}

export const SizePopover = ({
  effectiveSize,
  savedSize,
  isSaving,
  onChange,
  onSaveAsDefault,
  trigger,
}: SizePopoverProps) => {
  const [open, setOpen] = useState(false);
  const isDirty = effectiveSize !== savedSize;

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent align="start" className="w-auto p-3">
        <div className="flex flex-col gap-3">
          <SizeSelector onChange={onChange} value={effectiveSize} />
          {isDirty && (
            <div className="border border-dashed rounded-md p-3">
              <p className="text-xs text-muted-foreground mb-2">
                Other users won't be affected.
              </p>
              <Button
                disabled={isSaving}
                onClick={onSaveAsDefault}
                size="sm"
              >
                <Save className="h-4 w-4" />
                {isSaving ? "Saving…" : "Save as my default everywhere"}
              </Button>
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
};

export const SizeButton = ({
  isDirty,
  onClick,
}: {
  isDirty: boolean;
  onClick?: () => void;
}) => (
  <Button className="relative" onClick={onClick} size="sm" variant="outline">
    <Maximize2 className="h-4 w-4" />
    Size
    {isDirty && (
      <span
        aria-label="Size differs from default"
        className="absolute top-1 right-1 h-2 w-2 rounded-full bg-primary ring-2 ring-background"
      />
    )}
  </Button>
);
```

- [ ] **Step 4: Run test to verify pass**

Run: `pnpm vitest run app/components/library/SizePopover.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/components/library/SizePopover.tsx app/components/library/SizePopover.test.tsx
git commit -m "[Frontend] Add SizePopover and SizeButton"
```

---

## Task 11: Frontend — thread `gallerySize` through `BookItem` + `SelectableBookItem`

**Files:**
- Modify: `app/components/library/BookItem.tsx`
- Modify: `app/components/library/SelectableBookItem.tsx`

- [ ] **Step 1: Update `BookItem` to accept and apply `gallerySize`**

In `app/components/library/BookItem.tsx`:

Add to imports:

```ts
import { COVER_WIDTH_CLASS, DEFAULT_GALLERY_SIZE } from "@/constants/gallerySize";
import type { GallerySize } from "@/types";
```

Add to `BookItemProps`:

```ts
gallerySize?: GallerySize;
```

Add to the destructuring with the default:

```ts
const BookItem = ({
  // ... existing props
  gallerySize = DEFAULT_GALLERY_SIZE,
  // ...
}: BookItemProps) => {
```

Replace the wrapping `<div>`'s `className` (currently `"w-[calc(50%-0.5rem)] sm:w-32 group/card relative"`) with:

```tsx
className={cn(
  "w-[calc(50%-0.5rem)] group/card relative",
  COVER_WIDTH_CLASS[gallerySize],
  isSelectionMode && "cursor-pointer",
)}
```

- [ ] **Step 2: Update `SelectableBookItem` to forward the prop**

In `app/components/library/SelectableBookItem.tsx`:

Add to imports:

```ts
import type { GallerySize } from "@/types";
```

Add to `SelectableBookItemProps`:

```ts
gallerySize?: GallerySize;
```

Add to the destructuring and pass through to `BookItem`:

```tsx
gallerySize,
// ...
<BookItem
  // ... existing
  gallerySize={gallerySize}
  // ...
/>
```

- [ ] **Step 3: Verify TS and tests still pass**

Run: `pnpm vitest run app/components/library/`
Expected: PASS (existing BookItem callers unaffected because the prop has a default).

- [ ] **Step 4: Commit**

```bash
git add app/components/library/BookItem.tsx app/components/library/SelectableBookItem.tsx
git commit -m "[Frontend] Thread gallerySize prop through BookItem"
```

---

## Task 12: Frontend — wire size into `Home.tsx`

The library home is the most complex page (also reads library settings for sort). It's the template for all other gallery pages — get this right and the others are mechanical.

**Files:**
- Modify: `app/components/pages/Home.tsx`

- [ ] **Step 1: Add imports**

Append to the existing imports block:

```ts
import { SizeButton, SizePopover } from "@/components/library/SizePopover";
import {
  DEFAULT_GALLERY_SIZE,
  ITEMS_PER_PAGE_BY_SIZE,
} from "@/constants/gallerySize";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import {
  pageForSizeChange,
  parseGallerySize,
} from "@/libraries/gallerySize";
import type { GallerySize } from "@/types";
```

- [ ] **Step 2: Remove the hard-coded `ITEMS_PER_PAGE` constant**

Delete the line:

```ts
const ITEMS_PER_PAGE = 24;
```

We'll compute it from the effective size.

- [ ] **Step 3: Add user-settings hooks and resolve effective size**

Inside `HomeContent`, after the existing `librarySettingsQuery` / `updateLibrarySettings` block, add:

```ts
const userSettingsQuery = useUserSettings();
const updateUserSettings = useUpdateUserSettings();

const sizeParam = searchParams.get("size");
const urlSize: GallerySize | null = parseGallerySize(sizeParam);
const savedSize: GallerySize =
  userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;
const effectiveSize: GallerySize = urlSize ?? savedSize;
const isSizeDirty = urlSize !== null && urlSize !== savedSize;
const itemsPerPage = ITEMS_PER_PAGE_BY_SIZE[effectiveSize];
```

- [ ] **Step 4: Update the `settingsResolved` gate to also wait for user settings**

Replace:

```ts
const settingsResolved =
  libraryIdNum === undefined ||
  librarySettingsQuery.isSuccess ||
  librarySettingsQuery.isError;
```

with:

```ts
const librarySettingsResolved =
  libraryIdNum === undefined ||
  librarySettingsQuery.isSuccess ||
  librarySettingsQuery.isError;
const userSettingsResolved =
  userSettingsQuery.isSuccess || userSettingsQuery.isError;
const settingsResolved = librarySettingsResolved && userSettingsResolved;
```

- [ ] **Step 5: Replace `ITEMS_PER_PAGE` references with `itemsPerPage`**

Search inside `Home.tsx`:

```bash
grep -n "ITEMS_PER_PAGE" app/components/pages/Home.tsx
```

Replace each occurrence with `itemsPerPage`. Three sites:
- `const limit = ITEMS_PER_PAGE;` → `const limit = itemsPerPage;`
- `itemsPerPage={ITEMS_PER_PAGE}` (passed to `<Gallery>`) → `itemsPerPage={itemsPerPage}`

- [ ] **Step 6: Add size-change handler and save-as-default handler**

Below the existing `handleSaveSortAsDefault`/`applySortLevels` definitions, add:

```ts
const applyGallerySize = (next: GallerySize) => {
  setSearchParams((prev) => {
    const params = new URLSearchParams(prev);
    if (next === savedSize) {
      params.delete("size");
    } else {
      params.set("size", next);
    }
    const newPage = pageForSizeChange(offset, ITEMS_PER_PAGE_BY_SIZE[next]);
    params.set("page", String(newPage));
    return params;
  });
};

const handleSaveSizeAsDefault = () => {
  updateUserSettings.mutate(
    { gallery_size: effectiveSize },
    {
      onSuccess: () => {
        setSearchParams((prev) => {
          const params = new URLSearchParams(prev);
          params.delete("size");
          return params;
        });
      },
    },
  );
};
```

- [ ] **Step 7: Add `<SizePopover>` to the toolbar**

In the toolbar JSX block, find the `<SortSheet ... />` element and place the size popover immediately after it, wrapped to hide on mobile:

```tsx
<div className="hidden sm:flex">
  <SizePopover
    effectiveSize={effectiveSize}
    isSaving={updateUserSettings.isPending}
    onChange={applyGallerySize}
    onSaveAsDefault={handleSaveSizeAsDefault}
    savedSize={savedSize}
    trigger={<SizeButton isDirty={isSizeDirty} />}
  />
</div>
```

- [ ] **Step 8: Pass `gallerySize` to the rendered book items**

In `renderBookItem`:

```tsx
const renderBookItem = (book: Book) => (
  <SelectableBookItem
    book={book}
    cacheKey={booksQuery.dataUpdatedAt}
    coverAspectRatio={coverAspectRatio}
    gallerySize={effectiveSize}
    key={book.id}
    libraryId={libraryId!}
    pageBookIds={pageBookIds}
    seriesId={seriesId}
  />
);
```

- [ ] **Step 9: Run checks**

Run: `mise check:quiet`
Expected: PASS.

- [ ] **Step 10: Manual smoke test**

Run: `mise start`
Open `http://localhost:5173/libraries/1` in a browser. Verify:
- Size button appears in the toolbar (between Sort and Select).
- Clicking it opens the popover with four buttons; default highlight is M.
- Picking L makes covers larger and bumps `?size=l` into the URL.
- Save-as-default card appears; clicking it removes `?size=l` and persists the choice (refresh: covers stay L).
- Switching from L on page 5 to S preserves position (lands on page near same books).

If `mise start` isn't running here, say so explicitly rather than skipping.

- [ ] **Step 11: Commit**

```bash
git add app/components/pages/Home.tsx
git commit -m "[Frontend] Wire gallery size into library home"
```

---

## Task 13: Frontend — wire size into the other 6 gallery pages

The pattern is identical to Task 12 minus the library-settings (sort) wiring. Only Home + SeriesList + SeriesDetail + GenreDetail + TagDetail + PersonDetail + ListDetail render the book gallery — text-list pages (GenresList, TagsList, PersonList, etc.) don't.

**Files:**
- Modify: `app/components/pages/SeriesList.tsx`
- Modify: `app/components/pages/SeriesDetail.tsx`
- Modify: `app/components/pages/GenreDetail.tsx`
- Modify: `app/components/pages/TagDetail.tsx`
- Modify: `app/components/pages/PersonDetail.tsx`
- Modify: `app/components/pages/ListDetail.tsx`

For **each** of the six files, perform Steps 1–6 below. The diff shape is the same; only the surrounding code differs.

- [ ] **Step 1: Add imports**

Add to the imports block:

```ts
import { SizeButton, SizePopover } from "@/components/library/SizePopover";
import {
  DEFAULT_GALLERY_SIZE,
  ITEMS_PER_PAGE_BY_SIZE,
} from "@/constants/gallerySize";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import {
  pageForSizeChange,
  parseGallerySize,
} from "@/libraries/gallerySize";
import type { GallerySize } from "@/types";
```

- [ ] **Step 2: Delete the hard-coded `ITEMS_PER_PAGE` constant**

Each file currently declares `const ITEMS_PER_PAGE = 24;` near the top. Delete it.

- [ ] **Step 3: Resolve effective size**

Inside the page's main component, near where `searchParams` / `currentPage` are derived, add:

```ts
const userSettingsQuery = useUserSettings();
const updateUserSettings = useUpdateUserSettings();

const urlSize: GallerySize | null = parseGallerySize(searchParams.get("size"));
const savedSize: GallerySize =
  userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;
const effectiveSize: GallerySize = urlSize ?? savedSize;
const isSizeDirty = urlSize !== null && urlSize !== savedSize;
const itemsPerPage = ITEMS_PER_PAGE_BY_SIZE[effectiveSize];
```

If the page doesn't already use `useSearchParams`, add the standard `const [searchParams, setSearchParams] = useSearchParams();` line.

- [ ] **Step 4: Replace `ITEMS_PER_PAGE` with `itemsPerPage` (and gate on settings)**

Replace each `ITEMS_PER_PAGE` reference with `itemsPerPage` (typically: `const limit = itemsPerPage;` and `<Gallery itemsPerPage={itemsPerPage} />`).

If the page renders `<Gallery>` directly without an explicit settings gate, gate the books query enable on user-settings being resolved:

```ts
const userSettingsResolved =
  userSettingsQuery.isSuccess || userSettingsQuery.isError;
// ... existing query opts
{
  enabled: userSettingsResolved && /* existing conditions */,
}
```

If the page doesn't pass `enabled` today (the books query is unconditional), add it. Without the gate, users with a saved L would see a flash of M-sized covers before settings load.

- [ ] **Step 5: Add the size handlers and the popover**

Add the handlers (same as Task 12 Step 6):

```ts
const offset = (currentPage - 1) * itemsPerPage;

const applyGallerySize = (next: GallerySize) => {
  setSearchParams((prev) => {
    const params = new URLSearchParams(prev);
    if (next === savedSize) {
      params.delete("size");
    } else {
      params.set("size", next);
    }
    const newPage = pageForSizeChange(offset, ITEMS_PER_PAGE_BY_SIZE[next]);
    params.set("page", String(newPage));
    return params;
  });
};

const handleSaveSizeAsDefault = () => {
  updateUserSettings.mutate(
    { gallery_size: effectiveSize },
    {
      onSuccess: () => {
        setSearchParams((prev) => {
          const params = new URLSearchParams(prev);
          params.delete("size");
          return params;
        });
      },
    },
  );
};
```

Place the popover next to whatever toolbar/header exists. If the page has no toolbar (e.g. PersonDetail/GenreDetail), add a single-row toolbar above the gallery:

```tsx
<div className="hidden sm:flex justify-end mb-4">
  <SizePopover
    effectiveSize={effectiveSize}
    isSaving={updateUserSettings.isPending}
    onChange={applyGallerySize}
    onSaveAsDefault={handleSaveSizeAsDefault}
    savedSize={savedSize}
    trigger={<SizeButton isDirty={isSizeDirty} />}
  />
</div>
```

- [ ] **Step 6: Pass `gallerySize` to book items rendered on this page**

Find every `<BookItem ... />` or `<SelectableBookItem ... />` rendered by this page and add the `gallerySize={effectiveSize}` prop. Some pages use raw `BookItem` (PersonDetail, GenreDetail, TagDetail, ListDetail), others use `SelectableBookItem` (Home/SeriesList/SeriesDetail style); both accept the prop now.

- [ ] **Step 7: Per-file lint + test pass**

After updating all six files, run:

Run: `mise check:quiet`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add app/components/pages/SeriesList.tsx app/components/pages/SeriesDetail.tsx app/components/pages/GenreDetail.tsx app/components/pages/TagDetail.tsx app/components/pages/PersonDetail.tsx app/components/pages/ListDetail.tsx
git commit -m "[Frontend] Wire gallery size into Series/Genre/Tag/Person/List galleries"
```

---

## Task 14: Frontend — Gallery Size on User Settings page

**Files:**
- Modify: `app/components/pages/UserSettings.tsx`

- [ ] **Step 1: Add imports**

```ts
import { SizeSelector } from "@/components/library/SizeSelector";
import { DEFAULT_GALLERY_SIZE } from "@/constants/gallerySize";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import type { GallerySize } from "@/types";
```

- [ ] **Step 2: Read user settings and wire change handler**

Inside the `UserSettings` component, alongside the existing `useTheme()`:

```ts
const userSettingsQuery = useUserSettings();
const updateUserSettings = useUpdateUserSettings();
const gallerySize: GallerySize =
  userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;

const handleGallerySizeChange = (next: GallerySize) => {
  updateUserSettings.mutate({ gallery_size: next });
};
```

- [ ] **Step 3: Add the row inside the existing Appearance section**

Inside the `<section>` containing the theme picker, below the theme rows, add:

```tsx
<div className="mt-6 flex flex-col gap-2">
  <label className="text-sm font-medium">Gallery cover size</label>
  <SizeSelector
    onChange={handleGallerySizeChange}
    value={gallerySize}
  />
  <p className="text-xs text-muted-foreground">
    Applies to every gallery page. Used as your default; changes made
    inline on a gallery page override this until you save them.
  </p>
</div>
```

The exact JSX shape may need to adapt to existing markup in the section — match the surrounding patterns (labels, helper text styling).

- [ ] **Step 4: Run checks**

Run: `mise check:quiet`
Expected: PASS.

- [ ] **Step 5: Manual smoke test**

If `mise start` is running, navigate to `/settings/user-settings` (or wherever UserSettings is routed — confirm via `app/router.tsx`) and verify:
- Four S/M/L/XL buttons appear under the Appearance section.
- Clicking one persists immediately (refresh page; choice sticks).
- Choice is reflected next time you open a gallery page (no `?size=` URL param).

- [ ] **Step 6: Commit**

```bash
git add app/components/pages/UserSettings.tsx
git commit -m "[Frontend] Add gallery size picker to User Settings"
```

---

## Task 15: Sort copy fix in `SortSheet.tsx`

**Files:**
- Modify: `app/components/library/SortSheet.tsx`

- [ ] **Step 1: Update the "Save as default" card copy**

Find the dirty-state JSX block (currently lines ~284–294):

```tsx
{isDirty && (
  <div className="border border-dashed rounded-md p-3">
    <p className="text-sm text-muted-foreground mb-2">
      Save this as the default for this library?
    </p>
    <Button disabled={isSaving} onClick={onSaveAsDefault} size="sm">
      <Save className="h-4 w-4" />
      {isSaving ? "Saving…" : "Save as default"}
    </Button>
  </div>
)}
```

Replace with:

```tsx
{isDirty && (
  <div className="border border-dashed rounded-md p-3">
    <p className="text-xs text-muted-foreground mb-2">
      Other users won't be affected.
    </p>
    <Button disabled={isSaving} onClick={onSaveAsDefault} size="sm">
      <Save className="h-4 w-4" />
      {isSaving ? "Saving…" : "Save as my default for this library"}
    </Button>
  </div>
)}
```

- [ ] **Step 2: Run checks**

Run: `mise check:quiet`
Expected: PASS. (No tests assert on this string today; if any do, update them to match.)

- [ ] **Step 3: Commit**

```bash
git add app/components/library/SortSheet.tsx
git commit -m "[Frontend] Clarify sort save-as-default is per-user"
```

---

## Task 16: Docs — new `gallery-size.md` page + sort cross-link

**Files:**
- Create: `website/docs/gallery-size.md`
- Modify: `website/docs/gallery-sort.md`

- [ ] **Step 1: Create the new page**

Use `sidebar_position: 155` so it slots immediately after `gallery-sort.md` (position 150) without renumbering anything else.

```md
---
sidebar_position: 155
---

# Gallery Size

Pick how large book covers display in your gallery — Small, Medium, Large, or Extra Large. The choice applies everywhere you see covers (library home, series, genres, tags, people, lists).

## Using the Size popover

Click **Size** in the gallery toolbar to open the size popover. Pick S / M / L / XL — covers resize immediately and the gallery jumps to a page that contains the book you were last looking at, so you don't lose your place.

A dot on the Size button means your current size differs from your saved default.

The popover is hidden on small screens, where covers always render two per row regardless of size. Edit your saved default from User Settings on a phone, and it'll apply next time you're on a wider screen.

## Sizes

| Size | Cover width (desktop) | Books per page |
|------|----------------------|----------------|
| S    | 96px                 | 48 |
| M    | 128px (default)      | 24 |
| L    | 176px                | 16 |
| XL   | 224px                | 12 |

Items per page scale so screen density stays roughly constant — bigger covers, fewer per page.

## URL-addressable size

Non-default sizes live in the URL as `?size=s|m|l|xl`. You can share or bookmark a sized view and it loads at that size — for that recipient on that page only.

```
?size=l
```

When the URL has no `size` parameter, the gallery uses your saved default (or **M** if you haven't saved one).

## Saving a default

When your current size differs from your saved default, the size popover shows a **Save as my default everywhere** button. Clicking it:

1. Saves your current size as your new default for every gallery page.
2. Clears the `?size=` parameter from the URL — you're now viewing the default.

Defaults are per user — saving doesn't affect other users.

You can also edit the default from your **User Settings** page under Appearance.

## See also

- [Gallery Sort](./gallery-sort.md) for sorting books by author, series, date added, and more.
```

- [ ] **Step 2: Cross-link from gallery-sort.md**

In `website/docs/gallery-sort.md`, add a "See also" section at the bottom (or extend the existing "How this affects other surfaces" section):

```md
## See also

- [Gallery Size](./gallery-size.md) for resizing book covers in the gallery.
```

- [ ] **Step 3: Verify docs build**

Run: `cd website && pnpm build`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add website/docs/gallery-size.md website/docs/gallery-sort.md
git commit -m "[Docs] Add gallery size docs page"
```

---

## Task 17: Final sweep

- [ ] **Step 1: Full check**

Run: `mise check:quiet`
Expected: PASS.

- [ ] **Step 2: Verify worktree state**

Run: `git status` and `git log --oneline master..HEAD`
Expected: working tree clean; commit history shows the migration → backend rename → backend gallery_size → frontend rename → frontend hook extension → constants → utils → SizeSelector → SizePopover → BookItem → Home → other pages → UserSettings → sort copy → docs progression.

- [ ] **Step 3: Squash-merge readiness**

The branch is ready to hand to `squash-merge-worktree` skill (or the user's preferred merge flow). No new task needed here.
