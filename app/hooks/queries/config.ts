import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Settings, UpdateConfigPayload } from "@/types/generated/config";

export const useConfig = () => {
  return useQuery<Settings, ShishoAPIError>({
    queryKey: ["config"],
    queryFn: ({ signal }) => {
      return API.request("GET", "/config", null, null, signal);
    },
  });
};

export const useUpdateConfig = () => {
  const queryClient = useQueryClient();

  return useMutation<Settings, ShishoAPIError, UpdateConfigPayload>({
    mutationFn: (payload) => {
      return API.request("POST", "/config", payload, null);
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["config"], data);
    },
  });
};
