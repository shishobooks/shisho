import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Chapter, ChapterInput, ChaptersResponse } from "@/types";

import { useRequires } from "./permissions";

export enum QueryKey {
  FileChapters = "FileChapters",
}

export const useFileChapters = (
  fileId?: number,
  options: Omit<
    UseQueryOptions<Chapter[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Chapter[], ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? Boolean(fileId)),
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

interface UpdateFileChaptersMutationVariables {
  chapters: ChapterInput[];
}

export const useUpdateFileChapters = (fileId: number) => {
  const queryClient = useQueryClient();

  return useMutation<
    Chapter[],
    ShishoAPIError,
    UpdateFileChaptersMutationVariables
  >({
    mutationFn: async ({ chapters }) => {
      const response: ChaptersResponse = await API.request(
        "PUT",
        `/books/files/${fileId}/chapters`,
        { chapters },
        null,
      );
      return response.chapters;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.FileChapters, fileId],
      });
    },
  });
};
