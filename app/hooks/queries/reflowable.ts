import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { useAuth } from "@/hooks/useAuth";
import { API, ShishoAPIError } from "@/libraries/api";
import { RangeFile, RangeNotSupportedError } from "@/libraries/rangeFile";
import { fileDownloadUrl, fileOriginalDownloadUrl } from "@/utils/downloadUrl";

import { useRequires } from "./permissions";

export enum QueryKey {
  ReflowableBook = "ReflowableBook",
}

/**
 * The book the reader opens: read a part at a time from the server, or a
 * whole file from a server that ignores Range.
 */
export type ReflowableBook = RangeFile | Blob;

export const useReflowableBook = (
  fileId: number,
  options: Omit<
    UseQueryOptions<ReflowableBook, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  const { demoMode } = useAuth();
  return useQuery<ReflowableBook, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ReflowableBook, fileId],
    // Each new book makes the reader close the book and open it again, so the
    // reader gets one per opening. Never stale: returning to the tab doesn't
    // download the book again. Not kept once the reader closes: reopening it
    // would show the cached book and then the one refetchOnMount: "always"
    // brings. The book a reader opened stays put while it is open, so a change
    // to the file, such as a chapter edit in another tab, shows when the
    // reader next opens.
    staleTime: Infinity,
    gcTime: 0,
    queryFn: async ({ signal }) => {
      // A whole book can be more than a phone's browser tab can hold (a
      // cookbook's photos run to hundreds of MB), so it's read by part.
      try {
        return await openByPart(fileId, signal, demoMode);
      } catch (error) {
        // A server or proxy that ignores Range sends the file whole.
        if (!(error instanceof RangeNotSupportedError)) throw error;
      }
      return downloadWhole(fileId, signal, demoMode);
    },
  });
};

const openByPart = async (
  fileId: number,
  signal: AbortSignal,
  demoMode: boolean,
): Promise<RangeFile> => {
  try {
    return await RangeFile.open(fileDownloadUrl(fileId), { signal });
  } catch (error) {
    // Only the server's refusal to build the Generated File falls back: no
    // generator for the type, or a failed build. A dropped connection would
    // drop the original's too. Demo Mode denies the original, so its
    // rejection would only hide why the Generated File failed.
    if (signal.aborted || demoMode || !(error instanceof ShishoAPIError)) {
      throw error;
    }
    return RangeFile.open(fileOriginalDownloadUrl(fileId), { signal });
  }
};

const downloadWhole = async (
  fileId: number,
  signal: AbortSignal,
  demoMode: boolean,
): Promise<Blob> => {
  try {
    // checkStatus rejects with the API's error; the body is a file
    // otherwise, so only a failure goes through it.
    const response = await fetch(fileDownloadUrl(fileId), { signal });
    if (!response.ok) await API.checkStatus(response);
    // A build that fails partway cuts the connection, so the body
    // rejects here rather than arriving truncated.
    return await response.blob();
  } catch (error) {
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
};
