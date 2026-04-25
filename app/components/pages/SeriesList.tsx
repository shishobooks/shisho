import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import Gallery from "@/components/library/Gallery";
import LibraryLayout from "@/components/library/LibraryLayout";
import { SearchInput } from "@/components/library/SearchInput";
import { SizeButton, SizePopover } from "@/components/library/SizePopover";
import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  COVER_WIDTH_CLASS,
  DEFAULT_GALLERY_SIZE,
  ITEMS_PER_PAGE_BY_SIZE,
} from "@/constants/gallerySize";
import { useLibrary } from "@/hooks/queries/libraries";
import { useSeriesList } from "@/hooks/queries/series";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { useIsTruncated } from "@/hooks/useIsTruncated";
import { usePageTitle } from "@/hooks/usePageTitle";
import { pageForSizeChange, parseGallerySize } from "@/libraries/gallerySize";
import { cn } from "@/libraries/utils";
import type { GallerySize, Series } from "@/types";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";

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
  seriesItem: Series;
  libraryId: string;
  aspectClass: string;
  isAudiobook: boolean;
  cacheKey?: number;
  gallerySize?: GallerySize;
}

const SeriesCard = ({
  seriesItem,
  libraryId,
  aspectClass,
  isAudiobook,
  cacheKey,
  gallerySize = DEFAULT_GALLERY_SIZE,
}: SeriesCardProps) => {
  const [titleRef, isTitleTruncated] = useIsTruncated<HTMLDivElement>();
  const coverUrl = cacheKey
    ? `/api/series/${seriesItem.id}/cover?v=${cacheKey}`
    : `/api/series/${seriesItem.id}/cover`;
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
      className={cn("w-[calc(50%-0.5rem)]", COVER_WIDTH_CLASS[gallerySize])}
      title={`${seriesItem.name}${showSortName ? `\nSort: ${seriesItem.sort_name}` : ""}\n${bookCount} book${bookCount !== 1 ? "s" : ""}`}
    >
      <Link
        className="group cursor-pointer"
        to={`/libraries/${libraryId}/series/${seriesItem.id}`}
      >
        <div className={cn("relative", aspectClass)}>
          {/* Placeholder shown until image loads or on error */}
          {(!coverLoaded || coverError) && (
            <CoverPlaceholder
              className={cn(
                "absolute inset-0 rounded-sm border border-neutral-300 dark:border-neutral-600",
              )}
              variant={isAudiobook ? "audiobook" : "book"}
            />
          )}
          {/* Image hidden until loaded, removed on error */}
          {!coverError && (
            <img
              alt={`${seriesItem.name} Cover`}
              className={cn(
                "w-full h-full object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1",
                !coverLoaded && "opacity-0",
              )}
              onError={() => setCoverError(true)}
              onLoad={handleCoverLoad}
              src={coverUrl}
            />
          )}
        </div>
        <Tooltip
          delayDuration={300}
          open={isTitleTruncated ? undefined : false}
        >
          <TooltipTrigger asChild>
            <div
              className="mt-2 group-hover:underline text-sm font-bold line-clamp-2"
              ref={titleRef}
            >
              {seriesItem.name}
            </div>
          </TooltipTrigger>
          <TooltipContent>{seriesItem.name}</TooltipContent>
        </Tooltip>
      </Link>
      <div className="mt-1 text-xs line-clamp-1 text-neutral-500 dark:text-neutral-500">
        <Badge className="text-xs" variant="secondary">
          {bookCount} book{bookCount !== 1 ? "s" : ""}
        </Badge>
      </div>
    </div>
  );
};

const SeriesList = () => {
  usePageTitle("Series");

  const { libraryId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const searchQuery = searchParams.get("search") ?? "";

  const [debouncedSearch, setDebouncedSearch] = useState(searchQuery);

  const handleDebouncedSearchChange = (value: string) => {
    setDebouncedSearch(value);
    if (value !== searchQuery) {
      setSearchParams((prev) => {
        const newParams = new URLSearchParams(prev);
        if (value) {
          newParams.set("search", value);
        } else {
          newParams.delete("search");
        }
        newParams.set("page", "1");
        return newParams;
      });
    }
  };

  const userSettingsQuery = useUserSettings();
  const updateUserSettings = useUpdateUserSettings();

  const urlSize: GallerySize | null = parseGallerySize(
    searchParams.get("size"),
  );
  const savedSize: GallerySize =
    userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;
  const effectiveSize: GallerySize = urlSize ?? savedSize;
  const isSizeDirty = urlSize !== null && urlSize !== savedSize;
  const itemsPerPage = ITEMS_PER_PAGE_BY_SIZE[effectiveSize];

  const userSettingsResolved =
    userSettingsQuery.isSuccess || userSettingsQuery.isError;

  const limit = itemsPerPage;
  const offset = (currentPage - 1) * limit;

  const applyGallerySize = (next: GallerySize) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      if (next === savedSize) {
        params.delete("size");
      } else {
        params.set("size", next);
      }
      const newPage = pageForSizeChange(offset, ITEMS_PER_PAGE_BY_SIZE[next]);
      params.set("page", String(newPage));
      return params;
    });
  };

  const handleSaveSizeAsDefault = () => {
    updateUserSettings.mutate(
      { gallery_size: effectiveSize },
      {
        onSuccess: () => {
          setSearchParams((prev) => {
            const params = new URLSearchParams(prev);
            params.delete("size");
            return params;
          });
        },
      },
    );
  };

  const libraryQuery = useLibrary(libraryId);
  const coverAspectRatio = libraryQuery.data?.cover_aspect_ratio ?? "book";

  const seriesQuery = useSeriesList(
    {
      limit,
      offset,
      library_id: libraryId ? parseInt(libraryId, 10) : undefined,
      search: debouncedSearch || undefined,
    },
    { enabled: userSettingsResolved },
  );

  // Track the search value that produced the currently displayed data
  const [confirmedSearch, setConfirmedSearch] = useState<string | null>(null);

  useEffect(() => {
    if (seriesQuery.isSuccess && !seriesQuery.isFetching) {
      setConfirmedSearch(debouncedSearch);
    }
  }, [seriesQuery.isSuccess, seriesQuery.isFetching, debouncedSearch]);

  // Data is stale if search changed but query hasn't completed yet
  const isStaleData =
    confirmedSearch !== null && debouncedSearch !== confirmedSearch;

  const renderSeriesItem = (seriesItem: Series) => {
    const aspectClass = getSeriesAspectRatioClass(coverAspectRatio);
    const isAudiobook = coverAspectRatio.startsWith("audiobook");

    return (
      <SeriesCard
        aspectClass={aspectClass}
        cacheKey={seriesQuery.dataUpdatedAt}
        gallerySize={effectiveSize}
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

      <div className="mb-6 flex flex-wrap items-center gap-3">
        <SearchInput
          initialValue={searchQuery}
          onDebouncedChange={handleDebouncedSearchChange}
          placeholder="Search series..."
        />
        <div className="hidden sm:flex">
          <SizePopover
            effectiveSize={effectiveSize}
            isSaving={updateUserSettings.isPending}
            onChange={applyGallerySize}
            onSaveAsDefault={handleSaveSizeAsDefault}
            savedSize={savedSize}
            trigger={<SizeButton isDirty={isSizeDirty} />}
          />
        </div>
      </div>

      <Gallery
        emptyMessage={
          confirmedSearch
            ? "No series found matching your search."
            : "No series in this library yet."
        }
        isLoading={
          seriesQuery.isLoading || seriesQuery.isFetching || isStaleData
        }
        isSuccess={
          seriesQuery.isSuccess && !seriesQuery.isFetching && !isStaleData
        }
        itemLabel="series"
        items={seriesQuery.data?.series ?? []}
        itemsPerPage={itemsPerPage}
        renderItem={renderSeriesItem}
        total={seriesQuery.data?.total ?? 0}
      />
    </LibraryLayout>
  );
};

export default SeriesList;
