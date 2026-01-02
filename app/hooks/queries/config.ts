import { useQuery } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Config } from "@/types/generated/config";

export const useConfig = () => {
  return useQuery<Config, ShishoAPIError>({
    queryKey: ["config"],
    queryFn: ({ signal }) => {
      return API.request("GET", "/config", null, null, signal);
    },
  });
};
