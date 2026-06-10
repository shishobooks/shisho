import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

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
} from "@/types";

export enum QueryKey {
  RetrieveUser = "RetrieveUser",
  ListUsers = "ListUsers",
  ListRoles = "ListRoles",
}

export const useUser = (
  id?: string,
  options: Omit<
    UseQueryOptions<User, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<User, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(id),
    ...options,
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
    queryKey: [QueryKey.ListUsers, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/users", null, query, signal);
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
