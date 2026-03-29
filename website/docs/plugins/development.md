---
sidebar_position: 3
---

# Development

This page covers everything you need to build a Shisho plugin, from project setup to the available APIs.

## Getting Started

A plugin consists of two files:

- **`manifest.json`** — declares the plugin's identity, capabilities, permissions, and configuration schema
- **`main.js`** — the JavaScript code that implements the plugin's hooks

### TypeScript SDK

The `@shisho/plugin-sdk` npm package provides TypeScript type definitions for all plugin APIs. Install it for IDE autocompletion and type checking:

```bash
npm install @shisho/plugin-sdk
```

You can write plugins in TypeScript and compile to JavaScript, or use plain JavaScript with JSDoc annotations for type hints:

```javascript
/// <reference types="@shisho/plugin-sdk" />

/** @type {ShishoPlugin} */
var plugin = {
  // your hooks here
};
```

### Plugin Code Pattern

All plugins must define a global `plugin` variable using an IIFE (Immediately Invoked Function Expression). The runtime is ES5.1 (no `const`, `let`, arrow functions, or template literals):

```javascript
var plugin = (function() {
  return {
    // hook implementations go here
  };
})();
```

## Manifest

The manifest declares what your plugin does, what permissions it needs, and what configuration it accepts.

### Required Fields

```json
{
  "manifestVersion": 1,
  "id": "my-plugin",
  "name": "My Plugin",
  "version": "1.0.0"
}
```

| Field | Description |
|-------|-------------|
| `manifestVersion` | Must be `1` |
| `id` | Unique identifier for the plugin (lowercase, hyphens allowed) |
| `name` | Display name shown in the UI |
| `version` | Semver version string |

### Optional Fields

| Field | Description |
|-------|-------------|
| `description` | Brief description of the plugin |
| `author` | Plugin author name |
| `homepage` | URL to plugin homepage or repository |
| `license` | License identifier (e.g., `MIT`) |
| `minShishoVersion` | Minimum compatible Shisho version |

### Capabilities

The `capabilities` object declares which hooks the plugin implements and what permissions it needs.

#### Hook Capabilities

Each hook type has its own capability declaration:

```json
{
  "capabilities": {
    "inputConverter": {
      "sourceTypes": ["mobi"],
      "targetType": "epub"
    },
    "fileParser": {
      "types": ["pdf"]
    },
    "metadataEnricher": {
      "fields": ["description", "genres", "cover"]
    },
    "outputGenerator": {
      "id": "mobi",
      "name": "MOBI",
      "sourceTypes": ["epub"]
    }
  }
}
```

A plugin can implement multiple hook types.

#### Permission Capabilities

```json
{
  "capabilities": {
    "httpAccess": {
      "domains": ["*.goodreads.com", "api.example.com"]
    },
    "fileAccess": {
      "level": "read"
    },
    "ffmpegAccess": {},
    "shellAccess": {
      "commands": ["calibre-debug", "kindlegen"]
    }
  }
}
```

| Permission | Description |
|------------|-------------|
| `httpAccess` | Domains the plugin can make HTTP requests to. Supports wildcards (`*.example.com`) |
| `fileAccess` | Filesystem access beyond the plugin's own directory. `"read"` or `"readwrite"` |
| `ffmpegAccess` | Access to FFmpeg for media processing |
| `shellAccess` | Allowlist of shell commands the plugin can execute |

#### Custom Identifier Types

Plugins can register custom identifier types (e.g., for Goodreads or MyAnonamouse IDs):

```json
{
  "capabilities": {
    "identifierTypes": [
      {
        "id": "goodreads",
        "name": "Goodreads",
        "urlTemplate": "https://www.goodreads.com/book/show/{value}",
        "pattern": "^\\d+$"
      }
    ]
  }
}
```

### Configuration Schema

Plugins can define user-configurable settings:

```json
{
  "configSchema": {
    "apiKey": {
      "type": "string",
      "label": "API Key",
      "description": "Your API key for the service",
      "required": true,
      "secret": true
    },
    "maxResults": {
      "type": "number",
      "label": "Max Results",
      "min": 1,
      "max": 100,
      "default": 10
    },
    "mode": {
      "type": "select",
      "label": "Lookup Mode",
      "options": [
        { "value": "fast", "label": "Fast" },
        { "value": "thorough", "label": "Thorough" }
      ]
    }
  }
}
```

Supported field types: `string`, `boolean`, `number`, `select`, `textarea`. Fields marked `secret` are masked in the UI.

### Validation Rules

- If your JavaScript exports a hook but the manifest doesn't declare the corresponding capability, the plugin **will fail to load**
- If the manifest declares a capability but the JavaScript doesn't export the hook, it's silently ignored
- The built-in file types (`epub`, `cbz`, `m4b`) cannot be claimed by file parsers
- Metadata enrichers **must** declare a `fields` array — if missing or empty, the enricher hook is disabled

## Hook Types

### Input Converter

Converts unsupported file formats to supported ones. Runs during library scans when a file with a matching source type is discovered.

**Timeout:** 5 minutes

```javascript
var plugin = (function() {
  return {
    inputConverter: {
      convert: function(context) {
        // context.sourcePath - path to the input file
        // context.targetDir  - directory to write the output file

        var targetPath = context.targetDir + "/output.epub";
        // ... perform conversion ...

        return { success: true, targetPath: targetPath };
      }
    }
  };
})();
```

**Manifest capability:**

```json
{
  "inputConverter": {
    "sourceTypes": ["mobi"],
    "targetType": "epub"
  }
}
```

### File Parser

Extracts metadata from file formats that Shisho doesn't natively support. Runs during library scans for files with matching extensions.

**Timeout:** 1 minute

```javascript
var plugin = (function() {
  return {
    fileParser: {
      parse: function(context) {
        // context.filePath - path to the file
        // context.fileType - file extension (e.g., "pdf")

        return {
          title: "Book Title",
          authors: [{ name: "Author Name" }],
          description: "A description of the book.",
          genres: ["Fiction"],
          identifiers: [{ type: "isbn_13", value: "9781234567890" }]
        };
      }
    }
  };
})();
```

The full set of fields you can return:

| Field | Type | Description |
|-------|------|-------------|
| `title` | `string` | Book title |
| `subtitle` | `string` | Book subtitle |
| `authors` | `[{ name, role? }]` | List of authors |
| `narrators` | `[string]` | List of narrator names |
| `series` | `string` | Series name |
| `seriesNumber` | `number` | Position in series (supports decimals like `1.5`) |
| `genres` | `[string]` | Genre names |
| `tags` | `[string]` | Tag names |
| `description` | `string` | Book description |
| `publisher` | `string` | Publisher name |
| `imprint` | `string` | Imprint name |
| `url` | `string` | Book URL |
| `releaseDate` | `string` | ISO 8601 date string |
| `coverData` | `ArrayBuffer` | Cover image data |
| `coverMimeType` | `string` | Cover MIME type (e.g., `image/jpeg`) |
| `coverPage` | `number` | Cover page index (0-indexed, for page-based formats) |
| `coverUrl` | `string` | Public URL for cover image. Server downloads at apply time. Domain must be in `httpAccess.domains`. |
| `duration` | `number` | Audiobook duration in seconds |
| `bitrateBps` | `number` | Audio bitrate in bits per second |
| `pageCount` | `number` | Page count |
| `identifiers` | `[{ type, value }]` | Identifiers (isbn_10, isbn_13, asin, uuid, etc.) |
| `chapters` | `[{ title, startPage?, startTimestampMs?, href?, children? }]` | Chapter list |

:::note[Field groupings for enrichers]
When declaring `fields` in an enricher manifest, some return fields are grouped under a single logical name:
- **`cover`** controls `coverData`, `coverMimeType`, `coverPage`, and `coverUrl`
- **`series`** controls both `series` and `seriesNumber`
:::

**Manifest capability:**

```json
{
  "fileParser": {
    "types": ["pdf"],
    "mimeTypes": ["application/pdf"]
  }
}
```

### Metadata Enricher

Searches external APIs for book metadata. The enricher implements a single `search()` hook that returns candidate results with complete metadata. Users can then review and selectively apply fields from the result they choose.

**Timeout:** 1 minute

#### Search Results

The `search()` hook returns `{ results: ParsedMetadata[] }` — the same metadata structure used by file parsers, with all fields populated:

```javascript
var plugin = (function() {
  return {
    metadataEnricher: {
      search: function(context) {
        // context.query       — search query (title or free text)
        // context.author      — author name (optional)
        // context.identifiers — [{ type, value }] (optional)

        // context.file — read-only file metadata for matching
        // context.file.fileType      — "epub", "cbz", "m4b", "pdf"
        // context.file.duration      — seconds (audiobooks only)
        // context.file.pageCount     — CBZ/PDF page count
        // context.file.filesizeBytes — file size in bytes

        var apiKey = shisho.config.get("apiKey");

        // Check for ISBN in query (users may paste ISBNs into the search box)
        var isbnMatch = context.query.match(/^97[89]\d{10}$/);
        if (isbnMatch) {
          // Direct ISBN lookup
        }

        // Use author to narrow results
        var searchUrl = "https://api.example.com/search?q=" + shisho.url.encodeURIComponent(context.query);
        if (context.author) {
          searchUrl += "&author=" + shisho.url.encodeURIComponent(context.author);
        }

        // Check for known identifiers
        if (context.identifiers) {
          for (var i = 0; i < context.identifiers.length; i++) {
            var id = context.identifiers[i];
            if (id.type === "isbn_13") {
              // Direct lookup by ISBN
            }
          }
        }

        var resp = shisho.http.fetch(
          searchUrl,
          { headers: { "Authorization": "Bearer " + apiKey } }
        );

        if (!resp.ok) return { results: [] };

        var data = resp.json();
        return {
          results: data.items.map(function(item) {
            return {
              title: item.title,
              subtitle: item.subtitle,
              authors: [{ name: item.author, role: "writer" }],
              narrators: item.narrators,
              series: item.seriesName,
              seriesNumber: item.seriesNumber,
              genres: item.genres,
              tags: item.tags,
              description: item.description,
              coverUrl: item.coverUrl,
              releaseDate: item.publishDate,
              publisher: item.publisher,
              imprint: item.imprint,
              url: item.url,
              identifiers: [{ type: "isbn_13", value: item.isbn }],
              confidence: item.matchScore  // optional, 0-1
            };
          })
        };
      }
    }
  };
})();
```

**Search result fields:**

| Field | Type | Description |
|-------|------|-------------|
| `title` | `string` | **Required.** Book title |
| `subtitle` | `string` | Subtitle or edition info |
| `authors` | `Array<{name, role?}>` | Authors with optional role (e.g., `"writer"`, `"illustrator"`) |
| `narrators` | `string[]` | Narrator names (audiobooks) |
| `series` | `string` | Series name |
| `seriesNumber` | `number` | Position in series |
| `genres` | `string[]` | Genre classification |
| `tags` | `string[]` | Freeform labels |
| `description` | `string` | Book description |
| `coverUrl` | `string` | Cover image URL. Server downloads at apply time. Domain must be in `httpAccess.domains`. |
| `releaseDate` | `string` | Publication date (`YYYY-MM-DD` or ISO 8601) |
| `publisher` | `string` | Publisher name |
| `imprint` | `string` | Imprint name |
| `url` | `string` | Web URL for the book |
| `identifiers` | `Array<{type, value}>` | ISBNs, ASINs, etc. |
| `confidence` | `number` | Optional match confidence score, 0–1 |

All fields except `title` are optional. The more fields you provide, the easier it is for users to pick the correct match and the more metadata can be applied.

#### Cover Images

Set `coverUrl` on your search results — the server handles downloading and domain validation automatically. The URL's domain must be in your manifest's `httpAccess.domains` list.

```javascript
return {
  results: [{
    title: "Book Title",
    coverUrl: "https://covers.example.com/book.jpg"
  }]
};
```

For advanced use cases (file parsers extracting embedded covers, or enrichers that generate/composite images), you can set `coverData` as an `ArrayBuffer` instead. If both are set, `coverData` takes precedence.

If both `coverUrl` and `coverData` are set, `coverData` takes precedence.

#### File Hints

The `context.file` object provides read-only metadata about the file being enriched. Use it to narrow your search — for example, filtering audiobook results by duration or distinguishing a comic from a novel by page count:

```javascript
search: function(context) {
  // context.file — read-only file metadata for matching
  // context.file.fileType      — "epub", "cbz", "m4b", "pdf"
  // context.file.duration      — seconds (audiobooks only)
  // context.file.pageCount     — CBZ/PDF page count
  // context.file.filesizeBytes — file size in bytes

  var searchUrl = "https://api.example.com/search?q=" + shisho.url.encodeURIComponent(context.query);

  // Narrow to audiobooks when enriching an M4B
  if (context.file && context.file.fileType === "m4b") {
    searchUrl += "&type=audiobook";
    if (context.file.duration) {
      searchUrl += "&minDuration=" + Math.floor(context.file.duration);
    }
  }

  // ...
}
```

#### Confidence Scores

Return a `confidence` value (0–1) on each result to tell Shisho how confident you are in the match:

```javascript
return {
  results: [{
    title: "The Great Book",
    confidence: 0.92,  // optional, 0-1
    // ... other metadata fields
  }]
};
```

**Auto-apply behavior during automatic scans:**

- Results with `confidence >= threshold` are auto-applied
- Results with `confidence` below threshold are skipped (logged as a warning)
- Results without a `confidence` field are always applied (backwards compatible)
- The threshold defaults to 85% and is configurable globally via `enrichment_confidence_threshold` in your server config, and per-plugin in the plugin settings

#### Enrichment Behavior

When a user identifies a book using the interactive review screen, they choose which fields to keep on a field-by-field basis. During automatic scans, the first search result's metadata is applied for all enabled fields.

Enricher values **override** file-embedded metadata for the same field. If a file has a bad title and your enricher returns a corrected title, the enricher's value wins. Among multiple enrichers, the first one (in user-defined order) to provide a value for a field wins. File-embedded metadata is only used as a fallback for fields that no enricher provided.

Only fields declared in the manifest's `fields` array will be applied. Any returned fields not in the declared list are silently stripped. Users can further disable individual fields in the plugin settings.

**Valid enricher fields:** `title`, `subtitle`, `authors`, `narrators`, `series`, `seriesNumber`, `genres`, `tags`, `description`, `publisher`, `imprint`, `url`, `releaseDate`, `cover`, `identifiers`

**Manifest capability:**

```json
{
  "metadataEnricher": {
    "fields": ["description", "genres", "cover"],
    "fileTypes": ["epub", "cbz"]
  }
}
```

The `fileTypes` filter is optional — omit it to enrich all file types.

### Output Generator

Generates alternative download formats from existing files. The generated files are cached and served when users download the alternative format.

**Timeout:** 5 minutes

```javascript
var plugin = (function() {
  return {
    outputGenerator: {
      generate: function(context) {
        // context.sourcePath - path to the source file
        // context.destPath   - path to write the output file
        // context.book       - book metadata
        // context.file       - file metadata

        // ... generate output file at context.destPath ...
      },
      fingerprint: function(context) {
        // context.book - book metadata
        // context.file - file metadata

        // Return a string used for cache invalidation.
        // When the fingerprint changes, the cached output is regenerated.
        return context.file.fileType + "-" + context.book.title;
      }
    }
  };
})();
```

**Manifest capability:**

```json
{
  "outputGenerator": {
    "id": "mobi",
    "name": "MOBI",
    "sourceTypes": ["epub"]
  }
}
```

## Host APIs

Plugins access Shisho's host APIs through the global `shisho` object.

### Logging

```javascript
shisho.log.debug("Debug message");
shisho.log.info("Info message");
shisho.log.warn("Warning message");
shisho.log.error("Error message");
```

### Configuration

```javascript
var apiKey = shisho.config.get("apiKey");  // returns string or undefined
var all = shisho.config.getAll();          // returns { key: value, ... }
```

### HTTP

Make HTTP requests to domains declared in the manifest's `httpAccess.domains`:

```javascript
var resp = shisho.http.fetch("https://api.example.com/search", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ query: "search term" })
});

resp.ok;           // boolean (true if status is 2xx)
resp.status;       // number
resp.statusText;   // string
resp.headers;      // { "content-type": "application/json", ... }

var text = resp.text();         // response body as string
var json = resp.json();         // response body parsed as JSON
var bytes = resp.arrayBuffer(); // response body as ArrayBuffer
```

Domain patterns support wildcards: `"*.example.com"` matches `example.com`, `api.example.com`, and `a.b.example.com`.

### URL Utilities

Helpers for URL manipulation that aren't available in the ES5.1 runtime:

```javascript
shisho.url.encodeURIComponent("hello world");  // "hello+world"
shisho.url.decodeURIComponent("hello+world");  // "hello world"

// Build query strings (keys sorted alphabetically)
shisho.url.searchParams({ q: "test", page: 1 });  // "page=1&q=test"

// Parse URLs
var url = shisho.url.parse("https://api.example.com:8080/search?q=test");
url.protocol;  // "https"
url.hostname;  // "api.example.com"
url.port;      // "8080"
url.pathname;  // "/search"
url.query;     // { q: "test" }
```

### Filesystem

Read and write files within the sandbox. Plugins always have access to their own directory and a temporary directory. Access to other paths requires the `fileAccess` capability.

```javascript
var content = shisho.fs.readTextFile("/path/to/file.txt");
shisho.fs.writeTextFile("/path/to/output.txt", content);

var bytes = shisho.fs.readFile("/path/to/file.bin");    // ArrayBuffer
shisho.fs.writeFile("/path/to/output.bin", bytes);

shisho.fs.exists("/path/to/file");   // boolean
shisho.fs.mkdir("/path/to/dir");     // creates parents
shisho.fs.listDir("/path/to/dir");   // ["file1.txt", "file2.txt"]

var tmpDir = shisho.fs.tempDir();    // auto-cleaned after hook returns
```

### Archives

Work with ZIP files:

```javascript
shisho.archive.extractZip("/path/to/archive.zip", "/path/to/dest");
shisho.archive.createZip("/path/to/source/dir", "/path/to/output.zip");

var entries = shisho.archive.listZipEntries("/path/to/archive.zip");
var data = shisho.archive.readZipEntry("/path/to/archive.zip", "file.txt");
```

### XML

Parse and query XML documents:

```javascript
var doc = shisho.xml.parse(xmlString);
var title = shisho.xml.querySelector(doc, "metadata > title");
var items = shisho.xml.querySelectorAll(doc, "item");

// XMLElement properties:
title.tag;         // element tag name
title.text;        // direct text content
title.attributes;  // { "attr": "value" }
title.children;    // child elements
```

### HTML

HTML parsing with full CSS selector support. Use this instead of regex for scraping HTML content.

```javascript
// Find a single element
var meta = shisho.html.querySelector(html, 'meta[name="description"]');
var description = meta ? meta.attributes.content : "";

// Find all matching elements
var items = shisho.html.querySelectorAll(html, '.book-item');

// Extract JSON-LD (common pattern for metadata enrichers)
var scripts = shisho.html.querySelectorAll(html, 'script[type="application/ld+json"]');
if (scripts.length > 0) {
  var jsonLd = JSON.parse(scripts[0].text);
}

// Extract Open Graph data
var ogTitle = shisho.html.querySelector(html, 'meta[property="og:title"]');
var title = ogTitle ? ogTitle.attributes.content : "";
```

Each returned element has:
- `tag` — element tag name (e.g., `"div"`, `"meta"`)
- `attributes` — key-value pairs (e.g., `{ name: "description", content: "..." }`)
- `text` — recursive inner text content
- `innerHTML` — raw inner HTML string
- `children` — child elements

### FFmpeg

Requires the `ffmpegAccess` capability:

```javascript
var result = shisho.ffmpeg.transcode(["-i", input, "-c:a", "aac", output]);
result.exitCode;  // 0 = success
result.stderr;    // FFmpeg output

var probe = shisho.ffmpeg.probe([filePath]);
probe.format;     // { duration, bit_rate, tags, ... }
probe.streams;    // [{ codec_name, sample_rate, ... }]
probe.chapters;   // [{ start_time, end_time, tags, ... }]
```

### Shell

Execute allowlisted commands. Requires the `shellAccess` capability with the command in the `commands` array:

```javascript
var result = shisho.shell.exec("calibre-debug", ["-c", "print('hello')"]);
result.exitCode;  // 0 = success
result.stdout;
result.stderr;
```

Commands run via `exec` directly (no shell) to prevent injection.

## Local Development

For development, you can load plugins directly from the filesystem without going through a repository.

1. Create your plugin directory at `{pluginDir}/local/{your-plugin-id}/`
2. Add your `manifest.json` and `main.js` files
3. In **Admin > Plugins**, click **Scan** to discover the plugin
4. Enable the plugin to start using it

The default plugin directory is `plugins/` relative to the Shisho data directory. You can change this with the `plugin_dir` configuration option.

### Hot Reload

During development, you can modify your plugin's files and click **Reload** in the admin interface to pick up changes without restarting the server.

## Testing Plugins

The SDK includes test utilities at `@shisho/plugin-sdk/testing` that eliminate mock boilerplate.

### Setup

```typescript
import { createMockShisho } from "@shisho/plugin-sdk/testing";

const mockShisho = createMockShisho({
  fetch: {
    "https://api.example.com/search?q=test": {
      status: 200,
      body: JSON.stringify({ results: [{ title: "Test Book" }] }),
    },
  },
  config: {
    api_key: "test-key",
  },
});

globalThis.shisho = mockShisho;
```

### What's Included

| API | Behavior |
|-----|----------|
| `log.*` | Silent no-ops |
| `url.*` | Real implementations (encodeURIComponent, searchParams, parse) |
| `config.*` | Returns values from the config map you provide |
| `http.fetch` | Route-based mock — matches URLs, throws on unmatched |
| `fs.*` | Path-based mock — virtual filesystem from the map you provide |

Unmatched fetch URLs and missing fs paths throw descriptive errors so you know exactly what mock data to add.

:::warning[Runtime Differences]
Plugins run in a **goja** runtime (ES5.1), but tests run in **Node.js**. Your tests may pass using ES6+ features (arrow functions, `const`/`let`, template literals, destructuring) that will fail in the actual plugin runtime. Always write your `main.js` using ES5.1 syntax (var, function expressions, string concatenation) — the test utilities mock the `shisho.*` APIs, not the JavaScript runtime itself.
:::
