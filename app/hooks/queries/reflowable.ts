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
      // checkStatus rejects with the API's error; the body is a file
      // otherwise, so only a failure goes through it.
      const response = await fetch(fileDownloadUrl(fileId), { signal });
      if (response.ok) return response.blob();
      const error = await API.checkStatus(response).catch((e: unknown) => e);
      // invalid_state means the server has no generator for this type (MOBI
      // and AZW3 until #660), so read the file as it sits on disk, as
      // Download Original would.
      if (!(
        error instanceof ShishoAPIError && error.code === "invalid_state"
      )) {
        throw error;
      }
      const original = await fetch(fileOriginalDownloadUrl(fileId), { signal });
      if (!original.ok) await API.checkStatus(original);
      return original.blob();
    },
  });
};
