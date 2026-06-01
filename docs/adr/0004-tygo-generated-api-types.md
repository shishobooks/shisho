# Generate API types from Go via tygo

## Status

Accepted. This records the decision; the implementation is tracked in PRD #341
and its slices (#342 onward), so the codebase reaches the described state
incrementally rather than all at once.

## Context

Shisho's frontend and backend share a large surface of request and response
payloads. Go is the source of truth for these shapes, and tygo generates the
TypeScript equivalents into `app/types/generated/` so the frontend never has to
restate a type by hand.

In practice this contract had eroded. An audit found three recurring failures:

1. **Hand-written TypeScript duplicates of types that already exist in Go.**
   `app/hooks/queries/search.ts` redefined `BookSearchResult`,
   `SeriesSearchResult`, and friends byte-for-byte; `settings.ts` redefined the
   user-settings shape; `index.ts` redefined `ListBooksQuery`. These drift
   silently because nothing ties them back to the Go struct.

2. **Handlers returning anonymous structs, `echo.Map`, or `map[string]any`.**
   tygo cannot generate a type for an anonymous or untyped response, so the
   frontend was left to invent a matching type by hand (see failure 1) or go
   untyped. Roughly two dozen handlers did this, especially list endpoints
   returning `map[string]any{"items": ..., "total": ...}`.

3. **Enum-shaped string fields generating as bare `string`.** Go models already
   use a `//tygo:emit` union plus a `tstype:"..."` tag to emit precise union
   types (`FileRole`, `JobType`, and so on). Hand-rolled request and response
   structs frequently omitted the `tstype` tag, so a field constrained by
   `validate:"oneof=..."` on the backend arrived on the frontend as `string`,
   losing the exhaustiveness the union would provide.

The root cause is that nothing made "the type crosses the boundary through tygo"
the only viable path. When tygo could not see a shape, the path of least
resistance was to duplicate it in TypeScript.

Issue #324 (aliases rendering as `, ,` on the resource list pages) is the
canonical symptom. The handlers embed `*models.Person` and override the
`aliases` JSON field with `[]string`, but that anonymous struct was never seen
by tygo, so the generated `Person.aliases` stayed `PersonAlias[]` (the model
relation). The frontend extended `Person`, inherited `PersonAlias[]`, and called
`.map((a) => a.name)` on values that were actually strings. The detail pages
papered over the same mismatch with `as unknown as string[]` casts. PR #336
proposed extending that cast to the list pages rather than fixing the root
cause, and was closed in favor of the approach in this ADR. The alias and
publisher-hierarchy relations this reshapes are the subject of ADR 0001
(per-resource alias tables) and ADR 0002 (unified publisher hierarchy).

Alternatives considered and rejected:

- **Keep `echo.Map` / anonymous structs and hand-write the TS.** This is what we
  had. It is less Go boilerplate but guarantees drift and untyped frontends.
- **A single generic `PaginatedResponse[T]`.** tygo's handling of Go generics is
  unreliable, and a generic envelope still would not have addressed the
  non-list anonymous responses. Rejected in favor of explicit per-endpoint
  response structs.
- **Explicit response structs that re-list every model field (no embedding).**
  Fully tygo-friendly, but duplicates the model's field set in Go and drifts
  when the model changes. Rejected once we confirmed tygo can flatten an
  embedded model via `tstype:",extends"` (see Decision point 2).
- **A frontend intersection type (`Omit<Person, "aliases"> & {...}`).** Reuses
  the generated model, but is frontend-authored, which is the duplication this
  ADR is trying to eliminate. Rejected in favor of keeping the response type in
  Go.

## Decision

The API type boundary is generated from Go via tygo, with no hand-maintained
TypeScript duplicates. Concretely:

1. **Every request and response payload is a named, exported Go struct** living
   in the package's `types.go` (to be renamed from the older `validators.go`,
   which undersells its role now that it holds request, response, query, and
   enum types). No handler returns an anonymous struct, `echo.Map`, or
   `map[string]any`.

2. **Response structs reuse the model by embedding it with `tstype:",extends"`,
   not by re-listing fields.** tygo flattens such an embed into a TypeScript
   `extends`, so a response like `GenreResponse` carries every model field plus
   its computed extras (`book_count`, `aliases`) with no Go-side duplication.
   This requires a `frontmatter` import of the embedded model in the package's
   `tygo.yaml` entry. When a response **reshapes** a model relation (the metadata
   entities return `aliases` as `[]string` rather than the model's
   `PersonAlias[]`; publisher detail returns a flattened `children` rather than
   the model's `Publisher[]`), the model's relation field is excluded from
   TypeScript generation with `tstype:"-"` so the response's field is the only
   one and the `extends` does not collide. This is verified safe only when no
   consumer reads that relation as objects; both `aliases` and publisher
   `children` were confirmed to have no such consumers.

3. **List endpoints return a `List{Entities}Response` envelope** shaped
   `{ items, total }`. Single-resource responses are `{Entity}Response`; a
   list-item shape that genuinely differs from the single-resource shape (only
   publishers today, because computing the hierarchy per row would be an N+1) is
   `{Entity}ListItem`.

4. **A response body must carry information the client cannot already derive.**
   If the only thing a body conveys is "success" (already implied by the 2xx
   status) plus identifiers the client supplied, the endpoint returns
   `204 No Content` rather than a typed body. A body is warranted when it
   reports a consequence the client could not predict (for example a cascade
   delete count).

5. **Enum-shaped fields use the established `//tygo:emit` union plus a
   `tstype:"..."` tag**, so request and response fields are as strongly typed as
   the models. Cross-package references add a `frontmatter` import to the
   package's `tygo.yaml` entry.

6. **The frontend never hand-defines a type that has a Go counterpart.** If a
   needed type is missing, the fix is to add or correct the Go struct and
   regenerate, not to write the type in TypeScript.

## Consequences

- The frontend's generated types become trustworthy. Removing the manual
  duplicates means a backend shape change surfaces as a TypeScript error rather
  than silent drift. Concretely, #324's `aliases` mismatch turns into a compile
  error that forces the correct pass-through, and the nine alias workarounds
  across the list and detail pages (the five `as unknown as string[]` casts on
  the detail pages plus the four `.map((a) => a.name)` calls on the list pages)
  can be removed.
- Embedding with `tstype:",extends"` keeps the responses DRY, at the cost of a
  `frontmatter` import per package and the discipline of marking reshaped model
  relations `tstype:"-"`. A future reshape of a model relation must check for
  object consumers before excluding it.
- The convention must be enforced to avoid re-eroding. The root and
  subdirectory `CLAUDE.md` files will document the rule (no anonymous responses,
  no manual TS duplicates, `tstype` on enum fields, embed-with-extends for
  responses) so reviewers can treat violations as review failures.
- Some responses legitimately have no body. Endpoints that return a cosmetic
  `{"message": "..."}` move to `204`, and the frontend relies on the status, not
  the body.
- A genuinely new shared shape still requires a Go struct first. Reaching for a
  quick `echo.Map` is no longer acceptable even for one-off responses.
