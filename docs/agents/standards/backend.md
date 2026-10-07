# Backend Review Standards

Rules a reviewer applies to a Go backend diff. Each rule names the violation to flag. A site in the codebase that breaks one of these is a bug, not a precedent. Rules golangci-lint or a test already enforces are not repeated here; the implementer-facing conventions are in `pkg/AGENTS.md` and the topic docs under `docs/agents/backend/`.

## Error statuses

- **422 `validation_error`** (`errcodes.ValidationError`) for a request value the server rejects (an invalid series range).
- **422 `invalid_state`** (`errcodes.InvalidState`) for a well-formed request the target's current state cannot honor (KePub of an audiobook). The frontend switches on the code, so the two 422s must not be merged.
- **400 only from the binder** (`malformed_payload`) and `EmptyRequestBody`; there is no 400 constructor. A malformed or truncated multipart body is the binder's 400, a missing part is a 422, and a filesystem failure while spilling the upload is a 500.
- **The binder's error passes through unchanged:** `if err := c.Bind(&payload); err != nil { return errors.WithStack(err) }`. Wrapping it in `ValidationError` collapses 400 `malformed_payload` and 422 `unknown_parameter` into one generic 422.
- **502 `upstream_error`** (`errcodes.UpstreamError`) when an upstream server fails. Not 422, not 500.
- **404 only when the lookup found no row.** Map `sql.ErrNoRows` or the service's own `errcodes.NotFound` to 404; wrap everything else so it renders 500. Flag `if err != nil { return errcodes.NotFound(...) }`.
- **Filesystem errors are 404 only for `os.IsNotExist`.** `EACCES`, `EIO`, and other stat or open failures are 500.
- **A numeric path ID that does not parse is `errcodes.NotFound(resource)`**, through `httputil.ParamID` (flag inline `strconv.Atoi(c.Param(...))`), matching `RequireLibraryAccess`. `ValidationError` stays for payload and query values. A string path ID rejected for path safety is a `ValidationError`.
- **A server fault is a plain error (500)**: `errors.New` or `errors.WithStack`, including a handler registered without a dependency.
- **Service errors a handler must distinguish are exported sentinels matched with `errors.Is`; flag `strings.Contains(err.Error(), ...)` and comparisons against `err.Error()`.** Only a sentinel-matched or pure-validator error may carry its text into a 4xx; flag `ValidationError(err.Error())` on an arbitrary service error, since parse errors name library paths.
- **Shared conditions use the shared constructors in `pkg/errcodes`**, not a hand-written message. A message repeated within one package gets one helper there.
- **Messages are full sentences used verbatim.** `errcodes.Forbidden` is for denials that are not a missing role permission. `errcodes.NotFound` takes the resource noun only, because it appends " not found." (flag `NotFound("Download file has expired from cache")`).

## Error tests

- **Every 4xx mapping has a fault-injection test asserting 500**, since a test of only the 4xx path cannot tell a correct mapping from one that sends every error there. Accepted injections: drop a table, corrupt a row so it cannot scan (set a timestamp column to `'not a time'`; use this when foreign keys block the drop), a trigger that aborts one statement, a nonempty directory obstructing a destination, `chmod` (skip under root, restore in `t.Cleanup`), a failing `httptest` server, or a fake dependency.
- A handler that gains a rejection adds a case to `TestAPIContract_StatusCodes` in `pkg/server/api_contract_test.go`.

## Response shapes

- Every request and response payload is a named exported struct in the package's `types.go`, and the package is listed in `tygo.yaml`. A struct in `handlers.go` is invisible to the frontend. Flag `echo.Map`, `map[string]any`, and anonymous structs passed to `c.JSON`. A response carrying nothing the client cannot derive is `204 No Content`.
- Responses reuse the model by value embed with `tstype:",extends"`, not by re-listing fields or pointer embedding.
- Naming: `{Entity}Response` for one resource; `List{Entities}Response` shaped `{ items, total }` for paginated lists; `{Entity}ListItem` only when the list item genuinely differs from the single shape.
- **Bare-model rule:** return the bare generated model when the response adds nothing; flag passthrough wrappers.
- **Two-tier collections:** paginated endpoints return the envelope; unpaginated full collections return a bare array of a named type (ADR 0004).
- **One type per shape:** list and retrieve returning the same shape share one `{Entity}Response`.
- **Computed fields come from one `build{Entity}Response` or `build{Entity}ListItem` method returning `(T, error)`**, called by every route returning that shape, so mutations return the full shape (reference: `buildPublisherResponse`). A failed count or alias lookup fails the request; flag a discarded error there.
- Payloads authenticated-only routes return are built from a struct holding only the returned fields (`models.UserRef`), never a full model with columns blanked, which still emits the other keys as zero values.
- The camelCase fields of `/api/test` routes are not a precedent for `snake_case` JSON.

## Authorization

- Every new route names the resource and operation it affects, or is deliberately authenticated-only.
- Routes returning book or file data check library access, by middleware or `auth.RequireLibraryAccessFor`.
- The user comes from `auth.RequireUser`, never an optional lookup that fails open when no user is set. List filters use that user's `GetAccessibleLibraryIDs()`.
- A read-only lookup rendered on pages every role opens has its own group with the permission its consumers hold, not an admin group's.
- Device routes that load an entity by URL id re-check it against the listing scope and return 404 outside it (`auth-and-permissions.md`).
- User-scoped resources need no global permission; a users permission on them locks out every stock role but Admin. Embedded users in payloads other roles read are `models.UserRef`.
- Backend checks are the security boundary; the frontend check is UX only. Both are present.

## Context and cancellation

- Handlers pass `c.Request().Context()` as the first argument to anything doing significant work (generation, file I/O, loops over content).
- Long operations check `ctx.Err()` before expensive steps and never cache partial results after cancellation.
- Work that must finish after the client leaves (usage counts, the search reindex) detaches with `context.WithoutCancel`.

## Output safety

- Hand-built HTML escapes every interpolated value, and stored plain text is not trusted to be tag-free ("Hand-built HTML" in `pkg/AGENTS.md`).

## Cross-cutting checklists

- A resource merge or re-point follows every item of the merge checklist in `docs/agents/backend/data-sources-and-merges.md`; a resource delete stamps sources, recomputes Reviewed, and reindexes as that doc describes.
- A write path keeps FTS current with collect-then-deferred-reindex (`docs/agents/backend/search-fts.md`).
- A new route family or download path is classified for Demo Mode (`docs/agents/backend/demo-mode.md`).
- A new file-serving route has a `chmod 000` fault test (`docs/agents/backend/covers-and-file-serving.md`).
- Error-path examples and new code never discard errors with `_` where the value reaches a response or a write.
