# Plugin System

Plugins are third-party JavaScript run in sandboxed Goja VMs, one VM per plugin, with access to the `shisho.*` host APIs. The public contract (manifest fields, hook contexts and results, host API behavior, timeouts) is documented in `website/docs/plugins/manifest-hooks-reference.md` and `host-api-reference.md` and typed in `packages/plugin-sdk/*.d.ts`; read those rather than inferring it from Go.

- Read ADR 0006 (`docs/adr/0006-identify-source-attribution.md`) before changing Identify apply (`POST /plugins/apply`) or which source a field gets.

## Plugin SDK

`packages/plugin-sdk/` is the published contract for plugin authors, so it changes in the same PR as the Go it mirrors, along with the matching `website/docs/plugins/` page. Prefer additive changes (new optional fields); removing or retyping a field breaks plugins. `sdk_sync_test.go` checks names only: nothing checks hook context fields, the `ParsedMetadata` field mapping, or whether a change breaks plugins, so those stay on you.

SDK fields are camelCase (`seriesNumberEnd`); snake_case belongs only to Go JSON and the HTTP apply payload. `ParsedMetadata` fields tagged `json:"-"` are host-internal: no SDK change, and never populated from plugin output.

Manifest and repository-index passthrough fields in HTTP responses keep camelCase, so renaming one is a breaking wire change; fields the server adds to the same responses are snake_case.

## Concurrency

- **Goja VMs are single-threaded.** `Runtime.mu` serializes hooks and reloads; `Manager.mu` guards only the registry map. Every hook runner takes `rt.mu.Lock()`, never `RLock`: the parallel scan worker pool calls hooks from many goroutines, and two goroutines in one VM corrupt it.
- **One lifecycle lock per plugin.** Anything that changes a plugin's row, directory, runtime, or manifest-derived rows holds `lockPlugin(scope, id)` and re-reads the row inside it. Lock order and placement are in the `lockPlugin` comment.
- **Column-scoped writes only.** There is no full-row plugin update, on purpose: a full-row write from a stale read reverts concurrent changes. Use `Service.UpdatePluginColumns`, or `applyVersion`, the only place a loaded version is stored.

## Load failures and plugin refs

- **Load failures are typed.** When the plugin itself is at fault, `loadRuntime` returns a `*LoadError`, recorded on the plugin and surfaced as `422 plugin_load_failure`. Any other error is a server fault: 500, stored state untouched. A manifest mistake that would otherwise surface as a database error (for example a repeated `identifierTypes` id) must be rejected in `ParseManifest` so it stays a `LoadError`.
- Every route taking `:scope/:id` that touches the plugin directory calls `validatePluginRef` first.

## Hooks and host APIs

- **File parser and converter `mimeTypes` match with `(*mimetype.MIME).Is`**, never string equality, so registered aliases and MIME parameters keep matching. This applies to callers in `pkg/worker` too.
- **Blocking host APIs must honor `Runtime.hookCtx`.** `vm.Interrupt()` fires only between JS statements and cannot cancel a native wait, so a new blocking API that ignores the context holds `Runtime.mu` past the hook timeout.
- **A new host API that needs permission** adds a capability to the manifest and checks it inside the API, so an undeclared call throws.
- **A new `ParsedMetadata` field** must reach every JS-to-Go parser in `hooks.go`; there are several parallel ones (file parser results, search results, the series-number group), so grep for an existing field's camelCase key to find them all. The `metadata-field` skill covers the rest of the stack.

## Testing

- **`t.Parallel()`** belongs on pure-function tests and on tests that build their own database, plugin directory, and manager (`newLifecycleEnv`). Leave it off tests that share a manager or runtime, tests that set the package globals `AllowedDownloadHosts` or `AllowedFetchHosts` (every downloading install or update test, including those using `servePackage`), and tests that call `testumask.Set`.
- **Prove lock waits without sleeping.** `Manager.onLockWait`, when set, runs just before `lockPlugin` blocks; `servePackage` can hold a download until `releaseNow`. Use these instead of `time.Sleep` in concurrency tests.
