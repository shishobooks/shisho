import {
  useMutation,
  useQuery,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";

export enum QueryKey {
  AuthStatus = "AuthStatus",
  AuthMe = "AuthMe",
}

interface AuthStatusResponse {
  needs_setup: boolean;
}

export const useAuthStatus = (
  options: Omit<
    UseQueryOptions<AuthStatusResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<AuthStatusResponse, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.AuthStatus],
    queryFn: ({ signal }) => {
      return API.request("GET", "/auth/status", null, null, signal);
    },
  });
};

interface LoginPayload {
  username: string;
  password: string;
}

interface LoginResponse {
  id: number;
  username: string;
  email?: string;
  role_id: number;
  role_name: string;
  permissions: string[];
}

export const useLogin = () => {
  return useMutation<LoginResponse, ShishoAPIError, LoginPayload>({
    mutationFn: (payload) => {
      return API.request("POST", "/auth/login", payload);
    },
  });
};

export const useLogout = () => {
  return useMutation<void, ShishoAPIError, void>({
    mutationFn: () => {
      return API.request("POST", "/auth/logout");
    },
  });
};
