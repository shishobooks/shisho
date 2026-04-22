import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { ShishoAPIError } from "@/libraries/api";

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
    queryKey: [QueryKey.EpubBlob, fileId],
    staleTime: 5 * 60 * 1000,
    gcTime: 60 * 1000,
    queryFn: async ({ signal }) => {
      const response = await fetch(`/api/books/files/${fileId}/download`, {
        signal,
      });
      if (!response.ok) {
        throw new ShishoAPIError(
          `Failed to fetch EPUB: ${response.status} ${response.statusText}`,
          "epub_download_failed",
          response.status,
        );
      }
      return response.blob();
    },
  });
};
