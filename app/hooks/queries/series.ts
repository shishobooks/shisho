import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Book, ResourceListResponse, SeriesResponse } from "@/types";
import type {
  ListSeriesQuery,
  SubResourceQuery,
  UpdateSeriesPayload,
} from "@/types/generated/series";

import { QueryKey as BooksQueryKey } from "./books";
import { QueryKey as SearchQueryKey } from "./search";

export enum QueryKey {
  ListSeries = "ListSeries",
  RetrieveSeries = "RetrieveSeries",
  SeriesBooks = "SeriesBooks",
}

export type { ListSeriesQuery };

export type ListSeriesData = ResourceListResponse<SeriesResponse>;

export const useSeriesList = (
  query: ListSeriesQuery = {},
  options: Omit<
    UseQueryOptions<ListSeriesData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListSeriesData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListSeries, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/series", null, query, signal);
    },
  });
};

export const useSeries = (
  seriesId?: number,
  options: Omit<
    UseQueryOptions<SeriesResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<SeriesResponse, ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(seriesId),
    ...options,
    queryKey: [QueryKey.RetrieveSeries, seriesId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/series/${seriesId}`, null, null, signal);
    },
  });
};

// Alias of the generated query type; imported directly from the generated
// module because every sub-resource package emits a `SubResourceQuery` and
// the barrel cannot re-export them all under one name.
export type SeriesBooksQuery = SubResourceQuery;

export const useSeriesBooks = (
  seriesId?: number,
  query: SeriesBooksQuery = {},
  options: Omit<
    UseQueryOptions<ResourceListResponse<Book>, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ResourceListResponse<Book>, ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(seriesId),
    ...options,
    queryKey: [QueryKey.SeriesBooks, seriesId, query],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/series/${seriesId}/books`,
        null,
        query,
        signal,
      );
    },
  });
};

export const useUpdateSeries = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      seriesId,
      payload,
    }: {
      seriesId: number;
      payload: UpdateSeriesPayload;
    }) => {
      return API.request<SeriesResponse>(
        "PATCH",
        `/series/${seriesId}`,
        payload,
      );
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveSeries, variables.seriesId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListSeries] });
      // Invalidate book queries since they display series info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
      // Series name shows up in search — invalidate global search results.
      queryClient.invalidateQueries({
        queryKey: [SearchQueryKey.GlobalSearch],
      });
    },
  });
};

export const useMergeSeries = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      targetId,
      sourceId,
    }: {
      targetId: number;
      sourceId: number;
    }) => {
      return API.request<void>("POST", `/series/${targetId}/merge`, {
        source_id: sourceId,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveSeries, variables.targetId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListSeries] });
      // Invalidate series books query since books moved from source to target
      queryClient.invalidateQueries({
        queryKey: [QueryKey.SeriesBooks, variables.targetId],
      });
      // Invalidate book queries since they display series info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
      // Series name shows up in search — invalidate global search results.
      queryClient.invalidateQueries({
        queryKey: [SearchQueryKey.GlobalSearch],
      });
    },
  });
};

export const useDeleteSeries = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ seriesId }: { seriesId: number }) => {
      return API.request<void>("DELETE", `/series/${seriesId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListSeries] });
      // Invalidate book queries since they display series info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
      // Series name shows up in search — invalidate global search results.
      queryClient.invalidateQueries({
        queryKey: [SearchQueryKey.GlobalSearch],
      });
    },
  });
};
