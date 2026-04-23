import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import { QueryKey, useUninstallPlugin, useUpdatePlugin } from "./plugins";

// Mock the API layer so the mutation completes without a real HTTP call.
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

describe("useUninstallPlugin", () => {
  // AdvancedOrderSection reads from [PluginOrder, <hookType>] and
  // LibraryPluginsTab reads from ["libraries", libraryId, "plugins", "order", hookType].
  // Both caches must be invalidated on uninstall so the removed plugin stops
  // appearing — otherwise the order lists keep showing a now-gone plugin.
  it("invalidates plugin order queries (global and library-scoped) on success", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    // Seed caches so we can detect invalidation via state.isInvalidated.
    client.setQueryData([QueryKey.PluginOrder, "metadataEnricher"], []);
    client.setQueryData(
      ["libraries", "lib-1", "plugins", "order", "metadataEnricher"],
      { customized: false, plugins: [] },
    );

    const { result } = renderHook(() => useUninstallPlugin(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ scope: "shisho", id: "test" });
    });

    await waitFor(() => {
      const globalOrder = client.getQueryState([
        QueryKey.PluginOrder,
        "metadataEnricher",
      ]);
      expect(globalOrder?.isInvalidated).toBe(true);
    });

    const libraryOrder = client.getQueryState([
      "libraries",
      "lib-1",
      "plugins",
      "order",
      "metadataEnricher",
    ]);
    expect(libraryOrder?.isInvalidated).toBe(true);
  });

  it("does not invalidate unrelated 'libraries'-prefixed queries", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    // Unrelated library-scoped query — e.g. a library's books. Blanket
    // invalidation on the "libraries" prefix would sweep this up too.
    client.setQueryData(["libraries", "lib-1", "books"], []);
    client.setQueryData(["libraries", "lib-1", "settings"], {});

    const { result } = renderHook(() => useUninstallPlugin(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current.mutateAsync({ scope: "shisho", id: "test" });
    });

    expect(
      client.getQueryState(["libraries", "lib-1", "books"])?.isInvalidated,
    ).toBe(false);
    expect(
      client.getQueryState(["libraries", "lib-1", "settings"])?.isInvalidated,
    ).toBe(false);
  });
});

describe("useUpdatePlugin", () => {
  // A failed enable still mutates server state (the plugin row gets
  // Malfunctioned + load_error persisted), so the detail page needs the
  // installed-plugins query refetched on error too — otherwise the error
  // alert doesn't appear until the user reloads.
  it("invalidates PluginsInstalled when the mutation fails", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData([QueryKey.PluginsInstalled], []);

    vi.mocked(API.request).mockRejectedValueOnce(new Error("422"));

    const { result } = renderHook(() => useUpdatePlugin(), {
      wrapper: makeWrapper(client),
    });

    await act(async () => {
      await result.current
        .mutateAsync({
          id: "test",
          payload: { enabled: true },
          scope: "shisho",
        })
        .catch(() => undefined);
    });

    await waitFor(() => {
      expect(
        client.getQueryState([QueryKey.PluginsInstalled])?.isInvalidated,
      ).toBe(true);
    });
  });
});
