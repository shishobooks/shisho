---
sidebar_position: 3
---

# Development

This page covers everything you need to build a Shisho plugin, from project setup to the available APIs.

## Getting Started

A plugin consists of two files:

- **`manifest.json`** — declares the plugin's identity, capabilities, permissions, and configuration schema
- **`main.js`** — the JavaScript code that implements the plugin's hooks

### TypeScript Types Package

The `@shisho/plugin-types` npm package provides TypeScript type definitions for all plugin APIs. Install it for IDE autocompletion and type checking:

```bash
npm install @shisho/plugin-types
```

You can write plugins in TypeScript and compile to JavaScript, or use plain JavaScript with JSDoc annotations for type hints:

```javascript
/// <reference types="@shisho/plugin-types" />

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
| `duration` | `number` | Audiobook duration in seconds |
| `bitrateBps` | `number` | Audio bitrate in bits per second |
| `pageCount` | `number` | Page count |
| `identifiers` | `[{ type, value }]` | Identifiers (isbn_10, isbn_13, asin, uuid, etc.) |
| `chapters` | `[{ title, startPage?, startTimestampMs?, href?, children? }]` | Chapter list |

:::note[Field groupings for enrichers]
When declaring `fields` in an enricher manifest, some return fields are grouped under a single logical name:
- **`cover`** controls `coverData`, `coverMimeType`, and `coverPage`
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

Enhances existing book metadata from external APIs. Runs after file parsing during library scans.

**Timeout:** 1 minute

```javascript
var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(context) {
        // context.book  - book metadata (title, authors, etc.)
        // context.file  - file metadata (identifiers, etc.)

        var apiKey = shisho.config.get("apiKey");
        var query = shisho.url.encodeURIComponent(context.book.title);
        var resp = shisho.http.fetch(
          "https://api.example.com/search?q=" + query,
          { headers: { "Authorization": "Bearer " + apiKey } }
        );

        if (!resp.ok) {
          return { modified: false };
        }

        var data = resp.json();
        return {
          modified: true,
          metadata: {
            description: data.description,
            genres: data.genres
          }
        };
      }
    }
  };
})();
```

Return `{ modified: false }` to skip updating metadata for a book.

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
