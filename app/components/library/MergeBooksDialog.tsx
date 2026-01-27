import { useQueries } from "@tanstack/react-query";
import { AlertTriangle, GitMerge, Loader2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

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
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { QueryKey, useMergeBooks } from "@/hooks/queries/books";
import { API, ShishoAPIError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type { Book, Library } from "@/types";

interface MergeBooksDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bookIds: number[];
  library: Library;
  onSuccess?: (targetBook: Book) => void;
}

type DialogStep = "select" | "confirm";

export function MergeBooksDialog({
  open,
  onOpenChange,
  bookIds,
  library,
  onSuccess,
}: MergeBooksDialogProps) {
  const [selectedTargetId, setSelectedTargetId] = useState<string>("");
  const [step, setStep] = useState<DialogStep>("select");
  const mergeBooksMutation = useMergeBooks();

  // Fetch all books by ID
  const bookQueries = useQueries({
    queries: bookIds.map((id) => ({
      queryKey: [QueryKey.RetrieveBook, String(id)],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        API.request<Book>("GET", `/books/${id}`, null, null, signal),
      enabled: open,
    })),
  });

  const isLoadingBooks = bookQueries.some((q) => q.isLoading);
  const bookQueryError = bookQueries.find((q) => q.error)?.error as
    | ShishoAPIError
    | undefined;
  const books = useMemo(
    () => bookQueries.map((q) => q.data).filter((b): b is Book => b != null),
    [bookQueries],
  );

  // Reset state when dialog opens, default to first book
  useEffect(() => {
    if (open && books.length > 0 && !selectedTargetId) {
      setSelectedTargetId(String(books[0].id));
      setStep("select");
    }
  }, [open, books, selectedTargetId]);

  // Reset selection when dialog closes
  useEffect(() => {
    if (!open) {
      setSelectedTargetId("");
      setStep("select");
    }
  }, [open]);

  const handleMerge = async () => {
    const targetBookId = parseInt(selectedTargetId, 10);
    const sourceBookIds = books
      .map((b) => b.id)
      .filter((id) => id !== targetBookId);

    try {
      const result = await mergeBooksMutation.mutateAsync({
        payload: {
          source_book_ids: sourceBookIds,
          target_book_id: targetBookId,
        },
      });

      if (result.target_book) {
        toast.success(
          `Merged ${result.books_deleted} book${result.books_deleted !== 1 ? "s" : ""} into "${result.target_book.title}"`,
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

  const handleProceedToConfirm = () => {
    setStep("confirm");
  };

  const handleBack = () => {
    setStep("select");
  };

  const totalFiles = books.reduce(
    (sum, book) => sum + (book.files?.length || 0),
    0,
  );

  const targetBook = books.find((b) => String(b.id) === selectedTargetId);
  const booksToDelete = books.filter((b) => String(b.id) !== selectedTargetId);
  const filesToMove = booksToDelete.reduce(
    (sum, book) => sum + (book.files?.length || 0),
    0,
  );

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GitMerge className="h-5 w-5" />
            Merge Books
          </DialogTitle>
          <DialogDescription>
            {isLoadingBooks
              ? `Loading ${bookIds.length} books...`
              : `Merge ${books.length} books (${totalFiles} files total)`}
          </DialogDescription>
        </DialogHeader>

        {isLoadingBooks && (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        )}

        {bookQueryError && (
          <div className="flex items-start gap-3 p-3 rounded-lg bg-destructive/10 border border-destructive/20">
            <AlertTriangle className="h-5 w-5 text-destructive shrink-0 mt-0.5" />
            <div className="text-sm">
              <p className="font-medium text-destructive">
                Failed to load books
              </p>
              <p className="text-muted-foreground mt-1">
                {bookQueryError.message || "An error occurred"}
              </p>
            </div>
          </div>
        )}

        {!isLoadingBooks && !bookQueryError && step === "select" && (
          <>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>Select target book</Label>
                <p className="text-sm text-muted-foreground">
                  All files will be moved to this book. Other books will be
                  deleted.
                </p>
              </div>

              <ScrollArea className="h-64 rounded-md border">
                <RadioGroup
                  className="p-3"
                  onValueChange={setSelectedTargetId}
                  value={selectedTargetId}
                >
                  {books.map((book) => {
                    const isSelected = String(book.id) === selectedTargetId;
                    return (
                      <label
                        className={cn(
                          "flex items-start gap-3 p-2 rounded-md cursor-pointer transition-colors",
                          isSelected ? "bg-primary/10" : "hover:bg-muted/50",
                        )}
                        htmlFor={`book-${book.id}`}
                        key={book.id}
                      >
                        <RadioGroupItem
                          className="mt-0.5"
                          id={`book-${book.id}`}
                          value={String(book.id)}
                        />
                        <div className="min-w-0 flex-1">
                          <div className="font-medium">{book.title}</div>
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <span>
                              {book.files?.length || 0} file
                              {(book.files?.length || 0) !== 1 ? "s" : ""}
                            </span>
                            {book.authors && book.authors.length > 0 && (
                              <>
                                <span className="text-muted-foreground/50">
                                  ·
                                </span>
                                <span className="truncate">
                                  {book.authors
                                    .map((a) => a.person?.name)
                                    .filter(Boolean)
                                    .join(", ")}
                                </span>
                              </>
                            )}
                          </div>
                        </div>
                      </label>
                    );
                  })}
                </RadioGroup>
              </ScrollArea>
            </div>

            <DialogFooter>
              <Button onClick={() => onOpenChange(false)} variant="outline">
                Cancel
              </Button>
              <Button
                disabled={!selectedTargetId}
                onClick={handleProceedToConfirm}
              >
                Continue
              </Button>
            </DialogFooter>
          </>
        )}

        {!isLoadingBooks &&
          !bookQueryError &&
          step === "confirm" &&
          targetBook && (
            <>
              <div className="space-y-4">
                {/* Warning banner */}
                <div className="flex items-start gap-3 p-3 rounded-lg bg-destructive/10 border border-destructive/20">
                  <AlertTriangle className="h-5 w-5 text-destructive shrink-0 mt-0.5" />
                  <div className="text-sm">
                    <p className="font-medium text-destructive">
                      This action cannot be undone
                    </p>
                    <p className="text-muted-foreground mt-1">
                      {library.organize_file_structure
                        ? "Files will be physically moved to the target book's folder."
                        : "Files will be reassigned to the target book."}{" "}
                      Metadata from deleted books will not be transferred.
                    </p>
                  </div>
                </div>

                {/* Summary */}
                <div className="space-y-3">
                  <div className="p-3 rounded-lg border bg-card">
                    <div className="text-sm text-muted-foreground mb-1">
                      Target book
                    </div>
                    <div className="font-medium">{targetBook.title}</div>
                    {targetBook.authors && targetBook.authors.length > 0 && (
                      <div className="text-sm text-muted-foreground">
                        {targetBook.authors
                          .map((a) => a.person?.name)
                          .filter(Boolean)
                          .join(", ")}
                      </div>
                    )}
                  </div>

                  <Separator />

                  <div>
                    <div className="text-sm text-muted-foreground mb-2">
                      Books to be deleted ({booksToDelete.length})
                    </div>
                    <div className="space-y-1">
                      {booksToDelete.map((book) => (
                        <div
                          className="flex items-center gap-2 text-sm"
                          key={book.id}
                        >
                          <span className="truncate flex-1">{book.title}</span>
                          <Badge className="shrink-0" variant="secondary">
                            {book.files?.length || 0} file
                            {(book.files?.length || 0) !== 1 ? "s" : ""}
                          </Badge>
                        </div>
                      ))}
                    </div>
                  </div>

                  <Separator />

                  <div className="flex items-center justify-between text-sm">
                    <span className="text-muted-foreground">
                      Files to be moved
                    </span>
                    <span className="font-medium">{filesToMove}</span>
                  </div>
                </div>
              </div>

              <DialogFooter>
                <Button
                  disabled={mergeBooksMutation.isPending}
                  onClick={handleBack}
                  variant="outline"
                >
                  Back
                </Button>
                <Button
                  disabled={mergeBooksMutation.isPending}
                  onClick={handleMerge}
                  variant="destructive"
                >
                  {mergeBooksMutation.isPending && (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  )}
                  Merge {booksToDelete.length} Book
                  {booksToDelete.length !== 1 ? "s" : ""}
                </Button>
              </DialogFooter>
            </>
          )}
      </DialogContent>
    </Dialog>
  );
}
