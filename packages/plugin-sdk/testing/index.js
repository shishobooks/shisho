function statusText(status) {
    const texts = {
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
function createMockFetchResponse(url, mock) {
    const status = mock.status ?? 200;
    const body = mock.body ?? "";
    const headers = mock.headers ?? {};
    return {
        ok: status >= 200 && status < 300,
        status,
        statusText: statusText(status),
        headers,
        text() {
            return body;
        },
        json() {
            try {
                return JSON.parse(body);
            }
            catch {
                throw new Error(`Failed to parse response body as JSON for ${url}: ${body.slice(0, 100)}`);
            }
        },
        arrayBuffer() {
            const encoder = new TextEncoder();
            return encoder.encode(body).buffer;
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
export function createMockShisho(options = {}) {
    const fetchRoutes = options.fetch ?? {};
    const configMap = options.config ?? {};
    const fsMap = options.fs ?? {};
    // --- log: silent no-ops ---
    const log = {
        debug(_msg) { },
        info(_msg) { },
        warn(_msg) { },
        error(_msg) { },
    };
    // --- config: map-based ---
    const config = {
        get(key) {
            return configMap[key];
        },
        getAll() {
            return { ...configMap };
        },
    };
    // --- http: route-based mock ---
    const http = {
        fetch(url, _options) {
            const mock = fetchRoutes[url];
            if (!mock) {
                const definedRoutes = Object.keys(fetchRoutes);
                const routeList = definedRoutes.length > 0
                    ? definedRoutes.map((r) => `  - ${r}`).join("\n")
                    : "  (none)";
                throw new Error(`Mock fetch: no route defined for URL "${url}".\n\nDefined routes:\n${routeList}`);
            }
            return createMockFetchResponse(url, mock);
        },
    };
    // --- url: real implementations ---
    const url = {
        encodeURIComponent(str) {
            return encodeURIComponent(str);
        },
        decodeURIComponent(str) {
            return decodeURIComponent(str);
        },
        searchParams(params) {
            const keys = Object.keys(params).sort();
            const parts = [];
            for (const key of keys) {
                const value = params[key];
                if (value === null || value === undefined) {
                    continue;
                }
                if (Array.isArray(value)) {
                    for (const item of value) {
                        parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(String(item))}`);
                    }
                }
                else {
                    parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`);
                }
            }
            return parts.join("&");
        },
        parse(urlStr) {
            const parsed = new URL(urlStr);
            // Build query map: single values as string, repeated keys as array
            const query = {};
            parsed.searchParams.forEach((value, key) => {
                const existing = query[key];
                if (existing === undefined) {
                    query[key] = value;
                }
                else if (Array.isArray(existing)) {
                    existing.push(value);
                }
                else {
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
    const fs = {
        readFile(path) {
            const entry = fsMap[path];
            if (entry === undefined) {
                const definedPaths = Object.keys(fsMap);
                const pathList = definedPaths.length > 0
                    ? definedPaths.map((p) => `  - ${p}`).join("\n")
                    : "  (none)";
                throw new Error(`Mock fs.readFile: no entry for path "${path}".\n\nDefined paths:\n${pathList}`);
            }
            if (Array.isArray(entry)) {
                throw new Error(`Mock fs.readFile: path "${path}" is a directory (string[]), not a file.`);
            }
            if (typeof entry === "string") {
                const encoder = new TextEncoder();
                return encoder.encode(entry).buffer;
            }
            // Buffer
            return entry.buffer.slice(entry.byteOffset, entry.byteOffset + entry.byteLength);
        },
        readTextFile(path) {
            const entry = fsMap[path];
            if (entry === undefined) {
                const definedPaths = Object.keys(fsMap);
                const pathList = definedPaths.length > 0
                    ? definedPaths.map((p) => `  - ${p}`).join("\n")
                    : "  (none)";
                throw new Error(`Mock fs.readTextFile: no entry for path "${path}".\n\nDefined paths:\n${pathList}`);
            }
            if (Array.isArray(entry)) {
                throw new Error(`Mock fs.readTextFile: path "${path}" is a directory (string[]), not a file.`);
            }
            if (typeof entry === "string") {
                return entry;
            }
            // Buffer -> string
            const decoder = new TextDecoder();
            return decoder.decode(entry);
        },
        writeFile(_path, _data) {
            // no-op
        },
        writeTextFile(_path, _content) {
            // no-op
        },
        exists(path) {
            return path in fsMap;
        },
        mkdir(_path) {
            // no-op
        },
        listDir(path) {
            const entry = fsMap[path];
            if (entry === undefined) {
                const definedPaths = Object.keys(fsMap);
                const pathList = definedPaths.length > 0
                    ? definedPaths.map((p) => `  - ${p}`).join("\n")
                    : "  (none)";
                throw new Error(`Mock fs.listDir: no entry for path "${path}".\n\nDefined paths:\n${pathList}`);
            }
            if (!Array.isArray(entry)) {
                throw new Error(`Mock fs.listDir: path "${path}" is a file, not a directory (string[]).`);
            }
            return entry;
        },
        tempDir() {
            return "/tmp/shisho-mock-temp";
        },
    };
    // --- Stubs for APIs not covered by mock ---
    const notImplemented = (api) => () => {
        throw new Error(`Mock ${api}: not implemented. Use MockShishoOptions to configure the APIs you need, ` +
            `or provide your own mock for ${api}.`);
    };
    return {
        dataDir: "/tmp/shisho-mock-data",
        log,
        config,
        http,
        url,
        fs,
        archive: {
            extractZip: notImplemented("archive.extractZip"),
            createZip: notImplemented("archive.createZip"),
            readZipEntry: notImplemented("archive.readZipEntry"),
            listZipEntries: notImplemented("archive.listZipEntries"),
        },
        xml: {
            parse: notImplemented("xml.parse"),
            querySelector: notImplemented("xml.querySelector"),
            querySelectorAll: notImplemented("xml.querySelectorAll"),
        },
        html: {
            querySelector: notImplemented("html.querySelector"),
            querySelectorAll: notImplemented("html.querySelectorAll"),
        },
        ffmpeg: {
            transcode: notImplemented("ffmpeg.transcode"),
            probe: notImplemented("ffmpeg.probe"),
            version: notImplemented("ffmpeg.version"),
        },
        shell: {
            exec: notImplemented("shell.exec"),
        },
    };
}
