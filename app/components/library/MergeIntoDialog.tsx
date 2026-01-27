import { BookSelectionList } from "./BookSelectionList";
import { AlertTriangle, GitMerge, Loader2 } from "lucide-react";
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
import { useMergeBooks } from "@/hooks/queries/books";
import type { Book, Library } from "@/types";

interface MergeIntoDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sourceBook: Book;
  library: Library;
  onSuccess?: (targetBook: Book) => void;
}

export function MergeIntoDialog({
  open,
  onOpenChange,
  sourceBook,
  library,
  onSuccess,
}: MergeIntoDialogProps) {
  const [selectedTargetId, setSelectedTargetId] = useState<string>("");
  const mergeBooksMutation = useMergeBooks();

  // Reset state when dialog opens
  useEffect(() => {
    if (open) {
      setSelectedTargetId("");
    }
  }, [open]);

  const handleMerge = async () => {
    const targetBookId = parseInt(selectedTargetId, 10);

    try {
      const result = await mergeBooksMutation.mutateAsync({
        payload: {
          source_book_ids: [sourceBook.id],
          target_book_id: targetBookId,
        },
      });

      if (result.target_book) {
        toast.success(
          `Merged "${sourceBook.title}" into "${result.target_book.title}"`,
        );
        onSuccess?.(result.target_book);
      }
      onOpenChange(false);
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to merge books";
      toast.error(message);
    }
  };

  const sourceFileCount = sourceBook.files?.length || 0;

  const warningMessage = library.organize_file_structure
    ? "Files will be physically moved to the target book's folder. This book will be deleted."
    : "Files will be reassigned to the target book. This book will be deleted.";

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GitMerge className="h-5 w-5 shrink-0" />
            Merge Into Another Book
          </DialogTitle>
          <DialogDescription>
            Move {sourceFileCount} file{sourceFileCount !== 1 ? "s" : ""} from
            this book into another book.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="flex items-start gap-3 p-3 rounded-lg bg-destructive/10 border border-destructive/20">
            <AlertTriangle className="h-5 w-5 text-destructive shrink-0 mt-0.5" />
            <div className="text-sm">
              <p className="font-medium text-destructive">
                This action cannot be undone
              </p>
              <p className="text-muted-foreground mt-1">{warningMessage}</p>
            </div>
          </div>

          <div className="space-y-2">
            <Label>Select target book</Label>
            <BookSelectionList
              enabled={open}
              excludeBookId={sourceBook.id}
              key={open ? "open" : "closed"}
              libraryId={library.id}
              onSelectBook={setSelectedTargetId}
              selectedBookId={selectedTargetId}
            />
          </div>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={!selectedTargetId || mergeBooksMutation.isPending}
            onClick={handleMerge}
            variant="destructive"
          >
            {mergeBooksMutation.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Merge
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
