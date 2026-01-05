import { useEffect, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import Gallery from "@/components/library/Gallery";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { useBooks } from "@/hooks/queries/books";
import { useLibrary } from "@/hooks/queries/libraries";
import { useSeries } from "@/hooks/queries/series";
import { useDebounce } from "@/hooks/useDebounce";
import type { Book } from "@/types";

const ITEMS_PER_PAGE = 24;

const FILE_TYPE_OPTIONS = [
  { value: "epub", label: "EPUB" },
  { value: "m4b", label: "M4B" },
  { value: "cbz", label: "CBZ" },
];

const Home = () => {
  const { libraryId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const seriesIdParam = searchParams.get("series_id");
  const searchQuery = searchParams.get("search") ?? "";
  const fileTypesParam = searchParams.get("file_types") ?? "";

  const [searchInput, setSearchInput] = useState(searchQuery);
  const debouncedSearch = useDebounce(searchInput, 300);

  // Sync searchInput with URL when searchQuery changes (e.g., when clicking nav links)
  useEffect(() => {
    setSearchInput(searchQuery);
  }, [searchQuery]);

  // Parse file types from URL
  const selectedFileTypes = fileTypesParam
    ? fileTypesParam.split(",").filter(Boolean)
    : [];

  // Calculate pagination parameters
  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const seriesId = seriesIdParam ? parseInt(seriesIdParam, 10) : undefined;

  // Update URL when debounced search changes
  const updateSearchParams = (
    newSearch: string,
    newFileTypes: string[],
    resetPage: boolean = true,
  ) => {
    const newParams = new URLSearchParams(searchParams);
    if (newSearch) {
      newParams.set("search", newSearch);
    } else {
      newParams.delete("search");
    }
    if (newFileTypes.length > 0) {
      newParams.set("file_types", newFileTypes.join(","));
    } else {
      newParams.delete("file_types");
    }
    if (resetPage) {
      newParams.set("page", "1");
    }
    setSearchParams(newParams);
  };

  // Handle search input change
  const handleSearchChange = (value: string) => {
    setSearchInput(value);
    // Update URL after debounce
    setTimeout(() => {
      if (value !== searchQuery) {
        updateSearchParams(value, selectedFileTypes);
      }
    }, 300);
  };

  // Toggle file type filter
  const toggleFileType = (fileType: string) => {
    const newFileTypes = selectedFileTypes.includes(fileType)
      ? selectedFileTypes.filter((ft) => ft !== fileType)
      : [...selectedFileTypes, fileType];
    updateSearchParams(searchInput, newFileTypes);
  };

  // Build query with search and file types
  const booksQueryParams: Parameters<typeof useBooks>[0] = {
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    series_id: seriesId,
  };

  // Add search if present
  if (debouncedSearch) {
    booksQueryParams.search = debouncedSearch;
  }

  // Add file types if present
  if (selectedFileTypes.length > 0) {
    booksQueryParams.file_types = selectedFileTypes;
  }

  const booksQuery = useBooks(booksQueryParams);

  const libraryQuery = useLibrary(libraryId);
  const coverAspectRatio = libraryQuery.data?.cover_aspect_ratio ?? "book";

  const seriesQuery = useSeries(seriesId, {
    enabled: Boolean(seriesId),
  });

  const renderBookItem = (book: Book) => (
    <BookItem
      book={book}
      coverAspectRatio={coverAspectRatio}
      key={book.id}
      libraryId={libraryId!}
      seriesId={seriesId}
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

        {/* Search and Filters */}
        <div className="mb-6 flex flex-wrap items-center gap-4">
          <Input
            className="max-w-xs"
            onChange={(e) => handleSearchChange(e.target.value)}
            placeholder="Search books..."
            type="search"
            value={searchInput}
          />
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">File types:</span>
            {FILE_TYPE_OPTIONS.map((option) => (
              <Badge
                className="cursor-pointer"
                key={option.value}
                onClick={() => toggleFileType(option.value)}
                variant={
                  selectedFileTypes.includes(option.value)
                    ? "default"
                    : "outline"
                }
              >
                {option.label}
              </Badge>
            ))}
          </div>
        </div>

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
