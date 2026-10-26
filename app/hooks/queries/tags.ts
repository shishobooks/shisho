import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { ListTagsResponse, TagResponse } from "@/types";
import type { ListTagsQuery, UpdateTagPayload } from "@/types/generated/tags";

import { QueryKey as BooksQueryKey } from "./books";
import { useRequires } from "./permissions";

export enum QueryKey {
  ListTags = "ListTags",
  RetrieveTag = "RetrieveTag",
}

export type ListTagsData = ListTagsResponse;

export const useTagsList = (
  query: ListTagsQuery = {},
  options: Omit<
    UseQueryOptions<ListTagsData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListTagsData, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ListTags, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/tags", null, query, signal);
    },
  });
};

export const useTag = (
  tagId?: number,
  options: Omit<
    UseQueryOptions<TagResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<TagResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? Boolean(tagId)),
    queryKey: [QueryKey.RetrieveTag, tagId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/tags/${tagId}`, null, null, signal);
    },
  });
};

// Several tags at once, sharing the cache entries of useTag.
export const useTagsByIds = (ids: number[]) => {
  const permitted = useRequires("books:read");
  return useQueries({
    queries: ids.map((id) => ({
      queryKey: [QueryKey.RetrieveTag, id],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        API.request<TagResponse>("GET", `/tags/${id}`, null, null, signal),
      enabled: permitted,
    })),
  });
};

export const useUpdateTag = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      tagId,
      payload,
    }: {
      tagId: number;
      payload: UpdateTagPayload;
    }) => {
      return API.request<TagResponse>("PATCH", `/tags/${tagId}`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveTag, variables.tagId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListTags] });
      // Invalidate book queries since they display tag info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useDeleteTag = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ tagId }: { tagId: number }) => {
      return API.request<void>("DELETE", `/tags/${tagId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListTags] });
      // Invalidate book queries since they display tag info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useMergeTag = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      targetId,
      sourceId,
    }: {
      targetId: number;
      sourceId: number;
    }) => {
      return API.request<void>("POST", `/tags/${targetId}/merge`, {
        source_id: sourceId,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveTag, variables.targetId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListTags] });
      // Invalidate book queries since they display tag info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};
