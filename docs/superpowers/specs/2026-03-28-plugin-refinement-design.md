# Plugin Refinement: Type Unification, HTML API, Cover Docs, Test Utilities

**Date:** 2026-03-28
**Status:** Approved
**Scope:** `pkg/plugins/`, `pkg/mediafile/`, `packages/plugin-sdk/`, `app/`, `website/docs/plugins/`

## Summary

Address four plugin development friction points identified by plugin authors:

1. **Type unification** — collapse `SearchResult` into `ParsedMetadata`, eliminating duplicate types and manual field mapping
2. **Cover handling clarity** — document the existing server-side cover download flow so plugins don't need to reason about `coverUrl` vs `coverData` precedence
3. **HTML parsing API** — add `shisho.html.querySelector` / `querySelectorAll` to eliminate regex-based HTML scraping
4. **Test utilities** — add `@shisho/plugin-sdk/testing` with mock factories and real implementations for common host APIs

All four ship together in a single coordinated release. No backwards compatibility concerns — this is pre-prod.

---

## 1. Type Unification: Collapse SearchResult into ParsedMetadata

### Problem

`SearchResult` and `ParsedMetadata` have nearly identical fields. Plugins must manually map between them (e.g., `toMetadata` / `enrichSearchResult` boilerplate). The fields unique to `SearchResult` are either server-added or legacy aliases — never set by plugins:

- `PluginScope` — set by server in `parseSearchResponse`
- `PluginID` — set by server in `parseSearchResponse`
- `DisabledFields` — set by server in handler after looking up user field settings
- `ImageURL` — legacy alias for `CoverURL`, removed entirely

### Changes

**Go — `pkg/plugins/hooks.go`:**
- Delete `SearchResult` struct
- Delete `SearchResultToMetadata()` function
- Delete `ImageURL` alias handling in `parseSearchResponse` (hooks.go:386-389)
- Search hook returns `[]mediafile.ParsedMetadata` directly (wrapped in `SearchResponse`)
- `parseSearchResponse` populates `ParsedMetadata` fields directly instead of `SearchResult` fields

**Go — new wrapper for HTTP responses:**
```go
// EnrichSearchResult wraps ParsedMetadata with server-added fields
// for the search results HTTP response. These fields are never set by plugins.
type EnrichSearchResult struct {
    mediafile.ParsedMetadata
    PluginScope    string   `json:"plugin_scope"`
    PluginID       string   `json:"plugin_id"`
    DisabledFields []string `json:"disabled_fields,omitempty"`
}
```

This is used only in the handler layer when serializing search results to the frontend. The plugin-facing type is just `ParsedMetadata`.

**TypeScript SDK — `hooks.d.ts`:**
- Remove `SearchResult` interface
- Search hook return type becomes `{ results: ParsedMetadata[] }`

**Frontend — `app/hooks/queries/plugins.ts`:**
- `PluginSearchResult` interface stays as-is — it's the frontend's representation of `EnrichSearchResult`, not a plugin-facing type. No change needed.

**Tests:**
- Update `hooks_search_result_test.go` — tests that referenced `SearchResult` fields now use `ParsedMetadata`
- Delete `TestSearchResultToMetadata_*` tests (function no longer exists)
- Update `hooks_test.go` if it asserts on `SearchResult` types

### What This Eliminates
- The `SearchResultToMetadata()` conversion function
- The `SearchResult` struct and its TypeScript counterpart
- All `toMetadata` / `enrichSearchResult` mapping boilerplate in plugins
- The `imageUrl` alias concept

---

## 2. Cover Handling: Documentation Clarity

### Problem

Plugin authors are confused about `coverUrl` vs `coverData`, when to declare cover domains in `httpAccess`, and the precedence rules.

### The Reality (Already Implemented)

The server already handles this correctly:
1. Enrichers set `coverUrl` on the returned `ParsedMetadata`
2. Server calls `DownloadCoverFromURL()` which validates the domain against the plugin's `httpAccess.domains` and downloads the image
3. If a plugin sets `coverData` directly (raw bytes), that takes priority — no download needed

### Changes

**Code:** Audit for any redundant domain validation that happens before `DownloadCoverFromURL`. If the server already validates at download time, remove pre-validation.

**Docs — `website/docs/plugins/`:**
- Clearly document: enrichers should set `coverUrl`. The server handles downloading and domain validation.
- Document `coverData` as an advanced option for: (a) file parsers extracting embedded covers, (b) enrichers that generate/composite images
- Remove any guidance suggesting plugins need to think about precedence rules — the server handles it
- Clarify that cover domains come from the same `httpAccess.domains` list used for `shisho.http.fetch` — no separate declaration needed

**SDK type comments:** Add JSDoc to `coverUrl` and `coverData` fields explaining when to use each.

---

## 3. HTML Parsing API: `shisho.html`

### Problem

Plugins resort to fragile regex scraping for HTML content because goja doesn't provide DOM APIs. The existing `shisho.xml` doesn't work for HTML (unclosed tags, implicit structure, etc.).

### New Host API

**Go — `pkg/plugins/hostapi_html.go`:**

Uses `golang.org/x/net/html` (already in go.mod) for parsing and `github.com/andybalholm/cascadia` (new dependency) for CSS selector support.

**API surface — two methods, mirroring `shisho.xml`:**

```typescript
shisho.html.querySelector(html: string, selector: string): HtmlElement | null
shisho.html.querySelectorAll(html: string, selector: string): HtmlElement[]
```

**Return type — `HtmlElement`:**

```typescript
interface HtmlElement {
    tag: string;
    attributes: Record<string, string>;
    text: string;        // recursive inner text
    innerHTML: string;   // raw inner HTML string
    children: HtmlElement[];
}
```

Differences from the XML element type:
- `innerHTML` field — enables getting raw content of `<script>` tags (e.g., JSON-LD) or preserving rich description HTML
- No `namespace` field (HTML doesn't use XML namespaces)
- Full CSS selector support via cascadia (attribute selectors, combinators, pseudo-classes) vs the XML implementation's basic tag matching

**Manifest:** No capability declaration needed. HTML parsing is a pure utility with no security implications.

**Registration:** `injectHtmlNamespace()` called from `InjectHostAPIs()` in `hostapi.go`.

### Usage Examples

```javascript
// Extract meta description
const meta = shisho.html.querySelector(html, 'meta[name="description"]');
const description = meta?.attributes?.content;

// Extract JSON-LD (the main use case from feedback)
const scripts = shisho.html.querySelectorAll(html, 'script[type="application/ld+json"]');
const jsonLd = JSON.parse(scripts[0]?.text);

// Extract Open Graph data
const ogTitle = shisho.html.querySelector(html, 'meta[property="og:title"]');
const title = ogTitle?.attributes?.content;
```

### SDK Changes

- Add `HtmlAPI` interface to `host-api.d.ts`
- Add `HtmlElement` interface
- Add `html` property to `ShishoHostAPI`

### Docs

- Add `shisho.html` section to plugin development docs in `website/docs/plugins/`
- Include examples for common patterns (meta tags, JSON-LD, Open Graph)

---

## 4. Test Utilities: `@shisho/plugin-sdk/testing`

### Problem

Every plugin reimplements `vi.mock` setup for `shisho.http.fetch`, `shisho.url.searchParams`, `shisho.log.*`, etc. Significant boilerplate across all plugins.

### Package Rename

`@shisho/plugin-types` is renamed to `@shisho/plugin-sdk` to reflect the expanded scope (types + test utilities). The old package is deprecated via `npm deprecate`.

### Package Structure

```
packages/plugin-sdk/
├── index.d.ts              → type re-exports (same as before)
├── global.d.ts             → global shisho & plugin declarations
├── host-api.d.ts           → ShishoHostAPI interfaces
├── hooks.d.ts              → hook contexts & ShishoPlugin interface
├── metadata.d.ts           → ParsedMetadata & related types
├── manifest.d.ts           → PluginManifest & Capabilities
├── testing/
│   ├── index.ts            → exports createMockShisho factory
│   └── index.d.ts          → type declarations
└── package.json
```

**package.json exports:**
```json
{
  "name": "@shisho/plugin-sdk",
  "exports": {
    ".": "./index.d.ts",
    "./testing": "./testing/index.js"
  }
}
```

### The Factory API

```typescript
import { createMockShisho } from "@shisho/plugin-sdk/testing";

const mockShisho = createMockShisho({
  fetch: {
    "https://api.example.com/book/123": {
      status: 200,
      body: '{"title": "Test Book"}',
      headers: { "content-type": "application/json" }
    }
  },
  config: {
    "api_key": "test-key"
  },
  fs: {
    "/data/book.epub": Buffer.from("..."),
    "/data/metadata.xml": "<package>...</package>",
    "/data/": ["book.epub", "metadata.xml"]
  }
});

globalThis.shisho = mockShisho;
```

### API Strategy Per Namespace

| API | Strategy | Notes |
|-----|----------|-------|
| `log.*` | Silent no-ops | `debug`, `info`, `warn`, `error` |
| `url.*` | Real implementations | Delegates to JS builtins (`encodeURIComponent`, `URL`, etc.) |
| `config.*` | Config map mock | `get(key)` returns from map, `getAll()` returns full map |
| `fetch` | Route-based mock | Matches URL, throws on unmatched requests |
| `fs.*` | Path-based mock | Virtual filesystem keyed by path |
| `xml.*` | Real implementation | Node XML parser library |
| `html.*` | Real implementation | Node HTML parser library (e.g., `linkedom` or `cheerio`) |
| `archive`, `ffmpeg`, `shell` | Not included | Rare in enricher plugins, can add later |

### Dependencies

The testing subpath introduces runtime dependencies (HTML/XML parser libraries). These are dependencies of `@shisho/plugin-sdk` itself and only used by consumers who import the `/testing` subpath. The main entry point (type declarations) remains dependency-free.

### Build

The package transitions from a zero-build type declaration package to one with a build step:

**Before:** `packages/plugin-types/` shipped only `.d.ts` files. No compilation, no `tsconfig`, no build script. `"files": ["*.d.ts"]` in package.json.

**After:** `packages/plugin-sdk/` has two entry points with different build characteristics:
- **Main entry (`.`):** Still pure `.d.ts` files — no compilation needed, no change from before
- **Testing entry (`./testing`):** Runtime TypeScript that must be compiled to `.js` + `.d.ts`

**Build setup:**
- Add a `tsconfig.json` to `packages/plugin-sdk/` targeting the `testing/` directory (the main `.d.ts` files don't need compilation)
- Add a `build` script to `package.json` that runs `tsc` on the testing subpath
- The `"files"` field in `package.json` expands from `["*.d.ts"]` to include `["*.d.ts", "testing/"]`
- The compiled `testing/index.js` and `testing/index.d.ts` should be committed or generated before publish — decide during implementation whether to gitignore compiled output (build on publish) or commit it (simpler for monorepo consumers)

**Impact on monorepo:** The root `pnpm` workspace already references `packages/plugin-types`. This path updates to `packages/plugin-sdk`. Any mise tasks or CI steps that reference the old path need updating. The `scripts/release.sh` updates the version in `packages/plugin-types/package.json` — this path must change too.

---

## Files Affected

### Go (modify)
- `pkg/plugins/hooks.go` — delete SearchResult, delete SearchResultToMetadata, update parseSearchResponse
- `pkg/plugins/hooks_search_result_test.go` — rewrite for ParsedMetadata
- `pkg/plugins/hooks_test.go` — update if referencing SearchResult
- `pkg/plugins/handler.go` — add EnrichSearchResult wrapper, update search handler
- `pkg/plugins/handler_enrich_test.go` — update if referencing SearchResult
- `pkg/plugins/hostapi.go` — register html namespace
- `pkg/plugins/hostapi_html.go` — new file, HTML parsing host API
- `pkg/plugins/hostapi_html_test.go` — new file, tests for HTML parsing

### TypeScript SDK (rename + modify)
- `packages/plugin-types/` → `packages/plugin-sdk/` — rename directory
- `package.json` — rename, add exports field
- `hooks.d.ts` — remove SearchResult, update search hook return type
- `host-api.d.ts` — add HtmlAPI, HtmlElement interfaces
- `testing/index.ts` — new file, createMockShisho factory
- `testing/index.d.ts` — new file, type declarations

### Frontend (modify)
- `app/hooks/queries/plugins.ts` — no change needed (PluginSearchResult is the frontend type, independent of SDK)

### Docs (modify/create)
- `website/docs/plugins/` — update plugin development docs:
  - Add `shisho.html` API documentation with examples
  - Clarify cover handling (coverUrl vs coverData guidance)
  - Document `@shisho/plugin-sdk/testing` usage
  - Update import references from `@shisho/plugin-types` to `@shisho/plugin-sdk`

### Config/Build
- `go.mod` / `go.sum` — add `github.com/andybalholm/cascadia`
- Root `package.json` / pnpm workspace — update if package directory changes

### CLAUDE.md Files
- `pkg/plugins/CLAUDE.md` — document `shisho.html` API, update type references from SearchResult to ParsedMetadata

### Test Data
- `pkg/plugins/testdata/hooks-enricher/main.js` — update to return ParsedMetadata instead of SearchResult shape (if different)
