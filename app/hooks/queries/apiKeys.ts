import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { APIKey, APIKeyShortURL } from "@/types/generated/apikeys";

export enum QueryKey {
  ListApiKeys = "ListApiKeys",
}

export const useApiKeys = () => {
  return useQuery<APIKey[], ShishoAPIError>({
    queryKey: [QueryKey.ListApiKeys],
    queryFn: () => API.listApiKeys(),
  });
};

export const useCreateApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation<APIKey, ShishoAPIError, string>({
    mutationFn: (name) => API.createApiKey(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListApiKeys] });
    },
  });
};

export const useUpdateApiKeyName = () => {
  const queryClient = useQueryClient();
  return useMutation<APIKey, ShishoAPIError, { id: string; name: string }>({
    mutationFn: ({ id, name }) => API.updateApiKeyName(id, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListApiKeys] });
    },
  });
};

export const useDeleteApiKey = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, string>({
    mutationFn: (id) => API.deleteApiKey(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListApiKeys] });
    },
  });
};

export const useAddApiKeyPermission = () => {
  const queryClient = useQueryClient();
  return useMutation<
    APIKey,
    ShishoAPIError,
    { id: string; permission: string }
  >({
    mutationFn: ({ id, permission }) => API.addApiKeyPermission(id, permission),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListApiKeys] });
    },
  });
};

export const useRemoveApiKeyPermission = () => {
  const queryClient = useQueryClient();
  return useMutation<
    APIKey,
    ShishoAPIError,
    { id: string; permission: string }
  >({
    mutationFn: ({ id, permission }) =>
      API.removeApiKeyPermission(id, permission),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListApiKeys] });
    },
  });
};

export const useGenerateShortUrl = () => {
  return useMutation<APIKeyShortURL, ShishoAPIError, string>({
    mutationFn: (id) => API.generateApiKeyShortUrl(id),
  });
};

export const useClearKoboSync = () => {
  return useMutation<void, ShishoAPIError, string>({
    mutationFn: (id) => API.clearKoboSync(id),
  });
};
