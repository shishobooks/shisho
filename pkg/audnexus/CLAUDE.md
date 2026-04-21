# Audnexus API Integration

This package fetches chapter data for audiobooks from [Audnexus](https://audnex.us), a community-run proxy over Audible's chapter catalog. It's consumed by the M4B chapter edit UI via `GET /audnexus/books/:asin/chapters`.

## Architecture

- `service.go` — `Service` with a 5s-timeout HTTP client and a 24h in-memory TTL cache keyed by normalized (uppercase) ASIN. Only successes are cached; errors pass through so retries work.
- `handlers.go` — Echo handler that calls the service and maps typed errors to HTTP statuses.
- `routes.go` — `RegisterRoutes` wires the endpoint with `Authenticate` + `books:write` middleware.
- `types.go` — Response types with snake_case JSON tags. The upstream Audnexus camelCase shape is decoded into `audnexusUpstream` and converted at the parse boundary.
- `errors.go` — `ErrorCode` string identifiers and an `*Error` type. Use `AsAudnexusError(err)` to inspect.

## Endpoint

| Method | Path | Permission |
|--------|------|------------|
| GET | `/audnexus/books/:asin/chapters` | `books:write` |

### Error codes

| Service code | HTTP status |
|--------------|-------------|
| `invalid_asin` | 400 |
| `not_found` | 404 |
| `rate_limited` | 429 |
| `timeout` | 504 |
| `upstream_error` | 502 |

Audnexus rate-limits by IP and can return 429 or 503 under load (see [their error handling docs](https://github.com/laxamentumtech/audnexus#%EF%B8%8F-error-handling-)); both upstream statuses map to `rate_limited`.

ASINs are validated before upstream calls: 10 alphanumeric characters, normalized to uppercase.

## Permissions

Uses `books:write` rather than `books:read` because the only legitimate use is staging data into an editable chapter form. Read-only users can't save what they fetch, so exposing the endpoint to them is pointless and widens the surface area. If the UI hides the button, the endpoint must also reject the request.

## Caching

In-memory `map[string]*cacheEntry` protected by `sync.RWMutex`, TTL 24h. No persistence across restarts. The cache is small (one entry per ASIN), and Audnexus data rarely changes, so there's no eviction beyond TTL.
