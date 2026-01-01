import { useParams, useSearchParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import Gallery from "@/components/library/Gallery";
import TopNav from "@/components/library/TopNav";
import { useBooks } from "@/hooks/queries/books";
import { useSeries } from "@/hooks/queries/series";
import type { Book } from "@/types";

const ITEMS_PER_PAGE = 20;

const Home = () => {
  const { libraryId } = useParams();
  const [searchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const seriesIdParam = searchParams.get("series_id");

  // Calculate pagination parameters
  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const seriesId = seriesIdParam ? parseInt(seriesIdParam, 10) : undefined;

  const booksQuery = useBooks({
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    series_id: seriesId,
  });

  const seriesQuery = useSeries(seriesId, {
    enabled: Boolean(seriesId),
  });

  const renderBookItem = (book: Book) => (
    <BookItem
      book={book}
      key={book.id}
      libraryId={libraryId!}
      showSeriesNumber={Boolean(seriesId)}
    />
  );

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        {seriesQuery.data && seriesId && (
          <div className="mb-6">
            <h1 className="text-2xl font-semibold mb-1">
              {seriesQuery.data.name}
            </h1>
            {seriesQuery.data.description && (
              <p className="text-sm text-muted-foreground mb-2">
                {seriesQuery.data.description}
              </p>
            )}
          </div>
        )}
        <Gallery
          isLoading={booksQuery.isLoading}
          isSuccess={booksQuery.isSuccess}
          itemLabel="books"
          items={booksQuery.data?.books ?? []}
          itemsPerPage={ITEMS_PER_PAGE}
          renderItem={renderBookItem}
          total={booksQuery.data?.total ?? 0}
        />
      </div>
    </div>
  );
};

export default Home;
