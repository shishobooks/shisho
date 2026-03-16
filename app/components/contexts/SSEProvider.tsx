import type { ReactNode } from "react";

import { BulkDownloadProvider } from "@/contexts/BulkDownload";
import { useSSE } from "@/hooks/useSSE";

function SSEListener() {
  useSSE();
  return null;
}

export function SSEProvider({ children }: { children: ReactNode }) {
  return (
    <BulkDownloadProvider>
      <SSEListener />
      {children}
    </BulkDownloadProvider>
  );
}
