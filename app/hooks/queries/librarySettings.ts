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
      // Defensive guard: callers may construct this hook with a 0
      // placeholder when the route's libraryId param is missing (hooks
      // can't be called conditionally). Refuse to fire the request
      // rather than PUT /settings/libraries/0, which would 404 server
      // side and confuse the user.
      if (!libraryId) {
        return Promise.reject(
          new Error("useUpdateLibrarySettings called without a library id"),
        );
      }
      return API.request(
        "PUT",
        `/settings/libraries/${libraryId}`,
        payload,
        null,
      );
    },
    onSuccess: (data) => {
      // Optimistically write the freshly-saved settings into the cache so
      // dependent components (Home gallery's effective-sort logic, the
      // SortSheet's "dirty" indicator) re-render immediately without
      // waiting for a refetch.
      queryClient.setQueryData([QueryKey.LibrarySettings, libraryId], data);
      // Then invalidate to mark the entry stale and trigger a background
      // refetch. This is belt-and-suspenders: setQueryData already covers
      // the active query, but if another mount/component subscribes to
      // this key (e.g., a settings page open in another tab) we want it
      // to re-fetch and converge on the server's view of the row,
      // including server-set fields (updated_at).
      queryClient.invalidateQueries({
        queryKey: [QueryKey.LibrarySettings, libraryId],
      });
      // Gallery ordering may change, so invalidate the list cache.
      // RetrieveBook is intentionally not invalidated — sort preferences
      // don't change individual book data.
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
    },
  });
};
