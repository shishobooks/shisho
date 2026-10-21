import { useState, type ReactNode } from "react";

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useIsTruncated } from "@/hooks/useIsTruncated";

interface TruncatedTextProps {
  children: ReactNode;
  /** Classes that truncate the text, such as `truncate` or `line-clamp-2`. */
  className?: string;
  /** The tooltip's text, when the children hold more than the text. */
  tooltip?: ReactNode;
}

/**
 * Text that may be cut off with an ellipsis, with the full text in a tooltip
 * only while it is. Gallery titles and list entries use it.
 */
const TruncatedText = ({
  children,
  className,
  tooltip = children,
}: TruncatedTextProps) => {
  const [ref, isTruncated] = useIsTruncated<HTMLDivElement>();
  // Always controlled: switching `open` between false and undefined as the
  // text starts or stops overflowing makes React warn. Only a cut-off text
  // records a hover as open; Radix never reports the close of a tooltip it
  // was told stays shut, so a recorded open would pop it up later, when the
  // text first overflows.
  const [open, setOpen] = useState(false);
  return (
    <Tooltip
      delayDuration={300}
      onOpenChange={(next) => setOpen(next && isTruncated)}
      open={isTruncated && open}
    >
      <TooltipTrigger asChild>
        <div className={className} ref={ref}>
          {children}
        </div>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
};

export default TruncatedText;
