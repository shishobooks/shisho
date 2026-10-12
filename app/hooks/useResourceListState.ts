import { useCallback } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import { useSearchQueryParam } from "@/hooks/useSearchQueryParam";
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
  const { searchQuery, setSearchQuery } = useSearchQueryParam();

  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const queryParams: ResourceListQuery = {
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: searchQuery || undefined,
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
    setSearchQuery,
    queryParams,
    limit,
    offset,
    handlePageChange,
  };
}
