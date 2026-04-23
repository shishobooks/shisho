# Surface plugin load errors in the UI

## Problem

When a user toggles a plugin on from the installed list or detail page and loading fails (e.g. bad manifest field, incompatible version), three things go wrong:

1. The `PATCH /plugins/installed/:scope/:id` endpoint returns **HTTP 200** with the plugin object even though load failed. The plugin row has `status: -2` (Malfunctioned) and a populated `load_error`, but the client treats the response as success and shows a "Plugin enabled" toast.
2. On the installed plugins list, every non-Active status (`Disabled`, `Malfunctioned`, `NotSupported`) renders the same generic `Disabled` badge with a dimmed row. Users can't distinguish "I turned this off" from "this is broken".
3. The plugin detail page does not render `load_error` anywhere, so there is no way to see *why* a plugin is broken without inspecting the raw JSON response.

## Goals

- Enabling a broken plugin shows the actual error to the user immediately (toast).
- The installed-plugins list visibly distinguishes Malfunctioned / NotSupported from a user-disabled plugin.
- The detail page shows the full `load_error` text verbatim when one is present, so the user can act on it (fix manifest, reinstall, etc.).

## Non-goals

- No changes to the startup `LoadAll` path or the scan/install flows. Those already persist `LoadError` to the DB; the new UI picks them up automatically.
- No new status codes or fields on the `Plugin` model. `status` and `load_error` are sufficient.
- No retry/auto-heal logic. The existing switch already lets the user toggle off and back on after fixing the plugin.

## Design

### Backend — `pkg/plugins/handler.go`

In `handler.update`, the `enable: true` branch currently swallows the load error:

```go
if err := h.manager.LoadPlugin(ctx, scope, id); err != nil {
    errMsg := err.Error()
    plugin.LoadError = &errMsg
    // ... set Status, emit event ...
}
// ... later ...
return c.JSON(http.StatusOK, plugin)
```

Change this so that:

1. The plugin row is still persisted with its Malfunctioned/NotSupported status and `LoadError` (no regression — this is what allows the list to show the badge on refetch).
2. After persisting, if load failed, the handler returns `errcodes.ValidationError(loadErr.Error())`, which maps to HTTP 422 with body `{ error: { code: "validation_error", message: <loadErr> } }` via the existing `errcodes` handler. This is the same helper used a few lines below for confidence-threshold validation, so it fits the file's conventions.

This only affects the user-initiated enable path. Disable, config updates, and auto-update continue to return 200.

### Frontend — installed list

**`app/components/plugins/PluginRow.tsx`**

Replace the single `disabled` boolean prop with a richer status prop:

- New prop `status?: PluginStatus` (optional so the Discover tab, which lists not-installed plugins, doesn't need it).
- Map status → badge:
  - `PluginStatusActive` → no badge (current behavior for enabled rows)
  - `PluginStatusDisabled` → `<Badge variant="secondary">Disabled</Badge>` (current behavior)
  - `PluginStatusMalfunctioned` → `<Badge variant="destructive">Error</Badge>` with `title={load_error}` so hovering reveals the message
  - `PluginStatusNotSupported` → `<Badge variant="outline">Incompatible</Badge>`
- Keep the row dimming (`opacity-50 saturate-50`) for any non-Active status.

**`app/components/plugins/InstalledTab.tsx`**

Pass `status` and (when malfunctioned) `loadError` into `PluginRow`. The existing sort that groups disabled-after-enabled stays — malfunctioned rows sort with the disabled group.

### Frontend — plugin detail

**`app/components/plugins/PluginDetailHero.tsx`**

When `installed?.load_error` is truthy **or** status is Malfunctioned/NotSupported, render an alert block under the title row (above the meta line):

```tsx
{(installed.status === PluginStatusMalfunctioned ||
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
```

The enable switch remains in its current position so the user can toggle off and back on after fixing the underlying issue.

**`app/components/pages/PluginDetail.tsx` — `handleToggleEnabled`**

No change needed. The mutation's `catch` block already shows `err.message` via `toast.error`; the backend change feeds it the real load error.

## Testing

- **Backend unit test** (`pkg/plugins/handler_test.go` — add a case): toggling enable on a plugin whose manifest has an invalid enricher field returns a 422-class error, and a follow-up `GET /plugins/installed` shows the row with `status: -2` and populated `load_error`.
- **Frontend component tests**:
  - `PluginRow.test.tsx`: one case per status renders the expected badge text; `Malfunctioned` has the hover title attribute.
  - `PluginDetailHero.test.tsx`: renders the alert block when `load_error` is set; renders the "not compatible" copy when status is `NotSupported` without a load_error; does not render when status is Active.
- **Manual verification**: enable the broken `hmanga Enricher` plugin from the screenshot; confirm the toast shows the real error, the installed list row shows a red `Error` badge with hover title, and the detail page renders the error block.

## Docs

No user-facing doc changes. The plugin system's user docs don't currently describe plugin states granularly enough for this to warrant a new section, and the behavior is self-explanatory in the UI.
