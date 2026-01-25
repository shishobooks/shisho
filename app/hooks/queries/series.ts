import { QueryKey as BooksQueryKey } from "./books";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Book, Series } from "@/types";
import type {
  ListSeriesQuery,
  UpdateSeriesPayload,
} from "@/types/generated/series";

export enum QueryKey {
  ListSeries = "ListSeries",
  RetrieveSeries = "RetrieveSeries",
  SeriesBooks = "SeriesBooks",
}

export interface SeriesWithCount extends Series {
  book_count: number;
}

export type { ListSeriesQuery };

export interface ListSeriesData {
  series: Series[];
  total: number;
}

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
    UseQueryOptions<SeriesWithCount, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<SeriesWithCount, ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(seriesId),
    ...options,
    queryKey: [QueryKey.RetrieveSeries, seriesId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/series/${seriesId}`, null, null, signal);
    },
  });
};

export const useSeriesBooks = (
  seriesId?: number,
  options: Omit<
    UseQueryOptions<Book[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Book[], ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(seriesId),
    ...options,
    queryKey: [QueryKey.SeriesBooks, seriesId],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/series/${seriesId}/books`,
        null,
        null,
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
      return API.request<SeriesWithCount>(
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
    },
  });
};
