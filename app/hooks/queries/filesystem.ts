import {
  useQuery,
  type QueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  BrowseQuery,
  BrowseResponse,
  DetectFileTypesPayload,
  DetectFileTypesResponse,
} from "@/types";

import { useRequires } from "./permissions";

export enum QueryKey {
  FilesystemBrowse = "FilesystemBrowse",
  FilesystemDetect = "FilesystemDetect",
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
    enabled: useRequires("libraries:write", options.enabled ?? true),
    queryKey: [QueryKey.FilesystemBrowse, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/filesystem/browse", null, query, signal);
    },
  });
};

/**
 * The query that counts the ebooks, audiobooks, and comics in folders a new
 * library will scan, and names any folder the server can't open. The server
 * stops after a few seconds on a large library and says so. Nothing changes
 * on the server, so the answer is cached per set of paths.
 */
export const detectFileTypesQuery = (paths: string[]) => ({
  queryKey: [QueryKey.FilesystemDetect, paths],
  queryFn: ({ signal }: { signal: AbortSignal }) => {
    const payload: DetectFileTypesPayload = { paths };
    return API.request<DetectFileTypesResponse>(
      "POST",
      "/filesystem/detect",
      payload,
      null,
      signal,
    );
  },
});

/** Runs `detectFileTypesQuery` for the paths; an empty set asks nothing. */
export const useDetectFileTypes = (paths: string[]) => {
  return useQuery<DetectFileTypesResponse, ShishoAPIError>({
    ...detectFileTypesQuery(paths),
    enabled: useRequires("libraries:write", paths.length > 0),
  });
};

// Asks the server about the folders now, never from the cache, such as when
// Setup checks them before going on. Callers sit behind a route that needs
// Libraries Write, the endpoint's permission.
export const fetchDetectFileTypes = (
  queryClient: QueryClient,
  paths: string[],
) =>
  queryClient.fetchQuery<DetectFileTypesResponse, ShishoAPIError>({
    ...detectFileTypesQuery(paths),
    staleTime: 0,
  });
