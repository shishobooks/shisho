import { ChevronDown, Download, Loader2, X } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAuth } from "@/hooks/useAuth";
import { DownloadFormatAsk, type DeliveredFileType, type File } from "@/types";
import { fileTitle, fileTypeLabel } from "@/utils/format";

interface FileDownloadControlProps {
  file: Pick<File, "id" | "file_type" | "name" | "reachable_file_types">;
  /** Titles a file with no name of its own. */
  bookTitle: string;
  libraryDownloadPreference: string | undefined;
  isSupplement?: boolean;
  isDownloading: boolean;
  /** The main button: the File Type the library download setting picks. */
  onDownload: () => void;
  /** A menu entry: the file delivered as one File Type it can become. */
  onDownloadAs: (fileType: DeliveredFileType) => void;
  onDownloadOriginal: () => void;
  onCancelDownload: () => void;
}

// Download button and File Type menu for one file on the book detail page
// and the Share Link page. The menu lists every File Type the file can
// become, its own first. Hidden in Demo Mode, where downloads are disabled.
const FileDownloadControl = ({
  file,
  bookTitle,
  libraryDownloadPreference,
  isSupplement = false,
  isDownloading,
  onDownload,
  onDownloadAs,
  onDownloadOriginal,
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

  if (isDownloading) {
    return (
      <div className="flex items-center gap-1">
        <Loader2 className="h-3 w-3 animate-spin" />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              aria-label="Cancel download"
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

  const title = fileTitle(file, bookTitle);
  const menu = (trigger: ReactNode, tooltip: string) => (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{tooltip}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel
          className="truncate text-xs font-medium text-muted-foreground"
          title={title}
        >
          {title}
        </DropdownMenuLabel>
        {file.reachable_file_types.map((fileType) => {
          const label = fileTypeLabel(fileType);
          return (
            <DropdownMenuItem
              aria-label={`Download ${title} as ${label}`}
              key={fileType}
              onSelect={() => onDownloadAs(fileType)}
            >
              {label}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );

  // The Ask setting has no File Type of its own, so when there is a choice
  // its one button opens the menu.
  if (
    libraryDownloadPreference === DownloadFormatAsk &&
    file.reachable_file_types.length > 1
  ) {
    return menu(
      <Button aria-label="Download" size="sm" variant="ghost">
        <Download className="h-3 w-3" />
      </Button>,
      "Download",
    );
  }

  return (
    <div className="flex items-center">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            aria-label="Download"
            className="rounded-r-none pr-1.5"
            onClick={onDownload}
            size="sm"
            variant="ghost"
          >
            <Download className="h-3 w-3" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Download</TooltipContent>
      </Tooltip>
      {menu(
        <Button
          aria-label="Download as"
          className="rounded-l-none px-1"
          size="sm"
          variant="ghost"
        >
          <ChevronDown className="h-3 w-3" />
        </Button>,
        "Download as",
      )}
    </div>
  );
};

export default FileDownloadControl;
