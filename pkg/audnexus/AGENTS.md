# Audnexus

Fetches Audible chapter data from Audnexus (https://audnex.us) for the M4B chapter edit form, through `GET /api/audnexus/books/:asin/chapters`. The fetched chapters are staged into the form and saved only when the user saves.

- **The route requires `books:write`, not `books:read`.** The only use is filling an editable form, so read-only users gain nothing and the surface stays smaller. Hiding the button is not enough; the endpoint must reject the request too.
- ASINs are validated before any upstream call (10 alphanumeric characters) and normalized to uppercase, which is also the cache key.
- `Service` uses a 5-second HTTP client and a 24-hour in-memory cache of successes only, so a failed lookup can be retried at once. Nothing persists across restarts.
- Upstream camelCase is decoded into `audnexusUpstream` and converted to snake_case response types at the parse boundary.
- Failures are typed `*Error` values with an `ErrorCode` (`AsAudnexusError`), mapped in `handlers.go` to 400 `invalid_asin`, 404 `not_found`, 429 `rate_limited`, 504 `timeout`, and 502 `upstream_error`. Audnexus rate-limits by IP with either 429 or 503; both map to `rate_limited`.
- **Tests stub the upstream with an `http.RoundTripper`** through `ServiceConfig.HTTPClient` (`stubService`, `respondWith` in `service_test.go`). A live `httptest.NewServer` upstream races under CI load and turns status-mapping tests into flaky `upstream_error` results. `httptest.NewRequest` and `NewRecorder` for driving the handler are fine.
