import type {
  FetchResponse,
  ParsedURL,
  ShishoConfig,
  ShishoFS,
  ShishoHostAPI,
  ShishoHTTP,
  ShishoLog,
  ShishoURL,
} from "../index";

/** Configuration for a mock fetch response. */
export interface MockFetchResponse {
  /** HTTP status code (default: 200). */
  status?: number;
  /** Response body as string (default: ""). */
  body?: string;
  /** Response headers (default: {}). */
  headers?: Record<string, string>;
}

/** Options for createMockShisho(). */
export interface MockShishoOptions {
  /** Route-based fetch mock. Keys are URL strings, values are mock responses. */
  fetch?: Record<string, MockFetchResponse>;
  /** Config key-value pairs returned by shisho.config. */
  config?: Record<string, string>;
  /**
   * Path-based filesystem mock.
   * - string values are returned by readTextFile/readFile
   * - Buffer values are returned by readFile (as ArrayBuffer)
   * - string[] values are returned by listDir
   */
  fs?: Record<string, string | Buffer | string[]>;
}

function statusText(status: number): string {
  const texts: Record<number, string> = {
    200: "OK",
    201: "Created",
    204: "No Content",
    301: "Moved Permanently",
    302: "Found",
    304: "Not Modified",
    400: "Bad Request",
    401: "Unauthorized",
    403: "Forbidden",
    404: "Not Found",
    500: "Internal Server Error",
    502: "Bad Gateway",
    503: "Service Unavailable",
  };
  return texts[status] || "Unknown";
}

function createMockFetchResponse(
  url: string,
  mock: MockFetchResponse,
): FetchResponse {
  const status = mock.status ?? 200;
  const body = mock.body ?? "";
  const headers = mock.headers ?? {};

  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: statusText(status),
    headers,
    text(): string {
      return body;
    },
    json(): unknown {
      try {
        return JSON.parse(body);
      } catch {
        throw new Error(
          `Failed to parse response body as JSON for ${url}: ${body.slice(0, 100)}`,
        );
      }
    },
    arrayBuffer(): ArrayBuffer {
      const encoder = new TextEncoder();
      return encoder.encode(body).buffer as ArrayBuffer;
    },
  };
}

/**
 * Create a mock `shisho` host API object for testing plugins.
 *
 * Provides mock implementations of log, config, http, url, and fs.
 * Archive, XML, HTML, FFmpeg, and Shell are not mocked (they throw if called).
 *
 * @example
 * ```ts
 * const shisho = createMockShisho({
 *   fetch: {
 *     "https://api.example.com/search?q=test": {
 *       status: 200,
 *       body: JSON.stringify({ results: [] }),
 *     },
 *   },
 *   config: { apiKey: "test-key" },
 * });
 * ```
 */
export function createMockShisho(
  options: MockShishoOptions = {},
): ShishoHostAPI {
  const fetchRoutes = options.fetch ?? {};
  const configMap = options.config ?? {};
  const fsMap = options.fs ?? {};

  // --- log: silent no-ops ---
  const log: ShishoLog = {
    debug() {},
    info() {},
    warn() {},
    error() {},
  };

  // --- config: map-based ---
  const config: ShishoConfig = {
    get(key: string): string | undefined {
      return configMap[key];
    },
    getAll(): Record<string, string> {
      return { ...configMap };
    },
  };

  // --- http: route-based mock ---
  const http: ShishoHTTP = {
    fetch(url: string): FetchResponse {
      const mock = fetchRoutes[url];
      if (!mock) {
        const definedRoutes = Object.keys(fetchRoutes);
        const routeList =
          definedRoutes.length > 0
            ? definedRoutes.map((r) => `  - ${r}`).join("\n")
            : "  (none)";
        throw new Error(
          `Mock fetch: no route defined for URL "${url}".\n\nDefined routes:\n${routeList}`,
        );
      }
      return createMockFetchResponse(url, mock);
    },
  };

  // --- url: real implementations ---
  const url: ShishoURL = {
    encodeURIComponent(str: string): string {
      return encodeURIComponent(str);
    },

    decodeURIComponent(str: string): string {
      return decodeURIComponent(str);
    },

    searchParams(params: Record<string, unknown>): string {
      const keys = Object.keys(params).sort();
      const parts: string[] = [];

      for (const key of keys) {
        const value = params[key];
        if (value === null || value === undefined) {
          continue;
        }
        if (Array.isArray(value)) {
          for (const item of value) {
            parts.push(
              `${encodeURIComponent(key)}=${encodeURIComponent(String(item))}`,
            );
          }
        } else {
          parts.push(
            `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`,
          );
        }
      }

      return parts.join("&");
    },

    parse(urlStr: string): ParsedURL {
      const parsed = new URL(urlStr);

      // Build query map: single values as string, repeated keys as array
      const query: Record<string, string | string[]> = {};
      parsed.searchParams.forEach((value, key) => {
        const existing = query[key];
        if (existing === undefined) {
          query[key] = value;
        } else if (Array.isArray(existing)) {
          existing.push(value);
        } else {
          query[key] = [existing, value];
        }
      });

      return {
        href: parsed.href,
        protocol: parsed.protocol.replace(/:$/, ""),
        host: parsed.host,
        hostname: parsed.hostname,
        port: parsed.port,
        pathname: parsed.pathname,
        search: parsed.search,
        hash: parsed.hash,
        username: parsed.username,
        password: parsed.password,
        query,
      };
    },
  };

  // --- fs: path-based mock ---
  const fs: ShishoFS = {
    readFile(path: string): ArrayBuffer {
      const entry = fsMap[path];
      if (entry === undefined) {
        const definedPaths = Object.keys(fsMap);
        const pathList =
          definedPaths.length > 0
            ? definedPaths.map((p) => `  - ${p}`).join("\n")
            : "  (none)";
        throw new Error(
          `Mock fs.readFile: no entry for path "${path}".\n\nDefined paths:\n${pathList}`,
        );
      }
      if (Array.isArray(entry)) {
        throw new Error(
          `Mock fs.readFile: path "${path}" is a directory (string[]), not a file.`,
        );
      }
      if (typeof entry === "string") {
        const encoder = new TextEncoder();
        return encoder.encode(entry).buffer as ArrayBuffer;
      }
      // Buffer
      return entry.buffer.slice(
        entry.byteOffset,
        entry.byteOffset + entry.byteLength,
      ) as ArrayBuffer;
    },

    readTextFile(path: string): string {
      const entry = fsMap[path];
      if (entry === undefined) {
        const definedPaths = Object.keys(fsMap);
        const pathList =
          definedPaths.length > 0
            ? definedPaths.map((p) => `  - ${p}`).join("\n")
            : "  (none)";
        throw new Error(
          `Mock fs.readTextFile: no entry for path "${path}".\n\nDefined paths:\n${pathList}`,
        );
      }
      if (Array.isArray(entry)) {
        throw new Error(
          `Mock fs.readTextFile: path "${path}" is a directory (string[]), not a file.`,
        );
      }
      if (typeof entry === "string") {
        return entry;
      }
      // Buffer -> string
      const decoder = new TextDecoder();
      return decoder.decode(entry);
    },

    writeFile(): void {
      // no-op
    },

    writeTextFile(): void {
      // no-op
    },

    exists(path: string): boolean {
      return path in fsMap;
    },

    mkdir(): void {
      // no-op
    },

    listDir(path: string): string[] {
      const entry = fsMap[path];
      if (entry === undefined) {
        const definedPaths = Object.keys(fsMap);
        const pathList =
          definedPaths.length > 0
            ? definedPaths.map((p) => `  - ${p}`).join("\n")
            : "  (none)";
        throw new Error(
          `Mock fs.listDir: no entry for path "${path}".\n\nDefined paths:\n${pathList}`,
        );
      }
      if (!Array.isArray(entry)) {
        throw new Error(
          `Mock fs.listDir: path "${path}" is a file, not a directory (string[]).`,
        );
      }
      return entry;
    },

    tempDir(): string {
      return "/tmp/shisho-mock-temp";
    },
  };

  // --- Stubs for APIs not covered by mock ---
  const notImplemented = (api: string) => () => {
    throw new Error(
      `Mock ${api}: not implemented. Use MockShishoOptions to configure the APIs you need, ` +
        `or provide your own mock for ${api}.`,
    );
  };

  return {
    dataDir: "/tmp/shisho-mock-data",
    log,
    config,
    http,
    url,
    fs,
    archive: {
      extractZip: notImplemented("archive.extractZip") as never,
      createZip: notImplemented("archive.createZip") as never,
      readZipEntry: notImplemented("archive.readZipEntry") as never,
      listZipEntries: notImplemented("archive.listZipEntries") as never,
    },
    xml: {
      parse: notImplemented("xml.parse") as never,
      querySelector: notImplemented("xml.querySelector") as never,
      querySelectorAll: notImplemented("xml.querySelectorAll") as never,
    },
    html: {
      querySelector: notImplemented("html.querySelector") as never,
      querySelectorAll: notImplemented("html.querySelectorAll") as never,
    },
    ffmpeg: {
      transcode: notImplemented("ffmpeg.transcode") as never,
      probe: notImplemented("ffmpeg.probe") as never,
      version: notImplemented("ffmpeg.version") as never,
    },
    shell: {
      exec: notImplemented("shell.exec") as never,
    },
  };
}
