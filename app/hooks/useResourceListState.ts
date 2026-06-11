import { useCallback, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import { parsePageParam } from "@/libraries/pagination";

const ITEMS_PER_PAGE = 50;

export interface ResourceListQuery {
  limit: number;
  offset: number;
  search?: string;
  library_id?: number;
}

/** Hook that manages URL-driven search and pagination state for resource lists. */
export function useResourceListState() {
  const { libraryId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parsePageParam(searchParams.get("page"));
  const searchQuery = searchParams.get("search") ?? "";

  const [debouncedSearch, setDebouncedSearch] = useState(searchQuery);

  const handleDebouncedSearchChange = useCallback(
    (value: string) => {
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
    },
    [searchQuery, setSearchParams],
  );

  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const queryParams: ResourceListQuery = {
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: debouncedSearch || undefined,
  };

  const handlePageChange = useCallback(
    (page: number) => {
      const newParams = new URLSearchParams(searchParams);
      newParams.set("page", page.toString());
      setSearchParams(newParams);
    },
    [searchParams, setSearchParams],
  );

  return {
    libraryId: libraryId ?? "",
    currentPage,
    searchQuery,
    debouncedSearch,
    handleDebouncedSearchChange,
    queryParams,
    limit,
    offset,
    handlePageChange,
  };
}
