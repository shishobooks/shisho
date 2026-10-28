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
import CoverImage from "@/components/library/CoverImage";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { DeleteConfirmationDialog } from "@/components/library/DeleteConfirmationDialog";
import { IdentifyBookDialog } from "@/components/library/IdentifyBookDialog";
import { RescanDialog } from "@/components/library/RescanDialog";
import TruncatedText from "@/components/library/TruncatedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useDeleteBook, useResyncBook } from "@/hooks/queries/books";
import { useCan } from "@/hooks/useCan";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import {
  AuthorRolePenciller,
  AuthorRoleWriter,
  FileTypeCBZ,
  type Book,
  type File,
  type ResyncMode,
} from "@/types";
import { isBookNeedsReview } from "@/utils/book";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";
import { getCoverFileType, selectCoverFile } from "@/utils/coverSelection";
import { bookCoverUrl } from "@/utils/coverUrl";
import { fileTypeLabel } from "@/utils/format";
import { hasAnyCBZFile } from "@/utils/hasAnyCBZFile";
import { readingStatusLabel } from "@/utils/readingStatus";
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
}

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
  // Author pages need People Read; without it the names are plain text.
  const linkPeople = useCan("people:read");
  // Rescan, identify, and delete all require Books Write on the backend.
  // Adding to a list is governed by the list's own permission, so it stays.
  const canWriteBooks = useCan("books:write");

  // Find the series number and unit for the specific series context (if provided)
  const seriesEntry = seriesId
    ? book.book_series?.find((bs) => bs.series_id === seriesId)
    : undefined;
  const seriesNumber = seriesEntry?.series_number;
  const seriesNumberEnd = seriesEntry?.series_number_end;
  const seriesNumberUnit = seriesEntry?.series_number_unit;
  const anyCBZ = hasAnyCBZFile(book);

  const aspectClass = getAspectRatioClass(coverAspectRatio, book.files);
  const coverUrl = bookCoverUrl(book);
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

  const handleRescan = async (mode: ResyncMode) => {
    try {
      await resyncBookMutation.mutateAsync({
        bookId: book.id,
        payload: { mode },
      });
      toast.success("Book rescanned");
    } catch (error) {
      toastRequestError(error, "Failed to rescan book");
    }
  };

  const handleDeleteBook = async () => {
    try {
      await deleteBookMutation.mutateAsync(book.id);
      toast.success("Book deleted");
    } catch (error) {
      toastRequestError(error, "Failed to delete book");
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
        "w-full group/card relative",
        isSelectionMode && "cursor-pointer",
      )}
      key={book.id}
      onClick={handleClick}
    >
      {/* Selection checkbox overlay. It has no handler of its own: its
          click, from a pointer or the keyboard, bubbles to the card's. */}
      {isSelectionMode && (
        <Button
          aria-label={`Select ${book.title}`}
          aria-pressed={isSelected}
          className={cn(
            "absolute top-1 left-1 z-10 h-6 w-6 rounded-full border-2 flex items-center justify-center transition-all",
            isSelected
              ? "bg-primary border-primary"
              : "bg-black/50 border-white/50",
          )}
          variant="unstyled"
        >
          {isSelected && <Check className="h-4 w-4 text-white" />}
        </Button>
      )}
      {/* Context menu buttons - shows on hover (hidden in selection mode) */}
      {!isSelectionMode && (
        <div className="absolute top-1 right-1 z-10 opacity-0 group-hover/card:opacity-100 transition-opacity flex gap-1">
          <AddToListPopover
            bookId={book.id}
            trigger={
              <Button
                aria-label="Add to list"
                className="h-7 w-7 bg-black/50 hover:bg-black/70"
                size="icon"
                title="Add to list"
                variant="ghost"
              >
                <List className="h-4 w-4 text-white" />
              </Button>
            }
          />
          {canWriteBooks && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  aria-label="Book actions"
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
          )}
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
        // In selection mode the overlay button is the card's one tab stop.
        tabIndex={isSelectionMode ? -1 : undefined}
        to={`/libraries/${libraryId}/books/${book.id}`}
      >
        <div className={cn("relative", aspectClass)}>
          {/* Placeholder shown until image loads or on error */}
          {(!coverLoaded || coverError) && (
            <CoverPlaceholder
              className={cn(
                "absolute inset-0 rounded-sm border border-border",
                isSelected && "ring-2 ring-primary ring-offset-1",
              )}
              variant={placeholderVariant}
            />
          )}
          {/* Image hidden until loaded, removed on error */}
          {!coverError && (
            <CoverImage
              alt={`${book.title} Cover`}
              className={cn(
                "w-full h-full object-cover rounded-sm border-border border-1",
                !coverLoaded && "opacity-0",
                isSelected && "ring-2 ring-primary ring-offset-1",
              )}
              onError={() => setCoverError(true)}
              onLoad={handleCoverLoad}
              src={coverUrl}
            />
          )}
          {/* Badges stack in the bottom-left corner so a long status and
              Needs review never overlap on a narrow card. */}
          {(isBookNeedsReview(book) || book.reading_status) && (
            <div className="absolute bottom-1 left-1 right-1 flex flex-col items-start gap-0.5 pointer-events-none">
              {book.reading_status && (
                <Badge className="text-[10px] px-1.5 py-0" variant="secondary">
                  {readingStatusLabel(book.reading_status)}
                </Badge>
              )}
              {isBookNeedsReview(book) && (
                <Badge className="text-[10px] px-1.5 py-0" variant="secondary">
                  Needs review
                </Badge>
              )}
            </div>
          )}
        </div>
        <TruncatedText
          className={cn(
            "mt-2 group-hover:underline text-sm font-bold line-clamp-2",
            seriesNumber != null && "leading-[1.6]",
          )}
          tooltip={book.title}
        >
          {seriesNumber != null && (
            <span className="inline-flex items-center justify-center align-text-top min-w-5 h-[18px] px-[5px] bg-primary text-primary-foreground rounded text-[11px] font-extrabold tabular-nums tracking-tight mr-1.5">
              {formatSeriesNumber(
                seriesNumber,
                seriesNumberEnd,
                seriesNumberUnit,
                anyCBZ ? "cbz" : null,
              )}
            </span>
          )}
          {book.title}
        </TruncatedText>
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
            <div className="mt-1 text-xs line-clamp-2 text-muted-foreground">
              {uniqueAuthors.map((a, i) => (
                <span key={a.personId}>
                  {a.hasPerson && linkPeople ? (
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
                  {i < uniqueAuthors.length - 1 && ", "}
                </span>
              ))}
            </div>
          );
        })()}
      {book.files && (
        <div className="mt-2 flex flex-wrap gap-2 text-xs">
          {uniqBy(book.files, "file_type").map((f) => (
            <Badge className="text-xs" key={f.id} variant="secondary">
              {fileTypeLabel(f.file_type)}
            </Badge>
          ))}
        </div>
      )}
      {addedByUsername && (
        <div className="mt-1 text-xs text-muted-foreground">
          Added by {addedByUsername}
        </div>
      )}
      {canWriteBooks && (
        <>
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
        </>
      )}
    </div>
  );
};

export default BookItem;
