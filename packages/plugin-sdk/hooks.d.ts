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
}

/** Result returned from metadataEnricher.search(). */
export interface SearchResponse {
  results: ParsedMetadata[];
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
