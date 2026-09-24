# Serve the frontend and API from one Go binary

The production image previously needed Caddy plus Go and a shell that polled backend health and managed both processes. We embed the SPA in a dedicated `pkg/frontend` package and serve it alongside the API so deployment and shutdown need only one long-running process. Container port `5173` and the existing public integration URLs remain unchanged, as agreed in [#478](https://github.com/shishobooks/shisho/issues/478).

## Considered options

- Keep Caddy. This retains a mature static server and compression support but leaves proxy configuration, route rewriting, startup polling, and signal forwarding coupled to every deployment.
- Use a `scratch` image. This reduces the base image further but does not preserve the shell and `su-exec` workflow for selecting `PUID`/`PGID` and preparing `/config` ownership. Keep Alpine and `su-exec`; the entrypoint ends by replacing itself with Go rather than supervising it.
- Keep zstd alongside gzip. Gzip alone avoids another compression implementation. Responses below 1 KiB do not benefit enough to compress. Event streams bypass compression to avoid buffering, and covers, page images, and downloads bypass it to avoid recompressing bytes.

## Consequences

`mise build` generates TypeScript types, builds the frontend, copies its output into `pkg/frontend/dist`, and then compiles Go. Docker follows the same ordering. A checked-in `dist/placeholder.html` lets plain Go builds and tests compile without a frontend build. The handler uses it only when generated `index.html` is absent, returning a "frontend not built" page. Generated files are gitignored, so complete builds do not modify tracked files. Frontend changes now require rebuilding the binary for production.

The API owns `/api` in Echo; Vite forwards that prefix unchanged. `/health`, `/opds`, `/kobo`, `/ereader`, and `/e` stay at the root. Unknown paths in the API and integration families return JSON 404s; other unknown paths fall back to the SPA. Hashed assets receive immutable one-year caching, while `index.html` requires revalidation. Successful static asset responses stay out of request logs.

The Go server now owns the forwarded-header boundary. It strips `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Forwarded-Port`, `X-Forwarded-Prefix`, and `X-Forwarded-For` unless the direct peer is loopback, link-local, RFC 1918 IPv4, or IPv6 `fc00::/7`. This preserves Caddy's private-range behavior without a new proxy allowlist setting. It deliberately trusts private peers rather than authenticating individual proxies, so operators must restrict origin access and overwrite client-supplied forwarded headers. OPDS retains prefix-stripping proxy support through `X-Forwarded-Prefix`; this does not add path-prefix support to the SPA.

Same-origin serving in production and through Vite makes permissive CORS unnecessary, so it is removed. Go sends `X-Frame-Options: SAMEORIGIN`, `X-Content-Type-Options: nosniff`, and `Referrer-Policy: strict-origin-when-cross-origin`, but neither `Server` nor the deprecated `X-XSS-Protection` header. TLS termination remains an external proxy's responsibility.

The config default stays `server_port: 3689` for development; Docker sets `SERVER_PORT=5173`. The listener now honors `server_host`, including loopback-only test listeners. `PUID`, `PGID`, and `LOG_FORMAT` keep their meaning. `STARTUP_TIMEOUT_SECONDS` and `CADDY_ACCESS_LOG_OUTPUT` disappear with the second process, while Docker health-check timings stay unchanged. A container smoke test checks startup, `/health`, and the real SPA so the placeholder cannot silently substitute for a release frontend.
