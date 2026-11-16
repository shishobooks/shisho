import { useQuery } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { UpdateCheckResponse } from "@/types/generated/updatecheck";

import { useRequires } from "./permissions";

export enum QueryKey {
  UpdateCheck = "UpdateCheck",
}

// The server caches GitHub's answer for hours, so asking it hourly is cheap
// and lets a tab left open notice a release.
const REFETCH_INTERVAL_MS = 60 * 60 * 1000;

export const useUpdateCheck = () =>
  useQuery<UpdateCheckResponse, ShishoAPIError>({
    enabled: useRequires("config:read"),
    queryKey: [QueryKey.UpdateCheck],
    queryFn: ({ signal }) =>
      API.request("GET", "/update-check", null, null, signal),
    refetchInterval: REFETCH_INTERVAL_MS,
    // The sidebar remounts on every layout change and the phone drawer
    // stays mounted; the interval alone keeps both current.
    refetchOnMount: false,
  });
