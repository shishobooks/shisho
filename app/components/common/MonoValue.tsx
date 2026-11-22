import type { ReactNode } from "react";

import { cn } from "@/libraries/utils";

/**
 * A value read character by character, like a path or an identifier, in the
 * monospace font. A size below the text around it, since monospace letters
 * read larger, and breaking anywhere, since paths have few spaces.
 */
const MonoValue = ({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) => (
  <span className={cn("break-all font-mono text-[13px]", className)}>
    {children}
  </span>
);

export default MonoValue;
