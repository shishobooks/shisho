import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, isNotFoundError, ShishoAPIError } from "@/libraries/api";
import type {
  CreateShareLinkPayload,
  SharedBookResponse,
  ShareLinkResponse,
  SharingSettingsResponse,
  UpdateSharingSettingsPayload,
} from "@/types";

import { anyOf, useRequires } from "./permissions";

export enum QueryKey {
  SharingSettings = "SharingSettings",
  BookShareLinks = "BookShareLinks",
  SharedBook = "SharedBook",
}

// The endpoint allows either Shares permission or Config Read.
export const useSharingSettings = (
  options: Omit<
    UseQueryOptions<SharingSettingsResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) =>
  useQuery<SharingSettingsResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires(
      anyOf("shares:read", "shares:write", "config:read"),
      options.enabled ?? true,
    ),
    queryKey: [QueryKey.SharingSettings],
    queryFn: ({ signal }) =>
      API.request("GET", "/settings/sharing", null, null, signal),
  });

// Partial update: callers send only the switch that changed.
export const useUpdateSharingSettings = () => {
  const queryClient = useQueryClient();
  return useMutation<
    SharingSettingsResponse,
    ShishoAPIError,
    UpdateSharingSettingsPayload
  >({
    mutationFn: (payload) =>
      API.request("PUT", "/settings/sharing", payload, null),
    onSuccess: (data) => {
      queryClient.setQueryData([QueryKey.SharingSettings], data);
    },
  });
};

// Every Share Link on a book, from every creator. Requires Shares Read.
// Recipients change the counts without the sharer doing anything, so the list
// is always stale: reopening the dialog (which flips `enabled`) or returning
// to the window refetches it instead of showing the global Infinity cache.
export const useBookShareLinks = (
  bookId: number,
  options: Omit<
    UseQueryOptions<ShareLinkResponse[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) =>
  useQuery<ShareLinkResponse[], ShishoAPIError>({
    staleTime: 0,
    ...options,
    enabled: useRequires(
      anyOf("shares:read", "shares:write"),
      options.enabled ?? true,
    ),
    queryKey: [QueryKey.BookShareLinks, bookId],
    queryFn: ({ signal }) =>
      API.request("GET", `/books/${bookId}/share-links`, null, null, signal),
  });

export const useCreateShareLink = () => {
  const queryClient = useQueryClient();
  return useMutation<
    ShareLinkResponse,
    ShishoAPIError,
    { bookId: number; payload: CreateShareLinkPayload }
  >({
    mutationFn: ({ bookId, payload }) =>
      API.request("POST", `/books/${bookId}/share-links`, payload),
    onSuccess: (_data, { bookId }) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.BookShareLinks, bookId],
      });
    },
  });
};

// Revocation is permanent: the link stays listed as revoked with its counts.
export const useRevokeShareLink = () => {
  const queryClient = useQueryClient();
  return useMutation<
    ShareLinkResponse,
    ShishoAPIError,
    { bookId: number; linkId: number }
  >({
    mutationFn: ({ bookId, linkId }) =>
      API.request("POST", `/books/${bookId}/share-links/${linkId}/revoke`),
    // Refetch on failure too: a 404 means another sharer deleted the link.
    onSettled: (_data, _error, { bookId }) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.BookShareLinks, bookId],
      });
    },
  });
};

// Removes a link in any state, active included.
export const useDeleteShareLink = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, { bookId: number; linkId: number }>({
    mutationFn: ({ bookId, linkId }) =>
      API.request("DELETE", `/books/${bookId}/share-links/${linkId}`),
    onSettled: (_data, _error, { bookId }) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.BookShareLinks, bookId],
      });
    },
  });
};

// The book behind a Share Link, fetched anonymously by the recipient page.
// A 404 means the link is unavailable and is final. Anything else may be a
// blip, so retry it once before the page offers Try again.
//
// Each fetch counts as an open, so the query deliberately ignores the abort
// signal. With it, the StrictMode remount in development aborts a request the
// server already counted and sends a second one; without it, the remount
// reuses the request in flight.
export const useSharedBook = (token: string | undefined) =>
  useQuery<SharedBookResponse, ShishoAPIError>({
    enabled: Boolean(token),
    queryKey: [QueryKey.SharedBook, token],
    queryFn: () => API.request("GET", `/share/${token}`),
    retry: (failureCount, error) => !isNotFoundError(error) && failureCount < 1,
  });
