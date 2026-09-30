import { useCallback } from "react";

import { useAuth } from "@/hooks/useAuth";
import { filePageUrl, type PageSourceFile } from "@/utils/pageUrl";

/**
 * Returns `filePageUrl` with the server's PDF render key filled in. Build
 * every page image URL through it.
 */
export const useFilePageUrl = () => {
  const { pdfRenderKey } = useAuth();
  return useCallback(
    (file: PageSourceFile, page: number) =>
      filePageUrl(file, page, pdfRenderKey),
    [pdfRenderKey],
  );
};
