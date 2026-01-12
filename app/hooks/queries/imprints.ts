import { QueryKey as BooksQueryKey } from "./books";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { File, Imprint } from "@/types";
import type {
  ListImprintsQuery,
  UpdateImprintPayload,
} from "@/types/generated/imprints";

export enum QueryKey {
  ListImprints = "ListImprints",
  RetrieveImprint = "RetrieveImprint",
  ImprintFiles = "ImprintFiles",
}

export interface ListImprintsData {
  imprints: Imprint[];
  total: number;
}

export const useImprintsList = (
  query: ListImprintsQuery = {},
  options: Omit<
    UseQueryOptions<ListImprintsData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListImprintsData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListImprints, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/imprints", null, query, signal);
    },
  });
};

export const useImprint = (
  imprintId?: number,
  options: Omit<
    UseQueryOptions<Imprint, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Imprint, ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(imprintId),
    ...options,
    queryKey: [QueryKey.RetrieveImprint, imprintId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/imprints/${imprintId}`, null, null, signal);
    },
  });
};

export const useImprintFiles = (
  imprintId?: number,
  options: Omit<
    UseQueryOptions<File[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<File[], ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(imprintId),
    ...options,
    queryKey: [QueryKey.ImprintFiles, imprintId],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/imprints/${imprintId}/files`,
        null,
        null,
        signal,
      );
    },
  });
};

export const useUpdateImprint = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      imprintId,
      payload,
    }: {
      imprintId: number;
      payload: UpdateImprintPayload;
    }) => {
      return API.request<Imprint>("PATCH", `/imprints/${imprintId}`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveImprint, variables.imprintId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListImprints] });
      // Invalidate book queries since they display imprint info on files
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useDeleteImprint = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ imprintId }: { imprintId: number }) => {
      return API.request<void>("DELETE", `/imprints/${imprintId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListImprints] });
      // Invalidate book queries since they display imprint info on files
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useMergeImprint = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      targetId,
      sourceId,
    }: {
      targetId: number;
      sourceId: number;
    }) => {
      return API.request<void>("POST", `/imprints/${targetId}/merge`, {
        source_id: sourceId,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveImprint, variables.targetId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListImprints] });
      // Invalidate book queries since they display imprint info on files
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};
