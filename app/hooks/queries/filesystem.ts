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

// Counts the ebooks, audiobooks, and comics in folders, as Setup does for a
// new library's and the plugin page does for the libraries', and names any
// folder the server can't open and read. The server stops after a few
// seconds on a large library and says so. With foldersOnly it only checks
// the folders, which answers at once. Each answer is cached per set of
// paths; installed plugins change which files count, so plugin changes
// invalidate it (invalidateFileTypeDetection).
const detectFileTypesOptions = (paths: string[], foldersOnly = false) => ({
  queryKey: [QueryKey.FilesystemDetect, paths, { foldersOnly }],
  queryFn: ({ signal }: { signal: AbortSignal }) => {
    const payload: DetectFileTypesPayload = foldersOnly
      ? { paths, folders_only: true }
      : { paths };
    return API.request<DetectFileTypesResponse>(
      "POST",
      "/filesystem/detect",
      payload,
      null,
      signal,
    );
  },
});

/** The most paths one detection takes, as DetectFileTypesPayload allows. */
export const DETECT_MAX_PATHS = 100;

/**
 * Drops cached detection answers, for a plugin change that adds or removes
 * file types the scanner imports.
 */
export const invalidateFileTypeDetection = (queryClient: QueryClient) =>
  queryClient.invalidateQueries({ queryKey: [QueryKey.FilesystemDetect] });

/** Detects the file types in the paths; an empty set asks nothing. */
export const useDetectFileTypes = (paths: string[]) => {
  return useQuery<DetectFileTypesResponse, ShishoAPIError>({
    ...detectFileTypesOptions(paths),
    enabled: useRequires("libraries:write", paths.length > 0),
  });
};

// Asks the server now, never from the cache, whether it can open and read
// each folder, without counting what's in them, such as when Setup checks the
// folders before going on. Callers sit behind a route that needs Libraries
// Write, the endpoint's permission.
export const fetchFolderCheck = (queryClient: QueryClient, paths: string[]) =>
  queryClient.fetchQuery<DetectFileTypesResponse, ShishoAPIError>({
    ...detectFileTypesOptions(paths, true),
    staleTime: 0,
  });
