import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { PermissionInput, Role, User } from "@/types";

export enum QueryKey {
  RetrieveUser = "RetrieveUser",
  ListUsers = "ListUsers",
  RetrieveRole = "RetrieveRole",
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

interface ListUsersQuery {
  limit?: number;
  offset?: number;
}

interface ListUsersData {
  users: User[];
  total: number;
}

export const useUsers = (
  query: ListUsersQuery = {},
  options: Omit<
    UseQueryOptions<ListUsersData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListUsersData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListUsers, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/users", null, query, signal);
    },
  });
};

interface CreateUserPayload {
  username: string;
  email?: string;
  password: string;
  role_id: number;
  library_ids?: number[];
  all_library_access?: boolean;
}

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

interface UpdateUserPayload {
  username?: string;
  email?: string;
  role_id?: number;
  is_active?: boolean;
  library_ids?: number[];
  all_library_access?: boolean;
}

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

interface ResetPasswordPayload {
  current_password?: string;
  new_password: string;
}

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

interface ListRolesData {
  roles: Role[];
  total: number;
}

export const useRoles = (
  options: Omit<
    UseQueryOptions<ListRolesData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListRolesData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListRoles],
    queryFn: ({ signal }) => {
      return API.request("GET", "/roles", null, null, signal);
    },
  });
};

export const useRole = (
  id?: number,
  options: Omit<
    UseQueryOptions<Role, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Role, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(id),
    ...options,
    queryKey: [QueryKey.RetrieveRole, id],
    queryFn: ({ signal }) => {
      return API.request("GET", `/roles/${id}`, null, null, signal);
    },
  });
};

interface CreateRolePayload {
  name: string;
  permissions: PermissionInput[];
}

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

interface UpdateRolePayload {
  name?: string;
  permissions?: PermissionInput[];
}

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
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListRoles] });
      queryClient.setQueryData([QueryKey.RetrieveRole, data.id], data);
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
