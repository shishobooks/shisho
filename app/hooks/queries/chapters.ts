import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Chapter } from "@/types";

export enum QueryKey {
  FileChapters = "FileChapters",
}

interface ChaptersResponse {
  chapters: Chapter[];
}

export const useFileChapters = (
  fileId?: number,
  options: Omit<
    UseQueryOptions<Chapter[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Chapter[], ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(fileId),
    ...options,
    queryKey: [QueryKey.FileChapters, fileId],
    queryFn: async ({ signal }) => {
      const response: ChaptersResponse = await API.request(
        "GET",
        `/books/files/${fileId}/chapters`,
        null,
        null,
        signal,
      );
      return response.chapters;
    },
  });
};
