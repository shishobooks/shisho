/** An author with optional role information. */
export interface ParsedAuthor {
  /** Author name. */
  name: string;
  /** Role (empty for generic author, or one of: writer, penciller, inker, colorist, letterer, cover_artist, editor, translator). */
  role?: string;
}

/** An identifier parsed from file metadata. */
export interface ParsedIdentifier {
  /** Identifier type (e.g., isbn_10, isbn_13, asin, uuid, goodreads, google, other). */
  type: string;
  /** The identifier value. */
  value: string;
}

/** A chapter parsed from file metadata. */
export interface ParsedChapter {
  /** Chapter title. */
  title: string;
  /** CBZ: 0-indexed page number. */
  startPage?: number;
  /** M4B: milliseconds from start. */
  startTimestampMs?: number;
  /** EPUB: content document href. */
  href?: string;
  /** Nested child chapters (EPUB nesting only). */
  children?: ParsedChapter[];
}

/** Full metadata object returned by file parsers and metadata enrichers. */
export interface ParsedMetadata {
  title?: string;
  /**
   * When set, overrides the auto-generated sort title derived from `title`.
   *
   * Currently honored only when results flow through the manual identify-and-
   * apply UI (i.e. `metadataEnricher.search` results that the user reviews
   * and applies). The library scan pipeline ignores this field on file-parser
   * results and always derives the sort title from `title` instead. Returning
   * `sortTitle` from a `fileParser.parse` hook will be silently dropped on
   * scan; in those cases, prefer encoding sort information into `title`.
   */
  sortTitle?: string;
  subtitle?: string;
  authors?: ParsedAuthor[];
  narrators?: string[];
  series?: string;
  seriesNumber?: number;
  /** Whether the series number refers to a volume or a chapter. CBZ only. */
  seriesNumberUnit?: "volume" | "chapter";
  genres?: string[];
  tags?: string[];
  description?: string;
  publisher?: string;
  imprint?: string;
  url?: string;
  /** BCP 47 language tag (e.g., "en", "en-US", "zh-Hans"). */
  language?: string;
  /** Whether this is an abridged edition. true=abridged, false=unabridged, undefined=unknown. */
  abridged?: boolean;
  /** ISO 8601 date string (e.g., "2023-06-15T00:00:00Z"). */
  releaseDate?: string;
  /** MIME type of cover image (e.g., "image/jpeg"). */
  coverMimeType?: string;
  /**
   * URL to download the cover image from. The server handles downloading and validates
   * the domain against the plugin's httpAccess.domains. This is the recommended way for
   * enricher plugins to provide covers.
   */
  coverUrl?: string;
  /**
   * Raw cover image data as an ArrayBuffer. Use this for file parsers that extract
   * embedded covers, or enrichers that generate/composite images. If both coverData
   * and coverUrl are set, coverData takes precedence (no download occurs).
   */
  coverData?: ArrayBuffer;
  /** 0-indexed page number for CBZ cover. */
  coverPage?: number;
  /** Duration in seconds (float, M4B files). */
  duration?: number;
  /** Audio bitrate in bits per second (M4B files). */
  bitrateBps?: number;
  /** Number of pages (CBZ files). */
  pageCount?: number;
  identifiers?: ParsedIdentifier[];
  chapters?: ParsedChapter[];
  /**
   * Confidence score (0-1) indicating how well this result matches the search query.
   * Used by the scan pipeline to decide whether to auto-apply enrichment.
   * If omitted, the result is always applied (backwards compatible).
   */
  confidence?: number;
}
