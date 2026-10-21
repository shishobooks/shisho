import { useParams } from "react-router-dom";

import { RESOURCE_PAGE_TARGET } from "@/constants/resourceList";
import { useSearchQueryParam } from "@/hooks/useSearchQueryParam";
import { useSortParam } from "@/hooks/useSortParam";
import { useStartPaging } from "@/hooks/useStartPaging";
import {
  resourceListColumns,
  resourceListPageSize,
} from "@/libraries/resourceListLayout";
import type { SortConfig } from "@/libraries/sortSpec";

export interface ResourceListQuery {
  limit: number;
  offset: number;
  search?: string;
  library_id?: number;
  sort?: string;
}

/**
 * Hook that manages URL-driven search, sort, and pagination state for
 * resource lists. Entries sit side by side in the columns the content width
 * fits, and a page is whole rows of them (see `useStartPaging`).
 */
export function useResourceListState<F extends string>(
  sortConfig: SortConfig<F>,
) {
  const { libraryId } = useParams();
  const { searchQuery, setSearchQuery } = useSearchQueryParam();
  const sort = useSortParam(sortConfig);
  const { columns, pageSize, offset, currentPage, measured, goToPage } =
    useStartPaging({
      columnsFor: resourceListColumns,
      pageSizeFor: resourceListPageSize,
      // Pages held a fixed fifty entries before they filled the window.
      legacyPageSize: RESOURCE_PAGE_TARGET,
    });

  const queryParams: ResourceListQuery = {
    limit: pageSize,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: searchQuery || undefined,
    sort: sort.param,
  };

  return {
    libraryId: libraryId ?? "",
    currentPage,
    searchQuery,
    setSearchQuery,
    sort,
    columns,
    pageSizeResolved: measured,
    queryParams,
    limit: pageSize,
    offset,
    handlePageChange: goToPage,
  };
}

export type ResourceListState<F extends string> = ReturnType<
  typeof useResourceListState<F>
>;
