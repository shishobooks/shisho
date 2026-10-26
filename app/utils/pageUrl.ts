import {
  CBZPageKey,
  FileTypePDF,
  ThumbnailRenderKey,
  type File,
  type ThumbnailSize,
} from "@/types";

import { fileCacheKey, thumbnailSizeFor } from "./coverUrl";

// The largest page thumbnail worth asking for. A PDF page rendered bigger
// can cost more than the whole page at the server's render DPI, so a box
// past it gets the whole page.
const MAX_PAGE_THUMBNAIL = 1024;

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
 *
 * With a `size`, the URL asks for a thumbnail in the tiers cover URLs use
 * instead of the whole page: a CBZ page resized through the server's shared
 * thumbnail cache, a PDF page rendered at that size instead of at the full
 * DPI. Its `r` adds the thumbnail renderer's key to the page's
 * (`covers.PageThumbnailKey`). A size past 1024 gets the whole page.
 */
export const filePageUrl = (
  file: PageSourceFile,
  page: number,
  pdfRenderKey: string,
  size?: ThumbnailSize,
): string => {
  const url = `/api/books/files/${file.id}/page/${page}?v=${fileCacheKey(file)}`;
  const key = file.file_type === FileTypePDF ? pdfRenderKey : CBZPageKey;
  if (size && size <= MAX_PAGE_THUMBNAIL) {
    const thumbnailKey = `${key}.${ThumbnailRenderKey}`;
    return `${url}&size=${size}&r=${encodeURIComponent(thumbnailKey)}`;
  }
  return `${url}&r=${encodeURIComponent(key)}`;
};

/**
 * The thumbnail tier for a page image drawn in a fixed box `cssPixels` on
 * its longest side on this screen. Undefined past the largest page
 * thumbnail, where the whole page is the better image.
 */
export const pageThumbnailSize = (
  cssPixels: number,
): ThumbnailSize | undefined => {
  const size = thumbnailSizeFor(cssPixels);
  return size && size <= MAX_PAGE_THUMBNAIL ? size : undefined;
};
