import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  ListPublisherFilesResponse,
  ListPublishersResponse,
  PublisherResponse,
} from "@/types";
import type {
  ListPublishersQuery,
  UpdatePublisherPayload,
} from "@/types/generated/publishers";

import { QueryKey as BooksQueryKey } from "./books";

export type ListPublishersData = ListPublishersResponse;

export enum QueryKey {
  ListPublishers = "ListPublishers",
  RetrievePublisher = "RetrievePublisher",
  PublisherFiles = "PublisherFiles",
}

const invalidatePublisherHierarchyQueries = (queryClient: QueryClient) => {
  queryClient.invalidateQueries({ queryKey: [QueryKey.RetrievePublisher] });
  queryClient.invalidateQueries({ queryKey: [QueryKey.PublisherFiles] });
};

export const usePublishersList = (
  query: ListPublishersQuery = {},
  options: Omit<
    UseQueryOptions<ListPublishersData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListPublishersData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListPublishers, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/publishers", null, query, signal);
    },
  });
};

export const usePublisher = (
  publisherId?: number,
  options: Omit<
    UseQueryOptions<PublisherResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<PublisherResponse, ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(publisherId),
    ...options,
    queryKey: [QueryKey.RetrievePublisher, publisherId],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/publishers/${publisherId}`,
        null,
        null,
        signal,
      );
    },
  });
};

export interface PublisherFilesQuery {
  limit?: number;
  offset?: number;
}

export const usePublisherFiles = (
  publisherId?: number,
  query: PublisherFilesQuery = {},
  options: Omit<
    UseQueryOptions<ListPublisherFilesResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListPublisherFilesResponse, ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(publisherId),
    ...options,
    queryKey: [QueryKey.PublisherFiles, publisherId, query],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/publishers/${publisherId}/files`,
        null,
        query,
        signal,
      );
    },
  });
};

export const useUpdatePublisher = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      publisherId,
      payload,
    }: {
      publisherId: number;
      payload: UpdatePublisherPayload;
    }) => {
      return API.request<PublisherResponse>(
        "PATCH",
        `/publishers/${publisherId}`,
        payload,
      );
    },
    onSuccess: (_data, variables) => {
      if (
        variables.payload.parent_id !== undefined ||
        variables.payload.parent_name !== undefined
      ) {
        invalidatePublisherHierarchyQueries(queryClient);
      } else {
        queryClient.invalidateQueries({
          queryKey: [QueryKey.RetrievePublisher, variables.publisherId],
        });
      }
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPublishers] });
      // Invalidate book queries since they display publisher info on files
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useDeletePublisher = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ publisherId }: { publisherId: number }) => {
      return API.request<void>("DELETE", `/publishers/${publisherId}`);
    },
    onSuccess: () => {
      invalidatePublisherHierarchyQueries(queryClient);
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPublishers] });
      // Invalidate book queries since they display publisher info on files
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useMergePublisher = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      targetId,
      sourceId,
    }: {
      targetId: number;
      sourceId: number;
    }) => {
      return API.request<void>("POST", `/publishers/${targetId}/merge`, {
        source_id: sourceId,
      });
    },
    onSuccess: () => {
      invalidatePublisherHierarchyQueries(queryClient);
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPublishers] });
      // Invalidate book queries since they display publisher info on files
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useSetChildPublisher = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      parentId,
      childId,
    }: {
      parentId: number;
      childId: number;
    }) => {
      return API.request<void>("POST", `/publishers/${parentId}/set-child`, {
        child_id: childId,
      });
    },
    onSuccess: () => {
      invalidatePublisherHierarchyQueries(queryClient);
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPublishers] });
    },
  });
};
