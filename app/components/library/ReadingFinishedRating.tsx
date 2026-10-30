import StarRating from "@/components/library/StarRating";
import { useBookReading, useSetRating } from "@/hooks/queries/reading";
import { toastRequestError } from "@/libraries/api";

// The stars inside the finish toast, which save the Book's Rating as the
// book page's stars do.
const ReadingFinishedRating = ({ bookId }: { bookId: number }) => {
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
    <div className="mt-1">
      <StarRating
        onChange={handleRating}
        value={reading.data?.rating ?? null}
      />
    </div>
  );
};

export default ReadingFinishedRating;
