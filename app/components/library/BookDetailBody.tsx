import {
  ArrowRightLeft,
  BookOpen,
  ChevronDown,
  ChevronRight,
  Edit,
  GitMerge,
  Headphones,
  List,
  MoreVertical,
  RefreshCw,
  Search,
  Share2,
  Trash2,
  type LucideIcon,
} from "lucide-react";
import React, {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import AddToListPopover from "@/components/library/AddToListPopover";
import { BookEditDialog } from "@/components/library/BookEditDialog";
import CoverGalleryTabs from "@/components/library/CoverGalleryTabs";
import CoverImage from "@/components/library/CoverImage";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { DeleteConfirmationDialog } from "@/components/library/DeleteConfirmationDialog";
import FileCoverThumbnail from "@/components/library/FileCoverThumbnail";
import FileDownloadControl from "@/components/library/FileDownloadControl";
import { FileEditDialog } from "@/components/library/FileEditDialog";
import FileScanErrorBadge from "@/components/library/FileScanErrorBadge";
import { IdentifyBookDialog } from "@/components/library/IdentifyBookDialog";
import { MergeIntoDialog } from "@/components/library/MergeIntoDialog";
import { MoveFilesDialog } from "@/components/library/MoveFilesDialog";
import { RescanDialog } from "@/components/library/RescanDialog";
import { ReviewPanel } from "@/components/library/ReviewPanel";
import { ShareLinkDialog } from "@/components/library/ShareLinkDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
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
import { getLanguageName } from "@/constants/languages";
import {
  useDeleteBook,
  useDeleteFile,
  useResyncBook,
  useResyncFile,
} from "@/hooks/queries/books";
import { usePluginIdentifierTypes } from "@/hooks/queries/plugins";
import { useSetBookReview } from "@/hooks/queries/review";
import { useSharingSettings } from "@/hooks/queries/sharing";
import { useCan } from "@/hooks/useCan";
import {
  API,
  requestErrorMessage,
  ShishoAPIError,
  toastRequestError,
} from "@/libraries/api";
import { cn } from "@/libraries/utils";
import {
  DownloadFormatKepub,
  FileTypeCBZ,
  type Book,
  type CoverAspectRatio,
  type File,
  type LibrarySummary,
  type PluginIdentifierType,
  type ResyncMode,
} from "@/types";
import { getAuthorRoleLabel } from "@/utils/authorRoles";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";
import { getCoverFileType } from "@/utils/coverSelection";
import { bookCoverUrl } from "@/utils/coverUrl";
import {
  fileDownloadUrl,
  fileKepubDownloadUrl,
  fileOriginalDownloadUrl,
} from "@/utils/downloadUrl";
import {
  fileLabel,
  formatDate,
  formatDateTime,
  formatDuration,
  formatFileSize,
  formatIdentifierType,
  formatPageCount,
  getFilename,
} from "@/utils/format";
import { hasAnyCBZFile } from "@/utils/hasAnyCBZFile";
import { getIdentifierUrl } from "@/utils/identifiers";
import { anyOf } from "@/utils/permissions";
import { getReadingAction } from "@/utils/readingAction";
import { formatSeriesNumber } from "@/utils/seriesNumber";
import { supportsKepub } from "@/utils/supportsKepub";

/**
 * Set by a parent that renders a book for someone outside the library, such as
 * the recipient of a Share Link. Its presence switches the body into share
 * context: resource names render as plain text, and every action control
 * (book and file menus, Read and Listen, Add to list, review toggle, file
 * selection) is hidden regardless of the viewer's permissions. The builders
 * replace the authenticated download and cover endpoints.
 */
export interface ShareLinkContext {
  /** Endpoint the file's download button fetches (HEAD, then navigates). */
  downloadUrl: (file: File) => string;
  /** The book cover URL, or null when the book has no cover. */
  bookCoverUrl: (book: Book) => string | null;
  /** A file's cover URL, or null when the file has no cover. */
  fileCoverUrl: (file: File) => string | null;
  /**
   * The library's cover aspect ratio preference, which sizes the cover box.
   * Defaults to "book" (2:3).
   */
  coverAspectRatio?: CoverAspectRatio;
}

interface DownloadError {
  fileId: number;
  message: string;
}

// Renders a resource name as a link into the library, or as plain text when
// `to` is null (Share Link context, where the viewer cannot reach the library).
const ResourceLink = ({
  to,
  className,
  title,
  children,
}: {
  to: string | null;
  className?: string;
  title?: string;
  children: ReactNode;
}) =>
  to ? (
    <Link className={className} title={title} to={to}>
      {children}
    </Link>
  ) : (
    <span className={className} title={title}>
      {children}
    </span>
  );

interface BookMenuEntry {
  label: string;
  icon: LucideIcon;
  /**
   * Gated on the permission the entry's backend route requires. The menu
   * renders when at least one entry is visible.
   */
  visible: boolean;
  onClick?: () => void;
  onSelect?: () => void;
  disabled?: boolean;
  destructive?: boolean;
}

interface FileRowProps {
  file: File;
  libraryId: number;
  /** Share Link context: plain-text names and no Read or Listen. */
  isShareLink: boolean;
  /** Gates the file menu. Always false in Share Link context. */
  canWriteBooks: boolean;
  pluginIdentifierTypes: PluginIdentifierType[] | undefined;
  getCoverUrl?: (file: File) => string | null;
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
  onRescan: () => void;
  isResyncing: boolean;
  isSupplement?: boolean;
  isSelectMode?: boolean;
  isFileSelected?: boolean;
  onToggleSelect?: () => void;
  onMoveFile?: () => void;
  onDeleteFile: () => void;
  isDeletingFile: boolean;
}

const FileRow = ({
  file,
  libraryId,
  isShareLink,
  canWriteBooks,
  pluginIdentifierTypes,
  getCoverUrl,
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
  onRescan,
  isResyncing,
  isSupplement = false,
  isSelectMode = false,
  isFileSelected = false,
  onToggleSelect,
  onMoveFile,
  onDeleteFile,
  isDeletingFile,
}: FileRowProps) => {
  // Narrator pages need People Read; without it the names are plain text.
  const linkNarrators = useCan("people:read") && !isShareLink;
  const showChevron = hasExpandableMetadata && !isSupplement;
  // fileLabel falls back to the type when the Share Link payload has no name.
  const displayName = fileLabel(file);
  const readingAction = isShareLink ? null : getReadingAction(file.file_type);
  const isListen = readingAction === "listen";
  const readButton = readingAction && (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button asChild size="sm" variant="ghost">
          <Link
            aria-label={isListen ? "Listen" : "Read"}
            to={`/libraries/${libraryId}/books/${file.book_id}/files/${file.id}/read`}
          >
            {isListen ? (
              <Headphones className="h-3 w-3" />
            ) : (
              <BookOpen className="h-3 w-3" />
            )}
          </Link>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{isListen ? "Listen" : "Read"}</TooltipContent>
    </Tooltip>
  );

  return (
    <div className="py-3 flex gap-3">
      {/* Selection checkbox */}
      {isSelectMode && (
        <Checkbox
          aria-label={`Select ${displayName}`}
          checked={isFileSelected}
          className="mt-1 size-5 self-start"
          onCheckedChange={() => onToggleSelect?.()}
          onClick={(e) => e.stopPropagation()}
        />
      )}

      {/* Chevron indicator - aligned to top */}
      {showChevron ? (
        <Button
          aria-expanded={isExpanded}
          aria-label={isExpanded ? "Hide file details" : "Show file details"}
          className="mt-1 shrink-0 self-start"
          onClick={onToggleExpand}
          size="icon-xs"
          variant="ghost"
        >
          {isExpanded ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          )}
        </Button>
      ) : (
        <div className="w-5 shrink-0" /> // Spacer for alignment when no chevron
      )}

      {/* File cover thumbnail - constrained height, natural aspect ratio */}
      {!isSupplement && (
        <div className="shrink-0 self-start">
          <FileCoverThumbnail
            className="h-14"
            file={file}
            getCoverUrl={getCoverUrl}
          />
        </div>
      )}

      {/* Content area */}
      <div className="flex-1 min-w-0 space-y-1">
        {/* Main row: badge, name, stats, actions */}
        <div className="flex items-center gap-2">
          {/* File type badge */}
          <Badge
            className="uppercase text-xs shrink-0"
            variant={isSupplement ? "outline" : "secondary"}
          >
            {file.file_type}
          </Badge>

          {/* Unreadable file warning */}
          <FileScanErrorBadge className="text-xs shrink-0" file={file} />

          {/* Name */}
          <ResourceLink
            className={cn(
              "truncate min-w-0 flex-1 text-sm",
              !isShareLink && "hover:underline",
              !isSupplement && "font-medium",
            )}
            title={displayName}
            to={
              isShareLink
                ? null
                : `/libraries/${libraryId}/books/${file.book_id}/files/${file.id}`
            }
          >
            {displayName}
          </ResourceLink>

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
                <span>{formatPageCount(file.page_count)}</span>
                <span className="text-muted-foreground/50">·</span>
              </>
            )}
            {/* File size - always shown */}
            <span>{formatFileSize(file.filesize_bytes)}</span>

            {/* Download button/popover */}
            <FileDownloadControl
              file={file}
              isDownloading={isDownloading}
              isSupplement={isSupplement}
              libraryDownloadPreference={libraryDownloadPreference}
              onCancelDownload={onCancelDownload}
              onDownload={onDownload}
              onDownloadKepub={onDownloadKepub}
              onDownloadOriginal={onDownloadOriginal}
              onDownloadWithEndpoint={onDownloadWithEndpoint}
            />

            {/* Read button for ebooks/comics, Listen for M4B audiobooks */}
            {readButton}

            {/* Actions dropdown */}
            {canWriteBooks && (
              <DropdownMenu>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <DropdownMenuTrigger asChild>
                      <Button
                        aria-label="File actions"
                        disabled={isResyncing}
                        size="sm"
                        variant="ghost"
                      >
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
                  <DropdownMenuItem disabled={isResyncing} onClick={onRescan}>
                    <RefreshCw className="h-4 w-4 mr-2" />
                    Rescan file
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
                  <DropdownMenuItem
                    className="text-destructive focus:text-destructive"
                    disabled={isDeletingFile}
                    onClick={onDeleteFile}
                  >
                    <Trash2 className="h-4 w-4 mr-2 text-destructive" />
                    Delete file
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        </div>

        {/* Stats and actions - mobile only (separate row).
            Must wrap: audiobooks carry the widest stat payload (duration,
            bitrate, codec, filesize) plus three action buttons, which would
            otherwise overflow the content column and force page-level
            horizontal scroll on narrow viewports. */}
        <div className="flex md:hidden flex-wrap items-center gap-x-2 gap-y-1 min-w-0 text-xs text-muted-foreground">
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
              <span>{formatPageCount(file.page_count)}</span>
              <span className="text-muted-foreground/50">·</span>
            </>
          )}
          {/* File size - always shown */}
          <span>{formatFileSize(file.filesize_bytes)}</span>

          {/* Download button/popover */}
          <FileDownloadControl
            file={file}
            isDownloading={isDownloading}
            isSupplement={isSupplement}
            libraryDownloadPreference={libraryDownloadPreference}
            onCancelDownload={onCancelDownload}
            onDownload={onDownload}
            onDownloadKepub={onDownloadKepub}
            onDownloadOriginal={onDownloadOriginal}
            onDownloadWithEndpoint={onDownloadWithEndpoint}
          />

          {/* Read button for ebooks/comics, Listen for M4B audiobooks */}
          {readButton}

          {/* Actions dropdown */}
          {canWriteBooks && (
            <DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <DropdownMenuTrigger asChild>
                    <Button
                      aria-label="File actions"
                      disabled={isResyncing}
                      size="sm"
                      variant="ghost"
                    >
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
                <DropdownMenuItem disabled={isResyncing} onClick={onRescan}>
                  <RefreshCw className="h-4 w-4 mr-2" />
                  Rescan file
                </DropdownMenuItem>
                <DropdownMenuItem
                  className="text-destructive focus:text-destructive"
                  disabled={isDeletingFile}
                  onClick={onDeleteFile}
                >
                  <Trash2 className="h-4 w-4 mr-2 text-destructive" />
                  Delete file
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>

        {/* Filename row - only show when the display name differs from the filename. The
            share payload blanks filesystem paths, so there is nothing to show. */}
        {!isShareLink &&
          file.filepath &&
          file.display_name !== getFilename(file.filepath) && (
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
              <span className="text-xs min-w-0 break-words" key={narrator.id}>
                <ResourceLink
                  className={cn(linkNarrators && "hover:underline")}
                  to={
                    linkNarrators
                      ? `/libraries/${libraryId}/people/${narrator.person_id}`
                      : null
                  }
                >
                  {narrator.person?.name ?? "Unknown"}
                </ResourceLink>
                {index < file.narrators!.length - 1 ? "," : ""}
              </span>
            ))}
          </div>
        )}

        {/* Expandable details section */}
        {isExpanded && hasExpandableMetadata && (
          <div className="mt-2 bg-muted/50 rounded-md p-3 text-xs space-y-2">
            {/* Publisher, Released, URL */}
            <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1">
              {file.publisher && (
                <>
                  <span className="text-muted-foreground">Publisher</span>
                  <ResourceLink
                    className={cn(
                      "break-words",
                      !isShareLink && "hover:underline",
                    )}
                    to={
                      isShareLink
                        ? null
                        : `/libraries/${libraryId}/publishers/${file.publisher.id}`
                    }
                  >
                    {file.publisher.name}
                  </ResourceLink>
                </>
              )}
              {file.release_date && (
                <>
                  <span className="text-muted-foreground">Released</span>
                  <span>{formatDate(file.release_date)}</span>
                </>
              )}
              {file.url && !isShareLink && (
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
              {file.language && (
                <>
                  <span className="text-muted-foreground">Language</span>
                  <span>
                    {getLanguageName(file.language)
                      ? `${getLanguageName(file.language)} (${file.language})`
                      : file.language}
                  </span>
                </>
              )}
              {/* For M4B, always show abridged status (audiobooks historically
                  had abridged versions, so the distinction is meaningful).
                  For other formats, only show when explicitly marked as
                  abridged. */}
              {(file.file_type === "m4b" || file.abridged === true) && (
                <>
                  <span className="text-muted-foreground">Abridged</span>
                  <span>
                    {file.abridged == null
                      ? "Unknown"
                      : file.abridged
                        ? "Yes"
                        : "No"}
                  </span>
                </>
              )}
            </div>

            {/* Identifiers. Share Link context omits them with the URL: they
                are catalog details, often internal ones such as a UUID. */}
            {!isShareLink &&
              file.identifiers &&
              file.identifiers.length > 0 && (
                <div className="pt-2 border-t border-border/50">
                  <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1">
                    {file.identifiers.map((id, idx) => {
                      const url = getIdentifierUrl(
                        id.type,
                        id.value,
                        pluginIdentifierTypes,
                      );
                      return (
                        <React.Fragment key={idx}>
                          <span className="text-muted-foreground">
                            {formatIdentifierType(
                              id.type,
                              pluginIdentifierTypes,
                            )}
                          </span>
                          {url ? (
                            <a
                              className="font-mono select-all text-primary hover:underline break-all"
                              href={url}
                              rel="noopener noreferrer"
                              target="_blank"
                            >
                              {id.value}
                            </a>
                          ) : (
                            <span className="font-mono select-all break-all">
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
    </div>
  );
};

interface BookDetailBodyProps {
  book: Book;
  /**
   * The book's library, for the download format preference, the cover aspect
   * ratio, and the merge and move dialogs. Absent in Share Link context.
   */
  library?: LibrarySummary;
  shareLink?: ShareLinkContext;
}

// The presentational body of Book Detail: cover, metadata, resource lists,
// and the file list with download and read controls, plus the action menus
// and the dialogs behind them. The page supplies the book and library from
// the authenticated query hooks; a share page supplies a share payload and a
// `shareLink`. Every new control here decides its Share Link behavior:
// anything that links into the app or mutates data is hidden or rendered as
// plain text when `isShareLink` is true, with a case in
// BookDetailBody.test.tsx, and every query is disabled there so the body
// makes no authenticated requests for the recipient.
const BookDetailBody = ({ book, library, shareLink }: BookDetailBodyProps) => {
  const isShareLink = !!shareLink;
  const libraryId = book.library_id;
  const navigate = useNavigate();
  // Metadata, covers, chapters, review state, identify, rescan, merge, move,
  // and delete all require Books Write. List membership is governed by the
  // list's own permission and stays available to everyone.
  const canWriteBooks = useCan("books:write") && !isShareLink;
  // Shares Write creates links, and either shares operation lists them.
  const canWriteShares = useCan("shares:write") && !isShareLink;
  const canListShares =
    useCan(anyOf("shares:read", "shares:write")) && !isShareLink;
  // The sharing settings link in the Share dialog opens a Config Write page.
  const canManageSharing = useCan("config:write");
  // Author and series pages need People Read and Series Read. Without them
  // the names render as plain text instead of links to an error page.
  const linkPeople = useCan("people:read") && !isShareLink;
  const linkSeries = useCan("series:read") && !isShareLink;
  // The hook checks the endpoint's permissions itself.
  const { data: sharingSettings } = useSharingSettings({
    enabled: !isShareLink,
  });
  // Share stays offered while sharing is off: no link works then, but a
  // sharer can still revoke or delete one without an admin turning sharing
  // back on. The dialog explains the state and hides the new-link form, so it
  // waits for the settings rather than guessing the policy.
  const canShare = canListShares && sharingSettings !== undefined;
  const sharingEnabled = sharingSettings?.enabled === true;
  const { data: pluginIdentifierTypes } = usePluginIdentifierTypes({
    enabled: !isShareLink,
  });

  const resyncFileMutation = useResyncFile();
  const resyncBookMutation = useResyncBook();
  const deleteBookMutation = useDeleteBook();
  const deleteFileMutation = useDeleteFile();
  const setBookReviewMutation = useSetBookReview();
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [addToListOpen, setAddToListOpen] = useState(false);
  const [shareDialogOpen, setShareDialogOpen] = useState(false);
  const [showBookRescanDialog, setShowBookRescanDialog] = useState(false);
  const [rescanFileId, setRescanFileId] = useState<number | null>(null);
  const [showMergeIntoDialog, setShowMergeIntoDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
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
  const [deletingFileId, setDeletingFileId] = useState<number | null>(null);
  const [fileToDelete, setFileToDelete] = useState<File | null>(null);
  const [showIdentifyDialog, setShowIdentifyDialog] = useState(false);

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

  // Share Link context hides the URL and identifiers (see FileRow), so they
  // alone do not make a file expandable there.
  const hasExpandableMetadata = (file: File): boolean => {
    return !!(
      file.publisher ||
      file.release_date ||
      file.language ||
      file.file_type === "m4b" || // M4B always shows abridged status
      file.abridged === true ||
      (!isShareLink &&
        (file.url || (file.identifiers && file.identifiers.length > 0)))
    );
  };

  const coverCacheKey = book.cover_cache_key;
  const coverUrl = shareLink
    ? shareLink.bookCoverUrl(book)
    : bookCoverUrl(book);

  // Reset the error flag whenever the URL changes — either because we
  // navigated to a different book, or because a rescan bumped the
  // coverCacheKey. If we only reset on book id, a previously-missing cover
  // stays unmounted after a successful rescan until the user refreshes.
  useEffect(() => {
    setCoverError(false);
  }, [coverUrl, coverCacheKey]);

  // Check cache synchronously before paint to avoid placeholder flash
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
        // A HEAD response has no body, so GET the error message. The dialog
        // shows the API's message, or the fallback when a proxy answered or
        // the server failed without one (requestErrorMessage). A GET that
        // succeeds after all is not read, since its body is the whole file.
        const errorResponse = await fetch(endpoint, {
          signal: abortController.signal,
        });
        let message = "Failed to download file";
        if (!errorResponse.ok) {
          try {
            await API.checkStatus(errorResponse);
          } catch (error) {
            // A cancel while reading the body is an AbortError, which the
            // outer catch ignores.
            if (!(error instanceof ShishoAPIError)) throw error;
            message = requestErrorMessage(error, message);
          }
        }
        setDownloadError({ fileId, message });
        return;
      }

      // HEAD succeeded - file is ready, trigger streaming download
      window.location.assign(endpoint);
      toast.success("Download started");
    } catch (error) {
      // Don't show error dialog for user-initiated cancellation
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      console.error("Download error:", error);
      toastRequestError(error, "Failed to download file");
    } finally {
      downloadAbortController.current = null;
      setDownloadingFileId(null);
    }
  };

  const handleDownload = async (file: File) => {
    if (shareLink) {
      await handleDownloadWithEndpoint(file.id, shareLink.downloadUrl(file));
      return;
    }

    const preference = library?.download_format_preference;

    // For kepub preference with supported files, use kepub endpoint
    if (preference === DownloadFormatKepub && supportsKepub(file.file_type)) {
      await handleDownloadWithEndpoint(file.id, fileKepubDownloadUrl(file.id));
    } else {
      // Original format for unsupported files or "original" preference
      await handleDownloadWithEndpoint(file.id, fileDownloadUrl(file.id));
    }
  };

  const handleDownloadKepub = async (fileId: number) => {
    if (shareLink) return;
    await handleDownloadWithEndpoint(fileId, fileKepubDownloadUrl(fileId));
  };

  const handleDownloadOriginal = (fileId: number) => {
    // A Share Link exposes only the generated download.
    if (shareLink) {
      const file = book.files?.find((f) => f.id === fileId);
      if (file) handleDownload(file);
      return;
    }
    // Direct download of original file - this won't show any error since it's a simple file serve
    window.location.assign(fileOriginalDownloadUrl(fileId));
    setDownloadError(null);
  };

  const handleCancelDownload = () => {
    downloadAbortController.current?.abort();
    downloadAbortController.current = null;
    setDownloadingFileId(null);
  };

  const handleRescanFile = async (fileId: number, mode: ResyncMode) => {
    setResyncingFileId(fileId);
    try {
      const result = await resyncFileMutation.mutateAsync({
        fileId,
        payload: { mode },
      });
      if ("file_deleted" in result && result.file_deleted) {
        toast.success("File removed (no longer on disk, or DRM-protected)");
      } else {
        toast.success("File rescanned");
      }
    } catch (error) {
      toastRequestError(error, "Failed to rescan file");
    } finally {
      setResyncingFileId(null);
    }
  };

  const handleRescanBook = async (mode: ResyncMode) => {
    try {
      const result = await resyncBookMutation.mutateAsync({
        bookId: book.id,
        payload: { mode },
      });
      if ("book_deleted" in result && result.book_deleted) {
        toast.success("Book removed (no files remain)");
      } else {
        toast.success("Book rescanned");
      }
    } catch (error) {
      toastRequestError(error, "Failed to rescan book");
    }
  };

  const handleDeleteBook = async () => {
    try {
      await deleteBookMutation.mutateAsync(book.id);
      toast.success("Book deleted");
      navigate("/");
    } catch (error) {
      toastRequestError(error, "Failed to delete book");
    }
  };

  const handleDeleteFile = async () => {
    if (!fileToDelete) return;
    setDeletingFileId(fileToDelete.id);
    try {
      const result = await deleteFileMutation.mutateAsync(fileToDelete.id);
      setFileToDelete(null);
      if (result.book_deleted) {
        toast.success("Book deleted");
        navigate("/");
      } else {
        toast.success("File deleted");
      }
    } catch (error) {
      toastRequestError(error, "Failed to delete file");
    } finally {
      setDeletingFileId(null);
    }
  };

  // Check if any file is CBZ for series number display formatting
  const anyCBZ = hasAnyCBZFile(book);

  // Separate main files and supplements
  const mainFiles =
    book.files?.filter((f) => f.file_role !== "supplement") ?? [];
  const supplements =
    book.files?.filter((f) => f.file_role === "supplement") ?? [];

  // Determine which file type would provide the cover based on library's cover_aspect_ratio setting
  // This is used to determine the native aspect ratio (audiobook = square, book = 2:3)
  const libraryCoverAspectRatio =
    shareLink?.coverAspectRatio ?? library?.cover_aspect_ratio ?? "book";
  const coverFileType = getCoverFileType(book.files, libraryCoverAspectRatio);
  const isAudiobook = coverFileType === "audiobook";
  const coverAspectRatio = isAudiobook ? "aspect-square" : "aspect-[2/3]";

  const handleCoverLoad = () => {
    if (coverUrl) {
      markCoverLoaded(coverUrl);
    }
    setCoverLoaded(true);
  };

  // Book action menu entries, grouped by separator. Each entry carries its own
  // permission; the menu renders when any entry is visible.
  const editGroup: BookMenuEntry[] = [
    {
      label: "Edit",
      icon: Edit,
      visible: canWriteBooks,
      onClick: () => setEditDialogOpen(true),
    },
  ];
  const otherGroups: BookMenuEntry[][] = [
    [
      {
        label: "Share",
        icon: Share2,
        visible: canShare,
        onClick: () => setShareDialogOpen(true),
      },
    ],
    [
      {
        label: "Rescan book",
        icon: RefreshCw,
        visible: canWriteBooks,
        disabled: resyncBookMutation.isPending,
        onClick: () => setShowBookRescanDialog(true),
      },
      {
        label: "Identify book",
        icon: Search,
        visible: canWriteBooks,
        onClick: () => setShowIdentifyDialog(true),
      },
    ],
    [
      {
        label: "Merge into another book",
        icon: GitMerge,
        visible: canWriteBooks,
        onClick: () => setShowMergeIntoDialog(true),
      },
      {
        label: "Delete book",
        icon: Trash2,
        visible: canWriteBooks,
        destructive: true,
        onClick: () => setShowDeleteDialog(true),
      },
    ],
  ];
  // Add to list has no permission of its own (the list's permission governs
  // membership). It joins the menu whenever another entry puts the menu on
  // screen, and otherwise stands alone as a button, so it never appears twice.
  const addToListInMenu = [...editGroup, ...otherGroups.flat()].some(
    (entry) => entry.visible,
  );
  const addToListGroup: BookMenuEntry[] = [
    {
      label: "Add to list",
      icon: List,
      visible: addToListInMenu,
      onSelect: () => {
        setTimeout(() => setAddToListOpen(true), 0);
      },
    },
  ];
  const visibleBookMenuGroups = [editGroup, addToListGroup, ...otherGroups]
    .map((group) => group.filter((entry) => entry.visible))
    .filter((group) => group.length > 0);
  const showBookMenu = visibleBookMenuGroups.length > 0;

  const renderFileRow = (file: File, isSupplement: boolean) => (
    <FileRow
      canWriteBooks={canWriteBooks}
      file={file}
      getCoverUrl={shareLink?.fileCoverUrl}
      hasExpandableMetadata={hasExpandableMetadata(file)}
      isDeletingFile={deletingFileId === file.id}
      isDownloading={downloadingFileId === file.id}
      isExpanded={expandedFileIds.has(file.id)}
      isFileSelected={selectedFileIds.has(file.id)}
      isResyncing={resyncingFileId === file.id}
      isSelectMode={!isSupplement && canWriteBooks && isFileSelectMode}
      isShareLink={isShareLink}
      isSupplement={isSupplement}
      key={file.id}
      // Share Link context offers one download per file: no format choice.
      libraryDownloadPreference={
        isShareLink ? undefined : library?.download_format_preference
      }
      libraryId={libraryId}
      onCancelDownload={handleCancelDownload}
      onDeleteFile={() => setFileToDelete(file)}
      onDownload={() => handleDownload(file)}
      onDownloadKepub={() => handleDownloadKepub(file.id)}
      onDownloadOriginal={() => handleDownloadOriginal(file.id)}
      onDownloadWithEndpoint={(endpoint) =>
        handleDownloadWithEndpoint(file.id, endpoint)
      }
      onEdit={() => setEditingFile(file)}
      onMoveFile={isSupplement ? undefined : () => setSingleFileMoveId(file.id)}
      onRescan={() => setRescanFileId(file.id)}
      onToggleExpand={() => toggleFileExpanded(file.id)}
      onToggleSelect={
        isSupplement ? undefined : () => toggleFileSelection(file.id)
      }
      pluginIdentifierTypes={pluginIdentifierTypes}
    />
  );

  return (
    <>
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 md:gap-8">
        {/* Book Cover */}
        <div className="lg:col-span-1 space-y-4 md:space-y-6">
          {mainFiles.length > 1 ? (
            /* Multiple files - show cover gallery with tabs */
            <CoverGalleryTabs
              files={mainFiles}
              getCoverUrl={shareLink?.fileCoverUrl}
            />
          ) : (
            /* Single file - show book cover directly */
            <div
              className={cn(
                coverAspectRatio,
                "w-48 sm:w-64 lg:w-full mx-auto lg:mx-0 relative",
              )}
            >
              {/* Placeholder shown until image loads or on error */}
              {(!coverLoaded || coverError || !coverUrl) && (
                <CoverPlaceholder
                  className="absolute inset-0 rounded-md border border-border"
                  variant={coverFileType}
                />
              )}
              {/* Image hidden until loaded, removed on error */}
              {!coverError && coverUrl && (
                <CoverImage
                  alt={`${book.title} Cover`}
                  className={cn(
                    "w-full h-full object-cover rounded-md border border-border",
                    !coverLoaded && "opacity-0",
                  )}
                  key={coverCacheKey}
                  onError={() => setCoverError(true)}
                  onLoad={handleCoverLoad}
                  src={coverUrl}
                />
              )}
            </div>
          )}

          {/* Review Panel — visible when book has files, fires mutation immediately */}
          {!isShareLink && (book.files?.length ?? 0) > 0 && (
            <ReviewPanel
              book={book}
              files={book.files ?? []}
              isPending={setBookReviewMutation.isPending}
              onChange={(override) =>
                setBookReviewMutation.mutate(
                  { bookId: book.id, override },
                  {
                    onError: (error) =>
                      toastRequestError(error, "Failed to update review state"),
                  },
                )
              }
              readOnly={!canWriteBooks}
            />
          )}
        </div>

        {/* Book Details */}
        <div className="lg:col-span-2 space-y-4 md:space-y-6">
          <div>
            <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3 mb-2">
              <h1 className="text-2xl font-semibold min-w-0 break-words">
                {book.title}
              </h1>
              {/*
                The relative wrapper exists so AddToListPopover can be opened
                from the dropdown menu's "Add to list" item: its trigger is an
                invisible absolute span that anchors the popover to the same
                rectangle as the dropdown's "..." button.
              */}
              {!isShareLink &&
                (showBookMenu ? (
                  <div className="relative shrink-0">
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          aria-label="Book actions"
                          size="sm"
                          variant="outline"
                        >
                          <MoreVertical className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent
                        align="end"
                        onCloseAutoFocus={(e) => e.preventDefault()}
                      >
                        {visibleBookMenuGroups.map((group, groupIndex) => (
                          <React.Fragment key={group[0].label}>
                            {groupIndex > 0 && <DropdownMenuSeparator />}
                            {group.map((entry) => (
                              <DropdownMenuItem
                                className={cn(
                                  entry.destructive &&
                                    "text-destructive focus:text-destructive",
                                )}
                                disabled={entry.disabled}
                                key={entry.label}
                                onClick={entry.onClick}
                                onSelect={entry.onSelect}
                              >
                                <entry.icon
                                  className={cn(
                                    "h-4 w-4 mr-2",
                                    entry.destructive && "text-destructive",
                                  )}
                                />
                                {entry.label}
                              </DropdownMenuItem>
                            ))}
                          </React.Fragment>
                        ))}
                      </DropdownMenuContent>
                    </DropdownMenu>
                    <AddToListPopover
                      bookId={book.id}
                      onOpenChange={setAddToListOpen}
                      open={addToListOpen}
                      trigger={
                        <span
                          aria-hidden
                          className="absolute inset-0 pointer-events-none"
                        />
                      }
                    />
                  </div>
                ) : (
                  <AddToListPopover
                    bookId={book.id}
                    trigger={
                      <Button className="shrink-0" size="sm" variant="outline">
                        <List className="h-4 w-4 mr-2" />
                        Add to list
                      </Button>
                    }
                  />
                ))}
            </div>
            {/* A recipient has no use for the library's sort order. */}
            {!isShareLink &&
              book.sort_title &&
              book.sort_title !== book.title && (
                <p className="text-sm text-muted-foreground italic break-words">
                  Sort title: {book.sort_title}
                </p>
              )}
            {book.subtitle && (
              <p className="text-lg text-muted-foreground break-words">
                {book.subtitle}
              </p>
            )}
            {book.description && (
              <p className="text-sm text-muted-foreground mt-3 whitespace-pre-wrap break-words">
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
                        const roleLabel = getAuthorRoleLabel(author.role);
                        const linked = linkPeople && !!author.person;
                        const badge = (
                          <Badge
                            className={cn(
                              linked && "cursor-pointer hover:bg-secondary/80",
                            )}
                            variant="secondary"
                          >
                            {author.person?.name ?? "Unknown"}
                            {hasCBZFiles && roleLabel && (
                              <span className="text-muted-foreground ml-1">
                                ({roleLabel})
                              </span>
                            )}
                          </Badge>
                        );
                        return (
                          <ResourceLink
                            key={author.id}
                            to={
                              linked
                                ? `/libraries/${libraryId}/people/${author.person_id}`
                                : null
                            }
                          >
                            {badge}
                          </ResourceLink>
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
                      <ResourceLink
                        className={cn(
                          "text-sm font-medium",
                          linkSeries &&
                            "text-primary hover:text-primary/80 hover:underline",
                        )}
                        to={
                          linkSeries
                            ? `/libraries/${libraryId}/series/${bs.series_id}`
                            : null
                        }
                      >
                        {bs.series?.name ?? "Unknown Series"}
                      </ResourceLink>
                      {bs.series_number != null && (
                        <Badge className="text-xs" variant="outline">
                          {formatSeriesNumber(
                            bs.series_number,
                            bs.series_number_end,
                            bs.series_number_unit,
                            anyCBZ ? "cbz" : null,
                          )}
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
                    <ResourceLink
                      key={bg.id}
                      to={
                        isShareLink
                          ? null
                          : `/libraries/${libraryId}?genre_ids=${bg.genre_id}`
                      }
                    >
                      <Badge
                        className={cn(
                          !isShareLink &&
                            "cursor-pointer hover:bg-secondary/80",
                        )}
                        variant="secondary"
                      >
                        {bg.genre?.name ?? "Unknown"}
                      </Badge>
                    </ResourceLink>
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
                    <ResourceLink
                      key={bt.id}
                      to={
                        isShareLink
                          ? null
                          : `/libraries/${libraryId}?tag_ids=${bt.tag_id}`
                      }
                    >
                      <Badge
                        className={cn(
                          !isShareLink &&
                            "cursor-pointer hover:bg-secondary/80",
                        )}
                        variant="secondary"
                      >
                        {bt.tag?.name ?? "Unknown"}
                      </Badge>
                    </ResourceLink>
                  ))}
                </div>
              </div>
            )}

            {/* Metadata. Share Link context omits the whole block: the
                created and updated times, library, and file path describe the
                server's records rather than the book. */}
            {!isShareLink && (
              <>
                <Separator />
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
                  <div>
                    <p className="font-semibold">Created</p>
                    <p className="text-muted-foreground">
                      {formatDateTime(book.created_at)}
                    </p>
                  </div>
                  <div>
                    <p className="font-semibold">Updated</p>
                    <p className="text-muted-foreground">
                      {formatDateTime(book.updated_at)}
                    </p>
                  </div>
                  <div>
                    <p className="font-semibold">Library</p>
                    <p className="text-muted-foreground break-words">
                      {book.library?.name || `Library ${book.library_id}`}
                    </p>
                  </div>
                  <div>
                    <p className="font-semibold">File Path</p>
                    <p className="text-muted-foreground break-words">
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
              </>
            )}

            <Separator />

            {/* Files */}
            <div>
              <div className="flex items-center justify-between mb-3">
                <h3 className="font-semibold">Files ({mainFiles.length})</h3>
                {canWriteBooks && mainFiles.length > 1 && (
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
                {mainFiles.map((file) => renderFileRow(file, false))}
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
                    {supplements.map((file) => renderFileRow(file, true))}
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      </div>

      {canWriteBooks && (
        <>
          <BookEditDialog
            book={book}
            onOpenChange={setEditDialogOpen}
            open={editDialogOpen}
          />

          <IdentifyBookDialog
            book={book}
            onOpenChange={setShowIdentifyDialog}
            open={showIdentifyDialog}
          />

          <RescanDialog
            entityName={book.title}
            entityType="book"
            isPending={resyncBookMutation.isPending}
            onConfirm={handleRescanBook}
            onOpenChange={setShowBookRescanDialog}
            open={showBookRescanDialog}
          />

          {(() => {
            const rescanFile = rescanFileId
              ? book.files.find((f) => f.id === rescanFileId)
              : null;
            return (
              <RescanDialog
                entityName={rescanFile ? fileLabel(rescanFile) : ""}
                entityType="file"
                isPending={resyncFileMutation.isPending}
                onConfirm={(mode) => {
                  if (rescanFileId) handleRescanFile(rescanFileId, mode);
                }}
                onOpenChange={(open) => {
                  if (!open) setRescanFileId(null);
                }}
                open={rescanFileId !== null}
              />
            );
          })()}

          {editingFile && (
            <FileEditDialog
              book={book}
              file={editingFile}
              onOpenChange={(open) => {
                if (!open) setEditingFile(null);
              }}
              open={!!editingFile}
            />
          )}
        </>
      )}

      {/* Download Error Dialog */}
      <Dialog
        onOpenChange={(open) => {
          if (!open) setDownloadError(null);
        }}
        open={!!downloadError}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Download Failed</DialogTitle>
          </DialogHeader>
          <DialogBody>
            <DialogDescription>{downloadError?.message}</DialogDescription>
          </DialogBody>
          <DialogFooter>
            <Button
              onClick={() => setDownloadError(null)}
              size="sm"
              variant="outline"
            >
              Cancel
            </Button>
            {/* The original-file variant is not exposed in Share Link context. */}
            {downloadError && !isShareLink && (
              <Button
                onClick={() => handleDownloadOriginal(downloadError.fileId)}
                size="sm"
              >
                Download Original
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {canWriteBooks && library && (
        <MergeIntoDialog
          library={library}
          onOpenChange={setShowMergeIntoDialog}
          onSuccess={(targetBook) => {
            // Replace history so back button doesn't return to the now-deleted book
            navigate(`/libraries/${libraryId}/books/${targetBook.id}`, {
              replace: true,
            });
          }}
          open={showMergeIntoDialog}
          sourceBook={book}
        />
      )}

      {/* File selection action bar */}
      {canWriteBooks && selectedFileIds.size > 0 && (
        <div className="fixed bottom-4 left-1/2 -translate-x-1/2 bg-background border rounded-md shadow-lg p-3 flex items-center gap-3 z-50">
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
      {canWriteBooks && library && (
        <MoveFilesDialog
          library={library}
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
            // Replace history so back button doesn't return to the now-deleted book
            if (movedFileCount === mainFiles.length) {
              navigate(`/libraries/${libraryId}/books/${targetBook.id}`, {
                replace: true,
              });
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

      {canWriteBooks && (
        <DeleteConfirmationDialog
          files={book.files}
          isPending={deleteBookMutation.isPending}
          onConfirm={handleDeleteBook}
          onOpenChange={setShowDeleteDialog}
          open={showDeleteDialog}
          title={book.title}
          variant="book"
        />
      )}

      {canShare && (
        <ShareLinkDialog
          bookId={book.id}
          bookTitle={book.title}
          canList={canListShares}
          canManageSharing={canManageSharing}
          canWrite={canWriteShares}
          onOpenChange={setShareDialogOpen}
          open={shareDialogOpen}
          requireExpiration={sharingSettings?.require_expiration ?? false}
          sharingEnabled={sharingEnabled}
        />
      )}

      {canWriteBooks && fileToDelete && (
        <DeleteConfirmationDialog
          isPending={deleteFileMutation.isPending}
          onConfirm={handleDeleteFile}
          onOpenChange={(open) => !open && setFileToDelete(null)}
          open={!!fileToDelete}
          title={fileToDelete.filepath.split("/").pop() ?? "File"}
          variant="file"
        />
      )}
    </>
  );
};

export default BookDetailBody;
