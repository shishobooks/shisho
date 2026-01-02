import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { BrowseQuery, BrowseResponse } from "@/types";

export enum QueryKey {
  FilesystemBrowse = "FilesystemBrowse",
}

export const useFilesystemBrowse = (
  query: BrowseQuery = {},
  options: Omit<
    UseQueryOptions<BrowseResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<BrowseResponse, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.FilesystemBrowse, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/filesystem/browse", null, query, signal);
    },
  });
};
