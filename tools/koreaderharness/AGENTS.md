# KOReader Harness

Real KOReader, pinned to one release, running headless in Docker. Use it for KOReader QA instead of guessing what KOReader sends or does: expected hashes, XPointers, and request shapes in tests come from it, not from reading the plugin's source. `mise koreader help` lists the commands.

- **Point it at the API port `mise start` prints**, not the web port: the container can't reach Vite. `localhost` is rewritten to reach the host from the container.
- **Every command is one KOReader run**, so state lasts only in KOReader's own settings and sidecars under `tmp/koreader`. Delete that directory for a device fresh out of the box; a stale sidecar keeps a book's old hash, as a real device does.
- **`mise test:koreader`** runs round trips against an in-process server. It's behind a build tag so `go test ./...` doesn't need Docker; CI runs it in its own job.
