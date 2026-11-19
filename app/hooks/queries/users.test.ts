import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import { useUsers } from "./users";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const USERS = Array.from({ length: 130 }, (_, i) => ({
  id: i + 1,
  username: `user${i + 1}`,
}));

describe("useUsers", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["users:read"] });
  });

  // GET /users is paged, and the Users page lists every account.
  it("loads every user, page by page", async () => {
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, _path, _body, query) => {
        const { limit, offset } = query as { limit: number; offset: number };
        return {
          items: USERS.slice(offset, offset + limit),
          total: USERS.length,
        };
      });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { result } = renderHook(() => useUsers(), {
      wrapper: ({ children }: { children: React.ReactNode }) =>
        React.createElement(QueryClientProvider, { client }, children),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.items).toEqual(USERS);
    expect(result.current.data?.total).toBe(130);
    expect(request).toHaveBeenCalledTimes(2);
    expect(request).toHaveBeenLastCalledWith(
      "GET",
      "/users",
      null,
      { limit: 100, offset: 100 },
      expect.anything(),
    );
  });
});
