import {
  CheckSquare,
  ChevronsUpDown,
  Square,
  SquareCheckBig,
  X,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import Gallery from "@/components/library/Gallery";
import LibraryLayout from "@/components/library/LibraryLayout";
import { SearchInput } from "@/components/library/SearchInput";
import { SelectableBookItem } from "@/components/library/SelectableBookItem";
import { SelectionToolbar } from "@/components/library/SelectionToolbar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { BulkSelectionProvider } from "@/contexts/BulkSelection";
import { useBooks } from "@/hooks/queries/books";
import { useGenresList } from "@/hooks/queries/genres";
import { useLibrary } from "@/hooks/queries/libraries";
import { useSeries } from "@/hooks/queries/series";
import { useTagsList } from "@/hooks/queries/tags";
import { useBulkSelection } from "@/hooks/useBulkSelection";
import { useDebounce } from "@/hooks/useDebounce";
import type { Book, Genre, Tag } from "@/types";

const ITEMS_PER_PAGE = 24;

const FILE_TYPE_OPTIONS = [
  { value: "epub", label: "EPUB" },
  { value: "m4b", label: "M4B" },
  { value: "cbz", label: "CBZ" },
];

const HomeContent = () => {
  const { libraryId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const { isSelectionMode, enterSelectionMode, exitSelectionMode } =
    useBulkSelection();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const seriesIdParam = searchParams.get("series_id");
  const searchQuery = searchParams.get("search") ?? "";
  const fileTypesParam = searchParams.get("file_types") ?? "";
  const genreIdsParam = searchParams.get("genre_ids") ?? "";
  const tagIdsParam = searchParams.get("tag_ids") ?? "";

  const [debouncedSearch, setDebouncedSearch] = useState(searchQuery);

  // Callback for SearchInput - memoized to prevent unnecessary re-renders
  const handleDebouncedSearchChange = useMemo(
    () => (value: string) => {
      setDebouncedSearch(value);
      // Sync to URL when debounced value changes
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
    },
    [searchQuery, setSearchParams],
  );

  // Parse file types from URL
  const selectedFileTypes = fileTypesParam
    ? fileTypesParam.split(",").filter(Boolean)
    : [];

  // Parse genre IDs from URL
  const selectedGenreIds = genreIdsParam
    ? genreIdsParam
        .split(",")
        .filter(Boolean)
        .map((id) => parseInt(id, 10))
    : [];

  // Parse tag IDs from URL
  const selectedTagIds = tagIdsParam
    ? tagIdsParam
        .split(",")
        .filter(Boolean)
        .map((id) => parseInt(id, 10))
    : [];

  // State for filter popovers
  const [fileTypePopoverOpen, setFileTypePopoverOpen] = useState(false);
  const [genrePopoverOpen, setGenrePopoverOpen] = useState(false);
  const [tagPopoverOpen, setTagPopoverOpen] = useState(false);

  // State for genre/tag search inputs
  const [genreSearchInput, setGenreSearchInput] = useState("");
  const [tagSearchInput, setTagSearchInput] = useState("");
  const debouncedGenreSearch = useDebounce(genreSearchInput, 300);
  const debouncedTagSearch = useDebounce(tagSearchInput, 300);

  // Fetch genres and tags for the filter dropdowns
  const genresQuery = useGenresList({
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    limit: 50,
    search: debouncedGenreSearch || undefined,
  });
  const tagsQuery = useTagsList({
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    limit: 50,
    search: debouncedTagSearch || undefined,
  });

  const genres = useMemo(
    () => genresQuery.data?.genres ?? [],
    [genresQuery.data?.genres],
  );
  const tags = useMemo(
    () => tagsQuery.data?.tags ?? [],
    [tagsQuery.data?.tags],
  );

  // Cache genre/tag objects so selected items persist across searches
  const [genreCache, setGenreCache] = useState<Map<number, Genre>>(new Map());
  const [tagCache, setTagCache] = useState<Map<number, Tag>>(new Map());

  // Update cache when genres load
  useEffect(() => {
    if (genres.length > 0) {
      setGenreCache((prev) => {
        const next = new Map(prev);
        genres.forEach((g) => next.set(g.id, g));
        return next;
      });
    }
  }, [genres]);

  // Update cache when tags load
  useEffect(() => {
    if (tags.length > 0) {
      setTagCache((prev) => {
        const next = new Map(prev);
        tags.forEach((t) => next.set(t.id, t));
        return next;
      });
    }
  }, [tags]);

  // Get selected genre/tag objects from cache for display
  const selectedGenres = selectedGenreIds
    .map((id) => genreCache.get(id))
    .filter((g): g is Genre => g !== undefined);
  const selectedTags = selectedTagIds
    .map((id) => tagCache.get(id))
    .filter((t): t is Tag => t !== undefined);

  // Calculate pagination parameters
  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const seriesId = seriesIdParam ? parseInt(seriesIdParam, 10) : undefined;

  // Toggle file type filter
  const toggleFileType = (fileType: string) => {
    const newFileTypes = selectedFileTypes.includes(fileType)
      ? selectedFileTypes.filter((ft) => ft !== fileType)
      : [...selectedFileTypes, fileType];
    setSearchParams((prev) => {
      const newParams = new URLSearchParams(prev);
      if (newFileTypes.length > 0) {
        newParams.set("file_types", newFileTypes.join(","));
      } else {
        newParams.delete("file_types");
      }
      newParams.set("page", "1");
      return newParams;
    });
  };

  // Toggle genre filter
  const toggleGenreFilter = (genreId: number) => {
    const newParams = new URLSearchParams(searchParams);
    const newGenreIds = selectedGenreIds.includes(genreId)
      ? selectedGenreIds.filter((id) => id !== genreId)
      : [...selectedGenreIds, genreId];

    if (newGenreIds.length > 0) {
      newParams.set("genre_ids", newGenreIds.join(","));
    } else {
      newParams.delete("genre_ids");
    }
    newParams.set("page", "1");
    setSearchParams(newParams);
  };

  // Toggle tag filter
  const toggleTagFilter = (tagId: number) => {
    const newParams = new URLSearchParams(searchParams);
    const newTagIds = selectedTagIds.includes(tagId)
      ? selectedTagIds.filter((id) => id !== tagId)
      : [...selectedTagIds, tagId];

    if (newTagIds.length > 0) {
      newParams.set("tag_ids", newTagIds.join(","));
    } else {
      newParams.delete("tag_ids");
    }
    newParams.set("page", "1");
    setSearchParams(newParams);
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

  // Add genre IDs if present
  if (selectedGenreIds.length > 0) {
    booksQueryParams.genre_ids = selectedGenreIds;
  }

  // Add tag IDs if present
  if (selectedTagIds.length > 0) {
    booksQueryParams.tag_ids = selectedTagIds;
  }

  const booksQuery = useBooks(booksQueryParams);

  // Track the filter state that produced the currently displayed data
  // We use a stringified version of all filter params for comparison
  const currentFilterKey = JSON.stringify({
    search: debouncedSearch,
    fileTypes: selectedFileTypes,
    genreIds: selectedGenreIds,
    tagIds: selectedTagIds,
  });
  const [confirmedFilterKey, setConfirmedFilterKey] = useState<string | null>(
    null,
  );

  // Track whether the confirmed query had any filters applied
  const [confirmedHasFilters, setConfirmedHasFilters] = useState(false);

  useEffect(() => {
    if (booksQuery.isSuccess && !booksQuery.isFetching) {
      setConfirmedFilterKey(currentFilterKey);
      setConfirmedHasFilters(
        debouncedSearch !== "" ||
          selectedFileTypes.length > 0 ||
          selectedGenreIds.length > 0 ||
          selectedTagIds.length > 0,
      );
    }
  }, [
    booksQuery.isSuccess,
    booksQuery.isFetching,
    currentFilterKey,
    debouncedSearch,
    selectedFileTypes.length,
    selectedGenreIds.length,
    selectedTagIds.length,
  ]);

  // Data is stale if filters changed but query hasn't completed yet
  const isStaleData =
    confirmedFilterKey !== null && currentFilterKey !== confirmedFilterKey;

  const libraryQuery = useLibrary(libraryId);
  const coverAspectRatio = libraryQuery.data?.cover_aspect_ratio ?? "book";

  const seriesQuery = useSeries(seriesId, {
    enabled: Boolean(seriesId),
  });

  const pageBookIds = useMemo(
    () => booksQuery.data?.books?.map((b) => b.id) ?? [],
    [booksQuery.data?.books],
  );

  const renderBookItem = (book: Book) => (
    <SelectableBookItem
      book={book}
      coverAspectRatio={coverAspectRatio}
      key={book.id}
      libraryId={libraryId!}
      pageBookIds={pageBookIds}
      seriesId={seriesId}
    />
  );

  return (
    <LibraryLayout>
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
        <SearchInput
          initialValue={searchQuery}
          onDebouncedChange={handleDebouncedSearchChange}
          placeholder="Search books..."
        />
        {/* File Type Filter */}
        <Popover
          onOpenChange={setFileTypePopoverOpen}
          open={fileTypePopoverOpen}
        >
          <PopoverTrigger asChild>
            <Button
              aria-expanded={fileTypePopoverOpen}
              className="justify-between"
              role="combobox"
              variant="outline"
            >
              {selectedFileTypes.length > 0
                ? `${selectedFileTypes.length} file type${selectedFileTypes.length > 1 ? "s" : ""}`
                : "File types"}
              <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-[200px] p-0">
            <Command>
              <CommandList>
                <CommandGroup>
                  {FILE_TYPE_OPTIONS.map((option) => (
                    <CommandItem
                      key={option.value}
                      onSelect={() => toggleFileType(option.value)}
                      value={option.value}
                    >
                      {selectedFileTypes.includes(option.value) ? (
                        <SquareCheckBig className="mr-2 h-4 w-4" />
                      ) : (
                        <Square className="mr-2 h-4 w-4" />
                      )}
                      {option.label}
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>

        {/* Genre Filter */}
        <Popover onOpenChange={setGenrePopoverOpen} open={genrePopoverOpen}>
          <PopoverTrigger asChild>
            <Button
              aria-expanded={genrePopoverOpen}
              className="justify-between"
              role="combobox"
              variant="outline"
            >
              {selectedGenres.length > 0
                ? `${selectedGenres.length} genre${selectedGenres.length > 1 ? "s" : ""}`
                : "Genres"}
              <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-[200px] p-0">
            <Command shouldFilter={false}>
              <CommandInput
                onValueChange={setGenreSearchInput}
                placeholder="Search genres..."
                value={genreSearchInput}
              />
              <CommandList>
                {genresQuery.isLoading ? (
                  <div className="py-6 text-center text-sm text-muted-foreground">
                    Loading...
                  </div>
                ) : genresQuery.isError ? (
                  <div className="py-6 text-center text-sm text-destructive">
                    Error loading genres
                  </div>
                ) : genres.length === 0 ? (
                  <CommandEmpty>No genres found.</CommandEmpty>
                ) : (
                  <CommandGroup>
                    {genres.map((genre: Genre) => (
                      <CommandItem
                        key={genre.id}
                        onSelect={() => toggleGenreFilter(genre.id)}
                        value={genre.name}
                      >
                        {selectedGenreIds.includes(genre.id) ? (
                          <SquareCheckBig className="mr-2 h-4 w-4" />
                        ) : (
                          <Square className="mr-2 h-4 w-4" />
                        )}
                        {genre.name}
                        <span className="ml-auto text-xs text-muted-foreground">
                          {genre.book_count}
                        </span>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )}
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>

        {/* Tag Filter */}
        <Popover onOpenChange={setTagPopoverOpen} open={tagPopoverOpen}>
          <PopoverTrigger asChild>
            <Button
              aria-expanded={tagPopoverOpen}
              className="justify-between"
              role="combobox"
              variant="outline"
            >
              {selectedTags.length > 0
                ? `${selectedTags.length} tag${selectedTags.length > 1 ? "s" : ""}`
                : "Tags"}
              <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-[200px] p-0">
            <Command shouldFilter={false}>
              <CommandInput
                onValueChange={setTagSearchInput}
                placeholder="Search tags..."
                value={tagSearchInput}
              />
              <CommandList>
                {tagsQuery.isLoading ? (
                  <div className="py-6 text-center text-sm text-muted-foreground">
                    Loading...
                  </div>
                ) : tagsQuery.isError ? (
                  <div className="py-6 text-center text-sm text-destructive">
                    Error loading tags
                  </div>
                ) : tags.length === 0 ? (
                  <CommandEmpty>No tags found.</CommandEmpty>
                ) : (
                  <CommandGroup>
                    {tags.map((tag: Tag) => (
                      <CommandItem
                        key={tag.id}
                        onSelect={() => toggleTagFilter(tag.id)}
                        value={tag.name}
                      >
                        {selectedTagIds.includes(tag.id) ? (
                          <SquareCheckBig className="mr-2 h-4 w-4" />
                        ) : (
                          <Square className="mr-2 h-4 w-4" />
                        )}
                        {tag.name}
                        <span className="ml-auto text-xs text-muted-foreground">
                          {tag.book_count}
                        </span>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )}
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
        {isSelectionMode ? (
          <Button onClick={exitSelectionMode} variant="outline">
            Cancel
          </Button>
        ) : (
          <Button onClick={enterSelectionMode} variant="outline">
            <CheckSquare className="h-4 w-4" />
            Select
          </Button>
        )}
      </div>

      {/* Active Filters */}
      {(selectedFileTypes.length > 0 ||
        selectedGenres.length > 0 ||
        selectedTags.length > 0) && (
        <div className="mb-6 flex flex-wrap items-center gap-2">
          <span className="text-sm text-muted-foreground">Filtering by:</span>
          {selectedFileTypes.map((fileType) => {
            const option = FILE_TYPE_OPTIONS.find((o) => o.value === fileType);
            return (
              <Badge
                className="cursor-pointer gap-1"
                key={fileType}
                onClick={() => toggleFileType(fileType)}
                variant="secondary"
              >
                {option?.label ?? fileType}
                <X className="h-3 w-3" />
              </Badge>
            );
          })}
          {selectedGenres.map((genre) => (
            <Badge
              className="cursor-pointer gap-1"
              key={genre.id}
              onClick={() => toggleGenreFilter(genre.id)}
              variant="secondary"
            >
              Genre: {genre.name}
              <X className="h-3 w-3" />
            </Badge>
          ))}
          {selectedTags.map((tag) => (
            <Badge
              className="cursor-pointer gap-1"
              key={tag.id}
              onClick={() => toggleTagFilter(tag.id)}
              variant="secondary"
            >
              Tag: {tag.name}
              <X className="h-3 w-3" />
            </Badge>
          ))}
        </div>
      )}

      <Gallery
        emptyMessage={
          confirmedHasFilters
            ? "No books found matching your search or filters."
            : "No books in this library yet."
        }
        isLoading={booksQuery.isLoading || booksQuery.isFetching || isStaleData}
        isSuccess={
          booksQuery.isSuccess && !booksQuery.isFetching && !isStaleData
        }
        itemLabel="books"
        items={booksQuery.data?.books ?? []}
        itemsPerPage={ITEMS_PER_PAGE}
        renderItem={renderBookItem}
        total={booksQuery.data?.total ?? 0}
      />
      <SelectionToolbar />
    </LibraryLayout>
  );
};

const Home = () => (
  <BulkSelectionProvider>
    <HomeContent />
  </BulkSelectionProvider>
);

export default Home;
