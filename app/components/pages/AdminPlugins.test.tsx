import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import AdminPlugins from "./AdminPlugins";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

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
    setAuth({ permissions: ["config:read"] });
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
