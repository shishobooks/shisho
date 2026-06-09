import {
  useMutation,
  useQuery,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { LoginPayload, MeResponse, StatusResponse } from "@/types";

export enum QueryKey {
  AuthStatus = "AuthStatus",
  AuthMe = "AuthMe",
}

export const useAuthStatus = (
  options: Omit<
    UseQueryOptions<StatusResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<StatusResponse, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.AuthStatus],
    queryFn: ({ signal }) => {
      return API.request("GET", "/auth/status", null, null, signal);
    },
  });
};

export const useLogin = () => {
  return useMutation<MeResponse, ShishoAPIError, LoginPayload>({
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
