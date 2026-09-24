# Testing Plugins

Test a plugin in two stages: fast unit tests with the SDK mock, then an integration test in an actual Shisho server. The mock checks plugin logic and fixture handling, but it does not reproduce the Shisho JavaScript runtime, filesystem sandbox, capability enforcement, or external tools.

## Install the Testing Utilities

The `@shisho/plugin-sdk/testing` entry point includes `createMockShisho`:

```bash
npm install --save-dev @shisho/plugin-sdk vitest
```

The testing implementation is published with the SDK and typed as `ShishoHostAPI`.

## Create a Mock Host

```typescript
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createMockShisho } from "@shisho/plugin-sdk/testing";

const sleep = vi.fn();

beforeEach(() => {
  globalThis.shisho = createMockShisho({
    config: {
      apiKey: "test-key",
      maxResults: "5",
    },
    fetch: {
      "https://api.example.com/search?q=Dune": {
        status: 200,
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          results: [{ title: "Dune", author: "Frank Herbert" }],
        }),
      },
    },
    fs: {
      "/books/example.txt": "Example Book\nA test fixture.",
      "/books": ["example.txt"],
      "/books/cover.bin": new Uint8Array([1, 2, 3]),
    },
    sleep,
  });
});
```

The three main fixture maps are:

- `config`: string values returned by `config.get` and `getAll`
- `fetch`: exact URL keys mapped to status, headers, and body
- `fs`: exact paths mapped to text, bytes, or a `string[]` directory listing

An unmatched HTTP URL or missing filesystem path throws a descriptive error that lists configured routes or paths.

## Implemented Mock APIs

| API | Mock Behavior |
|-----|---------------|
| `dataDir` | Fixed string `/tmp/shisho-mock-data`; the mock does not create it. |
| `log.debug/info/warn/error` | Silent no-ops. Replace methods with spies if assertions are needed. |
| `config.get/getAll` | Reads the provided string map; `getAll` returns a copy. |
| `http.fetch` | Exact URL lookup in the provided map. Response readers support text, JSON, and `ArrayBuffer`. Request method, headers, and body do not affect route matching. |
| `sleep` | Validates a finite, non-negative number, then calls the optional override. Without an override it is a no-op. |
| `url` | Working encode, decode, sorted query builder, and URL parser implementations. |
| `fs.readFile/readTextFile/exists/listDir` | Reads the provided path map with file and directory validation. |
| `fs.tempDir` | Fixed string `/tmp/shisho-mock-temp`. |
| `xml` | Working in-memory parse and namespace-aware tag query implementation. |
| `html` | Working parse and CSS selector query implementation. |
| `yaml` | Working parse and stringify implementation. |

URL, XML, HTML, and YAML mocks use JavaScript libraries and aim to match Shisho's public shapes. Parser edge cases can still differ, so confirm subtle behavior in Shisho.

### Filesystem Write Caveat

The following mock methods are no-ops:

```text
fs.writeFile
fs.writeTextFile
fs.mkdir
```

They do not update the `fs` fixture map. A test that writes and then reads the same path can therefore pass only if the path was already configured, or fail because the write was not recorded. Assert calls with spies or provide a custom filesystem mock when write effects are part of the behavior under test.

## Stubbed APIs

The SDK mock deliberately throws a not-implemented error for:

- every `archive` method
- `ffmpeg.transcode`, `ffmpeg.probe`, and `ffmpeg.version`
- `shell.exec`

Provide a purpose-built replacement in the test when your plugin uses one of these APIs. Keep its return shape aligned with the [Host API Reference](./host-api-reference.md). Regardless of the unit mock, exercise that path in Shisho with the required manifest capability and actual external tool.

## Test Hook Logic

Keep the core hook object importable from source, then assign it to the runtime global in the entry file. This makes methods easy to call in unit tests:

```typescript
import type {
  SearchContext,
  SearchResponse,
  ShishoPlugin,
} from "@shisho/plugin-sdk";

export const implementation: ShishoPlugin = {
  metadataEnricher: {
    search(context: SearchContext): SearchResponse {
      const url =
        "https://api.example.com/search?" +
        shisho.url.searchParams({ q: context.query });
      const response = shisho.http.fetch(url);
      const data = response.json() as {
        results: Array<{ title: string; author: string }>;
      };
      return {
        results: data.results.map((item) => ({
          title: item.title,
          authors: [{ name: item.author, role: "writer" }],
          confidence: 1,
        })),
      };
    },
  },
};
```

```typescript
import { describe, expect, it } from "vitest";
import { implementation } from "./implementation";

describe("metadata search", () => {
  it("maps an API result", () => {
    const result = implementation.metadataEnricher!.search({ query: "Dune" });

    expect(result.results).toEqual([
      {
        title: "Dune",
        authors: [{ name: "Frank Herbert", role: "writer" }],
        confidence: 1,
      },
    ]);
  });
});
```

Useful unit cases include:

- successful and empty responses
- non-2xx HTTP responses and malformed JSON
- retry and `sleep` calls
- missing optional configuration
- XML, HTML, or YAML fixture variations
- malformed or incomplete source metadata
- correct `ParsedMetadata` and confidence values
- output fingerprint changes when relevant inputs change

Also type-check the project and build the final bundle during CI:

```bash
npx tsc --noEmit
npm test
npm run build
```

## Test the Built Artifact in Shisho

Node-based tests can use language features or globals that the Shisho runtime does not provide. A successful unit suite does not prove that `main.js` defines the global `plugin`, that bundled dependencies work, or that capabilities and paths are correct.

Copy the built artifact to:

```text
{pluginDir}/local/{id}/
  manifest.json
  main.js
```

Then:

1. Go to **Settings > Plugins > Installed**.
2. Select **Scan for Local Plugins**.
3. Open the discovered plugin and turn on **Enabled**.
4. Confirm its capabilities, configuration, hook order, and status.
5. Exercise every implemented hook with a small test library.
6. Verify output metadata or files and review job and application logs.

The local scan adds only previously unknown directories under the `local` scope. It does not reload an existing plugin.

After changing an already discovered plugin, rebuild and recopy it. Use **Reload plugin from disk** on the plugin detail page. That action appears only for an active plugin whose scope is `local`, and it requires **Config: Write**. If loading fails, the detail page displays the current error; correct the artifact and reload again.

## Integration Checklist

Before publishing, verify:

- `manifest.json` and `main.js` are at the artifact root.
- Executing `main.js` creates the global `plugin` object.
- Every JavaScript hook has its matching manifest capability.
- No code relies on Node.js, browser-only APIs, Promises, or timers.
- Every HTTP domain and shell command is declared.
- Filesystem reads and writes stay inside expected paths.
- FFmpeg and shell failure results are handled.
- Enricher fields, ordering, and confidence behavior are correct.
- Cover and series grouped metadata behaves correctly for each supported format.
- `onUninstalling`, if present, is safe to run as best-effort cleanup.

See [Developing Plugins](./development.md) for the local workflow and [Publishing Plugins](./publishing.md) for release packaging.
