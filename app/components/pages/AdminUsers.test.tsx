import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
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
  expires_at: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(),
  created_by_username: "admin",
  ...overrides,
});

const USERS = [
  {
    id: 1,
    username: "admin",
    is_active: true,
    role: { id: 1, name: "admin" },
    library_access: [{ id: 1, user_id: 1, library_id: null }],
  },
  {
    id: 2,
    username: "bobby",
    email: "bob@example.com",
    is_active: true,
    role: { id: 3, name: "viewer" },
    library_access: [
      { id: 2, user_id: 2, library_id: 7 },
      { id: 3, user_id: 2, library_id: 8 },
    ],
  },
  {
    id: 3,
    username: "carol",
    is_active: false,
    role: { id: 2, name: "editor" },
    library_access: [],
  },
];

const ROLES = [
  { id: 1, name: "admin", is_system: true, permissions: [{}, {}] },
  { id: 3, name: "viewer", is_system: true, permissions: [{}] },
];

const mockApi = (invites: unknown[], users: unknown[] = USERS) =>
  vi.spyOn(API, "request").mockImplementation(async (method, path) => {
    if (method === "GET" && path === "/users") {
      return { items: users, total: users.length };
    }
    if (method === "GET" && path === "/roles") {
      return { items: ROLES, total: ROLES.length };
    }
    if (method === "GET" && path === "/libraries") {
      return {
        items: [
          { id: 7, name: "Comics" },
          { id: 8, name: "Novels" },
        ],
        total: 2,
      };
    }
    if (method === "GET" && path === "/invite-links") return invites;
    return undefined;
  });

const Location = () => (
  <div data-testid="location">{useLocation().pathname}</div>
);

const renderPage = (path = "/admin/users") =>
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route element={<AdminUsers />} path="/admin/users" />
          <Route element={<AdminUsers />} path="/admin/users/roles" />
        </Routes>
        <Location />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const setupUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime, delay: null });

const usersList = () => screen.findByRole("list", { name: "Users" });

afterEach(() => {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: undefined,
  });
});

describe("AdminUsers", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["users:read", "users:write"] });
  });

  it("opens on the Users tab with one title and an Add User button", async () => {
    mockApi([]);
    renderPage();

    expect(
      screen.getByRole("heading", { level: 1, name: "Users & Roles" }),
    ).toBeVisible();
    expect(screen.getByRole("tab", { name: /Users/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("link", { name: "Add User" })).toHaveAttribute(
      "href",
      "/admin/users/create",
    );
    expect(
      screen.queryByRole("button", { name: "Add Role" }),
    ).not.toBeInTheDocument();
  });

  it("lists each user with their role, library access, and whether they're inactive", async () => {
    mockApi([]);
    renderPage();

    const rows = within(await usersList()).getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(within(rows[0]).getByText("All libraries")).toBeVisible();
    expect(
      await within(rows[1]).findByText("bob@example.com · Comics, Novels"),
    ).toBeVisible();
    expect(within(rows[1]).getByText("viewer")).toBeVisible();
    expect(within(rows[2]).getByText("No libraries")).toBeVisible();
    expect(within(rows[2]).getByText("Inactive")).toBeVisible();
    expect(
      within(rows[1]).getByRole("link", { name: /bobby/ }),
    ).toHaveAttribute("href", "/admin/users/2");
  });

  // GET /libraries lists only the Libraries the viewer can access, so an
  // admin limited to some can't name the rest; they're counted instead.
  it("counts the Libraries it can't name", async () => {
    mockApi(
      [],
      [
        {
          id: 4,
          username: "dana",
          is_active: true,
          library_access: [
            { id: 4, user_id: 4, library_id: 7 },
            { id: 5, user_id: 4, library_id: 9 },
          ],
        },
        {
          id: 5,
          username: "eve",
          is_active: true,
          library_access: [{ id: 6, user_id: 5, library_id: 9 }],
        },
      ],
    );
    renderPage();

    const rows = within(await usersList()).getAllByRole("listitem");
    expect(await within(rows[0]).findByText("Comics and 1 more")).toBeVisible();
    expect(within(rows[1]).getByText("1 library")).toBeVisible();
  });

  // The initial is the first character as it's read, with its vowel sign
  // or accent, never half of one.
  it("starts each user's avatar with their first whole character", async () => {
    mockApi(
      [],
      [
        {
          id: 6,
          username: "\u0915\u093f\u0930\u0923",
          is_active: true,
          library_access: [],
        },
      ],
    );
    renderPage();

    const [row] = within(await usersList()).getAllByRole("listitem");
    expect(within(row).getByText("\u0915\u093f")).toBeInTheDocument();
  });

  it("puts pending invites at the top of the same list, with their role, access, and expiry", async () => {
    mockApi([
      invite(1),
      invite(2, {
        all_library_access: true,
        libraries: [],
        role: { id: 2, name: "editor" },
      }),
    ]);
    renderPage();

    const list = await usersList();
    await within(list).findAllByText("Invite link");
    const rows = within(list).getAllByRole("listitem");
    expect(rows).toHaveLength(5);
    expect(within(rows[0]).getByText("Pending")).toBeVisible();
    expect(within(rows[0]).getByText("viewer")).toBeVisible();
    expect(rows[0]).toHaveTextContent(
      "Comics · Expires in 7 days · Created by admin",
    );
    expect(within(rows[1]).getByText("editor")).toBeVisible();
    expect(rows[1]).toHaveTextContent("All libraries · Expires in 7 days");
    expect(within(rows[2]).getByRole("link")).toHaveAttribute(
      "href",
      "/admin/users/1",
    );
    // No second heading: the invites belong to the user list.
    expect(screen.getAllByRole("heading", { name: /Users/ })).toHaveLength(1);
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

  it("shows Roles at its own URL, where the button adds a role", async () => {
    mockApi([]);
    const user = setupUser();
    renderPage("/admin/users/roles");

    expect(screen.getByRole("tab", { name: /Roles/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    const roles = await screen.findByRole("list", { name: "Roles" });
    expect(within(roles).getAllByRole("listitem")).toHaveLength(2);
    expect(within(roles).getByText("2 permissions")).toBeVisible();
    expect(
      screen.queryByRole("link", { name: "Add User" }),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Add Role" }));
    expect(
      await screen.findByRole("dialog", { name: "Create Role" }),
    ).toBeVisible();
  });

  it("switches tabs through the URL", async () => {
    mockApi([]);
    const user = setupUser();
    renderPage();

    await user.click(screen.getByRole("tab", { name: /Roles/ }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/admin/users/roles",
    );
    await user.click(screen.getByRole("tab", { name: /Users/ }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      /^\/admin\/users$/,
    );
  });

  it("gives a role that can only view users no add buttons and no invites", async () => {
    setAuth({ permissions: ["users:read"] });
    const request = mockApi([invite(1)]);
    renderPage();

    const rows = within(await usersList()).getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    // Without the Library list, a user's access is counted, not named.
    expect(
      within(rows[1]).getByText("bob@example.com · 2 libraries"),
    ).toBeVisible();
    expect(
      screen.queryByRole("link", { name: "Add User" }),
    ).not.toBeInTheDocument();
    expect(request).not.toHaveBeenCalledWith(
      "GET",
      "/invite-links",
      null,
      null,
      expect.anything(),
    );
  });
});
