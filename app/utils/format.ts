// Shared formatting utilities

/**
 * Formats bytes into a human-readable size in 1024-byte units, rounded to two
 * decimals. Values past the largest unit stay in terabytes.
 * @example formatFileSize(1024) // "1 KB"
 * @example formatFileSize(2 * 1024 ** 4) // "2 TB"
 */
export const formatFileSize = (bytes: number): string => {
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  if (bytes <= 0) return "0 B";
  const i = Math.min(
    sizes.length - 1,
    Math.max(0, Math.floor(Math.log(bytes) / Math.log(1024))),
  );
  return `${Math.round((bytes / 1024 ** i) * 100) / 100} ${sizes[i]}`;
};

/**
 * The label for a file: the server-resolved display_name (a supplement's is
 * its filename), or the file type when the file has neither a name nor a
 * path, which happens only in the Share Link payload.
 * @example fileLabel({ display_name: "", file_type: "epub" }) // "EPUB"
 */
export const fileLabel = (file: {
  display_name: string;
  file_type: string;
}): string => file.display_name || file.file_type.toUpperCase();

/**
 * Formats a page count with the right plural.
 * @example formatPageCount(1) // "1 page"
 * @example formatPageCount(12) // "12 pages"
 */
export const formatPageCount = (count: number): string =>
  `${count} ${count === 1 ? "page" : "pages"}`;

/**
 * Formats seconds into a human-readable duration string.
 * @example formatDuration(3661) // "1h 1m"
 */
export const formatDuration = (seconds: number): string => {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  return `${minutes}m`;
};

/**
 * Formats the time between two ISO timestamps, or from `start` until now when
 * `end` is omitted, as milliseconds, seconds, or minutes and seconds.
 * @example formatElapsed("2024-01-01T00:00:00Z", "2024-01-01T00:03:05Z") // "3m 5s"
 */
export const formatElapsed = (start: string, end?: string | null): string => {
  const endMs = end ? new Date(end).getTime() : Date.now();
  const durationMs = endMs - new Date(start).getTime();
  if (durationMs < 1000) {
    return `${durationMs}ms`;
  }
  const seconds = Math.floor(durationMs / 1000);
  if (seconds < 60) {
    return `${seconds}s`;
  }
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
};

/**
 * Formats a number of seconds into a clock-style media player timestamp.
 * Uses M:SS for durations under an hour and H:MM:SS for longer ones, with
 * seconds truncated (not rounded). Non-finite, NaN, and negative inputs clamp
 * to "0:00" so the player never shows garbage while metadata is loading.
 * @example formatPlayerTime(185) // "3:05"
 * @example formatPlayerTime(3661) // "1:01:01"
 */
export const formatPlayerTime = (seconds: number): string => {
  const total =
    Number.isFinite(seconds) && seconds > 0 ? Math.floor(seconds) : 0;
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;

  const ss = String(secs).padStart(2, "0");
  if (hours > 0) {
    const mm = String(minutes).padStart(2, "0");
    return `${hours}:${mm}:${ss}`;
  }
  return `${minutes}:${ss}`;
};

/**
 * Formats an ISO date string into a localized date string.
 * @example formatDate("2024-01-15T12:00:00Z") // "Jan 15, 2024" (locale-dependent)
 */
export const formatDate = (dateString: string): string => {
  return new Date(dateString).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    timeZone: "UTC",
  });
};

/**
 * Formats an ISO datetime string into a localized date+time string in the user's timezone.
 * Includes the short timezone name so it's clear the time is local.
 * @example formatDateTime("2024-01-15T18:30:00Z") // "1/15/2024, 12:30 PM CST" (locale-dependent)
 */
export const formatDateTime = (dateString: string): string => {
  return new Date(dateString).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZoneName: "short",
  });
};

/**
 * Extracts the filename from a filepath.
 * @example getFilename("/path/to/file.txt") // "file.txt"
 */
export const getFilename = (filepath: string): string => {
  return filepath.split("/").pop() || filepath;
};

/**
 * Formats milliseconds as HH:MM:SS.mmm timestamp.
 * @example formatTimestamp(3661500) // "01:01:01.500"
 */
export const formatTimestamp = (ms: number): string => {
  const hours = Math.floor(ms / 3600000);
  const minutes = Math.floor((ms % 3600000) / 60000);
  const seconds = Math.floor((ms % 60000) / 1000);
  const millis = ms % 1000;

  const hh = String(hours).padStart(2, "0");
  const mm = String(minutes).padStart(2, "0");
  const ss = String(seconds).padStart(2, "0");
  const mmm = String(millis).padStart(3, "0");

  return `${hh}:${mm}:${ss}.${mmm}`;
};

/**
 * Human-readable labels for plugin metadata field names.
 */
export const METADATA_FIELD_LABELS: Record<string, string> = {
  title: "Title",
  subtitle: "Subtitle",
  authors: "Authors",
  narrators: "Narrators",
  series: "Series",
  seriesNumber: "Series Number",
  genres: "Genres",
  tags: "Tags",
  description: "Description",
  publisher: "Publisher",
  url: "URL",
  releaseDate: "Release Date",
  cover: "Cover Image",
  identifiers: "Identifiers",
  language: "Language",
  abridged: "Abridged",
};

/**
 * Formats a metadata field name into a human-readable label.
 * @example formatMetadataFieldLabel("releaseDate") // "Release Date"
 */
export const formatMetadataFieldLabel = (field: string): string => {
  return METADATA_FIELD_LABELS[field] ?? field;
};

/**
 * Formats identifier type codes into human-readable labels.
 * @example formatIdentifierType("isbn_13") // "ISBN-13"
 */
export function formatIdentifierType(
  type: string,
  pluginTypes?: Array<{ id: string; name: string }>,
): string {
  switch (type) {
    case "isbn_10":
      return "ISBN-10";
    case "isbn_13":
      return "ISBN-13";
    case "asin":
      return "ASIN";
    case "uuid":
      return "UUID";
    case "goodreads":
      return "Goodreads";
    case "google":
      return "Google";
    case "other":
      return "Other";
    default: {
      const pluginType = pluginTypes?.find((pt) => pt.id === type);
      if (pluginType) {
        return pluginType.name;
      }
      return type;
    }
  }
}
