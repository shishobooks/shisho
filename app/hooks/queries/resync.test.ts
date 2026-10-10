import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryKey as ReflowableQueryKey } from "./reflowable";
import { useResyncBook, useResyncFile } from "./resync";

vi.mock("@/libraries/api", async () => {
  const actual = await vi.importActual<object>("@/libraries/api");
  return {
    ...actual,
    API: {
      request: vi.fn().mockResolvedValue({ id: 7, book_id: 3 }),
    },
  };
});

const makeWrapper = (client: QueryClient) => {
  const Wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
  return Wrapper;
};

const newClient = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(
    [ReflowableQueryKey.ReflowableBlob, 7],
    new Blob(["epub"]),
  );
  return client;
};

// A rescan can replace chapters and metadata, which the reader's book
// (the generated download) carries.
describe("resync", () => {
  it("useResyncFile invalidates the file's reader book", async () => {
    const client = newClient();
    const { result } = renderHook(() => useResyncFile(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ fileId: 7, payload: {} });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([ReflowableQueryKey.ReflowableBlob, 7])
          ?.isInvalidated,
      ).toBe(true);
    });
  });

  it("useResyncBook invalidates reader books", async () => {
    const client = newClient();
    const { result } = renderHook(() => useResyncBook(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ bookId: 3, payload: {} });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([ReflowableQueryKey.ReflowableBlob, 7])
          ?.isInvalidated,
      ).toBe(true);
    });
  });
});
