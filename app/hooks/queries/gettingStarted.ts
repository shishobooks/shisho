import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  GettingStartedResponse,
  GettingStartedStepKey,
} from "@/types/generated/gettingstarted";

export enum QueryKey {
  GettingStarted = "GettingStarted",
}

/**
 * Refetches the signed-in user's Getting Started list. Every mutation that
 * creates what a step asks for (a Library, a metadata plugin, a user, a
 * Device, a To Read entry, a Read-through) calls it, so the step ticks itself
 * off.
 */
export const invalidateGettingStarted = (queryClient: QueryClient) =>
  queryClient.invalidateQueries({ queryKey: [QueryKey.GettingStarted] });

// The signed-in user's own list. Every signed-in user has one, so the route
// needs no permission.
export const useGettingStarted = () =>
  useQuery<GettingStartedResponse, ShishoAPIError>({
    queryKey: [QueryKey.GettingStarted],
    queryFn: ({ signal }) =>
      API.request("GET", "/getting-started", null, null, signal),
  });

// Every change to the list answers with the whole list, which replaces the
// cached one.
const useGettingStartedMutation = <TVariables = void>(
  request: (variables: TVariables) => Promise<GettingStartedResponse>,
) => {
  const queryClient = useQueryClient();
  return useMutation<GettingStartedResponse, ShishoAPIError, TVariables>({
    mutationFn: request,
    onSuccess: (data) =>
      queryClient.setQueryData([QueryKey.GettingStarted], data),
  });
};

interface SetNotForMeVariables {
  step: GettingStartedStepKey;
  notForMe: boolean;
}

export const useSetGettingStartedNotForMe = () =>
  useGettingStartedMutation<SetNotForMeVariables>(({ step, notForMe }) =>
    API.request(
      notForMe ? "POST" : "DELETE",
      `/getting-started/steps/${step}/not-for-me`,
    ),
  );

export const useDismissGettingStarted = () =>
  useGettingStartedMutation(() =>
    API.request("POST", "/getting-started/dismiss"),
  );

export const useRestoreGettingStarted = () =>
  useGettingStartedMutation(() =>
    API.request("POST", "/getting-started/restore"),
  );

// Clears the flag in the cache before the request, so the panel can't open
// on its own twice while it is in flight.
export const useClearGettingStartedAutoOpen = () => {
  const queryClient = useQueryClient();
  return useMutation<GettingStartedResponse, ShishoAPIError, void>({
    mutationFn: () => API.request("DELETE", "/getting-started/auto-open"),
    onMutate: () =>
      queryClient.setQueryData<GettingStartedResponse>(
        [QueryKey.GettingStarted],
        (data) => data && { ...data, auto_open: false },
      ),
    onSuccess: (data) =>
      queryClient.setQueryData([QueryKey.GettingStarted], data),
  });
};

// An admin's Show Getting Started for another user. The server answers with
// nothing: an admin never sees which of the user's steps are done.
export const useShowGettingStarted = () =>
  useMutation<void, ShishoAPIError, number>({
    mutationFn: (userId) =>
      API.request("POST", `/users/${userId}/show-getting-started`),
  });
