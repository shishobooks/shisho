# Library Plugin Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace binary enabled/disabled toggle on plugin order with a three-state mode (Enabled / Manual Only / Disabled), applying at both global and per-library levels.

**Architecture:** Database migration renames `plugin_order` → `plugin_hook_config` and `library_plugins` → `library_plugin_hook_config`, adds a `mode` column to both tables. Go models, service, manager, and handlers are updated. A new `GetManualRuntimes` method returns plugins with mode `enabled` or `manual_only`. Frontend replaces toggle switches with dropdowns.

**Tech Stack:** Go (Bun ORM, Echo), SQLite, React (TypeScript), Tanstack Query, shadcn/ui

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `pkg/models/plugin.go` | Rename structs, add `Mode` field, add mode constants |
| Create | `pkg/migrations/20260405000000_plugin_hook_config_mode.go` | Migration: rename tables, add mode column, migrate data |
| Modify | `pkg/plugins/service.go` | Update all queries to use renamed models |
| Modify | `pkg/plugins/service_test.go` | Update tests for renamed models + mode field |
| Modify | `pkg/plugins/manager.go` | Filter by mode in `GetOrderedRuntimes`, add `GetManualRuntimes` |
| Modify | `pkg/plugins/manager_test.go` | Update existing tests, add mode-aware tests |
| Modify | `pkg/plugins/handler.go` | Update payload types, handler logic, manual search |
| Modify | `app/hooks/queries/plugins.ts` | Replace `enabled` with `mode` in types and mutations |
| Modify | `app/components/library/LibraryPluginsTab.tsx` | Replace toggle with mode dropdown |
| Modify | `app/components/pages/AdminPlugins.tsx` | Add mode dropdown to global order |
| Modify | `pkg/plugins/CLAUDE.md` | Update table names and field references |

---

### Task 1: Database Migration

**Files:**
- Create: `pkg/migrations/20260405000000_plugin_hook_config_mode.go`

- [ ] **Step 1: Write the migration file**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Rename plugin_order → plugin_hook_config and add mode column
		_, err := db.Exec(`ALTER TABLE plugin_order RENAME TO plugin_hook_config`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE plugin_hook_config ADD COLUMN mode TEXT NOT NULL DEFAULT 'enabled'`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Rename library_plugins → library_plugin_hook_config, add mode, migrate, drop enabled
		_, err = db.Exec(`ALTER TABLE library_plugins RENAME TO library_plugin_hook_config`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config ADD COLUMN mode TEXT NOT NULL DEFAULT 'enabled'`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`UPDATE library_plugin_hook_config SET mode = CASE WHEN enabled THEN 'enabled' ELSE 'disabled' END`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config DROP COLUMN enabled`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Rename the index
		_, err = db.Exec(`DROP INDEX IF EXISTS idx_library_plugins_order`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX idx_library_plugin_hook_config_order ON library_plugin_hook_config(library_id, hook_type, position)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		// Reverse index rename
		_, err := db.Exec(`DROP INDEX IF EXISTS idx_library_plugin_hook_config_order`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Reverse library table: add enabled back, migrate, drop mode, rename
		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT true`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`UPDATE library_plugin_hook_config SET enabled = (mode = 'enabled')`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config DROP COLUMN mode`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config RENAME TO library_plugins`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX idx_library_plugins_order ON library_plugins(library_id, hook_type, position)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Reverse global table: drop mode, rename
		_, err = db.Exec(`ALTER TABLE plugin_hook_config DROP COLUMN mode`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE plugin_hook_config RENAME TO plugin_order`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 2: Run the migration to verify it works**

Run: `cd /Users/robinjoseph/.worktrees/shisho/library-plugin && mise db:migrate`
Expected: Migration applies successfully

- [ ] **Step 3: Verify rollback works**

Run: `mise db:rollback && mise db:migrate`
Expected: Rollback and re-migrate succeed

- [ ] **Step 4: Commit**

```
[Backend] Add migration for plugin_hook_config rename and mode column
```

---

### Task 2: Update Go Models

**Files:**
- Modify: `pkg/models/plugin.go`

- [ ] **Step 1: Add mode constants and rename structs**

Add mode constants after the existing `PluginStatus` constants block (around line 30):

```go
// PluginMode represents the execution mode for a plugin in hook order.
const (
	PluginModeEnabled    = "enabled"
	PluginModeManualOnly = "manual_only"
	PluginModeDisabled   = "disabled"
)
```

Replace the `PluginOrder` struct (lines 85-92):

```go
type PluginHookConfig struct {
	bun.BaseModel `bun:"table:plugin_hook_config,alias:phc" tstype:"-"`

	HookType string `bun:",pk" json:"hook_type" tstype:"PluginHookType"`
	Scope    string `bun:",pk" json:"scope"`
	PluginID string `bun:",pk" json:"plugin_id"`
	Position int    `bun:",notnull" json:"position"`
	Mode     string `bun:",notnull,default:'enabled'" json:"mode"`
}
```

Replace the `LibraryPlugin` struct (lines 101-110):

```go
type LibraryPluginHookConfig struct {
	bun.BaseModel `bun:"table:library_plugin_hook_config,alias:lphc" tstype:"-"`

	LibraryID int    `bun:",pk" json:"library_id"`
	HookType  string `bun:",pk" json:"hook_type" tstype:"PluginHookType"`
	Scope     string `bun:",pk" json:"scope"`
	PluginID  string `bun:",pk" json:"plugin_id"`
	Position  int    `bun:",notnull" json:"position"`
	Mode      string `bun:",notnull,default:'enabled'" json:"mode"`
}
```

- [ ] **Step 2: Run tygo to regenerate TypeScript types**

Run: `mise tygo`
Expected: Types regenerated. `app/types/generated/models.ts` will have `PluginHookConfig` (with `mode`) and `LibraryPluginHookConfig` (with `mode`, without `enabled`).

- [ ] **Step 3: Verify the build compiles (expect errors — we'll fix in subsequent tasks)**

Run: `cd /Users/robinjoseph/.worktrees/shisho/library-plugin && go build ./... 2>&1 | head -30`
Expected: Compile errors referencing old type names `PluginOrder` and `LibraryPlugin`. This is expected — we fix them in the next tasks.

- [ ] **Step 4: Commit**

```
[Backend] Rename PluginOrder/LibraryPlugin models, add Mode field
```

---

### Task 3: Update Service Layer

**Files:**
- Modify: `pkg/plugins/service.go`

All references to old model names and fields need updating. The changes are mechanical renames.

- [ ] **Step 1: Update GetOrder and SetOrder**

In `GetOrder` (line 164): change `[]*models.PluginOrder` to `[]*models.PluginHookConfig` and update the model reference:

```go
func (s *Service) GetOrder(ctx context.Context, hookType string) ([]*models.PluginHookConfig, error) {
	var orders []*models.PluginHookConfig
	err := s.db.NewSelect().Model(&orders).
		Where("hook_type = ?", hookType).
		OrderExpr("position ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return orders, nil
}
```

In `SetOrder` (line 177): change `[]models.PluginOrder` to `[]models.PluginHookConfig` and update model references:

```go
func (s *Service) SetOrder(ctx context.Context, hookType string, entries []models.PluginHookConfig) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*models.PluginHookConfig)(nil)).
			Where("hook_type = ?", hookType).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		for i := range entries {
			entries[i].HookType = hookType
			entries[i].Position = i
		}

		if len(entries) > 0 {
			_, err = tx.NewInsert().Model(&entries).Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
}
```

- [ ] **Step 2: Update AppendToOrder**

In `AppendToOrder` (line 202): update model references:

```go
func (s *Service) AppendToOrder(ctx context.Context, hookType, scope, pluginID string) error {
	var maxPos int
	err := s.db.NewSelect().Model((*models.PluginHookConfig)(nil)).
		ColumnExpr("COALESCE(MAX(position), -1)").
		Where("hook_type = ?", hookType).
		Scan(ctx, &maxPos)
	if err != nil {
		return errors.WithStack(err)
	}

	order := &models.PluginHookConfig{
		HookType: hookType,
		Scope:    scope,
		PluginID: pluginID,
		Position: maxPos + 1,
		Mode:     models.PluginModeEnabled,
	}
	_, err = s.db.NewInsert().Model(order).Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
```

- [ ] **Step 3: Update GetLibraryOrder and SetLibraryOrder**

In `GetLibraryOrder` (line 370): change `[]*models.LibraryPlugin` to `[]*models.LibraryPluginHookConfig`:

```go
func (s *Service) GetLibraryOrder(ctx context.Context, libraryID int, hookType string) ([]*models.LibraryPluginHookConfig, error) {
	var entries []*models.LibraryPluginHookConfig
	err := s.db.NewSelect().Model(&entries).
		Where("library_id = ? AND hook_type = ?", libraryID, hookType).
		OrderExpr("position ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return entries, nil
}
```

In `SetLibraryOrder` (line 384): change `[]models.LibraryPlugin` to `[]models.LibraryPluginHookConfig` and update all model references:

```go
func (s *Service) SetLibraryOrder(ctx context.Context, libraryID int, hookType string, entries []models.LibraryPluginHookConfig) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		customization := &models.LibraryPluginCustomization{
			LibraryID: libraryID,
			HookType:  hookType,
		}
		_, err := tx.NewInsert().Model(customization).
			On("CONFLICT (library_id, hook_type) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = tx.NewDelete().Model((*models.LibraryPluginHookConfig)(nil)).
			Where("library_id = ? AND hook_type = ?", libraryID, hookType).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		for i := range entries {
			entries[i].LibraryID = libraryID
			entries[i].HookType = hookType
			entries[i].Position = i
		}
		if len(entries) > 0 {
			_, err = tx.NewInsert().Model(&entries).Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
}
```

- [ ] **Step 4: Update ResetLibraryOrder and ResetAllLibraryOrders**

In `ResetLibraryOrder` (line 423): replace `(*models.LibraryPlugin)(nil)` with `(*models.LibraryPluginHookConfig)(nil)`.

In `ResetAllLibraryOrders` (line 439): same replacement.

- [ ] **Step 5: Verify the service compiles**

Run: `cd /Users/robinjoseph/.worktrees/shisho/library-plugin && go build ./pkg/plugins/...`
Expected: May still have errors in handler/manager — those are next. Service itself should be clean.

- [ ] **Step 6: Commit**

```
[Backend] Update plugin service layer for renamed models and mode field
```

---

### Task 4: Update Manager — Mode Filtering and GetManualRuntimes

**Files:**
- Modify: `pkg/plugins/manager.go`

- [ ] **Step 1: Update GetOrderedRuntimes to filter by mode**

Replace the current `GetOrderedRuntimes` method (lines 271-315) with mode-aware filtering on both paths:

```go
func (m *Manager) GetOrderedRuntimes(ctx context.Context, hookType string, libraryID int) ([]*Runtime, error) {
	if libraryID > 0 {
		customized, err := m.service.IsLibraryCustomized(ctx, libraryID, hookType)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check library customization for hook type %s", hookType)
		}
		if customized {
			entries, err := m.service.GetLibraryOrder(ctx, libraryID, hookType)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get library order for hook type %s", hookType)
			}
			var runtimes []*Runtime
			m.mu.RLock()
			for _, entry := range entries {
				if entry.Mode != models.PluginModeEnabled {
					continue
				}
				key := pluginKey(entry.Scope, entry.PluginID)
				if rt, ok := m.plugins[key]; ok {
					runtimes = append(runtimes, rt)
				}
			}
			m.mu.RUnlock()
			return runtimes, nil
		}
	}

	orders, err := m.service.GetOrder(ctx, hookType)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get order for hook type %s", hookType)
	}

	var runtimes []*Runtime
	m.mu.RLock()
	for _, order := range orders {
		if order.Mode != models.PluginModeEnabled {
			continue
		}
		key := pluginKey(order.Scope, order.PluginID)
		if rt, ok := m.plugins[key]; ok {
			runtimes = append(runtimes, rt)
		}
	}
	m.mu.RUnlock()

	return runtimes, nil
}
```

- [ ] **Step 2: Add GetManualRuntimes method**

Add this method directly after `GetOrderedRuntimes`:

```go
// GetManualRuntimes returns runtimes for manual identification — includes mode "enabled" and "manual_only".
// Same library/global fallback logic as GetOrderedRuntimes.
func (m *Manager) GetManualRuntimes(ctx context.Context, hookType string, libraryID int) ([]*Runtime, error) {
	if libraryID > 0 {
		customized, err := m.service.IsLibraryCustomized(ctx, libraryID, hookType)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check library customization for hook type %s", hookType)
		}
		if customized {
			entries, err := m.service.GetLibraryOrder(ctx, libraryID, hookType)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get library order for hook type %s", hookType)
			}
			var runtimes []*Runtime
			m.mu.RLock()
			for _, entry := range entries {
				if entry.Mode == models.PluginModeDisabled {
					continue
				}
				key := pluginKey(entry.Scope, entry.PluginID)
				if rt, ok := m.plugins[key]; ok {
					runtimes = append(runtimes, rt)
				}
			}
			m.mu.RUnlock()
			return runtimes, nil
		}
	}

	orders, err := m.service.GetOrder(ctx, hookType)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get order for hook type %s", hookType)
	}

	var runtimes []*Runtime
	m.mu.RLock()
	for _, order := range orders {
		if order.Mode == models.PluginModeDisabled {
			continue
		}
		key := pluginKey(order.Scope, order.PluginID)
		if rt, ok := m.plugins[key]; ok {
			runtimes = append(runtimes, rt)
		}
	}
	m.mu.RUnlock()

	return runtimes, nil
}
```

- [ ] **Step 3: Verify the manager compiles**

Run: `go build ./pkg/plugins/...`
Expected: May still have errors in handler — that's next.

- [ ] **Step 4: Commit**

```
[Backend] Add mode filtering to GetOrderedRuntimes, add GetManualRuntimes
```

---

### Task 5: Update Handler — Payload Types and Endpoints

**Files:**
- Modify: `pkg/plugins/handler.go`

- [ ] **Step 1: Update global order payload types and handler**

Update `orderEntry` (line 128) to include mode:

```go
type orderEntry struct {
	Scope string `json:"scope" validate:"required"`
	ID    string `json:"id" validate:"required"`
	Mode  string `json:"mode"`
}
```

Update `setOrder` handler (line 520) to pass mode through:

```go
func (h *handler) setOrder(c echo.Context) error {
	ctx := c.Request().Context()

	hookType := c.Param("hookType")

	var payload setOrderPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	orderEntries := make([]models.PluginHookConfig, len(payload.Order))
	for i, entry := range payload.Order {
		mode := entry.Mode
		if mode == "" {
			mode = models.PluginModeEnabled
		}
		orderEntries[i] = models.PluginHookConfig{
			Scope:    entry.Scope,
			PluginID: entry.ID,
			Mode:     mode,
		}
	}

	if err := h.service.SetOrder(ctx, hookType, orderEntries); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 2: Update library order payload types**

Replace `libraryOrderEntry` (line 953):

```go
type libraryOrderEntry struct {
	Scope string `json:"scope" validate:"required"`
	ID    string `json:"id" validate:"required"`
	Mode  string `json:"mode"`
}
```

Replace `libraryOrderPlugin` (line 968):

```go
type libraryOrderPlugin struct {
	Scope string `json:"scope"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Mode  string `json:"mode"`
}
```

- [ ] **Step 3: Update getLibraryOrder handler**

Update `getLibraryOrder` (line 975) to use `Mode` instead of `Enabled`:

In the customized branch (building `libraryOrderPlugin`):
```go
plugins = append(plugins, libraryOrderPlugin{
	Scope: entry.Scope,
	ID:    entry.PluginID,
	Name:  name,
	Mode:  entry.Mode,
})
```

In the non-customized branch (building from global order):
```go
plugins = append(plugins, libraryOrderPlugin{
	Scope: order.Scope,
	ID:    order.PluginID,
	Name:  name,
	Mode:  order.Mode,
})
```

- [ ] **Step 4: Update setLibraryOrder handler**

Update `setLibraryOrder` (line 1033) to use `Mode` instead of `Enabled`:

```go
entries := make([]models.LibraryPluginHookConfig, len(payload.Plugins))
for i, p := range payload.Plugins {
	mode := p.Mode
	if mode == "" {
		mode = models.PluginModeEnabled
	}
	entries[i] = models.LibraryPluginHookConfig{
		Scope:    p.Scope,
		PluginID: p.ID,
		Mode:     mode,
	}
}
```

- [ ] **Step 5: Update resetLibraryOrder and resetAllLibraryOrders**

In `resetLibraryOrder` (line 1063) and `resetAllLibraryOrders` (line 1079): service calls don't reference the model type directly, so these should be fine. Verify no compile errors.

- [ ] **Step 6: Update searchMetadata handler to use GetManualRuntimes**

At line 1301, change:
```go
runtimes, err := h.manager.GetOrderedRuntimes(ctx, models.PluginHookMetadataEnricher, 0)
```
to:
```go
runtimes, err := h.manager.GetManualRuntimes(ctx, models.PluginHookMetadataEnricher, book.LibraryID)
```

The `book` variable is already populated a few lines later (line 1312). The book retrieval needs to happen before the runtimes call. Reorder the handler logic:

Move the book retrieval block (lines 1311-1328: the `var book *models.Book` block including library access check) to **before** the `GetManualRuntimes` call. Then pass `book.LibraryID` to `GetManualRuntimes`. The reordered flow:

1. Bind payload
2. Retrieve book and check library access
3. Get manual runtimes (using `book.LibraryID`)
4. Build search context and run searches

- [ ] **Step 7: Verify the entire backend compiles**

Run: `cd /Users/robinjoseph/.worktrees/shisho/library-plugin && go build ./...`
Expected: Clean compilation

- [ ] **Step 8: Commit**

```
[Backend] Update handlers for mode field, use GetManualRuntimes for manual search
```

---

### Task 6: Update Backend Tests

**Files:**
- Modify: `pkg/plugins/service_test.go`
- Modify: `pkg/plugins/manager_test.go`

- [ ] **Step 1: Update service_test.go — rename types**

In `TestService_GetOrder_SetOrder` (line 258): change `models.PluginOrder` to `models.PluginHookConfig`:

```go
entries := []models.PluginHookConfig{
	{Scope: "community", PluginID: "plugin-b"},
	{Scope: "community", PluginID: "plugin-a"},
	{Scope: "shisho", PluginID: "plugin-c"},
}
```

Same for the `newEntries` block.

In `TestService_AppendToOrder` (line 307): verify mode is set. Add assertion after the first append:

```go
assert.Equal(t, models.PluginModeEnabled, orders[0].Mode)
```

- [ ] **Step 2: Add a test for SetOrder with mode**

Add a new test after `TestService_AppendToOrder`:

```go
func TestService_SetOrder_WithMode(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "plugin-a")
	insertTestPlugin(t, db, "community", "plugin-b")

	entries := []models.PluginHookConfig{
		{Scope: "community", PluginID: "plugin-a", Mode: models.PluginModeEnabled},
		{Scope: "community", PluginID: "plugin-b", Mode: models.PluginModeManualOnly},
	}
	err := svc.SetOrder(ctx, models.PluginHookMetadataEnricher, entries)
	require.NoError(t, err)

	orders, err := svc.GetOrder(ctx, models.PluginHookMetadataEnricher)
	require.NoError(t, err)
	require.Len(t, orders, 2)
	assert.Equal(t, models.PluginModeEnabled, orders[0].Mode)
	assert.Equal(t, models.PluginModeManualOnly, orders[1].Mode)
}
```

- [ ] **Step 3: Update manager_test.go — WithLibrary test**

In `TestManager_GetOrderedRuntimes_WithLibrary` (line 399): update `models.PluginOrder` to `models.PluginHookConfig` and `models.LibraryPlugin` to `models.LibraryPluginHookConfig`:

```go
err = svc.SetOrder(ctx, "metadataEnricher", []models.PluginHookConfig{
	{Scope: "test", PluginID: "enricher1"},
	{Scope: "test", PluginID: "enricher2"},
})
```

```go
err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPluginHookConfig{
	{Scope: "test", PluginID: "enricher2", Mode: models.PluginModeEnabled},
	{Scope: "test", PluginID: "enricher1", Mode: models.PluginModeDisabled},
})
```

In `TestManager_GetOrderedRuntimes_GlobalDisabledExcluded` (line 461): same renames:

```go
err = svc.SetOrder(ctx, "metadataEnricher", []models.PluginHookConfig{...})
err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPluginHookConfig{
	{Scope: "test", PluginID: "enricher1", Mode: models.PluginModeEnabled},
	{Scope: "test", PluginID: "enricher2", Mode: models.PluginModeEnabled},
})
```

- [ ] **Step 4: Add tests for manual_only mode and GetManualRuntimes**

Add a new test:

```go
func TestManager_GetManualRuntimes(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	mgr := &Manager{
		service: svc,
		plugins: make(map[string]*Runtime),
	}

	library := insertTestLibrary(t, db, "Test Library")

	p1 := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Enricher 1", Version: "1.0.0", Status: models.PluginStatusActive}
	p2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Enricher 2", Version: "1.0.0", Status: models.PluginStatusActive}
	p3 := &models.Plugin{Scope: "test", ID: "enricher3", Name: "Enricher 3", Version: "1.0.0", Status: models.PluginStatusActive}
	_, err := db.NewInsert().Model(p1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(p2).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(p3).Exec(ctx)
	require.NoError(t, err)

	err = svc.SetOrder(ctx, "metadataEnricher", []models.PluginHookConfig{
		{Scope: "test", PluginID: "enricher1"},
		{Scope: "test", PluginID: "enricher2"},
		{Scope: "test", PluginID: "enricher3"},
	})
	require.NoError(t, err)

	rt1 := &Runtime{scope: "test", pluginID: "enricher1"}
	rt2 := &Runtime{scope: "test", pluginID: "enricher2"}
	rt3 := &Runtime{scope: "test", pluginID: "enricher3"}
	mgr.plugins["test/enricher1"] = rt1
	mgr.plugins["test/enricher2"] = rt2
	mgr.plugins["test/enricher3"] = rt3

	// Set library order: enricher1=enabled, enricher2=manual_only, enricher3=disabled
	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPluginHookConfig{
		{Scope: "test", PluginID: "enricher1", Mode: models.PluginModeEnabled},
		{Scope: "test", PluginID: "enricher2", Mode: models.PluginModeManualOnly},
		{Scope: "test", PluginID: "enricher3", Mode: models.PluginModeDisabled},
	})
	require.NoError(t, err)

	// GetOrderedRuntimes: only "enabled" — enricher1
	runtimes, err := mgr.GetOrderedRuntimes(ctx, "metadataEnricher", library.ID)
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	assert.Equal(t, "enricher1", runtimes[0].pluginID)

	// GetManualRuntimes: "enabled" + "manual_only" — enricher1, enricher2
	runtimes, err = mgr.GetManualRuntimes(ctx, "metadataEnricher", library.ID)
	require.NoError(t, err)
	require.Len(t, runtimes, 2)
	assert.Equal(t, "enricher1", runtimes[0].pluginID)
	assert.Equal(t, "enricher2", runtimes[1].pluginID)
}
```

- [ ] **Step 5: Add test for global mode filtering**

```go
func TestManager_GetOrderedRuntimes_GlobalModeFiltering(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	mgr := &Manager{
		service: svc,
		plugins: make(map[string]*Runtime),
	}

	p1 := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Enricher 1", Version: "1.0.0", Status: models.PluginStatusActive}
	p2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Enricher 2", Version: "1.0.0", Status: models.PluginStatusActive}
	_, err := db.NewInsert().Model(p1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(p2).Exec(ctx)
	require.NoError(t, err)

	// Global order: enricher1=enabled, enricher2=manual_only
	err = svc.SetOrder(ctx, "metadataEnricher", []models.PluginHookConfig{
		{Scope: "test", PluginID: "enricher1", Mode: models.PluginModeEnabled},
		{Scope: "test", PluginID: "enricher2", Mode: models.PluginModeManualOnly},
	})
	require.NoError(t, err)

	rt1 := &Runtime{scope: "test", pluginID: "enricher1"}
	rt2 := &Runtime{scope: "test", pluginID: "enricher2"}
	mgr.plugins["test/enricher1"] = rt1
	mgr.plugins["test/enricher2"] = rt2

	// Auto-scan (libraryID=0, falls back to global): only enricher1
	runtimes, err := mgr.GetOrderedRuntimes(ctx, "metadataEnricher", 0)
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	assert.Equal(t, "enricher1", runtimes[0].pluginID)

	// Manual (libraryID=0, falls back to global): enricher1 + enricher2
	runtimes, err = mgr.GetManualRuntimes(ctx, "metadataEnricher", 0)
	require.NoError(t, err)
	require.Len(t, runtimes, 2)
	assert.Equal(t, "enricher1", runtimes[0].pluginID)
	assert.Equal(t, "enricher2", runtimes[1].pluginID)
}
```

- [ ] **Step 6: Run the tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/library-plugin && go test ./pkg/plugins/... -v -count=1 2>&1 | tail -30`
Expected: All tests pass

- [ ] **Step 7: Commit**

```
[Test] Update plugin service/manager tests for mode field
```

---

### Task 7: Update Frontend Query Hooks

**Files:**
- Modify: `app/hooks/queries/plugins.ts`

- [ ] **Step 1: Update LibraryPluginOrderPlugin interface**

Replace `enabled: boolean` with `mode` (line 413-418):

```typescript
export interface LibraryPluginOrderPlugin {
  scope: string;
  id: string;
  name: string;
  mode: "enabled" | "manual_only" | "disabled";
}
```

- [ ] **Step 2: Update useSetPluginOrder mutation type**

At line 272, update the mutation variables type to include mode:

```typescript
export const useSetPluginOrder = () => {
  const queryClient = useQueryClient();

  return useMutation<
    void,
    ShishoAPIError,
    {
      hookType: string;
      order: { scope: string; id: string; mode: string }[];
    }
  >({
    mutationFn: ({ hookType, order }) => {
      return API.request("PUT", `/plugins/order/${hookType}`, { order });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PluginOrder, variables.hookType],
      });
    },
  });
};
```

- [ ] **Step 3: Update useSetLibraryPluginOrder mutation type**

At line 444, replace `enabled: boolean` with `mode: string`:

```typescript
export const useSetLibraryPluginOrder = () => {
  const queryClient = useQueryClient();
  return useMutation<
    void,
    ShishoAPIError,
    {
      libraryId: string;
      hookType: string;
      plugins: { scope: string; id: string; mode: string }[];
    }
  >({
    mutationFn: ({ libraryId, hookType, plugins }) => {
      return API.request(
        "PUT",
        `/libraries/${libraryId}/plugins/order/${hookType}`,
        { plugins },
      );
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["libraries", variables.libraryId, "plugins", "order"],
      });
    },
  });
};
```

- [ ] **Step 4: Commit**

```
[Frontend] Update plugin query hooks for mode field
```

---

### Task 8: Update LibraryPluginsTab — Replace Toggle with Dropdown

**Files:**
- Modify: `app/components/library/LibraryPluginsTab.tsx`

- [ ] **Step 1: Replace the toggle handler with a mode handler**

Remove the `handleToggle` function (lines 80-87) and replace with:

```typescript
const handleModeChange = (index: number, mode: LibraryPluginOrderPlugin["mode"]) => {
  const newPlugins = [...displayPlugins];
  newPlugins[index] = {
    ...newPlugins[index],
    mode,
  };
  setLocalPlugins(newPlugins);
};
```

- [ ] **Step 2: Update the hasChanged comparison**

Change line 61 from `item.enabled !== data?.plugins?.[i]?.enabled` to `item.mode !== data?.plugins?.[i]?.mode`.

- [ ] **Step 3: Update handleSave to send mode instead of enabled**

Replace the save mutation call (lines 93-113) to send `mode`:

```typescript
const handleSave = () => {
  setOrder.mutate(
    {
      libraryId,
      hookType: selectedHookType,
      plugins: displayPlugins.map((p) => ({
        scope: p.scope,
        id: p.id,
        mode: p.mode,
      })),
    },
    {
      onSuccess: () => {
        setLocalPlugins(null);
        toast.success("Library plugin order saved.");
      },
      onError: (err) => {
        toast.error(`Failed to save: ${err.message}`);
      },
    },
  );
};
```

- [ ] **Step 4: Update the plugin row rendering — replace Switch with Select**

Remove `Switch` from imports and ensure `Select`, `SelectContent`, `SelectItem`, `SelectTrigger`, `SelectValue` are imported.

Replace the `<Switch>` element in the plugin row (lines 200-203) with a mode select dropdown:

```tsx
<Select
  onValueChange={(value) =>
    handleModeChange(
      index,
      value as LibraryPluginOrderPlugin["mode"],
    )
  }
  value={plugin.mode}
>
  <SelectTrigger className="w-[140px] h-8 text-xs">
    <SelectValue />
  </SelectTrigger>
  <SelectContent>
    <SelectItem value="enabled">Enabled</SelectItem>
    {selectedHookType === "metadataEnricher" && (
      <SelectItem value="manual_only">
        Manual Only
      </SelectItem>
    )}
    <SelectItem value="disabled">Disabled</SelectItem>
  </SelectContent>
</Select>
```

- [ ] **Step 5: Update the row styling for mode states**

Replace the conditional class on the plugin row div (line 186-188) from:

```tsx
plugin.enabled ? "border-border" : "border-border/50 opacity-60"
```

to:

```tsx
plugin.mode === "disabled"
  ? "border-border/50 opacity-60"
  : plugin.mode === "manual_only"
    ? "border-border/70 opacity-80"
    : "border-border"
```

- [ ] **Step 6: Verify the component compiles**

Run: `cd /Users/robinjoseph/.worktrees/shisho/library-plugin && pnpm lint:types`
Expected: No type errors

- [ ] **Step 7: Commit**

```
[Frontend] Replace toggle with mode dropdown in LibraryPluginsTab
```

---

### Task 9: Update AdminPlugins — Add Mode Dropdown to Global Order

**Files:**
- Modify: `app/components/pages/AdminPlugins.tsx`

- [ ] **Step 1: Add local state type with mode**

The global order currently uses `PluginOrder` from generated types (which now includes `mode`). The `localOrder` state needs to track mode changes.

Update the `hasOrderChanged` comparison (line 466-473) to also detect mode changes:

```typescript
const hasOrderChanged =
  localOrder !== null &&
  (localOrder.length !== (order?.length ?? 0) ||
    localOrder.some(
      (item, i) =>
        item.scope !== order?.[i]?.scope ||
        item.plugin_id !== order?.[i]?.plugin_id ||
        item.mode !== order?.[i]?.mode,
    ));
```

- [ ] **Step 2: Add a mode change handler**

Add after `handleMove`:

```typescript
const handleModeChange = (index: number, mode: string) => {
  const newOrder = [...displayOrder];
  newOrder[index] = { ...newOrder[index], mode };
  setLocalOrder(newOrder);
};
```

- [ ] **Step 3: Update handleSave to include mode**

Update the save mutation (line 486-501) to include mode:

```typescript
const handleSave = () => {
  setPluginOrder.mutate(
    {
      hookType: selectedHookType,
      order: displayOrder.map((o) => ({
        scope: o.scope,
        id: o.plugin_id,
        mode: o.mode,
      })),
    },
    {
      onSuccess: () => {
        setLocalOrder(null);
        toast.success("Plugin order saved.");
      },
      onError: (err) => {
        toast.error(`Failed to save order: ${err.message}`);
      },
    },
  );
};
```

- [ ] **Step 4: Add mode dropdown to each plugin row**

Inside the plugin row div (after the name/badge section, within the `canWrite` conditional at line 567), add a mode select before the arrow buttons:

```tsx
{canWrite && (
  <div className="flex items-center gap-1">
    <Select
      onValueChange={(value) => handleModeChange(index, value)}
      value={item.mode}
    >
      <SelectTrigger className="w-[140px] h-8 text-xs">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="enabled">Enabled</SelectItem>
        {selectedHookType === "metadataEnricher" && (
          <SelectItem value="manual_only">
            Manual Only
          </SelectItem>
        )}
        <SelectItem value="disabled">Disabled</SelectItem>
      </SelectContent>
    </Select>
    <Button
      disabled={index === 0}
      onClick={() => handleMove(index, "up")}
      size="sm"
      variant="ghost"
    >
      <ArrowUp className="h-4 w-4" />
    </Button>
    <Button
      disabled={index === displayOrder.length - 1}
      onClick={() => handleMove(index, "down")}
      size="sm"
      variant="ghost"
    >
      <ArrowDown className="h-4 w-4" />
    </Button>
  </div>
)}
```

- [ ] **Step 5: Update row styling for mode states**

Update the row div class (line 555) from:

```tsx
className="flex items-center justify-between gap-3 rounded-md border border-border p-3"
```

to:

```tsx
className={`flex items-center justify-between gap-3 rounded-md border p-3 ${
  item.mode === "disabled"
    ? "border-border/50 opacity-60"
    : item.mode === "manual_only"
      ? "border-border/70 opacity-80"
      : "border-border"
}`}
```

- [ ] **Step 6: Add Select imports**

Ensure `Select`, `SelectContent`, `SelectItem`, `SelectTrigger`, `SelectValue` are imported in AdminPlugins.tsx. Check existing imports — the component may already import `Select` for the hook type dropdown.

- [ ] **Step 7: Verify the frontend compiles and lints**

Run: `cd /Users/robinjoseph/.worktrees/shisho/library-plugin && pnpm lint:types && pnpm lint:eslint`
Expected: No errors

- [ ] **Step 8: Commit**

```
[Frontend] Add mode dropdown to global plugin order in AdminPlugins
```

---

### Task 10: Update CLAUDE.md and Run Full Checks

**Files:**
- Modify: `pkg/plugins/CLAUDE.md`

- [ ] **Step 1: Update CLAUDE.md table names**

In `pkg/plugins/CLAUDE.md`, find the Database Tables section and update:
- `plugin_order` → `plugin_hook_config`
- `library_plugins` → `library_plugin_hook_config`
- Update the field descriptions to mention `mode` instead of or in addition to existing fields

Find the `GetOrderedRuntimes` entry in the Manager Lifecycle table and add `GetManualRuntimes`:

```
| `GetManualRuntimes(ctx, hookType, libraryID)` | Manual identification | Get runtimes with mode enabled or manual_only |
```

- [ ] **Step 2: Run full check suite**

Run: `cd /Users/robinjoseph/.worktrees/shisho/library-plugin && mise check:quiet`
Expected: All checks pass (Go tests, Go lint, JS lint)

- [ ] **Step 3: Fix any issues found by check suite**

If tests fail or lint errors appear, fix them. Common issues:
- Scan plugin tests in `pkg/worker/scan_plugins_test.go` reference `models.PluginOrder` or `models.LibraryPlugin` — update to new names
- Any remaining `Enabled` field references in test files

- [ ] **Step 4: Commit**

```
[Docs] Update plugin CLAUDE.md for renamed tables and new mode field
```

---

### Task 11: Update Documentation

**Files:**
- Modify: `website/docs/plugins/` (whichever page covers plugin ordering/configuration)

- [ ] **Step 1: Find and update the relevant docs page**

Search `website/docs/plugins/` for content about plugin order, library settings, or the enabled/disabled toggle. Update to describe the three modes:

- **Enabled** — Plugin runs during automated scans and is available for manual identification
- **Manual Only** — Plugin is skipped during automated scans but available for manual identification (metadata enrichers only)
- **Disabled** — Plugin is completely unavailable for this library/globally

Document that this applies at both global (Settings → Plugins → Order) and per-library (Library Settings → Plugin Order) levels.

- [ ] **Step 2: Commit**

```
[Docs] Document three-state plugin mode in plugin docs
```
