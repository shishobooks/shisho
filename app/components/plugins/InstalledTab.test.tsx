import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast, Toaster } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AuthProvider } from "@/components/contexts/Auth";

import { InstalledTab } from "./InstalledTab";

afterEach(() => {
  toast.dismiss();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const renderWithScanResponse = (scanResponse: () => Response) => {
  vi.stubGlobal("__APP_VERSION__", "test");
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const path = String(input).split("?")[0];
    if (init?.method === "POST" && path === "/api/plugins/scan")
      return scanResponse();
    if (path === "/api/auth/status")
      return Response.json({ needs_setup: false, demo_mode: true });
    if (path === "/api/auth/me")
      return Response.json({
        id: 1,
        username: "admin",
        permissions: ["config:read", "config:write"],
        role_name: "admin",
      });
    return Response.json([]);
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <AuthProvider>
        <MemoryRouter>
          <InstalledTab />
          <Toaster />
        </MemoryRouter>
      </AuthProvider>
    </QueryClientProvider>,
  );
  return { client, view };
};

describe("scanning for local plugins", () => {
  it("shows only the Demo Mode toast when the scan is rejected by Demo Mode", async () => {
    const { client, view } = renderWithScanResponse(() =>
      Response.json(
        {
          error: {
            code: "demo_mode",
            message: "This action is unavailable in the demo.",
          },
        },
        { status: 403 },
      ),
    );
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      await user.click(
        await screen.findByRole("button", { name: "Scan for Local Plugins" }),
      );
      expect(
        await screen.findByText("This action is unavailable in the demo."),
      ).toBeInTheDocument();
      expect(
        screen.getAllByText("This action is unavailable in the demo."),
      ).toHaveLength(1);
      expect(screen.queryByText(/Failed to scan/)).not.toBeInTheDocument();
    } finally {
      view.unmount();
      client.clear();
    }
  });

  it("shows the caller's toast for an ordinary failure", async () => {
    const { client, view } = renderWithScanResponse(() =>
      Response.json(
        { error: { code: "internal", message: "disk on fire" } },
        { status: 500 },
      ),
    );
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      await user.click(
        await screen.findByRole("button", { name: "Scan for Local Plugins" }),
      );
      // The toast carries the server's message (requestErrorMessage).
      expect(await screen.findByText("disk on fire")).toBeInTheDocument();
      expect(
        screen.queryByText("This action is unavailable in the demo."),
      ).not.toBeInTheDocument();
    } finally {
      view.unmount();
      client.clear();
    }
  });
});
