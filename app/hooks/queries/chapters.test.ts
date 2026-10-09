import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { QueryKey, useUpdateFileChapters } from "./chapters";
import { QueryKey as EpubQueryKey } from "./epub";

vi.mock("@/libraries/api", async () => {
  const actual = await vi.importActual<object>("@/libraries/api");
  return {
    ...actual,
    API: {
      request: vi.fn().mockResolvedValue([]),
    },
  };
});

const makeWrapper = (client: QueryClient) => {
  const Wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
  return Wrapper;
};

describe("useUpdateFileChapters", () => {
  it("invalidates the file's chapters and the reader's EPUB on success", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData([QueryKey.FileChapters, 7], []);
    client.setQueryData([EpubQueryKey.EpubBlob, 7], new Blob(["epub"]));

    const { result } = renderHook(() => useUpdateFileChapters(7), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ chapters: [] });
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.FileChapters, 7])?.isInvalidated,
      ).toBe(true);
      // The generated EPUB carries the chapters as its table of contents.
      expect(
        client.getQueryState([EpubQueryKey.EpubBlob, 7])?.isInvalidated,
      ).toBe(true);
    });
  });
});
