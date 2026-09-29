import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  CreateJobPayload,
  Job,
  ListJobLogsResponse,
  ListJobsQuery,
  ListJobsResponse,
} from "@/types";

import { useRequires } from "./permissions";

export enum QueryKey {
  RetrieveJob = "RetrieveJob",
  ListJobs = "ListJobs",
  ListJobLogs = "ListJobLogs",
  LatestScanJob = "LatestScanJob",
}

type ListJobsData = ListJobsResponse;

// GET /jobs/:id needs no role permission: the handler serves any job to a
// Jobs Read role and a bulk-download job to the user who created it. So the
// hook does not require Jobs Read; its page (JobDetail) is guarded by it.
export const useJob = (
  jobId?: string,
  options: Omit<
    UseQueryOptions<Job, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Job, ShishoAPIError>({
    ...options,
    enabled: options.enabled ?? Boolean(jobId),
    queryKey: [QueryKey.RetrieveJob, jobId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/jobs/${jobId}`, null, null, signal);
    },
  });
};

export const useJobs = (
  query: ListJobsQuery = {},
  options: Omit<
    UseQueryOptions<ListJobsData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListJobsData, ShishoAPIError>({
    ...options,
    enabled: useRequires("jobs:read", options.enabled ?? true),
    queryKey: [QueryKey.ListJobs, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/jobs", null, query, signal);
    },
  });
};

export const useLatestScanJob = (libraryId: number | undefined) => {
  const enabled = useRequires("jobs:read", libraryId !== undefined);
  return useQuery<ListJobsData, ShishoAPIError>({
    queryKey: [QueryKey.LatestScanJob, libraryId],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        "/jobs",
        null,
        {
          type: "scan",
          library_id_or_global: libraryId,
          limit: 1,
        },
        signal,
      );
    },
    enabled,
  });
};

type ListJobLogsData = ListJobLogsResponse;

interface UseJobLogsOptions {
  afterId?: number;
  level?: string[];
  search?: string;
  plugin?: string;
}

export const useJobLogs = (
  jobId?: string,
  options: UseJobLogsOptions = {},
  queryOptions: Omit<
    UseQueryOptions<ListJobLogsData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListJobLogsData, ShishoAPIError>({
    ...queryOptions,
    enabled: useRequires("jobs:read", queryOptions.enabled ?? Boolean(jobId)),
    queryKey: [QueryKey.ListJobLogs, jobId, options],
    queryFn: ({ signal }) => {
      const params: Record<string, string | string[]> = {};
      if (options.afterId !== undefined) {
        params.after_id = String(options.afterId);
      }
      if (options.level && options.level.length > 0) {
        params.level = options.level;
      }
      if (options.search) {
        params.search = options.search;
      }
      if (options.plugin) {
        params.plugin = options.plugin;
      }
      return API.request("GET", `/jobs/${jobId}/logs`, null, params, signal);
    },
  });
};

interface CreateJobMutationVariables {
  payload: CreateJobPayload;
}

export const useCreateJob = () => {
  const queryClient = useQueryClient();

  return useMutation<Job, ShishoAPIError, CreateJobMutationVariables>({
    mutationFn: ({ payload }) => {
      return API.request("POST", "/jobs", payload, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListJobs] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.LatestScanJob] });
    },
  });
};
