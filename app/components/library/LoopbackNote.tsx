import { AlertTriangle } from "lucide-react";
import type { ReactNode } from "react";

import { isLoopbackHost, pageHostname } from "@/utils/location";

/**
 * LoopbackNote warns, only while the page is open at localhost or another
 * loopback address, that URLs built from the page's address can't be
 * reached from anywhere else. `children` says what to do about it and gets
 * the hostname, styled to drop into the sentence.
 */
const LoopbackNote = ({
  children,
}: {
  children: (host: ReactNode) => ReactNode;
}) => {
  const host = pageHostname();
  if (!isLoopbackHost(host)) return null;
  return (
    <div
      className="flex gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400"
      role="note"
    >
      <AlertTriangle
        aria-hidden="true"
        className="mt-px h-3.5 w-3.5 shrink-0"
      />
      <p>{children(<code className="break-all font-mono">{host}</code>)}</p>
    </div>
  );
};

export default LoopbackNote;
