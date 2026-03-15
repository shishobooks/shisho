import type { ReactNode } from "react";

import { useSSE } from "@/hooks/useSSE";

export function SSEProvider({ children }: { children: ReactNode }) {
  useSSE();
  return <>{children}</>;
}
