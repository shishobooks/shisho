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

/**
 * HTTP client with domain whitelisting.
 *
 * Domain patterns in manifest httpAccess.domains:
 * - Exact match: "example.com" only allows "example.com"
 * - Wildcard: "*.example.com" allows "example.com", "api.example.com", "a.b.example.com"
 */
export interface ShishoHTTP {
  /**
   * Fetch a URL. Domain must be declared in manifest httpAccess.domains.
   * Supports wildcard patterns like "*.example.com" for subdomains.
   */
  fetch(url: string, options?: FetchOptions): FetchResponse;
}

/** Parsed URL components from shisho.url.parse(). */
export interface ParsedURL {
  /** The original URL string. */
  href: string;
  /** URL scheme without ":" (e.g., "https"). */
  protocol: string;
  /** Host including port if present (e.g., "example.com:8080"). */
  host: string;
  /** Hostname without port (e.g., "example.com"). */
  hostname: string;
  /** Port number as string, or empty if not specified. */
  port: string;
  /** Path component (e.g., "/path/to/resource"). */
  pathname: string;
  /** Query string with leading "?" or empty string. */
  search: string;
  /** Fragment with leading "#" or empty string. */
  hash: string;
  /** Username from URL, or empty string. */
  username: string;
  /** Password from URL, or empty string. */
  password: string;
  /** Parsed query parameters. Single values are strings, repeated keys are arrays. */
  query: Record<string, string | string[]>;
}

/**
 * URL utilities that aren't available in Goja's ES5.1 runtime.
 * Provides functionality similar to browser URLSearchParams and URL APIs.
 */
export interface ShishoURL {
  /**
   * Encode a string for use in URL query parameters.
   * Similar to JavaScript's encodeURIComponent().
   */
  encodeURIComponent(str: string): string;

  /**
   * Decode a URL-encoded string.
   * Similar to JavaScript's decodeURIComponent().
   */
  decodeURIComponent(str: string): string;

  /**
   * Convert an object to a URL query string.
   * Keys are sorted alphabetically for deterministic output.
   * Array values create multiple key=value pairs.
   * Null/undefined values are skipped.
   *
   * @example
   * shisho.url.searchParams({ q: "test", page: 1 }) // "page=1&q=test"
   * shisho.url.searchParams({ tags: ["a", "b"] })   // "tags=a&tags=b"
   */
  searchParams(params: Record<string, unknown>): string;

  /**
   * Parse a URL string into its components.
   *
   * @example
   * const url = shisho.url.parse("https://api.example.com/search?q=test");
   * url.hostname // "api.example.com"
   * url.pathname // "/search"
   * url.query.q  // "test"
   */
  parse(url: string): ParsedURL;
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

/** Result from shisho.ffmpeg.transcode(). */
export interface TranscodeResult {
  /** Process exit code (0 = success). */
  exitCode: number;
  /** Standard output. */
  stdout: string;
  /** Standard error. */
  stderr: string;
}

/** Result from shisho.ffmpeg.probe(). */
export interface ProbeResult {
  format: ProbeFormat;
  streams: ProbeStream[];
  chapters: ProbeChapter[];
  /** Standard error output (for debugging). */
  stderr: string;
  /** JSON parse error message if ffprobe output could not be parsed. Empty string if parsing succeeded. */
  parseError: string;
}

/** Format information from ffprobe. */
export interface ProbeFormat {
  filename: string;
  nb_streams: number;
  nb_programs: number;
  format_name: string;
  format_long_name: string;
  start_time: string;
  duration: string;
  size: string;
  bit_rate: string;
  probe_score: number;
  tags?: Record<string, string>;
}

/** Stream information from ffprobe. */
export interface ProbeStream {
  index: number;
  codec_name?: string;
  codec_long_name?: string;
  codec_type: "video" | "audio" | "subtitle" | "data" | "attachment";
  codec_tag_string?: string;
  codec_tag?: string;

  // Video-specific
  width?: number;
  height?: number;
  coded_width?: number;
  coded_height?: number;
  closed_captions?: number;
  has_b_frames?: number;
  sample_aspect_ratio?: string;
  display_aspect_ratio?: string;
  pix_fmt?: string;
  level?: number;
  color_range?: string;
  color_space?: string;
  color_transfer?: string;
  color_primaries?: string;
  chroma_location?: string;
  field_order?: string;
  refs?: number;

  // Audio-specific
  sample_fmt?: string;
  sample_rate?: string;
  channels?: number;
  channel_layout?: string;
  bits_per_sample?: number;

  // Common (optional since not all stream types have these)
  r_frame_rate?: string;
  avg_frame_rate?: string;
  time_base?: string;
  start_pts?: number;
  start_time?: string;
  duration_ts?: number;
  duration?: string;
  bit_rate?: string;
  bits_per_raw_sample?: string;
  nb_frames?: string;
  disposition?: ProbeDisposition;
  tags?: Record<string, string>;
}

/** Stream disposition flags from ffprobe. */
export interface ProbeDisposition {
  default: number;
  dub: number;
  original: number;
  comment: number;
  lyrics: number;
  karaoke: number;
  forced: number;
  hearing_impaired: number;
  visual_impaired: number;
  clean_effects: number;
  attached_pic: number;
  timed_thumbnails: number;
}

/** Chapter information from ffprobe. */
export interface ProbeChapter {
  id: number;
  time_base: string;
  start: number;
  start_time: string;
  end: number;
  end_time: string;
  tags?: Record<string, string>;
}

/** Result from shisho.ffmpeg.version(). */
export interface VersionResult {
  /** FFmpeg version string (e.g., "7.0"). */
  version: string;
  /** Build configuration flags (e.g., ["--enable-libx264", "--enable-gpl"]). */
  configuration: string[];
  /** Library versions (e.g., { libavcodec: "60.31.102", ... }). */
  libraries: Record<string, string>;
}

/** FFmpeg subprocess execution. */
export interface ShishoFFmpeg {
  /** Transcode files with FFmpeg. Requires ffmpegAccess capability. */
  transcode(args: string[]): TranscodeResult;
  /** Probe file metadata with ffprobe. Returns parsed JSON. Requires ffmpegAccess capability. */
  probe(args: string[]): ProbeResult;
  /** Get FFmpeg version and configuration. Requires ffmpegAccess capability. */
  version(): VersionResult;
}

/** Result from shisho.shell.exec(). */
export interface ExecResult {
  /** Process exit code (0 = success). */
  exitCode: number;
  /** Standard output. */
  stdout: string;
  /** Standard error. */
  stderr: string;
}

/** Shell command execution (with allowlist). */
export interface ShishoShell {
  /**
   * Execute an allowed command with arguments.
   * Command must be declared in manifest shellAccess.commands.
   * Uses exec directly (no shell) to prevent injection.
   */
  exec(command: string, args: string[]): ExecResult;
}

/** Top-level host API object available as the global `shisho` variable. */
export interface ShishoHostAPI {
  log: ShishoLog;
  config: ShishoConfig;
  http: ShishoHTTP;
  url: ShishoURL;
  fs: ShishoFS;
  archive: ShishoArchive;
  xml: ShishoXML;
  ffmpeg: ShishoFFmpeg;
  shell: ShishoShell;
}
