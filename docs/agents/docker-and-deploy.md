# Docker, Production Serving, and the Public Demo

Read this before changing the `Dockerfile`, image smoke tests, the release or demo workflows, how the server listens or serves the frontend, or anything under `demo/`.

## Docker builds

The type-generation, frontend, and backend Docker stages run on `$BUILDPLATFORM`. The backend uses `CGO_ENABLED=0` and cross-compiles with `$TARGETOS`/`$TARGETARCH`; declare target and version arguments only after dependency installation so they do not invalidate dependency layers. Keep the final Alpine stage on the target platform. Multiarch builders still need QEMU for its package installation and user setup, not for Go or Node compilation.

Release publishing keeps parallel native AMD64 and ARM64 build jobs. The single-builder alternative passed native smoke tests but was slower in both cold and version-only GitHub benchmarks, so cross-compilation support does not mean consolidating the release runners.

CI builds one multiarch OCI archive and runs that same artifact on native AMD64 and ARM64 runners. Smoke tests check architecture, startup, the embedded frontend and its assets, version injection, custom `PUID`/`PGID`, and graceful shutdown. Smoke jobs reuse that artifact on native runners: no rebuilds, no emulation.

Only trusted `master` pushes export the shared GHA `image-smoke` cache. PRs and release tags may import it but must not export tag-local caches: exporting the intermediate layers can take longer than compiling, and the next release tag cannot reuse the previous tag's cache.

## Production serving

The Alpine image runs a single Go process through `su-exec` after resolving `PUID`/`PGID` and preparing `/config` ownership. It has no Caddy layer, startup health polling, or signal-forwarding shell. The image sets `SERVER_PORT=5173`; the application default stays `3689`. The listener honors `server_host`.

The Go server owns `/api` directly. Vite forwards `/api` unchanged, without injecting `X-Forwarded-Prefix`. Keep `/health`, `/opds`, `/kobo`, `/ereader`, and `/e` at the root. Forwarded-header trust, compression exclusions, security headers, and frontend cache behavior belong to the Go server, not a bundled proxy. See ADR 0007 (`docs/adr/0007-single-binary-image.md`) for the trade-offs.

## Public Demo

`demo/` holds the derived Public Demo image (`Dockerfile`), the Fly.io config (`fly.toml`), the corpus authoring Compose file, and `demo/README.md` with the authoring loop and operator setup. `.github/workflows/demo.yml` deploys when the Release workflow calls it after publishing the image, on manual dispatch, and on `repository_dispatch` from `shishobooks/demo-corpus`; it refuses tags older than the first Demo Mode release. Media and the prepared database live only in that corpus repository; `demo/corpus/` is a gitignored CI checkout. Demo Mode behavior itself is documented in `docs/agents/backend/demo-mode.md`.
