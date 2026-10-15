import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import { QueryKey as BooksQueryKey } from "./books";
import { useSeriesBooks } from "./series";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

describe("useSeriesBooks", () => {
  // A series page can sort by any book field, so every change that refreshes
  // the book gallery has to refresh it too.
  it("refetches when book lists are invalidated", async () => {
    setAuth({ permissions: ["books:read", "series:read"] });
    const request = vi
      .spyOn(API, "request")
      .mockResolvedValue({ items: [], total: 0 });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { result } = renderHook(() => useSeriesBooks(5, { limit: 24 }), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const seriesCalls = () =>
      request.mock.calls.filter((call) => call[1] === "/series/5/books").length;
    expect(seriesCalls()).toBe(1);

    await client.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });

    await waitFor(() => expect(seriesCalls()).toBe(2));
    request.mockRestore();
  });
});
