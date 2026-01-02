import { uniqBy } from "lodash";
import { Link } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import type { Book } from "@/types";

interface BookItemProps {
  book: Book;
  libraryId: string;
  seriesId?: number;
}

const BookItem = ({ book, libraryId, seriesId }: BookItemProps) => {
  // Find the series number for the specific series context (if provided)
  const seriesNumber = seriesId
    ? book.book_series?.find((bs) => bs.series_id === seriesId)?.series_number
    : undefined;
  return (
    <div className="w-32" key={book.id}>
      <Link
        className="group cursor-pointer"
        to={`/libraries/${libraryId}/books/${book.id}`}
      >
        <img
          alt={`${book.title} Cover`}
          className="h-48 object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1"
          onError={(e) => {
            (e.target as HTMLImageElement).style.display = "none";
            (e.target as HTMLImageElement).nextElementSibling!.textContent =
              "no cover";
          }}
          src={`/api/books/${book.id}/cover`}
        />
        <div className="mt-2 group-hover:underline font-bold line-clamp-2 w-32">
          {book.title}
        </div>
      </Link>
      {book.authors && book.authors.length > 0 && (
        <div className="mt-1 text-sm line-clamp-2 text-neutral-500 dark:text-neutral-500">
          {book.authors.map((a) => a.person?.name ?? "Unknown").join(", ")}
        </div>
      )}
      {book.files && (
        <div className="mt-2 flex gap-2 text-sm">
          {uniqBy(book.files, "file_type").map((f) => (
            <Badge className="uppercase" key={f.id} variant="subtle">
              {f.file_type}
            </Badge>
          ))}
        </div>
      )}
      {seriesNumber && (
        <div className="mt-1">
          <Badge className="text-xs" variant="outline">
            #{seriesNumber}
          </Badge>
        </div>
      )}
    </div>
  );
};

export default BookItem;
