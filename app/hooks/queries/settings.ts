import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { EpubFlow, EpubTheme, FitMode } from "@/types";

export interface ViewerSettings {
  preload_count: number;
  fit_mode: FitMode;
  viewer_epub_font_size: number;
  viewer_epub_theme: EpubTheme;
  viewer_epub_flow: EpubFlow;
}

export enum QueryKey {
  ViewerSettings = "ViewerSettings",
}

export const useViewerSettings = (
  options: Omit<
    UseQueryOptions<ViewerSettings, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ViewerSettings, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ViewerSettings],
    queryFn: ({ signal }) => {
      return API.request("GET", "/settings/viewer", null, null, signal);
    },
  });
};

// Partial update: callers send only the fields they want to change. Omitted
// (or undefined) fields are left untouched on the server. Mirrors the backend
// ViewerSettingsPayload shape, which has all fields as omitempty pointers.
export interface UpdateViewerSettingsVariables {
  preload_count?: number;
  fit_mode?: FitMode;
  viewer_epub_font_size?: number;
  viewer_epub_theme?: EpubTheme;
  viewer_epub_flow?: EpubFlow;
}

export const useUpdateViewerSettings = () => {
  const queryClient = useQueryClient();

  return useMutation<
    ViewerSettings,
    ShishoAPIError,
    UpdateViewerSettingsVariables
  >({
    mutationFn: (payload) => {
      return API.request("PUT", "/settings/viewer", payload, null);
    },
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.ViewerSettings], data);
    },
  });
};
