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
import type { DeliveredFileType, File } from "@/types";
import { fileTitle, fileTypeLabel } from "@/utils/format";

interface FileDownloadControlProps {
  file: Pick<File, "id" | "file_type" | "name" | "reachable_file_types">;
  /** Titles a file with no name of its own. */
  bookTitle: string;
  /**
   * The File Type the main button delivers the file as, or undefined while
   * the rules that decide it load, which disables the button.
   */
  deliveredFileType: DeliveredFileType | undefined;
  isSupplement?: boolean;
  isDownloading: boolean;
  /** The main button: the file delivered as `deliveredFileType`. */
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
  deliveredFileType,
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

  // The button shows the format; its name adds the action around the visible
  // text, so it reads "Download EPUB" and still contains what is on screen.
  const label = (fileType: string) => (
    <>
      <Download aria-hidden="true" className="h-3.5 w-3.5" />
      {fileTypeLabel(fileType)}
    </>
  );

  if (isSupplement) {
    return (
      <Button
        aria-label={`Download ${fileTypeLabel(file.file_type)}`}
        className="gap-1.5"
        onClick={onDownloadOriginal}
        size="sm"
        variant="outline"
      >
        {label(file.file_type)}
      </Button>
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
          // Marks which entry is the file's own File Type and which is a
          // Conversion. "Original" names the type, not the Original File:
          // both entries download a Generated File.
          const kind = fileType === file.file_type ? "original" : "converted";
          return (
            <DropdownMenuItem
              aria-label={`Download ${title} as ${label} (${kind})`}
              key={fileType}
              onSelect={() => onDownloadAs(fileType)}
            >
              {label}
              <span className="ml-auto pl-4 text-xs text-muted-foreground">
                {kind}
              </span>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );

  // A split button: the main part names the File Type it delivers, which
  // varies by file and by Delivery Rules, and the arrow offers the rest.
  return (
    <div className="flex items-center">
      <Button
        aria-label={`Download ${fileTypeLabel(deliveredFileType ?? file.file_type)}`}
        className="gap-1.5 rounded-r-none"
        disabled={deliveredFileType === undefined}
        onClick={onDownload}
        size="sm"
        variant="outline"
      >
        {label(deliveredFileType ?? file.file_type)}
      </Button>
      {menu(
        <Button
          aria-label="Download as"
          className="rounded-l-none border-l-0 px-1.5"
          size="sm"
          variant="outline"
        >
          <ChevronDown className="h-3.5 w-3.5" />
        </Button>,
        "Download as",
      )}
    </div>
  );
};

export default FileDownloadControl;
