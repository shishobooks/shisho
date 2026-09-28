import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { useMobileNav } from "@/contexts/MobileNav";
import { useLibraries } from "@/hooks/queries/libraries";
import { useListLists } from "@/hooks/queries/lists";
import { useAuth } from "@/hooks/useAuth";

import MobileDrawer from "./MobileDrawer";

vi.mock("@/hooks/useAuth", () => ({
  useAuth: vi.fn(),
}));

vi.mock("@/contexts/MobileNav", () => ({
  useMobileNav: vi.fn(),
}));

vi.mock("@/hooks/queries/libraries", () => ({
  useLibraries: vi.fn(),
}));

vi.mock("@/hooks/queries/lists", () => ({
  useListLists: vi.fn(),
}));

let isOpen = false;

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

beforeEach(() => {
  isOpen = false;
  vi.mocked(useAuth).mockReturnValue({
    demoMode: false,
    user: { username: "admin", role_name: "Admin" },
    logout: vi.fn(),
    hasPermission: () => true,
  } as never);
  vi.mocked(useMobileNav).mockImplementation(
    () =>
      ({
        isOpen,
        open: vi.fn(),
        close: vi.fn(),
        toggle: vi.fn(),
      }) as never,
  );
  vi.mocked(useLibraries).mockReturnValue({ data: { items: [] } } as never);
  vi.mocked(useListLists).mockReturnValue({ data: { items: [] } } as never);
});

const renderDrawer = (path: string) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <MobileDrawer />
    </MemoryRouter>,
  );

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
    ["/settings", "Server"],
    ["/settings/sharing", "Sharing"],
    ["/settings/users/3", "Users"],
  ])("highlights exactly one item on %s", (path, expected) => {
    isOpen = true;
    renderDrawer(path);

    expect(getActiveLabels()).toEqual([expected]);
  });

  it("highlights Global Settings when no admin item matches the page", () => {
    // A user who can manage users but not the server config sees no Server
    // item, so /settings has no admin item to highlight.
    vi.mocked(useAuth).mockReturnValue({
      demoMode: false,
      user: { username: "manager", role_name: "Manager" },
      logout: vi.fn(),
      hasPermission: (resource: string) => resource === "users",
    } as never);
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
    vi.mocked(useListLists).mockReturnValue({
      data: { items: [{ id: 3, name: "To Read", book_count: 2 }] },
    } as never);
  });

  it("offers the library picker to a role that can read libraries", () => {
    vi.mocked(useLibraries).mockReturnValue({
      data: { items: [{ id: 1, name: "Fiction" }] },
    } as never);
    renderDrawer("/libraries/1/books/7");

    expect(within(getDrawer()).getByText("Fiction")).toBeInTheDocument();
  });

  it("hides the library picker from a role without Libraries Read", () => {
    vi.mocked(useAuth).mockReturnValue({
      demoMode: false,
      user: { username: "reader", role_name: "Shares Only" },
      logout: vi.fn(),
      hasPermission: (resource: string, operation: string) =>
        resource === "books" && operation === "read",
    } as never);
    vi.mocked(useLibraries).mockClear();
    renderDrawer("/libraries/1/books/7");

    expect(
      within(getDrawer()).queryByText("Select Library"),
    ).not.toBeInTheDocument();
    for (const call of vi.mocked(useLibraries).mock.calls) {
      expect(call[1]?.enabled).toBe(false);
    }
    // Lists stay reachable from the drawer's own nav item.
    expect(
      within(getDrawer()).getByRole("link", { name: "Lists" }),
    ).toBeInTheDocument();
  });
});
