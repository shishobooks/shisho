import { uniqBy } from "lodash";
import {
  Check,
  List,
  MoreVertical,
  RefreshCw,
  Search,
  Trash2,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import AddToListPopover from "@/components/library/AddToListPopover";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { DeleteConfirmationDialog } from "@/components/library/DeleteConfirmationDialog";
import { IdentifyBookDialog } from "@/components/library/IdentifyBookDialog";
import { RescanDialog } from "@/components/library/RescanDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import {
  COVER_WIDTH_CLASS,
  DEFAULT_GALLERY_SIZE,
} from "@/constants/gallerySize";
import { useDeleteBook, useResyncBook } from "@/hooks/queries/books";
import { type RescanMode } from "@/hooks/queries/resync";
import { useIsTruncated } from "@/hooks/useIsTruncated";
import { cn } from "@/libraries/utils";
import {
  AuthorRolePenciller,
  AuthorRoleWriter,
  FileTypeCBZ,
  type Book,
  type File,
  type GallerySize,
} from "@/types";
import { isBookNeedsReview } from "@/utils/book";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";
import { getPrimaryFileType } from "@/utils/primaryFile";
import { formatSeriesNumber } from "@/utils/seriesNumber";

interface BookItemProps {
  book: Book;
  libraryId: string;
  seriesId?: number;
  coverAspectRatio?: string;
  addedByUsername?: string;
  isSelectionMode?: boolean;
  isSelected?: boolean;
  onSelect?: () => void;
  onShiftSelect?: () => void;
  cacheKey?: number;
  gallerySize?: GallerySize;
}

// Selects the file that would be used for the cover based on cover_aspect_ratio setting
// This mirrors the backend's selectCoverFile logic (requires cover_image_filename)
const selectCoverFile = (
  files: File[] | undefined,
  coverAspectRatio: string,
): File | null => {
  if (!files) return null;

  const bookFiles = files.filter(
    (f) =>
      (f.file_type === "epub" || f.file_type === "cbz") &&
      f.cover_image_filename,
  );
  const audiobookFiles = files.filter(
    (f) => f.file_type === "m4b" && f.cover_image_filename,
  );

  switch (coverAspectRatio) {
    case "audiobook":
    case "audiobook_fallback_book":
      if (audiobookFiles.length > 0) return audiobookFiles[0];
      if (bookFiles.length > 0) return bookFiles[0];
      break;
    default: // "book", "book_fallback_audiobook"
      if (bookFiles.length > 0) return bookFiles[0];
      if (audiobookFiles.length > 0) return audiobookFiles[0];
  }
  return null;
};

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

const getAspectRatioClass = (
  coverAspectRatio: string,
  files?: File[],
): string => {
  // For non-fallback modes, just use the specified aspect ratio
  if (coverAspectRatio === "audiobook") return "aspect-square";
  if (coverAspectRatio === "book") return "aspect-[2/3]";

  // For fallback modes, first check if there's an actual cover file
  const coverFile = selectCoverFile(files, coverAspectRatio);
  if (coverFile) {
    // Use the actual cover file's type
    return coverFile.file_type === "m4b" ? "aspect-square" : "aspect-[2/3]";
  }

  // No cover - use getCoverFileType to determine which file type WOULD provide the cover
  const fileType = getCoverFileType(files, coverAspectRatio);
  return fileType === "audiobook" ? "aspect-square" : "aspect-[2/3]";
};

const BookItem = ({
  book,
  libraryId,
  seriesId,
  coverAspectRatio = "book",
  addedByUsername,
  isSelectionMode = false,
  isSelected = false,
  onSelect,
  onShiftSelect,
  cacheKey,
  gallerySize = DEFAULT_GALLERY_SIZE,
}: BookItemProps) => {
  const [titleRef, isTitleTruncated] = useIsTruncated<HTMLDivElement>();

  // Find the series number and unit for the specific series context (if provided)
  const seriesEntry = seriesId
    ? book.book_series?.find((bs) => bs.series_id === seriesId)
    : undefined;
  const seriesNumber = seriesEntry?.series_number;
  const seriesNumberUnit = seriesEntry?.series_number_unit;
  const primaryFileType = getPrimaryFileType(book);

  const aspectClass = getAspectRatioClass(coverAspectRatio, book.files);
  const coverUrl = cacheKey
    ? `/api/books/${book.id}/cover?v=${cacheKey}`
    : `/api/books/${book.id}/cover`;
  const [coverLoaded, setCoverLoaded] = useState(() => isCoverLoaded(coverUrl));
  const [coverError, setCoverError] = useState(false);
  const [showRescanDialog, setShowRescanDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [showIdentifyDialog, setShowIdentifyDialog] = useState(false);
  const resyncBookMutation = useResyncBook();
  const deleteBookMutation = useDeleteBook();

  // For placeholder variant: use same priority logic as backend's selectCoverFile
  const placeholderVariant = getCoverFileType(book.files, coverAspectRatio);

  const handleCoverLoad = () => {
    markCoverLoaded(coverUrl);
    setCoverLoaded(true);
  };

  const handleRescan = async (mode: RescanMode) => {
    try {
      await resyncBookMutation.mutateAsync({
        bookId: book.id,
        payload: { mode },
      });
      toast.success("Book rescanned");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to rescan book",
      );
    }
  };

  const handleDeleteBook = async () => {
    try {
      await deleteBookMutation.mutateAsync(book.id);
      toast.success("Book deleted");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to delete book",
      );
    }
  };

  const handleClick = (e: React.MouseEvent) => {
    if (isSelectionMode) {
      e.preventDefault();
      e.stopPropagation();
      if (e.shiftKey && onShiftSelect) {
        onShiftSelect();
      } else if (onSelect) {
        onSelect();
      }
    }
  };

  return (
    <div
      className={cn(
        "w-[calc(50%-0.5rem)] group/card relative",
        COVER_WIDTH_CLASS[gallerySize],
        isSelectionMode && "cursor-pointer",
      )}
      key={book.id}
      onClick={handleClick}
    >
      {/* Selection checkbox overlay */}
      {isSelectionMode && (
        <div
          className={cn(
            "absolute top-1 left-1 z-10 h-6 w-6 rounded-full border-2 flex items-center justify-center transition-all",
            isSelected
              ? "bg-primary border-primary"
              : "bg-black/50 border-white/50",
          )}
        >
          {isSelected && <Check className="h-4 w-4 text-white" />}
        </div>
      )}
      {/* Context menu buttons - shows on hover (hidden in selection mode) */}
      {!isSelectionMode && (
        <div className="absolute top-1 right-1 z-10 opacity-0 group-hover/card:opacity-100 transition-opacity flex gap-1">
          <AddToListPopover
            bookId={book.id}
            trigger={
              <Button
                className="h-7 w-7 bg-black/50 hover:bg-black/70"
                size="icon"
                title="Add to list"
                variant="ghost"
              >
                <List className="h-4 w-4 text-white" />
              </Button>
            }
          />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                className="h-7 w-7 bg-black/50 hover:bg-black/70"
                size="icon"
                variant="ghost"
              >
                <MoreVertical className="h-4 w-4 text-white" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              align="end"
              onCloseAutoFocus={(e) => e.preventDefault()}
            >
              <DropdownMenuItem
                disabled={resyncBookMutation.isPending}
                onClick={() => setShowRescanDialog(true)}
              >
                <RefreshCw className="h-4 w-4 mr-2" />
                Rescan book
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setShowIdentifyDialog(true)}>
                <Search className="h-4 w-4 mr-2" />
                Identify book
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-destructive focus:text-destructive"
                onClick={() => setShowDeleteDialog(true)}
              >
                <Trash2 className="h-4 w-4 mr-2 text-destructive" />
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}
      <Link
        className={cn(
          "group",
          isSelectionMode ? "pointer-events-none" : "cursor-pointer",
        )}
        onClick={(e) => {
          if (isSelectionMode) {
            e.preventDefault();
          }
        }}
        to={`/libraries/${libraryId}/books/${book.id}`}
      >
        <div className={cn("relative", aspectClass)}>
          {/* Placeholder shown until image loads or on error */}
          {(!coverLoaded || coverError) && (
            <CoverPlaceholder
              className={cn(
                "absolute inset-0 rounded-sm border border-neutral-300 dark:border-neutral-600",
                isSelected && "ring-2 ring-primary ring-offset-1",
              )}
              variant={placeholderVariant}
            />
          )}
          {/* Image hidden until loaded, removed on error */}
          {!coverError && (
            <img
              alt={`${book.title} Cover`}
              className={cn(
                "w-full h-full object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1",
                !coverLoaded && "opacity-0",
                isSelected && "ring-2 ring-primary ring-offset-1",
              )}
              onError={() => setCoverError(true)}
              onLoad={handleCoverLoad}
              src={coverUrl}
            />
          )}
          {/* Needs review badge */}
          {isBookNeedsReview(book) && (
            <Badge
              className="absolute bottom-1 left-1 text-[10px] px-1.5 py-0 pointer-events-none"
              variant="secondary"
            >
              Needs review
            </Badge>
          )}
        </div>
        <Tooltip
          delayDuration={300}
          open={isTitleTruncated ? undefined : false}
        >
          <TooltipTrigger asChild>
            <div
              className={cn(
                "mt-2 group-hover:underline text-sm font-bold line-clamp-2",
                seriesNumber && "leading-[1.6]",
              )}
              ref={titleRef}
            >
              {seriesNumber && (
                <span className="inline-flex items-center justify-center align-text-top min-w-5 h-[18px] px-[5px] bg-primary text-primary-foreground rounded text-[11px] font-extrabold tabular-nums tracking-tight mr-1.5">
                  {formatSeriesNumber(
                    seriesNumber,
                    seriesNumberUnit,
                    primaryFileType,
                  )}
                </span>
              )}
              {book.title}
            </div>
          </TooltipTrigger>
          <TooltipContent>{book.title}</TooltipContent>
        </Tooltip>
      </Link>
      {book.authors &&
        book.authors.length > 0 &&
        (() => {
          const hasCBZFiles = book.files?.some(
            (f) => f.file_type === FileTypeCBZ,
          );

          // For CBZ files, only show Writer and Penciller roles, deduplicated by name
          const displayAuthors = hasCBZFiles
            ? book.authors.filter(
                (a) =>
                  a.role === AuthorRoleWriter ||
                  a.role === AuthorRolePenciller ||
                  !a.role,
              )
            : book.authors;

          // Dedupe by name, keeping the first person_id for linking
          const seen = new Set<string>();
          const uniqueAuthors: {
            name: string;
            personId: number;
            hasPerson: boolean;
          }[] = [];
          for (const a of displayAuthors) {
            const name = a.person?.name ?? "Unknown";
            if (seen.has(name)) continue;
            seen.add(name);
            uniqueAuthors.push({
              name,
              personId: a.person_id,
              hasPerson: !!a.person,
            });
          }

          if (uniqueAuthors.length === 0) return null;

          return (
            <div className="mt-1 text-xs line-clamp-2 text-neutral-500 dark:text-neutral-500">
              {uniqueAuthors.map((a, i) => (
                <span key={a.name}>
                  {i > 0 && ", "}
                  {a.hasPerson ? (
                    <Link
                      className={cn(
                        "hover:underline hover:text-foreground",
                        isSelectionMode && "pointer-events-none",
                      )}
                      to={`/libraries/${libraryId}/people/${a.personId}`}
                    >
                      {a.name}
                    </Link>
                  ) : (
                    a.name
                  )}
                </span>
              ))}
            </div>
          );
        })()}
      {book.files && (
        <div className="mt-2 flex gap-2 text-xs">
          {uniqBy(book.files, "file_type").map((f) => (
            <Badge className="text-xs uppercase" key={f.id} variant="secondary">
              {f.file_type}
            </Badge>
          ))}
        </div>
      )}
      {addedByUsername && (
        <div className="mt-1 text-xs text-muted-foreground">
          Added by {addedByUsername}
        </div>
      )}
      <RescanDialog
        entityName={book.title}
        entityType="book"
        isPending={resyncBookMutation.isPending}
        onConfirm={handleRescan}
        onOpenChange={setShowRescanDialog}
        open={showRescanDialog}
      />
      <DeleteConfirmationDialog
        files={book.files}
        isPending={deleteBookMutation.isPending}
        onConfirm={handleDeleteBook}
        onOpenChange={setShowDeleteDialog}
        open={showDeleteDialog}
        title={book.title}
        variant="book"
      />
      <IdentifyBookDialog
        book={book}
        onOpenChange={setShowIdentifyDialog}
        open={showIdentifyDialog}
      />
    </div>
  );
};

export default BookItem;
