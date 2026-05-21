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

export interface PublisherAncestor {
  id: number;
  name: string;
}

export interface PublisherChild {
  id: number;
  name: string;
  file_count: number;
}

export interface PublisherDetail extends Omit<Publisher, "children"> {
  ancestors: PublisherAncestor[];
  descendant_ids: number[];
  children: PublisherChild[];
  descendant_file_count: number;
}

export interface PublisherListItem extends Publisher {
  descendant_file_count: number;
  descendant_publisher_count: number;
  parent_name: string | null;
}

export type ListPublishersData = ResourceListResponse<PublisherListItem>;

export enum QueryKey {
  ListPublishers = "ListPublishers",
  RetrievePublisher = "RetrievePublisher",
  PublisherFiles = "PublisherFiles",
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
    UseQueryOptions<PublisherDetail, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<PublisherDetail, ShishoAPIError>({
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
      const previousParentId = queryClient.getQueryData<PublisherDetail>([
        QueryKey.RetrievePublisher,
        variables.publisherId,
      ])?.parent_id;

      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrievePublisher, variables.publisherId],
      });
      if ("parent_id" in variables.payload) {
        const affectedParentIds = new Set(
          [previousParentId, variables.payload.parent_id].filter(
            (id): id is number => id != null,
          ),
        );

        affectedParentIds.forEach((parentId) => {
          queryClient.invalidateQueries({
            queryKey: [QueryKey.RetrievePublisher, parentId],
          });
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
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrievePublisher, variables.parentId],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrievePublisher, variables.childId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPublishers] });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.PublisherFiles, variables.parentId],
      });
    },
  });
};
