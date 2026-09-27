import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  CreateShareLinkPayload,
  SharedBookResponse,
  ShareLinkResponse,
  SharingSettingsResponse,
  UpdateSharingSettingsPayload,
} from "@/types";

export enum QueryKey {
  SharingSettings = "SharingSettings",
  BookShareLinks = "BookShareLinks",
  SharedBook = "SharedBook",
}

// The endpoint allows Shares Read or Config Read. Callers without either
// pass `enabled: false` so the request is never made.
export const useSharingSettings = (
  options: Omit<
    UseQueryOptions<SharingSettingsResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) =>
  useQuery<SharingSettingsResponse, ShishoAPIError>({
    ...options,
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
export const useBookShareLinks = (
  bookId: number,
  options: Omit<
    UseQueryOptions<ShareLinkResponse[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) =>
  useQuery<ShareLinkResponse[], ShishoAPIError>({
    ...options,
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

// The book behind a Share Link, fetched anonymously by the recipient page.
// A 404 means the link is unavailable and is final. Anything else may be a
// blip, so retry it once before the page offers Try again.
export const useSharedBook = (token: string | undefined) =>
  useQuery<SharedBookResponse, ShishoAPIError>({
    enabled: Boolean(token),
    queryKey: [QueryKey.SharedBook, token],
    queryFn: ({ signal }) =>
      API.request("GET", `/share/${token}`, null, null, signal),
    retry: (failureCount, error) =>
      !(error instanceof ShishoAPIError && error.status === 404) &&
      failureCount < 1,
  });
