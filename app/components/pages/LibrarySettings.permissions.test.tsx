import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import LibrarySettings from "./LibrarySettings";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/hooks/useUnsavedChanges", () => ({
  useUnsavedChanges: () => ({
    cancelNavigation: vi.fn(),
    proceedNavigation: vi.fn(),
    showBlockerDialog: false,
  }),
}));

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const renderSettings = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/libraries/1/settings"]}>
        <Routes>
          <Route
            element={<LibrarySettings />}
            path="/libraries/:libraryId/settings"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 50));
  });

describe("LibrarySettings permission gating", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("does not request the library for a role without Libraries Read", async () => {
    setAuth({ permissions: ["books:read", "libraries:write"] });
    const request = vi.spyOn(API, "request").mockResolvedValue({});

    renderSettings();
    await flush();

    const paths = request.mock.calls.map((call) => call[1]);
    expect(paths).not.toContain("/libraries/1");
  });

  it("requests the library for a role with Libraries Read and Write", async () => {
    setAuth({
      permissions: ["books:read", "libraries:read", "libraries:write"],
    });
    const request = vi.spyOn(API, "request").mockResolvedValue({});

    renderSettings();
    await flush();

    const paths = request.mock.calls.map((call) => call[1]);
    expect(paths).toContain("/libraries/1");
  });
});
