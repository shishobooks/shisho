import ReadingStatusBookmark from "@/components/library/ReadingStatusBookmark";
import { useBookReading } from "@/hooks/queries/reading";

interface BookReadingBookmarkProps {
  bookId: number;
}

// The book page's cover bookmark: the signed-in user's Reading Status, as
// book cards show it. Nothing shows without a status.
const BookReadingBookmark = ({ bookId }: BookReadingBookmarkProps) => {
  const status = useBookReading(bookId).data?.status;
  if (!status) return null;
  return <ReadingStatusBookmark status={status} />;
};

export default BookReadingBookmark;
