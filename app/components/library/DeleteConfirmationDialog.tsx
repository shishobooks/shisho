import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  FileText,
  Loader2,
} from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { Book, File } from "@/types";
import { formatFileSize } from "@/utils/format";

// Calculate total size of files
const getTotalSize = (files: File[] | undefined): number => {
  if (!files) return 0;
  return files.reduce((sum, f) => sum + (f.filesize_bytes ?? 0), 0);
};

type DeleteVariant = "book" | "books" | "file";

interface DeleteConfirmationDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  variant: DeleteVariant;
  // For single book/file delete
  title?: string;
  files?: File[];
  // For bulk delete
  books?: Pick<Book, "id" | "title" | "files">[];
  onConfirm: () => void;
  isPending: boolean;
}

export function DeleteConfirmationDialog({
  open,
  onOpenChange,
  variant,
  title,
  files,
  books,
  onConfirm,
  isPending,
}: DeleteConfirmationDialogProps) {
  const [showDetails, setShowDetails] = useState(false);

  const getDialogTitle = () => {
    switch (variant) {
      case "book":
        return "Delete Book";
      case "books":
        return "Delete Books";
      case "file":
        return "Delete File";
    }
  };

  const getSummary = () => {
    switch (variant) {
      case "book":
        return (
          <>
            <span className="font-medium">&ldquo;{title}&rdquo;</span>
            {files && files.length > 0 && (
              <span className="text-muted-foreground">
                {" "}
                ({files.length} file{files.length !== 1 ? "s" : ""})
              </span>
            )}
          </>
        );
      case "books": {
        const totalFiles =
          books?.reduce((sum, b) => sum + (b.files?.length ?? 0), 0) ?? 0;
        return (
          <>
            <span className="font-medium">
              {books?.length} book{books?.length !== 1 ? "s" : ""}
            </span>
            <span className="text-muted-foreground">
              {" "}
              ({totalFiles} file{totalFiles !== 1 ? "s" : ""} total)
            </span>
          </>
        );
      }
      case "file":
        return <span className="font-medium">&ldquo;{title}&rdquo;</span>;
    }
  };

  const renderDetails = () => {
    if (variant === "book" && files) {
      return (
        <ul className="text-sm space-y-1 overflow-hidden">
          {files.map((file) => (
            <li className="flex gap-4 overflow-hidden" key={file.id}>
              <span className="flex-1 truncate min-w-0">
                {file.filepath.split("/").pop()}
              </span>
              <span className="text-muted-foreground shrink-0">
                {formatFileSize(file.filesize_bytes)}
              </span>
            </li>
          ))}
        </ul>
      );
    }

    if (variant === "books" && books) {
      return (
        <div className="space-y-2">
          {books.map((book) => {
            const totalSize = getTotalSize(book.files);
            const fileCount = book.files?.length ?? 0;
            return (
              <div className="rounded-lg bg-muted/50 p-3 min-w-0" key={book.id}>
                {/* Book header row */}
                <div className="flex items-start justify-between gap-3">
                  <h4 className="font-medium text-sm leading-tight truncate min-w-0 flex-1">
                    {book.title}
                  </h4>
                  <span className="text-xs text-muted-foreground shrink-0 tabular-nums">
                    {formatFileSize(totalSize)}
                  </span>
                </div>
                {/* File count badge */}
                {fileCount > 0 && (
                  <div className="mt-2 flex items-center gap-1.5 text-xs text-muted-foreground">
                    <FileText className="h-3 w-3" />
                    <span>
                      {fileCount} file{fileCount !== 1 ? "s" : ""}
                    </span>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      );
    }

    return null;
  };

  const hasDetails =
    (variant === "book" && files && files.length > 0) ||
    (variant === "books" && books && books.length > 0);

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md max-h-[90vh] overflow-y-auto overflow-x-hidden">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive shrink-0" />
            {getDialogTitle()}
          </DialogTitle>
          <DialogDescription className="sr-only">
            Confirm deletion of {variant === "books" ? "books" : variant}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 min-w-0">
          {/* Warning banner */}
          <div className="bg-destructive/10 border border-destructive/20 rounded-md p-3 text-sm text-destructive break-words">
            This action cannot be undone. Files will be permanently deleted from
            disk.
          </div>

          {/* Summary */}
          <p className="text-sm">
            Are you sure you want to delete {getSummary()}?
          </p>

          {/* Expandable details */}
          {hasDetails && (
            <div>
              <button
                className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
                onClick={() => setShowDetails(!showDetails)}
                type="button"
              >
                {showDetails ? (
                  <ChevronDown className="h-4 w-4" />
                ) : (
                  <ChevronRight className="h-4 w-4" />
                )}
                {showDetails ? "Hide details" : "Show details"}
              </button>

              {showDetails && (
                <div className="mt-2 max-h-48 rounded-md border p-3 overflow-y-auto overflow-x-hidden">
                  {renderDetails()}
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            disabled={isPending}
            onClick={onConfirm}
            variant="destructive"
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
