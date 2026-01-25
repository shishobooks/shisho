import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { CreateJobPayload, Job, JobLog, ListJobsQuery } from "@/types";

export enum QueryKey {
  RetrieveJob = "RetrieveJob",
  ListJobs = "ListJobs",
  ListJobLogs = "ListJobLogs",
  LatestScanJob = "LatestScanJob",
}

export const useJob = (
  id?: string,
  options: Omit<
    UseQueryOptions<Job, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Job, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(id),
    ...options,
    queryKey: [QueryKey.RetrieveJob, id],
    queryFn: ({ signal }) => {
      return API.request("GET", `/jobs/${id}`, null, null, signal);
    },
  });
};

interface ListJobsData {
  jobs: Job[];
  total: number;
}

export const useJobs = (
  query: ListJobsQuery = {},
  options: Omit<
    UseQueryOptions<ListJobsData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListJobsData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListJobs, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/jobs", null, query, signal);
    },
  });
};

export const useLatestScanJob = (libraryId: number | undefined) => {
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
    enabled: libraryId !== undefined,
    refetchInterval: (query) => {
      const job = query.state.data?.jobs[0];
      const isActive =
        job?.status === "pending" || job?.status === "in_progress";
      return isActive ? 2000 : 30000;
    },
  });
};

interface ListJobLogsData {
  logs: JobLog[];
  job: Job;
}

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
    enabled:
      queryOptions.enabled !== undefined
        ? queryOptions.enabled
        : Boolean(jobId),
    ...queryOptions,
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
    onSuccess: (data: Job) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListJobs] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.LatestScanJob] });
      queryClient.setQueryData([QueryKey.RetrieveJob, data.id], data);
    },
  });
};
