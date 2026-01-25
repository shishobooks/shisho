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

/** Context passed to metadataEnricher.enrich(). */
export interface MetadataEnricherContext {
  /** Metadata freshly parsed from the file on disk (e.g., from ComicInfo.xml, OPF, etc.). */
  parsedMetadata: {
    title?: string;
    subtitle?: string;
    series?: string;
    seriesNumber?: number;
    description?: string;
    publisher?: string;
    imprint?: string;
    url?: string;
    dataSource?: string;
    authors?: Array<{ name: string; role?: string }>;
    narrators?: string[];
    genres?: string[];
    tags?: string[];
    releaseDate?: string;
    identifiers?: Array<{ type: string; value: string }>;
  };
  /** File information from the database. */
  file: {
    id?: number;
    filepath?: string;
    fileType?: string;
    fileRole?: string;
    filesizeBytes?: number;
    name?: string;
    url?: string;
  };
  /** Current book state from the database, including manually-edited fields. */
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
  enrich(context: MetadataEnricherContext): EnrichmentResult;
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
}
