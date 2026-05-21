import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryKey as BooksQueryKey } from "./books";
import { QueryKey, useMergePublisher, useUpdatePublisher } from "./publishers";

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

describe("useMergePublisher", () => {
  it("invalidates PublisherFiles for the target id on success", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    // Seed the detail-page file list cache for the target publisher.
    client.setQueryData(
      [QueryKey.PublisherFiles, 1, { limit: 50, offset: 0 }],
      { items: [], total: 0 },
    );

    const { result } = renderHook(() => useMergePublisher(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ targetId: 1, sourceId: 2 });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([
          QueryKey.PublisherFiles,
          1,
          { limit: 50, offset: 0 },
        ])?.isInvalidated,
      ).toBe(true);
    });
  });

  it("invalidates RetrievePublisher, ListPublishers, and book queries on success", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.RetrievePublisher, 1], { id: 1 });
    client.setQueryData([QueryKey.ListPublishers, {}], { items: [], total: 0 });
    client.setQueryData([BooksQueryKey.ListBooks, {}], { items: [], total: 0 });
    client.setQueryData([BooksQueryKey.RetrieveBook, 10], { id: 10 });

    const { result } = renderHook(() => useMergePublisher(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ targetId: 1, sourceId: 2 });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.RetrievePublisher, 1])?.isInvalidated,
      ).toBe(true);
    });
    expect(
      client.getQueryState([QueryKey.ListPublishers, {}])?.isInvalidated,
    ).toBe(true);
    expect(
      client.getQueryState([BooksQueryKey.ListBooks, {}])?.isInvalidated,
    ).toBe(true);
    expect(
      client.getQueryState([BooksQueryKey.RetrieveBook, 10])?.isInvalidated,
    ).toBe(true);
  });
});

describe("useUpdatePublisher", () => {
  it("invalidates the new parent publisher detail query when parent_id is set", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.RetrievePublisher, 1], {
      id: 1,
      parent_id: null,
    });
    client.setQueryData([QueryKey.RetrievePublisher, 2], {
      id: 2,
      children: [],
    });

    const { result } = renderHook(() => useUpdatePublisher(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({
        publisherId: 1,
        payload: { parent_id: 2 },
      });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.RetrievePublisher, 1])?.isInvalidated,
      ).toBe(true);
      expect(
        client.getQueryState([QueryKey.RetrievePublisher, 2])?.isInvalidated,
      ).toBe(true);
    });
  });

  it("invalidates the previous parent publisher detail query when parent_id changes", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.RetrievePublisher, 1], {
      id: 1,
      parent_id: 2,
    });
    client.setQueryData([QueryKey.RetrievePublisher, 2], {
      id: 2,
      children: [{ id: 1, name: "Child", file_count: 0 }],
    });
    client.setQueryData([QueryKey.RetrievePublisher, 3], {
      id: 3,
      children: [],
    });

    const { result } = renderHook(() => useUpdatePublisher(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({
        publisherId: 1,
        payload: { parent_id: 3 },
      });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.RetrievePublisher, 2])?.isInvalidated,
      ).toBe(true);
    });
  });

  it("invalidates the previous parent publisher detail query when parent_id is cleared", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.RetrievePublisher, 1], {
      id: 1,
      parent_id: 2,
    });
    client.setQueryData([QueryKey.RetrievePublisher, 2], {
      id: 2,
      children: [{ id: 1, name: "Child", file_count: 0 }],
    });

    const { result } = renderHook(() => useUpdatePublisher(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({
        publisherId: 1,
        payload: { parent_id: null },
      });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.RetrievePublisher, 2])?.isInvalidated,
      ).toBe(true);
    });
  });
});
