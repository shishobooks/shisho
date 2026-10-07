import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  ClearResponse,
  Info,
  SettingsResponse,
  UpdateSettingsPayload,
} from "@/types/generated/cache";

import { useRequires } from "./permissions";

export enum QueryKey {
  ListCaches = "ListCaches",
  CacheSettings = "CacheSettings",
}

export const useCaches = () => {
  return useQuery<Info[], ShishoAPIError>({
    enabled: useRequires("config:read"),
    queryKey: [QueryKey.ListCaches],
    queryFn: ({ signal }) => {
      return API.request("GET", "/cache", null, null, signal);
    },
  });
};

export const useCacheSettings = () =>
  useQuery<SettingsResponse, ShishoAPIError>({
    enabled: useRequires("config:read"),
    queryKey: [QueryKey.CacheSettings],
    queryFn: ({ signal }) =>
      API.request("GET", "/settings/cache", null, null, signal),
  });

export const useUpdateCacheSettings = () => {
  const queryClient = useQueryClient();
  return useMutation<SettingsResponse, ShishoAPIError, UpdateSettingsPayload>({
    mutationFn: (payload) =>
      API.request("PUT", "/settings/cache", payload, null),
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.CacheSettings], data);
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListCaches] });
    },
  });
};

export const useClearCache = () => {
  const queryClient = useQueryClient();
  return useMutation<ClearResponse, ShishoAPIError, string>({
    mutationFn: (id: string) => {
      return API.request(
        "POST",
        `/cache/${encodeURIComponent(id)}/clear`,
        null,
        null,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListCaches] });
    },
  });
};
