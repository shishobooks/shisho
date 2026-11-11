# KOReader Plugin

The KOReader plugin (ADR 0016) is plain Lua with no build step: `shisho.koplugin/` is the plugin exactly as KOReader loads it, embedded in the server by `embed.go` and zipped by `pkg/koreaderplugin`. Read `tools/koreaderharness/AGENTS.md` before testing it.

- **Decisions go in `shisho.koplugin/shisho/` modules with no UI**, never in `main.lua`. `main.lua` only handles KOReader's events, menus, dialogs, Wi-Fi, and HTTP, and can't be unit tested. Each module's tests are a `spec/*_spec.lua` file returning a table of test functions, run with KOReader's own luajit by `mise koreader lua-test` and `mise test:koreader`. Use KOReader's real modules in them (settings, JSON, files), not mocks; `scratch_dir()` gives a test its own directory.
- **Behavior in real KOReader is tested in the harness** (`tools/koreaderharness/plugin_scenario_test.go`), driving the plugin through the commands in `driver.lua`. KOReader fires no event for much of what the plugin watches, so check how KOReader itself behaves there before assuming.
- **Target the harness's pinned KOReader release** (`koreaderVersion` in `tools/koreaderharness/harness.go`) and newer. Read KOReader's own source in `tmp/koreader/release` for an API's signature rather than guessing.
- **The protocol is a contract with every server**: `shisho/protocol.lua` must equal `Protocol` in `pkg/koreaderplugin`, which a Go test checks. Bump it only for a change an older server can't handle, and raise the server's `ProtocolMin` only when it drops what an older plugin sends, so older plugins keep working against newer servers.
- **The stamped files `build.lua` and `pairing.lua` are added to the zip by the server, never committed.** Load them by path through `shisho/stamps.lua`, not `require`.
- **`for _, x in ipairs(...)` shadows gettext's `_`**, so loops use `__`.
