// Shared formatting utilities

/**
 * Formats bytes into a human-readable file size string.
 * @example formatFileSize(1024) // "1 KB"
 */
export const formatFileSize = (bytes: number): string => {
  const sizes = ["B", "KB", "MB", "GB"];
  if (bytes === 0) return "0 B";
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return Math.round((bytes / Math.pow(1024, i)) * 100) / 100 + " " + sizes[i];
};

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
 * Formats an ISO date string into a localized date string.
 * @example formatDate("2024-01-15T12:00:00Z") // "1/15/2024" (locale-dependent)
 */
export const formatDate = (dateString: string): string => {
  return new Date(dateString).toLocaleDateString();
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
 * Formats identifier type codes into human-readable labels.
 * @example formatIdentifierType("isbn_13") // "ISBN-13"
 */
export function formatIdentifierType(type: string): string {
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
    default:
      return type;
  }
}
