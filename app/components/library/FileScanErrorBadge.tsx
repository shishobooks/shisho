import { AlertTriangle } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { File } from "@/types";

interface FileScanErrorBadgeProps {
  file: Pick<File, "scan_error">;
  className?: string;
  /**
   * Whether the badge takes focus, so the keyboard can open its tooltip.
   * Defaults to true. Pass false inside a link or button, which cannot hold
   * another focusable control; an sr-only copy of the error then reaches
   * screen readers instead.
   */
  interactive?: boolean;
}

/**
 * Flags a file whose most recent scan could not read it (for example a
 * truncated EPUB). The badge is only rendered when `scan_error` is set; the
 * underlying error is shown in a tooltip so the file row stays compact.
 */
const FileScanErrorBadge = ({
  file,
  className,
  interactive = true,
}: FileScanErrorBadgeProps) => {
  if (!file.scan_error) {
    return null;
  }

  const content = (
    <>
      <AlertTriangle aria-hidden="true" />
      Unreadable
    </>
  );

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          {interactive ? (
            <Badge asChild className={className} variant="destructive">
              <Button
                className="cursor-help focus-visible:ring-1 focus-visible:ring-ring dark:focus-visible:ring-ring"
                variant="unstyled"
              >
                {content}
              </Button>
            </Badge>
          ) : (
            <Badge className={className} variant="destructive">
              {content}
            </Badge>
          )}
        </TooltipTrigger>
        <TooltipContent className="max-w-xs">
          <p>The last scan could not read this file.</p>
          <p className="mt-1 font-mono text-xs break-all">{file.scan_error}</p>
        </TooltipContent>
      </Tooltip>
      {!interactive && (
        <span className="sr-only">
          The last scan could not read this file: {file.scan_error}
        </span>
      )}
    </>
  );
};

export default FileScanErrorBadge;
