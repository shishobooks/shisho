import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { EpubFlow, EpubTheme, FitMode } from "@/types";

export interface UserSettings {
  preload_count: number;
  fit_mode: FitMode;
  viewer_epub_font_size: number;
  viewer_epub_theme: EpubTheme;
  viewer_epub_flow: EpubFlow;
}

export enum QueryKey {
  UserSettings = "UserSettings",
}

export const useUserSettings = (
  options: Omit<
    UseQueryOptions<UserSettings, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<UserSettings, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.UserSettings],
    queryFn: ({ signal }) => {
      return API.request("GET", "/settings/user", null, null, signal);
    },
  });
};

// Partial update: callers send only the fields they want to change. Omitted
// (or undefined) fields are left untouched on the server. Mirrors the backend
// UserSettingsPayload shape, which has all fields as omitempty pointers.
export interface UpdateUserSettingsVariables {
  preload_count?: number;
  fit_mode?: FitMode;
  viewer_epub_font_size?: number;
  viewer_epub_theme?: EpubTheme;
  viewer_epub_flow?: EpubFlow;
}

export const useUpdateUserSettings = () => {
  const queryClient = useQueryClient();

  return useMutation<UserSettings, ShishoAPIError, UpdateUserSettingsVariables>(
    {
      mutationFn: (payload) => {
        return API.request("PUT", "/settings/user", payload, null);
      },
      onSuccess: (data) => {
        queryClient.setQueryData([QueryKey.UserSettings], data);
      },
    },
  );
};
