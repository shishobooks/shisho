import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { GenreResponse, ListGenresResponse } from "@/types";
import type {
  ListGenresQuery,
  UpdateGenrePayload,
} from "@/types/generated/genres";

import { QueryKey as BooksQueryKey } from "./books";
import { useRequires } from "./permissions";

export enum QueryKey {
  ListGenres = "ListGenres",
  RetrieveGenre = "RetrieveGenre",
}

export type ListGenresData = ListGenresResponse;

export const useGenresList = (
  query: ListGenresQuery = {},
  options: Omit<
    UseQueryOptions<ListGenresData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListGenresData, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ListGenres, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/genres", null, query, signal);
    },
  });
};

export const useGenre = (
  genreId?: number,
  options: Omit<
    UseQueryOptions<GenreResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<GenreResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? Boolean(genreId)),
    queryKey: [QueryKey.RetrieveGenre, genreId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/genres/${genreId}`, null, null, signal);
    },
  });
};

// Several genres at once, sharing the cache entries of useGenre.
export const useGenresByIds = (ids: number[]) => {
  const permitted = useRequires("books:read");
  return useQueries({
    queries: ids.map((id) => ({
      queryKey: [QueryKey.RetrieveGenre, id],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        API.request<GenreResponse>("GET", `/genres/${id}`, null, null, signal),
      enabled: permitted,
    })),
  });
};

export const useUpdateGenre = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      genreId,
      payload,
    }: {
      genreId: number;
      payload: UpdateGenrePayload;
    }) => {
      return API.request<GenreResponse>("PATCH", `/genres/${genreId}`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveGenre, variables.genreId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListGenres] });
      // Invalidate book queries since they display genre info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useDeleteGenre = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ genreId }: { genreId: number }) => {
      return API.request<void>("DELETE", `/genres/${genreId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListGenres] });
      // Invalidate book queries since they display genre info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useMergeGenre = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      targetId,
      sourceId,
    }: {
      targetId: number;
      sourceId: number;
    }) => {
      return API.request<void>("POST", `/genres/${targetId}/merge`, {
        source_id: sourceId,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveGenre, variables.targetId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListGenres] });
      // Invalidate book queries since they display genre info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};
