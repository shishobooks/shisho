import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { useAuth } from "@/hooks/useAuth";
import { API, ShishoAPIError } from "@/libraries/api";
import {
  mergeDemoSettings,
  readDemoSettings,
  writeDemoSettings,
} from "@/libraries/demoSettings";
import {
  type LibrarySettingsResponse,
  type UpdateLibrarySettingsPayload,
} from "@/types";

export enum QueryKey {
  LibrarySettings = "LibrarySettings",
}

export const getDemoLibrarySettingsKey = (libraryId: number) =>
  `shisho-demo-library-settings-${libraryId}`;

const loadLibrarySettings = async (
  libraryId: number,
  demoMode: boolean,
  signal?: AbortSignal,
) => {
  const serverSettings = await API.request<LibrarySettingsResponse>(
    "GET",
    `/settings/libraries/${libraryId}`,
    null,
    null,
    signal,
  );
  return demoMode
    ? readDemoSettings(getDemoLibrarySettingsKey(libraryId), serverSettings)
    : serverSettings;
};

export const useLibrarySettings = (
  libraryId: number,
  options: Omit<
    UseQueryOptions<LibrarySettingsResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  const { demoMode } = useAuth();

  return useQuery<LibrarySettingsResponse, ShishoAPIError>({
    enabled: Boolean(libraryId),
    ...options,
    queryKey: [QueryKey.LibrarySettings, libraryId],
    queryFn: ({ signal }) => loadLibrarySettings(libraryId, demoMode, signal),
  });
};

export const useUpdateLibrarySettings = (libraryId: number) => {
  const queryClient = useQueryClient();
  const { demoMode } = useAuth();

  return useMutation<
    LibrarySettingsResponse,
    ShishoAPIError,
    UpdateLibrarySettingsPayload
  >({
    mutationFn: async (payload) => {
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
      if (!demoMode) {
        return API.request(
          "PUT",
          `/settings/libraries/${libraryId}`,
          payload,
          null,
        );
      }

      const current =
        queryClient.getQueryData<LibrarySettingsResponse>([
          QueryKey.LibrarySettings,
          libraryId,
        ]) ?? (await loadLibrarySettings(libraryId, true));
      const updated = mergeDemoSettings(current, payload);
      writeDemoSettings(getDemoLibrarySettingsKey(libraryId), updated);
      return updated;
    },
    onSuccess: (data) => {
      // Optimistically write the freshly-saved settings into the cache so
      // dependent components (Home gallery's effective-sort logic, the
      // SortSheet's "dirty" indicator) re-render immediately without
      // waiting for a refetch.
      queryClient.setQueryData([QueryKey.LibrarySettings, libraryId], data);
      // Server-backed settings refetch to converge on server-set fields.
      // Demo Mode settings already have their authoritative browser-local
      // value in the cache and must not issue a follow-up request.
      if (!demoMode) {
        queryClient.invalidateQueries({
          queryKey: [QueryKey.LibrarySettings, libraryId],
        });
      }
      // Gallery ordering may change, so invalidate the list cache.
      // RetrieveBook is intentionally not invalidated — sort preferences
      // don't change individual book data.
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
    },
  });
};
