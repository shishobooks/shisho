import { Download, Loader2, X } from "lucide-react";

import DownloadFormatPopover from "@/components/library/DownloadFormatPopover";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAuth } from "@/hooks/useAuth";
import { DownloadFormatAsk, type File } from "@/types";
import { supportsKepub } from "@/utils/supportsKepub";

interface FileDownloadControlProps {
  file: Pick<File, "id" | "file_type">;
  libraryDownloadPreference: string | undefined;
  isSupplement?: boolean;
  isDownloading: boolean;
  onDownload: () => void;
  onDownloadKepub: () => void;
  onDownloadOriginal: () => void;
  onDownloadWithEndpoint: (endpoint: string) => void;
  onCancelDownload: () => void;
}

// Download button or format popover for one file on the book detail page.
// Hidden in Demo Mode, where downloads are disabled.
const FileDownloadControl = ({
  file,
  libraryDownloadPreference,
  isSupplement = false,
  isDownloading,
  onDownload,
  onDownloadKepub,
  onDownloadOriginal,
  onDownloadWithEndpoint,
  onCancelDownload,
}: FileDownloadControlProps) => {
  const { demoMode } = useAuth();

  if (demoMode) {
    return null;
  }

  if (isSupplement) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            aria-label="Download"
            onClick={onDownloadOriginal}
            size="sm"
            variant="ghost"
          >
            <Download className="h-3 w-3" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Download</TooltipContent>
      </Tooltip>
    );
  }

  if (
    libraryDownloadPreference === DownloadFormatAsk &&
    supportsKepub(file.file_type)
  ) {
    return (
      <DownloadFormatPopover
        disabled={isDownloading}
        isLoading={isDownloading}
        onCancel={onCancelDownload}
        onDownloadKepub={onDownloadKepub}
        onDownloadOriginal={() =>
          onDownloadWithEndpoint(`/api/books/files/${file.id}/download`)
        }
      />
    );
  }

  if (isDownloading) {
    return (
      <div className="flex items-center gap-1">
        <Loader2 className="h-3 w-3 animate-spin" />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              className="h-6 w-6 p-0"
              onClick={onCancelDownload}
              size="sm"
              variant="ghost"
            >
              <X className="h-3 w-3" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Cancel download</TooltipContent>
        </Tooltip>
      </div>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          aria-label="Download"
          onClick={onDownload}
          size="sm"
          variant="ghost"
        >
          <Download className="h-3 w-3" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Download</TooltipContent>
    </Tooltip>
  );
};

export default FileDownloadControl;
