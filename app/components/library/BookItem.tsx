import { uniqBy } from "lodash";
import { Check, List, MoreVertical, RefreshCw } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import AddToListPopover from "@/components/library/AddToListPopover";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { ResyncConfirmDialog } from "@/components/library/ResyncConfirmDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useResyncBook } from "@/hooks/queries/books";
import { cn } from "@/libraries/utils";
import {
  AuthorRolePenciller,
  AuthorRoleWriter,
  FileTypeCBZ,
  type Book,
  type File,
} from "@/types";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";

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
}

// Selects the file that would be used for the cover based on cover_aspect_ratio setting
// This mirrors the backend's selectCoverFile logic (requires cover_image_path)
const selectCoverFile = (
  files: File[] | undefined,
  coverAspectRatio: string,
): File | null => {
  if (!files) return null;

  const bookFiles = files.filter(
    (f) =>
      (f.file_type === "epub" || f.file_type === "cbz") && f.cover_image_path,
  );
  const audiobookFiles = files.filter(
    (f) => f.file_type === "m4b" && f.cover_image_path,
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
// This mirrors the backend's selectCoverFile priority logic but doesn't require cover_image_path.
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
}: BookItemProps) => {
  // Find the series number for the specific series context (if provided)
  const seriesNumber = seriesId
    ? book.book_series?.find((bs) => bs.series_id === seriesId)?.series_number
    : undefined;

  const aspectClass = getAspectRatioClass(coverAspectRatio, book.files);
  const coverUrl = `/api/books/${book.id}/cover?t=${new Date(book.updated_at).getTime()}`;
  const [coverLoaded, setCoverLoaded] = useState(() => isCoverLoaded(coverUrl));
  const [coverError, setCoverError] = useState(false);
  const [showRefreshDialog, setShowRefreshDialog] = useState(false);
  const resyncBookMutation = useResyncBook();

  // For placeholder variant: use same priority logic as backend's selectCoverFile
  const placeholderVariant = getCoverFileType(book.files, coverAspectRatio);

  const handleCoverLoad = () => {
    markCoverLoaded(coverUrl);
    setCoverLoaded(true);
  };

  const handleScanMetadata = async () => {
    try {
      await resyncBookMutation.mutateAsync({
        bookId: book.id,
        payload: { refresh: false },
      });
      toast.success("Metadata scanned");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to scan metadata",
      );
    }
  };

  const handleRefreshMetadata = async () => {
    try {
      await resyncBookMutation.mutateAsync({
        bookId: book.id,
        payload: { refresh: true },
      });
      toast.success("Metadata refreshed");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to refresh metadata",
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
        "w-[calc(50%-0.5rem)] sm:w-32 group/card relative",
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
                onClick={handleScanMetadata}
              >
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
        </div>
        <div className="mt-2 group-hover:underline font-bold line-clamp-2">
          {book.title}
        </div>
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

          // Get unique author names
          const uniqueNames = [
            ...new Set(displayAuthors.map((a) => a.person?.name ?? "Unknown")),
          ];

          if (uniqueNames.length === 0) return null;

          return (
            <div className="mt-1 text-sm line-clamp-2 text-neutral-500 dark:text-neutral-500">
              {uniqueNames.join(", ")}
            </div>
          );
        })()}
      {book.files && (
        <div className="mt-2 flex gap-2 text-sm">
          {uniqBy(book.files, "file_type").map((f) => (
            <Badge className="uppercase" key={f.id} variant="subtle">
              {f.file_type}
            </Badge>
          ))}
        </div>
      )}
      {seriesNumber && (
        <div className="mt-1">
          <Badge className="text-xs" variant="outline">
            #{seriesNumber}
          </Badge>
        </div>
      )}
      {addedByUsername && (
        <div className="mt-1 text-xs text-muted-foreground">
          Added by {addedByUsername}
        </div>
      )}
      <ResyncConfirmDialog
        entityName={book.title}
        entityType="book"
        isPending={resyncBookMutation.isPending}
        onConfirm={handleRefreshMetadata}
        onOpenChange={setShowRefreshDialog}
        open={showRefreshDialog}
      />
    </div>
  );
};

export default BookItem;
