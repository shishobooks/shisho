import { useId } from "react";
import { useSearchParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import GalleryGrid from "@/components/library/GalleryGrid";
import QueryError from "@/components/library/QueryError";
import { Button } from "@/components/ui/button";
import { useBooks } from "@/hooks/queries/books";
import { resetPaging } from "@/libraries/pagination";
import { writeReadingFilters } from "@/libraries/readingFilters";

interface CurrentlyReadingRowProps {
  libraryId: string;
  /** The gallery's column count, so the row holds one gallery row. */
  columns: number;
  coverAspectRatio: string;
}

/**
 * One gallery row of the library's books the user is reading, in the
 * server's order: the ones underway before the paused ones. It's hidden when
 * nothing is underway.
 */
const CurrentlyReadingRow = ({
  libraryId,
  columns,
  coverAspectRatio,
}: CurrentlyReadingRowProps) => {
  const headingId = useId();
  const [, setSearchParams] = useSearchParams();
  const query = useBooks({
    library_id: parseInt(libraryId, 10),
    currently_reading: true,
    limit: columns,
  });

  if (query.error && !query.data) {
    return (
      <QueryError
        className="mb-6"
        fallback="Failed to load the books you're reading"
        query={query}
      />
    );
  }
  // Nothing shows while loading, as when nothing is underway: a spinner
  // would collapse for most libraries.
  const books = query.data?.items ?? [];
  if (books.length === 0) return null;
  const total = query.data?.total ?? books.length;

  // The gallery below, filtered to every book underway.
  const showAll = () =>
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      writeReadingFilters(params, {
        statuses: ["currently_reading", "paused"],
      });
      resetPaging(params);
      return params;
    });

  return (
    <section aria-labelledby={headingId} className="mb-6">
      <div className="mb-3 flex items-center justify-between gap-2">
        <h2 className="text-lg font-semibold" id={headingId}>
          Currently Reading
        </h2>
        {total > books.length && (
          <Button onClick={showAll} size="sm" variant="ghost">
            Show all {total.toLocaleString()}
          </Button>
        )}
      </div>
      <GalleryGrid className="mb-0 md:mb-0" columns={columns}>
        {books.map((book) => (
          <BookItem
            book={book}
            coverAspectRatio={coverAspectRatio}
            key={book.id}
            libraryId={libraryId}
          />
        ))}
      </GalleryGrid>
    </section>
  );
};

export default CurrentlyReadingRow;
