import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { useCreateDevice } from "./devices";
import { QueryKey } from "./gettingStarted";
import { useCreateLibrary } from "./libraries";
import { useCopyListToToRead } from "./lists";
import { useInstallPlugin, useUninstallPlugin } from "./plugins";
import { useSetOnToRead } from "./reading";
import { useCreateUser, useDeactivateUser, useUpdateUser } from "./users";

vi.mock("@/libraries/api", async () => {
  const actual = await vi.importActual<object>("@/libraries/api");
  return {
    ...actual,
    API: { request: vi.fn().mockResolvedValue({ id: 1, book_id: 1 }) },
  };
});

// A step ticks itself off as soon as what it asks for exists, so every
// mutation that can create or remove that thing refetches the list.
const MUTATIONS: Record<
  string,
  { hook: () => { mutateAsync: (v: never) => Promise<unknown> }; vars: unknown }
> = {
  useCreateLibrary: { hook: useCreateLibrary, vars: { payload: {} } },
  useInstallPlugin: { hook: useInstallPlugin, vars: { scope: "s", id: "p" } },
  useUninstallPlugin: {
    hook: useUninstallPlugin,
    vars: { scope: "s", id: "p" },
  },
  useCreateUser: { hook: useCreateUser, vars: {} },
  useUpdateUser: { hook: useUpdateUser, vars: { id: "1", payload: {} } },
  useDeactivateUser: { hook: useDeactivateUser, vars: "1" },
  useCreateDevice: { hook: useCreateDevice, vars: {} },
  useSetOnToRead: { hook: useSetOnToRead, vars: { bookId: 1, onToRead: true } },
  useCopyListToToRead: { hook: useCopyListToToRead, vars: { listId: 1 } },
};

describe("Getting Started invalidation", () => {
  it.each(Object.entries(MUTATIONS))(
    "%s refetches the Getting Started list",
    async (_, { hook, vars }) => {
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      client.setQueryData([QueryKey.GettingStarted], { steps: [] });
      const { result } = renderHook(hook, {
        wrapper: ({ children }: { children: React.ReactNode }) =>
          React.createElement(QueryClientProvider, { client }, children),
      });

      await act(async () => {
        await result.current.mutateAsync(vars as never);
      });

      expect(
        client.getQueryState([QueryKey.GettingStarted])?.isInvalidated,
      ).toBe(true);
    },
  );
});
