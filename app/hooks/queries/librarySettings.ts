import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { API, ShishoAPIError } from "@/libraries/api";

export interface LibrarySettings {
  sort_spec: string | null;
}

export interface UpdateLibrarySettingsPayload {
  sort_spec: string | null;
}

export enum QueryKey {
  LibrarySettings = "LibrarySettings",
}

export const useLibrarySettings = (
  libraryId: number,
  options: Omit<
    UseQueryOptions<LibrarySettings, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<LibrarySettings, ShishoAPIError>({
    enabled: Boolean(libraryId),
    ...options,
    queryKey: [QueryKey.LibrarySettings, libraryId],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/settings/libraries/${libraryId}`,
        null,
        null,
        signal,
      );
    },
  });
};

export const useUpdateLibrarySettings = (libraryId: number) => {
  const queryClient = useQueryClient();

  return useMutation<
    LibrarySettings,
    ShishoAPIError,
    UpdateLibrarySettingsPayload
  >({
    mutationFn: (payload) => {
      return API.request(
        "PUT",
        `/settings/libraries/${libraryId}`,
        payload,
        null,
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.LibrarySettings, libraryId], data);
      // Gallery ordering may change, so invalidate the list cache.
      // RetrieveBook is intentionally not invalidated — sort preferences
      // don't change individual book data.
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
    },
  });
};
