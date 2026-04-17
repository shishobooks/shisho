import { CheckSquare } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import { ActiveFilterChips } from "@/components/library/ActiveFilterChips";
import { FilterSheet } from "@/components/library/FilterSheet";
import Gallery from "@/components/library/Gallery";
import LibraryLayout from "@/components/library/LibraryLayout";
import { SearchInput } from "@/components/library/SearchInput";
import { SelectableBookItem } from "@/components/library/SelectableBookItem";
import { SelectionToolbar } from "@/components/library/SelectionToolbar";
import { Button } from "@/components/ui/button";
import { getLanguageName } from "@/constants/languages";
import { BulkSelectionProvider } from "@/contexts/BulkSelection";
import { useBooks } from "@/hooks/queries/books";
import { useGenresList } from "@/hooks/queries/genres";
import { useLibrary, useLibraryLanguages } from "@/hooks/queries/libraries";
import { useSeries } from "@/hooks/queries/series";
import { useTagsList } from "@/hooks/queries/tags";
import { useBulkSelection } from "@/hooks/useBulkSelection";
import { useDebounce } from "@/hooks/useDebounce";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { Book, Genre, Tag } from "@/types";

const ITEMS_PER_PAGE = 24;

const FILE_TYPE_OPTIONS = [
  { value: "epub", label: "EPUB" },
  { value: "m4b", label: "M4B" },
  { value: "cbz", label: "CBZ" },
  { value: "pdf", label: "PDF" },
];

const HomeContent = () => {
  const { libraryId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const libraryQuery = useLibrary(libraryId);

  usePageTitle(libraryQuery.data?.name ?? "Books");
  const { isSelectionMode, enterSelectionMode, exitSelectionMode } =
    useBulkSelection();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const seriesIdParam = searchParams.get("series_id");
  const searchQuery = searchParams.get("search") ?? "";
  const fileTypesParam = searchParams.get("file_types") ?? "";
  const genreIdsParam = searchParams.get("genre_ids") ?? "";
  const tagIdsParam = searchParams.get("tag_ids") ?? "";
  const languageParam = searchParams.get("language") ?? "";

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

  // Fetch distinct languages for the library
  const libraryIdNum = libraryId ? parseInt(libraryId, 10) : undefined;
  const languagesQuery = useLibraryLanguages(libraryIdNum);

  // Group languages by base subtag for the filter dropdown.
  // If a library has both "en" and "en-US", show only "English" (the bare "en" subsumes variants).
  // If it has "en-US" and "en-GB" (but no bare "en"), show them separately.
  const languageOptions = useMemo(() => {
    const rawLanguages = languagesQuery.data ?? [];
    if (rawLanguages.length < 2) return [];

    // Group tags by their base language subtag
    const groups = new Map<string, string[]>();
    for (const tag of rawLanguages) {
      const base = tag.split("-")[0];
      const existing = groups.get(base) ?? [];
      existing.push(tag);
      groups.set(base, existing);
    }

    const options: { value: string; label: string }[] = [];
    for (const [base, tags] of groups) {
      if (tags.includes(base)) {
        // Bare base tag exists — collapse all variants under it
        const label = getLanguageName(base) ?? base;
        options.push({ value: base, label });
      } else {
        // No bare base tag — show each variant separately
        for (const tag of tags) {
          const label = getLanguageName(tag) ?? tag;
          options.push({ value: tag, label });
        }
      }
    }

    return options.sort((a, b) => a.label.localeCompare(b.label));
  }, [languagesQuery.data]);

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

  // Set language filter
  const setLanguageFilter = (value: string) => {
    setSearchParams((prev) => {
      const newParams = new URLSearchParams(prev);
      if (value && value !== "all") {
        newParams.set("language", value);
      } else {
        newParams.delete("language");
      }
      newParams.set("page", "1");
      return newParams;
    });
  };

  const clearAllFilters = () => {
    setSearchParams((prev) => {
      const newParams = new URLSearchParams(prev);
      newParams.delete("file_types");
      newParams.delete("genre_ids");
      newParams.delete("tag_ids");
      newParams.delete("language");
      newParams.set("page", "1");
      return newParams;
    });
  };

  const hasActiveFilters =
    selectedFileTypes.length > 0 ||
    selectedGenreIds.length > 0 ||
    selectedTagIds.length > 0 ||
    Boolean(languageParam);

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

  // Add language filter if present
  if (languageParam) {
    booksQueryParams.language = languageParam;
  }

  const booksQuery = useBooks(booksQueryParams);

  // Track the filter state that produced the currently displayed data
  // We use a stringified version of all filter params for comparison
  const currentFilterKey = JSON.stringify({
    search: debouncedSearch,
    fileTypes: selectedFileTypes,
    genreIds: selectedGenreIds,
    tagIds: selectedTagIds,
    language: languageParam,
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
          selectedTagIds.length > 0 ||
          languageParam !== "",
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
    languageParam,
  ]);

  // Data is stale if filters changed but query hasn't completed yet
  const isStaleData =
    confirmedFilterKey !== null && currentFilterKey !== confirmedFilterKey;

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
      <div className="mb-6 space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <SearchInput
            initialValue={searchQuery}
            onDebouncedChange={handleDebouncedSearchChange}
            placeholder="Search books..."
          />
          <FilterSheet
            selectedFileTypes={selectedFileTypes}
            fileTypeOptions={FILE_TYPE_OPTIONS}
            onToggleFileType={toggleFileType}
            selectedGenreIds={selectedGenreIds}
            genres={genres}
            genresLoading={genresQuery.isLoading}
            genresError={genresQuery.isError}
            genreSearchInput={genreSearchInput}
            onGenreSearchChange={setGenreSearchInput}
            onToggleGenre={toggleGenreFilter}
            selectedTagIds={selectedTagIds}
            tags={tags}
            tagsLoading={tagsQuery.isLoading}
            tagsError={tagsQuery.isError}
            tagSearchInput={tagSearchInput}
            onTagSearchChange={setTagSearchInput}
            onToggleTag={toggleTagFilter}
            languageParam={languageParam}
            languageOptions={languageOptions}
            onLanguageChange={setLanguageFilter}
            onClearAll={clearAllFilters}
            hasActiveFilters={hasActiveFilters}
          />
          <div className="flex-1" />
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
        <ActiveFilterChips
          selectedFileTypes={selectedFileTypes}
          selectedGenres={selectedGenres}
          selectedTags={selectedTags}
          languageParam={languageParam}
          onToggleFileType={toggleFileType}
          onToggleGenre={toggleGenreFilter}
          onToggleTag={toggleTagFilter}
          onClearLanguage={() => setLanguageFilter("all")}
          onClearAll={clearAllFilters}
        />
      </div>

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
      <SelectionToolbar library={libraryQuery.data} />
    </LibraryLayout>
  );
};

const Home = () => (
  <BulkSelectionProvider>
    <HomeContent />
  </BulkSelectionProvider>
);

export default Home;
