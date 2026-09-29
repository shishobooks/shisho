import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import AdminPlugins from "./AdminPlugins";

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

const installed = {
  scope: "local",
  id: "sample",
  name: "Sample Plugin",
  version: "1.0.0",
  status: "active",
  enabled: true,
  capabilities: {},
};

describe("AdminPlugins with Config Read only", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    auth.permissions = new Set(["config:read"]);
  });

  it("lists installed plugins without management controls", async () => {
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, path) =>
        path === "/plugins/installed" ? [installed] : [],
      );
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={["/settings/plugins"]}>
          <AdminPlugins />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Sample Plugin")).toBeInTheDocument();
    expect(request.mock.calls.map((call) => call[1])).toContain(
      "/plugins/installed",
    );
    expect(
      screen.queryByRole("button", { name: "Scan for Local Plugins" }),
    ).toBeNull();
  });
});
