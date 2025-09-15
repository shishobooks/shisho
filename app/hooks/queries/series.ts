import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Book } from "@/types";

export enum QueryKey {
  ListSeries = "ListSeries",
  SeriesBooks = "SeriesBooks",
}

export interface SeriesInfo {
  name: string;
  book_count: number;
}

export interface ListSeriesQuery {
  limit?: number;
  offset?: number;
}

export interface ListSeriesData {
  series: SeriesInfo[];
  total: number;
}

export const useSeries = (
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

export const useSeriesBooks = (
  seriesName?: string,
  options: Omit<
    UseQueryOptions<Book[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Book[], ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(seriesName),
    ...options,
    queryKey: [QueryKey.SeriesBooks, seriesName],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/series/${encodeURIComponent(seriesName!)}/books`,
        null,
        null,
        signal,
      );
    },
  });
};
