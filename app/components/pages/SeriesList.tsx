import { Link, useParams, useSearchParams } from "react-router-dom";

import Gallery from "@/components/library/Gallery";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { useSeries, type SeriesInfo } from "@/hooks/queries/series";

const ITEMS_PER_PAGE = 20;

const SeriesList = () => {
  const { libraryId } = useParams();
  const [searchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);

  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const seriesQuery = useSeries({ limit, offset });

  const renderSeriesItem = (seriesItem: SeriesInfo) => (
    <div
      className="w-32"
      key={seriesItem.name}
      title={`${seriesItem.name}\n${seriesItem.book_count} book${seriesItem.book_count !== 1 ? "s" : ""}`}
    >
      <Link
        className="group cursor-pointer"
        to={`/libraries/${libraryId}/series/${encodeURIComponent(seriesItem.name)}`}
      >
        <img
          alt={`${seriesItem.name} Cover`}
          className="h-48 object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1"
          onError={(e) => {
            (e.target as HTMLImageElement).style.display = "none";
            (e.target as HTMLImageElement).nextElementSibling!.textContent =
              "no cover";
          }}
          src={`/api/series/${encodeURIComponent(seriesItem.name)}/cover`}
        />
        <div className="mt-2 group-hover:underline font-bold line-clamp-2 w-32">
          {seriesItem.name}
        </div>
      </Link>
      <div className="mt-1 text-sm line-clamp-1 text-neutral-500 dark:text-neutral-500">
        <Badge className="text-xs" variant="secondary">
          {seriesItem.book_count} book{seriesItem.book_count !== 1 ? "s" : ""}
        </Badge>
      </div>
    </div>
  );

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full p-8 m-auto">
        <div className="mb-8">
          <h1 className="text-3xl font-bold mb-2">Series</h1>
          <p className="text-muted-foreground">
            Browse book series in your library
          </p>
        </div>

        <Gallery
          isLoading={seriesQuery.isLoading}
          isSuccess={seriesQuery.isSuccess}
          itemLabel="series"
          items={seriesQuery.data?.series ?? []}
          itemsPerPage={ITEMS_PER_PAGE}
          renderItem={renderSeriesItem}
          total={seriesQuery.data?.total ?? 0}
        />
      </div>
    </div>
  );
};

export default SeriesList;
