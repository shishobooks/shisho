import StarRating from "@/components/library/StarRating";
import { Button } from "@/components/ui/button";
import { useBookReading, useSetRating } from "@/hooks/queries/reading";
import { toastRequestError } from "@/libraries/api";

interface ReadingFinishedToastBodyProps {
  bookId: number;
  onUndo: () => void;
}

// The finish toast's stars, which save the Book's Rating as the book page's
// stars do, and its Undo. Undo is the app's own button: the toast library's
// action button doesn't follow the app's styles.
const ReadingFinishedToastBody = ({
  bookId,
  onUndo,
}: ReadingFinishedToastBodyProps) => {
  const reading = useBookReading(bookId);
  const setRating = useSetRating();
  const handleRating = async (stars: number) => {
    try {
      await setRating.mutateAsync({ bookId, payload: { rating: stars } });
    } catch (error) {
      toastRequestError(error, "Failed to save rating");
    }
  };
  return (
    <div className="mt-1 flex items-center justify-between gap-3">
      <StarRating
        onChange={handleRating}
        value={reading.data?.rating ?? null}
      />
      <Button onClick={onUndo} size="sm" variant="outline">
        Undo
      </Button>
    </div>
  );
};

export default ReadingFinishedToastBody;
