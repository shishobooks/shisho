import type { ReactNode } from "react";

import { useSSE } from "@/hooks/useSSE";

/** The app mounts useSSE once at the root; the page reads what it receives. */
export const WithSSE = ({ children }: { children: ReactNode }) => {
  useSSE();
  return <>{children}</>;
};
