# Per-Library Plugin Configuration Implementation Plan

> **Status:** Implemented

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow each library to customize which plugins run and in what order per hook type, while inheriting global defaults by default.

**Architecture:** Two new DB tables (`library_plugin_customizations`, `library_plugins`) track per-library overrides. The plugin manager's `GetOrderedRuntimes` gains a `libraryID` parameter to resolve the effective order. New API endpoints on the libraries group expose per-library plugin management. The library settings UI gets a "Plugins" tab.

**Tech Stack:** Go (Echo, Bun ORM, SQLite), React 19 (TypeScript, TanStack Query, TailwindCSS)

---

### Task 1: Database Migration

**Files:**
- Create: `pkg/migrations/20260124000000_create_library_plugin_tables.go`

**Step 1: Create the migration file**

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
			CREATE TABLE library_plugin_customizations (
				library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
				hook_type TEXT NOT NULL,
				PRIMARY KEY (library_id, hook_type)
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			CREATE TABLE library_plugins (
				library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
				scope TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				hook_type TEXT NOT NULL,
				enabled BOOLEAN NOT NULL DEFAULT true,
				position INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY (library_id, hook_type, scope, plugin_id),
				FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_library_plugins_order ON library_plugins(library_id, hook_type, position)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS idx_library_plugins_order`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS library_plugins`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS library_plugin_customizations`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

**Step 2: Run migration to verify it applies**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugins-sp && make db:migrate`
Expected: Migration applies cleanly

**Step 3: Rollback to verify down migration works**

Run: `make db:rollback`
Expected: Tables dropped cleanly

**Step 4: Re-apply migration**

Run: `make db:migrate`
Expected: Migration applies cleanly again

**Step 5: Commit**

```bash
git add pkg/migrations/20260124000000_create_library_plugin_tables.go
git commit -m "[Plugins] Add library_plugin_customizations and library_plugins tables"
```

---

### Task 2: Bun Models

**Files:**
- Modify: `pkg/models/plugin.go`

**Step 1: Add the two new model structs to the end of `pkg/models/plugin.go`**

Append after the `PluginOrder` struct:

```go
type LibraryPluginCustomization struct {
	bun.BaseModel `bun:"table:library_plugin_customizations,alias:lpc" tstype:"-"`

	LibraryID int    `bun:",pk" json:"library_id"`
	HookType  string `bun:",pk" json:"hook_type" tstype:"PluginHookType"`
}

type LibraryPlugin struct {
	bun.BaseModel `bun:"table:library_plugins,alias:lp" tstype:"-"`

	LibraryID int    `bun:",pk" json:"library_id"`
	HookType  string `bun:",pk" json:"hook_type" tstype:"PluginHookType"`
	Scope     string `bun:",pk" json:"scope"`
	PluginID  string `bun:",pk" json:"plugin_id"`
	Enabled   bool   `bun:",notnull" json:"enabled"`
	Position  int    `bun:",notnull" json:"position"`
}
```

**Step 2: Run `make tygo` to regenerate TypeScript types**

Run: `make tygo`
Expected: Types generated (or "Nothing to be done" if already up-to-date from `make start`)

**Step 3: Commit**

```bash
git add pkg/models/plugin.go
git commit -m "[Plugins] Add LibraryPluginCustomization and LibraryPlugin models"
```

---

### Task 3: Service Layer - Library Plugin Order Methods

**Files:**
- Modify: `pkg/plugins/service.go`
- Create: `pkg/plugins/service_library_test.go`

**Step 1: Write tests for the new service methods**

Create `pkg/plugins/service_library_test.go`:

```go
package plugins

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_IsLibraryCustomized(t *testing.T) {
	db := testutils.SetupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Not customized by default
	customized, err := svc.IsLibraryCustomized(ctx, 1, "metadataEnricher")
	require.NoError(t, err)
	assert.False(t, customized)

	// Mark as customized
	err = svc.SetLibraryOrder(ctx, 1, "metadataEnricher", []models.LibraryPlugin{})
	require.NoError(t, err)

	customized, err = svc.IsLibraryCustomized(ctx, 1, "metadataEnricher")
	require.NoError(t, err)
	assert.True(t, customized)
}

func TestService_GetLibraryOrder(t *testing.T) {
	db := testutils.SetupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Setup: install a plugin first
	plugin := &models.Plugin{Scope: "test", ID: "enricher", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	// Set library order with two plugins
	plugin2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Test Enricher 2", Version: "1.0.0", Enabled: true}
	_, err = db.NewInsert().Model(plugin2).Exec(ctx)
	require.NoError(t, err)

	entries := []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher2", Enabled: true},
		{Scope: "test", PluginID: "enricher", Enabled: false},
	}
	err = svc.SetLibraryOrder(ctx, 1, "metadataEnricher", entries)
	require.NoError(t, err)

	// Retrieve - should be ordered by position
	order, err := svc.GetLibraryOrder(ctx, 1, "metadataEnricher")
	require.NoError(t, err)
	require.Len(t, order, 2)
	assert.Equal(t, "enricher2", order[0].PluginID)
	assert.True(t, order[0].Enabled)
	assert.Equal(t, 0, order[0].Position)
	assert.Equal(t, "enricher", order[1].PluginID)
	assert.False(t, order[1].Enabled)
	assert.Equal(t, 1, order[1].Position)
}

func TestService_ResetLibraryOrder(t *testing.T) {
	db := testutils.SetupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Setup
	plugin := &models.Plugin{Scope: "test", ID: "enricher", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	err = svc.SetLibraryOrder(ctx, 1, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher", Enabled: true},
	})
	require.NoError(t, err)

	// Reset
	err = svc.ResetLibraryOrder(ctx, 1, "metadataEnricher")
	require.NoError(t, err)

	customized, err := svc.IsLibraryCustomized(ctx, 1, "metadataEnricher")
	require.NoError(t, err)
	assert.False(t, customized)

	order, err := svc.GetLibraryOrder(ctx, 1, "metadataEnricher")
	require.NoError(t, err)
	assert.Len(t, order, 0)
}

func TestService_ResetAllLibraryOrders(t *testing.T) {
	db := testutils.SetupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Setup
	plugin := &models.Plugin{Scope: "test", ID: "enricher", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	err = svc.SetLibraryOrder(ctx, 1, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher", Enabled: true},
	})
	require.NoError(t, err)
	err = svc.SetLibraryOrder(ctx, 1, "fileParser", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher", Enabled: true},
	})
	require.NoError(t, err)

	// Reset all
	err = svc.ResetAllLibraryOrders(ctx, 1)
	require.NoError(t, err)

	customized, _ := svc.IsLibraryCustomized(ctx, 1, "metadataEnricher")
	assert.False(t, customized)
	customized, _ = svc.IsLibraryCustomized(ctx, 1, "fileParser")
	assert.False(t, customized)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugins-sp && go test ./pkg/plugins/ -run TestService_IsLibraryCustomized -v`
Expected: FAIL (methods don't exist yet)

**Step 3: Implement the service methods**

Add to the end of `pkg/plugins/service.go`:

```go
// IsLibraryCustomized checks if a library has customized plugin order for a hook type.
func (s *Service) IsLibraryCustomized(ctx context.Context, libraryID int, hookType string) (bool, error) {
	exists, err := s.db.NewSelect().Model((*models.LibraryPluginCustomization)(nil)).
		Where("library_id = ? AND hook_type = ?", libraryID, hookType).
		Exists(ctx)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return exists, nil
}

// GetLibraryOrder returns the per-library plugin order for a hook type, sorted by position.
func (s *Service) GetLibraryOrder(ctx context.Context, libraryID int, hookType string) ([]*models.LibraryPlugin, error) {
	var entries []*models.LibraryPlugin
	err := s.db.NewSelect().Model(&entries).
		Where("library_id = ? AND hook_type = ?", libraryID, hookType).
		OrderExpr("position ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return entries, nil
}

// SetLibraryOrder replaces all per-library plugin order entries for a hook type.
// Also creates the customization record if it doesn't exist.
func (s *Service) SetLibraryOrder(ctx context.Context, libraryID int, hookType string, entries []models.LibraryPlugin) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Upsert customization record
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

		// Delete existing entries
		_, err = tx.NewDelete().Model((*models.LibraryPlugin)(nil)).
			Where("library_id = ? AND hook_type = ?", libraryID, hookType).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Insert new entries with positions
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

// ResetLibraryOrder removes per-library customization for a specific hook type.
func (s *Service) ResetLibraryOrder(ctx context.Context, libraryID int, hookType string) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*models.LibraryPlugin)(nil)).
			Where("library_id = ? AND hook_type = ?", libraryID, hookType).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = tx.NewDelete().Model((*models.LibraryPluginCustomization)(nil)).
			Where("library_id = ? AND hook_type = ?", libraryID, hookType).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// ResetAllLibraryOrders removes all per-library plugin customizations for a library.
func (s *Service) ResetAllLibraryOrders(ctx context.Context, libraryID int) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().Model((*models.LibraryPlugin)(nil)).
			Where("library_id = ?", libraryID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = tx.NewDelete().Model((*models.LibraryPluginCustomization)(nil)).
			Where("library_id = ?", libraryID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./pkg/plugins/ -run "TestService_(IsLibraryCustomized|GetLibraryOrder|ResetLibraryOrder|ResetAllLibraryOrders)" -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/plugins/service.go pkg/plugins/service_library_test.go
git commit -m "[Plugins] Add service methods for per-library plugin order"
```

---

### Task 4: Manager - Library-Aware GetOrderedRuntimes

**Files:**
- Modify: `pkg/plugins/manager.go`
- Modify: `pkg/plugins/manager_test.go`

**Step 1: Write failing test for library-aware resolution**

Add to `pkg/plugins/manager_test.go`:

```go
func TestManager_GetOrderedRuntimes_WithLibrary(t *testing.T) {
	db := testutils.SetupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	mgr := &Manager{
		service: svc,
		plugins: make(map[string]*Runtime),
	}

	// Install two plugins
	p1 := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Enricher 1", Version: "1.0.0", Enabled: true}
	p2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Enricher 2", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(p1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(p2).Exec(ctx)
	require.NoError(t, err)

	// Set global order: enricher1, enricher2
	err = svc.SetOrder(ctx, "metadataEnricher", []models.PluginOrder{
		{Scope: "test", PluginID: "enricher1"},
		{Scope: "test", PluginID: "enricher2"},
	})
	require.NoError(t, err)

	// Create mock runtimes
	rt1 := &Runtime{scope: "test", pluginID: "enricher1"}
	rt2 := &Runtime{scope: "test", pluginID: "enricher2"}
	mgr.plugins["test/enricher1"] = rt1
	mgr.plugins["test/enricher2"] = rt2

	// libraryID=0 falls back to global order
	runtimes, err := mgr.GetOrderedRuntimes(ctx, "metadataEnricher", 0)
	require.NoError(t, err)
	require.Len(t, runtimes, 2)
	assert.Equal(t, "enricher1", runtimes[0].pluginID)
	assert.Equal(t, "enricher2", runtimes[1].pluginID)

	// Set library-specific order: only enricher2 (enricher1 disabled)
	err = svc.SetLibraryOrder(ctx, 1, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher2", Enabled: true},
		{Scope: "test", PluginID: "enricher1", Enabled: false},
	})
	require.NoError(t, err)

	// libraryID=1 uses library order (only enabled plugins)
	runtimes, err = mgr.GetOrderedRuntimes(ctx, "metadataEnricher", 1)
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	assert.Equal(t, "enricher2", runtimes[0].pluginID)

	// Non-customized library falls back to global
	runtimes, err = mgr.GetOrderedRuntimes(ctx, "metadataEnricher", 99)
	require.NoError(t, err)
	require.Len(t, runtimes, 2)
}

func TestManager_GetOrderedRuntimes_GlobalDisabledExcluded(t *testing.T) {
	db := testutils.SetupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	mgr := &Manager{
		service: svc,
		plugins: make(map[string]*Runtime),
	}

	// Install one enabled, one disabled plugin
	p1 := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Enricher 1", Version: "1.0.0", Enabled: true}
	p2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Enricher 2", Version: "1.0.0", Enabled: false}
	_, err := db.NewInsert().Model(p1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(p2).Exec(ctx)
	require.NoError(t, err)

	// Set library order with both enabled
	err = svc.SetLibraryOrder(ctx, 1, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher1", Enabled: true},
		{Scope: "test", PluginID: "enricher2", Enabled: true},
	})
	require.NoError(t, err)

	// Only enricher1 is loaded (enricher2 globally disabled, never loaded)
	rt1 := &Runtime{scope: "test", pluginID: "enricher1"}
	mgr.plugins["test/enricher1"] = rt1

	// enricher2 not in plugins map = globally disabled/unloaded, so excluded
	runtimes, err := mgr.GetOrderedRuntimes(ctx, "metadataEnricher", 1)
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	assert.Equal(t, "enricher1", runtimes[0].pluginID)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./pkg/plugins/ -run "TestManager_GetOrderedRuntimes_(WithLibrary|GlobalDisabledExcluded)" -v`
Expected: FAIL (signature mismatch - missing libraryID param)

**Step 3: Update GetOrderedRuntimes signature and implementation**

In `pkg/plugins/manager.go`, replace the existing `GetOrderedRuntimes` method (lines 199-217):

```go
// GetOrderedRuntimes returns runtimes for a hook type in user-defined order.
// If libraryID > 0 and the library has customized the order for this hook type,
// uses the per-library order (only enabled entries). Otherwise falls back to global order.
// In both paths, only runtimes that are actually loaded (globally enabled) are returned.
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
				if !entry.Enabled {
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

	// Fall back to global order
	orders, err := m.service.GetOrder(ctx, hookType)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get order for hook type %s", hookType)
	}

	var runtimes []*Runtime
	m.mu.RLock()
	for _, order := range orders {
		key := pluginKey(order.Scope, order.PluginID)
		if rt, ok := m.plugins[key]; ok {
			runtimes = append(runtimes, rt)
		}
	}
	m.mu.RUnlock()

	return runtimes, nil
}
```

**Step 4: Fix all existing callers to pass libraryID=0 (global fallback)**

The callers are:
- `pkg/worker/scan_unified.go:2154` — `w.pluginManager.GetOrderedRuntimes(ctx, models.PluginHookMetadataEnricher)` → add `, 0` for now (will be updated in Task 6)
- `pkg/worker/scan.go:448` — `w.pluginManager.GetOrderedRuntimes(ctx, models.PluginHookInputConverter)` → add `, 0` for now (will be updated in Task 6)

Also check for any test callers that need updating.

**Step 5: Run all tests to verify they pass**

Run: `go test ./pkg/plugins/ -run "TestManager_GetOrderedRuntimes" -v`
Expected: All PASS

Run: `go test ./pkg/... -count=1`
Expected: All PASS (no broken callers)

**Step 6: Commit**

```bash
git add pkg/plugins/manager.go pkg/plugins/manager_test.go pkg/worker/scan_unified.go pkg/worker/scan.go
git commit -m "[Plugins] Make GetOrderedRuntimes library-aware with fallback to global order"
```

---

### Task 5: API Endpoints - Library Plugin Order

**Files:**
- Modify: `pkg/plugins/handler.go`
- Modify: `pkg/plugins/routes.go`
- Modify: `pkg/server/server.go`

**Step 1: Add handler methods and payload types**

Add to `pkg/plugins/handler.go`:

```go
type libraryOrderEntry struct {
	Scope   string `json:"scope" validate:"required"`
	ID      string `json:"id" validate:"required"`
	Enabled bool   `json:"enabled"`
}

type setLibraryOrderPayload struct {
	Plugins []libraryOrderEntry `json:"plugins" validate:"required"`
}

type libraryOrderResponse struct {
	Customized bool                  `json:"customized"`
	Plugins    []libraryOrderPlugin  `json:"plugins"`
}

type libraryOrderPlugin struct {
	Scope   string `json:"scope"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func (h *handler) getLibraryOrder(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	hookType := c.Param("hookType")

	customized, err := h.service.IsLibraryCustomized(ctx, libraryID, hookType)
	if err != nil {
		return errors.WithStack(err)
	}

	var plugins []libraryOrderPlugin

	if customized {
		entries, err := h.service.GetLibraryOrder(ctx, libraryID, hookType)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, entry := range entries {
			name := entry.Scope + "/" + entry.PluginID
			if p, _ := h.service.GetPlugin(ctx, entry.Scope, entry.PluginID); p != nil {
				name = p.Name
			}
			plugins = append(plugins, libraryOrderPlugin{
				Scope:   entry.Scope,
				ID:      entry.PluginID,
				Name:    name,
				Enabled: entry.Enabled,
			})
		}
	} else {
		// Return global order (all enabled by default)
		orders, err := h.service.GetOrder(ctx, hookType)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, order := range orders {
			name := order.Scope + "/" + order.PluginID
			if p, _ := h.service.GetPlugin(ctx, order.Scope, order.PluginID); p != nil {
				name = p.Name
			}
			plugins = append(plugins, libraryOrderPlugin{
				Scope:   order.Scope,
				ID:      order.PluginID,
				Name:    name,
				Enabled: true,
			})
		}
	}

	return c.JSON(http.StatusOK, libraryOrderResponse{
		Customized: customized,
		Plugins:    plugins,
	})
}

func (h *handler) setLibraryOrder(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	hookType := c.Param("hookType")

	var payload setLibraryOrderPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	entries := make([]models.LibraryPlugin, len(payload.Plugins))
	for i, p := range payload.Plugins {
		entries[i] = models.LibraryPlugin{
			Scope:    p.Scope,
			PluginID: p.ID,
			Enabled:  p.Enabled,
		}
	}

	if err := h.service.SetLibraryOrder(ctx, libraryID, hookType, entries); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) resetLibraryOrder(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}
	hookType := c.Param("hookType")

	if err := h.service.ResetLibraryOrder(ctx, libraryID, hookType); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) resetAllLibraryOrders(c echo.Context) error {
	ctx := c.Request().Context()

	libraryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid library ID")
	}

	if err := h.service.ResetAllLibraryOrders(ctx, libraryID); err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}
```

Also add `"strconv"` to the imports block.

**Step 2: Register routes on the libraries group**

In `pkg/server/server.go`, after line 124 (`libraries.RegisterRoutesWithGroup(librariesGroup, db, authMiddleware)`), add:

```go
	// Per-library plugin order routes
	librariesGroup.GET("/:id/plugins/order/:hookType", pluginHandler.getLibraryOrder, authMiddleware.RequireLibraryAccess("id"))
	librariesGroup.PUT("/:id/plugins/order/:hookType", pluginHandler.setLibraryOrder, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
	librariesGroup.DELETE("/:id/plugins/order/:hookType", pluginHandler.resetLibraryOrder, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
	librariesGroup.DELETE("/:id/plugins/order", pluginHandler.resetAllLibraryOrders, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
```

This requires extracting the handler creation. Before `plugins.RegisterRoutesWithGroup(...)`, create the handler:

```go
	pluginHandler := plugins.NewHandler(pluginService, pm, pluginInstaller)
```

And modify `RegisterRoutesWithGroup` to also accept the handler (or alternatively, create a separate `RegisterLibraryRoutes` function in the plugins package). The simplest approach: add a `NewHandler` function to `pkg/plugins/handler.go`:

```go
// NewHandler creates a plugin handler for use with external route registration.
func NewHandler(service *Service, manager *Manager, installer *Installer) *handler {
	return &handler{service: service, manager: manager, installer: installer}
}
```

And export the library-order handler methods by making them public (capitalize first letter: `GetLibraryOrder`, `SetLibraryOrder`, etc.) — OR better: register them from within a new function in the plugins package.

**Better approach:** Add a `RegisterLibraryRoutes` function to `pkg/plugins/routes.go`:

```go
// RegisterLibraryRoutes registers per-library plugin order routes on a libraries group.
func RegisterLibraryRoutes(g *echo.Group, service *Service, authMiddleware *auth.Middleware) {
	h := &handler{service: service}

	g.GET("/:id/plugins/order/:hookType", h.getLibraryOrder, authMiddleware.RequireLibraryAccess("id"))
	g.PUT("/:id/plugins/order/:hookType", h.setLibraryOrder, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
	g.DELETE("/:id/plugins/order/:hookType", h.resetLibraryOrder, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
	g.DELETE("/:id/plugins/order", h.resetAllLibraryOrders, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
}
```

Add the needed import for `auth` in `pkg/plugins/routes.go`:

```go
import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)
```

Then in `pkg/server/server.go`, after line 124:

```go
	plugins.RegisterLibraryRoutes(librariesGroup, pluginService, authMiddleware)
```

**Step 3: Add `GetPlugin` method to service if it doesn't exist**

Check if there's already a method to get a single plugin by scope/id. If not, add to `pkg/plugins/service.go`:

```go
// GetPlugin returns a single plugin by scope and ID.
func (s *Service) GetPlugin(ctx context.Context, scope, id string) (*models.Plugin, error) {
	plugin := new(models.Plugin)
	err := s.db.NewSelect().Model(plugin).
		Where("scope = ? AND id = ?", scope, id).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.WithStack(err)
	}
	return plugin, nil
}
```

**Step 4: Build to verify compilation**

Run: `make build`
Expected: Compiles cleanly

**Step 5: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/routes.go pkg/server/server.go pkg/plugins/service.go
git commit -m "[Plugins] Add API endpoints for per-library plugin order management"
```

---

### Task 6: Scan Pipeline - Pass LibraryID to GetOrderedRuntimes

**Files:**
- Modify: `pkg/worker/scan_unified.go`
- Modify: `pkg/worker/scan.go`

**Step 1: Update `runMetadataEnrichers` to accept and pass libraryID**

In `pkg/worker/scan_unified.go`, change the signature (line 2149):

```go
func (w *Worker) runMetadataEnrichers(ctx context.Context, metadata *mediafile.ParsedMetadata, file *models.File, book *models.Book, libraryID int) *mediafile.ParsedMetadata {
```

And update the `GetOrderedRuntimes` call (line 2154):

```go
	runtimes, err := w.pluginManager.GetOrderedRuntimes(ctx, models.PluginHookMetadataEnricher, libraryID)
```

**Step 2: Update callers of `runMetadataEnrichers` to pass libraryID**

Call site 1 — `scanFileByID` (line 251): The file has a `LibraryID` field.

```go
	metadata = w.runMetadataEnrichers(ctx, metadata, file, book, file.LibraryID)
```

Call site 2 — `scanFileCreateNew` (line 1795): The `opts.LibraryID` is available.

```go
	metadata = w.runMetadataEnrichers(ctx, metadata, file, book, opts.LibraryID)
```

**Step 3: Update `runInputConverters` to accept and pass libraryID**

In `pkg/worker/scan.go`, change the signature (line 447):

```go
func (w *Worker) runInputConverters(ctx context.Context, filesToScan []string, jobLog *joblogs.JobLogger, libraryID int) []string {
```

And update the `GetOrderedRuntimes` call (line 448):

```go
	runtimes, err := w.pluginManager.GetOrderedRuntimes(ctx, models.PluginHookInputConverter, libraryID)
```

**Step 4: Update the caller of `runInputConverters`**

In `pkg/worker/scan.go` line 328:

```go
			convertedFiles := w.runInputConverters(ctx, filesToScan, jobLog, library.ID)
```

**Step 5: Remove the temporary `, 0` added in Task 4 (if any remain)**

Check that no callers still pass `0` where library ID is available.

**Step 6: Build and run tests**

Run: `make build && go test ./pkg/worker/ -count=1 -timeout=120s`
Expected: Compiles and all tests pass

**Step 7: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/worker/scan.go
git commit -m "[Plugins] Pass libraryID through scan pipeline for per-library plugin order"
```

---

### Task 7: Frontend - Query Hooks for Library Plugin Order

**Files:**
- Modify: `app/hooks/queries/plugins.ts`

**Step 1: Add query and mutation hooks**

Add to `app/hooks/queries/plugins.ts`:

```typescript
// --- Per-Library Plugin Order ---

export interface LibraryPluginOrderPlugin {
  scope: string;
  id: string;
  name: string;
  enabled: boolean;
}

export interface LibraryPluginOrderResponse {
  customized: boolean;
  plugins: LibraryPluginOrderPlugin[];
}

export const useLibraryPluginOrder = (libraryId: string | undefined, hookType: string) => {
  return useQuery<LibraryPluginOrderResponse>({
    queryKey: ["libraries", libraryId, "plugins", "order", hookType],
    queryFn: async () => {
      const res = await api.get(`/libraries/${libraryId}/plugins/order/${hookType}`);
      return res.data;
    },
    enabled: !!libraryId,
  });
};

export const useSetLibraryPluginOrder = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      libraryId,
      hookType,
      plugins,
    }: {
      libraryId: string;
      hookType: string;
      plugins: { scope: string; id: string; enabled: boolean }[];
    }) => {
      await api.put(`/libraries/${libraryId}/plugins/order/${hookType}`, { plugins });
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["libraries", variables.libraryId, "plugins", "order"],
      });
    },
  });
};

export const useResetLibraryPluginOrder = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      libraryId,
      hookType,
    }: {
      libraryId: string;
      hookType: string;
    }) => {
      await api.delete(`/libraries/${libraryId}/plugins/order/${hookType}`);
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["libraries", variables.libraryId, "plugins", "order"],
      });
    },
  });
};

export const useResetAllLibraryPluginOrders = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ libraryId }: { libraryId: string }) => {
      await api.delete(`/libraries/${libraryId}/plugins/order`);
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["libraries", variables.libraryId, "plugins", "order"],
      });
    },
  });
};
```

**Step 2: Verify TypeScript compiles**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugins-sp && yarn lint:types`
Expected: No type errors

**Step 3: Commit**

```bash
git add app/hooks/queries/plugins.ts
git commit -m "[Frontend] Add query hooks for per-library plugin order"
```

---

### Task 8: Frontend - Library Plugin Order Tab Component

**Files:**
- Create: `app/components/library/LibraryPluginsTab.tsx`
- Modify: `app/components/pages/LibrarySettings.tsx`

**Step 1: Create the LibraryPluginsTab component**

Create `app/components/library/LibraryPluginsTab.tsx`:

```tsx
import { ArrowDown, ArrowUp, Loader2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  LibraryPluginOrderPlugin,
  useLibraryPluginOrder,
  useResetLibraryPluginOrder,
  useSetLibraryPluginOrder,
} from "@/hooks/queries/plugins";
import { PluginHookType } from "@/types/generated/models";

const HOOK_TYPES: { label: string; value: PluginHookType }[] = [
  { label: "Input Converter", value: "inputConverter" },
  { label: "File Parser", value: "fileParser" },
  { label: "Output Generator", value: "outputGenerator" },
  { label: "Metadata Enricher", value: "metadataEnricher" },
];

interface Props {
  libraryId: string;
}

const LibraryPluginsTab = ({ libraryId }: Props) => {
  const [selectedHookType, setSelectedHookType] =
    useState<PluginHookType>("metadataEnricher");
  const { data, isLoading, error } = useLibraryPluginOrder(
    libraryId,
    selectedHookType,
  );
  const setOrder = useSetLibraryPluginOrder();
  const resetOrder = useResetLibraryPluginOrder();

  const [localPlugins, setLocalPlugins] = useState<
    LibraryPluginOrderPlugin[] | null
  >(null);

  const displayPlugins = localPlugins ?? data?.plugins ?? [];
  const isCustomized = localPlugins !== null ? true : (data?.customized ?? false);

  const hasChanged =
    localPlugins !== null &&
    (localPlugins.length !== (data?.plugins?.length ?? 0) ||
      localPlugins.some(
        (item, i) =>
          item.scope !== data?.plugins?.[i]?.scope ||
          item.id !== data?.plugins?.[i]?.id ||
          item.enabled !== data?.plugins?.[i]?.enabled,
      ));

  const handleMove = (index: number, direction: "up" | "down") => {
    const newPlugins = [...displayPlugins];
    const targetIndex = direction === "up" ? index - 1 : index + 1;
    if (targetIndex < 0 || targetIndex >= newPlugins.length) return;
    [newPlugins[index], newPlugins[targetIndex]] = [
      newPlugins[targetIndex],
      newPlugins[index],
    ];
    setLocalPlugins(newPlugins);
  };

  const handleToggle = (index: number) => {
    const newPlugins = [...displayPlugins];
    newPlugins[index] = { ...newPlugins[index], enabled: !newPlugins[index].enabled };
    setLocalPlugins(newPlugins);
  };

  const handleCustomize = () => {
    // Copy global order into local state for editing
    setLocalPlugins([...(data?.plugins ?? [])]);
  };

  const handleSave = () => {
    setOrder.mutate(
      {
        libraryId,
        hookType: selectedHookType,
        plugins: displayPlugins.map((p) => ({
          scope: p.scope,
          id: p.id,
          enabled: p.enabled,
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

  const handleReset = () => {
    resetOrder.mutate(
      { libraryId, hookType: selectedHookType },
      {
        onSuccess: () => {
          setLocalPlugins(null);
          toast.success("Reset to global default.");
        },
        onError: (err) => {
          toast.error(`Failed to reset: ${err.message}`);
        },
      },
    );
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <p className="text-sm text-destructive">
        Failed to load plugin order: {error.message}
      </p>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <Label className="text-sm" htmlFor="lib-hook-type-select">
          Hook Type
        </Label>
        <Select
          onValueChange={(value) => {
            setSelectedHookType(value as PluginHookType);
            setLocalPlugins(null);
          }}
          value={selectedHookType}
        >
          <SelectTrigger className="w-[200px]" id="lib-hook-type-select">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {HOOK_TYPES.map((ht) => (
              <SelectItem key={ht.value} value={ht.value}>
                {ht.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {isCustomized && !localPlugins && (
          <Badge variant="secondary">Customized</Badge>
        )}
      </div>

      {displayPlugins.length === 0 ? (
        <div className="py-8 text-center">
          <p className="text-sm text-muted-foreground">
            No plugins registered for this hook type.
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {displayPlugins.map((plugin, index) => (
            <div
              className={`flex items-center justify-between gap-3 rounded-md border p-3 ${
                plugin.enabled ? "border-border" : "border-border/50 opacity-60"
              }`}
              key={`${plugin.scope}/${plugin.id}`}
            >
              <div className="flex items-center gap-3">
                <span className="text-xs font-mono text-muted-foreground">
                  {index + 1}
                </span>
                <span className="text-sm">{plugin.name}</span>
                <Badge variant="secondary">{plugin.scope}</Badge>
              </div>
              {(isCustomized || localPlugins) && (
                <div className="flex items-center gap-2">
                  <Switch
                    checked={plugin.enabled}
                    onCheckedChange={() => handleToggle(index)}
                  />
                  <Button
                    disabled={index === 0}
                    onClick={() => handleMove(index, "up")}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowUp className="h-4 w-4" />
                  </Button>
                  <Button
                    disabled={index === displayPlugins.length - 1}
                    onClick={() => handleMove(index, "down")}
                    size="sm"
                    variant="ghost"
                  >
                    <ArrowDown className="h-4 w-4" />
                  </Button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      <div className="flex gap-2">
        {!isCustomized && !localPlugins && displayPlugins.length > 0 && (
          <Button onClick={handleCustomize} size="sm" variant="outline">
            Customize
          </Button>
        )}
        {(isCustomized || localPlugins) && (
          <>
            <Button
              disabled={!hasChanged || setOrder.isPending}
              onClick={handleSave}
              size="sm"
            >
              {setOrder.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                "Save"
              )}
            </Button>
            <Button
              disabled={resetOrder.isPending}
              onClick={handleReset}
              size="sm"
              variant="outline"
            >
              Reset to Default
            </Button>
          </>
        )}
      </div>
    </div>
  );
};

export default LibraryPluginsTab;
```

**Step 2: Add "Plugins" section to LibrarySettings page**

In `app/components/pages/LibrarySettings.tsx`, add import:

```typescript
import LibraryPluginsTab from "@/components/library/LibraryPluginsTab";
```

Then add a new section before the Save button (before the `<Separator />` and save button at the bottom). Insert after the download format section (before the final `<Separator />`):

```tsx
        <Separator />

        {/* Per-Library Plugin Order */}
        <div className="space-y-4">
          <Label>Plugin Order</Label>
          <p className="text-sm text-muted-foreground">
            Customize which plugins run and in what order for this library.
            By default, the global plugin order is used.
          </p>
          {libraryId && <LibraryPluginsTab libraryId={libraryId} />}
        </div>
```

**Step 3: Verify frontend compiles**

Run: `yarn lint:types`
Expected: No type errors

**Step 4: Verify lint passes**

Run: `yarn lint`
Expected: No lint errors

**Step 5: Commit**

```bash
git add app/components/library/LibraryPluginsTab.tsx app/components/pages/LibrarySettings.tsx
git commit -m "[Frontend] Add per-library plugin order UI to library settings"
```

---

### Task 9: Integration Testing

**Files:**
- Create: `pkg/plugins/handler_library_test.go`

**Step 1: Write integration test for the API endpoints**

Create `pkg/plugins/handler_library_test.go`:

```go
package plugins_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibraryPluginOrder_GetDefault(t *testing.T) {
	db := testutils.SetupTestDB(t)
	svc := plugins.NewService(db)
	ctx := t.Context()

	// Setup: install plugin and set global order
	plugin := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)
	err = svc.SetOrder(ctx, "metadataEnricher", []models.PluginOrder{
		{Scope: "test", PluginID: "enricher1"},
	})
	require.NoError(t, err)

	// Create library
	lib := &models.Library{Name: "Test Library"}
	_, err = db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	// GET - should return global default
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "hookType")
	c.SetParamValues("1", "metadataEnricher")

	h := plugins.NewHandler(svc, nil, nil)
	err = h.GetLibraryOrder(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Customized bool `json:"customized"`
		Plugins    []struct {
			Scope   string `json:"scope"`
			ID      string `json:"id"`
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"plugins"`
	}
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.Customized)
	assert.Len(t, resp.Plugins, 1)
	assert.Equal(t, "enricher1", resp.Plugins[0].ID)
	assert.True(t, resp.Plugins[0].Enabled)
}

func TestLibraryPluginOrder_SetAndGet(t *testing.T) {
	db := testutils.SetupTestDB(t)
	svc := plugins.NewService(db)
	ctx := t.Context()

	// Setup
	p1 := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Enricher 1", Version: "1.0.0", Enabled: true}
	p2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Enricher 2", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(p1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(p2).Exec(ctx)
	require.NoError(t, err)

	lib := &models.Library{Name: "Test Library"}
	_, err = db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	// PUT - set custom order
	e := echo.New()
	payload := `{"plugins":[{"scope":"test","id":"enricher2","enabled":true},{"scope":"test","id":"enricher1","enabled":false}]}`
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "hookType")
	c.SetParamValues("1", "metadataEnricher")

	h := plugins.NewHandler(svc, nil, nil)
	err = h.SetLibraryOrder(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// GET - should return customized order
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("id", "hookType")
	c.SetParamValues("1", "metadataEnricher")

	err = h.GetLibraryOrder(c)
	require.NoError(t, err)

	var resp struct {
		Customized bool `json:"customized"`
		Plugins    []struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		} `json:"plugins"`
	}
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Customized)
	require.Len(t, resp.Plugins, 2)
	assert.Equal(t, "enricher2", resp.Plugins[0].ID)
	assert.True(t, resp.Plugins[0].Enabled)
	assert.Equal(t, "enricher1", resp.Plugins[1].ID)
	assert.False(t, resp.Plugins[1].Enabled)
}

func TestLibraryPluginOrder_Reset(t *testing.T) {
	db := testutils.SetupTestDB(t)
	svc := plugins.NewService(db)
	ctx := t.Context()

	// Setup
	plugin := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Test Enricher", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(plugin).Exec(ctx)
	require.NoError(t, err)

	lib := &models.Library{Name: "Test Library"}
	_, err = db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	// Set custom order
	err = svc.SetLibraryOrder(ctx, 1, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher1", Enabled: true},
	})
	require.NoError(t, err)

	// DELETE - reset
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "hookType")
	c.SetParamValues("1", "metadataEnricher")

	h := plugins.NewHandler(svc, nil, nil)
	err = h.ResetLibraryOrder(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify no longer customized
	customized, err := svc.IsLibraryCustomized(ctx, 1, "metadataEnricher")
	require.NoError(t, err)
	assert.False(t, customized)
}
```

**Note:** This test requires that handler methods be exported. Add to `pkg/plugins/handler.go`:

```go
// NewHandler creates a handler for testing and external route registration.
func NewHandler(service *Service, manager *Manager, installer *Installer) *handler {
	return &handler{service: service, manager: manager, installer: installer}
}

// Exported handler methods for testing
func (h *handler) GetLibraryOrder(c echo.Context) error  { return h.getLibraryOrder(c) }
func (h *handler) SetLibraryOrder(c echo.Context) error  { return h.setLibraryOrder(c) }
func (h *handler) ResetLibraryOrder(c echo.Context) error { return h.resetLibraryOrder(c) }
func (h *handler) ResetAllLibraryOrders(c echo.Context) error { return h.resetAllLibraryOrders(c) }
```

**Step 2: Run integration tests**

Run: `go test ./pkg/plugins/ -run "TestLibraryPluginOrder" -v`
Expected: All PASS

**Step 3: Run full test suite**

Run: `make check`
Expected: All checks pass

**Step 4: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/handler_library_test.go
git commit -m "[Plugins] Add integration tests for per-library plugin order API"
```

---

### Task 10: Update Design Doc

**Files:**
- Modify: `docs/plans/2026-01-24-per-library-plugin-config-design.md`

**Step 1: Add implementation status to the top of the design doc**

Add after the title:

```markdown
> **Status:** Implemented
```

**Step 2: Commit**

```bash
git add docs/plans/2026-01-24-per-library-plugin-config-design.md
git commit -m "[Docs] Mark per-library plugin config design as implemented"
```

---

## Summary of Changes

| Layer | Files | Description |
|-------|-------|-------------|
| Migration | `pkg/migrations/20260124000000_create_library_plugin_tables.go` | Two new tables |
| Models | `pkg/models/plugin.go` | Two new Bun model structs |
| Service | `pkg/plugins/service.go` | 5 new methods for library plugin order |
| Manager | `pkg/plugins/manager.go` | `GetOrderedRuntimes` gains `libraryID` param |
| Handler | `pkg/plugins/handler.go` | 4 new handler methods + exported wrappers |
| Routes | `pkg/plugins/routes.go` | `RegisterLibraryRoutes` function |
| Server | `pkg/server/server.go` | Wire library plugin routes |
| Worker | `pkg/worker/scan_unified.go`, `pkg/worker/scan.go` | Pass `libraryID` through pipeline |
| Frontend Hooks | `app/hooks/queries/plugins.ts` | 4 new query/mutation hooks |
| Frontend UI | `app/components/library/LibraryPluginsTab.tsx` | New component |
| Frontend Page | `app/components/pages/LibrarySettings.tsx` | Add plugins section |
| Tests | `pkg/plugins/service_library_test.go`, `pkg/plugins/handler_library_test.go` | Service + integration tests |
