import { ParsedMetadata } from "./metadata";

/** Context passed to inputConverter.convert(). */
export interface InputConverterContext {
  /** Path to the input file. */
  sourcePath: string;
  /** Directory where the converted output should be written. */
  targetDir: string;
}

/** Result returned from inputConverter.convert(). */
export interface ConvertResult {
  /** Whether conversion succeeded. */
  success: boolean;
  /** Path to the converted output file. */
  targetPath: string;
}

/** Context passed to fileParser.parse(). */
export interface FileParserContext {
  /** Path to the file being parsed. */
  filePath: string;
  /** File extension (e.g., "pdf", "epub"). */
  fileType: string;
}

/** Context passed to metadataEnricher.search(). */
export interface SearchContext {
  /** Search query — title or free-text. Always present. */
  query: string;
  /** Author name to narrow results. Optional. */
  author?: string;
  /** Structured identifiers for direct lookup (ISBN, ASIN, etc.). Optional. */
  identifiers?: Array<{ type: string; value: string }>;
  /** Read-only file metadata for matching hints. Not user-editable. */
  file?: {
    /**
     * File type (e.g., "epub", "cbz", "m4b", "pdf"): the extension, except
     * "kepub" for a .kepub.epub and "mobi" for .azw and .prc files.
     */
    fileType?: string;
    /**
     * Absolute path to the enrichment target file. The runtime grants the
     * enricher scoped **read-only** access to exactly this one file, so
     * `shisho.fs.readFile(context.file.filePath)` works without the plugin
     * declaring the broader `fileAccess` capability. Writes to this path
     * are NOT granted — a plugin that needs to modify the file must declare
     * `fileAccess: { level: "readwrite" }` in its manifest. Sibling files
     * (sidecars, covers) are also not included; the scope is the target
     * file only. Omitted when there is no target file (e.g. a pre-scan
     * enrichment with no file yet).
     */
    filePath?: string;
    /** Audiobook duration in seconds. */
    duration?: number;
    /** Number of pages (CBZ/PDF). */
    pageCount?: number;
    /** File size in bytes. */
    filesizeBytes?: number;
  };
}

/** Result returned from metadataEnricher.search(). */
export interface SearchResponse {
  results: ParsedMetadata[];
}

/** Input converter hook. */
export interface InputConverterHook {
  convert(context: InputConverterContext): ConvertResult;
}

/** File parser hook. */
export interface FileParserHook {
  parse(context: FileParserContext): ParsedMetadata;
}

/** Metadata enricher hook. */
export interface MetadataEnricherHook {
  /** Search for candidate results from external sources. */
  search(context: SearchContext): SearchResponse;
}

/** The global plugin object created by main.js. */
export interface ShishoPlugin {
  inputConverter?: InputConverterHook;
  fileParser?: FileParserHook;
  metadataEnricher?: MetadataEnricherHook;

  /**
   * Optional lifecycle hook called before the plugin is uninstalled.
   * Use this to clean up resources (revoke tokens, delete caches, close connections).
   * Errors in this hook do not prevent uninstall.
   */
  onUninstalling?: () => void;
}
