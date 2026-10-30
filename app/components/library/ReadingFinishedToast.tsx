import { toast } from "sonner";

import ReadingFinishedRating from "@/components/library/ReadingFinishedRating";

// Long enough to rate the book before it goes.
const FINISHED_TOAST_MS = 15_000;

interface ShowReadingFinishedToastOptions {
  bookId: number;
  onUndo: () => void;
}

const toastId = (bookId: number) => `reading-finished-${bookId}`;

// The toast a reader shows when reading to the end finished the Book's
// Read-through: stars to rate it while it's fresh, and Undo.
export const showReadingFinishedToast = ({
  bookId,
  onUndo,
}: ShowReadingFinishedToastOptions) =>
  toast.success("Marked as read", {
    id: toastId(bookId),
    description: <ReadingFinishedRating bookId={bookId} />,
    duration: FINISHED_TOAST_MS,
    action: { label: "Undo", onClick: onUndo },
  });

// Takes the toast away once going back has undone the finish it announced.
export const dismissReadingFinishedToast = (bookId: number) =>
  toast.dismiss(toastId(bookId));
