import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { SearchInput } from "@/components/library/SearchInput";
import { Badge } from "@/components/ui/badge";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { useGenresList } from "@/hooks/queries/genres";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { Genre } from "@/types";

const ITEMS_PER_PAGE = 50;

const GenresList = () => {
  usePageTitle("Genres");

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

  const genresQuery = useGenresList({
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: debouncedSearch || undefined,
  });

  // Track the search value that produced the currently displayed data
  const [confirmedSearch, setConfirmedSearch] = useState<string | null>(null);

  useEffect(() => {
    if (genresQuery.isSuccess && !genresQuery.isFetching) {
      setConfirmedSearch(debouncedSearch);
    }
  }, [genresQuery.isSuccess, genresQuery.isFetching, debouncedSearch]);

  // Data is stale if search changed but query hasn't completed yet
  const isStaleData =
    confirmedSearch !== null && debouncedSearch !== confirmedSearch;

  const total = genresQuery.data?.total ?? 0;
  const totalPages = Math.ceil(total / ITEMS_PER_PAGE);

  const handlePageChange = (page: number) => {
    const newParams = new URLSearchParams(searchParams);
    newParams.set("page", page.toString());
    setSearchParams(newParams);
  };

  const renderGenreItem = (genre: Genre) => {
    const bookCount = genre.book_count ?? 0;

    return (
      <Link
        className="flex items-center justify-between p-3 rounded-md hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        key={genre.id}
        to={`/libraries/${libraryId}/genres/${genre.id}`}
      >
        <span className="font-medium">{genre.name}</span>
        <Badge variant="secondary">
          {bookCount} book{bookCount !== 1 ? "s" : ""}
        </Badge>
      </Link>
    );
  };

  return (
    <LibraryLayout maxWidth="max-w-3xl">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold mb-2">Genres</h1>
        <p className="text-muted-foreground">Browse genres in your library</p>
      </div>

      <div className="mb-6">
        <SearchInput
          initialValue={searchQuery}
          onDebouncedChange={handleDebouncedSearchChange}
          placeholder="Search genres..."
        />
      </div>

      {(genresQuery.isLoading || genresQuery.isFetching || isStaleData) && (
        <LoadingSpinner />
      )}

      {genresQuery.isSuccess && !genresQuery.isFetching && !isStaleData && (
        <>
          {total > 0 && (
            <div className="mb-4 text-sm text-muted-foreground">
              Showing {offset + 1}-{Math.min(offset + limit, total)} of {total}{" "}
              genres
            </div>
          )}

          {genresQuery.data.genres.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              {confirmedSearch
                ? "No genres found matching your search."
                : "No genres in this library yet."}
            </div>
          ) : (
            <div className="space-y-1 mb-6">
              {genresQuery.data.genres.map(renderGenreItem)}
            </div>
          )}

          {totalPages > 1 && (
            <Pagination>
              <PaginationContent>
                <PaginationItem>
                  <PaginationPrevious
                    className={
                      currentPage <= 1
                        ? "pointer-events-none opacity-50"
                        : "cursor-pointer"
                    }
                    onClick={() => handlePageChange(currentPage - 1)}
                  />
                </PaginationItem>
                {Array.from({ length: Math.min(5, totalPages) }, (_, i) => {
                  let pageNum;
                  if (totalPages <= 5) {
                    pageNum = i + 1;
                  } else if (currentPage <= 3) {
                    pageNum = i + 1;
                  } else if (currentPage >= totalPages - 2) {
                    pageNum = totalPages - 4 + i;
                  } else {
                    pageNum = currentPage - 2 + i;
                  }
                  return (
                    <PaginationItem key={pageNum}>
                      <PaginationLink
                        className="cursor-pointer"
                        isActive={pageNum === currentPage}
                        onClick={() => handlePageChange(pageNum)}
                      >
                        {pageNum}
                      </PaginationLink>
                    </PaginationItem>
                  );
                })}
                <PaginationItem>
                  <PaginationNext
                    className={
                      currentPage >= totalPages
                        ? "pointer-events-none opacity-50"
                        : "cursor-pointer"
                    }
                    onClick={() => handlePageChange(currentPage + 1)}
                  />
                </PaginationItem>
              </PaginationContent>
            </Pagination>
          )}
        </>
      )}
    </LibraryLayout>
  );
};

export default GenresList;
