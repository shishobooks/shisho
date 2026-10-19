import type { DeliveredFileType } from "@/types";

// Download and stream endpoints are served `private, no-store`, so these URLs
// carry no cache key. They live here, beside the cover and page URL helpers,
// so the path of each endpoint is written once. ESLint rejects a literal
// download or stream URL anywhere else in the app.

/** The file as the server generates it (with any metadata written in). */
export const fileDownloadUrl = (fileId: number): string =>
  `/api/books/files/${fileId}/download`;

/**
 * The file delivered as a File Type it can become: its own, or one a
 * Conversion reaches (`file.reachable_file_types`).
 */
export const fileDownloadAsUrl = (
  fileId: number,
  fileType: DeliveredFileType,
): string => `/api/books/files/${fileId}/download/${fileType}`;

/** The file exactly as it sits on disk. */
export const fileOriginalDownloadUrl = (fileId: number): string =>
  `/api/books/files/${fileId}/download/original`;

/** The audio stream an audiobook player plays. */
export const fileStreamUrl = (fileId: number): string =>
  `/api/books/files/${fileId}/stream`;

/** The zip a bulk download job produced. */
export const bulkDownloadUrl = (jobId: number): string =>
  `/api/jobs/${jobId}/download`;

/** A file download through a Share Link. */
export const shareFileDownloadUrl = (token: string, fileId: number): string =>
  `/api/share/${encodeURIComponent(token)}/files/${fileId}/download`;

/** A file download through a Share Link, as a File Type it can become. */
export const shareFileDownloadAsUrl = (
  token: string,
  fileId: number,
  fileType: DeliveredFileType,
): string =>
  `/api/share/${encodeURIComponent(token)}/files/${fileId}/download/${fileType}`;
