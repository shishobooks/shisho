import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { File, Publisher, ResourceListResponse } from "@/types";
import type {
  ListPublishersQuery,
  UpdatePublisherPayload,
} from "@/types/generated/publishers";

import { QueryKey as BooksQueryKey } from "./books";

export enum QueryKey {
  ListPublishers = "ListPublishers",
  RetrievePublisher = "RetrievePublisher",
  PublisherFiles = "PublisherFiles",
}

export type ListPublishersData = ResourceListResponse<Publisher>;

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
    UseQueryOptions<Publisher, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Publisher, ShishoAPIError>({
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
    UseQueryOptions<ResourceListResponse<File>, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ResourceListResponse<File>, ShishoAPIError>({
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
      return API.request<Publisher>(
        "PATCH",
        `/publishers/${publisherId}`,
        payload,
      );
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrievePublisher, variables.publisherId],
      });
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
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrievePublisher, variables.targetId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPublishers] });
      // Invalidate detail-page file list since files moved from source to target
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PublisherFiles, variables.targetId],
      });
      // Invalidate book queries since they display publisher info on files
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};
