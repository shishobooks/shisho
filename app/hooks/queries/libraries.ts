import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  CreateLibraryPayload,
  Library,
  ListLibrariesQuery,
  UpdateLibraryPayload,
} from "@/types";

import { QueryKey as BooksQueryKey } from "./books";
import { QueryKey as LibrarySettingsQueryKey } from "./librarySettings";

export enum QueryKey {
  RetrieveLibrary = "RetrieveLibrary",
  ListLibraries = "ListLibraries",
  LibraryLanguages = "LibraryLanguages",
}

export const useLibrary = (
  id?: string,
  options: Omit<
    UseQueryOptions<Library, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Library, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(id),
    ...options,
    queryKey: [QueryKey.RetrieveLibrary, id],
    queryFn: ({ signal }) => {
      return API.request("GET", `/libraries/${id}`, null, null, signal);
    },
  });
};

interface ListLibrariesData {
  libraries: Library[];
  total: number;
}

export const useLibraries = (
  query: ListLibrariesQuery = {},
  options: Omit<
    UseQueryOptions<ListLibrariesData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListLibrariesData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListLibraries, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/libraries", null, query, signal);
    },
  });
};

interface CreateLibraryMutationVariables {
  payload: CreateLibraryPayload;
}

export const useCreateLibrary = () => {
  const queryClient = useQueryClient();

  return useMutation<Library, ShishoAPIError, CreateLibraryMutationVariables>({
    mutationFn: ({ payload }) => {
      return API.request("POST", "/libraries", payload, null);
    },
    onSuccess: (data: Library) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLibraries] });
      queryClient.setQueryData(
        [QueryKey.RetrieveLibrary, String(data.id)],
        data,
      );
    },
  });
};

interface UpdateLibraryMutationVariables {
  id: string;
  payload: UpdateLibraryPayload;
}

export const useUpdateLibrary = () => {
  const queryClient = useQueryClient();

  return useMutation<Library, ShishoAPIError, UpdateLibraryMutationVariables>({
    mutationFn: ({ id, payload }) => {
      return API.request("POST", `/libraries/${id}`, payload, null);
    },
    onSuccess: (data: Library) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLibraries] });
      queryClient.setQueryData(
        [QueryKey.RetrieveLibrary, String(data.id)],
        data,
      );
    },
  });
};

export const useDeleteLibrary = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, { id: number }>({
    mutationFn: ({ id }) => {
      return API.request("DELETE", `/libraries/${id}`);
    },
    onSuccess: (_data, { id }) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLibraries] });
      queryClient.removeQueries({
        queryKey: [QueryKey.RetrieveLibrary, String(id)],
      });
      queryClient.removeQueries({
        queryKey: [QueryKey.LibraryLanguages, id],
      });
      queryClient.removeQueries({
        queryKey: [LibrarySettingsQueryKey.LibrarySettings, id],
      });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useLibraryLanguages = (
  libraryId?: number,
  options: Omit<
    UseQueryOptions<string[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<string[], ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(libraryId),
    staleTime: 5 * 60 * 1000, // Languages change infrequently; avoid refetching on every page visit
    ...options,
    queryKey: [QueryKey.LibraryLanguages, libraryId],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/libraries/${libraryId}/languages`,
        null,
        null,
        signal,
      );
    },
  });
};
