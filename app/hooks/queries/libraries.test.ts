import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import { useLibraries } from "./libraries";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const LIBRARIES = Array.from({ length: 230 }, (_, i) => ({
  id: i + 1,
  name: `Library ${i + 1}`,
}));

describe("useLibraries", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["libraries:read"] });
  });

  // The server pages GET /libraries, and the pages that use this (the
  // Libraries list, and the access pickers on Add User and a user's page)
  // need every one of them.
  it("loads every Library, page by page", async () => {
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, _path, _body, query) => {
        const { limit, offset } = query as { limit: number; offset: number };
        return {
          items: LIBRARIES.slice(offset, offset + limit),
          total: LIBRARIES.length,
        };
      });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { result } = renderHook(() => useLibraries(), {
      wrapper: ({ children }: { children: React.ReactNode }) =>
        React.createElement(QueryClientProvider, { client }, children),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.items).toEqual(LIBRARIES);
    expect(result.current.data?.total).toBe(230);
    expect(request).toHaveBeenCalledTimes(3);
    expect(request).toHaveBeenLastCalledWith(
      "GET",
      "/libraries",
      null,
      { limit: 100, offset: 200 },
      expect.anything(),
    );
  });
});
