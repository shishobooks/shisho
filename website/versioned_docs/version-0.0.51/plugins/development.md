# Developing Plugins

A Shisho plugin is a JavaScript artifact with a manifest. This guide builds a minimal file parser, loads it locally, and tests it in Shisho.

For exact types and edge cases, use the [Manifest and Hooks Reference](./manifest-hooks-reference.md), [Host API Reference](./host-api-reference.md), and declarations shipped in `@shisho/plugin-sdk`.

## Create a Project

TypeScript plus a bundler is recommended. Install the SDK as a development dependency so it does not become part of the runtime artifact:

```bash
mkdir shisho-example-plugin
cd shisho-example-plugin
npm init -y
npm install --save-dev @shisho/plugin-sdk typescript esbuild
mkdir src dist
```

A source project may contain any files your toolchain needs. The built plugin directory must have these files at its root:

```text
dist/
  manifest.json
  main.js
```

`manifest.json` declares identity, compatibility, hooks, configuration, and permissions. `main.js` must create a global object named `plugin`. Additional runtime files are allowed when your plugin needs them.

Shisho does not provide Node.js APIs, npm package loading, browser DOM APIs, Promises, or timers to plugin code. Bundle dependencies into `main.js` when they can run in the Shisho runtime, and use the synchronous [`shisho` host API](./host-api-reference.md) for supported host operations.

An IIFE is a common bundler output pattern, not a runtime requirement by itself. The runtime requirement is that executing `main.js` creates the global `plugin` object. Do not depend on a particular ECMAScript version without testing the emitted bundle in Shisho, and do not assume that code accepted by Node.js will be accepted by the plugin runtime.

## Add a Manifest

Create `manifest.json` in the project root:

```json
{
  "manifestVersion": 1,
  "id": "example-parser",
  "name": "Example Parser",
  "version": "0.1.0",
  "overview": "Reads titles from Example Book files.",
  "description": "A minimal parser used to demonstrate plugin development.",
  "capabilities": {
    "fileParser": {
      "description": "Parses .example files",
      "types": ["example"]
    }
  }
}
```

The built-in `epub`, `cbz`, `m4b`, and `pdf` extensions are reserved and cannot be claimed by a file parser. See [Manifest Fields](./manifest-hooks-reference.md#manifest-fields) for the complete schema.

## Implement a Hook

Create `src/main.ts`:

```typescript
import type {
  FileParserContext,
  ParsedMetadata,
  ShishoPlugin,
} from "@shisho/plugin-sdk";

const implementation: ShishoPlugin = {
  fileParser: {
    parse(context: FileParserContext): ParsedMetadata {
      const content = shisho.fs.readTextFile(context.filePath);
      const firstLine = content.split("\n")[0]?.trim();

      shisho.log.info(`Parsed ${context.filePath}`);
      return {
        title: firstLine || "Untitled",
        authors: [{ name: "Example Author", role: "writer" }],
        language: "en",
      };
    },
  },
};

export default implementation;
```

## Build the Runtime Artifact

Bundle the source and copy the manifest. This esbuild command exposes the default export through a temporary bundle global, then assigns it to the required global `plugin`:

```bash
npx esbuild src/main.ts --bundle --format=iife \
  --global-name=ShishoPluginBundle \
  --footer:js="var plugin = ShishoPluginBundle.default;" \
  --outfile=dist/main.js
cp manifest.json dist/manifest.json
```

The IIFE is just this build's bundling pattern. Inspect `dist/main.js` before loading it. It must define a global `plugin` with the hook object, and it must not rely on `require`, Node built-ins, unbundled imports, or browser-only globals.

A simple repeatable workflow is:

1. Type-check source with `npx tsc --noEmit` using your project's `tsconfig.json`.
2. Run unit tests against `@shisho/plugin-sdk/testing`.
3. Bundle to `dist/main.js` and copy `manifest.json`.
4. Copy the built artifact into Shisho's local plugin path.
5. Scan, enable, and exercise it in a real library.

See [Testing Plugins](./testing.md) for the unit and integration test sequence.

## Test in Shisho

The local development location is:

```text
{pluginDir}/local/{id}/
  manifest.json
  main.js
```

`pluginDir` is controlled by `plugin_dir`; see [Configuration](../configuration.md). For the example, copy `dist/manifest.json` and `dist/main.js` to `{pluginDir}/local/example-parser/`.

1. Go to **Settings > Plugins > Installed**.
2. Select **Scan for Local Plugins**.
3. Open the discovered plugin and turn on **Enabled**.
4. Add or scan a file with the declared extension in a test library.
5. Check the resulting metadata, job logs, and application logs.

A newly scanned local plugin starts disabled. **Reload plugin from disk** appears only when the plugin has scope `local`, is active, and the current user has **Config: Write**. Rebuild and recopy your files, then use that action to load changes. If the plugin is disabled, enable it instead. Reload also rereads the manifest name, version, and description.

Use `shisho.log.debug`, `info`, `warn`, and `error` for diagnostic messages. Logs include the plugin scope and ID. Keep secrets out of messages.

Unit tests cannot prove that a bundle works in Shisho. Always perform at least one real-runtime test for hook loading, host API behavior, declared capabilities, filesystem access, generated output, and any bundled dependency.

## Expand the Plugin

A plugin may implement one or more hooks:

- `inputConverter.convert` converts an input into a supported format.
- `fileParser.parse` extracts metadata from a file.
- `metadataEnricher.search` returns candidate metadata.
- `outputGenerator.generate` and `fingerprint` create and cache an alternative format.

Every exported hook must have its matching manifest capability. Permission capabilities must cover every network domain, broad filesystem operation, FFmpeg call, or shell command the plugin uses.

Next steps:

- [Manifest and Hooks Reference](./manifest-hooks-reference.md)
- [Host API Reference](./host-api-reference.md)
- [Testing Plugins](./testing.md)
- [Publishing Plugins](./publishing.md)
