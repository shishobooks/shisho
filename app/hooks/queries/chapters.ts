import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Chapter, ChapterInput, ReplaceChaptersPayload } from "@/types";

import { useRequires } from "./permissions";
import { QueryKey as ReflowableQueryKey } from "./reflowable";

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
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/books/files/${fileId}/chapters`,
        null,
        null,
        signal,
      );
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
    mutationFn: ({ chapters }) => {
      const payload: ReplaceChaptersPayload = { chapters };
      return API.request(
        "PUT",
        `/books/files/${fileId}/chapters`,
        payload,
        null,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.FileChapters, fileId],
      });
      // The reader's book is the generated download, whose table of contents
      // carries the chapters.
      queryClient.invalidateQueries({
        queryKey: [ReflowableQueryKey.ReflowableBlob, fileId],
      });
    },
  });
};
