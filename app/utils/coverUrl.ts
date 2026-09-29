import type { Book, File, Series } from "@/types";

// Cover endpoints are served `private, max-age=31536000, immutable`, so the
// browser never revalidates a cover URL it has seen. Every cover URL carries a
// `?v=` cache key that changes when the cover does, and every one is built
// here so no call site can leave the key off. See "Cover Image Caching" in
// app/AGENTS.md.

/** The fields of a book a cover URL depends on. */
export type CoverSourceBook = Pick<Book, "id" | "cover_cache_key">;

/** The fields of a series a cover URL depends on. */
export type CoverSourceSeries = Pick<Series, "id" | "cover_cache_key">;

/** The fields of a file a cover or page URL depends on. */
export type CoverSourceFile = Pick<File, "id" | "updated_at">;

/**
 * The cache key for anything served from a file: its `updated_at` in epoch
 * milliseconds. File covers and page images share it.
 */
export const fileCacheKey = (file: Pick<File, "updated_at">): number =>
  new Date(file.updated_at).getTime();

/**
 * The book cover URL, keyed on the backend-computed `cover_cache_key`, which
 * changes only when the file the cover is chosen from changes.
 */
export const bookCoverUrl = (book: CoverSourceBook): string =>
  `/api/books/${book.id}/cover?v=${book.cover_cache_key}`;

/** The series cover URL, keyed like a book cover. */
export const seriesCoverUrl = (series: CoverSourceSeries): string =>
  `/api/series/${series.id}/cover?v=${series.cover_cache_key}`;

/** The file cover URL, keyed on the file's `updated_at`. */
export const fileCoverUrl = (file: CoverSourceFile): string =>
  `/api/books/files/${file.id}/cover?v=${fileCacheKey(file)}`;

/**
 * The endpoint a new file cover is POSTed to. It carries no cache key, so
 * never render it: show the cover through `fileCoverUrl`.
 */
export const fileCoverUploadUrl = (fileId: number): string =>
  `/api/books/files/${fileId}/cover`;

const shareBase = (token: string) => `/api/share/${encodeURIComponent(token)}`;

/** The book cover a Share Link serves, keyed like `bookCoverUrl`. */
export const shareBookCoverUrl = (
  token: string,
  book: CoverSourceBook,
): string => `${shareBase(token)}/cover?v=${book.cover_cache_key}`;

/** A file cover a Share Link serves, keyed like `fileCoverUrl`. */
export const shareFileCoverUrl = (
  token: string,
  file: CoverSourceFile,
): string =>
  `${shareBase(token)}/files/${file.id}/cover?v=${fileCacheKey(file)}`;
