import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import { PluginDetail } from "./PluginDetail";

// Uses the real query hooks, so the Config Read gates inside them decide what
// a role that can open the page may load.

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    demoMode: false,
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
    canWrite: (resource: string) => auth.permissions.has(`${resource}:write`),
  }),
}));

vi.mock("@/hooks/useUnsavedChanges", () => ({
  useUnsavedChanges: () => ({
    cancelNavigation: vi.fn(),
    proceedNavigation: vi.fn(),
    showBlockerDialog: false,
  }),
}));

const installed = {
  scope: "local",
  id: "sample",
  name: "Sample Plugin",
  version: "1.0.0",
  status: "active",
  enabled: true,
  capabilities: {},
};

const renderDetail = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/settings/plugins/local/sample"]}>
        <Routes>
          <Route
            element={<PluginDetail />}
            path="/settings/plugins/:scope/:id"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("PluginDetail with Config Read only", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    auth.permissions = new Set(["config:read"]);
  });

  it("loads the installed plugin and shows no management controls", async () => {
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, path) => {
        if (path === "/plugins/installed") return [installed];
        if (path === "/plugins/installed/local/sample/config")
          return { schema: {}, values: {}, declaredFields: [] };
        return [];
      });

    renderDetail();

    expect(
      (await screen.findAllByText("Sample Plugin")).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText("Plugin not found")).toBeNull();
    expect(request.mock.calls.map((call) => call[1])).toContain(
      "/plugins/installed",
    );
    expect(screen.queryByRole("button", { name: "Uninstall" })).toBeNull();
    expect(screen.queryByRole("switch")).toBeNull();
  });
});
