import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { useMobileNav } from "@/contexts/MobileNav";
import { API } from "@/libraries/api";
import { ALL_PERMISSIONS, setAuth } from "@/testing/auth";

import MobileDrawer from "./MobileDrawer";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/contexts/MobileNav", () => ({
  useMobileNav: vi.fn(),
}));

let isOpen = false;
const server = { libraries: [] as unknown[], lists: [] as unknown[] };

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

beforeEach(() => {
  isOpen = false;
  server.libraries = [];
  server.lists = [];
  setAuth({
    permissions: ALL_PERMISSIONS,
    user: { username: "admin", role_name: "Admin" },
  });
  vi.mocked(useMobileNav).mockImplementation(
    () =>
      ({
        isOpen,
        open: vi.fn(),
        close: vi.fn(),
        toggle: vi.fn(),
      }) as never,
  );
});

afterEach(() => vi.restoreAllMocks());

const stubRequests = () =>
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    if (path === "/user/libraries") return server.libraries;
    if (path === "/lists") {
      return { items: server.lists, total: server.lists.length };
    }
    return {};
  });

const renderDrawer = (path: string, queryClient = new QueryClient()) => {
  const request = stubRequests();
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <MobileDrawer />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return request;
};

const getDrawer = () =>
  screen.getByRole("complementary", { name: "Mobile navigation" });

const getActiveLabels = () =>
  within(getDrawer())
    .getAllByRole("link")
    .filter((link) => link.classList.contains("bg-primary/10"))
    .map((link) => link.textContent);

describe("MobileDrawer", () => {
  it("casts no shadow while closed", () => {
    renderDrawer("/settings/review-criteria");

    expect(getDrawer()).not.toHaveClass("shadow-2xl");
  });

  it("casts a shadow while open", () => {
    isOpen = true;
    renderDrawer("/settings/review-criteria");

    expect(getDrawer()).toHaveClass("shadow-2xl");
  });

  it.each([
    ["/settings/server", "Server"],
    ["/settings/sharing", "Sharing"],
    ["/settings/users/3", "Users"],
  ])("highlights exactly one item on %s", (path, expected) => {
    isOpen = true;
    renderDrawer(path);

    expect(getActiveLabels()).toEqual([expected]);
  });

  it("highlights Global Settings when no admin item matches the page", () => {
    // /settings has no page of its own (it redirects to the first permitted
    // one), so no admin item matches it.
    setAuth({
      permissions: ["users:read", "users:write"],
      user: { username: "manager", role_name: "Manager" },
    });
    isOpen = true;
    renderDrawer("/settings");

    expect(getActiveLabels()).toEqual(["Global Settings"]);
  });

  it("highlights only Lists outside settings pages", () => {
    isOpen = true;
    renderDrawer("/lists");

    expect(getActiveLabels()).toEqual(["Lists"]);
  });
});

describe("MobileDrawer library picker", () => {
  beforeEach(() => {
    server.lists = [{ id: 3, name: "To Read", book_count: 2 }];
  });

  it("offers the role's libraries without Libraries Read", async () => {
    setAuth({
      permissions: ["books:read"],
      user: { username: "reader", role_name: "Reader" },
    });
    server.libraries = [{ id: 1, name: "Fiction" }];
    renderDrawer("/libraries/1/books/7");

    expect(await within(getDrawer()).findByText("Fiction")).toBeInTheDocument();
  });

  it("requests no libraries and shows none for a role without Books Read", async () => {
    setAuth({
      permissions: ["shares:read"],
      user: { username: "reader", role_name: "Shares Only" },
    });
    // The Kobo sync scope in Security Settings fills the same cache.
    const queryClient = new QueryClient();
    queryClient.setQueryData(["UserLibraries"], [{ id: 1, name: "Fiction" }]);
    const request = renderDrawer("/lists/3", queryClient);

    await waitFor(() =>
      expect(request.mock.calls.map((call) => call[1])).toContain("/lists"),
    );
    expect(request.mock.calls.map((call) => call[1])).not.toContain(
      "/user/libraries",
    );
    expect(within(getDrawer()).queryByText("Fiction")).not.toBeInTheDocument();
    // Lists stay reachable from the drawer's own nav item.
    expect(
      within(getDrawer()).getByRole("link", { name: "Lists" }),
    ).toBeInTheDocument();
  });

  it("marks the current library, list, and page for screen readers", async () => {
    setAuth({
      permissions: ["books:read"],
      user: { username: "reader", role_name: "Reader" },
    });
    server.libraries = [
      { id: 1, name: "Fiction" },
      { id: 2, name: "Comics" },
    ];
    isOpen = true;
    stubRequests();
    // The drawer reads the library from the route params.
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={["/libraries/1/books/7"]}>
          <Routes>
            <Route element={<MobileDrawer />} path="/libraries/:libraryId/*" />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const drawer = getDrawer();
    const fiction = await within(drawer).findByRole("button", {
      name: "Fiction",
    });
    expect(fiction).toHaveAttribute("aria-current", "true");
    expect(
      within(drawer).getByRole("button", { name: "Comics" }),
    ).not.toHaveAttribute("aria-current");
    const current = within(drawer)
      .getAllByRole("link")
      .filter((link) => link.getAttribute("aria-current") === "page");
    expect(current.map((link) => link.textContent)).toEqual(getActiveLabels());
    expect(current).toHaveLength(1);
  });

  it("marks the current list as the current page", async () => {
    isOpen = true;
    renderDrawer("/lists/3");

    const link = await within(getDrawer()).findByRole("link", {
      name: /To Read/,
    });
    expect(link).toHaveAttribute("aria-current", "page");
  });
});
