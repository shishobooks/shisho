import { useContext } from "react";

import { BulkDownloadContext } from "@/contexts/BulkDownload";

export const useBulkDownload = () => {
  const context = useContext(BulkDownloadContext);
  if (!context) {
    throw new Error(
      "useBulkDownload must be used within a BulkDownloadProvider",
    );
  }
  return context;
};
