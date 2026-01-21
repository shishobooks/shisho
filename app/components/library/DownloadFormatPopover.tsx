import { Download, Loader2, X } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

interface DownloadFormatPopoverProps {
  onDownloadOriginal: () => void;
  onDownloadKepub: () => void;
  onCancel?: () => void;
  isLoading?: boolean;
  disabled?: boolean;
}

const DownloadFormatPopover = ({
  onDownloadOriginal,
  onDownloadKepub,
  onCancel,
  isLoading = false,
  disabled = false,
}: DownloadFormatPopoverProps) => {
  const [open, setOpen] = useState(false);

  const handleOriginal = () => {
    setOpen(false);
    onDownloadOriginal();
  };

  const handleKepub = () => {
    setOpen(false);
    onDownloadKepub();
  };

  // When loading, show spinner + cancel button instead of popover trigger
  if (isLoading) {
    return (
      <div className="flex items-center gap-1">
        <Loader2 className="h-3 w-3 animate-spin" />
        {onCancel && (
          <Button
            className="h-6 w-6 p-0"
            onClick={onCancel}
            size="sm"
            title="Cancel download"
            variant="ghost"
          >
            <X className="h-3 w-3" />
          </Button>
        )}
      </div>
    );
  }

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>
        <Button disabled={disabled} size="sm" title="Download" variant="ghost">
          <Download className="h-3 w-3" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-48 p-0">
        <p className="text-xs font-medium text-muted-foreground px-3 py-2">
          Download format
        </p>
        <div className="flex flex-col gap-0.5 px-1 pb-1">
          <Button
            className="justify-start"
            onClick={handleOriginal}
            size="sm"
            variant="ghost"
          >
            Original
          </Button>
          <Button
            className="justify-start"
            onClick={handleKepub}
            size="sm"
            variant="ghost"
          >
            KePub (Kobo)
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
};

export default DownloadFormatPopover;
