import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import AdminUsers from "./AdminUsers";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const invite = (id: number, overrides = {}) => ({
  id,
  token: `token-${id}`,
  role_id: 3,
  role: { id: 3, name: "viewer" },
  all_library_access: false,
  libraries: [{ id: 7, name: "Comics" }],
  expires_at: "2026-11-25T12:00:00Z",
  created_by_username: "admin",
  ...overrides,
});

const mockApi = (invites: unknown[]) =>
  vi.spyOn(API, "request").mockImplementation(async (method, path) => {
    if (method === "GET" && path === "/users") {
      return {
        items: [{ id: 1, username: "admin", is_active: true }],
        total: 1,
      };
    }
    if (method === "GET" && path === "/roles") return { items: [], total: 0 };
    if (method === "GET" && path === "/invite-links") return invites;
    return undefined;
  });

const renderPage = () =>
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter>
        <AdminUsers />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const setupUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime, delay: null });

afterEach(() => {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: undefined,
  });
});

describe("AdminUsers pending invites", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["users:read", "users:write"] });
  });

  it("lists each pending invite with its role, access, and expiry at the top", async () => {
    mockApi([
      invite(1),
      invite(2, {
        all_library_access: true,
        libraries: [],
        role: { id: 2, name: "editor" },
      }),
    ]);
    renderPage();

    const section = await screen.findByRole("region", {
      name: "Pending invites",
    });
    const rows = within(section).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(within(rows[0]).getByText("viewer")).toBeVisible();
    expect(await within(rows[0]).findByText("Comics")).toBeVisible();
    expect(within(rows[0]).getByText(/^Expires /)).toBeVisible();
    expect(within(rows[1]).getByText("editor")).toBeVisible();
    expect(within(rows[1]).getByText("All libraries")).toBeVisible();
  });

  it("copies a pending invite's link again", async () => {
    mockApi([invite(1)]);
    // userEvent.setup installs its own clipboard, so replace it afterwards.
    const user = setupUser();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    renderPage();

    await user.click(
      await screen.findByRole("button", {
        name: "Copy invite link for viewer",
      }),
    );
    expect(writeText).toHaveBeenCalledWith(
      `${window.location.origin}/invite/token-1`,
    );
  });

  it("revokes a pending invite after confirming", async () => {
    const request = mockApi([invite(1)]);
    const user = setupUser();
    renderPage();

    await user.click(
      await screen.findByRole("button", { name: "Revoke invite for viewer" }),
    );
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Revoke",
      }),
    );
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith("POST", "/invite-links/1/revoke"),
    );
  });

  it("shows nothing for invites while none are pending, or for a role that can't add users", async () => {
    const request = mockApi([]);
    renderPage();
    expect(await screen.findByText("admin")).toBeVisible();
    expect(
      screen.queryByRole("region", { name: "Pending invites" }),
    ).not.toBeInTheDocument();

    setAuth({ permissions: ["users:read"] });
    request.mockClear();
    renderPage();
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "GET",
        "/users",
        null,
        {},
        expect.anything(),
      ),
    );
    expect(request).not.toHaveBeenCalledWith(
      "GET",
      "/invite-links",
      null,
      null,
      expect.anything(),
    );
  });
});
