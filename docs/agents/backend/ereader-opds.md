# OPDS and eReader

Both are device route families mounted at the root (`/opds`, `/ereader`, `/e`), not under `/api`. Their access rules (scope re-checks, API key middleware) are in `auth-and-permissions.md`; their downloads use the fallback in `covers-and-file-serving.md`. Keep both in step with new library features.

## OPDS (`pkg/opds`)

OPDS v1.2 with Basic Auth. **Feed cover URLs point at `/opds/v1/books/:id/cover`** (`bookCover`), never the books API, which needs session auth; bare `/books/*` paths belong to the SPA. Build them off `apiBase + "/opds/v1"` in `bookToEntryWithKepub` so a trusted proxy's `X-Forwarded-Prefix` is preserved.

## eReader browser UI (`pkg/ereader`)

Server-rendered HTML for stock Kobo and Kindle browsers that cannot use OPDS or the React app. Routes live under `/ereader/key/:apiKey/*`, authenticated by the API key in the path; covers have their own `/ereader/key/:apiKey/cover/:bookId` endpoint.

**Target browsers** have no flexbox or modern CSS, minimal JavaScript, no Basic Auth, and (Kobo) cookies cleared on close. So:

- Inline styles, not attribute selectors like `input[type="text"]`.
- `display: block` on block links; stack inputs and buttons vertically; `width: 100%`.
- 12px+ padding on tap targets, explicit `2px solid #000` borders.
- `<input type="submit">` rather than `<button>`.

**Escaping.** Pages are built by string concatenation on the app's own origin, so every interpolated value, URLs and safe-looking values included, goes through `html.EscapeString`. Stored plain text is not tag-free: titles and names come straight from file metadata, and `htmlutil.StripTags` output (descriptions) decodes only one entity level, so it is text to escape. Multi-line text escapes first and adds markup after (`descriptionHTML` in `templates.go`). Reference test: `TestEReaderPages_EscapeMetadata`. The KePub XHTML in `pkg/kepub/cbz.go` follows the same rule; OPDS (`xml.NewEncoder`) and Kobo (JSON) escape through their encoders.
