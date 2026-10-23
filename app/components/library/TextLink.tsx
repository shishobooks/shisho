import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import { cn } from "@/libraries/utils";

/**
 * A link that reads as text, with a faint underline that darkens on hover,
 * so a run of them (an author list, a series, a publisher) stays calm. With
 * no `to`, the text renders plain: a Share Link recipient, or a role the
 * target page would refuse.
 */
const TextLink = ({
  to,
  className,
  children,
}: {
  to: string | null;
  className?: string;
  children: ReactNode;
}) =>
  to ? (
    <Link
      className={cn(
        "underline decoration-foreground/25 underline-offset-4 transition-colors hover:decoration-foreground",
        className,
      )}
      to={to}
    >
      {children}
    </Link>
  ) : (
    <span className={className}>{children}</span>
  );

export default TextLink;
