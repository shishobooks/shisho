import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { ListLogsResponse } from "@/types";

export enum QueryKey {
  ListLogs = "ListLogs",
}

type ListLogsData = ListLogsResponse;

interface UseLogsOptions {
  level?: string;
  search?: string;
  limit?: number;
  afterId?: number;
}

export const useLogs = (
  options: UseLogsOptions = {},
  queryOptions: Omit<
    UseQueryOptions<ListLogsData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListLogsData, ShishoAPIError>({
    ...queryOptions,
    queryKey: [QueryKey.ListLogs, options],
    queryFn: ({ signal }) => {
      const params: Record<string, string> = {};
      if (options.level) {
        params.level = options.level;
      }
      if (options.search) {
        params.search = options.search;
      }
      if (options.limit !== undefined) {
        params.limit = String(options.limit);
      }
      if (options.afterId !== undefined) {
        params.after_id = String(options.afterId);
      }
      return API.request("GET", "/logs", null, params, signal);
    },
  });
};
