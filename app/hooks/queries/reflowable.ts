import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { useAuth } from "@/hooks/useAuth";
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
  const { demoMode } = useAuth();
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
      try {
        // checkStatus rejects with the API's error; the body is a file
        // otherwise, so only a failure goes through it.
        const response = await fetch(fileDownloadUrl(fileId), { signal });
        if (!response.ok) await API.checkStatus(response);
        // A build that fails partway cuts the connection, so the body
        // rejects here rather than arriving truncated.
        return await response.blob();
      } catch (error) {
        // Demo Mode denies the original, so its rejection would only hide
        // why the Generated File failed.
        if (signal.aborted || demoMode) throw error;
        // Read the file as it sits on disk, as Download unmodified file would,
        // whatever kept the Generated File from arriving: no generator for
        // the type, a failed build, or a cut connection.
        const original = await fetch(fileOriginalDownloadUrl(fileId), {
          signal,
        });
        if (!original.ok) await API.checkStatus(original);
        return original.blob();
      }
    },
  });
};
