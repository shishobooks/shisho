import { Search, User, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { Input } from "@/components/ui/input";
import { useLibrary } from "@/hooks/queries/libraries";
import {
  useGlobalSearch,
  type BookSearchResult,
  type PersonSearchResult,
  type SeriesSearchResult,
} from "@/hooks/queries/search";
import { useDebounce } from "@/hooks/useDebounce";
import { cn } from "@/libraries/utils";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";

const getSearchThumbnailClasses = (coverAspectRatio: string): string => {
  // For search thumbnails, we use a fixed width and vary the aspect ratio
  switch (coverAspectRatio) {
    case "audiobook":
    case "audiobook_fallback_book":
      return "w-10 aspect-square";
    case "book":
    case "book_fallback_audiobook":
    default:
      return "w-8 aspect-[2/3]";
  }
};

interface SearchResultCoverProps {
  type: "book" | "series";
  id: number;
  thumbnailClasses: string;
  variant: "book" | "audiobook";
  cacheBuster: number;
}

const SearchResultCover = ({
  type,
  id,
  thumbnailClasses,
  variant,
  cacheBuster,
}: SearchResultCoverProps) => {
  const coverUrl = `/api/${type === "book" ? "books" : "series"}/${id}/cover?t=${cacheBuster}`;
  const [coverLoaded, setCoverLoaded] = useState(() => isCoverLoaded(coverUrl));
  const [coverError, setCoverError] = useState(false);

  const handleCoverLoad = () => {
    markCoverLoaded(coverUrl);
    setCoverLoaded(true);
  };

  return (
    <div
      className={cn(
        "flex-shrink-0 bg-neutral-200 dark:bg-neutral-700 rounded overflow-hidden relative",
        thumbnailClasses,
      )}
    >
      {/* Placeholder shown until image loads or on error */}
      {(!coverLoaded || coverError) && (
        <CoverPlaceholder className="absolute inset-0" variant={variant} />
      )}
      {/* Image hidden until loaded, removed on error */}
      {!coverError && (
        <img
          alt=""
          className={cn(
            "w-full h-full object-cover",
            !coverLoaded && "opacity-0",
          )}
          onError={() => setCoverError(true)}
          onLoad={handleCoverLoad}
          src={coverUrl}
        />
      )}
    </div>
  );
};

interface GlobalSearchProps {
  fullWidth?: boolean;
  onClose?: () => void;
}

type ResultItem =
  | { type: "book"; data: BookSearchResult }
  | { type: "series"; data: SeriesSearchResult }
  | { type: "person"; data: PersonSearchResult };

const GlobalSearch = ({ fullWidth = false, onClose }: GlobalSearchProps) => {
  const { libraryId } = useParams();
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [isOpen, setIsOpen] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const debouncedQuery = useDebounce(query, 300);
  const inputRef = useRef<HTMLInputElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const resultRefs = useRef<(HTMLAnchorElement | null)[]>([]);

  const libraryQuery = useLibrary(libraryId);
  const coverAspectRatio = libraryQuery.data?.cover_aspect_ratio ?? "book";
  const thumbnailClasses = getSearchThumbnailClasses(coverAspectRatio);
  const isAudiobook = coverAspectRatio.startsWith("audiobook");

  const searchQuery = useGlobalSearch(
    {
      q: debouncedQuery,
      library_id: libraryId ? parseInt(libraryId, 10) : 0,
    },
    {
      enabled: Boolean(debouncedQuery && libraryId),
    },
  );

  const hasResults =
    searchQuery.data &&
    ((searchQuery.data.books?.length ?? 0) > 0 ||
      (searchQuery.data.series?.length ?? 0) > 0 ||
      (searchQuery.data.people?.length ?? 0) > 0);

  // Build a flat list of all results for keyboard navigation
  const allResults = useMemo(() => {
    const results: ResultItem[] = [];
    if (searchQuery.data?.books) {
      for (const book of searchQuery.data.books) {
        results.push({ type: "book", data: book });
      }
    }
    if (searchQuery.data?.series) {
      for (const series of searchQuery.data.series) {
        results.push({ type: "series", data: series });
      }
    }
    if (searchQuery.data?.people) {
      for (const person of searchQuery.data.people) {
        results.push({ type: "person", data: person });
      }
    }
    return results;
  }, [searchQuery.data]);

  // Reset selected index when results change
  useEffect(() => {
    setSelectedIndex(-1);
  }, [searchQuery.data]);

  // Scroll selected item into view
  useEffect(() => {
    if (selectedIndex >= 0 && resultRefs.current[selectedIndex]) {
      resultRefs.current[selectedIndex]?.scrollIntoView({
        block: "nearest",
      });
    }
  }, [selectedIndex]);

  const getResultUrl = useCallback(
    (result: ResultItem): string => {
      switch (result.type) {
        case "book":
          return `/libraries/${libraryId}/books/${result.data.id}`;
        case "series":
          return `/libraries/${libraryId}/series/${result.data.id}`;
        case "person":
          return `/libraries/${libraryId}/people/${result.data.id}`;
      }
    },
    [libraryId],
  );

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(event.target as Node) &&
        inputRef.current &&
        !inputRef.current.contains(event.target as Node)
      ) {
        setIsOpen(false);
      }
    };

    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Close dropdown when pressing Escape
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setIsOpen(false);
        inputRef.current?.blur();
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, []);

  const handleResultClick = useCallback(() => {
    setIsOpen(false);
    setQuery("");
    onClose?.();
  }, [onClose]);

  // Get the URL of the first visible result
  // If there's exactly 1 result, go to the detail page
  // If there's more than 1 result, go to the list page with search query
  const getFirstResultUrl = useCallback((): string | null => {
    if (!searchQuery.data || !libraryId) return null;

    const searchParam = `?search=${encodeURIComponent(debouncedQuery)}&page=1`;

    // Check books first
    if (searchQuery.data.books && searchQuery.data.books.length > 0) {
      if (searchQuery.data.books.length === 1) {
        return `/libraries/${libraryId}/books/${searchQuery.data.books[0].id}`;
      }
      return `/libraries/${libraryId}${searchParam}`;
    }

    // Then series
    if (searchQuery.data.series && searchQuery.data.series.length > 0) {
      if (searchQuery.data.series.length === 1) {
        return `/libraries/${libraryId}/series/${searchQuery.data.series[0].id}`;
      }
      return `/libraries/${libraryId}/series${searchParam}`;
    }

    // Then people
    if (searchQuery.data.people && searchQuery.data.people.length > 0) {
      if (searchQuery.data.people.length === 1) {
        return `/libraries/${libraryId}/people/${searchQuery.data.people[0].id}`;
      }
      return `/libraries/${libraryId}/people${searchParam}`;
    }

    return null;
  }, [searchQuery.data, libraryId, debouncedQuery]);

  // Handle keyboard navigation
  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLInputElement>) => {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        if (allResults.length > 0) {
          setSelectedIndex((prev) =>
            prev < allResults.length - 1 ? prev + 1 : prev,
          );
        }
      } else if (event.key === "ArrowUp") {
        event.preventDefault();
        setSelectedIndex((prev) => (prev > 0 ? prev - 1 : prev));
      } else if (event.key === "Enter") {
        event.preventDefault();
        // If an item is selected, navigate to it
        if (selectedIndex >= 0 && selectedIndex < allResults.length) {
          const url = getResultUrl(allResults[selectedIndex]);
          setIsOpen(false);
          setQuery("");
          onClose?.();
          navigate(url);
        } else {
          // Fall back to first result behavior
          const firstResultUrl = getFirstResultUrl();
          if (firstResultUrl) {
            setIsOpen(false);
            setQuery("");
            onClose?.();
            navigate(firstResultUrl);
          }
        }
      }
    },
    [
      allResults,
      selectedIndex,
      getResultUrl,
      getFirstResultUrl,
      navigate,
      onClose,
    ],
  );

  const renderBookResult = useCallback(
    (book: BookSearchResult, index: number) => (
      <Link
        className={cn(
          "flex items-center gap-3 px-3 py-2 rounded-md",
          selectedIndex === index
            ? "bg-neutral-100 dark:bg-neutral-800"
            : "hover:bg-neutral-100 dark:hover:bg-neutral-800",
        )}
        key={`book-${book.id}`}
        onClick={handleResultClick}
        ref={(el) => {
          resultRefs.current[index] = el;
        }}
        title={book.authors ? `${book.title}\nby ${book.authors}` : book.title}
        to={`/libraries/${libraryId}/books/${book.id}`}
      >
        <SearchResultCover
          cacheBuster={searchQuery.dataUpdatedAt}
          id={book.id}
          thumbnailClasses={thumbnailClasses}
          type="book"
          variant={isAudiobook ? "audiobook" : "book"}
        />
        <div className="flex-1 min-w-0">
          <div className="font-medium truncate">{book.title}</div>
          {book.authors && (
            <div className="text-sm text-muted-foreground truncate">
              {book.authors}
            </div>
          )}
        </div>
      </Link>
    ),
    [
      handleResultClick,
      libraryId,
      searchQuery.dataUpdatedAt,
      thumbnailClasses,
      isAudiobook,
      selectedIndex,
    ],
  );

  const renderSeriesResult = useCallback(
    (series: SeriesSearchResult, index: number) => (
      <Link
        className={cn(
          "flex items-center gap-3 px-3 py-2 rounded-md",
          selectedIndex === index
            ? "bg-neutral-100 dark:bg-neutral-800"
            : "hover:bg-neutral-100 dark:hover:bg-neutral-800",
        )}
        key={`series-${series.id}`}
        onClick={handleResultClick}
        ref={(el) => {
          resultRefs.current[index] = el;
        }}
        title={`${series.name}\n${series.book_count} book${series.book_count !== 1 ? "s" : ""}`}
        to={`/libraries/${libraryId}/series/${series.id}`}
      >
        <SearchResultCover
          cacheBuster={searchQuery.dataUpdatedAt}
          id={series.id}
          thumbnailClasses={thumbnailClasses}
          type="series"
          variant={isAudiobook ? "audiobook" : "book"}
        />
        <div className="flex-1 min-w-0">
          <div className="font-medium truncate">{series.name}</div>
          <div className="text-sm text-muted-foreground">
            {series.book_count} book{series.book_count !== 1 ? "s" : ""}
          </div>
        </div>
      </Link>
    ),
    [
      handleResultClick,
      libraryId,
      searchQuery.dataUpdatedAt,
      thumbnailClasses,
      isAudiobook,
      selectedIndex,
    ],
  );

  const renderPersonResult = useCallback(
    (person: PersonSearchResult, index: number) => (
      <Link
        className={cn(
          "flex items-center gap-3 px-3 py-2 rounded-md",
          selectedIndex === index
            ? "bg-neutral-100 dark:bg-neutral-800"
            : "hover:bg-neutral-100 dark:hover:bg-neutral-800",
        )}
        key={`person-${person.id}`}
        onClick={handleResultClick}
        ref={(el) => {
          resultRefs.current[index] = el;
        }}
        title={person.name}
        to={`/libraries/${libraryId}/people/${person.id}`}
      >
        <User className="h-4 w-4 text-muted-foreground flex-shrink-0" />
        <div className="flex-1 min-w-0">
          <div className="font-medium truncate">{person.name}</div>
        </div>
      </Link>
    ),
    [handleResultClick, libraryId, selectedIndex],
  );

  if (!libraryId) {
    return null;
  }

  return (
    <div className="relative">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          className={cn(
            "pl-9 [&::-webkit-search-cancel-button]:hidden",
            fullWidth
              ? "w-full pr-3 focus-visible:ring-0 focus-visible:border-border"
              : "w-64 pr-8",
          )}
          onChange={(e) => {
            setQuery(e.target.value);
            setIsOpen(true);
          }}
          onFocus={() => setIsOpen(true)}
          onKeyDown={handleKeyDown}
          placeholder="Search library..."
          ref={inputRef}
          type="search"
          value={query}
        />
        {query && !fullWidth && (
          <button
            className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={() => {
              setQuery("");
              inputRef.current?.focus();
            }}
            type="button"
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </div>

      {isOpen && debouncedQuery && (
        <div
          className={cn(
            "bg-background border border-border rounded-lg shadow-lg z-50 max-h-96 overflow-y-auto",
            fullWidth
              ? "fixed left-4 right-4 top-28"
              : "absolute top-full mt-2 left-0 w-80",
          )}
          ref={dropdownRef}
        >
          {searchQuery.isLoading && (
            <div className="p-4 text-center text-muted-foreground">
              Searching...
            </div>
          )}

          {searchQuery.isSuccess && !hasResults && (
            <div className="p-4 text-center text-muted-foreground">
              No results found for &quot;{debouncedQuery}&quot;
            </div>
          )}

          {searchQuery.isSuccess && hasResults && (
            <div className="p-2">
              {(searchQuery.data.books?.length ?? 0) > 0 && (
                <div className="mb-2">
                  <div className="px-3 py-1 text-xs font-semibold text-muted-foreground uppercase">
                    Books
                  </div>
                  {searchQuery.data.books?.map((book, i) =>
                    renderBookResult(book, i),
                  )}
                </div>
              )}

              {(searchQuery.data.series?.length ?? 0) > 0 && (
                <div className="mb-2">
                  <div className="px-3 py-1 text-xs font-semibold text-muted-foreground uppercase">
                    Series
                  </div>
                  {searchQuery.data.series?.map((series, i) =>
                    renderSeriesResult(
                      series,
                      (searchQuery.data.books?.length ?? 0) + i,
                    ),
                  )}
                </div>
              )}

              {(searchQuery.data.people?.length ?? 0) > 0 && (
                <div>
                  <div className="px-3 py-1 text-xs font-semibold text-muted-foreground uppercase">
                    People
                  </div>
                  {searchQuery.data.people?.map((person, i) =>
                    renderPersonResult(
                      person,
                      (searchQuery.data.books?.length ?? 0) +
                        (searchQuery.data.series?.length ?? 0) +
                        i,
                    ),
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default GlobalSearch;
