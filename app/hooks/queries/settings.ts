import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";

export interface ViewerSettings {
  preload_count: number;
  fit_mode: "fit-height" | "original";
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

interface UpdateViewerSettingsVariables {
  preload_count: number;
  fit_mode: "fit-height" | "original";
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
