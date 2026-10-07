# Plugin lifecycle, installation, and repositories

Reference for `pkg/plugins/lifecycle.go`, `installer.go`, `repository.go`, and the install, update, and repository handlers. The locking and column-write invariants every transition obeys are in `pkg/plugins/AGENTS.md` under Concurrency; this page is the per-transition behavior.

A plugin lives in four stores that every transition keeps in agreement: its `plugins` row, its directory `{pluginDir}/{scope}/{id}/`, its registered runtime, and the manifest-derived rows (`plugin_identifier_types`, `plugin_hook_configs`, `library_plugin_hook_configs`).

## Transitions

| Transition | Entry point | Behavior |
|------------|-------------|----------|
| Startup | `LoadAll` | `sweepTransitDirs` first restores any plugin a crash left only in its scope's `.trash` (live directory missing), then removes each `.staging` and `.trash`; then enables each Active row through `enableLocked` |
| Install | `installStaged` | Refuses an installed id (`ErrAlreadyInstalled`) and a directory with no row (`ErrDirectoryExists`, such as an unscanned local plugin), swaps in, loads, inserts through `applyVersion`. A `LoadError` installs Malfunctioned or Not Supported and still returns 201; any other failure rolls the directory back and returns 500 |
| Update version | `updateStaged` | Loads from the staging directory before swapping; a `LoadError` or later failure leaves directory, runtime, and row untouched. On success Malfunctioned and Not Supported become Active, Disabled stays Disabled and unloaded, `update_available_version` is cleared |
| Reload | `reload` | Active only (`ErrNotActive`). A `LoadError` stores `load_error` and keeps the old runtime and version |
| Enable / disable | `enableLocked` / `disableLocked` | Called by PATCH under its lock. Enable records a `LoadError` as Malfunctioned or Not Supported |
| Uninstall | `uninstall` | Runs `onUninstalling`, deletes the row (children cascade), unregisters, removes the directory, and with `delete_data=true` the data directory. The row goes first, so a failed delete leaves the plugin whole |
| Update check | `refreshPluginUpdateVersion` | Fetches repositories without the lock, then writes `update_available_version` against the re-read row, skipping a plugin whose `auto_update` was turned off meanwhile |
| Local scan | `addScannedPlugin` | Inserts a Disabled row for each unknown `local/<id>` whose manifest id is `<id>`; runtime and derived rows come later from enable |

`LoadPlugin`, `ReloadPlugin`, and `UnloadPlugin` are the exported wrappers (tests and the test-mode seed route); the row must exist.

**Hook order reconciliation** (`reconcileHookOrder`): rows for hook types the version no longer provides are deleted globally and per library. A newly provided hook type is appended to the global order and to every library order customized for that hook type, with the global row's mode. Existing rows keep position and mode. `library_plugin_customizations` rows are per library and hook type, so uninstall leaves them.

## Install and update flow

1. `POST /plugins/installed` takes `{scope, id}` or adds `downloadURL` and `sha256`. An installed id is refused with `422 invalid_state` before any download; changing an installed plugin's version goes through update version. Without a URL, enabled repositories are searched for the latest compatible version.
2. `stagePackage` downloads from an allowed host, verifies SHA256, and extracts into `{pluginDir}/{scope}/.staging/<random>`, beside the live plugins so the move is a rename even when a scope directory is its own mount. The package's `manifest.json` id must equal the requested id (`ErrInvalidPackage`, 422).
3. The repository icon is downloaded into the staged package. On update, `carryOverIcon` copies the installed `icon.png` into a package that has none, so a failed icon download keeps the old icon.
4. `swapIn` moves the live directory to `{scope}/.trash/<random>/{id}` and the staged one into place. Failures after the swap call `rollback` through `withRollback` (a failed rollback is added to the returned error); success calls `commit`, which deletes the trash.

**Error mapping** (`installerError` in `handler_install.go`, shared by install and update version): a URL outside `AllowedDownloadHosts`, a checksum mismatch, or a package that is not a ZIP or lacks a valid manifest is `422 validation_error`; an unreachable download host or non-200 is `502 upstream_error`; anything else is 500. `findPluginInRepos` returns 502 when no repository for the scope answered and 404 when one answered without the plugin.

## Repositories

Repositories serve a `repository.json` index (`repository.go`). Downloads are limited to `AllowedDownloadHosts` and index fetches to `AllowedFetchHosts` (both GitHub only), and every package needs a SHA256. The official repository cannot be removed.
