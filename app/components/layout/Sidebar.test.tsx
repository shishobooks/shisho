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

import Sidebar, { type SidebarItem } from "./Sidebar";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

const renderSidebar = (items: SidebarItem[]) =>
  render(
    <MemoryRouter>
      <Sidebar items={items} />
    </MemoryRouter>,
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
  });

  afterEach(() => {
    localStorage.clear();
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
        to: "/settings",
        icon: <Settings className="h-4 w-4" data-testid="settings-icon" />,
        label: "Settings",
      }),
    ]);
    expect(screen.getByTestId("settings-icon")).toBeInTheDocument();
  });
});
