import { Search, User, X } from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import CoverImage from "@/components/library/CoverImage";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useUserLibrary } from "@/hooks/queries/libraries";
import { useGlobalSearch } from "@/hooks/queries/search";
import { useCan } from "@/hooks/useCan";
import { useDebounce } from "@/hooks/useDebounce";
import { cn } from "@/libraries/utils";
import type {
  BookSearchResult,
  PersonSearchResult,
  SeriesSearchResult,
} from "@/types";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";
import { bookCoverUrl, seriesCoverUrl } from "@/utils/coverUrl";
import { isEbookFileType } from "@/utils/fileTypes";

const getSearchThumbnailClasses = (variant: "book" | "audiobook"): string => {
  // For search thumbnails, we use a fixed width and vary the aspect ratio
  return variant === "audiobook" ? "w-10 aspect-square" : "w-8 aspect-[2/3]";
};

// Determines which placeholder variant to show based on file types and library preference.
// This mirrors the backend's selectCoverFile priority logic.
const getPlaceholderVariant = (
  fileTypes: string[] | undefined,
  coverAspectRatio: string,
): "book" | "audiobook" => {
  if (!fileTypes || fileTypes.length === 0) return "book";

  const hasBookFiles = fileTypes.some(isEbookFileType);
  const hasAudiobookFiles = fileTypes.some((ft) => ft === "m4b");

  switch (coverAspectRatio) {
    case "audiobook":
    case "audiobook_fallback_book":
      if (hasAudiobookFiles) return "audiobook";
      if (hasBookFiles) return "book";
      break;
    default: // "book", "book_fallback_audiobook"
      if (hasBookFiles) return "book";
      if (hasAudiobookFiles) return "audiobook";
  }
  return "book";
};

interface SearchResultCoverProps {
  type: "book" | "series";
  id: number;
  thumbnailClasses: string;
  variant: "book" | "audiobook";
  /** When the results were fetched, standing in for `cover_cache_key`. */
  cacheKey: number;
}

const SearchResultCover = ({
  type,
  id,
  thumbnailClasses,
  variant,
  cacheKey,
}: SearchResultCoverProps) => {
  // Search results carry no cover_cache_key, so the covers are keyed on when
  // the results were fetched.
  const source = { id, cover_cache_key: String(cacheKey) };
  const coverUrl =
    type === "book" ? bookCoverUrl(source) : seriesCoverUrl(source);
  const [coverLoaded, setCoverLoaded] = useState(() => isCoverLoaded(coverUrl));
  const [coverError, setCoverError] = useState(false);

  // Reset error and re-seed loaded state when the URL changes so a
  // previously-failed cover can re-mount after the underlying image becomes
  // available (e.g., after a rescan), and so the loaded flag reflects the
  // current URL's cache status rather than whatever was set for a prior URL.
  useEffect(() => {
    setCoverError(false);
    setCoverLoaded(isCoverLoaded(coverUrl));
  }, [coverUrl]);

  const handleCoverLoad = () => {
    markCoverLoaded(coverUrl);
    setCoverLoaded(true);
  };

  return (
    <div
      className={cn(
        "flex-shrink-0 bg-muted rounded overflow-hidden relative",
        thumbnailClasses,
      )}
    >
      {/* Placeholder shown until image loads or on error */}
      {(!coverLoaded || coverError) && (
        <CoverPlaceholder className="absolute inset-0" variant={variant} />
      )}
      {/* Image hidden until loaded, removed on error */}
      {!coverError && (
        <CoverImage
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

interface ResultGroupProps {
  id: string;
  label: string;
  className?: string;
  children: React.ReactNode;
}

// A labelled group of results inside the listbox. The heading is hidden from
// the accessibility tree, since it names the group rather than being an
// option itself.
const ResultGroup = ({ id, label, className, children }: ResultGroupProps) => (
  <div aria-labelledby={id} className={cn("mb-2", className)} role="group">
    <div
      aria-hidden="true"
      className="px-3 py-1 text-xs font-semibold text-muted-foreground uppercase"
      id={id}
    >
      {label}
    </div>
    {children}
  </div>
);

interface GlobalSearchProps {
  fullWidth?: boolean;
  onClose?: () => void;
}

// The id of the result at `index` in the flat list the arrow keys move through.
const resultOptionId = (listboxId: string, index: number) =>
  `${listboxId}-option-${index}`;

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
  // The input is a combobox that keeps focus while the arrow keys move
  // through the results, so each result needs an id the input can point at
  // with aria-activedescendant.
  const listboxId = useId();

  const canReadBooks = useCan("books:read");
  const libraryQuery = useUserLibrary(libraryId);
  const coverAspectRatio = libraryQuery.data?.cover_aspect_ratio ?? "book";
  // Library-level variant used for series (which don't have file types)
  const libraryVariant: "book" | "audiobook" = coverAspectRatio.startsWith(
    "audiobook",
  )
    ? "audiobook"
    : "book";
  const seriesThumbnailClasses = getSearchThumbnailClasses(libraryVariant);

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
      if (event.key === "Tab") {
        // Results are not in the Tab order (the arrow keys reach them), so
        // Tab leaves the combobox and closes its list.
        setIsOpen(false);
      } else if (event.key === "ArrowDown") {
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
    (book: BookSearchResult, index: number) => {
      const variant = getPlaceholderVariant(book.file_types, coverAspectRatio);
      const thumbnailClasses = getSearchThumbnailClasses(variant);
      return (
        <Link
          aria-selected={selectedIndex === index}
          className={cn(
            "flex items-center gap-3 px-3 py-2 rounded-md",
            selectedIndex === index ? "bg-muted" : "hover:bg-muted",
          )}
          id={resultOptionId(listboxId, index)}
          key={`book-${book.id}`}
          onClick={handleResultClick}
          ref={(el) => {
            resultRefs.current[index] = el;
          }}
          role="option"
          tabIndex={-1}
          title={
            book.authors ? `${book.title}\nby ${book.authors}` : book.title
          }
          to={`/libraries/${libraryId}/books/${book.id}`}
        >
          <SearchResultCover
            cacheKey={searchQuery.dataUpdatedAt}
            id={book.id}
            thumbnailClasses={thumbnailClasses}
            type="book"
            variant={variant}
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
      );
    },
    [
      handleResultClick,
      libraryId,
      listboxId,
      coverAspectRatio,
      selectedIndex,
      searchQuery.dataUpdatedAt,
    ],
  );

  const renderSeriesResult = useCallback(
    (series: SeriesSearchResult, index: number) => (
      <Link
        aria-selected={selectedIndex === index}
        className={cn(
          "flex items-center gap-3 px-3 py-2 rounded-md",
          selectedIndex === index ? "bg-muted" : "hover:bg-muted",
        )}
        id={resultOptionId(listboxId, index)}
        key={`series-${series.id}`}
        onClick={handleResultClick}
        ref={(el) => {
          resultRefs.current[index] = el;
        }}
        role="option"
        tabIndex={-1}
        title={`${series.name}\n${series.book_count} book${series.book_count !== 1 ? "s" : ""}`}
        to={`/libraries/${libraryId}/series/${series.id}`}
      >
        <SearchResultCover
          cacheKey={searchQuery.dataUpdatedAt}
          id={series.id}
          thumbnailClasses={seriesThumbnailClasses}
          type="series"
          variant={libraryVariant}
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
      listboxId,
      seriesThumbnailClasses,
      libraryVariant,
      selectedIndex,
      searchQuery.dataUpdatedAt,
    ],
  );

  const renderPersonResult = useCallback(
    (person: PersonSearchResult, index: number) => (
      <Link
        aria-selected={selectedIndex === index}
        className={cn(
          "flex items-center gap-3 px-3 py-2 rounded-md",
          selectedIndex === index ? "bg-muted" : "hover:bg-muted",
        )}
        id={resultOptionId(listboxId, index)}
        key={`person-${person.id}`}
        onClick={handleResultClick}
        ref={(el) => {
          resultRefs.current[index] = el;
        }}
        role="option"
        tabIndex={-1}
        title={person.name}
        to={`/libraries/${libraryId}/people/${person.id}`}
      >
        <User className="h-4 w-4 text-muted-foreground flex-shrink-0" />
        <div className="flex-1 min-w-0">
          <div className="font-medium truncate">{person.name}</div>
        </div>
      </Link>
    ),
    [handleResultClick, libraryId, listboxId, selectedIndex],
  );

  // Search reads Books Read routes, so a role without it gets no search box.
  if (!libraryId || !canReadBooks) {
    return null;
  }

  const isDropdownShown = isOpen && Boolean(debouncedQuery);
  const isListShown = isDropdownShown && searchQuery.isSuccess && hasResults;
  const statusText = !isDropdownShown
    ? null
    : searchQuery.isLoading
      ? "Searching..."
      : searchQuery.isSuccess && !hasResults
        ? `No results found for "${debouncedQuery}"`
        : null;
  const booksCount = searchQuery.data?.books?.length ?? 0;
  const seriesCount = searchQuery.data?.series?.length ?? 0;

  return (
    <div className="relative">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          aria-activedescendant={
            isListShown && selectedIndex >= 0
              ? resultOptionId(listboxId, selectedIndex)
              : undefined
          }
          aria-autocomplete="list"
          aria-controls={isListShown ? listboxId : undefined}
          aria-expanded={Boolean(isListShown)}
          aria-label="Search library"
          className={cn(
            "pl-9 [&::-webkit-search-cancel-button]:hidden",
            fullWidth
              ? "w-full pr-3 focus-visible:ring-0 focus-visible:border-border"
              : "w-64 pr-8",
          )}
          onBlur={(e) => {
            // Leaving for anything but a result closes the list, so
            // aria-expanded never claims a popup the user cannot reach.
            if (!dropdownRef.current?.contains(e.relatedTarget as Node)) {
              setIsOpen(false);
            }
          }}
          onChange={(e) => {
            setQuery(e.target.value);
            setIsOpen(true);
          }}
          onFocus={() => setIsOpen(true)}
          onKeyDown={handleKeyDown}
          placeholder="Search library..."
          ref={inputRef}
          role="combobox"
          type="search"
          value={query}
        />
        {query && !fullWidth && (
          <Button
            aria-label="Clear search"
            className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground"
            onClick={() => {
              setQuery("");
              inputRef.current?.focus();
            }}
            size="icon-xs"
            variant="ghost"
          >
            <X className="h-4 w-4" />
          </Button>
        )}
      </div>

      {/* The announced copy of the dropdown's message. It stays mounted, so
          a new message is announced; a region mounted with its text often is
          not. */}
      <div className="sr-only" role="status">
        {statusText}
      </div>

      {isDropdownShown && (
        <div
          className={cn(
            "bg-background border border-border rounded-md shadow-lg z-50 max-h-96 overflow-y-auto",
            fullWidth
              ? "fixed left-4 right-4 top-28"
              : "absolute top-full mt-2 left-0 w-80",
          )}
          // Keep focus in the input for clicks inside the list (a heading,
          // padding, or a result), so its blur does not close the list.
          onMouseDown={(e) => e.preventDefault()}
          ref={dropdownRef}
        >
          {statusText && (
            <div
              aria-hidden="true"
              className="p-4 text-center text-muted-foreground"
            >
              {statusText}
            </div>
          )}

          {isListShown && (
            <div
              aria-label="Search results"
              className="p-2"
              id={listboxId}
              role="listbox"
            >
              {booksCount > 0 && (
                <ResultGroup id={`${listboxId}-books`} label="Books">
                  {searchQuery.data.books?.map((book, i) =>
                    renderBookResult(book, i),
                  )}
                </ResultGroup>
              )}

              {seriesCount > 0 && (
                <ResultGroup id={`${listboxId}-series`} label="Series">
                  {searchQuery.data.series?.map((series, i) =>
                    renderSeriesResult(series, booksCount + i),
                  )}
                </ResultGroup>
              )}

              {(searchQuery.data.people?.length ?? 0) > 0 && (
                <ResultGroup
                  className="mb-0"
                  id={`${listboxId}-people`}
                  label="People"
                >
                  {searchQuery.data.people?.map((person, i) =>
                    renderPersonResult(person, booksCount + seriesCount + i),
                  )}
                </ResultGroup>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default GlobalSearch;
