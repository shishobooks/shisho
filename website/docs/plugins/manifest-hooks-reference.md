# Manifest and Hooks Reference

This is the public reference for `manifest.json`, the global `plugin` object, hook contexts, and metadata returned by plugins. The TypeScript declarations are the source for compile-time contracts:

- [`manifest.d.ts`](https://github.com/shishobooks/shisho/blob/master/packages/plugin-sdk/manifest.d.ts)
- [`hooks.d.ts`](https://github.com/shishobooks/shisho/blob/master/packages/plugin-sdk/hooks.d.ts)
- [`metadata.d.ts`](https://github.com/shishobooks/shisho/blob/master/packages/plugin-sdk/metadata.d.ts)

Install them with `npm install --save-dev @shisho/plugin-sdk`. See [Developing Plugins](./development.md) for a build workflow.

## Artifact Layout

The plugin directory or release ZIP must put these files at its root:

```text
manifest.json
main.js
```

Additional files and directories are allowed. When `main.js` executes, it must create a non-null global object named `plugin`.

## Manifest Fields

```json
{
  "manifestVersion": 1,
  "id": "example-metadata",
  "name": "Example Metadata",
  "version": "1.2.0",
  "overview": "Finds metadata from Example Books.",
  "description": "Longer details shown on the plugin page.",
  "homepage": "https://github.com/example/example-metadata",
  "license": "MIT",
  "minShishoVersion": "0.0.49",
  "capabilities": {},
  "configSchema": {}
}
```

| Field | Required | Contract |
|-------|----------|----------|
| `manifestVersion` | Yes | Integer `1`. Other values are rejected. |
| `id` | Yes | Non-empty plugin identifier. Keep it stable across releases and match the repository entry and installation directory ID. |
| `name` | Yes | Non-empty display name. |
| `version` | Yes | Non-empty release version. Use a comparable semantic version for update handling. |
| `overview` | No | Short one-line summary, separate from the longer description. |
| `description` | No | Longer plugin description. |
| `homepage` | No | Plugin home or source URL. |
| `license` | No | License name or identifier. |
| `minShishoVersion` | No | Minimum compatible Shisho version. Loading fails as incompatible when the running version is older. |
| `capabilities` | No | Hook declarations, identifier registrations, and permissions. |
| `configSchema` | No | Administrator-configurable values available through `shisho.config`. |

A JavaScript hook without its corresponding capability causes plugin loading to fail. A declared capability without a JavaScript hook is ignored. An `onUninstalling` lifecycle callback does not need a manifest capability.

## Hook Capabilities

A plugin can declare and implement any combination of the four hook capabilities.

### Input Converter Capability

```json
{
  "capabilities": {
    "inputConverter": {
      "description": "Converts MOBI to EPUB",
      "sourceTypes": ["mobi"],
      "mimeTypes": ["application/x-mobipocket-ebook"],
      "targetType": "epub"
    }
  }
}
```

- `sourceTypes` is required and lists accepted file extensions without a leading dot.
- `targetType` is required and names the produced extension.
- `mimeTypes` and `description` are optional.

### File Parser Capability

```json
{
  "capabilities": {
    "fileParser": {
      "description": "Reads Example Book metadata",
      "types": ["example"],
      "mimeTypes": ["application/x-example-book"]
    }
  }
}
```

- `types` is required and lists extensions without a leading dot.
- `mimeTypes` and `description` are optional.
- `epub`, `cbz`, `m4b`, and `pdf` are reserved built-in extensions and cannot be claimed by a file parser.

When MIME types are present, Shisho compares compatible MIME values rather than requiring byte-for-byte string equality, so registered aliases and MIME parameters can match.

### Metadata Enricher Capability

```json
{
  "capabilities": {
    "metadataEnricher": {
      "description": "Searches Example Books",
      "fileTypes": ["epub", "m4b"],
      "fields": ["title", "authors", "description", "language", "abridged"]
    }
  }
}
```

- `fields` is required. Missing or empty fields disable only the enricher hook and produce a load warning.
- `fileTypes` is optional. Omit it to allow every file type.
- Invalid field names cause plugin loading to fail.

Valid declarations are:

```text
title, subtitle, authors, narrators, series, seriesNumber, genres, tags,
description, publisher, url, releaseDate, cover, identifiers, language,
abridged
```

`seriesNumber` is an alias for the full `series` group. `cover` and series grouping rules are described under [Grouped Metadata](#grouped-metadata).

### Output Generator Capability

```json
{
  "capabilities": {
    "outputGenerator": {
      "description": "Generates MOBI downloads",
      "id": "mobi",
      "name": "MOBI",
      "sourceTypes": ["epub"]
    }
  }
}
```

`id`, `name`, and `sourceTypes` are required. The ID is the stable output format identifier; name is its display label.

## Permission Capabilities

```json
{
  "capabilities": {
    "httpAccess": {
      "description": "Calls the Example Books API",
      "domains": ["api.example.com", "*.covers.example.com"]
    },
    "fileAccess": {
      "description": "Reads sidecars next to book files",
      "level": "read"
    },
    "ffmpegAccess": {
      "description": "Transcodes audio"
    },
    "shellAccess": {
      "description": "Runs the Example converter",
      "commands": ["example-convert"]
    }
  }
}
```

| Capability | Required Members | Effect |
|------------|------------------|--------|
| `httpAccess` | `domains: string[]` | Allows synchronous HTTP requests to exact domains or `*.example.com` wildcard patterns. Redirect targets are checked too. |
| `fileAccess` | `level: "read" \| "readwrite"` | Grants broad filesystem reads or reads and writes beyond paths supplied to a hook. |
| `ffmpegAccess` | None | Allows `shisho.ffmpeg.transcode`, `probe`, and `version`. |
| `shellAccess` | `commands: string[]` | Allows `shisho.shell.exec` only for listed command names. Arguments do not pass through a shell. |

Each capability can include an optional `description`. Review the precise path rules and APIs in the [Host API Reference](./host-api-reference.md).

## Custom Identifier Types

Register identifiers that Shisho can display and validate:

```json
{
  "capabilities": {
    "identifierTypes": [
      {
        "id": "example_books",
        "name": "Example Books",
        "urlTemplate": "https://example.com/book/{value}",
        "pattern": "^EB-[0-9]+$"
      }
    ]
  }
}
```

`id` and `name` are required. `urlTemplate` and validation `pattern` are optional. Use `{value}` as the value placeholder. Metadata can then return `{ "type": "example_books", "value": "EB-123" }`.

## Configuration Schema

`configSchema` maps arbitrary keys to form definitions:

```json
{
  "configSchema": {
    "apiKey": {
      "type": "string",
      "label": "API Key",
      "description": "Key issued by Example Books",
      "required": true,
      "secret": true
    },
    "maxResults": {
      "type": "number",
      "label": "Maximum Results",
      "default": 10,
      "min": 1,
      "max": 50
    },
    "strategy": {
      "type": "select",
      "label": "Strategy",
      "options": [
        { "value": "fast", "label": "Fast" },
        { "value": "thorough", "label": "Thorough" }
      ]
    }
  }
}
```

| Member | Required | Meaning |
|--------|----------|---------|
| `type` | Yes | `string`, `boolean`, `number`, `select`, or `textarea` |
| `label` | Yes | Display label |
| `description` | No | Help text |
| `required` | No | Marks the value required in the UI |
| `secret` | No | Masks the stored value when shown again |
| `default` | No | String, number, or boolean default |
| `min`, `max` | No | Numeric input limits |
| `options` | For useful selects | Array of `{ value, label }` string pairs |

At runtime, `shisho.config.get(key)` returns a configured string or `undefined`; `getAll()` returns string values. Parse booleans and numbers in plugin code when needed.

## Input Converter Hook

An input converter must complete within five minutes.

```typescript
interface InputConverterContext {
  sourcePath: string;
  targetDir: string;
}

interface ConvertResult {
  success: boolean;
  targetPath: string;
}
```

```javascript
plugin = {
  inputConverter: {
    convert: function (context) {
      var target = context.targetDir + "/converted.epub";
      // Produce target using shisho.fs, shisho.archive, FFmpeg, or an allowed command.
      return { success: true, targetPath: target };
    },
  },
};
```

Return the actual output path. Hook-provided source and target paths are available to filesystem and archive operations without broad `fileAccess`.

## File Parser Hook

A file parser must complete within one minute.

```typescript
interface FileParserContext {
  filePath: string;
  fileType: string;
}
```

`fileParser.parse(context)` returns `ParsedMetadata`:

```javascript
plugin = {
  fileParser: {
    parse: function (context) {
      return {
        title: "Example Book",
        authors: [{ name: "A. Writer", role: "writer" }],
        releaseDate: "2025-06-15",
        language: "en-US",
        identifiers: [{ type: "isbn_13", value: "9781234567890" }],
      };
    },
  },
};
```

The parser receives access to `filePath`. Return only metadata actually present or reliably derived.

## Metadata Enricher Hook

A metadata search must complete within one minute.

```typescript
interface SearchContext {
  query: string;
  author?: string;
  identifiers?: Array<{ type: string; value: string }>;
  file?: {
    fileType?: string;
    filePath?: string;
    duration?: number;
    pageCount?: number;
    filesizeBytes?: number;
  };
}

interface SearchResponse {
  results: ParsedMetadata[];
}
```

```javascript
plugin = {
  metadataEnricher: {
    search: function (context) {
      return {
        results: [
          {
            title: "Example Book",
            authors: [{ name: "A. Writer", role: "writer" }],
            description: "A matched edition.",
            coverUrl: "https://covers.example.com/123.jpg",
            confidence: 0.94,
          },
        ],
      };
    },
  },
};
```

`query` is always present. Other search hints are optional. When `file.filePath` is present, the enricher may read exactly that target file without broad `fileAccess`; it may not write it or read siblings unless `fileAccess` grants that access.

There is no separate enrich hook. Each result contains candidate metadata directly.

During automatic scans:

1. Shisho uses the first result.
2. A provided `confidence` below the effective threshold skips the result. Omitted confidence does not trigger threshold rejection.
3. Returned fields not declared by the manifest are removed. Disabled declared fields are also removed.
4. Enrichers run in administrator-defined order. The first enricher to provide a value for a field wins.
5. File-parsed metadata fills fields that no enricher supplied.

Interactive identification lets the user review candidate fields before applying them.

## Output Generator Hook

Output generation must complete within five minutes.

```typescript
interface OutputGeneratorContext {
  sourcePath: string;
  destPath: string;
  book: {
    id?: number;
    title?: string;
    subtitle?: string;
    description?: string;
    authors?: Array<{ name: string; role?: string }>;
    series?: Array<{ name: string; number?: number }>;
    genres?: string[];
    tags?: string[];
  };
  file: {
    id?: number;
    filepath?: string;
    fileType?: string;
    fileRole?: string;
    filesizeBytes?: number;
    name?: string;
    url?: string;
    publisher?: string;
    releaseDate?: string;
    narrators?: string[];
    identifiers?: Array<{ type: string; value: string }>;
  };
}
```

`outputGenerator.generate(context)` writes the output to `destPath` and returns nothing. `outputGenerator.fingerprint(context)` returns a string used to decide whether cached output remains valid:

```javascript
plugin = {
  outputGenerator: {
    generate: function (context) {
      // Write context.destPath.
    },
    fingerprint: function (context) {
      return String(context.book.title || "") + ":" +
        String(context.file.fileType || "");
    },
  },
};
```

Include every input that should invalidate generated output in the fingerprint.

## Parsed Metadata

All properties are optional unless stated by a nested type.

| Property | Type | Semantics |
|----------|------|-----------|
| `title`, `subtitle`, `description`, `publisher`, `url` | `string` | Text metadata. HTML is removed from descriptions when parsed. |
| `authors` | `ParsedAuthor[]` | Each author requires `name`; `role` is optional. |
| `narrators`, `genres`, `tags` | `string[]` | Resource names are resolved through Shisho's normal canonical names and aliases. |
| `series` | `string` | Series name. |
| `seriesNumber` | `number` | Finite series position, including fractional positions. |
| `seriesNumberEnd` | `number` | Optional inclusive omnibus range end, greater than `seriesNumber`. |
| `seriesNumberUnit` | `"volume" \| "chapter"` | Optional series unit. Chapter units apply to CBZ metadata. |
| `releaseDate` | `string` | `YYYY-MM-DD` or RFC3339 date. Invalid dates are ignored. |
| `language` | `string` | BCP 47 tag such as `en`, `en-US`, or `zh-Hans`; Shisho normalizes accepted tags. |
| `abridged` | `boolean` | `true` abridged, `false` unabridged, omitted unknown. |
| `coverMimeType` | `string` | MIME type for `coverData`. |
| `coverData` | `ArrayBuffer` | Raw image bytes, most useful for parsers extracting embedded covers. |
| `coverUrl` | `string` | Remote image URL. Its domain must be allowed by `httpAccess`. |
| `coverPage` | `number` | Zero-based page index for CBZ or PDF. |
| `duration` | `number` | Audiobook duration in seconds. |
| `bitrateBps` | `number` | Audio bitrate in bits per second. |
| `pageCount` | `number` | Page count. |
| `identifiers` | `ParsedIdentifier[]` | Each entry requires string `type` and `value`. Standard identifier values are canonicalized when stored. |
| `chapters` | `ParsedChapter[]` | Each chapter requires `title`; positions use `startPage`, `startTimestampMs`, or `href`, with optional nested `children`. |
| `confidence` | `number` | Enricher match score from `0` to `1`. |

Known author roles include `writer`, `penciller`, `inker`, `colorist`, `letterer`, `cover_artist`, `editor`, and `translator`. Omitting a role creates a generic author association.

### Grouped Metadata

The enricher field declaration `cover` controls `coverData`, `coverMimeType`, `coverUrl`, and `coverPage` together.

- For CBZ and PDF, return `coverPage`. Image data and image URLs are not applied.
- For other formats, return `coverData` or `coverUrl`. `coverData` takes precedence when both are present, and `coverPage` is ignored.
- An invalid or out-of-range page is skipped. During automatic enrichment, an external image must have greater total resolution than the existing file cover before it replaces it.

The `series` or `seriesNumber` declaration controls the series name and all three number properties. The number group is atomic: a finite start is required, a supplied end must be finite and greater than the start, and the unit must be `volume` or `chapter`. If any supplied member is invalid, Shisho discards the complete number group.

## Uninstall Lifecycle

The optional top-level `onUninstalling` function runs before an active plugin is removed:

```javascript
plugin = {
  fileParser: { parse: function () { return {}; } },
  onUninstalling: function () {
    shisho.log.info("Cleaning up Example Parser");
  },
};
```

Use it for best-effort cleanup such as revoking a token or deleting plugin-managed cache files. It receives no arguments and returns nothing. Errors do not stop uninstalling. Do not depend on this hook as the only way to preserve important user data.
