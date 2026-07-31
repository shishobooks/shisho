# Host API Reference

Plugin code receives a global `shisho: ShishoHostAPI`. Calls are synchronous. There is no Node.js runtime, native `fetch`, Promise-based host API, or timer API. Use `shisho.http.fetch` for HTTP and `shisho.sleep` for blocking retry delays.

The complete TypeScript contract is in [`host-api.d.ts`](https://github.com/shishobooks/shisho/blob/master/packages/plugin-sdk/host-api.d.ts). Install it with `npm install --save-dev @shisho/plugin-sdk`.

## Capability Summary

| API | Manifest Capability |
|-----|---------------------|
| `dataDir`, `log`, `config`, `sleep`, `url`, `xml`, `html`, `yaml` | None |
| `http` | `httpAccess` with an allowed domain |
| `fs`, `archive` | Sandboxed paths supplied by the hook; broad access needs `fileAccess` |
| `ffmpeg` | `ffmpegAccess` |
| `shell` | `shellAccess` with the command in `commands` |

The installation dialog shows capabilities supplied by the repository publisher. Shisho does not currently verify that this preview matches the downloaded plugin manifest, so administrators must still trust the publisher and review the plugin source. Plugin authors should declare only what the plugin needs in both places.

## Persistent Data Directory

```typescript
readonly shisho.dataDir: string;
```

`dataDir` is a persistent directory dedicated to the plugin and created when first accessed. It is always readable and writable through `shisho.fs`. Use it for plugin-managed caches or state that should survive restarts and updates. Do not use it as a substitute for Shisho configuration.

## Logging

```typescript
shisho.log.debug(msg: string): void;
shisho.log.info(msg: string): void;
shisho.log.warn(msg: string): void;
shisho.log.error(msg: string): void;
```

Messages are tagged with the plugin scope and ID. Keep credentials, tokens, book content, and other sensitive values out of logs.

## Configuration

```typescript
shisho.config.get(key: string): string | undefined;
shisho.config.getAll(): Record<string, string>;
```

Values come from the plugin's administrator-managed `configSchema`. Even boolean and numeric form values are exposed as strings, so parse them explicitly:

```javascript
var maxResults = Number(shisho.config.get("maxResults") || "10");
var includeImages = shisho.config.get("includeImages") === "true";
```

A missing value returns `undefined`. Secret fields are still available to plugin code; masking applies only when values are displayed again.

## Sleep

```typescript
shisho.sleep(ms: number): void;
```

Blocks synchronously for a finite, non-negative number of milliseconds. `0` returns immediately. Missing, negative, `NaN`, or infinite values throw. Use it for retry backoff:

```javascript
for (var attempt = 0; attempt < 3; attempt++) {
  var response = shisho.http.fetch(url);
  if (response.status !== 429) return response;
  shisho.sleep(1000 * Math.pow(2, attempt));
}
```

A hook cancellation interrupts the wait.

## HTTP

Requires `httpAccess`:

```json
{
  "capabilities": {
    "httpAccess": {
      "domains": ["api.example.com", "*.covers.example.com"]
    }
  }
}
```

Exact domains match only themselves. `*.example.com` matches the base domain and any depth of subdomain. Matching is case-insensitive. Standard HTTP and HTTPS ports are accepted; a non-standard port must be included in the manifest entry. Redirect destinations are checked against the same list.

### Fetch

```typescript
interface FetchOptions {
  method?: string; // default GET
  headers?: Record<string, string>;
  body?: string;
}

shisho.http.fetch(url: string, options?: FetchOptions): FetchResponse;
```

Only `http` and `https` URLs are supported. The full response body is available through any response reader:

```typescript
interface FetchResponse {
  ok: boolean; // true for 2xx
  status: number;
  statusText: string;
  headers: Record<string, string>; // lowercase keys
  text(): string;
  arrayBuffer(): ArrayBuffer;
  json(): unknown;
}
```

`json()` throws for invalid JSON. HTTP error statuses return a response normally, so check `ok` or `status` yourself.

```javascript
var response = shisho.http.fetch("https://api.example.com/books", {
  method: "POST",
  headers: {
    "Authorization": "Bearer " + shisho.config.get("apiKey"),
    "Content-Type": "application/json",
  },
  body: JSON.stringify({ query: "Dune" }),
});
if (!response.ok) {
  throw new Error("Lookup failed with HTTP " + response.status);
}
var body = response.json();
```

A metadata result's `coverUrl` is also checked against the plugin's allowed HTTP domains when Shisho downloads it.

## URL Utilities

### Encode and Decode

```typescript
shisho.url.encodeURIComponent(str: string): string;
shisho.url.decodeURIComponent(str: string): string;
```

These helpers use query-style escaping, so spaces encode as `+` and `+` decodes as a space.

### Build Query Parameters

```typescript
shisho.url.searchParams(params: Record<string, unknown>): string;
```

Keys are sorted for deterministic output. Array values produce repeated keys. `null` and `undefined` values are skipped.

```javascript
shisho.url.searchParams({ q: "space opera", page: 2 });
// "page=2&q=space+opera"

shisho.url.searchParams({ tag: ["fiction", "audio"] });
// "tag=fiction&tag=audio"
```

### Parse a URL

```typescript
interface ParsedURL {
  href: string;
  protocol: string;
  host: string;
  hostname: string;
  port: string;
  pathname: string;
  search: string;
  hash: string;
  username: string;
  password: string;
  query: Record<string, string | string[]>;
}

shisho.url.parse(url: string): ParsedURL;
```

`protocol` excludes the colon. `search` and `hash` include their leading punctuation when present. Repeated query keys become arrays.

## Filesystem

```typescript
shisho.fs.readFile(path: string): ArrayBuffer;
shisho.fs.readTextFile(path: string): string;
shisho.fs.writeFile(path: string, data: ArrayBuffer): void;
shisho.fs.writeTextFile(path: string, content: string): void;
shisho.fs.exists(path: string): boolean;
shisho.fs.mkdir(path: string): void;
shisho.fs.listDir(path: string): string[];
shisho.fs.tempDir(): string;
```

`readTextFile` and `writeTextFile` use UTF-8 text. `mkdir` creates parents. `listDir` returns entry names, not full paths. `tempDir` is created on first use and cleaned up after the hook returns.

### Path Access

| Path | Read | Write |
|------|------|-------|
| Plugin artifact directory | Yes | Yes |
| `shisho.dataDir` | Yes | Yes |
| `shisho.fs.tempDir()` | Yes | Yes |
| Input converter `sourcePath` and `targetDir` | Yes | Yes |
| File parser `filePath` | Yes | Yes |
| Output generator `sourcePath` and `destPath` | Yes | Yes |
| Metadata enricher target `filePath` | Yes | No |
| Other paths | With `fileAccess: read` or `readwrite` | With `fileAccess: readwrite` |

An enricher's scoped target access covers exactly that file, not siblings. Declare broad `read` access to inspect sidecars or neighboring assets. Access violations and failed file operations throw.

## ZIP Archives

Archive methods use the same filesystem path checks as `shisho.fs`:

```typescript
shisho.archive.extractZip(archivePath: string, destDir: string): void;
shisho.archive.createZip(srcDir: string, destPath: string): void;
shisho.archive.readZipEntry(
  archivePath: string,
  entryPath: string,
): ArrayBuffer;
shisho.archive.listZipEntries(archivePath: string): string[];
```

`extractZip` needs read access to the archive and write access to the destination. `createZip` needs read access to the source and write access to the destination. Entry paths use the names stored in the ZIP.

## XML

```typescript
interface XMLElement {
  tag: string;
  namespace: string;
  text: string;
  attributes: Record<string, string>;
  children: XMLElement[];
}

shisho.xml.parse(content: string): XMLElement;
shisho.xml.querySelector(
  doc: XMLElement,
  selector: string,
  namespaces?: Record<string, string>,
): XMLElement | null;
shisho.xml.querySelectorAll(
  doc: XMLElement,
  selector: string,
  namespaces?: Record<string, string>,
): XMLElement[];
```

XML selectors match a local tag name such as `title`, or a namespace-qualified form such as `dc|title` with a prefix map. Queries search the supplied element and its descendants.

```javascript
var root = shisho.xml.parse(xmlText);
var title = shisho.xml.querySelector(root, "dc|title", {
  dc: "http://purl.org/dc/elements/1.1/",
});
var value = title ? title.text.trim() : "";
```

`text` is direct character data for that element. Walk `children` when descendant text also matters.

## HTML

```typescript
interface HtmlElement {
  tag: string;
  attributes: Record<string, string>;
  text: string;
  innerHTML: string;
  children: HtmlElement[];
}

shisho.html.parse(html: string): HtmlElement;
shisho.html.querySelector(
  doc: HtmlElement,
  selector: string,
): HtmlElement | null;
shisho.html.querySelectorAll(
  doc: HtmlElement,
  selector: string,
): HtmlElement[];
```

HTML queries support CSS selectors and can start from the parsed document or a previous query result. `text` includes descendant text; `innerHTML` preserves the element's inner markup.

```javascript
var document = shisho.html.parse(response.text());
var node = shisho.html.querySelector(
  document,
  'script[type="application/ld+json"]',
);
var jsonLd = node ? JSON.parse(node.text) : null;
```

Use this API instead of regular expressions for HTML scraping.

## YAML

```typescript
shisho.yaml.parse(content: string): unknown;
shisho.yaml.stringify(value: unknown): string;
```

No capability is required. Parsing returns plain JavaScript objects, arrays, primitives, or `null`; invalid YAML throws. Mapping keys are exposed as strings. Stringification returns YAML text.

```javascript
var value = shisho.yaml.parse("title: My Book\npages: 100\n");
var output = shisho.yaml.stringify({ title: value.title, valid: true });
```

## FFmpeg

All methods require `ffmpegAccess`.

### Transcode

```typescript
interface TranscodeResult {
  exitCode: number;
  stdout: string;
  stderr: string;
}

shisho.ffmpeg.transcode(args: string[]): TranscodeResult;
```

Arguments are passed directly to FFmpeg. Network protocols are not enabled. A process that runs but exits unsuccessfully returns a nonzero `exitCode`; a failure to start throws.

```javascript
var result = shisho.ffmpeg.transcode([
  "-i", inputPath,
  "-c:a", "aac",
  outputPath,
]);
if (result.exitCode !== 0) throw new Error(result.stderr);
```

### Probe

```typescript
interface ProbeResult {
  format: ProbeFormat;
  streams: ProbeStream[];
  chapters: ProbeChapter[];
  stderr: string;
  parseError: string;
}

shisho.ffmpeg.probe(args: string[]): ProbeResult;
```

The host adds JSON output and format, stream, and chapter requests. Common `format` values include filename, duration, size, bitrate, and tags. Stream entries include codec type plus optional video, audio, timing, disposition, and tag properties. Chapter entries include ID, time base, start, end, and optional tags. Check `parseError` before relying on parsed fields. See the SDK declaration for the full property list.

### Version

```typescript
interface VersionResult {
  version: string;
  configuration: string[];
  libraries: Record<string, string>;
}

shisho.ffmpeg.version(): VersionResult;
```

Use this to check whether a required codec or build option is available before starting work.

## Shell Commands

```typescript
interface ExecResult {
  exitCode: number;
  stdout: string;
  stderr: string;
}

shisho.shell.exec(command: string, args: string[]): ExecResult;
```

Requires the exact command in `shellAccess.commands`:

```json
{
  "capabilities": {
    "shellAccess": {
      "commands": ["example-convert"]
    }
  }
}
```

The command receives the argument array directly. No command shell interprets pipes, redirects, substitutions, or quoting. A nonzero command exit returns an `ExecResult`; a disallowed command or failure to start throws.

```javascript
var result = shisho.shell.exec("example-convert", [inputPath, outputPath]);
if (result.exitCode !== 0) throw new Error(result.stderr);
```

Prefer a narrower host API such as `ffmpeg`, `archive`, or an in-memory parser when it can perform the same task.
