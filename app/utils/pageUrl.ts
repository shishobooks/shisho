import type { File } from "@/types";

/** The fields of a file that a page image URL depends on. */
export type PageSourceFile = Pick<File, "id" | "updated_at">;

/**
 * Returns the URL of a CBZ or PDF page image (0-indexed page).
 *
 * The page endpoint is served as `immutable`, so the URL carries the file's
 * `updated_at` as a cache key, in epoch milliseconds like file cover URLs. A
 * rescan that finds the file changed on disk bumps `updated_at` and drops the
 * server's cached pages, so the browser fetches the new pages instead of
 * reusing the old ones. Build every page URL
 * through this helper so no call site omits the key.
 */
export const filePageUrl = (file: PageSourceFile, page: number): string =>
  `/api/books/files/${file.id}/page/${page}?v=${new Date(file.updated_at).getTime()}`;
