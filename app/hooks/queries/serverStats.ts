import {
  keepPreviousData,
  useQuery,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  ReadingQuery,
  ReadingResponse,
  StatsResponse,
} from "@/types/generated/serverstats";

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

// Reading across the server for one year, or all time with no year. The
// previous range stays on screen while another loads.
export const useServerStatsReading = (
  query: ReadingQuery,
  options: Omit<
    UseQueryOptions<ReadingResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) =>
  useQuery<ReadingResponse, ShishoAPIError>({
    placeholderData: keepPreviousData,
    refetchOnMount: "always",
    ...options,
    enabled: useRequires("config:read", options.enabled ?? true),
    queryKey: [QueryKey.ServerStats, "reading", query],
    queryFn: ({ signal }) =>
      API.request("GET", "/server-stats/reading", null, query, signal),
  });
