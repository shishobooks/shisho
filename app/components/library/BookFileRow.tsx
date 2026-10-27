import {
  ArrowRightLeft,
  BookOpen,
  Headphones,
  Info,
  MoreHorizontal,
  Pencil,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import {
  fileEditionRows,
  fileIdentifierRows,
} from "@/components/files/fileFacts";
import CoverImage from "@/components/library/CoverImage";
import DetailsDialog, {
  type DetailRow,
} from "@/components/library/DetailsDialog";
import FileCoverThumbnail from "@/components/library/FileCoverThumbnail";
import FileDownloadControl from "@/components/library/FileDownloadControl";
import FileScanErrorBadge from "@/components/library/FileScanErrorBadge";
import TextLink from "@/components/library/TextLink";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useCan } from "@/hooks/useCan";
import { useFileImage } from "@/hooks/useFileImage";
import { cn } from "@/libraries/utils";
import {
  FileTypeM4B,
  type DeliveredFileType,
  type File,
  type PluginIdentifierType,
} from "@/types";
import {
  fileLabel,
  fileTypeLabel,
  formatBitrate,
  formatDateTime,
  formatDuration,
  formatFileSize,
  formatPageCount,
  formatYear,
  getFilename,
} from "@/utils/format";
import { getReadingAction } from "@/utils/readingAction";

/**
 * Everything the File details dialog lists. A Share Link recipient sees the
 * reader-facing rows only: no paths, URL, identifiers, or the server's dates.
 */
const fileDetailRows = (
  file: File,
  {
    isShareLink,
    linkPublisher,
    pluginIdentifierTypes,
  }: {
    isShareLink: boolean;
    linkPublisher: boolean;
    pluginIdentifierTypes: PluginIdentifierType[] | undefined;
  },
): DetailRow[] => {
  const rows: DetailRow[] = [
    { label: "Format", value: fileTypeLabel(file.file_type) },
    { label: "Size", value: formatFileSize(file.filesize_bytes) },
  ];
  if (!isShareLink && file.filepath) {
    rows.push({ label: "File", value: getFilename(file.filepath) });
    rows.push({ label: "Path", value: file.filepath });
  }
  if (file.page_count) {
    rows.push({ label: "Pages", value: formatPageCount(file.page_count) });
  }
  if (file.audiobook_duration_seconds) {
    rows.push({
      label: "Duration",
      value: formatDuration(file.audiobook_duration_seconds),
    });
  }
  const audio = [
    file.audiobook_codec,
    file.audiobook_bitrate_bps && formatBitrate(file.audiobook_bitrate_bps),
  ].filter(Boolean);
  if (audio.length > 0) rows.push({ label: "Audio", value: audio.join(" · ") });
  if (file.chapters?.length) {
    rows.push({ label: "Chapters", value: file.chapters.length });
  }
  rows.push(...fileEditionRows(file, { linkPublisher }));
  // A recipient has no use for catalog details, often internal ones such
  // as a UUID.
  if (!isShareLink) {
    rows.push(...fileIdentifierRows(file, pluginIdentifierTypes));
    if (file.file_modified_at) {
      rows.push({
        label: "Modified",
        value: formatDateTime(file.file_modified_at),
      });
    }
    rows.push({ label: "Added", value: formatDateTime(file.created_at) });
  }
  return rows;
};

/**
 * The facts a row shows under the name, quietly: length, an audiobook's
 * bitrate (what tells two recordings apart), year, and size.
 */
const rowFacts = (file: File): string[] =>
  [
    file.audiobook_duration_seconds
      ? formatDuration(file.audiobook_duration_seconds)
      : null,
    file.page_count ? formatPageCount(file.page_count) : null,
    file.audiobook_bitrate_bps
      ? formatBitrate(file.audiobook_bitrate_bps)
      : null,
    file.release_date ? formatYear(file.release_date) : null,
    formatFileSize(file.filesize_bytes),
  ].filter((fact): fact is string => fact !== null);

export interface BookFileRowProps {
  file: File;
  libraryId: number;
  bookTitle: string;
  /**
   * Share Link context: plain-text names, no Read or Listen, and a file menu
   * with only the file's details.
   */
  isShareLink: boolean;
  /** Gates the file's edit, rescan, move, and delete actions. */
  canWriteBooks: boolean;
  pluginIdentifierTypes: PluginIdentifierType[] | undefined;
  getCoverUrl?: (file: File) => string | null;
  deliveredFileType: DeliveredFileType | undefined;
  isDownloading: boolean;
  onDownload: () => void;
  onDownloadAs: (fileType: DeliveredFileType) => void;
  onDownloadOriginal: () => void;
  onCancelDownload: () => void;
  onEdit: () => void;
  onRescan: () => void;
  isResyncing: boolean;
  isSupplement?: boolean;
  onMoveFile?: () => void;
  onDeleteFile: () => void;
  isDeletingFile: boolean;
}

/**
 * One of a book's files: its cover (a click shows it large), full name,
 * format, a quiet line of facts, its narrators, and its actions. Read or
 * Listen sits after the variable-width download button so it lines up with
 * the rows above and below it.
 */
const BookFileRow = ({
  file,
  libraryId,
  bookTitle,
  isShareLink,
  canWriteBooks,
  pluginIdentifierTypes,
  getCoverUrl,
  deliveredFileType,
  isDownloading,
  onDownload,
  onDownloadAs,
  onDownloadOriginal,
  onCancelDownload,
  onEdit,
  onRescan,
  isResyncing,
  isSupplement = false,
  onMoveFile,
  onDeleteFile,
  isDeletingFile,
}: BookFileRowProps) => {
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [coverOpen, setCoverOpen] = useState(false);
  // People pages need People Read; without it the names are plain text
  // instead of links to an error page. Publisher pages need only the Books
  // Read this page already has.
  const linkPeople = useCan("people:read") && !isShareLink;
  const linkPublisher = !isShareLink;
  // fileLabel falls back to the type when the Share Link payload has no name.
  const name = fileLabel(file);
  const readingAction = isShareLink ? null : getReadingAction(file.file_type);
  const isListen = readingAction === "listen";
  const image = useFileImage(file, getCoverUrl);
  const coverUrl = image?.url ?? null;
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-md border border-border p-3 sm:flex-nowrap">
      {/* A fixed slot, so names line up whatever each cover's shape. */}
      <div className="flex w-14 shrink-0 justify-center self-start">
        {coverUrl ? (
          <Button
            aria-label={
              image?.isPage
                ? `View first page of ${name}`
                : `View cover of ${name}`
            }
            className={cn(
              "flex rounded",
              file.file_type === FileTypeM4B ? "w-14" : "w-[38px]",
            )}
            onClick={() => setCoverOpen(true)}
            variant="unstyled"
          >
            <FileCoverThumbnail
              className="w-full"
              file={file}
              getCoverUrl={getCoverUrl}
              interactive={false}
            />
          </Button>
        ) : (
          <FileCoverThumbnail
            className={file.file_type === FileTypeM4B ? "w-14" : "w-[38px]"}
            file={file}
            getCoverUrl={getCoverUrl}
            interactive={false}
          />
        )}
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2">
          {isShareLink ? (
            <span className="min-w-0 break-words font-semibold leading-snug">
              {name}
            </span>
          ) : (
            <Link
              className="min-w-0 break-words font-semibold leading-snug hover:underline"
              to={`/libraries/${libraryId}/books/${file.book_id}/files/${file.id}`}
            >
              {name}
            </Link>
          )}
          <FileScanErrorBadge className="shrink-0 text-xs" file={file} />
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <Badge className="px-1.5 py-0 text-[10px]" variant="outline">
            {isSupplement ? "Supplement" : fileTypeLabel(file.file_type)}
          </Badge>
          <span>{rowFacts(file).join(" · ")}</span>
        </div>
        {file.narrators && file.narrators.length > 0 && (
          <p className="mt-0.5 text-xs text-muted-foreground">
            Narrated by{" "}
            {file.narrators.map((narrator, index) => (
              <span key={narrator.id}>
                <TextLink
                  className="text-foreground"
                  to={
                    linkPeople
                      ? `/libraries/${libraryId}/people/${narrator.person_id}`
                      : null
                  }
                >
                  {narrator.person?.name ?? "Unknown"}
                </TextLink>
                {index < file.narrators!.length - 1 && ", "}
              </span>
            ))}
          </p>
        )}
      </div>

      {/* On a phone the actions take their own line under the name, so the
          name keeps the row's width. They sit at the right there too, so the
          icons after the variable-width download button line up row to row. */}
      <div className="flex shrink-0 items-center gap-1.5 max-sm:basis-full max-sm:justify-end">
        <FileDownloadControl
          bookTitle={bookTitle}
          deliveredFileType={deliveredFileType}
          file={file}
          isDownloading={isDownloading}
          isSupplement={isSupplement}
          onCancelDownload={onCancelDownload}
          onDownload={onDownload}
          onDownloadAs={onDownloadAs}
          onDownloadOriginal={onDownloadOriginal}
        />
        {readingAction && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button asChild size="icon-sm" variant="ghost">
                <Link
                  aria-label={isListen ? "Listen" : "Read"}
                  to={`/libraries/${libraryId}/books/${file.book_id}/files/${file.id}/read`}
                >
                  {isListen ? (
                    <Headphones className="h-4 w-4" />
                  ) : (
                    <BookOpen className="h-4 w-4" />
                  )}
                </Link>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{isListen ? "Listen" : "Read"}</TooltipContent>
          </Tooltip>
        )}
        {/* Out of the menu, like the book's own Edit, so editing a file is
            never a hunt. */}
        {canWriteBooks && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                aria-label="Edit file"
                onClick={onEdit}
                size="icon-sm"
                variant="ghost"
              >
                <Pencil className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Edit file</TooltipContent>
          </Tooltip>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              aria-label="File actions"
              disabled={isResyncing}
              size="icon-sm"
              variant="ghost"
            >
              <MoreHorizontal className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="end"
            onCloseAutoFocus={(e) => e.preventDefault()}
          >
            <DropdownMenuItem onSelect={() => setDetailsOpen(true)}>
              <Info className="h-4 w-4 mr-2" />
              File details
            </DropdownMenuItem>
            {canWriteBooks && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem disabled={isResyncing} onSelect={onRescan}>
                  <RefreshCw className="h-4 w-4 mr-2" />
                  Rescan file
                </DropdownMenuItem>
                {onMoveFile && (
                  <DropdownMenuItem onSelect={onMoveFile}>
                    <ArrowRightLeft className="h-4 w-4 mr-2" />
                    Move to another book
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  className="text-destructive focus:text-destructive"
                  disabled={isDeletingFile}
                  onSelect={onDeleteFile}
                >
                  <Trash2 className="h-4 w-4 mr-2 text-destructive" />
                  Delete file
                </DropdownMenuItem>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <DetailsDialog
        onOpenChange={setDetailsOpen}
        open={detailsOpen}
        rows={fileDetailRows(file, {
          isShareLink,
          linkPublisher,
          pluginIdentifierTypes,
        })}
        title={name}
      />
      {coverUrl && (
        <Dialog onOpenChange={setCoverOpen} open={coverOpen}>
          <DialogContent className="max-w-md">
            <DialogHeader>
              <DialogTitle className="break-words">{name}</DialogTitle>
            </DialogHeader>
            <DialogBody>
              <div
                className={cn(
                  "mx-auto w-full overflow-hidden rounded-md border border-border bg-muted",
                  file.file_type === FileTypeM4B
                    ? "aspect-square"
                    : "aspect-[2/3] max-w-xs",
                )}
              >
                <CoverImage
                  alt={image?.isPage ? `${name} first page` : `${name} cover`}
                  className={cn(
                    "size-full",
                    image?.isPage ? "bg-white object-contain" : "object-cover",
                  )}
                  key={coverUrl}
                  src={coverUrl}
                  thumbnailUrl={image?.thumbnailUrl}
                />
              </div>
            </DialogBody>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
};

export default BookFileRow;
