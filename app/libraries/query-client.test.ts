import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "./api";
import { queryClient } from "./query-client";

describe("query retry policy", () => {
  it.each([401, 403, 404, 422])("does not retry HTTP %s", async (status) => {
    const client = new QueryClient({
      defaultOptions: queryClient.getDefaultOptions(),
    });
    const error = new ShishoAPIError("Request rejected", "rejected", status);
    const queryFn = vi.fn().mockRejectedValue(error);
    try {
      await expect(
        client.fetchQuery({ queryKey: ["rejected"], queryFn, retryDelay: 0 }),
      ).rejects.toBe(error);
      expect(queryFn).toHaveBeenCalledTimes(1);
    } finally {
      client.clear();
    }
  });

  it.each([
    new ShishoAPIError("Server unavailable", "internal_server_error", 500),
    new TypeError("Failed to fetch"),
  ])(
    "still retries transient failures up to three times: $message",
    async (error) => {
      const client = new QueryClient({
        defaultOptions: queryClient.getDefaultOptions(),
      });
      const queryFn = vi.fn().mockRejectedValue(error);
      try {
        await expect(
          client.fetchQuery({
            queryKey: ["transient"],
            queryFn,
            retryDelay: 0,
          }),
        ).rejects.toBe(error);
        expect(queryFn).toHaveBeenCalledTimes(4);
      } finally {
        client.clear();
      }
    },
  );
});
