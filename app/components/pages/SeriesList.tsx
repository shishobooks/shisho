import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";

import CoverImage from "@/components/library/CoverImage";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import Gallery from "@/components/library/Gallery";
import LibraryLayout from "@/components/library/LibraryLayout";
import { SearchInput } from "@/components/library/SearchInput";
import { SizeButton, SizePopover } from "@/components/library/SizePopover";
import SortedByChips from "@/components/library/SortedByChips";
import SortSheet, { SortButton } from "@/components/library/SortSheet";
import TruncatedText from "@/components/library/TruncatedText";
import { Badge } from "@/components/ui/badge";
import { useUserLibrary } from "@/hooks/queries/libraries";
import { useSeriesList } from "@/hooks/queries/series";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useSearchQueryParam } from "@/hooks/useSearchQueryParam";
import { useSortParam } from "@/hooks/useSortParam";
import { BOOK_COUNT_RESOURCE_SORT } from "@/libraries/resourceSort";
import { cn } from "@/libraries/utils";
import type { SeriesResponse } from "@/types";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";
import { seriesCoverUrl } from "@/utils/coverUrl";
import { formatCount } from "@/utils/format";

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

interface SeriesCardProps {
  seriesItem: SeriesResponse;
  libraryId: string;
  aspectClass: string;
  isAudiobook: boolean;
}

export const SeriesCard = ({
  seriesItem,
  libraryId,
  aspectClass,
  isAudiobook,
}: SeriesCardProps) => {
  const coverUrl = seriesCoverUrl(seriesItem);
  const [coverLoaded, setCoverLoaded] = useState(() => isCoverLoaded(coverUrl));
  const [coverError, setCoverError] = useState(false);
  const bookCount = seriesItem.book_count ?? 0;
  const showSortName =
    seriesItem.sort_name && seriesItem.sort_name !== seriesItem.name;

  const handleCoverLoad = () => {
    markCoverLoaded(coverUrl);
    setCoverLoaded(true);
  };

  return (
    <div
      className="w-full"
      title={`${seriesItem.name}${showSortName ? `\nSort: ${seriesItem.sort_name}` : ""}\n${formatCount(bookCount, "book")}`}
    >
      <Link
        className="group cursor-pointer"
        to={`/libraries/${libraryId}/series/${seriesItem.id}`}
      >
        <div className={cn("relative", aspectClass)}>
          {/* Placeholder shown until image loads or on error */}
          {(!coverLoaded || coverError) && (
            <CoverPlaceholder
              className={cn("absolute inset-0 rounded-sm border border-border")}
              variant={isAudiobook ? "audiobook" : "book"}
            />
          )}
          {/* Image hidden until loaded, removed on error */}
          {!coverError && (
            <CoverImage
              alt={`${seriesItem.name} Cover`}
              className={cn(
                "w-full h-full object-cover rounded-sm border-border border-1",
                !coverLoaded && "opacity-0",
              )}
              onError={() => setCoverError(true)}
              onLoad={handleCoverLoad}
              src={coverUrl}
            />
          )}
        </div>
        <TruncatedText className="mt-2 group-hover:underline text-sm font-bold line-clamp-2">
          {seriesItem.name}
        </TruncatedText>
      </Link>
      <div className="mt-1 text-xs line-clamp-1 text-muted-foreground">
        <Badge className="text-xs" variant="secondary">
          {formatCount(bookCount, "book")}
        </Badge>
      </div>
    </div>
  );
};

const SeriesList = () => {
  usePageTitle("Series");

  const { libraryId } = useParams();
  const { searchQuery, setSearchQuery } = useSearchQueryParam();
  const sort = useSortParam(BOOK_COUNT_RESOURCE_SORT);

  const {
    savedSize,
    effectiveSize,
    isSizeDirty,
    columns,
    itemsPerPage,
    offset,
    currentPage,
    goToPage,
    pageSizeResolved,
    isSaving: isSavingSize,
    applyGallerySize,
    saveSizeAsDefault,
  } = useGallerySizeParam();

  const libraryQuery = useUserLibrary(libraryId);
  const coverAspectRatio = libraryQuery.data?.cover_aspect_ratio ?? "book";

  const seriesQuery = useSeriesList(
    {
      limit: itemsPerPage,
      offset,
      library_id: libraryId ? parseInt(libraryId, 10) : undefined,
      search: searchQuery || undefined,
      sort: sort.param,
    },
    { enabled: pageSizeResolved },
  );

  // Track the search value that produced the currently displayed data
  const [confirmedSearch, setConfirmedSearch] = useState<string | null>(null);

  // A failed search settles too, so the gallery shows its error instead of a
  // spinner that waits for a success.
  const seriesSettled = seriesQuery.isSuccess || seriesQuery.isError;
  useEffect(() => {
    if (seriesSettled && !seriesQuery.isFetching) {
      setConfirmedSearch(searchQuery);
    }
  }, [seriesSettled, seriesQuery.isFetching, searchQuery]);

  // Data is stale if search changed but query hasn't completed yet
  const isStaleData =
    confirmedSearch !== null && searchQuery !== confirmedSearch;

  const renderSeriesItem = (seriesItem: SeriesResponse) => {
    const aspectClass = getSeriesAspectRatioClass(coverAspectRatio);
    const isAudiobook = coverAspectRatio.startsWith("audiobook");

    return (
      <SeriesCard
        aspectClass={aspectClass}
        isAudiobook={isAudiobook}
        key={seriesItem.id}
        libraryId={libraryId ?? ""}
        seriesItem={seriesItem}
      />
    );
  };

  return (
    <LibraryLayout>
      <div className="mb-6">
        <h1 className="text-2xl font-semibold mb-2">Series</h1>
        <p className="text-muted-foreground">
          Browse book series in your library
        </p>
      </div>

      <div className="mb-6">
        <div className="flex flex-wrap items-center gap-3">
          <SearchInput
            initialValue={searchQuery}
            label="Search series"
            onDebouncedChange={setSearchQuery}
            placeholder="Search series..."
          />
          <SortSheet
            fieldLabels={sort.config.labels}
            fields={sort.config.fields}
            isDirty={sort.isDirty}
            levels={sort.levels}
            onChange={sort.apply}
            trigger={<SortButton isDirty={sort.isDirty} />}
          />
          <div className="hidden sm:flex">
            <SizePopover
              effectiveSize={effectiveSize}
              isSaving={isSavingSize}
              onChange={applyGallerySize}
              onSaveAsDefault={saveSizeAsDefault}
              savedSize={savedSize}
              trigger={<SizeButton isDirty={isSizeDirty} />}
            />
          </div>
        </div>
        <SortedByChips
          fieldLabels={sort.config.labels}
          levels={sort.isDirty ? sort.levels : []}
          onRemoveLevel={(index) =>
            sort.apply(sort.levels.filter((_, i) => i !== index))
          }
          onReset={sort.reset}
        />
      </div>

      <Gallery
        columns={columns}
        currentPage={currentPage}
        emptyMessage={
          confirmedSearch
            ? "No series found matching your search."
            : "No series in this library yet."
        }
        // A background refetch (an SSE event) keeps the loaded items on
        // screen. A new page or search shows the spinner, not the previous
        // key's items kept as placeholder data.
        isLoading={
          seriesQuery.isLoading || seriesQuery.isPlaceholderData || isStaleData
        }
        itemLabel="series"
        items={seriesQuery.data?.items ?? []}
        itemsPerPage={itemsPerPage}
        offset={offset}
        onPageChange={goToPage}
        query={seriesQuery}
        renderItem={renderSeriesItem}
        total={seriesQuery.data?.total ?? 0}
      />
    </LibraryLayout>
  );
};

export default SeriesList;
