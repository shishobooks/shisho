import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type {
  DeviceResponse,
  UpdateDevicePayload,
} from "@/types/generated/devices";
import { copyText } from "@/utils/clipboard";

import DeviceDetail from "./DeviceDetail";
import Devices from "./Devices";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("@/components/library/TopNav", () => ({ default: () => null }));
vi.mock("@/utils/clipboard", () => ({ copyText: vi.fn() }));
vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const kobo = (overrides: Partial<DeviceResponse> = {}): DeviceResponse => ({
  id: "kobo-1",
  user_id: 1,
  name: "Libra 2",
  device_type: "kobo",
  scope_type: "all",
  url_scope_pending: false,
  credential: "abcdefghjkmnpqrs",
  connections: ["kobo_sync", "ereader_browser"],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  ...overrides,
});

// stubAPI answers the Device routes from an in-memory Device, applying
// each change the page sends, and records every request.
const stubAPI = (initial: DeviceResponse) => {
  let device = initial;
  let devices = [initial];
  return vi
    .spyOn(API, "request")
    .mockImplementation(async (method, path, body) => {
      if (method === "GET" && path === "/user/devices") return devices;
      if (method === "GET" && path === `/user/devices/${device.id}`)
        return device;
      if (method === "POST" && path === "/user/devices") {
        const payload = body as { name: string; device_type: "kindle" };
        device = kobo({
          id: "new-1",
          name: payload.name,
          device_type: payload.device_type,
          connections: ["ereader_browser"],
        });
        devices = [...devices, device];
        return device;
      }
      if (method === "PATCH" && path === `/user/devices/${device.id}`) {
        const payload = body as UpdateDevicePayload;
        device = {
          ...device,
          ...(payload.name ? { name: payload.name } : {}),
          ...(payload.scope
            ? {
                scope_type: payload.scope.type,
                scope_library_id: payload.scope.library_id,
                scope_list_id: payload.scope.list_id,
                scope_name:
                  payload.scope.type === "library"
                    ? "Comics"
                    : payload.scope.type === "list"
                      ? "To Read"
                      : undefined,
              }
            : {}),
        };
        return device;
      }
      if (
        method === "POST" &&
        path === `/user/devices/${device.id}/credential`
      ) {
        device = { ...device, credential: "zyxwvutsrqpnmkjh" };
        return device;
      }
      if (method === "DELETE" && path === `/user/devices/${device.id}`) {
        devices = devices.filter((d) => d.id !== device.id);
        return undefined;
      }
      if (path === "/user/libraries")
        return [
          { id: 1, name: "Fiction" },
          { id: 2, name: "Comics" },
        ];
      if (path === "/lists")
        return { items: [{ id: 7, name: "To Read" }], total: 1 };
      throw new Error(`unexpected request ${method} ${path}`);
    });
};

const renderAt = (path: string) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route element={<Devices />} path="/user/devices" />
          <Route element={<DeviceDetail />} path="/user/devices/:id" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const setupUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const calls = (request: ReturnType<typeof stubAPI>, method: string) =>
  request.mock.calls.filter((call) => call[0] === method);

describe("Devices", () => {
  // Devices need no permission: any signed-in user manages their own.
  beforeEach(() => setAuth());
  afterEach(() => vi.restoreAllMocks());

  it("lists the user's Devices", async () => {
    stubAPI(kobo());
    renderAt("/user/devices");

    const link = await screen.findByRole("link", { name: /Libra 2/ });
    expect(link).toHaveAttribute("href", "/user/devices/kobo-1");
    expect(link).toHaveTextContent("All books · Kobo sync · eReader browser");
  });

  it("creates a Device with a name and type, then opens it", async () => {
    const request = stubAPI(kobo());
    const user = setupUser();
    renderAt("/user/devices");

    await user.click(await screen.findByRole("button", { name: "Add Device" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText("Name"), "Bedroom Kindle");
    await user.click(within(dialog).getByRole("radio", { name: /Kindle/ }));
    await user.click(
      within(dialog).getByRole("button", { name: "Add Device" }),
    );

    await waitFor(() =>
      expect(calls(request, "POST")).toContainEqual([
        "POST",
        "/user/devices",
        { name: "Bedroom Kindle", device_type: "kindle" },
      ]),
    );
    expect(
      await screen.findByRole("heading", { name: "Bedroom Kindle" }),
    ).toBeInTheDocument();
  });
});

describe("DeviceDetail", () => {
  beforeEach(() => {
    setAuth();
    vi.mocked(copyText).mockReset();
    vi.mocked(toast.success).mockReset();
  });
  afterEach(() => vi.restoreAllMocks());

  it("shows the credential unbroken, copies it, and gives a Kobo sync URL with no scope segment", async () => {
    stubAPI(kobo());
    vi.mocked(copyText).mockResolvedValue(true);
    const user = setupUser();
    renderAt("/user/devices/kobo-1");

    expect(await screen.findByLabelText("Credential")).toHaveTextContent(
      "abcdefghjkmnpqrs",
    );
    expect(screen.getByLabelText("Kobo sync URL")).toHaveTextContent(
      `${window.location.origin}/kobo/abcdefghjkmnpqrs`,
    );

    await user.click(screen.getByRole("button", { name: "Copy credential" }));
    expect(copyText).toHaveBeenCalledWith("abcdefghjkmnpqrs");
  });

  it("toasts an error when copying fails", async () => {
    stubAPI(kobo());
    vi.mocked(copyText).mockResolvedValue(false);
    vi.mocked(toast.error).mockReset();
    const user = setupUser();
    renderAt("/user/devices/kobo-1");

    await user.click(
      await screen.findByRole("button", { name: "Copy credential" }),
    );

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Could not copy"),
    );
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("lets a migrated Kobo waiting on its old sync URL save All Books", async () => {
    const request = stubAPI(kobo({ url_scope_pending: true }));
    const user = setupUser();
    renderAt("/user/devices/kobo-1");

    expect(
      await screen.findByText(/Its next sync sets the scope/),
    ).toBeInTheDocument();
    const scope = screen.getByRole("group", { name: "Scope" });
    await user.click(within(scope).getByRole("button", { name: "All Books" }));

    await waitFor(() =>
      expect(calls(request, "PATCH")).toContainEqual([
        "PATCH",
        "/user/devices/kobo-1",
        { scope: { type: "all" } },
      ]),
    );
  });

  it("regenerates the credential after confirming", async () => {
    const request = stubAPI(kobo());
    const user = setupUser();
    renderAt("/user/devices/kobo-1");

    await user.click(await screen.findByRole("button", { name: "Regenerate" }));
    const dialog = await screen.findByRole("dialog");
    expect(calls(request, "POST")).toHaveLength(0);
    await user.click(
      within(dialog).getByRole("button", { name: "Regenerate" }),
    );

    await waitFor(() =>
      expect(screen.getByLabelText("Credential")).toHaveTextContent(
        "zyxwvutsrqpnmkjh",
      ),
    );
    expect(calls(request, "POST")).toContainEqual([
      "POST",
      "/user/devices/kobo-1/credential",
    ]);
    expect(screen.getByLabelText("Kobo sync URL")).toHaveTextContent(
      `${window.location.origin}/kobo/zyxwvutsrqpnmkjh`,
    );
  });

  it("sets the scope to a library, and loads libraries only once asked", async () => {
    const request = stubAPI(kobo());
    const user = setupUser();
    renderAt("/user/devices/kobo-1");

    await screen.findByLabelText("Credential");
    expect(request.mock.calls.map((call) => call[1])).not.toContain(
      "/user/libraries",
    );
    expect(request.mock.calls.map((call) => call[1])).not.toContain("/lists");

    const scope = screen.getByRole("group", { name: "Scope" });
    await user.click(within(scope).getByRole("button", { name: "Library" }));
    expect(calls(request, "PATCH")).toHaveLength(0);

    expect(
      screen.getByText("Nothing is saved until you choose a library."),
    ).toBeInTheDocument();
    await user.click(await screen.findByRole("combobox", { name: "Library" }));
    await user.click(await screen.findByRole("option", { name: "Comics" }));

    await waitFor(() =>
      expect(calls(request, "PATCH")).toContainEqual([
        "PATCH",
        "/user/devices/kobo-1",
        { scope: { type: "library", library_id: 2 } },
      ]),
    );
    expect(
      within(scope).getByRole("button", { name: "Library" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("sets the scope back to all books in one click", async () => {
    const request = stubAPI(kobo({ scope_type: "list", scope_list_id: 7 }));
    const user = setupUser();
    renderAt("/user/devices/kobo-1");

    const scope = await screen.findByRole("group", { name: "Scope" });
    await user.click(within(scope).getByRole("button", { name: "All Books" }));

    await waitFor(() =>
      expect(calls(request, "PATCH")).toContainEqual([
        "PATCH",
        "/user/devices/kobo-1",
        { scope: { type: "all" } },
      ]),
    );
  });

  it("shows setup for every connection the Device's type uses", async () => {
    stubAPI(kobo());
    renderAt("/user/devices/kobo-1");

    expect(
      await screen.findByRole("heading", { name: "Kobo sync" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "eReader browser" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("eReader browser URL")).toHaveTextContent(
      `${window.location.origin}/ereader/key/abcdefghjkmnpqrs/`,
    );
    expect(screen.queryByRole("switch")).toBeNull();
  });

  it("closes the regenerate confirmation when Demo Mode rejects it", async () => {
    const request = stubAPI(kobo());
    const stubbed = request.getMockImplementation()!;
    request.mockImplementation(async (method, path, ...rest) => {
      if (method === "POST" && path === "/user/devices/kobo-1/credential") {
        throw new ShishoAPIError(
          "This action is unavailable in the demo.",
          "demo_mode",
          403,
        );
      }
      return stubbed(method, path, ...rest);
    });
    const user = setupUser();
    renderAt("/user/devices/kobo-1");

    await user.click(await screen.findByRole("button", { name: "Regenerate" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: "Regenerate" }),
    );

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByLabelText("Credential")).toHaveTextContent(
      "abcdefghjkmnpqrs",
    );
  });

  it("deletes the Device after confirming and returns to the list", async () => {
    const request = stubAPI(kobo());
    const user = setupUser();
    renderAt("/user/devices/kobo-1");

    await user.click(await screen.findByRole("button", { name: "Delete" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() =>
      expect(calls(request, "DELETE")).toContainEqual([
        "DELETE",
        "/user/devices/kobo-1",
      ]),
    );
    expect(
      await screen.findByRole("heading", { name: "Devices" }),
    ).toBeInTheDocument();
    expect(await screen.findByText(/No devices yet/)).toBeInTheDocument();
  });
});
