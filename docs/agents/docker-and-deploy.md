# Docker, Production Serving, and the Public Demo

Read this before changing the `Dockerfile`, image smoke tests, the release or demo workflows, how the server listens or serves the frontend, or anything under `demo/`.

## Docker builds

Build stages cross-compile on the build platform; only the final stage runs on the target platform. Keep it that way: emulated compilation is what made multiarch builds slow. The comments in the `Dockerfile` and in `.github/workflows/ci.yml` and `release.yml` explain the stage layout, the smoke-test artifact, and the cache rules; read them before changing either.

## Production serving

The image runs the Go binary as its only process, with no reverse proxy or supervising shell (ADR 0007, `docs/adr/0007-single-binary-image.md`). Forwarded-header trust, compression, security headers, and frontend cache behavior belong in the Go server; don't add a proxy layer to handle them. The Go server owns `/api` directly, and device and health routes stay at the root, so don't add path rewriting or a prefix header in front of it.

## Public Demo

Demo media and the prepared database live only in the `shishobooks/demo-corpus` repository; never commit them here (`demo/corpus/` is a gitignored CI checkout). `demo/README.md` covers authoring and operator setup, and `docs/agents/backend/demo-mode.md` covers Demo Mode behavior.
