# EPUB Reader Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an in-app EPUB reader that renders reflowable EPUB content using foliate-js, with font size / theme / flow settings persisted per user.

**Architecture:** A new `EPUBReader` React component (sibling of the existing `PageReader`) mounts foliate-js's `<foliate-view>` custom element, fed by a Blob fetched from the existing `/api/books/files/:id/download` endpoint (which serves a regenerated EPUB with current metadata and cover). Three new EPUB-specific fields are added to the existing `user_settings` table. The EPUB file is dispatched by `FileReader.tsx` based on `file_type`.

**Tech Stack:** React 19, TypeScript, Tanstack Query, Go, Bun ORM, SQLite. Vendored ES modules from foliate-js (https://github.com/johnfactotum/foliate-js) — no npm package.

**Spec:** `docs/superpowers/specs/2026-04-22-epub-reader-design.md`

---

## Background context for the implementing engineer

Read these first:

- `docs/superpowers/specs/2026-04-22-epub-reader-design.md` — the design this plan implements.
- `CLAUDE.md`, `pkg/CLAUDE.md`, `app/CLAUDE.md`, `pkg/epub/CLAUDE.md` — project conventions. Especially: snake_case JSON, `t.Parallel()` in Go tests, `mise check:quiet` before committing, update `website/docs/` for user-facing changes.
- `app/components/pages/PageReader.tsx` — the existing CBZ/PDF reader. Copy its chrome patterns (back link, header layout, progress-bar click-to-seek, keyboard bindings, tap zones).
- `app/components/pages/FileReader.tsx` — where the dispatch switch lives. You'll add a `case FileTypeEPUB:` branch.
- `pkg/settings/` and `app/hooks/queries/settings.ts` — existing viewer settings plumbing you'll extend.

**foliate-js vendor reference:** https://github.com/johnfactotum/foliate-js — no npm package, no build step, distributed as plain ES modules. You'll clone a pinned commit into `app/libraries/foliate/`.

**Key gotcha:** `user_settings.tstype:"-"` on the Go model means tygo skips this struct. The frontend `ViewerSettings` TypeScript interface is hand-maintained in `app/hooks/queries/settings.ts`. You must update it manually.

---

## Task 1: Extend `user_settings` with EPUB fields (backend)

**Files:**
- Create: `pkg/migrations/20260422000000_add_epub_viewer_settings.go`
- Modify: `pkg/models/user_settings.go`
- Modify: `pkg/settings/validators.go`
- Modify: `pkg/settings/service.go`
- Modify: `pkg/settings/handlers.go`
- Create: `pkg/settings/viewer_service_test.go`
- Create: `pkg/settings/viewer_handlers_test.go`

- [ ] **Step 1.1: Write failing service test for EPUB defaults**

Create `pkg/settings/viewer_service_test.go`:

```go
package settings

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetViewerSettings_ReturnsEpubDefaults(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "alice")
	svc := NewService(db)

	settings, err := svc.GetViewerSettings(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, settings.EpubFontSize)
	assert.Equal(t, models.EpubThemeLight, settings.EpubTheme)
	assert.Equal(t, models.EpubFlowPaginated, settings.EpubFlow)
}

func TestUpdateViewerSettings_PersistsEpubFields(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "bob")
	svc := NewService(db)

	updated, err := svc.UpdateViewerSettings(
		context.Background(),
		user.ID,
		5, "original",
		140, models.EpubThemeSepia, models.EpubFlowScrolled,
	)
	require.NoError(t, err)
	assert.Equal(t, 140, updated.EpubFontSize)
	assert.Equal(t, models.EpubThemeSepia, updated.EpubTheme)
	assert.Equal(t, models.EpubFlowScrolled, updated.EpubFlow)

	// Re-read to confirm persistence
	reloaded, err := svc.GetViewerSettings(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 140, reloaded.EpubFontSize)
	assert.Equal(t, models.EpubThemeSepia, reloaded.EpubTheme)
	assert.Equal(t, models.EpubFlowScrolled, reloaded.EpubFlow)
}
```

- [ ] **Step 1.2: Run the tests and confirm they fail**

Run: `go test ./pkg/settings/ -run TestGetViewerSettings_ReturnsEpubDefaults -v`
Expected: FAIL — `EpubFontSize`, `EpubThemeLight`, `EpubFlowPaginated` do not exist. `UpdateViewerSettings` has wrong signature.

- [ ] **Step 1.3: Add the migration**

Create `pkg/migrations/20260422000000_add_epub_viewer_settings.go`:

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
			ALTER TABLE user_settings ADD COLUMN viewer_epub_font_size INTEGER NOT NULL DEFAULT 100
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			ALTER TABLE user_settings ADD COLUMN viewer_epub_theme TEXT NOT NULL DEFAULT 'light'
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			ALTER TABLE user_settings ADD COLUMN viewer_epub_flow TEXT NOT NULL DEFAULT 'paginated'
		`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		for _, col := range []string{"viewer_epub_font_size", "viewer_epub_theme", "viewer_epub_flow"} {
			if _, err := db.Exec("ALTER TABLE user_settings DROP COLUMN " + col); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 1.4: Update the model**

Edit `pkg/models/user_settings.go`. Replace the whole file:

```go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	//tygo:emit export type FitMode = typeof FitModeHeight | typeof FitModeOriginal;
	FitModeHeight   = "fit-height"
	FitModeOriginal = "original"
)

const (
	//tygo:emit export type EpubTheme = typeof EpubThemeLight | typeof EpubThemeDark | typeof EpubThemeSepia;
	EpubThemeLight = "light"
	EpubThemeDark  = "dark"
	EpubThemeSepia = "sepia"
)

const (
	//tygo:emit export type EpubFlow = typeof EpubFlowPaginated | typeof EpubFlowScrolled;
	EpubFlowPaginated = "paginated"
	EpubFlowScrolled  = "scrolled"
)

type UserSettings struct {
	bun.BaseModel `bun:"table:user_settings,alias:us" tstype:"-"`

	ID                 int       `bun:",pk,autoincrement" json:"id"`
	CreatedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
	UserID             int       `bun:",notnull,unique" json:"user_id"`
	ViewerPreloadCount int       `bun:",notnull,default:3" json:"viewer_preload_count"`
	ViewerFitMode      string    `bun:",notnull,default:'fit-height'" json:"viewer_fit_mode" tstype:"FitMode"`
	EpubFontSize       int       `bun:"viewer_epub_font_size,notnull,default:100" json:"viewer_epub_font_size"`
	EpubTheme          string    `bun:"viewer_epub_theme,notnull,default:'light'" json:"viewer_epub_theme" tstype:"EpubTheme"`
	EpubFlow           string    `bun:"viewer_epub_flow,notnull,default:'paginated'" json:"viewer_epub_flow" tstype:"EpubFlow"`
}

// DefaultUserSettings returns a UserSettings with default values.
func DefaultUserSettings() *UserSettings {
	return &UserSettings{
		ViewerPreloadCount: 3,
		ViewerFitMode:      FitModeHeight,
		EpubFontSize:       100,
		EpubTheme:          EpubThemeLight,
		EpubFlow:           EpubFlowPaginated,
	}
}
```

- [ ] **Step 1.5: Update validators**

Replace the top of `pkg/settings/validators.go` (keep the library payloads at the bottom untouched):

```go
package settings

import "github.com/shishobooks/shisho/pkg/models"

// ViewerSettingsPayload is the request body for updating viewer settings.
type ViewerSettingsPayload struct {
	PreloadCount int    `json:"preload_count"`
	FitMode      string `json:"fit_mode"`
	EpubFontSize int    `json:"viewer_epub_font_size"`
	EpubTheme    string `json:"viewer_epub_theme"`
	EpubFlow     string `json:"viewer_epub_flow"`
}

// ViewerSettingsResponse is the response for viewer settings.
type ViewerSettingsResponse struct {
	PreloadCount int    `json:"preload_count"`
	FitMode      string `json:"fit_mode"`
	EpubFontSize int    `json:"viewer_epub_font_size"`
	EpubTheme    string `json:"viewer_epub_theme"`
	EpubFlow     string `json:"viewer_epub_flow"`
}

// ValidFitModes returns all valid fit mode values.
func ValidFitModes() []string {
	return []string{models.FitModeHeight, models.FitModeOriginal}
}

// IsValidFitMode returns true if the fit mode is valid.
func IsValidFitMode(mode string) bool {
	for _, valid := range ValidFitModes() {
		if mode == valid {
			return true
		}
	}
	return false
}

// IsValidEpubTheme returns true if the theme is a supported EPUB theme.
func IsValidEpubTheme(theme string) bool {
	switch theme {
	case models.EpubThemeLight, models.EpubThemeDark, models.EpubThemeSepia:
		return true
	}
	return false
}

// IsValidEpubFlow returns true if the flow is a supported EPUB flow mode.
func IsValidEpubFlow(flow string) bool {
	switch flow {
	case models.EpubFlowPaginated, models.EpubFlowScrolled:
		return true
	}
	return false
}
```

- [ ] **Step 1.6: Update the service**

Replace `UpdateViewerSettings` in `pkg/settings/service.go`:

```go
// UpdateViewerSettings updates viewer settings for a user, creating if not exists.
func (svc *Service) UpdateViewerSettings(
	ctx context.Context,
	userID int,
	preloadCount int,
	fitMode string,
	epubFontSize int,
	epubTheme string,
	epubFlow string,
) (*models.UserSettings, error) {
	now := time.Now()

	settings := &models.UserSettings{
		CreatedAt:          now,
		UpdatedAt:          now,
		UserID:             userID,
		ViewerPreloadCount: preloadCount,
		ViewerFitMode:      fitMode,
		EpubFontSize:       epubFontSize,
		EpubTheme:          epubTheme,
		EpubFlow:           epubFlow,
	}

	_, err := svc.db.NewInsert().
		Model(settings).
		On("CONFLICT (user_id) DO UPDATE").
		Set("updated_at = EXCLUDED.updated_at").
		Set("viewer_preload_count = EXCLUDED.viewer_preload_count").
		Set("viewer_fit_mode = EXCLUDED.viewer_fit_mode").
		Set("viewer_epub_font_size = EXCLUDED.viewer_epub_font_size").
		Set("viewer_epub_theme = EXCLUDED.viewer_epub_theme").
		Set("viewer_epub_flow = EXCLUDED.viewer_epub_flow").
		Returning("*").
		Exec(ctx)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return settings, nil
}
```

- [ ] **Step 1.7: Update the handlers**

Replace the two handler functions in `pkg/settings/handlers.go` (leave imports intact, add no new ones):

```go
func (h *handler) getViewerSettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	settings, err := h.settingsService.GetViewerSettings(ctx, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, ViewerSettingsResponse{
		PreloadCount: settings.ViewerPreloadCount,
		FitMode:      settings.ViewerFitMode,
		EpubFontSize: settings.EpubFontSize,
		EpubTheme:    settings.EpubTheme,
		EpubFlow:     settings.EpubFlow,
	})
}

func (h *handler) updateViewerSettings(c echo.Context) error {
	ctx := c.Request().Context()

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("Authentication required")
	}

	var payload ViewerSettingsPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	if payload.PreloadCount < 1 || payload.PreloadCount > 10 {
		return errcodes.ValidationError("preload_count must be between 1 and 10")
	}

	if !IsValidFitMode(payload.FitMode) {
		return errcodes.ValidationError("fit_mode must be 'fit-height' or 'original'")
	}

	if payload.EpubFontSize < 50 || payload.EpubFontSize > 200 {
		return errcodes.ValidationError("viewer_epub_font_size must be between 50 and 200")
	}

	if !IsValidEpubTheme(payload.EpubTheme) {
		return errcodes.ValidationError("viewer_epub_theme must be 'light', 'dark', or 'sepia'")
	}

	if !IsValidEpubFlow(payload.EpubFlow) {
		return errcodes.ValidationError("viewer_epub_flow must be 'paginated' or 'scrolled'")
	}

	settings, err := h.settingsService.UpdateViewerSettings(
		ctx, user.ID,
		payload.PreloadCount, payload.FitMode,
		payload.EpubFontSize, payload.EpubTheme, payload.EpubFlow,
	)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, ViewerSettingsResponse{
		PreloadCount: settings.ViewerPreloadCount,
		FitMode:      settings.ViewerFitMode,
		EpubFontSize: settings.EpubFontSize,
		EpubTheme:    settings.EpubTheme,
		EpubFlow:     settings.EpubFlow,
	})
}
```

- [ ] **Step 1.8: Write a handler validation test**

Create `pkg/settings/viewer_handlers_test.go` (use the existing `library_handlers_test.go` for the setup pattern — spin up an echo instance, inject a user into the context, POST the payload, assert the response). If `library_handlers_test.go` doesn't use a helper you can reuse, copy its setup verbatim. The goal: test that rejecting an out-of-range `viewer_epub_font_size` returns a 400, and that a valid payload returns 200 with the submitted values echoed back.

Minimum test:

```go
package settings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateViewerSettings_RejectsBadEpubFontSize(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "alice")

	e := echo.New()
	h := &handler{settingsService: NewService(db)}

	body := bytes.NewBufferString(`{
		"preload_count": 3,
		"fit_mode": "fit-height",
		"viewer_epub_font_size": 999,
		"viewer_epub_theme": "light",
		"viewer_epub_flow": "paginated"
	}`)
	req := httptest.NewRequest(http.MethodPut, "/settings/viewer", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	err := h.updateViewerSettings(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "viewer_epub_font_size")
}

func TestUpdateViewerSettings_AcceptsValidEpubPayload(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	user := createTestUser(t, db, "bob")

	e := echo.New()
	h := &handler{settingsService: NewService(db)}

	body := bytes.NewBufferString(`{
		"preload_count": 3,
		"fit_mode": "fit-height",
		"viewer_epub_font_size": 130,
		"viewer_epub_theme": "dark",
		"viewer_epub_flow": "scrolled"
	}`)
	req := httptest.NewRequest(http.MethodPut, "/settings/viewer", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.updateViewerSettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ViewerSettingsResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&resp))
	assert.Equal(t, 130, resp.EpubFontSize)
	assert.Equal(t, models.EpubThemeDark, resp.EpubTheme)
	assert.Equal(t, models.EpubFlowScrolled, resp.EpubFlow)
}
```

If Echo's default binder conflicts with the project's custom binder on `c.Bind`, switch to matching what `library_handlers_test.go` does. Check that file before writing — the setup may need `binder.New()` or similar.

- [ ] **Step 1.9: Run the backend tests and confirm they pass**

Run: `go test ./pkg/settings/ ./pkg/models/ ./pkg/migrations/ -v`
Expected: all pass.

Also verify migration reversibility:

```bash
mise db:migrate && mise db:rollback && mise db:migrate
```

Expected: no errors.

- [ ] **Step 1.10: Commit**

```bash
git add pkg/migrations/20260422000000_add_epub_viewer_settings.go \
        pkg/models/user_settings.go \
        pkg/settings/validators.go \
        pkg/settings/service.go \
        pkg/settings/handlers.go \
        pkg/settings/viewer_service_test.go \
        pkg/settings/viewer_handlers_test.go
git commit -m "[Backend] Add EPUB viewer settings (font size, theme, flow)"
```

---

## Task 2: Update frontend settings types and hook

**Files:**
- Modify: `app/hooks/queries/settings.ts`

**Note:** The Go model has `tstype:"-"` so tygo does NOT regenerate these types. Update by hand.

- [ ] **Step 2.1: Run `mise tygo` to confirm no auto-regeneration is needed**

Run: `mise tygo`
Expected: prints "skipping, outputs are up-to-date" OR regenerates unrelated files. The `UserSettings` struct will NOT appear in `app/types/generated/` because of `tstype:"-"`. The emitted `FitMode`, `EpubTheme`, and `EpubFlow` type aliases from the `//tygo:emit` directives WILL land in the generated output — verify they're present in `app/types/generated/models.ts` via `grep -n "EpubTheme\|EpubFlow" app/types/generated/models.ts`.

If the grep returns no matches, the tygo config doesn't include `pkg/models/` output for these constants. In that case, declare the types inline in the next step instead of importing.

- [ ] **Step 2.2: Replace `app/hooks/queries/settings.ts`**

```typescript
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { EpubFlow, EpubTheme, FitMode } from "@/types";

export interface ViewerSettings {
  preload_count: number;
  fit_mode: FitMode;
  viewer_epub_font_size: number;
  viewer_epub_theme: EpubTheme;
  viewer_epub_flow: EpubFlow;
}

export enum QueryKey {
  ViewerSettings = "ViewerSettings",
}

export const useViewerSettings = (
  options: Omit<
    UseQueryOptions<ViewerSettings, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ViewerSettings, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ViewerSettings],
    queryFn: ({ signal }) => {
      return API.request("GET", "/settings/viewer", null, null, signal);
    },
  });
};

interface UpdateViewerSettingsVariables {
  preload_count: number;
  fit_mode: FitMode;
  viewer_epub_font_size: number;
  viewer_epub_theme: EpubTheme;
  viewer_epub_flow: EpubFlow;
}

export const useUpdateViewerSettings = () => {
  const queryClient = useQueryClient();

  return useMutation<
    ViewerSettings,
    ShishoAPIError,
    UpdateViewerSettingsVariables
  >({
    mutationFn: (payload) => {
      return API.request("PUT", "/settings/viewer", payload, null);
    },
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.ViewerSettings], data);
    },
  });
};
```

If Step 2.1 showed that `EpubFlow` and `EpubTheme` are NOT in `@/types`, replace the import line with inline string-literal unions:

```typescript
export type FitMode = "fit-height" | "original";
export type EpubTheme = "light" | "dark" | "sepia";
export type EpubFlow = "paginated" | "scrolled";
```

- [ ] **Step 2.3: Update existing callers of `useUpdateViewerSettings`**

Search: `grep -rn "updateSettings.mutate\|useUpdateViewerSettings" app/ --include="*.tsx"`

Existing callers in `PageReader.tsx` (lines ~222 and ~238 per the current file) pass only `preload_count` and `fit_mode`. They must also pass the three new EPUB fields — use the current values from `settings` so they don't clobber them:

```tsx
updateSettings.mutate({
  preload_count: value,
  fit_mode: fitMode,
  viewer_epub_font_size: settings?.viewer_epub_font_size ?? 100,
  viewer_epub_theme: settings?.viewer_epub_theme ?? "light",
  viewer_epub_flow: settings?.viewer_epub_flow ?? "paginated",
});
```

Apply to every `updateSettings.mutate` call found in `PageReader.tsx`.

- [ ] **Step 2.4: Type-check and lint**

Run: `mise check:quiet`
Expected: all checks pass.

- [ ] **Step 2.5: Commit**

```bash
git add app/hooks/queries/settings.ts app/components/pages/PageReader.tsx
git commit -m "[Frontend] Extend viewer settings with EPUB fields"
```

---

## Task 3: Vendor foliate-js

**Files:**
- Create: `app/libraries/foliate/` (directory with vendored files)
- Create: `app/libraries/foliate/README.md` (source attribution, pinned commit)
- Modify: `app/types/foliate.d.ts` (JSX type declaration)

foliate-js has no npm release. Vendor it by checking out a pinned commit from its repo. As of writing, the latest default branch commit is stable; pin to whatever is current on the day you do this.

- [ ] **Step 3.1: Clone foliate-js at a pinned commit**

```bash
cd /tmp
git clone https://github.com/johnfactotum/foliate-js foliate-js-src
cd foliate-js-src
git rev-parse HEAD  # note this SHA for the README attribution
```

- [ ] **Step 3.2: Copy the required files**

From the foliate-js repo root, copy these files/directories into `app/libraries/foliate/` of this repo:

```
view.js
paginator.js
fixed-layout.js
overlayer.js
epub.js
epubcfi.js
comic-book.js
text-walker.js
ui/tree.js
ui/menu.js
vendor/zip.js
vendor/fflate.js
```

**Command:**
```bash
mkdir -p app/libraries/foliate/ui app/libraries/foliate/vendor
cp /tmp/foliate-js-src/{view,paginator,fixed-layout,overlayer,epub,epubcfi,comic-book,text-walker}.js app/libraries/foliate/
cp /tmp/foliate-js-src/ui/{tree,menu}.js app/libraries/foliate/ui/
cp /tmp/foliate-js-src/vendor/{zip,fflate}.js app/libraries/foliate/vendor/
```

Verify by opening one of them and confirming the imports resolve to other files in the set. If `view.js` imports from a file not in the list above, copy that too — `grep "^import" app/libraries/foliate/*.js` will show you the full transitive set.

- [ ] **Step 3.3: Write the README attribution**

Create `app/libraries/foliate/README.md`:

```markdown
# foliate-js (vendored)

Vendored copy of [foliate-js](https://github.com/johnfactotum/foliate-js) used by the in-app EPUB reader.

- **Source:** https://github.com/johnfactotum/foliate-js
- **License:** MIT
- **Commit:** <INSERT THE SHA FROM STEP 3.1>

foliate-js is distributed as plain ES modules with no build step and no npm release. To update:

1. `git clone https://github.com/johnfactotum/foliate-js /tmp/foliate-js-src`
2. Record the new commit SHA
3. Replace files in this directory with the upstream versions (see `CONTRIBUTING` for the file list, or re-run Task 3 of `docs/superpowers/plans/2026-04-22-epub-reader.md`)
4. Test with several real EPUBs — foliate-js's API occasionally shifts
```

- [ ] **Step 3.4: Declare the custom element for TypeScript**

Create `app/types/foliate.d.ts`:

```typescript
// Type declarations for foliate-js's <foliate-view> custom element.
// See app/libraries/foliate/view.js for the runtime definition.

import "react";

interface FoliateViewElement extends HTMLElement {
  open(file: Blob | File): Promise<void>;
  goLeft(): void;
  goRight(): void;
  goTo(target: string): Promise<void>;
  goToFraction(fraction: number): Promise<void>;
  renderer: {
    setStyles(styles: Record<string, string>): void;
    setAttribute(name: string, value: string): void;
  };
  book?: {
    toc?: Array<{ label: string; href: string; subitems?: unknown[] }>;
  };
}

declare global {
  interface HTMLElementTagNameMap {
    "foliate-view": FoliateViewElement;
  }

  namespace JSX {
    interface IntrinsicElements {
      "foliate-view": React.DetailedHTMLProps<
        React.HTMLAttributes<FoliateViewElement>,
        FoliateViewElement
      >;
    }
  }
}
```

**If the real foliate-view API differs from the types above** (verify against `app/libraries/foliate/view.js` — look for the `class View extends HTMLElement` block and its public methods/properties), adjust the declaration to match. Do not guess; read the source.

- [ ] **Step 3.5: Verify the app still builds**

Run: `pnpm lint:types && pnpm build`
Expected: both succeed. The vendored JS files are not yet imported by the app; this step just confirms we haven't broken anything.

- [ ] **Step 3.6: Commit**

```bash
git add app/libraries/foliate/ app/types/foliate.d.ts
git commit -m "[Frontend] Vendor foliate-js for EPUB rendering"
```

---

## Task 4: Add `useEpubBlob` hook

**Files:**
- Create: `app/hooks/queries/epub.ts`
- Create: `app/hooks/queries/epub.test.ts`

The hook fetches the regenerated EPUB from `/api/books/files/:id/download` as a Blob and caches it per-file via React Query.

- [ ] **Step 4.1: Write the failing test**

Create `app/hooks/queries/epub.test.ts`:

```typescript
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useEpubBlob } from "./epub";

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

describe("useEpubBlob", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  it("fetches the EPUB from the download endpoint and resolves to a Blob", async () => {
    const blob = new Blob(["epub-bytes"], { type: "application/epub+zip" });
    fetchSpy.mockResolvedValue(
      new Response(blob, { status: 200, headers: { "Content-Type": "application/epub+zip" } }),
    );

    const { result } = renderHook(() => useEpubBlob(42), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/books/files/42/download",
      expect.objectContaining({ credentials: "include" }),
    );
    expect(result.current.data).toBeInstanceOf(Blob);
  });

  it("surfaces fetch errors", async () => {
    fetchSpy.mockResolvedValue(new Response("nope", { status: 500 }));

    const { result } = renderHook(() => useEpubBlob(42), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});
```

- [ ] **Step 4.2: Run the test to confirm it fails**

Run: `pnpm vitest run app/hooks/queries/epub.test.ts`
Expected: FAIL — module `./epub` not found.

- [ ] **Step 4.3: Implement the hook**

Create `app/hooks/queries/epub.ts`:

```typescript
import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { ShishoAPIError } from "@/libraries/api";

export enum QueryKey {
  EpubBlob = "EpubBlob",
}

export const useEpubBlob = (
  fileId: number,
  options: Omit<
    UseQueryOptions<Blob, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Blob, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.EpubBlob, fileId],
    staleTime: 5 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    queryFn: async ({ signal }) => {
      const response = await fetch(`/api/books/files/${fileId}/download`, {
        credentials: "include",
        signal,
      });
      if (!response.ok) {
        throw new ShishoAPIError(
          `Failed to fetch EPUB: ${response.status} ${response.statusText}`,
          response.status,
        );
      }
      return response.blob();
    },
  });
};
```

If `ShishoAPIError`'s constructor signature differs from `(message, status)`, read `app/libraries/api.ts` and match it. Do not guess.

- [ ] **Step 4.4: Run the test and confirm it passes**

Run: `pnpm vitest run app/hooks/queries/epub.test.ts`
Expected: PASS.

- [ ] **Step 4.5: Commit**

```bash
git add app/hooks/queries/epub.ts app/hooks/queries/epub.test.ts
git commit -m "[Frontend] Add useEpubBlob hook"
```

---

## Task 5: `EPUBReader` — component skeleton, loading and error states

**Files:**
- Create: `app/components/pages/EPUBReader.tsx`
- Create: `app/components/pages/EPUBReader.test.tsx`

This task lands the outer shell, fetch wiring, loading indicator (with 10-second extended-wait hint), error state, and retry button. The foliate rendering integration is the next task.

- [ ] **Step 5.1: Write failing component tests**

Create `app/components/pages/EPUBReader.test.tsx`:

```typescript
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import EPUBReader from "./EPUBReader";

vi.mock("@/hooks/queries/epub", () => ({
  useEpubBlob: vi.fn(),
}));

vi.mock("@/hooks/queries/settings", () => ({
  useViewerSettings: vi.fn(() => ({ data: undefined, isLoading: true })),
  useUpdateViewerSettings: vi.fn(() => ({ mutate: vi.fn() })),
}));

import { useEpubBlob } from "@/hooks/queries/epub";
import {
  useUpdateViewerSettings,
  useViewerSettings,
} from "@/hooks/queries/settings";

const renderReader = () => {
  const client = new QueryClient();
  const file = {
    id: 7,
    book_id: 3,
    file_type: "epub",
  } as never;
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <EPUBReader file={file} libraryId="1" bookTitle="Test Book" />
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("EPUBReader", () => {
  it("shows a loading indicator while fetching the EPUB", () => {
    vi.mocked(useEpubBlob).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    expect(screen.getByText(/preparing book/i)).toBeInTheDocument();
  });

  it("shows an error state with a retry button on fetch failure", () => {
    const refetch = vi.fn();
    vi.mocked(useEpubBlob).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error("boom"),
      refetch,
    } as never);

    renderReader();
    expect(screen.getByText(/couldn't load/i)).toBeInTheDocument();
    screen.getByRole("button", { name: /retry/i }).click();
    expect(refetch).toHaveBeenCalled();
  });

  it("shows the extended-wait hint after 10 seconds of loading", () => {
    vi.useFakeTimers();
    vi.mocked(useEpubBlob).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
      refetch: vi.fn(),
    } as never);

    renderReader();
    expect(screen.queryByText(/may take a moment/i)).not.toBeInTheDocument();

    vi.advanceTimersByTime(10_000);
    expect(screen.getByText(/may take a moment/i)).toBeInTheDocument();

    vi.useRealTimers();
  });
});
```

- [ ] **Step 5.2: Run the test to confirm it fails**

Run: `pnpm vitest run app/components/pages/EPUBReader.test.tsx`
Expected: FAIL — `EPUBReader` module not found.

- [ ] **Step 5.3: Implement the skeleton component**

Create `app/components/pages/EPUBReader.tsx`:

```tsx
import { AlertCircle, ArrowLeft, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { useEpubBlob } from "@/hooks/queries/epub";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { File } from "@/types";

interface EPUBReaderProps {
  file: File;
  libraryId: string;
  bookTitle?: string;
}

export default function EPUBReader({
  file,
  libraryId,
  bookTitle,
}: EPUBReaderProps) {
  usePageTitle(bookTitle ? `Reading: ${bookTitle}` : "Reader");

  const { data: blob, isLoading, isError, error, refetch } = useEpubBlob(
    file.id,
  );

  const [showExtendedHint, setShowExtendedHint] = useState(false);
  useEffect(() => {
    if (!isLoading) {
      setShowExtendedHint(false);
      return;
    }
    const timer = setTimeout(() => setShowExtendedHint(true), 10_000);
    return () => clearTimeout(timer);
  }, [isLoading]);

  const backHref = `/libraries/${libraryId}/books/${file.book_id}`;

  if (isError) {
    return (
      <div className="fixed inset-0 bg-background flex flex-col items-center justify-center gap-4 p-4 text-center">
        <AlertCircle className="h-8 w-8 text-destructive" />
        <div>
          <p className="font-medium">We couldn't load this book.</p>
          <p className="text-sm text-muted-foreground mt-1">
            {error?.message ?? "Unknown error"}
          </p>
        </div>
        <div className="flex gap-2">
          <Button onClick={() => refetch()} variant="default">
            Retry
          </Button>
          <Button asChild variant="outline">
            <Link to={backHref}>Back</Link>
          </Button>
        </div>
      </div>
    );
  }

  if (isLoading || !blob) {
    return (
      <div className="fixed inset-0 bg-background flex flex-col items-center justify-center gap-3">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        <p className="text-sm text-muted-foreground">Preparing book…</p>
        {showExtendedHint && (
          <p className="text-xs text-muted-foreground">
            This may take a moment for large books.
          </p>
        )}
      </div>
    );
  }

  // Reader chrome rendered in Task 6.
  return (
    <div className="fixed inset-0 bg-background flex flex-col">
      <header className="flex items-center justify-between px-4 py-2 border-b">
        <Link
          className="flex items-center gap-2 text-muted-foreground hover:text-foreground"
          to={backHref}
        >
          <ArrowLeft className="h-4 w-4" />
          <span className="text-sm">Back</span>
        </Link>
      </header>
      <main className="flex-1 bg-background" />
      <footer className="border-t px-4 py-2 text-xs text-muted-foreground">
        Loaded {(blob.size / 1024 / 1024).toFixed(1)} MB
      </footer>
    </div>
  );
}
```

- [ ] **Step 5.4: Run the test and confirm it passes**

Run: `pnpm vitest run app/components/pages/EPUBReader.test.tsx`
Expected: PASS.

- [ ] **Step 5.5: Commit**

```bash
git add app/components/pages/EPUBReader.tsx app/components/pages/EPUBReader.test.tsx
git commit -m "[Frontend] Add EPUBReader skeleton with loading and error states"
```

---

## Task 6: `EPUBReader` — foliate rendering, TOC, navigation, progress

**Files:**
- Modify: `app/components/pages/EPUBReader.tsx`

This task wires up the actual reader: mount `<foliate-view>`, load the Blob, handle the `relocate` event, build the TOC dropdown from `view.book.toc`, drive the progress bar from `fraction`, wire keyboard + tap zones.

- [ ] **Step 6.1: Verify the foliate-view API surface against the vendored source**

Before writing integration code, read `app/libraries/foliate/view.js` and confirm:

1. The custom element tag name (should be `foliate-view`).
2. The method to open a book (typically `view.open(file)` where `file` is a Blob/File).
3. The event name for page changes (typically `relocate`, a `CustomEvent` with `detail: { fraction, tocItem, cfi }`).
4. The navigation methods (typically `goLeft()`, `goRight()`, `goToFraction(n)`, `goTo(href)`).
5. The TOC accessor (typically `view.book.toc` — an array of `{ label, href, subitems? }`).

If any of these differ in your pinned commit, update the component code in the steps below to match. Do not guess.

- [ ] **Step 6.2: Replace `EPUBReader.tsx` with the full implementation**

Overwrite `app/components/pages/EPUBReader.tsx`:

```tsx
import { AlertCircle, ArrowLeft, ChevronLeft, ChevronRight, Loader2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { useEpubBlob } from "@/hooks/queries/epub";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { File } from "@/types";

import "@/libraries/foliate/view.js";

interface EPUBReaderProps {
  file: File;
  libraryId: string;
  bookTitle?: string;
}

interface TocEntry {
  label: string;
  href: string;
}

interface RelocateDetail {
  fraction: number;
  tocItem?: { label?: string; href?: string } | null;
  cfi?: string;
}

const flattenToc = (
  nodes: Array<{ label: string; href: string; subitems?: unknown[] }> | undefined,
): TocEntry[] => {
  if (!nodes) return [];
  const out: TocEntry[] = [];
  for (const n of nodes) {
    if (n.href) out.push({ label: n.label, href: n.href });
    if (Array.isArray(n.subitems)) {
      out.push(...flattenToc(n.subitems as typeof nodes));
    }
  }
  return out;
};

export default function EPUBReader({
  file,
  libraryId,
  bookTitle,
}: EPUBReaderProps) {
  usePageTitle(bookTitle ? `Reading: ${bookTitle}` : "Reader");

  const { data: blob, isLoading, isError, error, refetch } = useEpubBlob(
    file.id,
  );

  const [showExtendedHint, setShowExtendedHint] = useState(false);
  useEffect(() => {
    if (!isLoading) {
      setShowExtendedHint(false);
      return;
    }
    const timer = setTimeout(() => setShowExtendedHint(true), 10_000);
    return () => clearTimeout(timer);
  }, [isLoading]);

  const viewRef = useRef<HTMLElement | null>(null);
  const [toc, setToc] = useState<TocEntry[]>([]);
  const [fraction, setFraction] = useState(0);
  const [currentTocHref, setCurrentTocHref] = useState<string | null>(null);
  const [currentTocLabel, setCurrentTocLabel] = useState<string | null>(null);
  const [bookReady, setBookReady] = useState(false);

  // Load the blob into foliate once both are available.
  useEffect(() => {
    if (!blob) return;
    const view = viewRef.current;
    if (!view) return;

    let cancelled = false;
    setBookReady(false);

    const bookFile = new File([blob], `${bookTitle ?? "book"}.epub`, {
      type: "application/epub+zip",
    });

    (async () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      await (view as any).open(bookFile);
      if (cancelled) return;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const book = (view as any).book;
      setToc(flattenToc(book?.toc));
      setBookReady(true);
    })().catch(() => {
      // Surfaced via the main error state if it fails; foliate typically rejects with a descriptive Error.
    });

    return () => {
      cancelled = true;
    };
  }, [blob, bookTitle]);

  // Wire the relocate event for progress tracking.
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;

    const handleRelocate = (evt: Event) => {
      const detail = (evt as CustomEvent<RelocateDetail>).detail;
      if (typeof detail.fraction === "number") setFraction(detail.fraction);
      setCurrentTocHref(detail.tocItem?.href ?? null);
      setCurrentTocLabel(detail.tocItem?.label ?? null);
    };

    view.addEventListener("relocate", handleRelocate);
    return () => view.removeEventListener("relocate", handleRelocate);
  }, [bookReady]);

  const goPrev = useCallback(() => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (viewRef.current as any)?.goLeft?.();
  }, []);
  const goNext = useCallback(() => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (viewRef.current as any)?.goRight?.();
  }, []);

  // Keyboard navigation.
  useEffect(() => {
    if (!bookReady) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "ArrowRight" || e.key === "d" || e.key === "D") goNext();
      else if (e.key === "ArrowLeft" || e.key === "a" || e.key === "A") goPrev();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [bookReady, goNext, goPrev]);

  const backHref = `/libraries/${libraryId}/books/${file.book_id}`;

  const handleTocChange = (href: string) => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (viewRef.current as any)?.goTo?.(href);
  };

  const handleProgressClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const target = (e.clientX - rect.left) / rect.width;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (viewRef.current as any)?.goToFraction?.(Math.max(0, Math.min(1, target)));
  };

  const progressPercent = useMemo(() => Math.round(fraction * 100), [fraction]);

  if (isError) {
    return (
      <div className="fixed inset-0 bg-background flex flex-col items-center justify-center gap-4 p-4 text-center">
        <AlertCircle className="h-8 w-8 text-destructive" />
        <div>
          <p className="font-medium">We couldn't load this book.</p>
          <p className="text-sm text-muted-foreground mt-1">
            {error?.message ?? "Unknown error"}
          </p>
        </div>
        <div className="flex gap-2">
          <Button onClick={() => refetch()} variant="default">
            Retry
          </Button>
          <Button asChild variant="outline">
            <Link to={backHref}>Back</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="fixed inset-0 bg-background flex flex-col">
      <header className="flex items-center justify-between px-4 py-2 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <Link
          className="flex items-center gap-2 text-muted-foreground hover:text-foreground"
          to={backHref}
        >
          <ArrowLeft className="h-4 w-4" />
          <span className="text-sm">Back</span>
        </Link>

        <div className="flex items-center gap-2">
          {toc.length > 0 && (
            <select
              className="text-sm bg-transparent border rounded px-2 py-1"
              onChange={(e) => handleTocChange(e.target.value)}
              value={currentTocHref ?? ""}
            >
              {currentTocHref === null && <option value="">—</option>}
              {toc.map((entry) => (
                <option key={entry.href} value={entry.href}>
                  {entry.label}
                </option>
              ))}
            </select>
          )}
          {/* Settings popover added in Task 7 */}
        </div>
      </header>

      <main className="flex-1 relative bg-background">
        {(isLoading || !blob || !bookReady) && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-background z-20">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            <p className="text-sm text-muted-foreground">Preparing book…</p>
            {showExtendedHint && (
              <p className="text-xs text-muted-foreground">
                This may take a moment for large books.
              </p>
            )}
          </div>
        )}

        <button
          aria-label="Previous page"
          className="absolute left-0 top-0 w-1/3 h-full z-10 cursor-pointer opacity-0"
          onClick={goPrev}
          type="button"
        />
        <button
          aria-label="Next page"
          className="absolute right-0 top-0 w-1/3 h-full z-10 cursor-pointer opacity-0"
          onClick={goNext}
          type="button"
        />

        <foliate-view
          ref={(el) => {
            viewRef.current = el;
          }}
          style={{ display: "block", width: "100%", height: "100%" }}
        />
      </main>

      <footer className="border-t bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="px-4 pt-3">
          <div
            className="relative h-1.5 bg-muted rounded-full cursor-pointer"
            onClick={handleProgressClick}
          >
            <div
              className="absolute inset-y-0 left-0 bg-primary rounded-full"
              style={{ width: `${progressPercent}%` }}
            />
          </div>
          {currentTocLabel && (
            <div className="text-xs text-muted-foreground mt-1">
              {currentTocLabel}
            </div>
          )}
        </div>

        <div className="flex items-center justify-between px-4 py-2">
          <Button onClick={goPrev} size="icon" variant="ghost">
            <ChevronLeft className="h-5 w-5" />
          </Button>
          <span className="text-sm text-muted-foreground">
            {progressPercent}%
          </span>
          <Button onClick={goNext} size="icon" variant="ghost">
            <ChevronRight className="h-5 w-5" />
          </Button>
        </div>
      </footer>
    </div>
  );
}
```

- [ ] **Step 6.3: Update the component test to tolerate the new structure**

The existing tests from Task 5 still pass in concept, but the rendered tree is now more complex. If Vitest complains that `foliate-view` is an unknown element, add a mock in the test file:

```typescript
beforeAll(() => {
  if (!customElements.get("foliate-view")) {
    customElements.define(
      "foliate-view",
      class extends HTMLElement {
        open = vi.fn().mockResolvedValue(undefined);
        goLeft = vi.fn();
        goRight = vi.fn();
        goTo = vi.fn();
        goToFraction = vi.fn();
        book = { toc: [] };
      },
    );
  }
});
```

And mock the foliate import so jsdom doesn't try to execute `view.js`:

```typescript
vi.mock("@/libraries/foliate/view.js", () => ({}));
```

Place both at the top of `EPUBReader.test.tsx` (the `vi.mock` hoists automatically).

- [ ] **Step 6.4: Run tests**

Run: `pnpm vitest run app/components/pages/EPUBReader.test.tsx`
Expected: PASS.

- [ ] **Step 6.5: Manual browser verification**

```bash
mise start
```

Navigate to a book that has an EPUB file, open the file, and click "Read" (or however the existing UI enters the reader — `/libraries/:libraryId/books/:bookId/files/:fileId/read`). Since FileReader has not been wired yet (that's Task 8), use the browser URL bar directly.

Confirm:
- Loading spinner appears, then disappears when the book loads.
- Content renders inside the main area.
- Arrow keys and click zones turn pages.
- Progress bar updates as you page.
- TOC dropdown navigates to chapters.

If foliate's rendering isn't visible, open devtools console and look for errors. Common issues:
- Missing files in the vendor set — foliate will throw `Failed to resolve module specifier`. Copy the missing file from `/tmp/foliate-js-src/`.
- CSP blocking the ZIP worker — check `pkg/server/` for any `Content-Security-Policy` header and relax `script-src`/`worker-src` if needed. Document any change.

- [ ] **Step 6.6: Commit**

```bash
git add app/components/pages/EPUBReader.tsx app/components/pages/EPUBReader.test.tsx
git commit -m "[Frontend] Wire foliate rendering, TOC, and progress into EPUBReader"
```

---

## Task 7: `EPUBReader` — settings popover (font size, theme, flow)

**Files:**
- Modify: `app/components/pages/EPUBReader.tsx`

Add the settings popover to the header, matching the visual pattern of `PageReader.tsx`. Apply settings to foliate live via `renderer.setStyles` and the flow property.

- [ ] **Step 7.1: Extend the component with settings wiring**

Inside `EPUBReader.tsx`, add imports:

```tsx
import { Settings } from "lucide-react";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Slider } from "@/components/ui/slider";
import {
  useUpdateViewerSettings,
  useViewerSettings,
} from "@/hooks/queries/settings";
```

Add these hooks inside the component body (near the top, under the existing `useEpubBlob` call):

```tsx
const { data: settings, isLoading: settingsLoading } = useViewerSettings();
const updateSettings = useUpdateViewerSettings();
const settingsReady = !settingsLoading && settings != null;

const fontSize = settings?.viewer_epub_font_size ?? 100;
const theme = settings?.viewer_epub_theme ?? "light";
const flow = settings?.viewer_epub_flow ?? "paginated";

const commitSettings = useCallback(
  (partial: Partial<{
    preload_count: number;
    fit_mode: "fit-height" | "original";
    viewer_epub_font_size: number;
    viewer_epub_theme: "light" | "dark" | "sepia";
    viewer_epub_flow: "paginated" | "scrolled";
  }>) => {
    if (!settings) return;
    updateSettings.mutate({
      preload_count: settings.preload_count,
      fit_mode: settings.fit_mode,
      viewer_epub_font_size: settings.viewer_epub_font_size,
      viewer_epub_theme: settings.viewer_epub_theme,
      viewer_epub_flow: settings.viewer_epub_flow,
      ...partial,
    });
  },
  [settings, updateSettings],
);
```

- [ ] **Step 7.2: Apply settings to the foliate renderer**

Add an effect that pushes the current settings into the foliate view whenever they change and the book is ready:

```tsx
useEffect(() => {
  const view = viewRef.current;
  if (!view || !bookReady) return;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const renderer = (view as any).renderer;
  if (!renderer) return;

  renderer.setStyles?.({
    fontSize: `${fontSize}%`,
    // foliate applies colors via CSS variables; these names match upstream defaults.
    "--foliate-color-bg": theme === "dark" ? "#1a1a1a" : theme === "sepia" ? "#f4ecd8" : "#ffffff",
    "--foliate-color-fg": theme === "dark" ? "#e8e8e8" : theme === "sepia" ? "#5b4636" : "#111111",
  });
  renderer.setAttribute?.("flow", flow);
}, [bookReady, fontSize, theme, flow]);
```

**Verify against the vendored source** before relying on the CSS variable names above. `app/libraries/foliate/view.js` and `paginator.js` define how styles flow through. The foliate reader demo uses `renderer.setStyles({ ... })` with style-sheet-shaped input; adjust to whatever your pinned commit expects. If live theme switching proves flaky, fall back to passing a full CSS string via `renderer.setStyles(cssString)`.

- [ ] **Step 7.3: Render the popover in the header**

Replace the `{/* Settings popover added in Task 7 */}` comment with:

```tsx
<Popover>
  <PopoverTrigger asChild>
    <Button size="icon" variant="ghost">
      <Settings className="h-4 w-4" />
    </Button>
  </PopoverTrigger>
  <PopoverContent align="end" className="w-64">
    <div className="space-y-4">
      <div>
        <label className="text-sm font-medium">
          Font size: {fontSize}%
        </label>
        <Slider
          className="mt-2"
          disabled={!settingsReady}
          max={200}
          min={50}
          onValueChange={([value]) =>
            commitSettings({ viewer_epub_font_size: value })
          }
          step={10}
          value={[fontSize]}
        />
      </div>
      <div>
        <label className="text-sm font-medium">Theme</label>
        <div className="flex gap-2 mt-2">
          {(["light", "dark", "sepia"] as const).map((t) => (
            <Button
              disabled={!settingsReady}
              key={t}
              onClick={() => commitSettings({ viewer_epub_theme: t })}
              size="sm"
              variant={theme === t ? "default" : "outline"}
            >
              {t.charAt(0).toUpperCase() + t.slice(1)}
            </Button>
          ))}
        </div>
      </div>
      <div>
        <label className="text-sm font-medium">Flow</label>
        <div className="flex gap-2 mt-2">
          {(["paginated", "scrolled"] as const).map((f) => (
            <Button
              disabled={!settingsReady}
              key={f}
              onClick={() => commitSettings({ viewer_epub_flow: f })}
              size="sm"
              variant={flow === f ? "default" : "outline"}
            >
              {f.charAt(0).toUpperCase() + f.slice(1)}
            </Button>
          ))}
        </div>
      </div>
    </div>
  </PopoverContent>
</Popover>
```

- [ ] **Step 7.4: Add a settings test**

Append to `EPUBReader.test.tsx`:

```typescript
import userEvent from "@testing-library/user-event";

// ... other imports and mocks ...

it("updates settings when the theme button is clicked", async () => {
  const mutate = vi.fn();
  vi.mocked(useViewerSettings).mockReturnValue({
    data: {
      preload_count: 3,
      fit_mode: "fit-height",
      viewer_epub_font_size: 100,
      viewer_epub_theme: "light",
      viewer_epub_flow: "paginated",
    },
    isLoading: false,
  } as never);
  vi.mocked(useUpdateViewerSettings).mockReturnValue({ mutate } as never);

  vi.mocked(useEpubBlob).mockReturnValue({
    data: new Blob(["x"], { type: "application/epub+zip" }),
    isLoading: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  } as never);

  renderReader();
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  await user.click(await screen.findByRole("button", { name: /settings/i }));
  await user.click(screen.getByRole("button", { name: /dark/i }));
  expect(mutate).toHaveBeenCalledWith(
    expect.objectContaining({ viewer_epub_theme: "dark" }),
  );
});
```

- [ ] **Step 7.5: Run tests**

Run: `pnpm vitest run app/components/pages/EPUBReader.test.tsx`
Expected: PASS.

- [ ] **Step 7.6: Manual browser verification**

Reload the reader in the browser. Confirm:
- Font size slider moves text visibly.
- Theme buttons change background/foreground colors.
- Flow toggle switches between paginated and scrolled layout.
- Settings persist across reader reopen.

If CSS-variable-based theming doesn't visibly change, consult `app/libraries/foliate/` for how themes are actually applied in the vendored commit and adjust the effect in Step 7.2.

- [ ] **Step 7.7: Commit**

```bash
git add app/components/pages/EPUBReader.tsx app/components/pages/EPUBReader.test.tsx
git commit -m "[Frontend] Add settings popover to EPUBReader"
```

---

## Task 8: Wire `EPUBReader` into `FileReader`

**Files:**
- Modify: `app/components/pages/FileReader.tsx`

- [ ] **Step 8.1: Add the EPUB case**

Replace `app/components/pages/FileReader.tsx`:

```tsx
import { useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import CBZReader from "@/components/pages/CBZReader";
import EPUBReader from "@/components/pages/EPUBReader";
import PDFReader from "@/components/pages/PDFReader";
import { useBook } from "@/hooks/queries/books";
import { FileTypeCBZ, FileTypeEPUB, FileTypePDF } from "@/types";

export default function FileReader() {
  const { libraryId, bookId, fileId } = useParams<{
    libraryId: string;
    bookId: string;
    fileId: string;
  }>();

  const { data: book, isLoading } = useBook(bookId);
  const file = book?.files?.find((f) => f.id === Number(fileId));

  if (isLoading || !file) {
    return (
      <div className="fixed inset-0 bg-background flex items-center justify-center">
        <LoadingSpinner />
      </div>
    );
  }

  switch (file.file_type) {
    case FileTypeCBZ:
      return (
        <CBZReader bookTitle={book?.title} file={file} libraryId={libraryId!} />
      );
    case FileTypePDF:
      return (
        <PDFReader bookTitle={book?.title} file={file} libraryId={libraryId!} />
      );
    case FileTypeEPUB:
      return (
        <EPUBReader bookTitle={book?.title} file={file} libraryId={libraryId!} />
      );
    default:
      return (
        <div className="fixed inset-0 bg-background flex items-center justify-center">
          <p className="text-muted-foreground">
            Reading is not supported for this file type.
          </p>
        </div>
      );
  }
}
```

- [ ] **Step 8.2: Confirm the "Read" entry point exists for EPUB files**

Run: `grep -rn "/read\"\|/read'\|to={.*read" app/components/ --include="*.tsx" | head`

Find the component that renders the "Read" link/button (likely in `BookDetail.tsx` or a file row). Check that the button is shown for EPUB files, not just CBZ/PDF. If it's gated by a type check like `canRead(fileType)` that excludes EPUB, add `FileTypeEPUB` to the allowed list.

If the button is universally shown, skip this step.

- [ ] **Step 8.3: Manual end-to-end verification in the browser**

```bash
mise start
```

From a book detail page, click the "Read" action on an EPUB file. Expect:
- Preparing-book spinner appears.
- Book loads; TOC is populated.
- Arrow keys page through.
- Settings popover works.
- Back button returns to book detail.

Also verify fallback still works:
- Click "Read" on an M4B file — expect the "Reading is not supported" placeholder (unchanged behavior).

- [ ] **Step 8.4: Run full check**

Run: `mise check:quiet`
Expected: all green.

- [ ] **Step 8.5: Commit**

```bash
git add app/components/pages/FileReader.tsx
git commit -m "[Frontend] Dispatch EPUB files to EPUBReader"
```

---

## Task 9: Documentation

**Files:**
- Modify: `website/docs/supported-formats.md`

- [ ] **Step 9.1: Update the EPUB line**

In `website/docs/supported-formats.md`, replace the EPUB bullet:

```markdown
- **EPUB** — Full [metadata extraction](./metadata#epub) including title, authors, series, description, cover art, language, and more. Includes an in-app reader with font size, theme, and flow controls.
```

Do not edit anything under `website/versioned_docs/` (per `website/CLAUDE.md`).

- [ ] **Step 9.2: Run the docs site to verify the change renders**

Run: `mise docs` (or `cd website && pnpm start`)
Expected: page builds, EPUB bullet reflects new copy.

- [ ] **Step 9.3: Commit**

```bash
git add website/docs/supported-formats.md
git commit -m "[Docs] Note EPUB in-app reader on supported formats page"
```

---

## Task 10: Final verification

- [ ] **Step 10.1: Run all checks**

Run: `mise check:quiet`
Expected: all green. If anything fails, fix it before proceeding.

- [ ] **Step 10.2: Exercise the reader with three real EPUBs**

Pick three EPUB files from `tmp/library/`:
- A small, simple EPUB (< 1 MB).
- A larger EPUB with images (5+ MB).
- An EPUB with a custom cover uploaded through the Shisho UI (trigger a cover upload if none exists).

For each:
- Reader opens without errors.
- Cover on the first "page" matches what's shown in the book detail (proving regeneration applied the custom cover).
- Font size, theme, and flow controls apply live.
- Settings persist across navigation away and back.
- TOC navigates correctly.
- Keyboard and tap zones work.

- [ ] **Step 10.3: Confirm KePub preference isn't being honored for the reader**

In a library set to `download_format_preference: kepub`, open an EPUB in the reader. The reader must still work — it should fetch a plain EPUB, not a KePub file. (The `/download` endpoint for EPUB files returns EPUB regardless of library preference; this is verifying that assumption in practice.)

- [ ] **Step 10.4: Verify first-load latency UX**

With browser devtools → Network → "Slow 3G" throttling, open a large EPUB. Within 10 seconds, the "This may take a moment for large books" hint should appear. Once loaded, the reader should work normally.

- [ ] **Step 10.5: (If everything passes) No final commit needed — you've been committing per task.**

Check `git log --oneline | head -12` and confirm the task commits are present and readable.

---

## Things explicitly NOT in this plan

As per the spec, these are follow-up features, not part of this implementation:

- Reading progress persistence (saving CFI per user/file)
- Annotations, highlights, bookmarks
- Text search within the book
- Custom font files or additional themes
- Structural editing of EPUBs from the reader
- MOBI / KF8 / FB2 support (foliate-js supports them, but the file types aren't recognized by the scanner yet)
- Consolidating CBZ / PDF into foliate-js

If you notice a reasonable improvement that isn't in this plan, add it to the Notion board (per user's global CLAUDE.md) rather than implementing it inline.
