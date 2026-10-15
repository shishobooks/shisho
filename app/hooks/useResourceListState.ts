import { useCallback } from "react";
import { useParams, useSearchParams } from "react-router-dom";

import { useResourceSortParam } from "@/hooks/useResourceSortParam";
import { useSearchQueryParam } from "@/hooks/useSearchQueryParam";
import { parsePageParam } from "@/libraries/pagination";
import type { ResourceSortConfig } from "@/libraries/resourceSort";

const ITEMS_PER_PAGE = 50;

export interface ResourceListQuery {
  limit: number;
  offset: number;
  search?: string;
  library_id?: number;
  sort?: string;
}

/** Hook that manages URL-driven search, sort, and pagination state for resource lists. */
export function useResourceListState<F extends string>(
  sortConfig: ResourceSortConfig<F>,
) {
  const { libraryId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parsePageParam(searchParams.get("page"));
  const { searchQuery, setSearchQuery } = useSearchQueryParam();
  const sort = useResourceSortParam(sortConfig);

  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const queryParams: ResourceListQuery = {
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: searchQuery || undefined,
    sort: sort.param,
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
    sort,
    queryParams,
    limit,
    offset,
    handlePageChange,
  };
}

export type ResourceListState<F extends string> = ReturnType<
  typeof useResourceListState<F>
>;
