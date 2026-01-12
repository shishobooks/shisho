import { QueryKey as BooksQueryKey } from "./books";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Book, Genre } from "@/types";
import type {
  ListGenresQuery,
  UpdateGenrePayload,
} from "@/types/generated/genres";

export enum QueryKey {
  ListGenres = "ListGenres",
  RetrieveGenre = "RetrieveGenre",
  GenreBooks = "GenreBooks",
}

export interface ListGenresData {
  genres: Genre[];
  total: number;
}

export const useGenresList = (
  query: ListGenresQuery = {},
  options: Omit<
    UseQueryOptions<ListGenresData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListGenresData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListGenres, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/genres", null, query, signal);
    },
  });
};

export const useGenre = (
  genreId?: number,
  options: Omit<
    UseQueryOptions<Genre, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Genre, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(genreId),
    ...options,
    queryKey: [QueryKey.RetrieveGenre, genreId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/genres/${genreId}`, null, null, signal);
    },
  });
};

export const useGenreBooks = (
  genreId?: number,
  options: Omit<
    UseQueryOptions<Book[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Book[], ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(genreId),
    ...options,
    queryKey: [QueryKey.GenreBooks, genreId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/genres/${genreId}/books`, null, null, signal);
    },
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
      return API.request<Genre>("PATCH", `/genres/${genreId}`, payload);
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
