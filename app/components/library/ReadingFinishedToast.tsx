import { CircleCheck } from "lucide-react";
import { toast } from "sonner";

import ReadingFinishedToastBody from "@/components/library/ReadingFinishedToastBody";

// Long enough to rate the book before it goes.
const FINISHED_TOAST_MS = 15_000;

interface ShowReadingFinishedToastOptions {
  bookId: number;
  onUndo: () => void;
}

const toastId = (bookId: number) => `reading-finished-${bookId}`;

// The toast a reader shows when reading to the end finished the Book's
// Read-through: stars to rate it while it's fresh, and Undo. It's a plain
// toast, not a success one, whose colors would tint the stars.
export const showReadingFinishedToast = ({
  bookId,
  onUndo,
}: ShowReadingFinishedToastOptions) =>
  toast("Marked as read", {
    id: toastId(bookId),
    icon: <CircleCheck aria-hidden className="h-4 w-4 text-primary" />,
    description: (
      <ReadingFinishedToastBody
        bookId={bookId}
        onUndo={() => {
          toast.dismiss(toastId(bookId));
          onUndo();
        }}
      />
    ),
    duration: FINISHED_TOAST_MS,
  });

// Takes the toast away once going back has undone the finish it announced.
export const dismissReadingFinishedToast = (bookId: number) =>
  toast.dismiss(toastId(bookId));
