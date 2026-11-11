import { useQuery } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { StatsResponse } from "@/types/generated/serverstats";

import { useRequires } from "./permissions";

export enum QueryKey {
  ServerStats = "ServerStats",
}

// Server Stats is computed on each request, so the page refetches whenever
// it opens rather than showing figures from an earlier visit.
export const useServerStats = () =>
  useQuery<StatsResponse, ShishoAPIError>({
    enabled: useRequires("config:read"),
    queryKey: [QueryKey.ServerStats],
    queryFn: ({ signal }) =>
      API.request("GET", "/server-stats", null, null, signal),
    refetchOnMount: "always",
  });
