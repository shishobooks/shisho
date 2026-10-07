import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import AdminPlugins from "./AdminPlugins";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("@/components/plugins/AdvancedOrderSection", () => ({
  AdvancedOrderSection: () => <div>Order Section Content</div>,
}));
vi.mock("@/components/plugins/AdvancedRepositoriesSection", () => ({
  AdvancedRepositoriesSection: () => <div>Repositories Section Content</div>,
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

const LocationProbe = () => {
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <div data-testid="location">{location.pathname + location.search}</div>
      <button onClick={() => navigate(-1)} type="button">
        Browser Back
      </button>
    </>
  );
};

// The last entry is the current URL; earlier ones are history Back reaches.
const renderAt = (...entries: string[]) => {
  vi.spyOn(API, "request").mockResolvedValue([]);
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={entries} initialIndex={entries.length - 1}>
        <AdminPlugins />
        <LocationProbe />
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const currentURL = () => screen.getByTestId("location").textContent;

describe("AdminPlugins advanced dialog deep link", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["config:read", "config:write"] });
  });

  it("opens on Repositories when the URL has ?advanced=repositories", async () => {
    renderAt("/settings/plugins?advanced=repositories");

    expect(
      await screen.findByRole("dialog", { name: "Advanced plugin settings" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Repositories" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(currentURL()).toBe("/settings/plugins?advanced=repositories");
  });

  it("falls back to the Order tab for an invalid section", async () => {
    renderAt("/settings/plugins?advanced=bogus");

    expect(
      await screen.findByRole("dialog", { name: "Advanced plugin settings" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Order" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("updates the URL param when switching tabs, keeping other params", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderAt("/settings/plugins?advanced=repositories&foo=bar");

    await user.click(await screen.findByRole("tab", { name: "Order" }));

    expect(screen.getByRole("tab", { name: "Order" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(currentURL()).toBe("/settings/plugins?advanced=order&foo=bar");
  });

  it("sets the param when opened from the gear button", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderAt("/settings/plugins/discover");

    await user.click(
      screen.getByRole("button", { name: "Advanced plugin settings" }),
    );

    expect(
      await screen.findByRole("dialog", { name: "Advanced plugin settings" }),
    ).toBeInTheDocument();
    expect(currentURL()).toBe("/settings/plugins/discover?advanced=order");
  });

  it("removes the param when the dialog closes, keeping other params", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderAt("/settings/plugins?foo=bar&advanced=order");

    await screen.findByRole("dialog", { name: "Advanced plugin settings" });
    await user.keyboard("{Escape}");

    expect(
      screen.queryByRole("dialog", { name: "Advanced plugin settings" }),
    ).not.toBeInTheDocument();
    expect(currentURL()).toBe("/settings/plugins?foo=bar");
  });

  it("switching tabs and closing replace the entry, so Back after closing does not reopen the dialog", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderAt("/settings", "/settings/plugins");

    await user.click(
      screen.getByRole("button", { name: "Advanced plugin settings" }),
    );
    await user.click(await screen.findByRole("tab", { name: "Repositories" }));
    expect(currentURL()).toBe("/settings/plugins?advanced=repositories");
    await user.keyboard("{Escape}");
    expect(currentURL()).toBe("/settings/plugins");

    await user.click(screen.getByRole("button", { name: "Browser Back" }));

    expect(currentURL()).toBe("/settings/plugins");
    expect(
      screen.queryByRole("dialog", { name: "Advanced plugin settings" }),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Browser Back" }));

    expect(currentURL()).toBe("/settings");
  });

  it("closing a deep-linked dialog replaces its entry, so Back does not reopen it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderAt("/settings", "/settings/plugins?advanced=repositories");

    await screen.findByRole("dialog", { name: "Advanced plugin settings" });
    await user.keyboard("{Escape}");
    expect(currentURL()).toBe("/settings/plugins");

    await user.click(screen.getByRole("button", { name: "Browser Back" }));

    expect(currentURL()).toBe("/settings");
  });
});
