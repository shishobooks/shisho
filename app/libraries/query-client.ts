import { keepPreviousData, QueryClient } from "@tanstack/react-query";

import { ShishoAPIError } from "./api";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      placeholderData: keepPreviousData,
      // Retry transient failures up to three times, not authentication,
      // permission, missing-resource, or validation errors.
      retry: (failureCount: number, err: unknown) => {
        if (err instanceof ShishoAPIError) {
          return (
            failureCount < 3 &&
            err.status !== 401 &&
            err.status !== 403 &&
            err.status !== 404 &&
            err.status !== 422
          );
        }
        return failureCount < 3;
      },
      refetchOnMount: "always",
      staleTime: Infinity,
    },
  },
});
