import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import { QueryKey as BooksQueryKey } from "./books";
import {
  QueryKey,
  useDeletePublisher,
  useMergePublisher,
  useSetChildPublisher,
  useUpdatePublisher,
} from "./publishers";

vi.mock("@/libraries/api", async () => {
  const actual = await vi.importActual<object>("@/libraries/api");
  return {
    ...actual,
    API: {
      // The publisher the request names, for the mutations that return one.
      request: vi.fn((_method: string, path: string) =>
        Promise.resolve({ id: Number(path.split("/")[2]) }),
      ),
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

  it("invalidates RetrievePublisher and PublisherFiles for non-target publishers on success", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.RetrievePublisher, 3], {
      id: 3,
      children: [{ id: 2, name: "Source", file_count: 5 }],
      descendant_ids: [2],
      descendant_file_count: 5,
    });
    client.setQueryData(
      [QueryKey.PublisherFiles, 3, { limit: 50, offset: 0 }],
      { items: [], total: 5 },
    );

    const { result } = renderHook(() => useMergePublisher(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ targetId: 1, sourceId: 2 });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.RetrievePublisher, 3])?.isInvalidated,
      ).toBe(true);
      expect(
        client.getQueryState([
          QueryKey.PublisherFiles,
          3,
          { limit: 50, offset: 0 },
        ])?.isInvalidated,
      ).toBe(true);
    });
  });
});

describe("useUpdatePublisher", () => {
  it("does not invalidate parent publisher detail queries when parent_id is unchanged", async () => {
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
        payload: { name: "Updated name", parent_id: undefined },
      });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.RetrievePublisher, 1])?.isInvalidated,
      ).toBe(true);
    });
    expect(
      client.getQueryState([QueryKey.RetrievePublisher, 2])?.isInvalidated,
    ).not.toBe(true);
  });

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

  it("invalidates the previous and new parent publisher file queries when parent_id changes", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.RetrievePublisher, 1], {
      id: 1,
      parent_id: 2,
    });
    client.setQueryData(
      [QueryKey.PublisherFiles, 2, { limit: 50, offset: 0 }],
      { items: [], total: 1 },
    );
    client.setQueryData(
      [QueryKey.PublisherFiles, 3, { limit: 50, offset: 0 }],
      { items: [], total: 0 },
    );

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
        client.getQueryState([
          QueryKey.PublisherFiles,
          2,
          { limit: 50, offset: 0 },
        ])?.isInvalidated,
      ).toBe(true);
      expect(
        client.getQueryState([
          QueryKey.PublisherFiles,
          3,
          { limit: 50, offset: 0 },
        ])?.isInvalidated,
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

  // A fetch of the publisher a rename merged into, begun before the save,
  // holds what it was before the merge.
  it("isn't overwritten by a fetch that began before the save", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    let land!: (publisher: unknown) => void;
    const stale = client
      .fetchQuery({
        queryKey: [QueryKey.RetrievePublisher, 3],
        queryFn: () => new Promise((resolve) => (land = resolve)),
      })
      .catch(() => {});
    vi.mocked(API.request).mockResolvedValueOnce({
      id: 3,
      name: "Tor",
      aliases: ["Tor Books"],
    });
    const { result } = renderHook(() => useUpdatePublisher(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({
        publisherId: 7,
        payload: { name: "Tor" },
      });
    });
    land({ id: 3, name: "Tor", aliases: [] });
    await stale;

    expect(
      client.getQueryData<{ aliases: string[] }>([
        QueryKey.RetrievePublisher,
        3,
      ])?.aliases,
    ).toEqual(["Tor Books"]);
  });

  // Renaming a publisher to another's name merges it into that one, and the
  // response is the publisher it merged into. Its edit dialog fills from the
  // cache when reopened, so the cache holds the response at once.
  it("holds the publisher the save returns, under its own id", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    vi.mocked(API.request).mockResolvedValueOnce({
      id: 3,
      name: "Tor",
      aliases: ["Tor Books"],
    });

    const { result } = renderHook(() => useUpdatePublisher(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({
        publisherId: 7,
        payload: { name: "Tor" },
      });
    });

    expect(
      client.getQueryData<{ aliases: string[] }>([
        QueryKey.RetrievePublisher,
        3,
      ])?.aliases,
    ).toEqual(["Tor Books"]);
  });
});

describe("useDeletePublisher", () => {
  it("invalidates RetrievePublisher and PublisherFiles query families on success", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.RetrievePublisher, 2], {
      id: 2,
      children: [{ id: 1, name: "Child", file_count: 3 }],
      descendant_ids: [1],
      descendant_file_count: 3,
    });
    client.setQueryData(
      [QueryKey.PublisherFiles, 2, { limit: 50, offset: 0 }],
      { items: [], total: 3 },
    );

    const { result } = renderHook(() => useDeletePublisher(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ publisherId: 1 });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.RetrievePublisher, 2])?.isInvalidated,
      ).toBe(true);
      expect(
        client.getQueryState([
          QueryKey.PublisherFiles,
          2,
          { limit: 50, offset: 0 },
        ])?.isInvalidated,
      ).toBe(true);
    });
  });
});

describe("useSetChildPublisher", () => {
  it("invalidates the previous and new parent detail and file queries without a cached child detail query", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    client.setQueryData([QueryKey.RetrievePublisher, 1], {
      id: 1,
      children: [],
    });
    client.setQueryData([QueryKey.RetrievePublisher, 2], {
      id: 2,
      children: [{ id: 3, name: "Child", file_count: 0 }],
    });
    client.setQueryData([QueryKey.ListPublishers, { library_id: 10 }], {
      items: [
        {
          id: 1,
          name: "New Parent",
          library_id: 10,
          aliases: [],
          file_count: 0,
          descendant_file_count: 1,
          descendant_publisher_count: 1,
          parent_id: null,
          parent_name: null,
        },
        {
          id: 2,
          name: "Old Parent",
          library_id: 10,
          aliases: [],
          file_count: 0,
          descendant_file_count: 1,
          descendant_publisher_count: 1,
          parent_id: null,
          parent_name: null,
        },
        {
          id: 3,
          name: "Child",
          library_id: 10,
          aliases: [],
          file_count: 1,
          descendant_file_count: 0,
          descendant_publisher_count: 0,
          parent_id: 2,
          parent_name: "Old Parent",
        },
      ],
      total: 3,
    });
    client.setQueryData(
      [QueryKey.PublisherFiles, 1, { limit: 50, offset: 0 }],
      { items: [], total: 0 },
    );
    client.setQueryData(
      [QueryKey.PublisherFiles, 2, { limit: 50, offset: 0 }],
      { items: [], total: 1 },
    );

    const { result } = renderHook(() => useSetChildPublisher(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({
        parentId: 1,
        childId: 3,
      });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.RetrievePublisher, 1])?.isInvalidated,
      ).toBe(true);
      expect(
        client.getQueryState([QueryKey.RetrievePublisher, 2])?.isInvalidated,
      ).toBe(true);
      expect(
        client.getQueryState([
          QueryKey.PublisherFiles,
          1,
          { limit: 50, offset: 0 },
        ])?.isInvalidated,
      ).toBe(true);
      expect(
        client.getQueryState([
          QueryKey.PublisherFiles,
          2,
          { limit: 50, offset: 0 },
        ])?.isInvalidated,
      ).toBe(true);
    });
  });
});
