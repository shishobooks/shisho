import {
  CheckCircle,
  ChevronUp,
  Circle,
  Download,
  GitMerge,
  List,
  Loader2,
  MoreHorizontal,
  Plus,
  Trash2,
  X,
} from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { CreateListDialog } from "@/components/library/CreateListDialog";
import { DeleteConfirmationDialog } from "@/components/library/DeleteConfirmationDialog";
import { MergeBooksDialog } from "@/components/library/MergeBooksDialog";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useBooks, useDeleteBooks } from "@/hooks/queries/books";
import { useCreateJob } from "@/hooks/queries/jobs";
import {
  useAddBooksToList,
  useCreateList,
  useListLists,
} from "@/hooks/queries/lists";
import { useBulkSetReview } from "@/hooks/queries/review";
import { useBulkDownload } from "@/hooks/useBulkDownload";
import { useBulkSelection } from "@/hooks/useBulkSelection";
import type { CreateListPayload, Library, ReviewOverride } from "@/types";
import { formatFileSize } from "@/utils/format";

interface SelectionToolbarProps {
  library?: Library;
}

export const SelectionToolbar = ({ library }: SelectionToolbarProps) => {
  const { selectedBookIds, exitSelectionMode, clearSelection } =
    useBulkSelection();
  const [popoverOpen, setPopoverOpen] = useState(false);
  const [morePopoverOpen, setMorePopoverOpen] = useState(false);
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

  // Fetch all selected books (not just current page) for download info and delete dialog
  const allSelectedBooksQuery = useBooks(
    { ids: selectedBookIds },
    { enabled: selectedBookIds.length > 0 },
  );

  const downloadInfo = useMemo(() => {
    const allBooks = allSelectedBooksQuery.data?.books;
    if (!allBooks || selectedBookIds.length === 0) return null;

    const fileIds: number[] = [];
    let totalSize = 0;

    for (const bookId of selectedBookIds) {
      const book = allBooks.find((b) => b.id === bookId);
      if (!book?.primary_file_id) continue;
      const primaryFile = book.files?.find(
        (f) => f.id === book.primary_file_id,
      );
      if (primaryFile) {
        fileIds.push(primaryFile.id);
        totalSize += primaryFile.filesize_bytes ?? 0;
      }
    }

    return { fileIds, totalSize };
  }, [allSelectedBooksQuery.data?.books, selectedBookIds]);

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
      const message =
        error instanceof Error ? error.message : "Failed to start download";
      toast.error(message);
    }
  };

  const lists = listsQuery.data?.lists ?? [];
  const editableLists = lists.filter((list) => list.permission !== "viewer");

  const handleAddToList = async (listId: number, listName: string) => {
    setAddingToListId(listId);
    try {
      await addToListMutation.mutateAsync({
        listId,
        payload: { book_ids: selectedBookIds },
      });
      const count = selectedBookIds.length;
      toast.success(
        `Added ${count} book${count !== 1 ? "s" : ""} to "${listName}"`,
      );
      setPopoverOpen(false);
      setActionsPopoverOpen(false);
      exitSelectionMode();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to add books to list";
      toast.error(message);
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
      const count = selectedBookIds.length;
      toast.success(
        `Added ${count} book${count !== 1 ? "s" : ""} to "${payload.name}"`,
      );
      exitSelectionMode();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to create list";
      toast.error(message);
      throw error; // Re-throw so CreateListDialog knows it failed
    }
  };

  const handleBulkReview = async (override: ReviewOverride) => {
    try {
      await bulkSetReviewMutation.mutateAsync({
        bookIds: selectedBookIds,
        override,
      });
      const count = selectedBookIds.length;
      const label = override === "reviewed" ? "reviewed" : "needs review";
      toast.success(
        `Marked ${count} book${count !== 1 ? "s" : ""} as ${label}`,
      );
      exitSelectionMode();
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : "Failed to update review state";
      toast.error(message);
    }
  };

  const handleDeleteBooks = async () => {
    try {
      const result = await deleteBooksMutation.mutateAsync({
        book_ids: selectedBookIds,
      });
      toast.success(
        `Deleted ${result.books_deleted} book${result.books_deleted !== 1 ? "s" : ""}`,
      );
      setShowDeleteDialog(false);
      exitSelectionMode();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to delete books";
      toast.error(message);
    }
  };

  if (selectedBookIds.length === 0) {
    return null;
  }

  const bookLabel =
    selectedBookIds.length === 1 ? `1 book` : `${selectedBookIds.length} books`;

  const addToListContent = (
    <>
      <p className="text-xs font-medium text-muted-foreground px-3 py-2">
        Add to List
      </p>
      {listsQuery.isLoading && (
        <div className="flex items-center justify-center py-4">
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        </div>
      )}
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
              <button
                className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-accent text-left w-full text-sm cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                disabled={isAdding}
                key={list.id}
                onClick={() => handleAddToList(list.id, list.name)}
                type="button"
              >
                {isAdding ? (
                  <Loader2 className="h-4 w-4 animate-spin shrink-0" />
                ) : (
                  <List className="h-4 w-4 shrink-0 text-muted-foreground" />
                )}
                <span className="truncate">{list.name}</span>
              </button>
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
    <div className="fixed bottom-4 left-1/2 transform -translate-x-1/2 z-50 bg-background border rounded-lg shadow-lg px-3 py-2 flex items-center gap-3 max-w-[calc(100vw-2rem)]">
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

          <div className="border-t p-1">
            <Button
              className="w-full justify-start gap-2 font-normal"
              disabled={
                !downloadInfo ||
                downloadInfo.fileIds.length === 0 ||
                createJobMutation.isPending
              }
              onClick={() => {
                setActionsPopoverOpen(false);
                handleDownload();
              }}
              size="sm"
              variant="ghost"
            >
              {createJobMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin shrink-0" />
              ) : (
                <Download className="h-4 w-4 shrink-0 text-muted-foreground" />
              )}
              <span className="truncate">
                Download
                {downloadInfo && downloadInfo.totalSize > 0 && (
                  <span className="text-xs opacity-75 ml-1">
                    ({formatFileSize(downloadInfo.totalSize)})
                  </span>
                )}
              </span>
            </Button>

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

        <Button
          disabled={
            !downloadInfo ||
            downloadInfo.fileIds.length === 0 ||
            createJobMutation.isPending
          }
          onClick={handleDownload}
          size="sm"
          variant="default"
        >
          {createJobMutation.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Download className="h-4 w-4" />
          )}
          Download
          {downloadInfo && downloadInfo.totalSize > 0 && (
            <span className="text-xs opacity-75">
              ({formatFileSize(downloadInfo.totalSize)})
            </span>
          )}
        </Button>

        {selectedBookIds.length >= 2 && library && (
          <Button
            onClick={() => setShowMergeDialog(true)}
            size="sm"
            variant="default"
          >
            <GitMerge className="h-4 w-4" />
            Merge
          </Button>
        )}

        <Button
          onClick={() => setShowDeleteDialog(true)}
          size="sm"
          variant="destructive"
        >
          <Trash2 className="h-4 w-4" />
          Delete
        </Button>

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
      </div>

      <Button onClick={clearSelection} size="sm" variant="ghost">
        Clear
      </Button>

      <Button
        className="h-8 w-8"
        onClick={exitSelectionMode}
        size="icon"
        variant="ghost"
      >
        <X className="h-4 w-4" />
      </Button>

      {showMergeDialog && library && (
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

      <DeleteConfirmationDialog
        books={allSelectedBooksQuery.data?.books?.map((b) => ({
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
    </div>
  );
};
