import {
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { useAuth } from "@/hooks/useAuth";
import { API, ShishoAPIError } from "@/libraries/api";
import {
  BuildTimeoutError,
  RangeFile,
  RangeNotSupportedError,
} from "@/libraries/rangeFile";
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
  const defaultRetry = useQueryClient().getDefaultOptions().queries?.retry;
  return useQuery<ReflowableBook, ShishoAPIError>({
    // A build that outlasted the wait is reported, not waited on again.
    retry: (failureCount, error) =>
      !(error instanceof BuildTimeoutError) &&
      (typeof defaultRetry === "function"
        ? defaultRetry(failureCount, error)
        : failureCount < Number(defaultRetry ?? 0)),
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
      // Only Shisho refusing the Generated File falls back to the original:
      // no generator for the type, a file it can't decrypt, or an earlier
      // build that failed partway (422), or a build failing now (Shisho's
      // own 5xx). A proxy's error page, a dropped connection, a missing
      // file, or a session that ended would fail the original's too. Demo
      // Mode denies the original, so its rejection would only hide why the
      // Generated File failed.
      const refused = (error: unknown) =>
        !signal.aborted &&
        !demoMode &&
        error instanceof ShishoAPIError &&
        error.code !== undefined &&
        (error.status === 422 || error.status >= 500);
      try {
        return await openBook(fileDownloadUrl(fileId), signal);
      } catch (error) {
        if (!refused(error)) throw error;
      }
      return openBook(fileOriginalDownloadUrl(fileId), signal);
    },
  });
};

// A whole book can be more than a phone's browser tab can hold (a cookbook's
// photos run to hundreds of MB), so it's read by part. A server or proxy
// that ignores Range sends the file whole.
const openBook = async (
  url: string,
  signal: AbortSignal,
): Promise<ReflowableBook> => {
  try {
    return await RangeFile.open(url, { signal });
  } catch (error) {
    if (!(error instanceof RangeNotSupportedError)) throw error;
  }
  // checkStatus rejects with the API's error; the body is a file otherwise,
  // so only a failure goes through it.
  const response = await fetch(url, { signal });
  if (!response.ok) await API.checkStatus(response);
  return response.blob();
};
