# Plugin Min-Version Enforcement

## Problem

The `minShishoVersion` field exists in both `repository.json` plugin versions and plugin manifests, but it is only checked at runtime when loading an already-installed plugin (`runtime.go`). The three critical paths that interact with repository data — browsing available plugins, installing plugins, and checking for updates — all ignore `minShishoVersion`. This means:

1. Users can download and install plugins that are guaranteed to fail on load
2. The Browse tab gives no indication that a plugin/version is incompatible
3. Update checks can flag versions the user can't actually run

## Design

### Backend

#### 1. Version-aware filtering in `findPluginInRepos` and `CheckForUpdates`

Both `findPluginInRepos()` (used for install and update) and `CheckForUpdates()` currently call `FilterCompatibleVersions()` which only checks `manifestVersion`. After this filter, add a second filter using `version.IsCompatible(v.MinShishoVersion)` to exclude versions whose `minShishoVersion` exceeds the running Shisho version.

This prevents installation or update to an incompatible version.

#### 2. Compatibility annotations in `listAvailable` / `getAvailable` responses

The Browse tab should show ALL versions (not filter them out) but annotate each with compatibility status. Introduce a response wrapper:

```go
type availablePluginVersionResponse struct {
    PluginVersion
    Compatible bool `json:"compatible"`
}
```

And add a top-level `Compatible` field to `availablePluginResponse`:

```go
type availablePluginResponse struct {
    Scope       string                          `json:"scope"`
    ID          string                          `json:"id"`
    Name        string                          `json:"name"`
    Overview    string                          `json:"overview"`
    Description string                          `json:"description"`
    Author      string                          `json:"author"`
    Homepage    string                          `json:"homepage"`
    Versions    []availablePluginVersionResponse `json:"versions"`
    Compatible  bool                            `json:"compatible"`
}
```

`Compatible` on the plugin level is `true` if at least one version has `compatible: true`. Each version's `compatible` field is set by calling `version.IsCompatible(v.MinShishoVersion)`.

Both `listAvailable` and `getAvailable` handlers use this response format.

The `overview` field is also added to the response (it was previously missing from `availablePluginResponse` despite being present on `AvailablePlugin`).

#### 3. Install endpoint — no payload changes needed

The frontend's install flow always goes through the `findPluginInRepos` path (scope+id lookup), which gets minShishoVersion filtering from point 1. The `download_url`/`sha256` fields in `installPayload` exist for direct-URL installs but the Browse UI doesn't use them. The runtime version check in `LoadPlugin()` remains as a safety net for any direct-URL installs.

### Frontend

#### 4. TypeScript type updates

Update `PluginVersion` interface to include `compatible`:

```typescript
export interface PluginVersion {
  version: string;
  minShishoVersion: string;
  compatible: boolean;
  changelog: string;
  downloadUrl: string;
  sha256: string;
  manifestVersion: number;
  releaseDate: string;
}
```

Update `AvailablePlugin` to include `compatible`:

```typescript
export interface AvailablePlugin {
  // ...existing fields...
  compatible: boolean;
}
```

#### 5. BrowseTab UI changes

**Fully incompatible plugins** (`plugin.compatible === false`):
- Show an "Incompatible" badge (destructive variant) where the Install button would be
- Add explanatory text: "Requires a newer version of Shisho"

**Partially compatible plugins** (some versions compatible, some not):
- Show the Install button as normal — the install flow will pick the latest compatible version
- The version badge next to the name shows the latest compatible version (first version where `compatible === true`)

#### 6. Install flow: pick latest compatible version

`handleInstall` in `BrowseTab` currently picks `versions[0]`. Change to pick the first version where `compatible === true`. The `CapabilitiesWarning` dialog also receives the correct compatible version.

### What's NOT changing

- The runtime version check in `LoadPlugin()` stays as-is — it's a safety net for manually installed plugins or plugins installed before this feature
- `FilterCompatibleVersions()` keeps its current behavior (manifest version only) — it's a separate concern
- No new API endpoints — all changes are to existing response shapes and filtering logic

## Testing

### Backend tests

- `FilterCompatibleVersions` behavior unchanged (existing tests pass)
- `findPluginInRepos` with mixed compatible/incompatible versions: returns only compatible
- `findPluginInRepos` with all incompatible versions: returns not-found error
- `CheckForUpdates` skips incompatible versions when determining latest update
- `listAvailable` response includes `compatible` field on both plugin and version levels
- Install via `findPluginInRepos` skips incompatible versions

### Frontend

- BrowseTab renders "Incompatible" badge when `compatible === false`
- BrowseTab renders Install button when `compatible === true`
- Install handler selects first compatible version, not `versions[0]`

## Files to modify

### Backend
- `pkg/plugins/repository.go` — no changes to `FilterCompatibleVersions`, but may add a helper
- `pkg/plugins/handler.go` — `listAvailable`, `getAvailable`, `findPluginInRepos`, `install` handlers; `availablePluginResponse` type
- `pkg/plugins/manager.go` — `CheckForUpdates` filtering

### Frontend
- `app/hooks/queries/plugins.ts` — `PluginVersion` and `AvailablePlugin` types
- `app/components/pages/AdminPlugins.tsx` — `BrowseTab` component

### Tests
- `pkg/plugins/handler_test.go` or relevant test files
- `pkg/plugins/manager_test.go` — `CheckForUpdates` tests
- `pkg/plugins/update_check_test.go` — if update check tests live here
