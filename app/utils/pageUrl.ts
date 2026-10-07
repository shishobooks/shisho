import { CBZPageKey, FileTypePDF, type File } from "@/types";

import { fileCacheKey } from "./coverUrl";

/**
 * The fields of a file that a page image URL depends on: the ones a file
 * cover is keyed by, plus the file type, which picks the page key.
 */
export type PageSourceFile = Pick<File, "id" | "updated_at" | "file_type">;

/**
 * Returns the URL of a CBZ or PDF page image (0-indexed page).
 *
 * The page endpoint is served as `immutable`, so the URL carries the file's
 * `updated_at` as a cache key, in epoch milliseconds like file cover URLs
 * (`fileCacheKey`). A rescan that finds the file changed on disk bumps
 * `updated_at` and drops the server's cached pages, so the browser fetches
 * the new pages instead of reusing the old ones.
 *
 * The URL also carries, as `r`, what else decides the page image. PDF pages
 * are rendered with the server's `pdf_render_dpi` and `pdf_render_quality`,
 * so their key is `pdfRenderKey` (from GET /auth/status): a restart with new
 * settings changes the URL, and the browser fetches pages at the new
 * settings. CBZ pages carry `CBZPageKey`, which the server bumps when a page
 * number starts naming a different image (a new page order). Components get
 * this function, with the PDF key filled in, from `useFilePageUrl`, so no
 * call site omits a key.
 */
export const filePageUrl = (
  file: PageSourceFile,
  page: number,
  pdfRenderKey: string,
): string => {
  const url = `/api/books/files/${file.id}/page/${page}?v=${fileCacheKey(file)}`;
  const key = file.file_type === FileTypePDF ? pdfRenderKey : CBZPageKey;
  return `${url}&r=${encodeURIComponent(key)}`;
};
