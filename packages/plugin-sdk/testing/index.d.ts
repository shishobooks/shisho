import type { ShishoHostAPI } from "../index";
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
export declare function createMockShisho(options?: MockShishoOptions): ShishoHostAPI;
