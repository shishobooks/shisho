import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Book, Settings } from "lucide-react";
import { MemoryRouter } from "react-router-dom";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { UpdateCheckResponse } from "@/types/generated/updatecheck";

import Sidebar, { type SidebarItem } from "./Sidebar";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

const upToDate: UpdateCheckResponse = {
  current_version: "0.8.0",
  latest_version: "0.8.0",
  releases_behind: 0,
  update_available: false,
};

const behind: UpdateCheckResponse = {
  current_version: "0.6.0",
  latest_version: "0.8.0",
  releases_behind: 2,
  update_available: true,
};

const stubUpdateCheck = (response: UpdateCheckResponse) =>
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    if (path === "/update-check") return response;
    throw new Error(`unexpected request to ${path}`);
  });

const renderSidebar = (items: SidebarItem[]) =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <Sidebar items={items} />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const buildItem = (overrides: Partial<SidebarItem> = {}): SidebarItem => ({
  to: "/books",
  icon: <Book className="h-4 w-4" />,
  label: "Books",
  isActive: false,
  ...overrides,
});

describe("Sidebar", () => {
  beforeEach(() => {
    localStorage.clear();
    setAuth({ permissions: ["config:read"] });
    stubUpdateCheck(upToDate);
  });

  afterEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("renders each visible item's label", () => {
    renderSidebar([
      buildItem({ to: "/a", label: "Alpha" }),
      buildItem({ to: "/b", label: "Bravo" }),
    ]);
    expect(screen.getByText("Alpha")).toBeInTheDocument();
    expect(screen.getByText("Bravo")).toBeInTheDocument();
  });

  it("hides items with show: false", () => {
    renderSidebar([
      buildItem({ to: "/a", label: "Alpha" }),
      buildItem({ to: "/b", label: "Bravo", show: false }),
    ]);
    expect(screen.getByText("Alpha")).toBeInTheDocument();
    expect(screen.queryByText("Bravo")).not.toBeInTheDocument();
  });

  it("treats missing show as visible", () => {
    renderSidebar([buildItem({ to: "/a", label: "Alpha" })]);
    expect(screen.getByText("Alpha")).toBeInTheDocument();
  });

  it("starts expanded by default and shows version footer", () => {
    renderSidebar([buildItem()]);
    expect(screen.getByText("shisho")).toBeInTheDocument();
  });

  it("starts collapsed when localStorage says so, and hides version footer", () => {
    localStorage.setItem("shisho-sidebar-collapsed", "true");
    renderSidebar([buildItem()]);
    expect(screen.queryByText("shisho")).not.toBeInTheDocument();
  });

  it("toggles collapsed state and persists to localStorage", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderSidebar([buildItem()]);
    expect(localStorage.getItem("shisho-sidebar-collapsed")).toBe("false");

    const collapseButton = screen.getByRole("button", {
      name: /collapse sidebar/i,
    });
    await user.click(collapseButton);

    expect(localStorage.getItem("shisho-sidebar-collapsed")).toBe("true");
    expect(
      screen.getByRole("button", { name: /expand sidebar/i }),
    ).toBeInTheDocument();
  });

  it("names each nav link by its label when collapsed to icons", () => {
    localStorage.setItem("shisho-sidebar-collapsed", "true");
    renderSidebar([buildItem({ to: "/a", label: "Alpha" })]);
    expect(screen.queryByText("Alpha")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Alpha" })).toHaveAttribute(
      "href",
      "/a",
    );
  });

  it("applies active styling to active items", () => {
    renderSidebar([
      buildItem({ to: "/a", label: "Alpha", isActive: true }),
      buildItem({ to: "/b", label: "Bravo", isActive: false }),
    ]);
    const alpha = screen.getByText("Alpha").closest("a");
    const bravo = screen.getByText("Bravo").closest("a");
    expect(alpha?.className).toContain("bg-primary/10");
    expect(bravo?.className).not.toContain("bg-primary/10");
  });

  it("supports a Settings icon item type", () => {
    renderSidebar([
      buildItem({
        to: "/admin",
        icon: <Settings className="h-4 w-4" data-testid="settings-icon" />,
        label: "Settings",
      }),
    ]);
    expect(screen.getByTestId("settings-icon")).toBeInTheDocument();
  });

  it("marks the active item as the current page", () => {
    renderSidebar([
      buildItem({ to: "/a", label: "Alpha", isActive: true }),
      buildItem({ to: "/b", label: "Bravo" }),
    ]);
    expect(screen.getByRole("link", { name: "Alpha" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Bravo" })).not.toHaveAttribute(
      "aria-current",
    );
  });

  describe("update check", () => {
    it("shows the version linked to the release notes while up to date", async () => {
      renderSidebar([buildItem()]);
      expect(await screen.findByText("shisho")).toBeInTheDocument();
      expect(screen.getByText("test").closest("a")).toHaveAttribute(
        "href",
        "https://github.com/shishobooks/shisho/releases",
      );
      expect(screen.queryByText("Update available")).not.toBeInTheDocument();
    });

    it("replaces the version with the update and how far behind it is", async () => {
      stubUpdateCheck(behind);
      renderSidebar([buildItem()]);

      const notice = (await screen.findByText("Update available")).closest("a");
      expect(notice).toHaveAttribute(
        "href",
        "https://github.com/shishobooks/shisho/releases",
      );
      expect(notice).toHaveTextContent("0.6.0 → 0.8.0");
      expect(notice).toHaveTextContent("2 releases behind");
      expect(screen.queryByText("shisho")).not.toBeInTheDocument();
    });

    it("keeps an icon link to the release notes when collapsed", async () => {
      stubUpdateCheck(behind);
      localStorage.setItem("shisho-sidebar-collapsed", "true");
      renderSidebar([buildItem()]);

      expect(
        await screen.findByRole("link", {
          name: "Update available: 0.8.0, 2 releases behind",
        }),
      ).toHaveAttribute(
        "href",
        "https://github.com/shishobooks/shisho/releases",
      );
    });

    it("never asks without Config Read", () => {
      setAuth({ permissions: ["books:read"] });
      const request = stubUpdateCheck(behind);
      renderSidebar([buildItem()]);

      expect(screen.getByText("shisho")).toBeInTheDocument();
      expect(request).not.toHaveBeenCalled();
    });
  });
});
