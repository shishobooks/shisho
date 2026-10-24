import { useCallback } from "react";

import { useAuth } from "@/hooks/useAuth";
import type { ThumbnailSize } from "@/types";
import { filePageUrl, type PageSourceFile } from "@/utils/pageUrl";

/**
 * Returns `filePageUrl` with the server's PDF render key filled in. Build
 * every page image URL through it; pass a `size` for a thumbnail.
 */
export const useFilePageUrl = () => {
  const { pdfRenderKey } = useAuth();
  return useCallback(
    (file: PageSourceFile, page: number, size?: ThumbnailSize) =>
      filePageUrl(file, page, pdfRenderKey, size),
    [pdfRenderKey],
  );
};
