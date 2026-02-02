import { useQuery, type UseQueryOptions } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";

export enum QueryKey {
  GlobalSearch = "GlobalSearch",
}

export interface BookSearchResult {
  id: number;
  title: string;
  subtitle: string | null;
  authors: string;
  file_types: string[];
  library_id: number;
}

export interface SeriesSearchResult {
  id: number;
  name: string;
  book_count: number;
  library_id: number;
}

export interface PersonSearchResult {
  id: number;
  name: string;
  sort_name: string;
  library_id: number;
}

export interface GlobalSearchResult {
  books: BookSearchResult[];
  series: SeriesSearchResult[];
  people: PersonSearchResult[];
}

export interface GlobalSearchQuery {
  q: string;
  library_id: number;
}

export const useGlobalSearch = (
  query: GlobalSearchQuery,
  options: Omit<
    UseQueryOptions<GlobalSearchResult, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<GlobalSearchResult, ShishoAPIError>({
    enabled:
      options.enabled !== undefined
        ? options.enabled
        : Boolean(query.q && query.library_id),
    ...options,
    queryKey: [QueryKey.GlobalSearch, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/search", null, query, signal);
    },
  });
};
