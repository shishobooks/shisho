import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryKey as BooksQueryKey } from "./books";
import { QueryKey, useMergeGenre } from "./genres";

vi.mock("@/libraries/api", async () => {
  const actual = await vi.importActual<object>("@/libraries/api");
  return {
    ...actual,
    API: {
      request: vi.fn().mockResolvedValue(undefined),
    },
  };
});

const makeWrapper = (client: QueryClient) => {
  const Wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
  return Wrapper;
};

describe("useMergeGenre", () => {
  it("invalidates GenreBooks for the target id on success", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.GenreBooks, 1, { limit: 50, offset: 0 }], {
      items: [],
      total: 0,
    });

    const { result } = renderHook(() => useMergeGenre(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ targetId: 1, sourceId: 2 });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.GenreBooks, 1, { limit: 50, offset: 0 }])
          ?.isInvalidated,
      ).toBe(true);
    });
  });

  it("invalidates RetrieveGenre, ListGenres, and book queries on success", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.RetrieveGenre, 1], { id: 1 });
    client.setQueryData([QueryKey.ListGenres, {}], { items: [], total: 0 });
    client.setQueryData([BooksQueryKey.ListBooks, {}], { items: [], total: 0 });
    client.setQueryData([BooksQueryKey.RetrieveBook, 10], { id: 10 });

    const { result } = renderHook(() => useMergeGenre(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ targetId: 1, sourceId: 2 });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.RetrieveGenre, 1])?.isInvalidated,
      ).toBe(true);
    });
    expect(client.getQueryState([QueryKey.ListGenres, {}])?.isInvalidated).toBe(
      true,
    );
    expect(
      client.getQueryState([BooksQueryKey.ListBooks, {}])?.isInvalidated,
    ).toBe(true);
    expect(
      client.getQueryState([BooksQueryKey.RetrieveBook, 10])?.isInvalidated,
    ).toBe(true);
  });
});
