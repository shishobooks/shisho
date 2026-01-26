import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import LibraryLayout from "@/components/library/LibraryLayout";
import { SearchInput } from "@/components/library/SearchInput";
import { Badge } from "@/components/ui/badge";
import { useImprintsList } from "@/hooks/queries/imprints";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { Imprint } from "@/types";

const ITEMS_PER_PAGE = 50;

const ImprintsList = () => {
  usePageTitle("Imprints");

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

  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const imprintsQuery = useImprintsList({
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: debouncedSearch || undefined,
  });

  // Track the search value that produced the currently displayed data
  const [confirmedSearch, setConfirmedSearch] = useState<string | null>(null);

  useEffect(() => {
    if (imprintsQuery.isSuccess && !imprintsQuery.isFetching) {
      setConfirmedSearch(debouncedSearch);
    }
  }, [imprintsQuery.isSuccess, imprintsQuery.isFetching, debouncedSearch]);

  // Data is stale if search changed but query hasn't completed yet
  const isStaleData =
    confirmedSearch !== null && debouncedSearch !== confirmedSearch;

  const total = imprintsQuery.data?.total ?? 0;
  const totalPages = Math.ceil(total / ITEMS_PER_PAGE);

  const handlePageChange = (page: number) => {
    const newParams = new URLSearchParams(searchParams);
    newParams.set("page", page.toString());
    setSearchParams(newParams);
  };

  const renderImprintItem = (imprint: Imprint) => {
    const fileCount = imprint.file_count ?? 0;

    return (
      <Link
        className="flex items-center justify-between p-3 rounded-md hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        key={imprint.id}
        to={`/libraries/${libraryId}/imprints/${imprint.id}`}
      >
        <span className="font-medium">{imprint.name}</span>
        <Badge variant="secondary">
          {fileCount} file{fileCount !== 1 ? "s" : ""}
        </Badge>
      </Link>
    );
  };

  return (
    <LibraryLayout maxWidth="max-w-3xl">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold mb-2">Imprints</h1>
        <p className="text-muted-foreground">Browse imprints in your library</p>
      </div>

      <div className="mb-6">
        <SearchInput
          initialValue={searchQuery}
          onDebouncedChange={handleDebouncedSearchChange}
          placeholder="Search imprints..."
        />
      </div>

      {(imprintsQuery.isLoading || imprintsQuery.isFetching || isStaleData) && (
        <div className="text-muted-foreground">Loading...</div>
      )}

      {imprintsQuery.isSuccess &&
        !imprintsQuery.isFetching &&
        !isStaleData &&
        imprintsQuery.data.imprints.length === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            {confirmedSearch
              ? "No imprints found matching your search."
              : "No imprints in this library yet."}
          </div>
        )}

      {imprintsQuery.isSuccess &&
        !imprintsQuery.isFetching &&
        !isStaleData &&
        imprintsQuery.data.imprints.length > 0 && (
          <div className="space-y-1">
            {imprintsQuery.data.imprints.map(renderImprintItem)}
          </div>
        )}

      {totalPages > 1 && (
        <div className="mt-6 flex justify-center gap-2">
          <button
            className="px-3 py-1 rounded-md border cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            disabled={currentPage <= 1}
            onClick={() => handlePageChange(currentPage - 1)}
          >
            Previous
          </button>
          <span className="px-3 py-1">
            Page {currentPage} of {totalPages}
          </span>
          <button
            className="px-3 py-1 rounded-md border cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            disabled={currentPage >= totalPages}
            onClick={() => handlePageChange(currentPage + 1)}
          >
            Next
          </button>
        </div>
      )}
    </LibraryLayout>
  );
};

export default ImprintsList;
