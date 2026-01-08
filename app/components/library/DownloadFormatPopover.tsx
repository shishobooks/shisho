import { Download, Loader2 } from "lucide-react";
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
  isLoading?: boolean;
  disabled?: boolean;
}

const DownloadFormatPopover = ({
  onDownloadOriginal,
  onDownloadKepub,
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

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>
        <Button
          disabled={disabled || isLoading}
          size="sm"
          title="Download"
          variant="ghost"
        >
          {isLoading ? (
            <Loader2 className="h-3 w-3 animate-spin" />
          ) : (
            <Download className="h-3 w-3" />
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-48 p-2">
        <div className="flex flex-col gap-1">
          <p className="text-xs font-medium text-muted-foreground px-2 py-1">
            Download format
          </p>
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
