# @shisho/plugin-types

TypeScript type definitions for [Shisho](https://github.com/shishobooks/shisho) plugin development. Provides IDE autocompletion and type checking for the `shisho.*` host APIs, hook contexts, metadata structures, and manifest schema.

## Installation

```bash
npm install --save-dev @shisho/plugin-types
```

## Usage

The types provide autocompletion for:

- **`shisho.*`** - Host APIs (log, config, http, fs, archive, xml, ffmpeg)
- **`plugin`** - Hook structure (inputConverter, fileParser, metadataEnricher, outputGenerator)
- **Hook contexts** - Typed `context` parameters for each hook method
- **Return types** - `ParsedMetadata`, `ConvertResult`, `EnrichmentResult`, etc.

### TypeScript

If you write your plugin in TypeScript (and compile to JavaScript for deployment), import the types directly:

```typescript
import type {
  EnrichmentResult,
  FileParserContext,
  MetadataEnricherContext,
  ParsedMetadata,
  ShishoPlugin,
} from "@shisho/plugin-types";

const plugin: ShishoPlugin = {
  fileParser: {
    parse(context: FileParserContext): ParsedMetadata {
      const content = shisho.fs.readTextFile(context.filePath);
      return {
        title: "My Book",
        authors: [{ name: "Author Name", role: "writer" }],
        identifiers: [{ type: "isbn_13", value: "9781234567890" }],
      };
    },
  },

  metadataEnricher: {
    enrich(context: MetadataEnricherContext): EnrichmentResult {
      const apiKey = shisho.config.get("apiKey");
      const resp = shisho.http.fetch(
        `https://api.example.com/lookup?title=${context.book.title}`,
        { method: "GET" },
      );
      const data = resp.json() as { description: string };
      return {
        modified: true,
        metadata: { description: data.description },
      };
    },
  },
};
```

### JavaScript with JSDoc

Add a triple-slash reference at the top of your `main.js` and use JSDoc annotations for type checking:

```javascript
/// <reference types="@shisho/plugin-types" />

var plugin = (function () {
  return {
    fileParser: {
      /** @param {FileParserContext} context @returns {ParsedMetadata} */
      parse: function (context) {
        var content = shisho.fs.readTextFile(context.filePath);
        return {
          title: "My Book",
          authors: [{ name: "Author Name" }],
        };
      },
    },

    metadataEnricher: {
      /** @param {MetadataEnricherContext} context @returns {EnrichmentResult} */
      enrich: function (context) {
        var apiKey = shisho.config.get("apiKey");
        var resp = shisho.http.fetch(
          "https://api.example.com/lookup?title=" + context.book.title,
          { method: "GET" },
        );
        var data = resp.json();
        return {
          modified: true,
          metadata: { description: data.description },
        };
      },
    },
  };
})();
```

## What's Included

| File            | Contents                                                              |
| --------------- | --------------------------------------------------------------------- |
| `global.d.ts`   | Global `shisho` and `plugin` variable declarations                    |
| `host-api.d.ts` | `ShishoHostAPI` and all namespace interfaces                          |
| `hooks.d.ts`    | Hook context/result types and `ShishoPlugin` interface                |
| `metadata.d.ts` | `ParsedMetadata`, `ParsedAuthor`, `ParsedIdentifier`, `ParsedChapter` |
| `manifest.d.ts` | `PluginManifest`, `Capabilities`, `ConfigSchema`                      |

## Links

- [Plugin Development Guide](https://github.com/shishobooks/shisho/blob/master/docs/plugins.md)
- [Shisho Repository](https://github.com/shishobooks/shisho)
