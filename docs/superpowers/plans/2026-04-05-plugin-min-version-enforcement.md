# Plugin Min-Version Enforcement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce `minShishoVersion` from repository.json so incompatible plugin versions can't be installed, and incompatible plugins are clearly marked in the UI.

**Architecture:** Add `version.IsCompatible()` filtering to the three backend paths that use repository data (browse, install, update-check). Annotate available plugin API responses with a `compatible` boolean on both plugin and version levels. Update the BrowseTab frontend to show incompatibility state and prevent install of incompatible plugins.

**Tech Stack:** Go (Echo handlers, version package), React/TypeScript (Tanstack Query hooks, shadcn/ui components)

---

### Task 1: Add `minShishoVersion` filtering to `findPluginInRepos`

This is the install/update path — must block incompatible versions.

**Files:**
- Modify: `pkg/plugins/repository.go:100-113`
- Test: `pkg/plugins/repository_test.go`

- [ ] **Step 1: Write test for new `FilterVersionCompatibleVersions` function**

In `pkg/plugins/repository_test.go`, add:

```go
func TestFilterVersionCompatibleVersions(t *testing.T) {
	t.Parallel()

	versions := []PluginVersion{
		{Version: "1.0.0", MinShishoVersion: "0.1.0", ManifestVersion: 1},
		{Version: "2.0.0", MinShishoVersion: "99.0.0", ManifestVersion: 1},
		{Version: "1.1.0", MinShishoVersion: "", ManifestVersion: 1},
	}

	compatible := FilterVersionCompatibleVersions(versions)
	require.Len(t, compatible, 2)
	assert.Equal(t, "1.0.0", compatible[0].Version)
	assert.Equal(t, "1.1.0", compatible[1].Version)
}

func TestFilterVersionCompatibleVersions_AllIncompatible(t *testing.T) {
	t.Parallel()

	versions := []PluginVersion{
		{Version: "1.0.0", MinShishoVersion: "99.0.0", ManifestVersion: 1},
		{Version: "2.0.0", MinShishoVersion: "100.0.0", ManifestVersion: 1},
	}

	compatible := FilterVersionCompatibleVersions(versions)
	assert.Empty(t, compatible)
}

func TestFilterVersionCompatibleVersions_Empty(t *testing.T) {
	t.Parallel()

	compatible := FilterVersionCompatibleVersions(nil)
	assert.Empty(t, compatible)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -run TestFilterVersionCompatibleVersions -v`
Expected: FAIL — `FilterVersionCompatibleVersions` undefined

- [ ] **Step 3: Implement `FilterVersionCompatibleVersions`**

In `pkg/plugins/repository.go`, add after `FilterCompatibleVersions`:

```go
// FilterVersionCompatibleVersions returns only versions whose minShishoVersion
// is satisfied by the running Shisho version.
func FilterVersionCompatibleVersions(versions []PluginVersion) []PluginVersion {
	var compatible []PluginVersion
	for _, v := range versions {
		if version.IsCompatible(v.MinShishoVersion) {
			compatible = append(compatible, v)
		}
	}
	return compatible
}
```

Add `"github.com/shishobooks/shisho/pkg/version"` to the imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -run TestFilterVersionCompatibleVersions -v`
Expected: PASS (all 3 tests). Note: in dev mode `version.Version` is `"dev"`, which `IsCompatible` treats as always compatible. The test for "99.0.0" works because `isVersionCompatible("dev", "99.0.0")` returns true. We need to override `version.Version` for the incompatible tests.

- [ ] **Step 5: Fix tests — set `version.Version` to a real version**

Update the two tests that check filtering:

```go
func TestFilterVersionCompatibleVersions(t *testing.T) {
	t.Parallel()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	versions := []PluginVersion{
		{Version: "1.0.0", MinShishoVersion: "0.1.0", ManifestVersion: 1},
		{Version: "2.0.0", MinShishoVersion: "99.0.0", ManifestVersion: 1},
		{Version: "1.1.0", MinShishoVersion: "", ManifestVersion: 1},
	}

	compatible := FilterVersionCompatibleVersions(versions)
	require.Len(t, compatible, 2)
	assert.Equal(t, "1.0.0", compatible[0].Version)
	assert.Equal(t, "1.1.0", compatible[1].Version)
}

func TestFilterVersionCompatibleVersions_AllIncompatible(t *testing.T) {
	t.Parallel()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	versions := []PluginVersion{
		{Version: "1.0.0", MinShishoVersion: "99.0.0", ManifestVersion: 1},
		{Version: "2.0.0", MinShishoVersion: "100.0.0", ManifestVersion: 1},
	}

	compatible := FilterVersionCompatibleVersions(versions)
	assert.Empty(t, compatible)
}
```

`TestFilterVersionCompatibleVersions_Empty` doesn't need the override since there are no versions to check.

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -run TestFilterVersionCompatibleVersions -v`
Expected: PASS (all 3 tests)

- [ ] **Step 7: Apply `FilterVersionCompatibleVersions` in `findPluginInRepos`**

In `pkg/plugins/handler.go`, in the `findPluginInRepos` method (around line 301), change:

```go
			compatible := FilterCompatibleVersions(p.Versions)
```

to:

```go
			compatible := FilterVersionCompatibleVersions(FilterCompatibleVersions(p.Versions))
```

- [ ] **Step 8: Run full plugin test suite**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -v -count=1`
Expected: All existing tests pass

- [ ] **Step 9: Commit**

```bash
git add pkg/plugins/repository.go pkg/plugins/repository_test.go pkg/plugins/handler.go
git commit -m "[Backend] Add minShishoVersion filtering to plugin install/update path"
```

---

### Task 2: Add `minShishoVersion` filtering to `CheckForUpdates`

**Files:**
- Modify: `pkg/plugins/manager.go:461-488`
- Test: `pkg/plugins/update_check_test.go`

- [ ] **Step 1: Write test for minShishoVersion filtering in CheckForUpdates**

In `pkg/plugins/update_check_test.go`, add:

```go
func TestManager_CheckForUpdates_MinShishoVersionFiltered(t *testing.T) {
	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	db := setupTestDB(t)
	service := NewService(db)
	mgr := NewManager(service, t.TempDir(), "")
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "official",
		ID:          "my-plugin",
		Name:        "My Plugin",
		Version:     "1.0.0",
		Status:      models.PluginStatusActive,
		InstalledAt: time.Now(),
	}
	err := service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	repo := &models.PluginRepository{
		URL:        "https://raw.githubusercontent.com/test/repo/main/manifest.json",
		Scope:      "official",
		Name:       strPtr("Official Repo"),
		IsOfficial: true,
		Enabled:    true,
	}
	err = service.AddRepository(ctx, repo)
	require.NoError(t, err)

	mgr.fetchRepo = func(_ string) (*RepositoryManifest, error) {
		return &RepositoryManifest{
			RepositoryVersion: 1,
			Scope:             "official",
			Name:              "Official Repo",
			Plugins: []AvailablePlugin{
				{
					ID:   "my-plugin",
					Name: "My Plugin",
					Versions: []PluginVersion{
						{Version: "1.0.0", ManifestVersion: 1, MinShishoVersion: "0.1.0"},
						{Version: "1.5.0", ManifestVersion: 1, MinShishoVersion: "0.5.0"},
						{Version: "2.0.0", ManifestVersion: 1, MinShishoVersion: "99.0.0"},
					},
				},
			},
		}, nil
	}

	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// Should flag 1.5.0 as available, not 2.0.0 (incompatible minShishoVersion)
	updated, err := service.RetrievePlugin(ctx, "official", "my-plugin")
	require.NoError(t, err)
	require.NotNil(t, updated.UpdateAvailableVersion)
	assert.Equal(t, "1.5.0", *updated.UpdateAvailableVersion)
}
```

Add `"github.com/shishobooks/shisho/pkg/version"` to the imports in `update_check_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -run TestManager_CheckForUpdates_MinShishoVersionFiltered -v`
Expected: FAIL — asserts `1.5.0` but gets `2.0.0`

- [ ] **Step 3: Add version filtering to `CheckForUpdates`**

In `pkg/plugins/manager.go`, in the `CheckForUpdates` method (around line 478), change:

```go
			compatible := FilterCompatibleVersions(available.Versions)
```

to:

```go
			compatible := FilterVersionCompatibleVersions(FilterCompatibleVersions(available.Versions))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -run TestManager_CheckForUpdates_MinShishoVersionFiltered -v`
Expected: PASS

- [ ] **Step 5: Run full update check tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -run TestManager_CheckForUpdates -v`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add pkg/plugins/manager.go pkg/plugins/update_check_test.go
git commit -m "[Backend] Filter incompatible minShishoVersion in update checks"
```

---

### Task 3: Add compatibility annotations to available plugins API response

**Files:**
- Modify: `pkg/plugins/handler.go:649-704` and `772-817`
- Test: `pkg/plugins/repository_test.go` (unit test for the annotation helper)

- [ ] **Step 1: Write test for `annotateVersionCompatibility` helper**

In `pkg/plugins/repository_test.go`, add:

```go
func TestAnnotateVersionCompatibility(t *testing.T) {
	t.Parallel()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	versions := []PluginVersion{
		{Version: "1.0.0", MinShishoVersion: "0.5.0", ManifestVersion: 1},
		{Version: "2.0.0", MinShishoVersion: "99.0.0", ManifestVersion: 1},
		{Version: "1.1.0", MinShishoVersion: "", ManifestVersion: 1},
	}

	annotated := AnnotateVersionCompatibility(versions)
	require.Len(t, annotated, 3)

	assert.True(t, annotated[0].Compatible)   // 0.5.0 <= 1.0.0
	assert.False(t, annotated[1].Compatible)   // 99.0.0 > 1.0.0
	assert.True(t, annotated[2].Compatible)    // empty = compatible

	// Verify original fields preserved
	assert.Equal(t, "1.0.0", annotated[0].Version)
	assert.Equal(t, "2.0.0", annotated[1].Version)
	assert.Equal(t, "1.1.0", annotated[2].Version)
}

func TestAnnotateVersionCompatibility_AllIncompatible(t *testing.T) {
	t.Parallel()

	origVersion := version.Version
	version.Version = "1.0.0"
	defer func() { version.Version = origVersion }()

	versions := []PluginVersion{
		{Version: "1.0.0", MinShishoVersion: "99.0.0", ManifestVersion: 1},
	}

	annotated := AnnotateVersionCompatibility(versions)
	require.Len(t, annotated, 1)
	assert.False(t, annotated[0].Compatible)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -run TestAnnotateVersionCompatibility -v`
Expected: FAIL — `AnnotateVersionCompatibility` undefined

- [ ] **Step 3: Add `AnnotatedPluginVersion` type and `AnnotateVersionCompatibility` function**

In `pkg/plugins/repository.go`, add:

```go
// AnnotatedPluginVersion extends PluginVersion with a compatibility flag.
type AnnotatedPluginVersion struct {
	PluginVersion
	Compatible bool `json:"compatible"`
}

// AnnotateVersionCompatibility annotates each version with whether it is
// compatible with the running Shisho version based on minShishoVersion.
func AnnotateVersionCompatibility(versions []PluginVersion) []AnnotatedPluginVersion {
	result := make([]AnnotatedPluginVersion, len(versions))
	for i, v := range versions {
		result[i] = AnnotatedPluginVersion{
			PluginVersion: v,
			Compatible:    version.IsCompatible(v.MinShishoVersion),
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -run TestAnnotateVersionCompatibility -v`
Expected: PASS

- [ ] **Step 5: Update `availablePluginResponse` to use annotated versions**

In `pkg/plugins/handler.go`, change the `availablePluginResponse` struct:

```go
type availablePluginResponse struct {
	Scope       string                    `json:"scope"`
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Overview    string                    `json:"overview"`
	Description string                    `json:"description"`
	Author      string                    `json:"author"`
	Homepage    string                    `json:"homepage"`
	Versions    []AnnotatedPluginVersion  `json:"versions"`
	Compatible  bool                      `json:"compatible"`
}
```

- [ ] **Step 6: Update `listAvailable` handler to annotate versions**

In `pkg/plugins/handler.go`, update the `listAvailable` method. Change the plugin loop body (around line 681-696):

```go
		for _, p := range manifest.Plugins {
			compatible := FilterCompatibleVersions(p.Versions)
			if len(compatible) == 0 {
				continue
			}

			annotated := AnnotateVersionCompatibility(compatible)
			hasCompatible := false
			for _, v := range annotated {
				if v.Compatible {
					hasCompatible = true
					break
				}
			}

			result = append(result, availablePluginResponse{
				Scope:       manifest.Scope,
				ID:          p.ID,
				Name:        p.Name,
				Overview:    p.Overview,
				Description: p.Description,
				Author:      p.Author,
				Homepage:    p.Homepage,
				Versions:    annotated,
				Compatible:  hasCompatible,
			})
		}
```

- [ ] **Step 7: Update `retrieveAvailable` handler the same way**

In `pkg/plugins/handler.go`, update the `retrieveAvailable` method (around line 799-812):

```go
			compatible := FilterCompatibleVersions(p.Versions)
			if len(compatible) == 0 {
				continue
			}

			annotated := AnnotateVersionCompatibility(compatible)
			hasCompatible := false
			for _, v := range annotated {
				if v.Compatible {
					hasCompatible = true
					break
				}
			}

			return errors.WithStack(c.JSON(http.StatusOK, availablePluginResponse{
				Scope:       manifest.Scope,
				ID:          p.ID,
				Name:        p.Name,
				Overview:    p.Overview,
				Description: p.Description,
				Author:      p.Author,
				Homepage:    p.Homepage,
				Versions:    annotated,
				Compatible:  hasCompatible,
			}))
```

- [ ] **Step 8: Run full plugin test suite**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -v -count=1`
Expected: All tests pass

- [ ] **Step 9: Commit**

```bash
git add pkg/plugins/repository.go pkg/plugins/repository_test.go pkg/plugins/handler.go
git commit -m "[Backend] Add compatibility annotations to available plugins API"
```

---

### Task 4: Update frontend types and BrowseTab UI

**Files:**
- Modify: `app/hooks/queries/plugins.ts:27-46`
- Modify: `app/components/pages/AdminPlugins.tsx:317-441`

- [ ] **Step 1: Update `PluginVersion` and `AvailablePlugin` TypeScript interfaces**

In `app/hooks/queries/plugins.ts`, change the `PluginVersion` interface to match the actual JSON field names from the server (the current snake_case names are wrong — the server sends camelCase):

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

Update `AvailablePlugin` to add `compatible`:

```typescript
export interface AvailablePlugin {
  scope: string;
  id: string;
  name: string;
  overview: string;
  description: string;
  author: string;
  homepage: string;
  imageUrl: string;
  versions: PluginVersion[];
  compatible: boolean;
}
```

- [ ] **Step 2: Update `InstallPluginPayload` to match**

In the same file, the `InstallPluginPayload` uses `download_url` which is what the *install endpoint* expects (snake_case, since the Go `installPayload` struct uses `json:"download_url"`). This is correct — it's a different struct from `PluginVersion`. Leave it as-is.

- [ ] **Step 3: Update `CapabilitiesWarning` to use new field names**

In `app/components/plugins/CapabilitiesWarning.tsx`, update the reference to `manifest_version` (line 104):

```tsx
            Manifest v{latestVersion.manifestVersion}
```

- [ ] **Step 4: Update `BrowseTab` to show compatibility state**

In `app/components/pages/AdminPlugins.tsx`, add `AlertTriangle` to the lucide imports:

```typescript
import {
  AlertTriangle,
  ArrowDown,
  // ... rest of existing imports
```

Then update the `BrowseTab` component. Replace the `handleInstall` function:

```typescript
  const handleInstall = () => {
    if (!installTarget) return;
    const compatibleVersion = installTarget.versions.find((v) => v.compatible);
    if (!compatibleVersion) return;
    installPlugin.mutate(
      {
        scope: installTarget.scope,
        id: installTarget.id,
        name: installTarget.name,
        version: compatibleVersion.version,
      },
      { onSuccess: () => setInstallTarget(null) },
    );
  };
```

Note: we no longer pass `download_url` and `sha256` — the backend's `findPluginInRepos` resolves those. This was already the effective behavior since the old field names (`download_url`, `sha256`) didn't match the JSON (`downloadUrl`, `sha256`) and resolved to `undefined`.

Update the plugin card rendering inside the `available.map()` callback. Replace the existing return block (the `<div>` with key):

```tsx
          return (
            <div
              className="flex items-start justify-between gap-4 rounded-md border border-border p-4"
              key={`${plugin.scope}/${plugin.id}`}
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <h3 className="text-sm font-medium">{plugin.name}</h3>
                  {latestVersion && (
                    <Badge variant="outline">{latestVersion.version}</Badge>
                  )}
                  <Badge variant="secondary">{plugin.scope}</Badge>
                  {alreadyInstalled && (
                    <Badge variant="subtle">Installed</Badge>
                  )}
                  {!plugin.compatible && (
                    <Badge variant="destructive">Incompatible</Badge>
                  )}
                </div>
                {plugin.description && (
                  <p className="mt-1 text-xs text-muted-foreground">
                    {plugin.description}
                  </p>
                )}
                {plugin.author && (
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    by {plugin.author}
                  </p>
                )}
                {!plugin.compatible && (
                  <p className="mt-1 flex items-center gap-1 text-xs text-destructive">
                    <AlertTriangle className="h-3 w-3" />
                    Requires a newer version of Shisho
                  </p>
                )}
              </div>

              <div className="flex shrink-0 items-center gap-2">
                {canWrite && !alreadyInstalled && plugin.compatible && (
                  <Button
                    onClick={() => setInstallTarget(plugin)}
                    size="sm"
                    variant="outline"
                  >
                    Install
                  </Button>
                )}
                {plugin.homepage && (
                  <a
                    className="text-muted-foreground hover:text-foreground"
                    href={plugin.homepage}
                    rel="noopener noreferrer"
                    target="_blank"
                  >
                    <ExternalLink className="h-4 w-4" />
                  </a>
                )}
              </div>
            </div>
          );
```

Key changes:
- Show "Incompatible" destructive badge when `!plugin.compatible`
- Show "Requires a newer version of Shisho" warning text when incompatible
- Only show Install button when `plugin.compatible` is true

- [ ] **Step 5: Update `CapabilitiesWarning` to use first compatible version**

In `app/components/plugins/CapabilitiesWarning.tsx`, change line 59:

```tsx
  const latestVersion = plugin.versions.find((v) => v.compatible) ?? plugin.versions[0];
```

This picks the first compatible version for display. Falls back to `versions[0]` defensively (should never happen since the Install button is hidden for fully incompatible plugins).

- [ ] **Step 6: Run linting**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && pnpm lint:types && pnpm lint:eslint`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add app/hooks/queries/plugins.ts app/components/pages/AdminPlugins.tsx app/components/plugins/CapabilitiesWarning.tsx
git commit -m "[Frontend] Show plugin version compatibility in Browse tab"
```

---

### Task 5: Final validation

- [ ] **Step 1: Run full backend tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && go test ./pkg/plugins/ -v -count=1`
Expected: All tests pass

- [ ] **Step 2: Run full check suite**

Run: `cd /Users/robinjoseph/.worktrees/shisho/min-version && mise check:quiet`
Expected: All checks pass

- [ ] **Step 3: Commit any fixes if needed**
