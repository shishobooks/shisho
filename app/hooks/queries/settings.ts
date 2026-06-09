import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { UserSettingsPayload, UserSettingsResponse } from "@/types";

export enum QueryKey {
  UserSettings = "UserSettings",
}

export const useUserSettings = (
  options: Omit<
    UseQueryOptions<UserSettingsResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<UserSettingsResponse, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.UserSettings],
    queryFn: ({ signal }) => {
      return API.request("GET", "/settings/user", null, null, signal);
    },
  });
};

export const useUpdateUserSettings = () => {
  const queryClient = useQueryClient();

  // Partial update: callers send only the fields they want to change. Omitted
  // (or undefined) fields are left untouched on the server. The generated
  // UserSettingsPayload has all fields as optional, mirroring the backend's
  // omitempty pointers.
  return useMutation<UserSettingsResponse, ShishoAPIError, UserSettingsPayload>(
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
