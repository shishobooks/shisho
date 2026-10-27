import { useFilePageUrl } from "@/hooks/useFilePageUrl";
import {
  FileTypeCBZ,
  FileTypePDF,
  type File,
  type ThumbnailSize,
} from "@/types";
import { fileCoverUrl } from "@/utils/coverUrl";

export interface FileImage {
  url: string;
  /** A page of the file rather than a cover. */
  isPage: boolean;
  /**
   * For a page, the URL of its thumbnail at a size: CoverImage's
   * `thumbnailUrl`, since a page URL is not a cover's.
   */
  thumbnailUrl?: (size: ThumbnailSize) => string;
}

/**
 * The picture that stands for a file: its cover, or for a PDF or CBZ with
 * no cover (often a supplement) its first page. `getCoverUrl` replaces the
 * lookup on a Share Link page, which has no page endpoint, so it gets no
 * page fallback.
 */
export const useFileImage = (
  file: File,
  getCoverUrl?: (file: File) => string | null,
): FileImage | null => {
  const pageUrl = useFilePageUrl();
  if (getCoverUrl) {
    const url = getCoverUrl(file);
    return url ? { url, isPage: false } : null;
  }
  if (file.cover_image_filename) {
    return { url: fileCoverUrl(file), isPage: false };
  }
  if (
    (file.file_type === FileTypeCBZ || file.file_type === FileTypePDF) &&
    (file.page_count ?? 0) > 0
  ) {
    return {
      url: pageUrl(file, 0),
      isPage: true,
      thumbnailUrl: (size) => pageUrl(file, 0, size),
    };
  }
  return null;
};
