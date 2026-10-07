# Plugin lifecycle, installation, and repositories

Reference for `pkg/plugins/lifecycle.go`, `installer.go`, `repository.go`, and the install, update, and repository handlers. The locking and column-write invariants every transition obeys are in `pkg/plugins/AGENTS.md` under Concurrency; this page is the per-transition behavior.

A plugin lives in four stores that every transition keeps in agreement: its `plugins` row, its directory `{pluginDir}/{scope}/{id}/`, its registered runtime, and the manifest-derived rows (`plugin_identifier_types`, `plugin_hook_configs`, `library_plugin_hook_configs`).

## Transitions

Each transition is one function in `lifecycle.go` (startup is `LoadAll` in `manager.go`, the update check `refreshPluginUpdateVersion`, the local scan `addScannedPlugin`). Rules they share:

- **A `LoadError` is stored on the row, not returned as a failure.** Install and enable record Malfunctioned or Not Supported (install still returns 201); reload keeps the old runtime and version. Any other failure rolls back and is a 500.
- **Nothing live changes until the new version has loaded.** Update version loads from the staging directory before swapping, so a failure leaves directory, runtime, and row untouched.
- **Uninstall deletes the row first**, so a failed delete leaves the plugin whole.
- **A directory with no row is never overwritten** (`ErrDirectoryExists`, such as an unscanned local plugin).
- The update check fetches repositories without the lock and writes against the re-read row, skipping a plugin whose `auto_update` was turned off meanwhile.

**Hook order reconciliation** (`reconcileHookOrder`): rows for hook types the version no longer provides are deleted globally and per library. A newly provided hook type is appended to the global order and to every library order customized for that hook type, with the global row's mode. Existing rows keep position and mode. `library_plugin_customizations` rows are per library and hook type, so uninstall leaves them.

## Install and update flow

1. `POST /plugins/installed` takes `{scope, id}` or adds `downloadURL` and `sha256`. An installed id is refused with `422 invalid_state` before any download; changing an installed plugin's version goes through update version. Without a URL, enabled repositories are searched for the latest compatible version.
2. `stagePackage` downloads from an allowed host, verifies SHA256, and extracts into `{pluginDir}/{scope}/.staging/<random>`, beside the live plugins so the move is a rename even when a scope directory is its own mount. The package's `manifest.json` id must equal the requested id (`ErrInvalidPackage`, 422).
3. The repository icon is downloaded into the staged package. On update, `carryOverIcon` copies the installed `icon.png` into a package that has none, so a failed icon download keeps the old icon.
4. `swapIn` moves the live directory to `{scope}/.trash/<random>/{id}` and the staged one into place. Failures after the swap call `rollback` through `withRollback` (a failed rollback is added to the returned error); success calls `commit`, which deletes the trash.

## Repositories

Repositories serve a `repository.json` index (`repository.go`). Downloads are limited to `AllowedDownloadHosts` and index fetches to `AllowedFetchHosts` (both GitHub only), and every package needs a SHA256. The official repository cannot be removed.
