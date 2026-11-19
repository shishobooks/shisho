import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { useAuth } from "@/hooks/useAuth";
import { API, ShishoAPIError } from "@/libraries/api";
import type {
  AcceptInvitePayload,
  CreateInviteLinkPayload,
  InviteLinkResponse,
  InviteResponse,
  MeResponse,
} from "@/types";

import { invalidateGettingStarted } from "./gettingStarted";
import { useRequires } from "./permissions";

export enum QueryKey {
  InviteLinks = "InviteLinks",
  Invite = "Invite",
}

// The Invite Links that can still be accepted. Users Write manages them.
// Invites expire on their own, so returning to the page refetches the list.
// Demo Mode refuses the list, so it isn't asked for there.
export const useInviteLinks = (
  options: Omit<
    UseQueryOptions<InviteLinkResponse[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  const { demoMode } = useAuth();
  const permitted = useRequires("users:write", options.enabled ?? true);
  return useQuery<InviteLinkResponse[], ShishoAPIError>({
    staleTime: 0,
    ...options,
    enabled: permitted && !demoMode,
    queryKey: [QueryKey.InviteLinks],
    queryFn: ({ signal }) =>
      API.request("GET", "/invite-links", null, null, signal),
  });
};

// A pending invite ticks "Invite someone", so creating and revoking one
// refetch Getting Started.
export const useCreateInviteLink = () => {
  const queryClient = useQueryClient();
  return useMutation<
    InviteLinkResponse,
    ShishoAPIError,
    CreateInviteLinkPayload
  >({
    mutationFn: (payload) => API.request("POST", "/invite-links", payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.InviteLinks] });
      invalidateGettingStarted(queryClient);
    },
  });
};

export const useRevokeInviteLink = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, number>({
    mutationFn: (id) => API.request("POST", `/invite-links/${id}/revoke`),
    // Refetch on failure too: another admin may have revoked it, or it was
    // accepted meanwhile.
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.InviteLinks] });
      invalidateGettingStarted(queryClient);
    },
  });
};

// The public lookup behind the welcome page. It needs no account. A token
// that matches no invite is a 404, which the page shows as no longer valid,
// so it isn't retried. It waits for the server's status, since Demo Mode
// refuses the lookup and the page says so itself.
export const useInvite = (
  token: string | undefined,
  { enabled = true }: { enabled?: boolean } = {},
) => {
  const { demoMode, isLoading } = useAuth();
  return useQuery<InviteResponse, ShishoAPIError>({
    enabled: enabled && Boolean(token) && !isLoading && !demoMode,
    retry: false,
    queryKey: [QueryKey.Invite, token],
    queryFn: ({ signal }) =>
      API.request("GET", `/invites/${token}`, null, null, signal),
  });
};

// Creates the invited user and signs them in; the response is the new
// session's user.
export const useAcceptInvite = (token: string) =>
  useMutation<MeResponse, ShishoAPIError, AcceptInvitePayload>({
    mutationFn: (payload) =>
      API.request("POST", `/invites/${token}/accept`, payload),
  });
