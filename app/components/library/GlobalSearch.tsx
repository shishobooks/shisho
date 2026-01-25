import { Search, User, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
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
  const [coverError, setCoverError] = useState(false);

  return (
    <div
      className={cn(
        "flex-shrink-0 bg-neutral-200 dark:bg-neutral-700 rounded overflow-hidden",
        thumbnailClasses,
      )}
    >
      {!coverError ? (
        <img
          alt=""
          className="w-full h-full object-cover"
          onError={() => setCoverError(true)}
          src={`/api/${type === "book" ? "books" : "series"}/${id}/cover?t=${cacheBuster}`}
        />
      ) : (
        <CoverPlaceholder variant={variant} />
      )}
    </div>
  );
};

const GlobalSearch = () => {
  const { libraryId } = useParams();
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [isOpen, setIsOpen] = useState(false);
  const debouncedQuery = useDebounce(query, 300);
  const inputRef = useRef<HTMLInputElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

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
  }, []);

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

  // Handle Enter key to navigate to first result
  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLInputElement>) => {
      if (event.key === "Enter") {
        const firstResultUrl = getFirstResultUrl();
        if (firstResultUrl) {
          event.preventDefault();
          setIsOpen(false);
          setQuery("");
          navigate(firstResultUrl);
        }
      }
    },
    [getFirstResultUrl, navigate],
  );

  const renderBookResult = (book: BookSearchResult) => (
    <Link
      className="flex items-center gap-3 px-3 py-2 hover:bg-neutral-100 dark:hover:bg-neutral-800 rounded-md"
      key={`book-${book.id}`}
      onClick={handleResultClick}
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
  );

  const renderSeriesResult = (series: SeriesSearchResult) => (
    <Link
      className="flex items-center gap-3 px-3 py-2 hover:bg-neutral-100 dark:hover:bg-neutral-800 rounded-md"
      key={`series-${series.id}`}
      onClick={handleResultClick}
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
  );

  const renderPersonResult = (person: PersonSearchResult) => (
    <Link
      className="flex items-center gap-3 px-3 py-2 hover:bg-neutral-100 dark:hover:bg-neutral-800 rounded-md"
      key={`person-${person.id}`}
      onClick={handleResultClick}
      title={person.name}
      to={`/libraries/${libraryId}/people/${person.id}`}
    >
      <User className="h-4 w-4 text-muted-foreground flex-shrink-0" />
      <div className="flex-1 min-w-0">
        <div className="font-medium truncate">{person.name}</div>
      </div>
    </Link>
  );

  if (!libraryId) {
    return null;
  }

  return (
    <div className="relative">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          className="w-64 pl-9 pr-8"
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
        {query && (
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
          className="absolute top-full left-0 mt-2 w-80 bg-background border border-border rounded-lg shadow-lg z-50 max-h-96 overflow-y-auto"
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
                  {searchQuery.data.books?.map(renderBookResult)}
                </div>
              )}

              {(searchQuery.data.series?.length ?? 0) > 0 && (
                <div className="mb-2">
                  <div className="px-3 py-1 text-xs font-semibold text-muted-foreground uppercase">
                    Series
                  </div>
                  {searchQuery.data.series?.map(renderSeriesResult)}
                </div>
              )}

              {(searchQuery.data.people?.length ?? 0) > 0 && (
                <div>
                  <div className="px-3 py-1 text-xs font-semibold text-muted-foreground uppercase">
                    People
                  </div>
                  {searchQuery.data.people?.map(renderPersonResult)}
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
