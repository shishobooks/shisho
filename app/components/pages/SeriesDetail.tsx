import { ArrowLeft } from "lucide-react";
import { Link, useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { useSeries, useSeriesBooks } from "@/hooks/queries/series";

const SeriesDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const seriesId = id ? parseInt(id, 10) : undefined;

  const seriesQuery = useSeries(seriesId);
  const booksQuery = useSeriesBooks(seriesId);

  if (seriesQuery.isLoading || booksQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full p-8 m-auto">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!seriesQuery.isSuccess || !seriesQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full p-8 m-auto">
          <div className="mb-6">
            <Button asChild variant="ghost">
              <Link to={`/libraries/${libraryId}/series`}>
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Series
              </Link>
            </Button>
          </div>
          <div className="text-center">
            <h1 className="text-2xl font-bold mb-4">Series Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The series you're looking for doesn't exist or may have been
              removed.
            </p>
          </div>
        </div>
      </div>
    );
  }

  const series = seriesQuery.data;
  const books = booksQuery.data ?? [];

  // Sort books by series number if available, otherwise by title
  const sortedBooks = [...books].sort((a, b) => {
    if (a.series_number && b.series_number) {
      return a.series_number - b.series_number;
    }
    if (a.series_number && !b.series_number) return -1;
    if (!a.series_number && b.series_number) return 1;
    return a.title.localeCompare(b.title);
  });

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full p-8 m-auto">
        <div className="mb-6">
          <Button asChild variant="ghost">
            <Link to={`/libraries/${libraryId}/series`}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Series
            </Link>
          </Button>
        </div>

        <div className="mb-8">
          <h1 className="text-3xl font-bold mb-2">{series.name}</h1>
          {series.description && (
            <p className="text-muted-foreground mb-2">{series.description}</p>
          )}
          <p className="text-muted-foreground">
            {books.length} book{books.length !== 1 ? "s" : ""} in this series
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
          {sortedBooks.map((book) => {
            // Check if book cover is from an audiobook based on cover_image_path
            const isAudiobookCover =
              book.cover_image_path?.includes("audiobook_cover");
            const coverAspectRatio = isAudiobookCover
              ? "aspect-square"
              : "aspect-[2/3]";

            return (
              <Link
                key={book.id}
                to={`/libraries/${libraryId}/books/${book.id}`}
              >
                <Card className="h-full hover:shadow-md transition-shadow cursor-pointer">
                  <CardContent className="p-4">
                    <div className="mb-4">
                      <div className={`${coverAspectRatio} w-full mb-3`}>
                        <img
                          alt={`${book.title} Cover`}
                          className="w-full h-full object-cover rounded-md border"
                          onError={(e) => {
                            (e.target as HTMLImageElement).style.display =
                              "none";
                            (
                              e.target as HTMLImageElement
                            ).nextElementSibling!.textContent = "No cover";
                          }}
                          src={`/api/books/${book.id}/cover`}
                        />
                        <div className="hidden text-center text-muted-foreground text-sm"></div>
                      </div>

                      <div className="space-y-2">
                        <div className="flex items-center gap-2">
                          <h3 className="font-semibold text-sm leading-tight flex-1 line-clamp-2">
                            {book.title}
                          </h3>
                          {book.series_number && (
                            <Badge
                              className="text-xs flex-shrink-0"
                              variant="outline"
                            >
                              #{book.series_number}
                            </Badge>
                          )}
                        </div>

                        {book.authors && book.authors.length > 0 && (
                          <p className="text-xs text-muted-foreground line-clamp-1">
                            {book.authors
                              .map((author) => author.name)
                              .join(", ")}
                          </p>
                        )}
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </Link>
            );
          })}
        </div>

        {books.length === 0 && (
          <div className="text-center py-12">
            <h2 className="text-xl font-semibold mb-2">No Books Found</h2>
            <p className="text-muted-foreground">
              This series doesn't contain any books yet.
            </p>
          </div>
        )}
      </div>
    </div>
  );
};

export default SeriesDetail;
