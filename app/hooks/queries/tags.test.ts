import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryKey as BooksQueryKey } from "./books";
import { QueryKey, useMergeTag } from "./tags";

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

describe("useMergeTag", () => {
  it("invalidates TagBooks for the target id on success", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.TagBooks, 1, { limit: 50, offset: 0 }], {
      items: [],
      total: 0,
    });

    const { result } = renderHook(() => useMergeTag(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ targetId: 1, sourceId: 2 });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.TagBooks, 1, { limit: 50, offset: 0 }])
          ?.isInvalidated,
      ).toBe(true);
    });
  });

  it("invalidates RetrieveTag, ListTags, and book queries on success", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.RetrieveTag, 1], { id: 1 });
    client.setQueryData([QueryKey.ListTags, {}], { items: [], total: 0 });
    client.setQueryData([BooksQueryKey.ListBooks, {}], { items: [], total: 0 });
    client.setQueryData([BooksQueryKey.RetrieveBook, 10], { id: 10 });

    const { result } = renderHook(() => useMergeTag(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ targetId: 1, sourceId: 2 });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.RetrieveTag, 1])?.isInvalidated,
      ).toBe(true);
    });
    expect(client.getQueryState([QueryKey.ListTags, {}])?.isInvalidated).toBe(
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
