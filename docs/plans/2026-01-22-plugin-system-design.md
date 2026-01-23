# Plugin System Design

This document describes the architecture for Shisho's plugin system, enabling third-party developers to extend functionality through JavaScript plugins.

## Overview

The plugin system allows users to install plugins that:
- Convert file formats to other formats (PDF → EPUB, EPUB → MOBI, etc.)
- Parse new file formats for metadata extraction
- Generate new output/download formats (MOBI, AZW3, etc.)
- Enrich metadata from external sources (Goodreads, OpenLibrary, etc.)
- Register custom identifier types

Plugins are JavaScript files executed in an embedded interpreter (goja). Plugin developers write TypeScript, bundle it to JavaScript, and distribute via GitHub.

## Architecture Decisions

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Runtime | goja (pure Go JS interpreter) | No CGO, good performance, ES2020 support (async/await, Promises, no ES modules) |
| Plugin language | JavaScript (TS bundled to JS) | Large developer pool, familiar tooling |
| Manifest format | JSON with camelCase | Idiomatic for JS/TS developers |
| Repository hosting | GitHub only | Security (no arbitrary domains), reliable CDN |
| Config storage | Database | UI-editable, survives plugin updates |
| Processing order | User-defined | Deterministic, explicit control |
| Load timing | App startup + hot-reload | Initial load at startup; install/update/enable hot-reloads without restart |
| File matching | Extensions + optional MIME types | Extensions for fast discovery during dir walk; MIME types for content validation after discovery |
| FFmpeg | Bundled binary, subprocess execution | LGPL-compatible with MIT app via process boundary; no CGO linking required |

## Versioning Strategy

Two version fields control plugin compatibility:

- **`manifestVersion`** - The plugin API contract version. When Shisho makes breaking changes to plugin APIs, it introduces a new manifest version. Shisho declares which manifest versions it supports; plugins with unsupported versions receive a clear incompatibility error. This is the primary compatibility mechanism.

- **`minShishoVersion`** - The minimum Shisho release required for a non-breaking feature dependency (e.g., "I need the archive API added in 1.3"). Does not imply breaking changes.

This means plugin authors don't need to predict future breaking changes (no `maxShishoVersion`). Shisho controls the compatibility boundary through manifest versions.

## Plugin Structure

A plugin is a directory containing:

```
my-plugin/
├── manifest.json      # Required: metadata, capabilities, config schema
├── main.js            # Required: bundled entry point
└── assets/            # Optional: icons, static files
```

### Manifest Format

```json
{
  "manifestVersion": 1,
  "id": "goodreads-metadata",
  "name": "Goodreads Metadata Provider",
  "version": "1.0.0",
  "description": "Fetches book metadata from Goodreads",
  "author": "Community Developer",
  "homepage": "https://github.com/example/goodreads-plugin",
  "license": "MIT",
  "minShishoVersion": "1.0.0",

  "capabilities": {
    "inputConverter": {
      "description": "Converts PDF files to EPUB for indexing",
      "sourceTypes": ["pdf"],
      "mimeTypes": ["application/pdf"],
      "targetType": "epub"
    },
    "metadataEnricher": {
      "description": "Fetches additional metadata from Goodreads API",
      "fileTypes": ["epub", "cbz", "m4b"]
    },
    "identifierTypes": [
      {
        "id": "goodreads",
        "name": "Goodreads ID",
        "urlTemplate": "https://www.goodreads.com/book/show/{value}",
        "pattern": "^[0-9]+$"
      }
    ],
    "httpAccess": {
      "description": "Calls Goodreads API to fetch book data",
      "domains": ["goodreads.com", "api.goodreads.com"]
    },
    "ffmpegAccess": {
      "description": "Uses FFmpeg to transcode audio files"
    }
  },

  "configSchema": {
    "apiKey": {
      "type": "string",
      "label": "Goodreads API Key",
      "description": "Your Goodreads developer API key",
      "required": true,
      "secret": true
    },
    "includeReviews": {
      "type": "boolean",
      "label": "Include Reviews",
      "default": false
    },
    "timeout": {
      "type": "number",
      "label": "Request Timeout (seconds)",
      "default": 30,
      "min": 5,
      "max": 120
    },
    "outputFormat": {
      "type": "select",
      "label": "Output Format",
      "options": [
        { "value": "epub", "label": "EPUB" },
        { "value": "mobi", "label": "MOBI" }
      ],
      "default": "epub"
    },
    "customTemplate": {
      "type": "textarea",
      "label": "Custom Template",
      "description": "Template for metadata formatting"
    }
  }
}
```

**Config validation:** Constraints (`required`, `min`, `max`, `options`) are validated when the user saves config in the UI. The save is rejected if validation fails. At plugin load time, config is read from the database as-is. If required fields are unset (user hasn't configured yet), the plugin still loads but the UI shows a "Needs configuration" badge. Plugins must handle `undefined` from `config.get()` gracefully.

**Secret handling:** Fields with `"secret": true` are stored as plain text in the database (SQLite is a local file; encryption at rest adds complexity without meaningful security for a self-hosted app). The `secret` flag affects only presentation: the UI renders a password input, and the GET API for plugin config returns `"***"` for secret fields (or `null` if unset) rather than the actual value. This prevents accidental exposure in the browser while keeping the implementation simple.

**Config schema evolution:** Config values are stored independently of the schema — the schema is only used for UI rendering and validation. When a plugin update changes its configSchema:
- Existing values for keys still in the schema are preserved.
- Values for removed keys remain in the database but are invisible in the UI. `config.get()` still returns them if the plugin code reads them (useful for backwards-compatible migrations within plugin code).
- New keys start as unset (`undefined` from `config.get()` until configured by the user).

### Capability Types

**v1 (Priority):**
- `inputConverter` - Convert file formats during scan
- `fileParser` - Parse new file formats for metadata extraction
- `outputGenerator` - Generate download formats
- `metadataEnricher` - Enrich metadata during sync
- `identifierTypes` - Register custom identifier types
- `httpAccess` - Make HTTP requests (with domain restrictions)
- `fileAccess` - Access library files (read or read-write)
- `ffmpegAccess` - Execute FFmpeg commands (subprocess)

**Future (v2+):**
- `apiEndpoints` - Register custom API routes
- `uiComponents` - Provide React components
- `sidecarFields` - Add custom sidecar fields

### MIME Type Validation

Plugins can optionally declare a `mimeTypes` array in their capabilities (`inputConverter`, `fileParser`). This provides a content-based validation layer on top of extension-based file discovery:

1. **Discovery (extensions):** During directory walking, files are discovered based on their extension matching a plugin's declared types (e.g., `sourceTypes: ["pdf"]` or `types: ["m4a"]`). This is fast — no file reads required.

2. **Validation (MIME types):** After discovery, if the plugin declares `mimeTypes`, the system reads the first 512 bytes of the file and detects the actual MIME type via magic byte signatures (using Go's `net/http.DetectContentType` or equivalent). If the detected type matches any entry in the `mimeTypes` array, the file is passed to the plugin. If not, the file is skipped with a debug log.

3. **Omitted (no validation):** When `mimeTypes` is not declared, all files matching the extension are passed to the plugin without content validation. This is the default behavior and appropriate for most plugins.

**When to use `mimeTypes`:** Useful when an extension is ambiguous (e.g., `.m4a` and `.m4b` are both MPEG-4 audio but serve different purposes) or when a plugin wants to guard against misnamed files. Not necessary for unambiguous formats like `.epub` or `.cbz`.

**MIME type matching:** Matching is prefix-based — `audio/mp4` matches a detected type of `audio/mp4` or `audio/mp4; codecs=...`. The detected type is normalized to lowercase before comparison.

### HTTP Access Rules

The `httpAccess` capability declares allowed domains:

```json
{
  "httpAccess": {
    "description": "Calls Goodreads API to fetch book data",
    "domains": ["goodreads.com", "api.goodreads.com"]
  }
}
```

Security rules:
- **Subdomains**: Must be listed explicitly. `goodreads.com` does not allow `api.goodreads.com`.
- **Redirects**: Requests that redirect to an unlisted domain are blocked. The plugin receives an error.
- **Ports**: Only standard ports (80/443) unless explicitly specified (e.g., `example.com:8080`).

**Why this is enforceable:** goja is a bare JS interpreter with no built-in network I/O — there's no native `fetch`, `XMLHttpRequest`, `http` module, or WebSocket. The only way plugins can make network requests is through the host-provided `shisho.http.fetch`, which enforces domain restrictions. npm packages that attempt network calls will fail at runtime since the underlying primitives don't exist. The same applies to `shisho.fs` — there's no `fs` module or file API available outside of what the host provides. FFmpeg (if `ffmpegAccess` is declared) has its network protocols disabled via `-protocol_whitelist file,pipe`, preventing it from being used as a network bypass.

### File Access Levels

The `fileAccess` capability controls access to the `shisho.fs.*` API for **arbitrary** library file operations. Without it, `shisho.fs` calls are restricted to the plugin's own directory and temp dirs.

Hook-provided paths (e.g., `sourcePath` in inputConverter, `filePath` in fileParser) are **always accessible** regardless of `fileAccess` — they're part of the hook contract. The host passes these paths directly to the plugin and grants implicit read access to them. A plugin only needs `fileAccess` if it wants to browse or modify library files beyond what the hook provides.

```json
{
  "fileAccess": {
    "level": "read",
    "description": "Reads related files in the same directory for metadata cross-referencing"
  }
}
```

Levels:
- `"read"` - Read-only access to library files via `shisho.fs`
- `"readwrite"` - Read and write access to library files via `shisho.fs`

The level is displayed to the user during installation so they understand what the plugin can do.

### FFmpeg Access

The `ffmpegAccess` capability grants access to `shisho.ffmpeg.run()`, which executes the bundled FFmpeg binary as a subprocess. This enables CPU-intensive media operations (transcoding, remuxing, format conversion) that would be impractical in a JavaScript interpreter.

```json
{
  "ffmpegAccess": {
    "description": "Uses FFmpeg to convert M4A files to M4B format"
  }
}
```

**How it works:** The host spawns FFmpeg as a child process with the provided arguments array. The plugin receives stdout, stderr, and exit code when the process completes. The call is async — the plugin awaits the result while FFmpeg runs.

**File path access:** Plugins should use paths they already have access to — hook-provided paths (`sourcePath`, `targetDir`), `shisho.fs.tempDir()`, or library paths (if `fileAccess` is declared). The host does not parse FFmpeg arguments to validate paths; the trust boundary is at the plugin installation level (the user reviewed and accepted the plugin's capabilities).

**Network restrictions:** FFmpeg's network protocol support is disabled via `-protocol_whitelist file,pipe` prepended to the argument list by the host. This prevents plugins from using FFmpeg as a network bypass (e.g., downloading via http/rtmp/rtsp inputs). Only local file and pipe I/O is permitted.

**Timeout:** FFmpeg invocations share the parent hook's timeout (e.g., 5 minutes for inputConverter). If the hook times out, the FFmpeg subprocess is killed via SIGTERM, then SIGKILL after 5 seconds.

**Licensing:** FFmpeg is bundled in the Docker image as a separate binary (LGPL 2.1+, compiled without `--enable-gpl`). Calling it as a subprocess does not create a derivative work — Shisho's MIT license is unaffected. The Docker image includes FFmpeg's license text and a source code reference in `/usr/share/licenses/ffmpeg/`. The LGPL build includes AAC, MP3, Opus, FLAC, and other common codecs sufficient for audiobook and music processing without requiring GPL components.

**Example usage (M4A → M4B converter):**

```typescript
export const inputConverter: shisho.InputConverter = {
  sourceTypes: ['m4a'],
  mimeTypes: ['audio/mp4', 'audio/x-m4a'],
  targetType: 'm4b',

  async convert(context: ConvertContext): Promise<ConvertResult> {
    const { sourcePath, targetDir } = context;
    const baseName = sourcePath.split('/').pop()!.replace(/\.[^.]+$/, '');
    const targetPath = `${targetDir}/${baseName}.m4b`;

    // Remux M4A to M4B (container change, no re-encoding)
    const result = await shisho.ffmpeg.run([
      '-i', sourcePath,
      '-c', 'copy',        // Copy streams without re-encoding
      '-f', 'mp4',         // Force MP4/M4B container
      targetPath
    ]);

    if (result.exitCode !== 0) {
      shisho.log.error(`FFmpeg failed: ${result.stderr}`);
      return { success: false };
    }

    return { success: true, targetPath };
  }
};
```

## Plugin APIs

Plugins access Shisho functionality through a global `shisho` object:

```typescript
declare namespace shisho {
  // Logging — logs go to the app logger with a structured `plugin` field (e.g., "shisho/goodreads-metadata").
  // During scan jobs, also captured in job logs. Log level filtering follows app config.
  namespace log {
    function debug(message: string): void;
    function info(message: string): void;
    function warn(message: string): void;
    function error(message: string): void;
  }

  // Configuration (plugin-specific settings, validated on save via UI)
  // Returns undefined for unset keys. Plugins should handle missing config gracefully.
  namespace config {
    function get<T>(key: string): T | undefined;
    function getAll(): Record<string, unknown>;
  }

  // HTTP client (respects httpAccess capability)
  namespace http {
    function fetch(url: string, options?: RequestOptions): Promise<Response>;
  }

  interface RequestOptions {
    method?: string;       // GET, POST, PUT, DELETE, etc. Default: GET
    headers?: Record<string, string>;
    body?: string | Uint8Array;
  }

  interface Response {
    ok: boolean;           // true if status 200-299
    status: number;
    statusText: string;
    headers: Record<string, string>;
    json(): Promise<any>;
    text(): Promise<string>;
    bytes(): Promise<Uint8Array>;
  }

  // File operations (respects fileAccess capability)
  namespace fs {
    function readFile(path: string): Promise<Uint8Array>;
    function readTextFile(path: string): Promise<string>;
    function writeFile(path: string, data: Uint8Array): Promise<void>;
    function writeTextFile(path: string, content: string): Promise<void>;
    function exists(path: string): Promise<boolean>;
    function mkdir(path: string): Promise<void>;
    function listDir(path: string): Promise<string[]>;
    // Returns a per-invocation temp directory. Same path if called multiple times
    // within one hook invocation. Cleaned up by the host after the hook completes
    // (regardless of success/failure). For persistent storage, use fileAccess.
    function tempDir(): string;
  }

  // Archive utilities (ZIP-only for v1; namespace allows future format expansion)
  // No separate capability required — follows the same path-level access rules as shisho.fs
  // (hook-provided paths, temp dirs, and library files if fileAccess is declared).
  namespace archive {
    function extractZip(archivePath: string, destDir: string): Promise<void>;
    function createZip(srcDir: string, destPath: string): Promise<void>;
    function readZipEntry(archivePath: string, entryPath: string): Promise<Uint8Array>;
    function listZipEntries(archivePath: string): Promise<string[]>;
  }

  // FFmpeg subprocess execution (requires ffmpegAccess capability)
  // Runs the bundled FFmpeg binary with the provided arguments.
  // The plugin is responsible for using accessible paths (hook-provided paths, tempDir(), or
  // library paths if fileAccess is declared). FFmpeg's network features (http/rtmp/etc. inputs)
  // are disabled via -protocol_whitelist (only file and pipe protocols are allowed).
  namespace ffmpeg {
    function run(args: string[]): Promise<FFmpegResult>;
  }

  interface FFmpegResult {
    exitCode: number;
    stdout: string;
    stderr: string;
  }

  // XML/HTML parsing
  // Selector syntax uses CSS Namespaces Level 3 (pipe notation: 'dc|title').
  // The namespaces map binds prefixes to URIs for the query.
  // Implemented in Go using encoding/xml parsing + custom selector matching.
  namespace xml {
    function parse(content: string): XMLDocument;
    function querySelector(doc: XMLDocument, selector: string, namespaces?: Record<string, string>): XMLElement | null;
    function querySelectorAll(doc: XMLDocument, selector: string, namespaces?: Record<string, string>): XMLElement[];
    // Usage: querySelector(doc, 'dc|title', { dc: 'http://purl.org/dc/elements/1.1/' })
  }

  interface XMLElement {
    tag: string;              // Local name (e.g., "title")
    namespace: string;        // Namespace URI (e.g., "http://purl.org/dc/elements/1.1/")
    attributes: Record<string, string>;
    text: string;             // Text content (concatenated text nodes)
    children: XMLElement[];
  }

  type XMLDocument = XMLElement;  // Root element
}
```

**Hook context types:** The `book` and `file` objects passed to hooks are read-only snapshots — modifications do not affect the database.

```typescript
interface BookContext {
  id: string;
  title: string;
  subtitle: string | null;
  authors: Array<{ name: string; role: string }>;
  narrators: string[];
  series: { name: string; number: number | null } | null;
  description: string | null;
  publisher: string | null;
  imprint: string | null;
  genres: string[];
  tags: string[];
}

interface FileContext {
  id: string;
  name: string;
  fileType: string;
  identifiers: Array<{ type: string; value: string }>;
  chapters: Array<{ title: string; startPage?: number; startTimestampMs?: number }>;
  duration: number | null;          // Seconds (audio formats)
  bitrateBps: number | null;        // Audio bitrate
  pageCount: number | null;         // Comic/document formats
  coverPage: number | null;         // 0-indexed (comic formats)
}
```

**Error propagation:** All async `shisho.*` functions return rejected promises on error (e.g., access denied, domain not allowed, file not found). The rejection value is an `Error` with a descriptive message. Sync functions (like `tempDir()`) throw on error. Plugin authors can use standard try/catch or `.catch()` patterns.

## Hook Interfaces

### Input Converter

Converts file formats to other formats during library scan. The converter writes a new file alongside the original. Both the source and target files are indexed independently based on parser availability:
- If a parser exists for the file's format (built-in or plugin), it's tracked as a main file with parsed metadata
- If no parser exists for the file's format, it's tracked as a supplement file (visible in UI, downloadable, but no parsed metadata)

This rule applies symmetrically — a converter's target file without a matching parser becomes a supplement, just like a source file without a parser. Converter success or failure does not affect how the original file is treated.

Multiple converters can handle the same source type as long as they target different formats. For example, a PDF→EPUB converter and a PDF→MOBI converter both produce their own output from the same PDF. However, for a given source→target pair, only the first converter in the user-defined order runs — subsequent converters for the same pair are skipped (same pattern as file parsers). The ordering UI's "Input Converters" tab controls this priority.

**targetDir lifecycle:**
- The host creates a temporary directory per conversion invocation and passes it as `targetDir`
- On success, the host moves the output file to the library alongside the source file
- On failure or exception, the host deletes the temp dir — no cleanup needed in plugin code
- If the destination filename already exists in the library (e.g., a converter from a different source type already produced the same file), the conversion is treated as a runtime error: logged and skipped

```typescript
export const inputConverter: shisho.InputConverter = {
  // Types this converter handles
  sourceTypes: ['pdf', 'mobi', 'doc'],

  // What it converts to
  targetType: 'epub',

  async convert(context: ConvertContext): Promise<ConvertResult> {
    const { sourcePath, targetDir } = context;

    // Read source file
    const data = await shisho.fs.readFile(sourcePath);

    // Plugin controls the output filename (useful for adding metadata like narrator)
    const sourceBaseName = sourcePath.split('/').pop().replace(/\.[^.]+$/, '');
    const targetPath = `${targetDir}/${sourceBaseName}.epub`;

    // ... conversion logic ...

    await shisho.fs.writeFile(targetPath, convertedData);

    return {
      success: true,
      targetPath: targetPath,
    };
  }
};
```

**Manifest capability:**

```json
{
  "capabilities": {
    "inputConverter": {
      "description": "Converts PDF and MOBI files to EPUB for indexing",
      "sourceTypes": ["pdf", "mobi"],
      "mimeTypes": ["application/pdf", "application/x-mobipocket-ebook"],
      "targetType": "epub"
    }
  }
}
```

**MIME type validation:** The optional `mimeTypes` array provides content-based validation after extension-based file discovery. When present, the system reads the file's magic bytes to detect its MIME type. If the detected MIME type matches any entry in the array, the file is passed to the converter. If it does not match, the file is skipped with a debug log (e.g., "file.pdf: detected MIME type image/png does not match inputConverter sourceTypes, skipping"). When `mimeTypes` is omitted, all files matching the declared extensions in `sourceTypes` are passed to the converter without content validation.

### File Parser

Extracts metadata from file formats. Use this when you want Shisho to natively support a format (track it, display metadata, allow downloads) without converting it.

```typescript
export const fileParser: shisho.FileParser = {
  // File extensions this parser handles (e.g., 'pdf', 'djvu')
  types: ['pdf', 'djvu'],

  async parse(context: FileParseContext): Promise<ParsedMetadata> {
    const { filePath } = context;
    const data = await shisho.fs.readFile(filePath);

    return {
      title: '...',
      subtitle: null,                    // Optional
      authors: [{ name: 'Author Name', role: '' }],  // role: '' for generic, or 'writer', 'penciller', etc.
      narrators: [],                     // Audiobook narrators
      series: null,                      // Series name
      seriesNumber: null,                // e.g., 1.5
      genres: [],                        // Genre names
      tags: [],                          // Tag names
      description: null,
      publisher: null,
      imprint: null,
      url: null,
      releaseDate: null,                 // ISO 8601 string, converted to time.Time by host
      coverData: null,                   // Uint8Array of image bytes
      coverMimeType: null,               // 'image/jpeg' or 'image/png'
      coverPage: null,                   // 0-indexed page for comic formats
      duration: null,                    // Seconds (float) for audio formats
      bitrateBps: null,                  // Audio bitrate
      pageCount: null,                   // Page count for comic/document formats
      identifiers: [],                   // [{ type: 'isbn_13', value: '...' }]
      chapters: [],                      // [{ title, startPage?, startTimestampMs?, href?, children? }]
    };
  }
};
```

All fields except `title` are optional — return only what your parser can extract. The Go host maps the JS object to the internal `ParsedMetadata` struct.

**Manifest capability:**

```json
{
  "capabilities": {
    "fileParser": {
      "description": "Parses PDF and DjVu files for metadata",
      "types": ["pdf", "djvu"],
      "mimeTypes": ["application/pdf", "image/vnd.djvu"]
    }
  }
}
```

**MIME type validation:** The optional `mimeTypes` array works identically to inputConverter — after extension-based discovery, the system validates the file's detected MIME type against this list. If the detected type matches any entry, the file is parsed. If it does not match, the file is skipped with a debug log. When `mimeTypes` is omitted, all files matching the declared extensions are passed to the parser without content validation.

**Scan integration:** Plugin file parsers run after the built-in parser switch (EPUB/CBZ/M4B). When a plugin registers a fileParser with `types: ['pdf', 'djvu']`, those extensions are added to the set of scannable file types (alongside `.epub`, `.cbz`, `.m4b`). Files with plugin-registered extensions are discovered during library walks, stored with the plugin-registered `FileType` value (e.g., `"pdf"`), and parsed by the plugin. The `FileType` is derived from the extension string in the plugin's `types` array — the plugin effectively registers new file type constants.

**Built-in extensions are reserved:** Plugin file parsers cannot override built-in parsers (epub, cbz, m4b). The built-in switch runs unconditionally for these extensions. If a plugin declares a built-in extension in its `types` array, that extension is ignored with a load-time warning. Plugins can only register parsers for extensions not claimed by built-in parsers.

**Conflicting plugin parsers:** If multiple plugins register a fileParser for the same extension, the plugin ordering (configured via the "File Parsers" tab in the ordering UI) determines which one runs. Only the first enabled parser for a given extension is used — subsequent parsers for the same extension are skipped. The UI shows a warning when multiple parsers claim the same extension so users understand the ordering matters.

### Output Generator

```typescript
export const outputGenerator: shisho.OutputGenerator = {
  id: 'mobi',
  name: 'MOBI (Kindle)',
  sourceTypes: ['epub'],

  // GenerateContext = { sourcePath: string, destPath: string, book: BookContext, file: FileContext }
  async generate(context: GenerateContext): Promise<void> {
    const { sourcePath, destPath, book, file } = context;
    // Convert source to output format, write to destPath
  },

  // Returns a string that changes when the output would differ.
  // Used by the download cache to invalidate stale entries.
  // FingerprintContext = { book: BookContext, file: FileContext }
  // Include config values if they affect output (e.g., shisho.config.get('quality')).
  fingerprint(context: FingerprintContext): string {
    const { book, file } = context;
    return `${file.id}:${book.title}:${shisho.config.get('quality')}`;
  }
};
```

The `sourceTypes` array determines which files can be converted to this format. The download API validates at request time: if the file's type isn't in the generator's `sourceTypes`, the request returns 400. The UI uses this to show only applicable download format options per file.

**Download integration:** Plugin output generators integrate with the download system the same way KePub does — as an additional format option. Specifically:

- When a library's `DownloadFormat` is set to `"ask"`, plugin-registered formats appear as options in the format selection UI alongside built-in formats (Original, KePub). Only formats whose `sourceTypes` include the file's type are shown.
- Plugin formats are also available as a `DownloadFormat` setting value, so users can set a plugin format as their default.
- OPDS feeds include plugin-generated formats as additional acquisition links (similar to how KePub links are added).
- The download cache calls the plugin's `fingerprint()` function to determine cache validity. If the fingerprint changes, a new cached version is generated.
- The download endpoint accepts the plugin generator's `id` as the format parameter (e.g., `/api/files/:id/download?format=mobi`).

### Metadata Enricher

Enrichers run in user-defined order (configured via the plugin ordering UI). Enrichment operates at the **field level** — each field is set independently by the first enricher to provide a non-empty value. If enricher A provides `description` and enricher B provides `genres`, both values are used. Once a field is set by an enricher, subsequent enrichers cannot overwrite it. Users should order their most trusted/preferred enricher first.

**Empty value rules:** A value is considered "empty" (and thus does not claim the field) if it is `null`, `undefined`, or `""` (empty string). Values like `false`, `0`, and `[]` are considered non-empty and will claim the field.

**Priority integration:** Enricher results use a new `DataSourcePlugin` priority level, which sits between file metadata and sidecar metadata in the existing priority system. The source is tracked as the specific plugin that set each field (e.g., `"plugin:shisho/goodreads-metadata"`), enabling the UI to show provenance per field. Manual user edits and sidecar metadata still take precedence over plugin-enriched values.

```typescript
export const metadataEnricher: shisho.MetadataEnricher = {
  name: 'Goodreads Metadata',
  fileTypes: ['epub', 'cbz', 'm4b'],

  // EnrichmentContext = { book: BookContext, file: FileContext, parsedMetadata: ParsedMetadata }
  // parsedMetadata is the fresh file parser output (same shape as fileParser.parse() returns).
  // book/file are the current DB state for comparison.
  async enrich(context: EnrichmentContext): Promise<EnrichmentResult> {
    const { book, file, parsedMetadata } = context;

    const isbn = parsedMetadata.identifiers.find(id => id.type === 'isbn');
    if (!isbn) return { modified: false };

    const apiKey = shisho.config.get<string>('apiKey');
    const response = await shisho.http.fetch(
      `https://api.goodreads.com/book/isbn/${isbn.value}?key=${apiKey}`
    );

    if (!response.ok) return { modified: false };
    const data = await response.json();

    return {
      modified: true,
      metadata: {
        description: data.description,
        genres: data.genres,
        identifiers: [{ type: 'goodreads', value: data.id }]
      }
    };
  }
};
```

**`modified` flag:** When `modified: false`, the host skips field inspection entirely and moves to the next enricher — this is an early-exit optimization for the common "nothing found" path. When `modified: true`, the host inspects each field in `metadata` individually, applying the "first non-empty wins" rule per field.

### Identifier Types

Identifier types are purely declarative — they are read from `manifest.json` only (no code export needed). The Go host registers them at load time from the manifest's `capabilities.identifierTypes` array.

### Multiple Hooks

A plugin can export multiple hooks:

```typescript
// PDF plugin with conversion, parsing, and generation
export const inputConverter: shisho.InputConverter = { /* ... */ };
export const fileParser: shisho.FileParser = { /* ... */ };
export const outputGenerator: shisho.OutputGenerator = { /* ... */ };
// identifierTypes are declarative — defined in manifest.json only, not exported from code
```

## Repository System

### Repository Manifest Format

Repositories are GitHub-hosted JSON files listing available plugins:

```json
{
  "repositoryVersion": 1,
  "scope": "shisho",
  "name": "Official Shisho Plugins",
  "plugins": [
    {
      "id": "goodreads-metadata",
      "name": "Goodreads Metadata",
      "description": "Fetches book metadata from Goodreads",
      "author": "Shisho Team",
      "versions": [
        {
          "version": "1.2.0",
          "minShishoVersion": "1.1.0",
          "releaseDate": "2025-01-15",
          "changelog": "Added series detection",
          "downloadUrl": "https://github.com/shishobooks/plugins/releases/download/goodreads-1.2.0/goodreads-metadata.zip",
          "sha256": "abc123..."
        }
      ],
      "hookTypes": ["metadataEnricher"],  // For plugin browser filtering (not security — that comes from actual manifest)
      "iconUrl": "https://..../icon.png"
    }
  ]
}
```

### URL Restrictions

For security, only GitHub URLs are allowed:
- Repository manifests: `https://raw.githubusercontent.com/{owner}/{repo}/...`
- Plugin downloads: `https://github.com/{owner}/{repo}/releases/download/...`

### Scope Assignment

The scope comes from the repository, not the plugin. When a user adds a repository:
1. Shisho fetches the manifest
2. Reads the suggested `scope` field
3. User confirms or edits the scope
4. Scope is stored with the repository

This prevents plugins from impersonating official sources.

### Official Repository

Pre-seeded on first run, cannot be removed:
```
URL: https://raw.githubusercontent.com/shishobooks/plugins/main/repository.json
Scope: shisho
Name: Official Shisho Plugins
```

Displayed with a star icon and "Official" tooltip in UI.

## Storage

### Directory Structure

```
/config/plugins/
├── installed/
│   ├── shisho/                         # Official repo scope
│   │   ├── goodreads-metadata/
│   │   │   ├── manifest.json
│   │   │   └── main.js
│   │   └── kindle-sync/
│   ├── awesome-plugins/                # Third-party repo scope
│   │   └── goodreads-metadata/         # Same ID, different scope = OK
│   └── local/                          # Manual installs
│       └── my-test-plugin/
└── cache/                              # Temp files per plugin
```

### Database Schema

```sql
-- Plugin repositories
CREATE TABLE plugin_repositories (
    url TEXT PRIMARY KEY,
    scope TEXT NOT NULL UNIQUE,
    name TEXT,
    is_official BOOLEAN NOT NULL DEFAULT false,
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_fetched_at TIMESTAMP,
    fetch_error TEXT
);

-- Installed plugins
CREATE TABLE plugins (
    scope TEXT NOT NULL,
    id TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    description TEXT,
    author TEXT,
    homepage TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    installed_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP,
    load_error TEXT,
    update_available_version TEXT,
    PRIMARY KEY (scope, id)
);

-- Plugin configuration values
CREATE TABLE plugin_configs (
    scope TEXT NOT NULL,
    plugin_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT,
    PRIMARY KEY (scope, plugin_id, key),
    FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
);

-- Plugin-registered identifier types
CREATE TABLE plugin_identifier_types (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    plugin_id TEXT NOT NULL,
    name TEXT NOT NULL,
    url_template TEXT,
    pattern TEXT,
    FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
);

-- Plugin processing order per hook type
-- New plugins are appended to the end (MAX(position) + 1) for each hook type they provide.
-- Users can reorder via the UI to change priority.
CREATE TABLE plugin_order (
    hook_type TEXT NOT NULL,
    scope TEXT NOT NULL,
    plugin_id TEXT NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (hook_type, scope, plugin_id),
    FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
);
```

## Plugin Lifecycle

### Hook Execution During Scan

Plugins execute at specific points during the library scan process:

```
1. Discover files in library → initial file list (extensions from built-in + plugin fileParsers)
2. Run inputConverters on discovered files
   ├─ MIME validation: if converter declares mimeTypes, detect file's MIME type via magic bytes
   │   ├─ Match → proceed with conversion
   │   └─ No match → skip with debug log, try next converter in order
   ├─ For each source→target pair, only the first converter in order runs
   ├─ Different target types run independently (e.g., PDF→EPUB and PDF→MOBI both produce output)
   └─ Newly created files added back to scan list for full processing
3. Detect file types (built-in + plugin fileParsers)
   └─ MIME validation: if parser declares mimeTypes, detect file's MIME type via magic bytes
       ├─ Match → proceed with parsing
       └─ No match → skip with debug log, try next parser in order
4. Parse metadata from detected files
   └─ Files with no parser tracked as supplement (visible, downloadable, no metadata)
5. Run metadataEnrichers on parsed files
6. Save to database
```

**Note:** `outputGenerator` hooks are **not** part of the scan flow. They run on-demand when a user requests a download in a plugin-registered format, using the same lazy-generation pattern as built-in formats (via the download cache).

### Startup Loading

```
1. App starts
2. Load plugin records from database
3. Scan /config/plugins/installed/ for plugin directories
4. For each enabled plugin:
   a. Read manifest.json
   b. Validate manifest version and minShishoVersion
   c. Create isolated goja runtime context
   d. Execute main.js (IIFE assigns exports to `plugin` global)
   e. Read hook implementations from `plugin` global, validate against declared capabilities
   f. Register hooks with plugin manager
   g. On error: log, store in plugins.load_error, continue
5. Sort hooks by user-defined order
6. Continue normal app startup
```

### Installation Flow

```
1. User clicks "Install" in plugin browser
2. Download plugin zip from GitHub release URL
3. Verify SHA256 checksum
4. Extract to /config/plugins/installed/{scope}/{plugin-id}/
5. Read manifest.json, validate structure and manifestVersion
   └─ If manifestVersion is unsupported: fail with "Requires a newer version of Shisho"
6. Show capabilities and security warning to user (generated from actual manifest.json)
7. User confirms
8. Insert row into plugins table (enabled = true)
9. Hot-reload: create runtime, execute main.js, register hooks — plugin is active immediately
```

If the user declines at step 7, the extracted files are cleaned up. The plugin browser UI only shows installable versions — plugins with no compatible version (all versions require an unsupported manifestVersion) are greyed out with a note like "Requires Shisho v2.0+".

**New file type prompt:** If the installed plugin provides a `fileParser` hook, the UI shows a prompt after installation: "This plugin adds support for new file types (.pdf, .djvu). Run a library scan to index existing files?" with a "Scan Now" button. No automatic rescan — the user stays in control.

### Uninstallation Flow

```
1. User clicks "Uninstall" in plugin management UI
2. UI shows warning: "Files processed by this plugin will retain their current metadata."
3. User confirms
4. Acquire plugin write lock (wait for in-progress hooks to complete)
5. Deregister hooks from plugin manager
6. Delete plugin_identifier_types rows (file_identifiers preserved as unlinked)
7. Delete plugin_order rows for this plugin
8. Delete plugin_configs rows (CASCADE)
9. Delete plugins row
10. Remove plugin directory from /config/plugins/installed/{scope}/{plugin-id}/
11. Release lock, tear down runtime
```

No data reprocessing occurs. Files parsed by the plugin retain their metadata and file type (but become un-reparseable until reinstalled). Enricher-sourced metadata fields are preserved. Converted files remain in the library.

### Update Flow

```
1. Daily job fetches all repository manifests
2. Compares installed versions to available versions
3. Filters: only versions with a manifestVersion supported by this Shisho release are considered
4. Sets plugins.update_available_version for the latest compatible version (if newer than installed)
5. UI shows "Update available" badge
6. User clicks update
7. Download new version, verify SHA256, replace files
8. Hot-reload: new version loaded without restart
```

If a plugin's only newer versions require a manifestVersion that Shisho doesn't support, no update is shown. The user sees a note like "Newer versions available but require Shisho v2.0+" in the plugin details.

### Hot-Reload Mechanics

Plugins are loaded and unloaded without requiring an app restart:

```
1. Create new goja runtime for the plugin
2. Execute new main.js, extract and validate hooks
3. Acquire plugin's write lock (blocks until all in-progress hook calls complete)
4. Swap hook references in plugin manager
5. Release write lock
6. Tear down old runtime
7. Re-register identifier types (diff and update database):
   - New types: insert into plugin_identifier_types
   - Removed types: delete from plugin_identifier_types (existing file_identifiers rows are preserved as unlinked — displayed as plain text without URL template or validation)
8. Update hook ordering (diff old vs new hooks):
   - New hook types: append to plugin_order (MAX(position) + 1)
   - Removed hook types: delete from plugin_order, compact remaining positions (shift down to eliminate gaps)
```

**Enable/disable:** Disabling a plugin performs a full unload — hooks are deregistered and the goja runtime is torn down (freeing memory). Enabling performs a full load — a fresh runtime is created, main.js is executed, and hooks are registered. The plugin's database row, config, and ordering position are preserved across disable/enable cycles.

**Concurrency mechanism:** Each plugin has a `sync.RWMutex`. Hook invocations acquire a read lock; hot-reload acquires a write lock. This means a reload blocks until all in-progress hooks finish (up to the hook's timeout), then subsequent invocations use the new runtime. A long-running conversion (up to 5 min) will delay the reload, which is acceptable since the user explicitly triggered the update.

This works because plugins are stateless—configuration lives in the database, not in the runtime.

## API Endpoints

```
# Repositories
GET    /api/plugins/repositories              # List configured repos
POST   /api/plugins/repositories              # Add repo {url, scope}
DELETE /api/plugins/repositories/:scope       # Remove repo (not official)
POST   /api/plugins/repositories/:scope/sync  # Refresh manifest

# Available plugins (from repos)
GET    /api/plugins/available                 # List from all repos
GET    /api/plugins/available/:scope/:id      # Plugin details + versions

# Installed plugins
GET    /api/plugins/installed                 # List installed
POST   /api/plugins/installed                 # Install {scope, id, version?}
DELETE /api/plugins/installed/:scope/:id      # Uninstall
PATCH  /api/plugins/installed/:scope/:id      # Update enabled, config
POST   /api/plugins/installed/:scope/:id/update  # Update to latest

# Plugin ordering
GET    /api/plugins/order/:hookType           # Get order
PUT    /api/plugins/order/:hookType           # Set order [{scope, id}, ...]

# Manual scanning
POST   /api/plugins/scan                      # Scan for manual installs
```

## UI Pages

### Plugin Browser (`/settings/plugins/browse`)
- Search/filter available plugins from all repos
- Shows name, description, author, hook types, install status
- "Install" button (downloads, then shows capabilities confirmation from actual manifest)

### Installed Plugins (`/settings/plugins/installed`)
- List with enable/disable toggles
- Load errors displayed with red warning
- "Configure" button for plugin settings
- "Update available" badge
- "Uninstall" button

**Job logs integration:** The job logs UI gains a plugin filter dropdown, allowing users to view logs from a specific plugin. Plugin logs carry a structured `plugin` field (`"scope/id"`) for filtering.

### Plugin Ordering (`/settings/plugins/order`)
- Tabs per hook type (Input Converters, Metadata Enrichers, File Parsers, Output Generators)
- Drag-and-drop reordering
- Only shows enabled plugins providing that hook

### Repositories (`/settings/plugins/repositories`)
- List with scope, URL, last sync, status
- Official repo marked with star, cannot be removed
- "Add Repository" form with scope confirmation
- Duplicate scope prevention: if the suggested scope conflicts with an existing one, the user must choose a different scope before adding

### Security Warnings

Displayed during installation (after download, from the actual manifest.json), tailored to the plugin's declared capabilities:

```
⚠️ This plugin requests the following permissions:

• Network access to: goodreads.com, api.goodreads.com
• Read access to library files
• FFmpeg execution (can run audio/video processing commands)
• Metadata enrichment (can modify book metadata)

Only install plugins from sources you trust.
```

The warning is generated from the plugin's manifest capabilities. Plugins without `httpAccess` show no network line; plugins without `fileAccess` show no filesystem line. A generic fallback is shown in repository management:

```
⚠️ Third-party plugins can request access to network, files,
and library data. Review each plugin's permissions before installing.
```

## Error Handling

### Load-time Errors

| Error | Display | Action |
|-------|---------|--------|
| Invalid manifest.json | Parse error details | Disable plugin |
| Missing main.js | "Entry point not found" | Disable plugin |
| Runtime init error | Stack trace | Disable plugin |
| Incompatible version | "Requires Shisho v{x}" | Disable plugin |
| Undeclared capability | "Uses {hook} but doesn't declare it" | Disable plugin |

### Runtime Errors

| Error | Behavior |
|-------|----------|
| Converter exception | Log, skip conversion. Original still indexed normally (main if parser exists, supplement if not). |
| Enricher exception | Log, continue to next enricher |
| Parser exception | Log, skip file with error status |
| Generator exception | Log, return error to user (500 response with plugin name and error message) |
| HTTP timeout | Plugin receives error, handles it |

Runtime errors are captured in job logs (since converters, enrichers, and parsers run during scan jobs). Generator errors are surfaced directly to the user in the API response. Load-time errors are stored in `plugins.load_error` for display in the plugin management UI.

### Isolation and Timeouts

Each plugin runs in its own goja runtime. A crash or panic in one plugin does not affect others or the host application.

Execution timeouts per hook type:

| Hook Type | Timeout |
|-----------|---------|
| inputConverter | 5 minutes |
| outputGenerator | 5 minutes |
| fileParser | 1 minute |
| metadataEnricher | 1 minute |

When a timeout is reached, the plugin receives a context cancellation and should clean up gracefully. The operation is treated as a runtime error (see table above).

**Known v1 limitation:** There is no memory sandboxing. A misbehaving plugin could allocate unbounded memory within the host process. Timeouts protect against CPU exhaustion, but memory limits are deferred to a future version. Users can disable plugins that cause performance issues.

## Developer Experience

### Provided by Shisho

1. **TypeScript type definitions** (`@shisho/plugin-types` on npm)
2. **Plugin template repository** (`github.com/shishobooks/plugin-template`)
3. **Documentation** with guides and API reference

### Plugin Template Structure

```
plugin-template/
├── src/
│   └── main.ts
├── manifest.json
├── package.json
├── tsconfig.json
├── build.js
└── README.md
```

### Build Configuration

```javascript
// build.js
require('esbuild').build({
  entryPoints: ['src/main.ts'],
  bundle: true,
  outfile: 'dist/main.js',
  format: 'iife',
  globalName: 'plugin',
  platform: 'neutral',
  target: 'es2020',
});
```

This bundles the TypeScript source (which uses `export const`) into an IIFE that assigns all exports to `var plugin = { inputConverter: {...}, fileParser: {...}, ... }`. The Go host evaluates the script then reads hook implementations from the `plugin` global object in the goja runtime. Plugin developers write standard `export const` in TypeScript — the IIFE wrapping is transparent.

### Local Development

```bash
cd my-plugin/
npm install
npm run build
cp -r dist/main.js manifest.json /config/plugins/installed/local/my-plugin/
# Run POST /api/plugins/scan to detect, then enable in UI (hot-reloads automatically)
```

**Manual scan behavior:** `POST /api/plugins/scan` walks `/config/plugins/installed/local/` for directories not already in the `plugins` table with scope `"local"`. For each new directory, it reads and validates `manifest.json`, inserts a row with `enabled = false`, and returns the list of newly discovered plugins. It only operates on the `local` scope — repo-installed plugins are managed through the install/update APIs. Local plugins use the `"local"` scope, so a `local/foo` plugin coexists with a `shisho/foo` plugin without conflict.

## Implementation Phases

### Phase 1: Core Infrastructure
- Plugin manager with goja runtime
- Manifest parsing and validation
- Database tables and migrations
- Basic API endpoints (list, install, enable/disable)
- Configuration loading from database
- Hot-reload mechanics (load/unload without restart)

### Phase 2: Hook Implementations
- `inputConverter` integration in scan worker (convert before indexing)
- `metadataEnricher` integration in scan worker
- `fileParser` integration in file detection
- `outputGenerator` integration in download system
- `identifierTypes` registration and storage

### Phase 3: Repository & UI
- Repository fetching and caching
- GitHub URL validation
- Plugin browser UI
- Plugin management UI
- Plugin ordering UI
- Security warning dialogs

### Phase 4: Polish
- Daily update check job
- Error display in UI
- TypeScript type definitions package
- Plugin template repository
- Developer documentation
