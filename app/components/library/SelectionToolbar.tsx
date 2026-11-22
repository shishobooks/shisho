import {
  BookCheck,
  BookDashed,
  BookOpen,
  BookX,
  CheckCircle,
  ChevronUp,
  Circle,
  Download,
  GitMerge,
  Library,
  List,
  Loader2,
  MoreHorizontal,
  Plus,
  Trash2,
  X,
} from "lucide-react";
import { useEffect, useId, useMemo, useState } from "react";
import { toast } from "sonner";

import { CreateListDialog } from "@/components/library/CreateListDialog";
import { DeleteConfirmationDialog } from "@/components/library/DeleteConfirmationDialog";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MergeBooksDialog } from "@/components/library/MergeBooksDialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { FILE_TYPE_OPTIONS } from "@/constants/fileTypes";
import { useBooks, useDeleteBooks } from "@/hooks/queries/books";
import { useCreateJob } from "@/hooks/queries/jobs";
import {
  useAddBooksToList,
  useCreateList,
  useListLists,
} from "@/hooks/queries/lists";
import {
  useBulkReadingChange,
  type BulkReadingAction,
} from "@/hooks/queries/reading";
import { useBulkSetReview } from "@/hooks/queries/review";
import { useAuth } from "@/hooks/useAuth";
import { useBulkDownload } from "@/hooks/useBulkDownload";
import { useBulkSelection } from "@/hooks/useBulkSelection";
import { useCan } from "@/hooks/useCan";
import { toastRequestError } from "@/libraries/api";
import {
  type CreateListPayload,
  type FileType,
  type LibrarySummary,
  type ReviewOverride,
} from "@/types";
import {
  collectDownloadFiles,
  getAvailableFileTypes,
} from "@/utils/downloadFiles";
import { formatCount, formatFileSize } from "@/utils/format";

// The most books one list request returns (ListBooksQuery in
// pkg/books/types.go).
const MAX_BOOKS_PAGE = 100;

interface SelectionToolbarProps {
  library?: LibrarySummary;
}

export const SelectionToolbar = ({ library }: SelectionToolbarProps) => {
  const { demoMode } = useAuth();
  // Merge, delete, and review changes require Books Write. Downloads and
  // list membership are governed separately and stay available.
  const canWriteBooks = useCan("books:write");
  // Reading Status is the user's own, so the reading routes need only
  // Books Read.
  const canTrackReading = useCan("books:read");
  const { selectedBookIds, exitSelectionMode, clearSelection } =
    useBulkSelection();
  const [popoverOpen, setPopoverOpen] = useState(false);
  const fileTypesLabelId = useId();
  const [morePopoverOpen, setMorePopoverOpen] = useState(false);
  const [readingPopoverOpen, setReadingPopoverOpen] = useState(false);
  const [confirmingNotStarted, setConfirmingNotStarted] = useState(false);
  const [confirmingReference, setConfirmingReference] = useState(false);
  const [actionsPopoverOpen, setActionsPopoverOpen] = useState(false);
  const [addingToListId, setAddingToListId] = useState<number | null>(null);
  const [showMergeDialog, setShowMergeDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  const { startDownload } = useBulkDownload();
  const createJobMutation = useCreateJob();

  const listsQuery = useListLists();
  const addToListMutation = useAddBooksToList();
  const createListMutation = useCreateList();
  const deleteBooksMutation = useDeleteBooks();
  const bulkSetReviewMutation = useBulkSetReview();
  const bulkReadingMutation = useBulkReadingChange();

  const [downloadPopoverOpen, setDownloadPopoverOpen] = useState(false);
  const [selectedDownloadTypes, setSelectedDownloadTypes] = useState<string[]>(
    [],
  );

  // Fetch all selected books (not just current page) for download info and
  // delete dialog, up to the most one page of books holds.
  const allSelectedBooksQuery = useBooks(
    {
      ids: selectedBookIds,
      limit: Math.min(Math.max(selectedBookIds.length, 1), MAX_BOOKS_PAGE),
    },
    { enabled: selectedBookIds.length > 0 },
  );

  const allBooks = useMemo(
    () => allSelectedBooksQuery.data?.items ?? [],
    [allSelectedBooksQuery.data?.items],
  );

  // Distinct file types available across main files of selected books.
  // Sorted and joined into a stable key so the effect below doesn't loop
  // when upstream hooks return fresh array references with identical contents.
  const availableFileTypes = useMemo(
    () => getAvailableFileTypes(allBooks, selectedBookIds).sort(),
    [allBooks, selectedBookIds],
  );
  const availableFileTypesKey = availableFileTypes.join(",");

  // Auto-select all available types when the set of available types changes
  useEffect(() => {
    setSelectedDownloadTypes(availableFileTypesKey.split(",").filter(Boolean));
  }, [availableFileTypesKey]);

  const downloadInfo = useMemo(() => {
    if (allBooks.length === 0 || selectedBookIds.length === 0) return null;
    return collectDownloadFiles(
      allBooks,
      selectedBookIds,
      selectedDownloadTypes,
    );
  }, [allBooks, selectedBookIds, selectedDownloadTypes]);

  const toggleDownloadType = (fileType: string) => {
    setSelectedDownloadTypes((prev) =>
      prev.includes(fileType)
        ? prev.filter((t) => t !== fileType)
        : [...prev, fileType],
    );
  };

  const handleDownload = async () => {
    if (!downloadInfo || downloadInfo.fileIds.length === 0) return;

    try {
      const job = await createJobMutation.mutateAsync({
        payload: {
          type: "bulk_download",
          data: {
            file_ids: downloadInfo.fileIds,
            estimated_size_bytes: downloadInfo.totalSize,
          },
        },
      });
      startDownload(
        job.id,
        downloadInfo.fileIds.length,
        downloadInfo.totalSize,
      );
      exitSelectionMode();
    } catch (error) {
      toastRequestError(error, "Failed to start download");
    }
  };

  const lists = listsQuery.data?.items ?? [];
  const editableLists = lists.filter((list) => list.permission !== "viewer");

  const handleAddToList = async (listId: number, listName: string) => {
    setAddingToListId(listId);
    try {
      await addToListMutation.mutateAsync({
        listId,
        payload: { book_ids: selectedBookIds },
      });
      toast.success(
        `Added ${formatCount(selectedBookIds.length, "book")} to "${listName}"`,
      );
      setPopoverOpen(false);
      setActionsPopoverOpen(false);
      exitSelectionMode();
    } catch (error) {
      toastRequestError(error, "Failed to add books to list");
    } finally {
      setAddingToListId(null);
    }
  };

  const handleCreateList = () => {
    setPopoverOpen(false);
    setActionsPopoverOpen(false);
    setCreateDialogOpen(true);
  };

  const handleCreate = async (payload: CreateListPayload) => {
    try {
      const newList = await createListMutation.mutateAsync(payload);
      toast.success(`Created "${payload.name}" list`);
      // Automatically add the selected books to the newly created list
      await addToListMutation.mutateAsync({
        listId: newList.id,
        payload: { book_ids: selectedBookIds },
      });
      toast.success(
        `Added ${formatCount(selectedBookIds.length, "book")} to "${payload.name}"`,
      );
      exitSelectionMode();
    } catch (error) {
      toastRequestError(error, "Failed to create list");
      throw error; // Re-throw so CreateListDialog knows it failed
    }
  };

  const handleBulkReview = async (override: ReviewOverride) => {
    try {
      await bulkSetReviewMutation.mutateAsync({
        bookIds: selectedBookIds,
        override,
      });
      const label = override === "reviewed" ? "reviewed" : "needs review";
      toast.success(
        `Marked ${formatCount(selectedBookIds.length, "book")} as ${label}`,
      );
      exitSelectionMode();
    } catch (error) {
      toastRequestError(error, "Failed to update review status");
    }
  };

  // What each bulk reading change says it did, and what it says on failure.
  const bulkReadingMessages: Record<
    BulkReadingAction,
    { done: (books: string) => string; failed: string }
  > = {
    finish: {
      done: (books) => `Marked ${books} as read`,
      failed: "Failed to mark books as read",
    },
    "not-started": {
      done: (books) => `Marked ${books} as not started`,
      failed: "Failed to mark books as not started",
    },
    reference: {
      done: (books) => `Marked ${books} as Reference`,
      failed: "Failed to mark books as Reference",
    },
    "remove-reference": {
      done: (books) => `Removed ${books} from Reference`,
      failed: "Failed to remove books from Reference",
    },
  };

  const handleBulkReading = async (action: BulkReadingAction) => {
    const messages = bulkReadingMessages[action];
    try {
      const result = await bulkReadingMutation.mutateAsync({
        action,
        bookIds: selectedBookIds,
      });
      const skipped =
        result.skipped > 0
          ? `, ${result.skipped.toLocaleString()} skipped`
          : "";
      toast.success(
        `${messages.done(formatCount(result.changed, "book"))}${skipped}`,
      );
      setConfirmingNotStarted(false);
      setConfirmingReference(false);
      exitSelectionMode();
    } catch (error) {
      toastRequestError(error, messages.failed);
    }
  };

  const handleDeleteBooks = async () => {
    try {
      const result = await deleteBooksMutation.mutateAsync({
        book_ids: selectedBookIds,
      });
      toast.success(`Deleted ${formatCount(result.books_deleted, "book")}`);
      setShowDeleteDialog(false);
      exitSelectionMode();
    } catch (error) {
      toastRequestError(error, "Failed to delete books");
    }
  };

  if (selectedBookIds.length === 0) {
    return null;
  }

  const bookLabel = formatCount(selectedBookIds.length, "book");

  // Marking not started deletes each read underway, so its confirmation
  // says how many of the selected books have one, once they've loaded.
  const underwayCount = allBooks.filter(
    (book) =>
      selectedBookIds.includes(book.id) &&
      (book.reading_status === "currently_reading" ||
        book.reading_status === "paused"),
  ).length;
  // Known only when every selected book loaded: a selection can be larger
  // than one page of books, and a selection that just changed may still
  // show the previous one's books.
  const underwayKnown =
    !allSelectedBooksQuery.isPlaceholderData &&
    selectedBookIds.every((id) => allBooks.some((book) => book.id === id));
  const readsUnderway =
    !underwayKnown || underwayCount === 1
      ? `The read underway on ${underwayKnown ? "1 book" : "each selected book"} is deleted`
      : `The reads underway on ${formatCount(underwayCount, "book")} are deleted`;
  // "it" for one read, or when each book has its own, "them" for several.
  const duringReads =
    !underwayKnown || underwayCount === 1 ? "during it" : "during them";
  const notStartedDescription = `${readsUnderway}, along with the reading time and places recorded ${duringReads}, and the books' main files open at the start again. A book that left To Read when it was started goes back to its place there. Books that aren't being read are skipped.`;
  // Marking Reference deletes each Read-through underway but keeps its time,
  // so its confirmation counts them the same way.
  const referenceReadThroughsDeleted =
    !underwayKnown || underwayCount === 1
      ? `The current Read-through of ${underwayKnown ? "1 book" : "each selected book"} is deleted from your reading history`
      : `The current Read-throughs of ${formatCount(underwayCount, "book")} are deleted from your reading history`;
  const referenceDescription = `${referenceReadThroughsDeleted}, and this can't be undone. The reading time recorded ${duringReads} still counts toward your Reading Stats, and every file keeps its place. The books leave To Read, and opening them never starts or finishes a Read-through while they're Reference.`;

  // The reading actions, for the phone menu and the desktop popover. onPick
  // closes whichever holds them.
  const readingActions = (onPick: () => void) => (
    <>
      <Button
        className="w-full justify-start gap-2 font-normal"
        disabled={bulkReadingMutation.isPending}
        onClick={() => {
          onPick();
          handleBulkReading("finish");
        }}
        size="sm"
        variant="ghost"
      >
        <BookCheck className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="truncate">Mark as read</span>
      </Button>
      <Button
        className="w-full justify-start gap-2 font-normal"
        disabled={bulkReadingMutation.isPending}
        onClick={() => {
          onPick();
          // With nothing being read, there's nothing to confirm.
          if (underwayKnown && underwayCount === 0) {
            toast.info("None of the selected books is being read.");
            return;
          }
          setConfirmingNotStarted(true);
        }}
        size="sm"
        variant="ghost"
      >
        <BookDashed className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="truncate">Mark as not started</span>
      </Button>
      <Button
        className="w-full justify-start gap-2 font-normal"
        disabled={bulkReadingMutation.isPending}
        onClick={() => {
          onPick();
          // With nothing being read, nothing leaves the history to confirm.
          if (underwayKnown && underwayCount === 0) {
            handleBulkReading("reference");
            return;
          }
          setConfirmingReference(true);
        }}
        size="sm"
        variant="ghost"
      >
        <Library className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="truncate">Mark as Reference</span>
      </Button>
      <Button
        className="w-full justify-start gap-2 font-normal"
        disabled={bulkReadingMutation.isPending}
        onClick={() => {
          onPick();
          handleBulkReading("remove-reference");
        }}
        size="sm"
        variant="ghost"
      >
        <BookX className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="truncate">Remove from Reference</span>
      </Button>
    </>
  );

  const downloadContent = (onDownloadClick: () => void) => (
    <div className="p-3 space-y-3">
      <p
        className="text-xs font-medium text-muted-foreground"
        id={fileTypesLabelId}
      >
        File types to include
      </p>
      <div
        aria-labelledby={fileTypesLabelId}
        className="space-y-2"
        role="group"
      >
        {FILE_TYPE_OPTIONS.map((option) => {
          const isAvailable = availableFileTypes.includes(
            option.value as FileType,
          );
          const isChecked =
            isAvailable && selectedDownloadTypes.includes(option.value);
          return (
            <Label
              className="flex items-center gap-2 text-sm cursor-pointer"
              key={option.value}
              variant="unstyled"
            >
              <Checkbox
                checked={isChecked}
                disabled={!isAvailable}
                onCheckedChange={() => toggleDownloadType(option.value)}
              />
              <span
                className={
                  isAvailable ? "" : "text-muted-foreground opacity-50"
                }
              >
                {option.label}
              </span>
            </Label>
          );
        })}
      </div>
      {downloadInfo && downloadInfo.totalSize > 0 && (
        <p className="text-xs text-muted-foreground" role="status">
          {formatCount(downloadInfo.fileIds.length, "file")} &middot;{" "}
          {formatFileSize(downloadInfo.totalSize)}
        </p>
      )}
      <Button
        className="w-full"
        disabled={
          !downloadInfo ||
          downloadInfo.fileIds.length === 0 ||
          createJobMutation.isPending
        }
        onClick={onDownloadClick}
        size="sm"
      >
        {createJobMutation.isPending ? (
          <Loader2 className="h-4 w-4 animate-spin" />
        ) : (
          <Download className="h-4 w-4" />
        )}
        Download
      </Button>
    </div>
  );

  const addToListContent = (
    <>
      <p className="text-xs font-medium text-muted-foreground px-3 py-2">
        Add to List
      </p>
      {listsQuery.isLoading && <LoadingSpinner className="py-3" />}
      {!listsQuery.isLoading && editableLists.length === 0 && (
        <p className="text-sm text-muted-foreground px-3 py-3 text-center">
          No editable lists
        </p>
      )}
      {!listsQuery.isLoading && editableLists.length > 0 && (
        <div className="flex flex-col gap-0.5 px-1 pb-1">
          {editableLists.map((list) => {
            const isAdding = addingToListId === list.id;
            return (
              <Button
                className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-accent text-left w-full text-sm"
                disabled={isAdding}
                key={list.id}
                onClick={() => handleAddToList(list.id, list.name)}
                variant="unstyled"
              >
                {isAdding ? (
                  <Loader2 className="h-4 w-4 animate-spin shrink-0" />
                ) : (
                  <List className="h-4 w-4 shrink-0 text-muted-foreground" />
                )}
                <span className="truncate">{list.name}</span>
              </Button>
            );
          })}
        </div>
      )}
      {!listsQuery.isLoading && (
        <div className="border-t px-1 py-1">
          <Button
            className="w-full justify-start"
            onClick={handleCreateList}
            size="sm"
            variant="ghost"
          >
            <Plus className="h-4 w-4" />
            Create New List
          </Button>
        </div>
      )}
    </>
  );

  return (
    <div className="fixed bottom-4 left-1/2 transform -translate-x-1/2 z-50 bg-background border rounded-md shadow-lg px-3 py-2 flex items-center gap-3 max-w-[calc(100vw-2rem)]">
      <span className="text-sm font-medium whitespace-nowrap tabular-nums">
        {bookLabel}
      </span>

      {/* Mobile: single Actions popover */}
      <Popover onOpenChange={setActionsPopoverOpen} open={actionsPopoverOpen}>
        <PopoverTrigger asChild>
          <Button className="sm:hidden" size="sm" variant="default">
            Actions
            <ChevronUp className="h-4 w-4" />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="center"
          className="w-56 max-h-[70vh] overflow-y-auto p-0"
          side="top"
        >
          {addToListContent}

          {canTrackReading && (
            <div className="border-t p-1">
              {readingActions(() => setActionsPopoverOpen(false))}
            </div>
          )}

          {canWriteBooks && (
            <div className="border-t p-1">
              <Button
                className="w-full justify-start gap-2 font-normal"
                onClick={() => {
                  setActionsPopoverOpen(false);
                  handleBulkReview("reviewed");
                }}
                size="sm"
                variant="ghost"
              >
                <CheckCircle className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="truncate">Mark reviewed</span>
              </Button>
              <Button
                className="w-full justify-start gap-2 font-normal"
                onClick={() => {
                  setActionsPopoverOpen(false);
                  handleBulkReview("unreviewed");
                }}
                size="sm"
                variant="ghost"
              >
                <Circle className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="truncate">Mark needs review</span>
              </Button>
            </div>
          )}

          {!demoMode && (
            <div className="border-t">
              {downloadContent(() => {
                setActionsPopoverOpen(false);
                handleDownload();
              })}
            </div>
          )}

          {canWriteBooks && (
            <div className="border-t p-1">
              {selectedBookIds.length >= 2 && library && (
                <Button
                  className="w-full justify-start gap-2 font-normal"
                  onClick={() => {
                    setActionsPopoverOpen(false);
                    setShowMergeDialog(true);
                  }}
                  size="sm"
                  variant="ghost"
                >
                  <GitMerge className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="truncate">Merge</span>
                </Button>
              )}

              <Button
                className="w-full justify-start gap-2 font-normal text-destructive hover:text-destructive"
                onClick={() => {
                  setActionsPopoverOpen(false);
                  setShowDeleteDialog(true);
                }}
                size="sm"
                variant="ghost"
              >
                <Trash2 className="h-4 w-4 shrink-0" />
                <span className="truncate">Delete</span>
              </Button>
            </div>
          )}
        </PopoverContent>
      </Popover>

      {/* Desktop: inline action buttons */}
      <div className="hidden sm:flex items-center gap-3">
        <Popover onOpenChange={setPopoverOpen} open={popoverOpen}>
          <PopoverTrigger asChild>
            <Button size="sm" variant="default">
              <List className="h-4 w-4" />
              Add
            </Button>
          </PopoverTrigger>
          <PopoverContent align="center" className="w-56 p-0" side="top">
            {addToListContent}
          </PopoverContent>
        </Popover>

        {canTrackReading && (
          <Popover
            onOpenChange={setReadingPopoverOpen}
            open={readingPopoverOpen}
          >
            <PopoverTrigger asChild>
              <Button size="sm" variant="default">
                <BookOpen className="h-4 w-4" />
                Reading
              </Button>
            </PopoverTrigger>
            <PopoverContent align="center" className="w-56 p-1" side="top">
              {readingActions(() => setReadingPopoverOpen(false))}
            </PopoverContent>
          </Popover>
        )}

        {!demoMode && (
          <Popover
            onOpenChange={setDownloadPopoverOpen}
            open={downloadPopoverOpen}
          >
            <PopoverTrigger asChild>
              <Button size="sm" variant="default">
                <Download className="h-4 w-4" />
                Download
                {downloadInfo && downloadInfo.totalSize > 0 && (
                  <span className="text-xs opacity-75">
                    ({formatFileSize(downloadInfo.totalSize)})
                  </span>
                )}
              </Button>
            </PopoverTrigger>
            <PopoverContent align="center" className="w-56 p-0" side="top">
              {downloadContent(() => {
                setDownloadPopoverOpen(false);
                handleDownload();
              })}
            </PopoverContent>
          </Popover>
        )}

        {canWriteBooks && selectedBookIds.length >= 2 && library && (
          <Button
            onClick={() => setShowMergeDialog(true)}
            size="sm"
            variant="default"
          >
            <GitMerge className="h-4 w-4" />
            Merge
          </Button>
        )}

        {canWriteBooks && (
          <Button
            onClick={() => setShowDeleteDialog(true)}
            size="sm"
            variant="destructive"
          >
            <Trash2 className="h-4 w-4" />
            Delete
          </Button>
        )}

        {canWriteBooks && (
          <Popover onOpenChange={setMorePopoverOpen} open={morePopoverOpen}>
            <PopoverTrigger asChild>
              <Button size="sm" variant="default">
                <MoreHorizontal className="h-4 w-4" />
                More
              </Button>
            </PopoverTrigger>
            <PopoverContent align="center" className="w-56 p-1" side="top">
              <Button
                className="w-full justify-start gap-2 font-normal"
                onClick={() => {
                  setMorePopoverOpen(false);
                  handleBulkReview("reviewed");
                }}
                size="sm"
                variant="ghost"
              >
                <CheckCircle className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="truncate">Mark reviewed</span>
              </Button>
              <Button
                className="w-full justify-start gap-2 font-normal"
                onClick={() => {
                  setMorePopoverOpen(false);
                  handleBulkReview("unreviewed");
                }}
                size="sm"
                variant="ghost"
              >
                <Circle className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="truncate">Mark needs review</span>
              </Button>
            </PopoverContent>
          </Popover>
        )}
      </div>

      <Button onClick={clearSelection} size="sm" variant="ghost">
        Clear
      </Button>

      <Button
        aria-label="Exit selection"
        className="h-8 w-8"
        onClick={exitSelectionMode}
        size="icon"
        variant="ghost"
      >
        <X className="h-4 w-4" />
      </Button>

      {canWriteBooks && showMergeDialog && library && (
        <MergeBooksDialog
          bookIds={selectedBookIds}
          library={library}
          onOpenChange={setShowMergeDialog}
          onSuccess={() => {
            exitSelectionMode();
            setShowMergeDialog(false);
          }}
          open={showMergeDialog}
        />
      )}

      <CreateListDialog
        isPending={createListMutation.isPending}
        onCreate={handleCreate}
        onOpenChange={setCreateDialogOpen}
        open={createDialogOpen}
      />

      {canTrackReading && (
        <ConfirmDialog
          confirmLabel="Mark as not started"
          description={notStartedDescription}
          isPending={bulkReadingMutation.isPending}
          onConfirm={() => handleBulkReading("not-started")}
          onOpenChange={setConfirmingNotStarted}
          open={confirmingNotStarted}
          title={`Mark ${bookLabel} as not started?`}
        />
      )}

      {canTrackReading && (
        <ConfirmDialog
          confirmLabel="Mark as Reference"
          description={referenceDescription}
          isPending={bulkReadingMutation.isPending}
          onConfirm={() => handleBulkReading("reference")}
          onOpenChange={setConfirmingReference}
          open={confirmingReference}
          title={`Mark ${bookLabel} as Reference?`}
        />
      )}

      {canWriteBooks && (
        <DeleteConfirmationDialog
          books={allSelectedBooksQuery.data?.items?.map((b) => ({
            id: b.id,
            title: b.title,
            files: b.files,
          }))}
          isPending={deleteBooksMutation.isPending}
          onConfirm={handleDeleteBooks}
          onOpenChange={setShowDeleteDialog}
          open={showDeleteDialog}
          variant="books"
        />
      )}
    </div>
  );
};
