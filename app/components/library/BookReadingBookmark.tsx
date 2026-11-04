import ReadingStatusBookmark from "@/components/library/ReadingStatusBookmark";
import { useBookReading } from "@/hooks/queries/reading";

interface BookReadingBookmarkProps {
  bookId: number;
}

// The book page's cover bookmark: the signed-in user's Reading Status, as
// book cards show it. Nothing shows without a status.
const BookReadingBookmark = ({ bookId }: BookReadingBookmarkProps) => {
  const reading = useBookReading(bookId).data;
  if (!reading?.status) return null;
  return (
    <ReadingStatusBookmark
      progress={reading.progress}
      status={reading.status}
    />
  );
};

export default BookReadingBookmark;
