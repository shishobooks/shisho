import { BookSelectionList } from "./BookSelectionList";
import { AlertTriangle, Info, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { useMoveFiles } from "@/hooks/queries/books";
import type { Book, File, Library } from "@/types";

interface MoveFilesDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sourceBook: Book;
  selectedFiles: File[];
  library: Library;
  onSuccess?: (targetBook: Book) => void;
}

export function MoveFilesDialog({
  open,
  onOpenChange,
  sourceBook,
  selectedFiles,
  library,
  onSuccess,
}: MoveFilesDialogProps) {
  const [selectedBookId, setSelectedBookId] = useState<string>("new");
  const moveFilesMutation = useMoveFiles();

  // Reset state when dialog opens
  useEffect(() => {
    if (open) {
      setSelectedBookId("new");
    }
  }, [open]);

  const handleMove = async () => {
    const targetBookId =
      selectedBookId === "new" ? undefined : parseInt(selectedBookId, 10);

    try {
      const result = await moveFilesMutation.mutateAsync({
        bookId: sourceBook.id,
        payload: {
          file_ids: selectedFiles.map((f) => f.id),
          target_book_id: targetBookId,
        },
      });

      if (result.target_book) {
        toast.success(
          `Moved ${result.files_moved} file${result.files_moved !== 1 ? "s" : ""} to "${result.target_book.title}"`,
        );
        onSuccess?.(result.target_book);
      }
      onOpenChange(false);
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to move files";
      toast.error(message);
    }
  };

  // Check if moving all files will delete the source book
  const willDeleteBook =
    selectedFiles.length === (sourceBook.files?.length || 0);

  const warningMessage = willDeleteBook
    ? library.organize_file_structure
      ? "The selected files will be moved to the target book's folder. This book will be deleted."
      : "The selected files will be reassigned to the target book. This book will be deleted."
    : library.organize_file_structure
      ? "The selected files will be moved to the target book's folder. Metadata from the source book will not be transferred."
      : "The selected files will be reassigned to the target book. Metadata from the source book will not be transferred.";

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Move Files</DialogTitle>
          <DialogDescription>
            Move {selectedFiles.length} file
            {selectedFiles.length !== 1 ? "s" : ""} to another book
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {willDeleteBook ? (
            <div className="flex items-start gap-3 p-3 rounded-lg bg-destructive/10 border border-destructive/20">
              <AlertTriangle className="h-5 w-5 text-destructive shrink-0 mt-0.5" />
              <div className="text-sm">
                <p className="font-medium text-destructive">
                  This action cannot be undone
                </p>
                <p className="text-muted-foreground mt-1">{warningMessage}</p>
              </div>
            </div>
          ) : (
            <div className="flex items-start gap-3 p-3 rounded-lg bg-muted/50 border">
              <Info className="h-5 w-5 text-muted-foreground shrink-0 mt-0.5" />
              <p className="text-sm text-muted-foreground">{warningMessage}</p>
            </div>
          )}

          <div className="space-y-2">
            <Label>Destination</Label>
            <BookSelectionList
              enabled={open}
              excludeBookId={sourceBook.id}
              key={open ? "open" : "closed"}
              libraryId={library.id}
              onSelectBook={setSelectedBookId}
              selectedBookId={selectedBookId}
              showCreateNew
            />
          </div>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={moveFilesMutation.isPending}
            onClick={handleMove}
            variant={willDeleteBook ? "destructive" : "default"}
          >
            {moveFilesMutation.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Move Files
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
