import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Book, Series } from "@/types";

export enum QueryKey {
  ListSeries = "ListSeries",
  RetrieveSeries = "RetrieveSeries",
  SeriesBooks = "SeriesBooks",
}

export interface ListSeriesQuery {
  limit?: number;
  offset?: number;
  library_id?: number;
}

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
    UseQueryOptions<Series, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Series, ShishoAPIError>({
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
