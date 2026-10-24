import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  Book,
  BookDevice,
  DeliveryPreviewQuery,
  DeliveryPreviewResponse,
  Rule,
} from "@/types";
import type {
  AdminDeviceResponse,
  CreateDevicePayload,
  DeliveryRulesMode,
  DeviceResponse,
  DeviceShortURL,
  UpdateDevicePayload,
} from "@/types/generated/devices";

import { useRequires } from "./permissions";

export enum QueryKey {
  ListDevices = "ListDevices",
  RetrieveDevice = "RetrieveDevice",
  ListAllDevices = "ListAllDevices",
  DeliveryPreview = "DeliveryPreview",
  BookDevices = "BookDevices",
}

// The caller's own Devices. Any signed-in user manages their own, so the
// routes need no permission.
export const useDevices = () => {
  return useQuery<DeviceResponse[], ShishoAPIError>({
    queryKey: [QueryKey.ListDevices],
    queryFn: ({ signal }) =>
      API.request("GET", "/user/devices", null, null, signal),
  });
};

// Fetches the caller's Devices outside a component, such as when a save
// needs them, from the cache when they are loaded.
export const fetchDevices = (queryClient: QueryClient) =>
  queryClient.fetchQuery<DeviceResponse[], ShishoAPIError>({
    queryKey: [QueryKey.ListDevices],
    queryFn: ({ signal }) =>
      API.request("GET", "/user/devices", null, null, signal),
  });

export const useDevice = (id?: string) => {
  return useQuery<DeviceResponse, ShishoAPIError>({
    enabled: Boolean(id),
    queryKey: [QueryKey.RetrieveDevice, id],
    queryFn: ({ signal }) =>
      API.request("GET", `/user/devices/${id}`, null, null, signal),
  });
};

// Every user's Devices, for admins. Never carries a credential.
export const useAllDevices = () => {
  return useQuery<AdminDeviceResponse[], ShishoAPIError>({
    enabled: useRequires("users:read", true),
    queryKey: [QueryKey.ListAllDevices],
    queryFn: ({ signal }) => API.request("GET", "/devices", null, null, signal),
  });
};

const useInvalidateDevices = () => {
  const queryClient = useQueryClient();
  return () => {
    queryClient.invalidateQueries({ queryKey: [QueryKey.ListDevices] });
    queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveDevice] });
    queryClient.invalidateQueries({ queryKey: [QueryKey.ListAllDevices] });
    queryClient.invalidateQueries({ queryKey: [QueryKey.DeliveryPreview] });
    queryClient.invalidateQueries({ queryKey: [QueryKey.BookDevices] });
  };
};

export const useCreateDevice = () => {
  const invalidate = useInvalidateDevices();
  return useMutation<DeviceResponse, ShishoAPIError, CreateDevicePayload>({
    mutationFn: (payload) => API.request("POST", "/user/devices", payload),
    onSuccess: invalidate,
  });
};

export const useUpdateDevice = () => {
  const queryClient = useQueryClient();
  const invalidate = useInvalidateDevices();
  return useMutation<
    DeviceResponse,
    ShishoAPIError,
    { id: string; payload: UpdateDevicePayload }
  >({
    mutationFn: ({ id, payload }) =>
      API.request("PATCH", `/user/devices/${id}`, payload),
    // The saved Device shows at once, so an editor reset to it never reads
    // the stale one while the refetch runs.
    onSuccess: (device) => {
      queryClient.setQueryData([QueryKey.RetrieveDevice, device.id], device);
      invalidate();
    },
  });
};

export const useRegenerateDeviceCredential = () => {
  const invalidate = useInvalidateDevices();
  return useMutation<DeviceResponse, ShishoAPIError, string>({
    mutationFn: (id) => API.request("POST", `/user/devices/${id}/credential`),
    onSuccess: invalidate,
  });
};

export const useDeleteDevice = () => {
  const invalidate = useInvalidateDevices();
  return useMutation<void, ShishoAPIError, string>({
    mutationFn: (id) => API.request("DELETE", `/user/devices/${id}`),
    onSuccess: invalidate,
  });
};

export const useRevokeDevice = () => {
  const invalidate = useInvalidateDevices();
  return useMutation<void, ShishoAPIError, string>({
    mutationFn: (id) => API.request("DELETE", `/devices/${id}`),
    onSuccess: invalidate,
  });
};

export const useGenerateDeviceShortUrl = () => {
  return useMutation<DeviceShortURL, ShishoAPIError, string>({
    mutationFn: (id) => API.request("POST", `/user/devices/${id}/short-url`),
  });
};

export const useClearKoboSync = () => {
  return useMutation<void, ShishoAPIError, string>({
    mutationFn: (id) => API.request("DELETE", `/user/devices/${id}/kobo-sync`),
  });
};

/**
 * A change to a Device's Delivery Rules to preview before saving it: a mode
 * to follow, with the custom list when the mode is custom.
 */
export interface RulesProposal {
  mode: DeliveryRulesMode;
  rules?: Rule[];
}

const previewQuery = (
  proposal: RulesProposal | undefined,
): DeliveryPreviewQuery => {
  if (!proposal) return {};
  return {
    mode: proposal.mode,
    rules: proposal.rules
      ?.map((r) => `${r.source_file_type}:${r.delivered_file_type}`)
      .join(","),
  };
};

const deliveryPreviewOptions = (
  id: string,
  proposal: RulesProposal | undefined,
) => {
  const query = previewQuery(proposal);
  return {
    queryKey: [QueryKey.DeliveryPreview, id, query],
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      API.request<DeliveryPreviewResponse>(
        "GET",
        `/user/devices/${id}/delivery-preview`,
        null,
        query,
        signal,
      ),
  };
};

// What a Device receives under the rules it follows, or under a proposed
// change. It depends on the library and on what a Kobo last synced, which
// change without any mutation here, so it is always refetched when shown.
export const useDeliveryPreview = (id: string, proposal?: RulesProposal) => {
  return useQuery<DeliveryPreviewResponse, ShishoAPIError>({
    ...deliveryPreviewOptions(id, proposal),
    staleTime: 0,
    // Editing a custom list changes the key on every move; keep showing the
    // last counts until the new ones arrive.
    placeholderData: keepPreviousData,
  });
};

// Fetches the preview of a proposed change, for a warning before saving it.
export const fetchDeliveryPreview = (
  queryClient: QueryClient,
  id: string,
  proposal: RulesProposal,
) =>
  queryClient.fetchQuery<DeliveryPreviewResponse, ShishoAPIError>({
    ...deliveryPreviewOptions(id, proposal),
    staleTime: 0,
  });

// What each of the caller's Devices gets of a book. Like the preview, it
// depends on what a Kobo last synced, so it is refetched when shown. What a
// Device gets follows the book's Files, so the key describes them: a File
// added, removed, or moved, from the book page or a scan, refetches it.
export const useBookDevices = (
  book: Pick<Book, "id" | "files">,
  options: { enabled?: boolean } = {},
) => {
  const filesKey = (book.files ?? [])
    .map((f) => `${f.id}:${f.file_type}:${f.file_role}`)
    .join(",");
  return useQuery<BookDevice[], ShishoAPIError>({
    enabled: useRequires("books:read", options.enabled ?? true),
    staleTime: 0,
    queryKey: [QueryKey.BookDevices, book.id, filesKey],
    queryFn: ({ signal }) =>
      API.request("GET", `/books/${book.id}/devices`, null, null, signal),
  });
};
