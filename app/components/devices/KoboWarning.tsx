import { AlertTriangle } from "lucide-react";
import type { ReactNode } from "react";

// KoboWarning says what a Kobo's next sync will do to books already on it,
// such as moving one to a different edition or switching it between KePub
// and EPUB (ADR 0012). The rules editor and the book page share it, so the
// two warnings look the same wherever they appear.
const KoboWarning = ({ children }: { children: ReactNode }) => (
  <p className="flex gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400">
    <AlertTriangle aria-hidden="true" className="mt-px h-3.5 w-3.5 shrink-0" />
    <span>{children}</span>
  </p>
);

export default KoboWarning;
