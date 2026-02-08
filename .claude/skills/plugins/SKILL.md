---
name: plugins
description: You MUST use this before working on anything plugin-related (e.g. hooks, runtime, manager, host APIs, manifests, repositories, installation, configuration, or scan integration). Covers the Goja JS runtime, hook types, host APIs, filesystem sandboxing, metadata priority, and the plugin lifecycle.
---

# Plugin System Reference

Shisho's plugin system allows third-party JavaScript to extend file parsing, metadata enrichment, format conversion, and output generation. Plugins run in sandboxed Goja VMs with controlled access to host APIs.

## Architecture Overview

```
pkg/plugins/
  manifest.go       - Manifest parsing and types
  runtime.go        - Goja VM wrapper, plugin loading
  manager.go        - Plugin lifecycle coordination
  hooks.go          - Hook invocation and result parsing
  hostapi.go        - Host API injection (shisho.*)
  hostapi_fs.go     - Filesystem sandbox (FSContext)
  hostapi_http.go   - HTTP with domain whitelisting (supports wildcards)
  hostapi_url.go    - URL utilities (encode/decode/searchParams/parse)
  hostapi_archive.go - ZIP operations
  hostapi_xml.go    - XML parsing
  hostapi_ffmpeg.go - FFmpeg transcode/probe/version
  hostapi_shell.go  - Shell exec with command allowlist
  generator.go      - PluginGenerator (filegen.Generator interface)
  installer.go      - Download, verify, extract
  repository.go     - Repository manifest fetching
  service.go        - Database CRUD operations
  handler.go        - HTTP API handlers
  routes.go         - Echo route registration
```

## Plugin Types SDK (`@shisho/plugin-types`)

The `packages/plugin-types/` directory contains a TypeScript type definitions package that plugin developers install for IDE autocompletion and type checking. It is the public API contract for plugin authors.

**CRITICAL: The SDK must always be kept in sync with the Go implementation.** Any change to host APIs, hook contexts/return types, manifest schema, or metadata structures MUST be reflected in the corresponding `.d.ts` files. Breaking changes to the SDK should be avoided whenever possible — prefer additive changes (new optional fields) over removals or type changes.

```
packages/plugin-types/
├── package.json       # @shisho/plugin-types
├── index.d.ts         # Re-exports everything + imports global declarations
├── global.d.ts        # Declares global `shisho` and `plugin` variables
├── host-api.d.ts      # ShishoHostAPI (log, config, http, url, fs, archive, xml, ffmpeg, shell)
├── hooks.d.ts         # Hook contexts, return types, ShishoPlugin interface
├── metadata.d.ts      # ParsedMetadata, ParsedAuthor, ParsedIdentifier, ParsedChapter
└── manifest.d.ts      # PluginManifest, Capabilities, ConfigSchema, ConfigField
```

## Plugin File Structure

Plugins live at `{pluginDir}/{scope}/{id}/` with exactly two files:

```
shisho/goodreads-metadata/
  manifest.json   # Declares capabilities, config schema, permissions
  main.js         # JavaScript IIFE defining a `plugin` global
```

## Manifest Schema

```json
{
  "manifestVersion": 1,
  "id": "plugin-id",
  "name": "Display Name",
  "version": "1.0.0",
  "description": "...",
  "author": "...",
  "homepage": "...",
  "license": "...",
  "minShishoVersion": "...",
  "capabilities": {
    "inputConverter": { "description": "", "sourceTypes": ["mobi"], "mimeTypes": [], "targetType": "epub" },
    "fileParser": { "description": "", "types": ["pdf"], "mimeTypes": ["application/pdf"] },
    "outputGenerator": { "description": "", "id": "mobi", "name": "MOBI", "sourceTypes": ["epub"] },
    "metadataEnricher": { "description": "", "fileTypes": ["epub", "cbz"], "fields": ["title", "authors", "description", "cover"] },
    "identifierTypes": [{ "id": "goodreads", "name": "Goodreads", "urlTemplate": "https://goodreads.com/book/show/{value}", "pattern": "^\\d+$" }],
    "httpAccess": { "description": "", "domains": ["*.goodreads.com"] },
    "fileAccess": { "level": "read", "description": "" },
    "ffmpegAccess": { "description": "" },
    "shellAccess": { "description": "", "commands": ["calibre-debug", "kindlegen"] }
  },
  "configSchema": {
    "apiKey": { "type": "string", "label": "API Key", "description": "", "required": true, "secret": true },
    "maxResults": { "type": "number", "label": "Max Results", "min": 1, "max": 100, "default": 10 },
    "mode": { "type": "select", "label": "Mode", "options": [{"value": "fast", "label": "Fast"}] }
  }
}
```

**Required fields:** `manifestVersion` (must be 1), `id`, `name`, `version`

**Config field types:** `string`, `boolean`, `number`, `select`, `textarea`

**Validation rules:**
- JS exports hook but manifest doesn't declare capability → **load fails**
- Manifest declares capability but JS doesn't export → silent (no error)
- Reserved extensions (`epub`, `cbz`, `m4b`) cannot be claimed by fileParsers
- `metadataEnricher` requires `fields` array → if missing/empty, enricher hook is **disabled** (other hooks still work)
- Invalid field names in `fields` → **load fails**

**Valid metadata fields for enrichers:**
`title`, `subtitle`, `authors`, `narrators`, `series`, `seriesNumber`, `genres`, `tags`, `description`, `publisher`, `imprint`, `url`, `releaseDate`, `cover`, `identifiers`

**Logical field groupings:**
- `cover` → controls `coverData`, `coverMimeType`, and `coverPage`
- `series` → controls both `series` (name) and `seriesNumber`

## main.js Pattern

All plugins use IIFE to define the `plugin` global:

```javascript
var plugin = (function() {
  return {
    hookName: {
      methodName: function(context) {
        // Use shisho.* host APIs
        return result;
      }
    }
  };
})();
```

## Hook Types

### inputConverter (5 min timeout)

Converts unsupported formats to supported ones.

```javascript
inputConverter: {
  convert: function(context) {
    // context.sourcePath - input file path
    // context.targetDir  - directory for output
    var content = shisho.fs.readTextFile(context.sourcePath);
    var targetPath = context.targetDir + "/output.epub";
    shisho.fs.writeTextFile(targetPath, converted);
    return { success: true, targetPath: targetPath };
  }
}
```

**Go invocation:** `Manager.RunInputConverter(ctx, rt, sourcePath, targetDir) → *ConvertResult`

### fileParser (1 min timeout)

Extracts metadata from files. DataSource auto-set to `plugin:scope/id` if not returned.

```javascript
fileParser: {
  parse: function(context) {
    // context.filePath - file to parse
    // context.fileType - extension (e.g., "pdf")
    return {
      title: "Book Title",
      subtitle: "Subtitle",
      authors: [{ name: "Author", role: "writer" }],
      narrators: ["Narrator"],
      series: "Series Name",
      seriesNumber: 2.5,
      genres: ["Fiction"],
      tags: ["epic"],
      description: "...",
      publisher: "Publisher",
      imprint: "Imprint",
      url: "https://...",
      releaseDate: "2023-06-15T00:00:00Z",  // ISO 8601
      coverMimeType: "image/jpeg",
      coverData: arrayBuffer,                 // ArrayBuffer
      coverPage: 0,
      duration: 3661.5,                       // seconds (float)
      bitrateBps: 128000,
      pageCount: 42,
      identifiers: [{ type: "isbn_13", value: "9781234567890" }],
      chapters: [{
        title: "Chapter 1",
        startPage: 0,
        startTimestampMs: 0,
        href: "chapter1.xhtml",
        children: [{ title: "Section 1.1", startPage: 2 }]
      }]
    };
  }
}
```

**Go invocation:** `Manager.RunFileParser(ctx, rt, filePath, fileType) → *mediafile.ParsedMetadata`

### metadataEnricher (1 min timeout)

Enriches existing metadata from external sources. No file access beyond plugin dir.

```javascript
metadataEnricher: {
  enrich: function(context) {
    // context contains book info (title, authors, identifiers, etc.)
    var apiKey = shisho.config.get("apiKey");
    var resp = shisho.http.fetch("https://api.example.com/lookup?title=" + context.book.title, {});
    return {
      modified: true,
      metadata: { description: "...", genres: ["..."] }
    };
    // Or: return { modified: false }; to skip
  }
}
```

**Go invocation:** `Manager.RunMetadataEnricher(ctx, rt, enrichCtx) → *EnrichmentResult`

**Field filtering:** Enricher results are filtered before merging:
- Fields not declared in manifest → stripped + warning logged
- Fields declared but disabled by user → stripped silently
- Users configure field toggles globally and per-library via UI

### outputGenerator (5 min timeout)

Generates output files. Implements `filegen.Generator` interface via `PluginGenerator`.

```javascript
outputGenerator: {
  generate: function(context) {
    // context.sourcePath - source book file
    // context.destPath   - output destination
    // context.book       - book metadata object
    // context.file       - file metadata object
    var content = shisho.fs.readTextFile(context.sourcePath);
    shisho.fs.writeTextFile(context.destPath, transformed);
  },
  fingerprint: function(context) {
    // context.book, context.file
    // Return string for cache invalidation
    return "fp-" + context.book.title + "-" + context.file.fileType;
  }
}
```

**Go invocation:** `Manager.RunOutputGenerator(ctx, rt, sourcePath, destPath, bookCtx, fileCtx)` and `Manager.RunFingerprint(rt, bookCtx, fileCtx) → string`

## Host APIs (shisho.*)

### shisho.log

```javascript
shisho.log.debug(msg)
shisho.log.info(msg)
shisho.log.warn(msg)
shisho.log.error(msg)
```

### shisho.config

```javascript
shisho.config.get("apiKey")    // → string | undefined
shisho.config.getAll()         // → { key: value, ... }
```

### shisho.http

Mirrors the native `fetch()` Response API (synchronous since Goja has no Promises).

```javascript
var resp = shisho.http.fetch(url, { method: "GET", headers: {}, body: "" });
// Domain must be in manifest's httpAccess.domains

resp.ok          // boolean — true if status is 2xx
resp.status      // number — HTTP status code
resp.statusText  // string — HTTP status text
resp.headers     // Record<string, string> — response headers (lowercase keys)
resp.text()      // string — response body as text
resp.json()      // any — response body parsed as JSON (throws on invalid JSON)
resp.arrayBuffer() // ArrayBuffer — response body as raw bytes
```

**Domain patterns in `httpAccess.domains`:**
- Exact match: `"example.com"` only allows `example.com`
- Wildcard: `"*.example.com"` allows `example.com`, `api.example.com`, `a.b.example.com`

### shisho.url

URL utilities that aren't available in Goja's ES5.1 runtime.

```javascript
// Encode/decode URL components (like browser APIs)
shisho.url.encodeURIComponent("hello world")  // → "hello+world"
shisho.url.decodeURIComponent("hello+world")  // → "hello world"

// Build query strings from objects (keys sorted alphabetically)
shisho.url.searchParams({ q: "test", page: 1 })     // → "page=1&q=test"
shisho.url.searchParams({ tags: ["a", "b"] })       // → "tags=a&tags=b"

// Parse URL into components
var url = shisho.url.parse("https://api.example.com:8080/search?q=test#results");
url.protocol   // "https"
url.hostname   // "api.example.com"
url.port       // "8080"
url.pathname   // "/search"
url.search     // "?q=test"
url.hash       // "#results"
url.query      // { q: "test" }
url.query.q    // "test"
```

### shisho.fs

```javascript
shisho.fs.readFile(path)             // → ArrayBuffer
shisho.fs.readTextFile(path)         // → string
shisho.fs.writeFile(path, ab)        // ab: ArrayBuffer
shisho.fs.writeTextFile(path, str)
shisho.fs.exists(path)               // → boolean
shisho.fs.mkdir(path)                // creates parents
shisho.fs.listDir(path)              // → string[] (entry names)
shisho.fs.tempDir()                  // → string (lazy-created, auto-cleaned)
```

### shisho.archive

```javascript
shisho.archive.extractZip(archivePath, destDir)          // extract all entries
shisho.archive.createZip(srcDir, destPath)               // create zip from directory
shisho.archive.readZipEntry(archivePath, entryPath)      // → ArrayBuffer
shisho.archive.listZipEntries(archivePath)               // → string[]
```

### shisho.xml

```javascript
var root = shisho.xml.parse(xmlString)    // → XMLElement
var node = shisho.xml.querySelector(root, "metadata > title")   // → XMLElement | null
var nodes = shisho.xml.querySelectorAll(root, "item")           // → XMLElement[]

// XMLElement properties:
node.tag          // string — element tag name
node.namespace    // string — namespace URI
node.text         // string — direct text content
node.attributes   // Record<string, string>
node.children     // XMLElement[]
```

### shisho.ffmpeg

```javascript
// Requires ffmpegAccess capability declared in manifest

// Transcode files with FFmpeg
var result = shisho.ffmpeg.transcode(["-i", input, "-c:a", "aac", output]);
result.exitCode   // number — 0 = success
result.stdout     // string
result.stderr     // string

// Probe file metadata with ffprobe (returns parsed JSON)
var probe = shisho.ffmpeg.probe([filePath]);
probe.format      // { filename, duration, bit_rate, tags, ... }
probe.streams     // [{ codec_name, codec_type, sample_rate, channels, ... }]
probe.chapters    // [{ id, start_time, end_time, tags, ... }]
probe.stderr      // string — for debugging
probe.parseError  // string — empty if JSON parsed successfully

// Get FFmpeg version and configuration
var ver = shisho.ffmpeg.version();
ver.version       // string — e.g., "7.0"
ver.configuration // string[] — e.g., ["--enable-libx264", "--enable-gpl"]
ver.libraries     // { libavcodec: "60.31.102", ... }
```

### shisho.shell

```javascript
// Requires shellAccess capability with command in allowlist
var result = shisho.shell.exec("calibre-debug", ["-c", "print('hello')"]);
// Command must be declared in manifest shellAccess.commands
// Uses exec directly (no shell) to prevent injection

result.exitCode   // number — 0 = success
result.stdout     // string
result.stderr     // string
```

## Filesystem Sandbox (FSContext)

Each hook invocation creates an `FSContext` controlling access:

| Path | Read | Write |
|------|------|-------|
| Plugin's own directory | Always | Always |
| Hook-provided paths (sourcePath, targetDir, etc.) | Always | Always |
| Temp directory (lazy-created) | Always | Always |
| Anywhere else | Only if `fileAccess.level` is `"read"` or `"readwrite"` | Only if `"readwrite"` |

**Enrichers** get no extra allowed paths (only plugin dir + temp + fileAccess).

Temp dirs are auto-cleaned after hook returns.

## Data Source Priority

Lower number = higher priority. Higher priority overwrites lower.

| Priority | Source | Examples |
|----------|--------|----------|
| 0 | Manual | User edits |
| 1 | Sidecar | OPF sidecar files |
| 2 | Plugin | `plugin:shisho/goodreads` |
| 3 | File Metadata | `epub_metadata`, `cbz_metadata`, `m4b_metadata` |
| 4 | Filepath | Parsed from file path |

Plugin data sources use format `plugin:scope/id` (e.g., `plugin:shisho/goodreads-metadata`). The `models.PluginDataSource(scope, id)` helper creates these. Priority lookup uses prefix matching for `plugin:*` strings.

## Manager Lifecycle

| Method | When Called | What It Does |
|--------|------------|--------------|
| `LoadAll(ctx)` | Startup | Load all enabled plugins; errors stored, don't prevent others |
| `LoadPlugin(ctx, scope, id)` | Install/Enable | Load single plugin, inject APIs, register identifiers, append to order |
| `UnloadPlugin(scope, id)` | Uninstall/Disable | Remove from memory |
| `ReloadPlugin(ctx, scope, id)` | Update/Hot-reload | Write-lock old runtime, swap new, wait for in-progress hooks |
| `GetRuntime(scope, id)` | Any | Get loaded runtime (nil if not loaded) |
| `GetOrderedRuntimes(ctx, hookType)` | Scan pipeline | Get runtimes for hook in user-defined order |
| `GetParserForType(fileType)` | File scanning | First runtime with fileParser for type |
| `GetOutputGenerator(formatID)` | Output generation | PluginGenerator wrapping runtime |
| `CheckForUpdates(ctx)` | Periodic/on-demand | Check repos for newer versions |

## Thread Safety

- `Manager.mu` (RWMutex): protects `plugins` map
- `Runtime.mu` (RWMutex): Read lock for hook invocation, write lock for reload
- Hot-reload: acquire write lock on old runtime → swap in new → release

## Scan Pipeline Integration

In `pkg/worker/scan_unified.go`:

1. **File discovery** - `RegisteredFileExtensions()` and `RegisteredConverterExtensions()` determine which files to scan
2. **Input conversion** - For converter source types, `RunInputConverter()` converts to supported format
3. **File parsing** - `GetParserForType(ext)` finds plugin parser; validates MIME if declared; `RunFileParser()` extracts metadata
4. **Metadata application** - Plugin metadata applied with priority 2 (overwrites filepath, preserves manual/sidecar)
5. **Enrichment** - After file parsing, `GetOrderedRuntimes(ctx, "metadataEnricher")` runs enrichers in order. Uses a two-phase merge: enrichers merge into an empty `ParsedMetadata` (first non-empty wins among enrichers), then file-parsed metadata fills remaining gaps as fallback. This gives enrichers priority over file-embedded metadata per-field.

## Installation Flow

1. `POST /plugins/installed` with `{ scope, id }` or `{ downloadURL, sha256 }`
2. If no downloadURL → search enabled repositories for latest compatible version
3. Download ZIP from GitHub URL, verify SHA256
4. Extract to `{pluginDir}/{scope}/{id}/`
5. Parse manifest, insert DB record
6. `LoadPlugin()` → inject APIs, register identifiers, append to hook order
7. Load errors stored in DB but don't fail the install

## Repository System

Repositories provide a `repository.json` manifest:

```json
{
  "repositoryVersion": 1,
  "scope": "shisho",
  "name": "Official Plugins",
  "plugins": [{
    "id": "goodreads-metadata",
    "name": "...",
    "versions": [{
      "version": "1.0.0",
      "minShishoVersion": "0.1.0",
      "downloadUrl": "https://github.com/.../releases/download/.../plugin.zip",
      "sha256": "..."
    }]
  }]
}
```

**Security:** Only GitHub URLs allowed for downloads/repositories. SHA256 verification required.

## Database Tables

| Table | Key | Purpose |
|-------|-----|---------|
| `plugins` | `(scope, id)` | Installed plugin records |
| `plugin_configs` | `(scope, plugin_id, key)` | Configuration values (CASCADE delete) |
| `plugin_repositories` | `url` (unique `scope`) | Repository sources |
| `plugin_identifier_types` | `id` | Custom identifier types |
| `plugin_order` | `(hook_type, scope, plugin_id)` | Execution order per hook |
| `plugin_field_settings` | `(scope, plugin_id, field)` | Global field enabled/disabled state |
| `library_plugin_field_settings` | `(library_id, scope, plugin_id, field)` | Per-library field overrides |

**Field settings behavior:**
- No rows = all declared fields enabled by default
- Per-library settings fully override global (can enable or disable)
- Cascade deletes on plugin uninstall or library deletion

## API Endpoints

**Installation:**
- `GET /plugins/installed` - List installed
- `POST /plugins/installed` - Install
- `DELETE /plugins/installed/:scope/:id` - Uninstall
- `PATCH /plugins/installed/:scope/:id` - Enable/disable
- `POST /plugins/installed/:scope/:id/update` - Update version (hot-reload)
- `GET /plugins/installed/:scope/:id/config` - Get config schema + values + declaredFields + fieldSettings
- `POST /plugins/scan` - Scan local/ directory

**Field Settings:**
- `GET /plugins/installed/:scope/:id/fields` - Get global field settings
- `PUT /plugins/installed/:scope/:id/fields` - Set global field settings
- `GET /libraries/:libraryId/plugins/:scope/:id/fields` - Get per-library field settings
- `PUT /libraries/:libraryId/plugins/:scope/:id/fields` - Set per-library field settings
- `DELETE /libraries/:libraryId/plugins/:scope/:id/fields` - Reset to global defaults

**Repositories:**
- `GET /plugins/repositories` - List
- `POST /plugins/repositories` - Add
- `DELETE /plugins/repositories/:scope` - Remove (non-official)
- `POST /plugins/repositories/:scope/sync` - Sync manifest

**Available:**
- `GET /plugins/available` - From enabled repos
- `GET /plugins/available/:scope/:id` - Details

**Ordering:**
- `GET /plugins/order/:hookType` - Get order
- `PUT /plugins/order/:hookType` - Set order

## Frontend Hooks (app/hooks/queries/plugins.ts)

**Queries:** `usePluginsInstalled()`, `usePluginsAvailable()`, `usePluginOrder(hookType)`, `usePluginConfig(scope, id)`, `usePluginRepositories()`

**Mutations:** `useInstallPlugin()`, `useUninstallPlugin()`, `useUpdatePlugin()`, `useUpdatePluginVersion()`, `useSetPluginOrder()`, `useSavePluginConfig()`, `useSavePluginFieldSettings()`, `useScanPlugins()`, `useSyncRepository()`, `useAddRepository()`, `useRemoveRepository()`

**Note:** `usePluginConfig` returns `declaredFields` and `fieldSettings` for enrichers, displayed in `PluginConfigDialog`.

## Testing

**Test helpers** in `pkg/worker/scan_plugins_test.go`:
- `newTestContextWithPlugins(pluginDir)` - Creates test context with plugin manager
- `installTestPlugin(tc, pluginDir, id, manifestJSON, mainJS)` - Creates and registers test plugin on disk

**Test fixtures** in `pkg/plugins/testdata/`:
- `hooks-parser/`, `hooks-converter/`, `hooks-enricher/`, `hooks-generator/` - Working hook examples
- `simple-enricher/`, `multi-hook/` - Multi-capability examples
- `undeclared-hook/`, `missing-mainjs/`, `invalid-js/` - Error case fixtures

**Key test patterns:**
- Use `installTestPlugin()` to create minimal plugins inline
- Verify metadata fields individually after `RunFileParser()`
- Test MIME validation by declaring `mimeTypes` in manifest
- Test reserved extensions cannot be claimed
- Test enricher context receives correct book data

## Common Patterns

### Adding a new host API

1. Create `hostapi_newapi.go` with `injectNewAPINamespace(vm, shishoObj, rt)` function
2. Add call in `hostapi.go`'s `InjectHostAPIs()`
3. Add manifest capability type if needed (in `manifest.go`)
4. Add tests in `hostapi_newapi_test.go`
5. **Update `packages/plugin-types/host-api.d.ts`** — add the new interface and include it in `ShishoHostAPI`
6. If a new manifest capability was added, update `packages/plugin-types/manifest.d.ts`

### Adding a new hook type

1. Add Go constant in `pkg/models/plugin.go`
2. Add `goja.Value` field to `Runtime` struct
3. Extract in `LoadPlugin()`, validate against manifest
4. Add to `HookTypes()` list
5. Create `RunNewHook()` method on Manager
6. Add result parsing function
7. Integrate in scan pipeline or relevant service
8. **Update `packages/plugin-types/hooks.d.ts`** — add context/result interfaces and include the hook in `ShishoPlugin`
9. **Update `packages/plugin-types/manifest.d.ts`** if a new capability type was added

### Modifying ParsedMetadata or related structs

When changing `mediafile.ParsedMetadata`, `ParsedAuthor`, `ParsedIdentifier`, or `ParsedChapter`:

1. Update the Go struct in `pkg/mediafile/mediafile.go`
2. Update parsing in `pkg/plugins/hooks.go` (`parseParsedMetadata` and related functions)
3. **Update `packages/plugin-types/metadata.d.ts`** to match
4. Prefer adding new optional fields over changing/removing existing ones to avoid breaking plugins

### Writing a test plugin

```go
// File parser (no fields required)
manifest := `{"manifestVersion":1,"id":"test","name":"Test","version":"1.0.0","capabilities":{"fileParser":{"types":["pdf"]}}}`
mainJS := `var plugin=(function(){return{fileParser:{parse:function(ctx){return{title:"Test"}}}};})();`
installTestPlugin(tc, pluginDir, "test", manifest, mainJS)

// Enricher (fields required)
manifest := `{"manifestVersion":1,"id":"test-enricher","name":"Test Enricher","version":"1.0.0","capabilities":{"metadataEnricher":{"fileTypes":["epub"],"fields":["title","description"]}}}`
mainJS := `var plugin=(function(){return{metadataEnricher:{enrich:function(ctx){return{modified:true,metadata:{title:"Enriched"}}}}};})();`
installTestPlugin(tc, pluginDir, "test-enricher", manifest, mainJS)
```
