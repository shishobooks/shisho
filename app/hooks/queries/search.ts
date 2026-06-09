import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { GlobalSearchQuery, GlobalSearchResponse } from "@/types";

export enum QueryKey {
  GlobalSearch = "GlobalSearch",
}

export const useGlobalSearch = (
  query: GlobalSearchQuery,
  options: Omit<
    UseQueryOptions<GlobalSearchResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<GlobalSearchResponse, ShishoAPIError>({
    enabled:
      options.enabled !== undefined
        ? options.enabled
        : Boolean(query.q && query.library_id),
    ...options,
    queryKey: [QueryKey.GlobalSearch, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/search", null, query, signal);
    },
  });
};
