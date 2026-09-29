import { useQuery } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Config } from "@/types/generated/config";

import { useRequires } from "./permissions";

export enum QueryKey {
  RetrieveConfig = "RetrieveConfig",
}

export const useConfig = () => {
  return useQuery<Config, ShishoAPIError>({
    enabled: useRequires("config:read"),
    queryKey: [QueryKey.RetrieveConfig],
    queryFn: ({ signal }) => {
      return API.request("GET", "/config", null, null, signal);
    },
  });
};
