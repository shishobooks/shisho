import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  Bookmark,
  CreateBookmarkPayload,
  UpdateBookmarkPayload,
} from "@/types";

import { useRequires } from "./permissions";

export enum QueryKey {
  FileBookmarks = "FileBookmarks",
}

// The signed-in user's Bookmarks in an audiobook File, in time order.
// Another browser or device can add them, and nothing here hears about it,
// so they count as stale at once: each time the list is shown again it
// fetches anew, while showing the copy it has.
export const useFileBookmarks = (
  fileId?: number,
  options: Omit<
    UseQueryOptions<Bookmark[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Bookmark[], ShishoAPIError>({
    staleTime: 0,
    ...options,
    enabled: useRequires("books:read", options.enabled ?? Boolean(fileId)),
    queryKey: [QueryKey.FileBookmarks, fileId],
    queryFn: ({ signal }) =>
      API.request(
        "GET",
        `/reading/files/${fileId}/bookmarks`,
        null,
        null,
        signal,
      ),
  });
};

export const useCreateBookmark = (fileId: number) => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreateBookmarkPayload) =>
      API.request<Bookmark>(
        "POST",
        `/reading/files/${fileId}/bookmarks`,
        payload,
      ),
    // Waiting for the refetch keeps the mutation pending until the list
    // shows the change, so a second click can't act on a stale row.
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: [QueryKey.FileBookmarks, fileId],
      }),
  });
};

export const useUpdateBookmark = (fileId: number) => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      bookmarkId,
      payload,
    }: {
      bookmarkId: number;
      payload: UpdateBookmarkPayload;
    }) =>
      API.request<Bookmark>(
        "PATCH",
        `/reading/bookmarks/${bookmarkId}`,
        payload,
      ),
    // Waiting for the refetch keeps the mutation pending until the list
    // shows the change, so a second click can't act on a stale row.
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: [QueryKey.FileBookmarks, fileId],
      }),
  });
};

export const useDeleteBookmark = (fileId: number) => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (bookmarkId: number) =>
      API.request<void>("DELETE", `/reading/bookmarks/${bookmarkId}`),
    // Waiting for the refetch keeps the mutation pending until the list
    // shows the change, so a second click can't act on a stale row.
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: [QueryKey.FileBookmarks, fileId],
      }),
  });
};
