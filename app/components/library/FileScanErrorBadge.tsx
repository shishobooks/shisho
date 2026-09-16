import { AlertTriangle } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { File } from "@/types";

interface FileScanErrorBadgeProps {
  file: Pick<File, "scan_error">;
  className?: string;
}

/**
 * Flags a file whose most recent scan could not read it (for example a
 * truncated EPUB). The badge is only rendered when `scan_error` is set; the
 * underlying error is shown in a tooltip so the file row stays compact.
 */
const FileScanErrorBadge = ({ file, className }: FileScanErrorBadgeProps) => {
  if (!file.scan_error) {
    return null;
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge className={className} variant="destructive">
          <AlertTriangle aria-hidden="true" />
          Unreadable
        </Badge>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">
        <p>The last scan could not read this file.</p>
        <p className="mt-1 font-mono text-xs break-all">{file.scan_error}</p>
      </TooltipContent>
    </Tooltip>
  );
};

export default FileScanErrorBadge;
