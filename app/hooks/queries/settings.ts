import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { useAuth } from "@/hooks/useAuth";
import { API, ShishoAPIError } from "@/libraries/api";
import {
  mergeDemoSettings,
  readDemoSettings,
  writeDemoSettings,
} from "@/libraries/demoSettings";
import type { UserSettingsPayload, UserSettingsResponse } from "@/types";

export enum QueryKey {
  UserSettings = "UserSettings",
}

export const DEMO_USER_SETTINGS_KEY = "shisho-demo-user-settings";

const loadUserSettings = async (demoMode: boolean, signal?: AbortSignal) => {
  const serverSettings = await API.request<UserSettingsResponse>(
    "GET",
    "/settings/user",
    null,
    null,
    signal,
  );
  return demoMode
    ? readDemoSettings(DEMO_USER_SETTINGS_KEY, serverSettings)
    : serverSettings;
};

export const useUserSettings = (
  options: Omit<
    UseQueryOptions<UserSettingsResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  const { demoMode } = useAuth();

  return useQuery<UserSettingsResponse, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.UserSettings],
    queryFn: ({ signal }) => loadUserSettings(demoMode, signal),
  });
};

export const useUpdateUserSettings = () => {
  const queryClient = useQueryClient();
  const { demoMode } = useAuth();

  // Partial update: callers send only the fields they want to change. Omitted
  // (or undefined) fields are left untouched on the server. The generated
  // UserSettingsPayload has all fields as optional, mirroring the backend's
  // omitempty pointers.
  return useMutation<UserSettingsResponse, ShishoAPIError, UserSettingsPayload>(
    {
      mutationFn: async (payload) => {
        if (!demoMode) {
          return API.request("PUT", "/settings/user", payload, null);
        }

        const current =
          queryClient.getQueryData<UserSettingsResponse>([
            QueryKey.UserSettings,
          ]) ?? (await loadUserSettings(true));
        const updated = mergeDemoSettings(current, payload);
        writeDemoSettings(DEMO_USER_SETTINGS_KEY, updated);
        return updated;
      },
      onSuccess: (data) => {
        queryClient.setQueryData([QueryKey.UserSettings], data);
      },
    },
  );
};
