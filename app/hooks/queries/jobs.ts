import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { CreateJobPayload, Job, ListJobsQuery } from "@/types";

export enum QueryKey {
  RetrieveJob = "RetrieveJob",
  ListJobs = "ListJobs",
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
      queryClient.setQueryData([QueryKey.RetrieveJob, data.id], data);
    },
  });
};
