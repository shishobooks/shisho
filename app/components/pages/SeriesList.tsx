import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import Gallery from "@/components/library/Gallery";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { useLibrary } from "@/hooks/queries/libraries";
import { useSeriesList } from "@/hooks/queries/series";
import { useDebounce } from "@/hooks/useDebounce";
import { cn } from "@/libraries/utils";
import type { Series } from "@/types";

// For series, we don't have access to the underlying files, so we use the
// library's cover_aspect_ratio preference to determine aspect ratio.
// For fallback modes, we use the primary preference.
const getSeriesAspectRatioClass = (coverAspectRatio: string): string => {
  switch (coverAspectRatio) {
    case "audiobook":
    case "audiobook_fallback_book":
      return "aspect-square";
    case "book":
    case "book_fallback_audiobook":
    default:
      return "aspect-[2/3]";
  }
};

const ITEMS_PER_PAGE = 24;

const SeriesList = () => {
  const { libraryId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const searchQuery = searchParams.get("search") ?? "";

  const [searchInput, setSearchInput] = useState(searchQuery);
  const debouncedSearch = useDebounce(searchInput, 300);

  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const libraryQuery = useLibrary(libraryId);
  const coverAspectRatio = libraryQuery.data?.cover_aspect_ratio ?? "book";

  // Handle search input change
  const handleSearchChange = (value: string) => {
    setSearchInput(value);
    // Update URL after debounce
    setTimeout(() => {
      if (value !== searchQuery) {
        const newParams = new URLSearchParams(searchParams);
        if (value) {
          newParams.set("search", value);
        } else {
          newParams.delete("search");
        }
        newParams.set("page", "1");
        setSearchParams(newParams);
      }
    }, 300);
  };

  const seriesQuery = useSeriesList({
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: debouncedSearch || undefined,
  });

  const renderSeriesItem = (seriesItem: Series) => {
    const bookCount = seriesItem.book_count ?? 0;
    const aspectClass = getSeriesAspectRatioClass(coverAspectRatio);

    return (
      <div
        className="w-32"
        key={seriesItem.id}
        title={`${seriesItem.name}\n${bookCount} book${bookCount !== 1 ? "s" : ""}`}
      >
        <Link
          className="group cursor-pointer"
          to={`/libraries/${libraryId}/series/${seriesItem.id}`}
        >
          <img
            alt={`${seriesItem.name} Cover`}
            className={cn(
              "w-full object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1",
              aspectClass,
            )}
            onError={(e) => {
              (e.target as HTMLImageElement).style.display = "none";
              (e.target as HTMLImageElement).nextElementSibling!.textContent =
                "no cover";
            }}
            src={`/api/series/${seriesItem.id}/cover`}
          />
          <div className="mt-2 group-hover:underline font-bold line-clamp-2 w-32">
            {seriesItem.name}
          </div>
        </Link>
        <div className="mt-1 text-sm line-clamp-1 text-neutral-500 dark:text-neutral-500">
          <Badge className="text-xs" variant="secondary">
            {bookCount} book{bookCount !== 1 ? "s" : ""}
          </Badge>
        </div>
      </div>
    );
  };

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        <div className="mb-6">
          <h1 className="text-2xl font-semibold mb-2">Series</h1>
          <p className="text-muted-foreground">
            Browse book series in your library
          </p>
        </div>

        <div className="mb-6">
          <Input
            className="max-w-xs"
            onChange={(e) => handleSearchChange(e.target.value)}
            placeholder="Search series..."
            type="search"
            value={searchInput}
          />
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
