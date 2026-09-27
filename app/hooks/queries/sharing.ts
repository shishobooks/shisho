import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  SharingSettingsResponse,
  UpdateSharingSettingsPayload,
} from "@/types";

export enum QueryKey {
  SharingSettings = "SharingSettings",
}

export const useSharingSettings = () =>
  useQuery<SharingSettingsResponse, ShishoAPIError>({
    queryKey: [QueryKey.SharingSettings],
    queryFn: ({ signal }) =>
      API.request("GET", "/settings/sharing", null, null, signal),
  });

// Partial update: callers send only the switch that changed.
export const useUpdateSharingSettings = () => {
  const queryClient = useQueryClient();
  return useMutation<
    SharingSettingsResponse,
    ShishoAPIError,
    UpdateSharingSettingsPayload
  >({
    mutationFn: (payload) =>
      API.request("PUT", "/settings/sharing", payload, null),
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.SharingSettings], data);
    },
  });
};
