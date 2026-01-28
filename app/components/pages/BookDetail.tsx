import {
  ArrowLeft,
  ArrowRightLeft,
  BookOpen,
  Check,
  ChevronDown,
  ChevronRight,
  Download,
  Edit,
  GitMerge,
  List,
  Loader2,
  MoreVertical,
  RefreshCw,
  X,
} from "lucide-react";
import React, { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import AddToListPopover from "@/components/library/AddToListPopover";
import { BookEditDialog } from "@/components/library/BookEditDialog";
import CoverGalleryTabs from "@/components/library/CoverGalleryTabs";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import DownloadFormatPopover from "@/components/library/DownloadFormatPopover";
import FileCoverThumbnail from "@/components/library/FileCoverThumbnail";
import { FileEditDialog } from "@/components/library/FileEditDialog";
import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MergeIntoDialog } from "@/components/library/MergeIntoDialog";
import { MoveFilesDialog } from "@/components/library/MoveFilesDialog";
import { ResyncConfirmDialog } from "@/components/library/ResyncConfirmDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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
import { Separator } from "@/components/ui/separator";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useBook, useResyncBook, useResyncFile } from "@/hooks/queries/books";
import { useLibrary } from "@/hooks/queries/libraries";
import { usePluginIdentifierTypes } from "@/hooks/queries/plugins";
import { usePageTitle } from "@/hooks/usePageTitle";
import { cn } from "@/libraries/utils";
import {
  DownloadFormatAsk,
  DownloadFormatKepub,
  FileTypeCBZ,
  FileTypeEPUB,
  type File,
} from "@/types";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";
import {
  formatDate,
  formatDuration,
  formatFileSize,
  formatIdentifierType,
  getFilename,
} from "@/utils/format";
import { getIdentifierUrl } from "@/utils/identifiers";

// Determines which file type would provide the cover based on library preference.
// This mirrors the backend's selectCoverFile priority logic but doesn't require cover_image_filename.
// Used for placeholder variant selection when there's no cover image.
const getCoverFileType = (
  files: File[] | undefined,
  coverAspectRatio: string,
): "book" | "audiobook" => {
  if (!files || files.length === 0) return "book";

  const hasBookFiles = files.some(
    (f) => f.file_type === "epub" || f.file_type === "cbz",
  );
  const hasAudiobookFiles = files.some((f) => f.file_type === "m4b");

  switch (coverAspectRatio) {
    case "audiobook":
    case "audiobook_fallback_book":
      if (hasAudiobookFiles) return "audiobook";
      if (hasBookFiles) return "book";
      break;
    default: // "book", "book_fallback_audiobook"
      if (hasBookFiles) return "book";
      if (hasAudiobookFiles) return "audiobook";
  }
  return "book";
};

const getRoleLabel = (role: string | undefined): string | null => {
  if (!role) return null;
  const roleLabels: Record<string, string> = {
    writer: "Writer",
    penciller: "Penciller",
    inker: "Inker",
    colorist: "Colorist",
    letterer: "Letterer",
    cover_artist: "Cover Artist",
    editor: "Editor",
    translator: "Translator",
  };
  return roleLabels[role] || role;
};

interface DownloadError {
  fileId: number;
  message: string;
}

// KePub format is only supported for EPUB and CBZ files
const supportsKepub = (fileType: string): boolean => {
  return fileType === FileTypeEPUB || fileType === FileTypeCBZ;
};

interface FileRowProps {
  file: File;
  libraryId: string;
  libraryDownloadPreference: string | undefined;
  isExpanded: boolean;
  hasExpandableMetadata: boolean;
  onToggleExpand: () => void;
  isDownloading: boolean;
  onDownload: () => void;
  onDownloadKepub: () => void;
  onDownloadOriginal: () => void;
  onDownloadWithEndpoint: (endpoint: string) => void;
  onCancelDownload: () => void;
  onEdit: () => void;
  onScanMetadata: () => void;
  onRefreshMetadata: () => void;
  isResyncing: boolean;
  isSupplement?: boolean;
  isSelectMode?: boolean;
  isFileSelected?: boolean;
  onToggleSelect?: () => void;
  onMoveFile?: () => void;
  cacheBuster?: number;
}

const FileRow = ({
  file,
  libraryId,
  libraryDownloadPreference,
  isExpanded,
  hasExpandableMetadata,
  onToggleExpand,
  isDownloading,
  onDownload,
  onDownloadKepub,
  onDownloadOriginal,
  onDownloadWithEndpoint,
  onCancelDownload,
  onEdit,
  onScanMetadata,
  onRefreshMetadata,
  isResyncing,
  isSupplement = false,
  isSelectMode = false,
  isFileSelected = false,
  onToggleSelect,
  onMoveFile,
  cacheBuster,
}: FileRowProps) => {
  const showChevron = hasExpandableMetadata && !isSupplement;
  const [showRefreshDialog, setShowRefreshDialog] = useState(false);
  const { data: pluginIdentifierTypes } = usePluginIdentifierTypes();

  return (
    <div className="py-3 flex gap-3">
      {/* Selection checkbox */}
      {isSelectMode && (
        <button
          className={cn(
            "shrink-0 h-5 w-5 rounded border flex items-center justify-center self-start mt-1 cursor-pointer",
            isFileSelected
              ? "bg-primary border-primary"
              : "border-muted-foreground/50 hover:border-primary/50",
          )}
          onClick={(e) => {
            e.stopPropagation();
            onToggleSelect?.();
          }}
          type="button"
        >
          {isFileSelected && <Check className="h-3 w-3 text-white" />}
        </button>
      )}

      {/* Chevron indicator - aligned to top */}
      {showChevron ? (
        <button
          aria-expanded={isExpanded}
          className="p-0.5 rounded hover:bg-muted/50 shrink-0 cursor-pointer self-start mt-1"
          onClick={onToggleExpand}
          type="button"
        >
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          )}
        </button>
      ) : (
        <div className="w-5 shrink-0" /> // Spacer for alignment when no chevron
      )}

      {/* File cover thumbnail - constrained height, natural aspect ratio */}
      {!isSupplement && (
        <div className="shrink-0 self-start">
          <FileCoverThumbnail
            cacheBuster={cacheBuster}
            className="h-14"
            file={file}
          />
        </div>
      )}

      {/* Content area */}
      <div className="flex-1 min-w-0 space-y-1">
        {/* Primary row: badge, name, stats, actions */}
        <div className="flex items-center gap-2">
          {/* File type badge */}
          <Badge
            className="uppercase text-xs shrink-0"
            variant={isSupplement ? "outline" : "secondary"}
          >
            {file.file_type}
          </Badge>

          {/* Name */}
          <Link
            className={`truncate hover:underline min-w-0 flex-1 ${isSupplement ? "text-sm" : "text-sm font-medium"}`}
            title={file.name || getFilename(file.filepath)}
            to={`/libraries/${libraryId}/books/${file.book_id}/files/${file.id}`}
          >
            {file.name || getFilename(file.filepath)}
          </Link>

          {/* Stats and actions - desktop only (inline) */}
          <div className="hidden md:flex items-center gap-3 text-xs text-muted-foreground shrink-0">
            {/* M4B stats */}
            {file.audiobook_duration_seconds && (
              <>
                <span>{formatDuration(file.audiobook_duration_seconds)}</span>
                <span className="text-muted-foreground/50">·</span>
              </>
            )}
            {file.audiobook_bitrate_bps && (
              <>
                <span>
                  {Math.round(file.audiobook_bitrate_bps / 1000)} kbps
                </span>
                <span className="text-muted-foreground/50">·</span>
              </>
            )}
            {file.audiobook_codec && (
              <>
                <span>{file.audiobook_codec}</span>
                <span className="text-muted-foreground/50">·</span>
              </>
            )}
            {/* CBZ stats */}
            {file.page_count && (
              <>
                <span>{file.page_count} pages</span>
                <span className="text-muted-foreground/50">·</span>
              </>
            )}
            {/* File size - always shown */}
            <span>{formatFileSize(file.filesize_bytes)}</span>

            {/* Download button/popover */}
            {isSupplement ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    onClick={onDownloadOriginal}
                    size="sm"
                    variant="ghost"
                  >
                    <Download className="h-3 w-3" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Download</TooltipContent>
              </Tooltip>
            ) : libraryDownloadPreference === DownloadFormatAsk &&
              supportsKepub(file.file_type) ? (
              <DownloadFormatPopover
                disabled={isDownloading}
                isLoading={isDownloading}
                onCancel={onCancelDownload}
                onDownloadKepub={onDownloadKepub}
                onDownloadOriginal={() =>
                  onDownloadWithEndpoint(`/api/books/files/${file.id}/download`)
                }
              />
            ) : isDownloading ? (
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
            ) : (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button onClick={onDownload} size="sm" variant="ghost">
                    <Download className="h-3 w-3" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Download</TooltipContent>
              </Tooltip>
            )}

            {/* Read button - CBZ only */}
            {file.file_type === FileTypeCBZ && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Link
                    to={`/libraries/${libraryId}/books/${file.book_id}/files/${file.id}/read`}
                  >
                    <Button size="sm" variant="ghost">
                      <BookOpen className="h-3 w-3" />
                    </Button>
                  </Link>
                </TooltipTrigger>
                <TooltipContent>Read</TooltipContent>
              </Tooltip>
            )}

            {/* Actions dropdown */}
            <DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <DropdownMenuTrigger asChild>
                    <Button disabled={isResyncing} size="sm" variant="ghost">
                      <MoreVertical className="h-3 w-3" />
                    </Button>
                  </DropdownMenuTrigger>
                </TooltipTrigger>
                <TooltipContent>More actions</TooltipContent>
              </Tooltip>
              <DropdownMenuContent
                align="end"
                onCloseAutoFocus={(e) => e.preventDefault()}
              >
                <DropdownMenuItem onClick={onEdit}>
                  <Edit className="h-4 w-4 mr-2" />
                  Edit
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  disabled={isResyncing}
                  onClick={onScanMetadata}
                >
                  <RefreshCw className="h-4 w-4 mr-2" />
                  Scan for new metadata
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => setShowRefreshDialog(true)}>
                  <RefreshCw className="h-4 w-4 mr-2" />
                  Refresh all metadata
                </DropdownMenuItem>
                {onMoveFile && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onClick={onMoveFile}>
                      <ArrowRightLeft className="h-4 w-4 mr-2" />
                      Move to another book
                    </DropdownMenuItem>
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>

        {/* Stats and actions - mobile only (separate row) */}
        <div className="flex md:hidden items-center gap-2 text-xs text-muted-foreground">
          {/* M4B stats */}
          {file.audiobook_duration_seconds && (
            <>
              <span>{formatDuration(file.audiobook_duration_seconds)}</span>
              <span className="text-muted-foreground/50">·</span>
            </>
          )}
          {file.audiobook_bitrate_bps && (
            <>
              <span>{Math.round(file.audiobook_bitrate_bps / 1000)} kbps</span>
              <span className="text-muted-foreground/50">·</span>
            </>
          )}
          {file.audiobook_codec && (
            <>
              <span>{file.audiobook_codec}</span>
              <span className="text-muted-foreground/50">·</span>
            </>
          )}
          {/* CBZ stats */}
          {file.page_count && (
            <>
              <span>{file.page_count} pages</span>
              <span className="text-muted-foreground/50">·</span>
            </>
          )}
          {/* File size - always shown */}
          <span>{formatFileSize(file.filesize_bytes)}</span>

          {/* Download button/popover */}
          {isSupplement ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button onClick={onDownloadOriginal} size="sm" variant="ghost">
                  <Download className="h-3 w-3" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Download</TooltipContent>
            </Tooltip>
          ) : libraryDownloadPreference === DownloadFormatAsk &&
            supportsKepub(file.file_type) ? (
            <DownloadFormatPopover
              disabled={isDownloading}
              isLoading={isDownloading}
              onCancel={onCancelDownload}
              onDownloadKepub={onDownloadKepub}
              onDownloadOriginal={() =>
                onDownloadWithEndpoint(`/api/books/files/${file.id}/download`)
              }
            />
          ) : isDownloading ? (
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
          ) : (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button onClick={onDownload} size="sm" variant="ghost">
                  <Download className="h-3 w-3" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Download</TooltipContent>
            </Tooltip>
          )}

          {/* Read button - CBZ only */}
          {file.file_type === FileTypeCBZ && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Link
                  to={`/libraries/${libraryId}/books/${file.book_id}/files/${file.id}/read`}
                >
                  <Button size="sm" variant="ghost">
                    <BookOpen className="h-3 w-3" />
                  </Button>
                </Link>
              </TooltipTrigger>
              <TooltipContent>Read</TooltipContent>
            </Tooltip>
          )}

          {/* Actions dropdown */}
          <DropdownMenu>
            <Tooltip>
              <TooltipTrigger asChild>
                <DropdownMenuTrigger asChild>
                  <Button disabled={isResyncing} size="sm" variant="ghost">
                    <MoreVertical className="h-3 w-3" />
                  </Button>
                </DropdownMenuTrigger>
              </TooltipTrigger>
              <TooltipContent>More actions</TooltipContent>
            </Tooltip>
            <DropdownMenuContent
              align="end"
              onCloseAutoFocus={(e) => e.preventDefault()}
            >
              <DropdownMenuItem onClick={onEdit}>
                <Edit className="h-4 w-4 mr-2" />
                Edit
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem disabled={isResyncing} onClick={onScanMetadata}>
                <RefreshCw className="h-4 w-4 mr-2" />
                Scan for new metadata
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setShowRefreshDialog(true)}>
                <RefreshCw className="h-4 w-4 mr-2" />
                Refresh all metadata
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        {/* Filename row - only show when name differs from filename */}
        {file.name && (
          <div>
            <span
              className="text-xs text-muted-foreground truncate block"
              title={file.filepath}
            >
              {getFilename(file.filepath)}
            </span>
          </div>
        )}

        {/* Narrators row - M4B only, always visible when present */}
        {file.narrators && file.narrators.length > 0 && (
          <div className="flex items-center gap-1 flex-wrap">
            <span className="text-xs text-muted-foreground">Narrated by</span>
            {file.narrators.map((narrator, index) => (
              <span className="text-xs" key={narrator.id}>
                <Link
                  className="hover:underline"
                  to={`/libraries/${libraryId}/people/${narrator.person_id}`}
                >
                  {narrator.person?.name ?? "Unknown"}
                </Link>
                {index < file.narrators!.length - 1 ? "," : ""}
              </span>
            ))}
          </div>
        )}

        {/* Expandable details section */}
        {isExpanded && hasExpandableMetadata && (
          <div className="mt-2 bg-muted/50 rounded-md p-3 text-xs space-y-2">
            {/* Publisher, Imprint, Released, URL */}
            <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
              {file.publisher && (
                <>
                  <span className="text-muted-foreground">Publisher</span>
                  <span>{file.publisher.name}</span>
                </>
              )}
              {file.imprint && (
                <>
                  <span className="text-muted-foreground">Imprint</span>
                  <span>{file.imprint.name}</span>
                </>
              )}
              {file.release_date && (
                <>
                  <span className="text-muted-foreground">Released</span>
                  <span>{formatDate(file.release_date)}</span>
                </>
              )}
              {file.url && (
                <>
                  <span className="text-muted-foreground">URL</span>
                  <a
                    className="text-primary hover:underline truncate"
                    href={file.url}
                    rel="noopener noreferrer"
                    target="_blank"
                    title={file.url}
                  >
                    {file.url.length > 60
                      ? file.url.substring(0, 60) + "..."
                      : file.url}
                  </a>
                </>
              )}
            </div>

            {/* Identifiers */}
            {file.identifiers && file.identifiers.length > 0 && (
              <div className="pt-2 border-t border-border/50">
                <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
                  {file.identifiers.map((id, idx) => {
                    const url = getIdentifierUrl(
                      id.type,
                      id.value,
                      pluginIdentifierTypes,
                    );
                    return (
                      <React.Fragment key={idx}>
                        <span className="text-muted-foreground">
                          {formatIdentifierType(id.type, pluginIdentifierTypes)}
                        </span>
                        {url ? (
                          <a
                            className="font-mono select-all text-primary hover:underline"
                            href={url}
                            rel="noopener noreferrer"
                            target="_blank"
                          >
                            {id.value}
                          </a>
                        ) : (
                          <span className="font-mono select-all">
                            {id.value}
                          </span>
                        )}
                      </React.Fragment>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      <ResyncConfirmDialog
        entityName={file.name || getFilename(file.filepath)}
        entityType="file"
        isPending={isResyncing}
        onConfirm={onRefreshMetadata}
        onOpenChange={setShowRefreshDialog}
        open={showRefreshDialog}
      />
    </div>
  );
};

const BookDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const bookQuery = useBook(id);
  const libraryQuery = useLibrary(libraryId);

  usePageTitle(bookQuery.data?.title ?? "Book Details");
  const resyncFileMutation = useResyncFile();
  const resyncBookMutation = useResyncBook();
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [showBookRefreshDialog, setShowBookRefreshDialog] = useState(false);
  const [showMergeIntoDialog, setShowMergeIntoDialog] = useState(false);
  const [editingFile, setEditingFile] = useState<File | null>(null);
  const [downloadError, setDownloadError] = useState<DownloadError | null>(
    null,
  );
  const [downloadingFileId, setDownloadingFileId] = useState<number | null>(
    null,
  );
  const [resyncingFileId, setResyncingFileId] = useState<number | null>(null);
  const [coverLoaded, setCoverLoaded] = useState(false);
  const [coverError, setCoverError] = useState(false);
  const [expandedFileIds, setExpandedFileIds] = useState<Set<number>>(
    new Set(),
  );
  const downloadAbortController = useRef<AbortController | null>(null);

  // File selection state for split/move
  const [isFileSelectMode, setIsFileSelectMode] = useState(false);
  const [selectedFileIds, setSelectedFileIds] = useState<Set<number>>(
    new Set(),
  );
  const [showMoveFilesDialog, setShowMoveFilesDialog] = useState(false);
  const [singleFileMoveId, setSingleFileMoveId] = useState<number | null>(null);

  const toggleFileSelection = (fileId: number) => {
    setSelectedFileIds((prev) => {
      const next = new Set(prev);
      if (next.has(fileId)) {
        next.delete(fileId);
      } else {
        next.add(fileId);
      }
      return next;
    });
  };

  const exitFileSelectMode = () => {
    setIsFileSelectMode(false);
    setSelectedFileIds(new Set());
  };

  const toggleFileExpanded = (fileId: number) => {
    setExpandedFileIds((prev) => {
      const next = new Set(prev);
      if (next.has(fileId)) {
        next.delete(fileId);
      } else {
        next.add(fileId);
      }
      return next;
    });
  };

  const hasExpandableMetadata = (file: File): boolean => {
    return !!(
      file.publisher ||
      file.imprint ||
      file.release_date ||
      file.url ||
      (file.identifiers && file.identifiers.length > 0)
    );
  };

  useEffect(() => {
    setCoverError(false);
  }, [bookQuery.data?.id]);

  // Cache-busting parameter for cover images - computed early for hook ordering
  const coverCacheBuster = bookQuery.dataUpdatedAt;
  const coverUrl = bookQuery.data?.id
    ? `/api/books/${bookQuery.data.id}/cover?t=${coverCacheBuster}`
    : null;

  // Check cache synchronously before paint to avoid placeholder flash
  // Must be called before early returns to maintain hook ordering
  useLayoutEffect(() => {
    if (coverUrl && isCoverLoaded(coverUrl)) {
      setCoverLoaded(true);
    }
  }, [coverUrl]);

  const handleDownloadWithEndpoint = async (
    fileId: number,
    endpoint: string,
  ) => {
    setDownloadError(null);
    setDownloadingFileId(fileId);

    // Create abort controller for this download
    const abortController = new AbortController();
    downloadAbortController.current = abortController;

    try {
      // Use HEAD request to trigger generation and check for errors
      // This avoids loading the entire file into browser memory
      const headResponse = await fetch(endpoint, {
        method: "HEAD",
        signal: abortController.signal,
      });

      if (!headResponse.ok) {
        // HEAD failed - make a GET request to get the error message (small JSON response)
        const errorResponse = await fetch(endpoint, {
          signal: abortController.signal,
        });
        const contentType = errorResponse.headers.get("content-type");
        if (contentType && contentType.includes("application/json")) {
          const error = await errorResponse.json();
          setDownloadError({
            fileId,
            message: error.message || "Failed to generate file",
          });
        } else {
          setDownloadError({
            fileId,
            message: "Failed to download file",
          });
        }
        return;
      }

      // HEAD succeeded - file is ready, trigger streaming download
      window.location.href = endpoint;
      toast.success("Download started");
    } catch (error) {
      // Don't show error dialog for user-initiated cancellation
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      console.error("Download error:", error);
      toast.error("Failed to download file");
    } finally {
      downloadAbortController.current = null;
      setDownloadingFileId(null);
    }
  };

  const handleDownload = async (fileId: number, fileType: string) => {
    const preference = libraryQuery.data?.download_format_preference;

    // For kepub preference with supported files, use kepub endpoint
    if (preference === DownloadFormatKepub && supportsKepub(fileType)) {
      await handleDownloadWithEndpoint(
        fileId,
        `/api/books/files/${fileId}/download/kepub`,
      );
    } else {
      // Original format for unsupported files or "original" preference
      await handleDownloadWithEndpoint(
        fileId,
        `/api/books/files/${fileId}/download`,
      );
    }
  };

  const handleDownloadKepub = async (fileId: number) => {
    await handleDownloadWithEndpoint(
      fileId,
      `/api/books/files/${fileId}/download/kepub`,
    );
  };

  const handleDownloadOriginal = (fileId: number) => {
    // Direct download of original file - this won't show any error since it's a simple file serve
    window.location.href = `/api/books/files/${fileId}/download/original`;
    setDownloadError(null);
  };

  const handleCancelDownload = () => {
    downloadAbortController.current?.abort();
    downloadAbortController.current = null;
    setDownloadingFileId(null);
  };

  const handleScanFileMetadata = async (fileId: number) => {
    setResyncingFileId(fileId);
    try {
      const result = await resyncFileMutation.mutateAsync({
        fileId,
        payload: { refresh: false },
      });
      if ("file_deleted" in result && result.file_deleted) {
        toast.success("File removed (no longer exists on disk)");
      } else {
        toast.success("Metadata scanned");
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to scan metadata",
      );
    } finally {
      setResyncingFileId(null);
    }
  };

  const handleRefreshFileMetadata = async (fileId: number) => {
    setResyncingFileId(fileId);
    try {
      const result = await resyncFileMutation.mutateAsync({
        fileId,
        payload: { refresh: true },
      });
      if ("file_deleted" in result && result.file_deleted) {
        toast.success("File removed (no longer exists on disk)");
      } else {
        toast.success("Metadata refreshed");
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to refresh metadata",
      );
    } finally {
      setResyncingFileId(null);
    }
  };

  const handleScanBookMetadata = async () => {
    if (!id) return;
    try {
      const result = await resyncBookMutation.mutateAsync({
        bookId: parseInt(id),
        payload: { refresh: false },
      });
      if ("book_deleted" in result && result.book_deleted) {
        toast.success("Book removed (no files remain)");
      } else {
        toast.success("Book metadata scanned");
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to scan book metadata",
      );
    }
  };

  const handleRefreshBookMetadata = async () => {
    if (!id) return;
    try {
      const result = await resyncBookMutation.mutateAsync({
        bookId: parseInt(id),
        payload: { refresh: true },
      });
      if ("book_deleted" in result && result.book_deleted) {
        toast.success("Book removed (no files remain)");
      } else {
        toast.success("Book metadata refreshed");
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to refresh book metadata",
      );
    }
  };

  if (bookQuery.isLoading) {
    return (
      <LibraryLayout>
        <LoadingSpinner />
      </LibraryLayout>
    );
  }

  if (!bookQuery.isSuccess || !bookQuery.data) {
    return (
      <LibraryLayout>
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-4">Book Not Found</h1>
          <p className="text-muted-foreground mb-6">
            The book you're looking for doesn't exist or may have been removed.
          </p>
          <Button asChild>
            <Link to={`/libraries/${libraryId}`}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Home
            </Link>
          </Button>
        </div>
      </LibraryLayout>
    );
  }

  const book = bookQuery.data;

  // Separate main files and supplements
  const mainFiles =
    book.files?.filter((f) => f.file_role !== "supplement") ?? [];
  const supplements =
    book.files?.filter((f) => f.file_role === "supplement") ?? [];

  // Determine which file type would provide the cover based on library's cover_aspect_ratio setting
  // This is used to determine the native aspect ratio (audiobook = square, book = 2:3)
  const libraryCoverAspectRatio =
    libraryQuery.data?.cover_aspect_ratio ?? "book";
  const coverFileType = getCoverFileType(book.files, libraryCoverAspectRatio);
  const isAudiobook = coverFileType === "audiobook";
  const coverAspectRatio = isAudiobook ? "aspect-square" : "aspect-[2/3]";

  const handleCoverLoad = () => {
    if (coverUrl) {
      markCoverLoaded(coverUrl);
    }
    setCoverLoaded(true);
  };

  return (
    <LibraryLayout>
      <div className="mb-6">
        <Button asChild variant="ghost">
          <Link to={`/libraries/${libraryId}`}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Books
          </Link>
        </Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 md:gap-8">
        {/* Book Cover */}
        <div className="lg:col-span-1">
          {mainFiles.length > 1 ? (
            /* Multiple files - show cover gallery with tabs */
            <CoverGalleryTabs
              cacheBuster={coverCacheBuster}
              files={mainFiles}
            />
          ) : (
            /* Single file - show book cover directly */
            <div
              className={`${coverAspectRatio} w-48 sm:w-64 lg:w-full mx-auto lg:mx-0 relative`}
            >
              {/* Placeholder shown until image loads or on error */}
              {(!coverLoaded || coverError) && (
                <CoverPlaceholder
                  className={`absolute inset-0 rounded-md border border-border`}
                  variant={coverFileType}
                />
              )}
              {/* Image hidden until loaded, removed on error */}
              {!coverError && (
                <img
                  alt={`${book.title} Cover`}
                  className={`w-full h-full object-cover rounded-md border border-border ${!coverLoaded ? "opacity-0" : ""}`}
                  onError={() => setCoverError(true)}
                  onLoad={handleCoverLoad}
                  src={coverUrl!}
                />
              )}
            </div>
          )}
        </div>

        {/* Book Details */}
        <div className="lg:col-span-2 space-y-4 md:space-y-6">
          <div>
            <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3 mb-2">
              <h1 className="text-2xl md:text-3xl font-semibold">
                {book.title}
              </h1>
              <div className="flex items-center gap-2 shrink-0">
                <AddToListPopover
                  bookId={book.id}
                  trigger={
                    <Button size="sm" title="Add to list" variant="outline">
                      <List className="h-4 w-4 sm:mr-2" />
                      <span className="hidden sm:inline">Add to list</span>
                    </Button>
                  }
                />
                <Button
                  onClick={() => setEditDialogOpen(true)}
                  size="sm"
                  variant="outline"
                >
                  <Edit className="h-4 w-4 sm:mr-2" />
                  <span className="hidden sm:inline">Edit</span>
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button size="sm" variant="outline">
                      <MoreVertical className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent
                    align="end"
                    onCloseAutoFocus={(e) => e.preventDefault()}
                  >
                    <DropdownMenuItem
                      disabled={resyncBookMutation.isPending}
                      onClick={handleScanBookMetadata}
                    >
                      <RefreshCw className="h-4 w-4 mr-2" />
                      Scan for new metadata
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() => setShowBookRefreshDialog(true)}
                    >
                      <RefreshCw className="h-4 w-4 mr-2" />
                      Refresh all metadata
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      onClick={() => setShowMergeIntoDialog(true)}
                    >
                      <GitMerge className="h-4 w-4 mr-2" />
                      Merge into another book
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
            {book.sort_title && book.sort_title !== book.title && (
              <p className="text-sm text-muted-foreground italic">
                Sort title: {book.sort_title}
              </p>
            )}
            {book.subtitle && (
              <p className="text-lg text-muted-foreground">{book.subtitle}</p>
            )}
            {book.description && (
              <p className="text-sm text-muted-foreground mt-3 whitespace-pre-wrap">
                {book.description}
              </p>
            )}
          </div>

          <div className="space-y-4 md:space-y-6">
            {/* Authors */}
            {book.authors &&
              book.authors.length > 0 &&
              (() => {
                const hasCBZFiles = book.files?.some(
                  (f) => f.file_type === FileTypeCBZ,
                );
                return (
                  <div>
                    <h3 className="font-semibold mb-2">Authors</h3>
                    <div className="flex flex-wrap gap-2">
                      {book.authors.map((author) => {
                        const roleLabel = getRoleLabel(author.role);
                        return (
                          <Link
                            key={author.id}
                            to={`/libraries/${libraryId}/people/${author.person_id}`}
                          >
                            <Badge
                              className="cursor-pointer hover:bg-secondary/80"
                              variant="secondary"
                            >
                              {author.person?.name ?? "Unknown"}
                              {hasCBZFiles && roleLabel && (
                                <span className="text-muted-foreground ml-1">
                                  ({roleLabel})
                                </span>
                              )}
                            </Badge>
                          </Link>
                        );
                      })}
                    </div>
                  </div>
                );
              })()}

            {/* Series */}
            {book.book_series && book.book_series.length > 0 && (
              <div>
                <h3 className="font-semibold mb-2">Series</h3>
                <div className="flex flex-wrap gap-3">
                  {book.book_series.map((bs) => (
                    <div className="flex items-center gap-2" key={bs.id}>
                      <Link
                        className="text-sm font-medium text-primary hover:text-primary/80 hover:underline dark:text-violet-300 dark:hover:text-violet-400"
                        to={`/libraries/${libraryId}/series/${bs.series_id}`}
                      >
                        {bs.series?.name ?? "Unknown Series"}
                      </Link>
                      {bs.series_number && (
                        <Badge className="text-xs" variant="outline">
                          #{bs.series_number}
                        </Badge>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Genres */}
            {book.book_genres && book.book_genres.length > 0 && (
              <div>
                <h3 className="font-semibold mb-2">Genres</h3>
                <div className="flex flex-wrap gap-2">
                  {book.book_genres.map((bg) => (
                    <Link
                      key={bg.id}
                      to={`/libraries/${libraryId}?genre_ids=${bg.genre_id}`}
                    >
                      <Badge
                        className="cursor-pointer hover:bg-secondary/80"
                        variant="secondary"
                      >
                        {bg.genre?.name ?? "Unknown"}
                      </Badge>
                    </Link>
                  ))}
                </div>
              </div>
            )}

            {/* Tags */}
            {book.book_tags && book.book_tags.length > 0 && (
              <div>
                <h3 className="font-semibold mb-2">Tags</h3>
                <div className="flex flex-wrap gap-2">
                  {book.book_tags.map((bt) => (
                    <Link
                      key={bt.id}
                      to={`/libraries/${libraryId}?tag_ids=${bt.tag_id}`}
                    >
                      <Badge
                        className="cursor-pointer hover:bg-secondary/80"
                        variant="secondary"
                      >
                        {bt.tag?.name ?? "Unknown"}
                      </Badge>
                    </Link>
                  ))}
                </div>
              </div>
            )}

            <Separator />

            {/* Metadata */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
              <div>
                <p className="font-semibold">Created</p>
                <p className="text-muted-foreground">
                  {formatDate(book.created_at)}
                </p>
              </div>
              <div>
                <p className="font-semibold">Updated</p>
                <p className="text-muted-foreground">
                  {formatDate(book.updated_at)}
                </p>
              </div>
              <div>
                <p className="font-semibold">Library</p>
                <p className="text-muted-foreground">
                  {book.library?.name || `Library ${book.library_id}`}
                </p>
              </div>
              <div>
                <p className="font-semibold">File Path</p>
                <p className="text-muted-foreground">
                  {book.filepath.split("/").map((segment, i, arr) => (
                    <React.Fragment key={i}>
                      {segment}
                      {i < arr.length - 1 && (
                        <>
                          /
                          <wbr />
                        </>
                      )}
                    </React.Fragment>
                  ))}
                </p>
              </div>
            </div>

            <Separator />

            {/* Files */}
            <div>
              <div className="flex items-center justify-between mb-3">
                <h3 className="font-semibold">Files ({mainFiles.length})</h3>
                {mainFiles.length > 1 && (
                  <Button
                    onClick={() => {
                      if (isFileSelectMode) {
                        exitFileSelectMode();
                      } else {
                        setIsFileSelectMode(true);
                      }
                    }}
                    size="sm"
                    variant="ghost"
                  >
                    {isFileSelectMode ? "Cancel" : "Select"}
                  </Button>
                )}
              </div>
              <div className="space-y-2">
                {mainFiles.map((file) => (
                  <FileRow
                    cacheBuster={coverCacheBuster}
                    file={file}
                    hasExpandableMetadata={hasExpandableMetadata(file)}
                    isDownloading={downloadingFileId === file.id}
                    isExpanded={expandedFileIds.has(file.id)}
                    isFileSelected={selectedFileIds.has(file.id)}
                    isResyncing={resyncingFileId === file.id}
                    isSelectMode={isFileSelectMode}
                    key={file.id}
                    libraryDownloadPreference={
                      libraryQuery.data?.download_format_preference
                    }
                    libraryId={libraryId!}
                    onCancelDownload={handleCancelDownload}
                    onDownload={() => handleDownload(file.id, file.file_type)}
                    onDownloadKepub={() => handleDownloadKepub(file.id)}
                    onDownloadOriginal={() => handleDownloadOriginal(file.id)}
                    onDownloadWithEndpoint={(endpoint) =>
                      handleDownloadWithEndpoint(file.id, endpoint)
                    }
                    onEdit={() => setEditingFile(file)}
                    onMoveFile={() => setSingleFileMoveId(file.id)}
                    onRefreshMetadata={() => handleRefreshFileMetadata(file.id)}
                    onScanMetadata={() => handleScanFileMetadata(file.id)}
                    onToggleExpand={() => toggleFileExpanded(file.id)}
                    onToggleSelect={() => toggleFileSelection(file.id)}
                  />
                ))}
              </div>
            </div>

            {/* Supplements */}
            {supplements.length > 0 && (
              <>
                <Separator />
                <div>
                  <h3 className="font-semibold mb-3">
                    Supplements ({supplements.length})
                  </h3>
                  <div className="space-y-2">
                    {supplements.map((file) => (
                      <FileRow
                        cacheBuster={coverCacheBuster}
                        file={file}
                        hasExpandableMetadata={hasExpandableMetadata(file)}
                        isDownloading={downloadingFileId === file.id}
                        isExpanded={expandedFileIds.has(file.id)}
                        isResyncing={resyncingFileId === file.id}
                        isSupplement
                        key={file.id}
                        libraryDownloadPreference={
                          libraryQuery.data?.download_format_preference
                        }
                        libraryId={libraryId!}
                        onCancelDownload={handleCancelDownload}
                        onDownload={() =>
                          handleDownload(file.id, file.file_type)
                        }
                        onDownloadKepub={() => handleDownloadKepub(file.id)}
                        onDownloadOriginal={() =>
                          handleDownloadOriginal(file.id)
                        }
                        onDownloadWithEndpoint={(endpoint) =>
                          handleDownloadWithEndpoint(file.id, endpoint)
                        }
                        onEdit={() => setEditingFile(file)}
                        onRefreshMetadata={() =>
                          handleRefreshFileMetadata(file.id)
                        }
                        onScanMetadata={() => handleScanFileMetadata(file.id)}
                        onToggleExpand={() => toggleFileExpanded(file.id)}
                      />
                    ))}
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      </div>

      <BookEditDialog
        book={book}
        onOpenChange={setEditDialogOpen}
        open={editDialogOpen}
      />

      <ResyncConfirmDialog
        entityName={book.title}
        entityType="book"
        isPending={resyncBookMutation.isPending}
        onConfirm={handleRefreshBookMetadata}
        onOpenChange={setShowBookRefreshDialog}
        open={showBookRefreshDialog}
      />

      {editingFile && (
        <FileEditDialog
          file={editingFile}
          onOpenChange={(open) => {
            if (!open) setEditingFile(null);
          }}
          open={!!editingFile}
        />
      )}

      {/* Download Error Dialog */}
      <Dialog
        onOpenChange={(open) => {
          if (!open) setDownloadError(null);
        }}
        open={!!downloadError}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Download Failed</DialogTitle>
            <DialogDescription>{downloadError?.message}</DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button onClick={() => setDownloadError(null)} variant="outline">
              Cancel
            </Button>
            {downloadError && (
              <Button
                onClick={() => handleDownloadOriginal(downloadError.fileId)}
              >
                Download Original
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {libraryQuery.data && (
        <MergeIntoDialog
          library={libraryQuery.data}
          onOpenChange={setShowMergeIntoDialog}
          onSuccess={(targetBook) => {
            navigate(`/libraries/${libraryId}/books/${targetBook.id}`);
          }}
          open={showMergeIntoDialog}
          sourceBook={book}
        />
      )}

      {/* File selection action bar */}
      {selectedFileIds.size > 0 && (
        <div className="fixed bottom-4 left-1/2 -translate-x-1/2 bg-background border rounded-lg shadow-lg p-3 flex items-center gap-3 z-50">
          <span className="text-sm text-muted-foreground">
            {selectedFileIds.size} file
            {selectedFileIds.size !== 1 ? "s" : ""} selected
          </span>
          <Button
            onClick={() => {
              if (selectedFileIds.size === mainFiles.length) {
                setSelectedFileIds(new Set());
              } else {
                setSelectedFileIds(new Set(mainFiles.map((f) => f.id)));
              }
            }}
            size="sm"
            variant="outline"
          >
            {selectedFileIds.size === mainFiles.length
              ? "Deselect All"
              : "Select All"}
          </Button>
          <Button onClick={() => setShowMoveFilesDialog(true)} size="sm">
            Move to...
          </Button>
        </div>
      )}

      {/* Move files dialog - handles both selection mode and single file move */}
      {libraryQuery.data && (
        <MoveFilesDialog
          library={libraryQuery.data}
          onOpenChange={(open) => {
            if (!open) {
              setShowMoveFilesDialog(false);
              setSingleFileMoveId(null);
            }
          }}
          onSuccess={(targetBook) => {
            const movedFileCount =
              singleFileMoveId !== null ? 1 : selectedFileIds.size;
            exitFileSelectMode();
            setSingleFileMoveId(null);
            // Navigate to target book if current book was deleted (all files moved)
            if (movedFileCount === mainFiles.length) {
              navigate(`/libraries/${libraryId}/books/${targetBook.id}`);
            }
          }}
          open={showMoveFilesDialog || singleFileMoveId !== null}
          selectedFiles={
            singleFileMoveId !== null
              ? mainFiles.filter((f) => f.id === singleFileMoveId)
              : mainFiles.filter((f) => selectedFileIds.has(f.id))
          }
          sourceBook={book}
        />
      )}
    </LibraryLayout>
  );
};

export default BookDetail;
