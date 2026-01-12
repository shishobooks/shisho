import { QueryKey as BooksQueryKey } from "./books";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { File, Publisher } from "@/types";
import type {
  ListPublishersQuery,
  UpdatePublisherPayload,
} from "@/types/generated/publishers";

export enum QueryKey {
  ListPublishers = "ListPublishers",
  RetrievePublisher = "RetrievePublisher",
  PublisherFiles = "PublisherFiles",
}

export interface ListPublishersData {
  publishers: Publisher[];
  total: number;
}

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

export const usePublisherFiles = (
  publisherId?: number,
  options: Omit<
    UseQueryOptions<File[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<File[], ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(publisherId),
    ...options,
    queryKey: [QueryKey.PublisherFiles, publisherId],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/publishers/${publisherId}/files`,
        null,
        null,
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
      // Invalidate book queries since they display publisher info on files
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};
