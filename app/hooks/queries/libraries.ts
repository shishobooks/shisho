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
  LibrarySummary,
  ListLibrariesQuery,
  ListLibrariesResponse,
  UpdateLibraryPayload,
} from "@/types";

import { QueryKey as BooksQueryKey } from "./books";
import { QueryKey as LibrarySettingsQueryKey } from "./librarySettings";
import { anyOf, useRequires } from "./permissions";

export enum QueryKey {
  RetrieveLibrary = "RetrieveLibrary",
  ListLibraries = "ListLibraries",
  LibraryLanguages = "LibraryLanguages",
  UserLibraries = "UserLibraries",
}

export const useLibrary = (
  id?: string,
  options: Omit<
    UseQueryOptions<Library, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Library, ShishoAPIError>({
    ...options,
    enabled: useRequires("libraries:read", options.enabled ?? Boolean(id)),
    queryKey: [QueryKey.RetrieveLibrary, id],
    queryFn: ({ signal }) => {
      return API.request("GET", `/libraries/${id}`, null, null, signal);
    },
  });
};

export const useLibraries = (
  query: ListLibrariesQuery = {},
  options: Omit<
    UseQueryOptions<ListLibrariesResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListLibrariesResponse, ShishoAPIError>({
    ...options,
    // Users Write lists libraries to assign access to a user.
    enabled: useRequires(
      anyOf("libraries:read", "users:write"),
      options.enabled ?? true,
    ),
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
      queryClient.invalidateQueries({ queryKey: [QueryKey.UserLibraries] });
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
      queryClient.invalidateQueries({ queryKey: [QueryKey.UserLibraries] });
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
      queryClient.invalidateQueries({ queryKey: [QueryKey.UserLibraries] });
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
    staleTime: 5 * 60 * 1000, // Languages change infrequently; avoid refetching on every page visit
    ...options,
    enabled: useRequires("books:read", options.enabled ?? Boolean(libraryId)),
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

// The signed-in user's accessible libraries, without paths. Any signed-in
// role may read it, so reader pages (breadcrumbs, cover aspect ratio, download
// preference, the Kobo sync scope, and the Merge and Move dialogs) use it
// instead of the Libraries Read routes above. The navigation uses
// useNavLibraries below.
const fetchUserLibraries = ({ signal }: { signal: AbortSignal }) =>
  API.request<LibrarySummary[]>("GET", "/user/libraries", null, null, signal);

export const useUserLibraries = (
  options: Omit<
    UseQueryOptions<LibrarySummary[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<LibrarySummary[], ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.UserLibraries],
    queryFn: fetchUserLibraries,
  });
};

// The libraries the navigation offers to switch to: the library picker, the
// mobile drawer, and the redirect from `/`. Every library page needs Books
// Read, so without it this sends nothing and returns no libraries, even when
// the Kobo sync scope in Security Settings has cached them.
export const useNavLibraries = () => {
  const canOpenLibraries = useRequires("books:read");
  return useQuery<LibrarySummary[], ShishoAPIError>({
    enabled: canOpenLibraries,
    queryKey: [QueryKey.UserLibraries],
    queryFn: fetchUserLibraries,
    select: (libraries) => (canOpenLibraries ? libraries : []),
  });
};

// One library from useUserLibraries. `data` is undefined while the list loads
// and when the library is not in it.
export const useUserLibrary = (id?: string | number) => {
  const libraryId = Number(id);
  return useQuery<LibrarySummary[], ShishoAPIError, LibrarySummary | undefined>(
    {
      queryKey: [QueryKey.UserLibraries],
      queryFn: fetchUserLibraries,
      enabled: Boolean(id),
      select: (libraries) => libraries.find((l) => l.id === libraryId),
    },
  );
};
