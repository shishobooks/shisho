import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { ClearResponse, Info } from "@/types/generated/cache";

import { useRequires } from "./permissions";

export enum QueryKey {
  ListCaches = "ListCaches",
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
