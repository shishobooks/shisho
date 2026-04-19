import { CheckSquare, Loader2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import { ActiveFilterChips } from "@/components/library/ActiveFilterChips";
import { FilterSheet } from "@/components/library/FilterSheet";
import Gallery from "@/components/library/Gallery";
import LibraryLayout from "@/components/library/LibraryLayout";
import { SearchInput } from "@/components/library/SearchInput";
import { SelectableBookItem } from "@/components/library/SelectableBookItem";
import { SelectionToolbar } from "@/components/library/SelectionToolbar";
import SortedByChips from "@/components/library/SortedByChips";
import SortSheet, { SortButton } from "@/components/library/SortSheet";
import { Button } from "@/components/ui/button";
import { FILE_TYPE_OPTIONS } from "@/constants/fileTypes";
import { getLanguageName } from "@/constants/languages";
import { BulkSelectionProvider } from "@/contexts/BulkSelection";
import { useBooks } from "@/hooks/queries/books";
import { useGenresList } from "@/hooks/queries/genres";
import { useLibrary, useLibraryLanguages } from "@/hooks/queries/libraries";
import {
  useLibrarySettings,
  useUpdateLibrarySettings,
} from "@/hooks/queries/librarySettings";
import { useSeries } from "@/hooks/queries/series";
import { useTagsList } from "@/hooks/queries/tags";
import { useBulkSelection } from "@/hooks/useBulkSelection";
import { useDebounce } from "@/hooks/useDebounce";
import { usePageTitle } from "@/hooks/usePageTitle";
import {
  BUILTIN_DEFAULT_SORT,
  parseSortSpec,
  serializeSortSpec,
  sortSpecsEqual,
  type SortLevel,
} from "@/libraries/sortSpec";
import type { Book, Genre, Tag } from "@/types";

const ITEMS_PER_PAGE = 24;

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
  const sortParam = searchParams.get("sort") ?? "";
  const urlSort: SortLevel[] | null = sortParam
    ? parseSortSpec(sortParam)
    : null;

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

  // Fetch per-library sort preference. The query is auto-disabled when
  // libraryIdNum is undefined (via Boolean(0) guard in useLibrarySettings).
  const librarySettingsQuery = useLibrarySettings(libraryIdNum ?? 0);
  const updateLibrarySettings = useUpdateLibrarySettings(libraryIdNum ?? 0);

  // Resolve effective sort: URL wins if valid; else stored preference; else builtin.
  const storedSort: SortLevel[] | null = librarySettingsQuery.data?.sort_spec
    ? parseSortSpec(librarySettingsQuery.data.sort_spec)
    : null;
  const defaultSort: readonly SortLevel[] =
    storedSort && storedSort.length > 0 ? storedSort : BUILTIN_DEFAULT_SORT;
  const effectiveSort: readonly SortLevel[] =
    urlSort && urlSort.length > 0 ? urlSort : defaultSort;

  // "Dirty" = a sort was explicitly provided via URL and differs from default.
  const isSortDirty = urlSort !== null && !sortSpecsEqual(urlSort, defaultSort);

  // Gate gallery render on the settings query having resolved, so we
  // don't flash the builtin default before the stored preference loads.
  const settingsResolved =
    libraryIdNum === undefined ||
    librarySettingsQuery.isSuccess ||
    librarySettingsQuery.isError;

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

  const handleSaveSortAsDefault = () => {
    if (libraryIdNum === undefined) return;
    const serialized = serializeSortSpec(effectiveSort);
    updateLibrarySettings.mutate(
      { sort_spec: serialized || null },
      {
        onSuccess: () => {
          setSearchParams((prev) => {
            const params = new URLSearchParams(prev);
            params.delete("sort");
            return params;
          });
        },
      },
    );
  };

  const applySortLevels = (next: readonly SortLevel[]) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      const serialized = serializeSortSpec(next);
      if (serialized && !sortSpecsEqual(next, defaultSort)) {
        params.set("sort", serialized);
      } else {
        params.delete("sort");
      }
      params.set("page", "1");
      return params;
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

  // Sort: serialize only the effective (possibly resolved-from-default) sort.
  // Always sending a sort query param makes server-side sort explicit and deterministic.
  const serializedSort = serializeSortSpec(effectiveSort);
  if (serializedSort) {
    booksQueryParams.sort = serializedSort;
  }

  const booksQuery = useBooks(booksQueryParams, {
    enabled: settingsResolved,
  });

  // Track the filter state that produced the currently displayed data
  // We use a stringified version of all filter params for comparison
  const currentFilterKey = JSON.stringify({
    search: debouncedSearch,
    fileTypes: selectedFileTypes,
    genreIds: selectedGenreIds,
    tagIds: selectedTagIds,
    language: languageParam,
    sort: sortParam,
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
    sortParam,
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
            fileTypeOptions={FILE_TYPE_OPTIONS}
            genreSearchInput={genreSearchInput}
            genres={genres}
            genresError={genresQuery.isError}
            genresLoading={genresQuery.isLoading}
            hasActiveFilters={hasActiveFilters}
            languageOptions={languageOptions}
            languageParam={languageParam}
            onClearAll={clearAllFilters}
            onGenreSearchChange={setGenreSearchInput}
            onLanguageChange={setLanguageFilter}
            onTagSearchChange={setTagSearchInput}
            onToggleFileType={toggleFileType}
            onToggleGenre={toggleGenreFilter}
            onToggleTag={toggleTagFilter}
            selectedFileTypes={selectedFileTypes}
            selectedGenreIds={selectedGenreIds}
            selectedTagIds={selectedTagIds}
            tagSearchInput={tagSearchInput}
            tags={tags}
            tagsError={tagsQuery.isError}
            tagsLoading={tagsQuery.isLoading}
          />
          <SortSheet
            isDirty={isSortDirty}
            isSaving={updateLibrarySettings.isPending}
            levels={effectiveSort}
            onChange={(next) => applySortLevels(next)}
            onSaveAsDefault={handleSaveSortAsDefault}
            trigger={<SortButton isDirty={isSortDirty} />}
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
          hasActiveFilters={hasActiveFilters}
          languageParam={languageParam}
          onClearAll={clearAllFilters}
          onClearLanguage={() => setLanguageFilter("all")}
          onToggleFileType={toggleFileType}
          onToggleGenre={toggleGenreFilter}
          onToggleTag={toggleTagFilter}
          selectedFileTypes={selectedFileTypes}
          selectedGenres={selectedGenres}
          selectedTags={selectedTags}
        />
        <SortedByChips
          levels={isSortDirty ? effectiveSort : []}
          onRemoveLevel={(index) => {
            const next = effectiveSort.filter((_, i) => i !== index);
            applySortLevels(next);
          }}
          onReset={() => {
            setSearchParams((prev) => {
              const params = new URLSearchParams(prev);
              params.delete("sort");
              return params;
            });
          }}
        />
      </div>

      {!settingsResolved ? (
        <div className="flex min-h-[300px] items-center justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <Gallery
          emptyMessage={
            confirmedHasFilters
              ? "No books found matching your search or filters."
              : "No books in this library yet."
          }
          isLoading={
            booksQuery.isLoading || booksQuery.isFetching || isStaleData
          }
          isSuccess={
            booksQuery.isSuccess && !booksQuery.isFetching && !isStaleData
          }
          itemLabel="books"
          items={booksQuery.data?.books ?? []}
          itemsPerPage={ITEMS_PER_PAGE}
          renderItem={renderBookItem}
          total={booksQuery.data?.total ?? 0}
        />
      )}
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
