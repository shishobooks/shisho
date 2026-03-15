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
  /** Search query (book title for auto, user input for manual). */
  query: string;
  /** Current book state from the database. */
  book: {
    id?: number;
    title?: string;
    subtitle?: string;
    description?: string;
    series?: Array<{ name: string; number?: number }>;
    authors?: Array<{ name: string; role?: string }>;
    genres?: string[];
    tags?: string[];
    identifiers?: Array<{ type: string; value: string }>;
    publisher?: string;
  };
  /** File information. */
  file: {
    fileType?: string;
    filePath?: string;
  };
}

/** A single search result from metadataEnricher.search(). */
export interface SearchResult {
  title: string;
  authors?: string[];
  description?: string;
  imageUrl?: string;
  releaseDate?: string;
  publisher?: string;
  subtitle?: string;
  series?: string;
  seriesNumber?: number;
  genres?: string[];
  tags?: string[];
  narrators?: string[];
  identifiers?: Array<{ type: string; value: string }>;
  /** Opaque data passed back to enrich(). Use this to store internal IDs. */
  providerData?: unknown;
  /** Full metadata for passthrough pattern. If provided, enrich() can return it as-is. */
  metadata?: ParsedMetadata;
}

/** Result returned from metadataEnricher.search(). */
export interface SearchResponse {
  results: SearchResult[];
}

/** Context passed to metadataEnricher.enrich(). */
export interface EnrichContext {
  /** The selected search result's providerData. */
  selectedResult: unknown;
  /** Current book state from the database. */
  book: {
    id?: number;
    title?: string;
    subtitle?: string;
    description?: string;
    series?: Array<{ name: string; number?: number }>;
    authors?: Array<{ name: string; role?: string }>;
    genres?: string[];
    tags?: string[];
    identifiers?: Array<{ type: string; value: string }>;
    publisher?: string;
  };
  /** File information. */
  file: {
    fileType?: string;
    filePath?: string;
  };
}

/** Result returned from metadataEnricher.enrich(). */
export interface EnrichmentResult {
  /** Whether metadata was modified. */
  modified: boolean;
  /** Updated metadata (only used if modified is true). */
  metadata?: ParsedMetadata;
}

/** Context passed to outputGenerator.generate(). */
export interface OutputGeneratorContext {
  /** Path to the source book file. */
  sourcePath: string;
  /** Path where the output file should be written. */
  destPath: string;
  /** Book metadata. */
  book: {
    title?: string;
    authors?: Array<{ name: string; role?: string }>;
    series?: string;
    seriesNumber?: number;
    publisher?: string;
    description?: string;
    genres?: string[];
    tags?: string[];
    identifiers?: Array<{ type: string; value: string }>;
  };
  /** File metadata. */
  file: {
    fileType?: string;
    filePath?: string;
  };
}

/** Context passed to outputGenerator.fingerprint(). */
export interface FingerprintContext {
  /** Book metadata. */
  book: {
    title?: string;
    authors?: Array<{ name: string; role?: string }>;
    series?: string;
    seriesNumber?: number;
    publisher?: string;
  };
  /** File metadata. */
  file: {
    fileType?: string;
    filePath?: string;
  };
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
  /** Enrich metadata from a selected search result. */
  enrich(context: EnrichContext): EnrichmentResult;
}

/** Output generator hook. */
export interface OutputGeneratorHook {
  generate(context: OutputGeneratorContext): void;
  fingerprint(context: FingerprintContext): string;
}

/** The plugin object exported by main.js via IIFE. */
export interface ShishoPlugin {
  inputConverter?: InputConverterHook;
  fileParser?: FileParserHook;
  metadataEnricher?: MetadataEnricherHook;
  outputGenerator?: OutputGeneratorHook;

  /**
   * Optional lifecycle hook called before the plugin is uninstalled.
   * Use this to clean up resources (revoke tokens, delete caches, close connections).
   * Errors in this hook do not prevent uninstall.
   */
  onUninstalling?: () => void;
}
