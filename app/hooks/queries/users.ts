import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { useAuth } from "@/hooks/useAuth";
import { API, ShishoAPIError } from "@/libraries/api";
import type {
  CreateRolePayload,
  CreateUserPayload,
  ListRolesResponse,
  ListUsersQuery,
  ListUsersResponse,
  ResetPasswordPayload,
  Role,
  UpdateRolePayload,
  UpdateUserPayload,
  User,
  UserDirectoryEntry,
} from "@/types";

import { QueryKey as LibrariesQueryKey } from "./libraries";
import { useRequires } from "./permissions";

export enum QueryKey {
  RetrieveUser = "RetrieveUser",
  ListUsers = "ListUsers",
  ListRoles = "ListRoles",
  UserDirectory = "UserDirectory",
}

export const useUser = (
  id?: string,
  options: Omit<
    UseQueryOptions<User, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<User, ShishoAPIError>({
    ...options,
    enabled: useRequires("users:read", options.enabled ?? Boolean(id)),
    queryKey: [QueryKey.RetrieveUser, id],
    queryFn: ({ signal }) => {
      return API.request("GET", `/users/${id}`, null, null, signal);
    },
  });
};

export const useUsers = (
  query: ListUsersQuery = {},
  options: Omit<
    UseQueryOptions<ListUsersResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListUsersResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("users:read", options.enabled ?? true),
    queryKey: [QueryKey.ListUsers, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/users", null, query, signal);
    },
  });
};

// Every active user's id and username, for picking whom to share a list with.
// Any signed-in role may read it, but the server does not register the route
// in Demo Mode, so the hook stays off there.
export const useUserDirectory = (
  options: Omit<
    UseQueryOptions<UserDirectoryEntry[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  const { demoMode } = useAuth();
  return useQuery<UserDirectoryEntry[], ShishoAPIError>({
    ...options,
    enabled: !demoMode && (options.enabled ?? true),
    queryKey: [QueryKey.UserDirectory],
    queryFn: ({ signal }) => {
      return API.request("GET", "/users/directory", null, null, signal);
    },
  });
};

export const useCreateUser = () => {
  const queryClient = useQueryClient();

  return useMutation<User, ShishoAPIError, CreateUserPayload>({
    mutationFn: (payload) => {
      return API.request("POST", "/users", payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListUsers] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.UserDirectory] });
    },
  });
};

interface UpdateUserVariables {
  id: string;
  payload: UpdateUserPayload;
}

export const useUpdateUser = () => {
  const queryClient = useQueryClient();

  return useMutation<User, ShishoAPIError, UpdateUserVariables>({
    mutationFn: ({ id, payload }) => {
      return API.request("POST", `/users/${id}`, payload);
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListUsers] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.UserDirectory] });
      // An admin may have changed their own library access.
      queryClient.invalidateQueries({
        queryKey: [LibrariesQueryKey.UserLibraries],
      });
      queryClient.setQueryData([QueryKey.RetrieveUser, String(data.id)], data);
    },
  });
};

interface ResetPasswordVariables {
  id: string;
  payload: ResetPasswordPayload;
}

export const useResetPassword = () => {
  return useMutation<void, ShishoAPIError, ResetPasswordVariables>({
    mutationFn: ({ id, payload }) => {
      return API.request("POST", `/users/${id}/reset-password`, payload);
    },
  });
};

export const useDeactivateUser = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, string>({
    mutationFn: (id) => {
      return API.request("DELETE", `/users/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListUsers] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.UserDirectory] });
    },
  });
};

// Roles

export const useRoles = (
  options: Omit<
    UseQueryOptions<ListRolesResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListRolesResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("users:read", options.enabled ?? true),
    queryKey: [QueryKey.ListRoles],
    queryFn: ({ signal }) => {
      return API.request("GET", "/roles", null, null, signal);
    },
  });
};

export const useCreateRole = () => {
  const queryClient = useQueryClient();

  return useMutation<Role, ShishoAPIError, CreateRolePayload>({
    mutationFn: (payload) => {
      return API.request("POST", "/roles", payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListRoles] });
    },
  });
};

interface UpdateRoleVariables {
  id: number;
  payload: UpdateRolePayload;
}

export const useUpdateRole = () => {
  const queryClient = useQueryClient();

  return useMutation<Role, ShishoAPIError, UpdateRoleVariables>({
    mutationFn: ({ id, payload }) => {
      return API.request("POST", `/roles/${id}`, payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListRoles] });
    },
  });
};

export const useDeleteRole = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, number>({
    mutationFn: (id) => {
      return API.request("DELETE", `/roles/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListRoles] });
    },
  });
};
