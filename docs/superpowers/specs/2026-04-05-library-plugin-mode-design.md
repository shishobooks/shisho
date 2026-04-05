# Library Plugin Mode: Three-State Plugin Control

**Date:** 2026-04-05
**Status:** Approved
**Scope:** `pkg/models/`, `pkg/plugins/`, `app/components/library/`, `app/hooks/queries/`, database migration

## Summary

Replace the binary enabled/disabled toggle on per-library (and global) plugin order with a three-state mode control: **Enabled**, **Manual Only**, and **Disabled**. This clarifies the distinction between automated scan enrichment and manual identification availability, giving users granular control over how each plugin operates per hook type.

Currently the toggle only affects automated scans but the UI doesn't communicate this. Manual identification always uses the global order, ignoring per-library settings entirely.

---

## Problem

1. The current toggle in the library plugin order UI is ambiguous — toggling a plugin off looks like it fully disables the plugin, but it only affects automated scans. Manual identification ignores per-library settings (uses `libraryID=0`).
2. There's no way to say "I want this enricher available for manual identification but not running on every scan."
3. The global plugin order has no enabled/disabled concept at all — all plugins in the global order are always active.

## States

Three modes per plugin, per hook type:

| Mode | Auto-Scan | Manual Identification |
|------|-----------|----------------------|
| **Enabled** | Runs in configured order | Available |
| **Manual Only** | Skipped | Available |
| **Disabled** | Skipped | Unavailable |

- "Manual Only" is only meaningful for `metadataEnricher` hook type. Other hook types (inputConverter, fileParser, outputGenerator) only support Enabled/Disabled.
- Per-library settings override global. When not customized, libraries inherit the global state.

---

## 1. Database Changes

### Table Renames

| Old Name | New Name |
|----------|----------|
| `plugin_order` | `plugin_hook_config` |
| `library_plugins` | `library_plugin_hook_config` |

### Schema Changes

**`plugin_hook_config`** (renamed from `plugin_order`):
- Add `mode TEXT NOT NULL DEFAULT 'enabled'`
- Valid values: `enabled`, `manual_only`, `disabled`
- All existing rows get `'enabled'`

**`library_plugin_hook_config`** (renamed from `library_plugins`):
- Add `mode TEXT NOT NULL DEFAULT 'enabled'`
- Migrate existing data: `enabled=true` → `mode='enabled'`, `enabled=false` → `mode='disabled'`
- Drop `enabled` column

### Migration

Single migration that:
1. Renames `plugin_order` → `plugin_hook_config`, adds `mode` column defaulting to `'enabled'`
2. Renames `library_plugins` → `library_plugin_hook_config`, adds `mode` column, migrates from `enabled` bool, drops `enabled` column
3. Rollback reverses all changes

---

## 2. Backend Model Changes

### `pkg/models/plugin.go`

Update model structs:

```go
// PluginOrder → PluginHookConfig
type PluginHookConfig struct {
    bun.BaseModel `bun:"table:plugin_hook_config,alias:phc"`
    HookType      string `bun:"hook_type,pk" json:"hook_type"`
    Scope         string `bun:"scope,pk" json:"scope"`
    PluginID      string `bun:"plugin_id,pk" json:"plugin_id"`
    Position      int    `bun:"position" json:"position"`
    Mode          string `bun:"mode" json:"mode"`
}

// LibraryPlugin → LibraryPluginHookConfig
type LibraryPluginHookConfig struct {
    bun.BaseModel `bun:"table:library_plugin_hook_config,alias:lphc"`
    LibraryID     int    `bun:"library_id,pk" json:"library_id"`
    HookType      string `bun:"hook_type,pk" json:"hook_type"`
    Scope         string `bun:"scope,pk" json:"scope"`
    PluginID      string `bun:"plugin_id,pk" json:"plugin_id"`
    Position      int    `bun:"position" json:"position"`
    Mode          string `bun:"mode" json:"mode"`
}
```

Add mode constants:

```go
const (
    PluginModeEnabled    = "enabled"
    PluginModeManualOnly = "manual_only"
    PluginModeDisabled   = "disabled"
)
```

---

## 3. Backend Service/Manager Changes

### `pkg/plugins/service.go`

Update all queries referencing old table names/models to use the renamed structs. No logic changes beyond the rename and replacing `Enabled bool` references with `Mode string`.

### `pkg/plugins/manager.go`

**Existing `GetOrderedRuntimes`** — filter changes from `!entry.Enabled` to `entry.Mode != PluginModeEnabled`:

```go
// For auto-scan: only mode="enabled" plugins run
for _, entry := range entries {
    if entry.Mode != models.PluginModeEnabled {
        continue
    }
    // ...
}
```

For the global fallback path, apply the same filter (currently returns all plugins unconditionally).

**New `GetManualRuntimes`** — returns plugins with mode `enabled` OR `manual_only`:

```go
func (m *Manager) GetManualRuntimes(ctx context.Context, hookType string, libraryID int) ([]*Runtime, error) {
    // Same structure as GetOrderedRuntimes but filters:
    // entry.Mode == "enabled" || entry.Mode == "manual_only"
}
```

### `pkg/plugins/handler.go`

**Manual identification handler** (~line 1301):
- Change `GetOrderedRuntimes(ctx, models.PluginHookMetadataEnricher, 0)` to `GetManualRuntimes(ctx, models.PluginHookMetadataEnricher, book.LibraryID)`
- This makes manual search respect per-library plugin settings

### API Payload Changes

**Global order endpoints** (`GET/PUT /plugins/order/:hookType`):
- Response/request includes `mode` field per plugin entry
- Replace any `enabled` field with `mode`

**Per-library order endpoints** (`GET/PUT /libraries/:libraryId/plugins/order/:hookType`):
- Response/request replaces `enabled: bool` with `mode: string`

---

## 4. Frontend Changes

### `app/hooks/queries/plugins.ts`

Update `LibraryPluginOrderPlugin` interface:

```typescript
interface LibraryPluginOrderPlugin {
  scope: string;
  id: string;
  name: string;
  mode: "enabled" | "manual_only" | "disabled";  // replaces `enabled: boolean`
}
```

Update mutations to send `mode` instead of `enabled`.

### `app/components/library/LibraryPluginsTab.tsx`

**Replace the toggle switch with a dropdown/select per plugin row:**

- For `metadataEnricher` hook type: three options — Enabled, Manual Only, Disabled
- For other hook types: two options — Enabled, Disabled

**Visual treatment:**
- **Enabled** — full opacity (as today when toggled on)
- **Manual Only** — slightly muted style to distinguish from fully enabled
- **Disabled** — reduced opacity (as today when toggled off)

### `app/components/pages/AdminPlugins.tsx` (Global Plugin Order)

Apply the same dropdown control to the global plugin order section in the admin plugins page. Currently the global order has no toggle at all — add the mode dropdown per plugin row, consistent with the per-library UI.

---

## 5. Documentation

Update `website/docs/plugins/` to document the three modes and their behavior for both global and per-library configuration.

---

## Out of Scope

- Changing the `library_plugin_customizations` table (keeps tracking whether a library has customized the order for a hook type)
- Changing `plugin_field_settings` or `library_plugin_field_settings` (field-level enable/disable is orthogonal to this)
- Adding mode to `plugins` table itself (global enable/disable of the plugin installation is a separate concern)
