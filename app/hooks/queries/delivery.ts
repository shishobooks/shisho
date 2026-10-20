import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

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

export const useUpdateDeliveryRules = () => {
  const queryClient = useQueryClient();
  return useMutation<
    DefaultRulesResponse,
    ShishoAPIError,
    UpdateDeliveryRulesPayload
  >({
    mutationFn: (payload) =>
      API.request("PUT", "/user/delivery-rules", payload),
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.RetrieveDeliveryRules], data);
    },
  });
};

export const useResetDeliveryRules = () => {
  const queryClient = useQueryClient();
  return useMutation<DefaultRulesResponse, ShishoAPIError, void>({
    mutationFn: () => API.request("DELETE", "/user/delivery-rules"),
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.RetrieveDeliveryRules], data);
    },
  });
};
