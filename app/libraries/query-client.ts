import { ShishoAPIError } from "./api";
import { keepPreviousData, QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      placeholderData: keepPreviousData,
      // The default retry logic is to retry up to 3 times. We want to retry up to 3 times as well, but if it's a 401,
      // 404, or 422, we don't want that to retry since that's probably a real error, not a transient one.
      retry: (failureCount: number, err: unknown) => {
        if (err instanceof ShishoAPIError) {
          return (
            failureCount < 3 &&
            err.status !== 401 &&
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
