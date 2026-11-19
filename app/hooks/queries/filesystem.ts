import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

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
 * Counts the ebooks, audiobooks, and comics in folders a new library will
 * scan. The server stops after a few seconds on a large library and says so.
 * Nothing changes on the server, so the answer is cached per set of paths and
 * an empty set asks nothing.
 */
export const useDetectFileTypes = (paths: string[]) => {
  const payload: DetectFileTypesPayload = { paths };
  return useQuery<DetectFileTypesResponse, ShishoAPIError>({
    enabled: useRequires("libraries:write", paths.length > 0),
    queryKey: [QueryKey.FilesystemDetect, paths],
    queryFn: ({ signal }) => {
      return API.request("POST", "/filesystem/detect", payload, null, signal);
    },
  });
};
