import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  AdminDeviceResponse,
  CreateDevicePayload,
  DeviceResponse,
  DeviceShortURL,
  UpdateDevicePayload,
} from "@/types/generated/devices";

import { useRequires } from "./permissions";

export enum QueryKey {
  ListDevices = "ListDevices",
  RetrieveDevice = "RetrieveDevice",
  ListAllDevices = "ListAllDevices",
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
