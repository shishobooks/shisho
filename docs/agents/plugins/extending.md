# Extending the plugin system

Go-side steps for growing the plugin surface. Every step here also needs the SDK and docs updates in the table under "Plugin SDK" in `pkg/plugins/AGENTS.md`.

## New host API

1. Add `hostapi_<name>.go` with an `inject<Name>Namespace(vm, shishoObj, rt)` function, following the existing ones, and call it from `InjectHostAPIs` in `hostapi.go`.
2. If it needs permission, add the capability to `Capabilities` in `manifest.go` and check it inside the API, so an undeclared call throws.
3. If it can block (network, process, timer), select on `rt.hookCtx` (see Host APIs in `pkg/plugins/AGENTS.md`).
4. Add `hostapi_<name>_test.go`.

## New hook type

1. Add the constant in `pkg/models/plugin.go` and extend the `PluginHookType` tygo emit line beside it.
2. Add a `goja.Value` field to `Runtime`, extract it in `LoadPlugin` (`runtime.go`), validate it against the manifest capability, and add it to `HookTypes()`, which drives hook order reconciliation.
3. Add a `Run<Hook>` method on `Manager` in `hooks.go` that goes through `invokeHook` with a timeout and takes `rt.mu.Lock()`, plus its result parser.
4. Wire it into the scan pipeline or the service that calls it.

## New ParsedMetadata field

Prefer a new optional field over changing an existing one. Update the struct in `pkg/mediafile/mediafile.go` and every JS-to-Go parser in `hooks.go`; the plugin bridge has several parallel parsers (file parser results, search results, and the series-number group), so grep for an existing field's camelCase key to find them all. The `metadata-field` skill covers the rest of the stack.
