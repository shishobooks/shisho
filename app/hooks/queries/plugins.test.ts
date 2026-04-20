import { QueryKey, useUninstallPlugin } from "./plugins";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

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
});
