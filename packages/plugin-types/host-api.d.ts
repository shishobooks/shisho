/** Logging methods. */
export interface ShishoLog {
  debug(msg: string): void;
  info(msg: string): void;
  warn(msg: string): void;
  error(msg: string): void;
}

/** Plugin configuration access. */
export interface ShishoConfig {
  /** Get a single config value by key. Returns undefined if not set. */
  get(key: string): string | undefined;
  /** Get all config values as a key-value map. */
  getAll(): Record<string, string>;
}

/** Options for shisho.http.fetch(). */
export interface FetchOptions {
  /** HTTP method (default: "GET"). */
  method?: string;
  /** Request headers. */
  headers?: Record<string, string>;
  /** Request body string. */
  body?: string;
}

/** Response from shisho.http.fetch(). */
export interface FetchResponse {
  /** Whether the status code is 2xx. */
  ok: boolean;
  /** HTTP status code. */
  status: number;
  /** HTTP status text. */
  statusText: string;
  /** Response headers (lowercase keys). */
  headers: Record<string, string>;
  /** Get response body as string. */
  text(): string;
  /** Get response body as ArrayBuffer. */
  arrayBuffer(): ArrayBuffer;
  /** Parse response body as JSON. */
  json(): unknown;
}

/** HTTP client with domain whitelisting. */
export interface ShishoHTTP {
  /** Fetch a URL. Domain must be declared in manifest httpAccess.domains. */
  fetch(url: string, options?: FetchOptions): FetchResponse;
}

/** Filesystem operations (sandboxed). */
export interface ShishoFS {
  /** Read file contents as ArrayBuffer. */
  readFile(path: string): ArrayBuffer;
  /** Read file contents as UTF-8 string. */
  readTextFile(path: string): string;
  /** Write ArrayBuffer data to a file. */
  writeFile(path: string, data: ArrayBuffer): void;
  /** Write string content to a file. */
  writeTextFile(path: string, content: string): void;
  /** Check if a path exists. */
  exists(path: string): boolean;
  /** Create a directory (and parents). */
  mkdir(path: string): void;
  /** List directory entries (file/dir names). */
  listDir(path: string): string[];
  /** Get a temporary directory path (auto-cleaned after hook returns). */
  tempDir(): string;
}

/** ZIP archive operations. */
export interface ShishoArchive {
  /** Extract all entries from a ZIP archive to a directory. */
  extractZip(archivePath: string, destDir: string): void;
  /** Create a ZIP archive from a directory's contents. */
  createZip(srcDir: string, destPath: string): void;
  /** Read a specific entry from a ZIP archive as ArrayBuffer. */
  readZipEntry(archivePath: string, entryPath: string): ArrayBuffer;
  /** List all entry paths in a ZIP archive. */
  listZipEntries(archivePath: string): string[];
}

/** A parsed XML element. */
export interface XMLElement {
  /** Element tag name (local part). */
  tag: string;
  /** Element namespace URI. */
  namespace: string;
  /** Direct text content. */
  text: string;
  /** Element attributes. */
  attributes: Record<string, string>;
  /** Child elements. */
  children: XMLElement[];
}

/** XML parsing and querying. */
export interface ShishoXML {
  /** Parse an XML string into an element tree. */
  parse(content: string): XMLElement;
  /** Find the first element matching a selector. Supports "prefix|tag" namespace syntax. */
  querySelector(
    doc: XMLElement,
    selector: string,
    namespaces?: Record<string, string>,
  ): XMLElement | null;
  /** Find all elements matching a selector. Supports "prefix|tag" namespace syntax. */
  querySelectorAll(
    doc: XMLElement,
    selector: string,
    namespaces?: Record<string, string>,
  ): XMLElement[];
}

/** Result from shisho.ffmpeg.run(). */
export interface FFmpegResult {
  /** Process exit code (0 = success). */
  exitCode: number;
  /** Standard output. */
  stdout: string;
  /** Standard error. */
  stderr: string;
}

/** FFmpeg subprocess execution. */
export interface ShishoFFmpeg {
  /** Run FFmpeg with the given arguments. Requires ffmpegAccess capability. */
  run(args: string[]): FFmpegResult;
}

/** Top-level host API object available as the global `shisho` variable. */
export interface ShishoHostAPI {
  log: ShishoLog;
  config: ShishoConfig;
  http: ShishoHTTP;
  fs: ShishoFS;
  archive: ShishoArchive;
  xml: ShishoXML;
  ffmpeg: ShishoFFmpeg;
}
