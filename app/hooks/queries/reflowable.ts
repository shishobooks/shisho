import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import { fileDownloadUrl, fileOriginalDownloadUrl } from "@/utils/downloadUrl";

import { useRequires } from "./permissions";

export enum QueryKey {
  ReflowableBlob = "ReflowableBlob",
}

export const useReflowableBlob = (
  fileId: number,
  options: Omit<
    UseQueryOptions<Blob, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Blob, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ReflowableBlob, fileId],
    // staleTime matches gcTime: Blobs are a few MB and add up across books.
    // We want a short cache window (~60s — long enough for tab-switch-back
    // to feel instant, short enough not to hold multiple books in memory),
    // and a longer staleTime would be unreachable because the cache gets
    // GC'd before it could suppress a refetch.
    staleTime: 60 * 1000,
    gcTime: 60 * 1000,
    queryFn: async ({ signal }) => {
      let response = await fetch(fileDownloadUrl(fileId), { signal });
      // A 422 means the server has no generator for this type, so read the
      // file as it sits on disk, as Download Original would.
      if (response.status === 422) {
        response = await fetch(fileOriginalDownloadUrl(fileId), { signal });
      }
      // checkStatus rejects with the API's error; the body is a file
      // otherwise, so only a failure goes through it.
      if (!response.ok) await API.checkStatus(response);
      return response.blob();
    },
  });
};
