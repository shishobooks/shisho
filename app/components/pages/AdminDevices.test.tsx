import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { AdminDeviceResponse } from "@/types/generated/devices";

import AdminDevices from "./AdminDevices";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const readersKobo: AdminDeviceResponse = {
  id: "kobo-1",
  user_id: 2,
  name: "Reader's Kobo",
  device_type: "kobo",
  scope_type: "all",
  url_scope_pending: false,
  connections: ["kobo_sync"],
  owner: { id: 2, username: "reader" },
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const renderPage = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <AdminDevices />
    </QueryClientProvider>,
  );
};

describe("AdminDevices", () => {
  afterEach(() => vi.restoreAllMocks());

  it("lists every user's Devices and revokes one", async () => {
    setAuth({ permissions: ["users:read", "users:write"] });
    let devices = [readersKobo];
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (method, path) => {
        if (method === "GET" && path === "/devices") return devices;
        if (method === "DELETE" && path === "/devices/kobo-1") {
          devices = [];
          return undefined;
        }
        throw new Error(`unexpected request ${method} ${path}`);
      });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();

    expect(await screen.findByText("Reader's Kobo")).toBeInTheDocument();
    expect(screen.getByText(/reader · Kobo sync/)).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "Revoke Reader's Kobo" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith("DELETE", "/devices/kobo-1"),
    );
    expect(
      await screen.findByText("No user has added a device."),
    ).toBeInTheDocument();
  });

  it("hides revoke from a role that cannot write users", async () => {
    setAuth({ permissions: ["users:read"] });
    vi.spyOn(API, "request").mockResolvedValue([readersKobo]);
    renderPage();

    expect(await screen.findByText("Reader's Kobo")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Revoke/ })).toBeNull();
    expect(screen.queryByText(/Revoke a lost or stolen one/)).toBeNull();
  });
});
