import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { QueryKey as DevicesQueryKey } from "@/hooks/queries/devices";
import { API, ShishoAPIError } from "@/libraries/api";
import type { DefaultRulesResponse, UpdateDeliveryRulesPayload } from "@/types";

export enum QueryKey {
  RetrieveDeliveryRules = "RetrieveDeliveryRules",
}

// The caller's own default Delivery Rules. Any signed-in user has them, so
// the routes need no permission.
export const useDeliveryRules = (options: { enabled?: boolean } = {}) => {
  return useQuery<DefaultRulesResponse, ShishoAPIError>({
    enabled: options.enabled ?? true,
    queryKey: [QueryKey.RetrieveDeliveryRules],
    queryFn: ({ signal }) =>
      API.request("GET", "/user/delivery-rules", null, null, signal),
  });
};

// Devices that follow the default show it as their rules, so a change to it
// refetches them.
const useInvalidateFollowingDevices = () => {
  const queryClient = useQueryClient();
  return () => {
    queryClient.invalidateQueries({ queryKey: [DevicesQueryKey.ListDevices] });
    queryClient.invalidateQueries({
      queryKey: [DevicesQueryKey.RetrieveDevice],
    });
  };
};

export const useUpdateDeliveryRules = () => {
  const queryClient = useQueryClient();
  const invalidateDevices = useInvalidateFollowingDevices();
  return useMutation<
    DefaultRulesResponse,
    ShishoAPIError,
    UpdateDeliveryRulesPayload
  >({
    mutationFn: (payload) =>
      API.request("PUT", "/user/delivery-rules", payload),
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.RetrieveDeliveryRules], data);
      invalidateDevices();
    },
  });
};

export const useResetDeliveryRules = () => {
  const queryClient = useQueryClient();
  const invalidateDevices = useInvalidateFollowingDevices();
  return useMutation<DefaultRulesResponse, ShishoAPIError, void>({
    mutationFn: () => API.request("DELETE", "/user/delivery-rules"),
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.RetrieveDeliveryRules], data);
      invalidateDevices();
    },
  });
};
