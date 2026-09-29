import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import { fileDownloadUrl } from "@/utils/downloadUrl";

import { useRequires } from "./permissions";

export enum QueryKey {
  EpubBlob = "EpubBlob",
}

export const useEpubBlob = (
  fileId: number,
  options: Omit<
    UseQueryOptions<Blob, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Blob, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.EpubBlob, fileId],
    // staleTime matches gcTime: Blobs are a few MB and add up across books.
    // We want a short cache window (~60s — long enough for tab-switch-back
    // to feel instant, short enough not to hold multiple books in memory),
    // and a longer staleTime would be unreachable because the cache gets
    // GC'd before it could suppress a refetch.
    staleTime: 60 * 1000,
    gcTime: 60 * 1000,
    queryFn: async ({ signal }) => {
      const response = await fetch(fileDownloadUrl(fileId), {
        signal,
      });
      // checkStatus rejects with the API's error; the body is a file
      // otherwise, so only a failure goes through it.
      if (!response.ok) await API.checkStatus(response);
      return response.blob();
    },
  });
};
