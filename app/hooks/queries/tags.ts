import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Book, Tag } from "@/types";
import type { ListTagsQuery, UpdateTagPayload } from "@/types/generated/tags";

export enum QueryKey {
  ListTags = "ListTags",
  RetrieveTag = "RetrieveTag",
  TagBooks = "TagBooks",
}

export interface ListTagsData {
  tags: Tag[];
  total: number;
}

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
    UseQueryOptions<Tag, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Tag, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(tagId),
    ...options,
    queryKey: [QueryKey.RetrieveTag, tagId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/tags/${tagId}`, null, null, signal);
    },
  });
};

export const useTagBooks = (
  tagId?: number,
  options: Omit<
    UseQueryOptions<Book[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Book[], ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(tagId),
    ...options,
    queryKey: [QueryKey.TagBooks, tagId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/tags/${tagId}/books`, null, null, signal);
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
      return API.request<Tag>("PATCH", `/tags/${tagId}`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveTag, variables.tagId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListTags] });
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
    },
  });
};
