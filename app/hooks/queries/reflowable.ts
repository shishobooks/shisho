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
    // Each new Blob makes the reader close the book and open it again, so the
    // reader gets one per opening. Never stale: returning to the tab doesn't
    // download the book again. Not kept once the reader closes: reopening it
    // would show the cached Blob and then the one refetchOnMount: "always"
    // brings. The book a reader opened stays put while it is open, so a change
    // to the file, such as a chapter edit in another tab, shows when the
    // reader next opens.
    staleTime: Infinity,
    gcTime: 0,
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
