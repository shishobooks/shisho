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
  subtitle?: string;
  authors?: ParsedAuthor[];
  narrators?: string[];
  series?: string;
  seriesNumber?: number;
  genres?: string[];
  tags?: string[];
  description?: string;
  publisher?: string;
  imprint?: string;
  url?: string;
  /** ISO 8601 date string (e.g., "2023-06-15T00:00:00Z"). */
  releaseDate?: string;
  /** MIME type of cover image (e.g., "image/jpeg"). */
  coverMimeType?: string;
  /** Cover image data as ArrayBuffer. */
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
}
