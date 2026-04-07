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
      queryClient.setQueryData([QueryKey.RetrieveLibrary, data.id], data);
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
      queryClient.setQueryData([QueryKey.RetrieveLibrary, data.id], data);
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
