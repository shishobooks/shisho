import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  ListTagBooksResponse,
  ResourceListResponse,
  TagResponse,
} from "@/types";
import type {
  ListTagsQuery,
  SubResourceQuery,
  UpdateTagPayload,
} from "@/types/generated/tags";

import { QueryKey as BooksQueryKey } from "./books";

export enum QueryKey {
  ListTags = "ListTags",
  RetrieveTag = "RetrieveTag",
  TagBooks = "TagBooks",
}

export type ListTagsData = ResourceListResponse<TagResponse>;

export const useTagsList = (
  query: ListTagsQuery = {},
  options: Omit<
    UseQueryOptions<ListTagsData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListTagsData, ShishoAPIError>({
    ...options,
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
    enabled: options.enabled !== undefined ? options.enabled : Boolean(tagId),
    ...options,
    queryKey: [QueryKey.RetrieveTag, tagId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/tags/${tagId}`, null, null, signal);
    },
  });
};

// Alias of the generated query type; imported directly from the generated
// module because every sub-resource package emits a `SubResourceQuery` and
// the barrel cannot re-export them all under one name.
export type TagBooksQuery = SubResourceQuery;

export const useTagBooks = (
  tagId?: number,
  query: TagBooksQuery = {},
  options: Omit<
    UseQueryOptions<ListTagBooksResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListTagBooksResponse, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(tagId),
    ...options,
    queryKey: [QueryKey.TagBooks, tagId, query],
    queryFn: ({ signal }) => {
      return API.request("GET", `/tags/${tagId}/books`, null, query, signal);
    },
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
      // Invalidate detail-page book list since books moved from source to target
      queryClient.invalidateQueries({
        queryKey: [QueryKey.TagBooks, variables.targetId],
      });
      // Invalidate book queries since they display tag info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};
