import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";

import TopNav from "./TopNav";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/contexts/MobileNav", () => ({
  useMobileNav: () => ({ toggle: vi.fn() }),
}));
vi.mock("@/components/layout/UserMenu", () => ({ default: () => null }));
vi.mock("@/components/library/GlobalSearch", () => ({ default: () => null }));
vi.mock("@/components/library/LibraryListPicker", () => ({
  default: () => null,
}));
vi.mock("@/components/library/Logo", () => ({ default: () => null }));
vi.mock("@/components/library/ResyncButton", () => ({
  ResyncButton: () => <div data-testid="resync-button" />,
}));

const renderNav = () =>
  render(
    <MemoryRouter initialEntries={["/libraries/1"]}>
      <Routes>
        <Route element={<TopNav />} path="/libraries/:libraryId" />
      </Routes>
    </MemoryRouter>,
  );

const settingsLink = () =>
  screen
    .queryAllByRole("link")
    .find((link) => link.getAttribute("href") === "/settings");

describe("TopNav", () => {
  beforeEach(() => {
    setAuth();
  });

  it("hides the resync button without Jobs Read, which its status query needs", () => {
    setAuth({ permissions: ["books:read", "jobs:write"] });
    renderNav();

    expect(screen.queryByTestId("resync-button")).toBeNull();
  });

  it("shows the resync button with both jobs permissions", () => {
    setAuth({ permissions: ["books:read", "jobs:read", "jobs:write"] });
    renderNav();

    expect(screen.getByTestId("resync-button")).toBeInTheDocument();
  });

  it("offers the mobile search toggle only with Books Read", () => {
    setAuth({ permissions: ["shares:read"] });
    const { unmount } = renderNav();
    expect(screen.queryByRole("button", { name: "Open search" })).toBeNull();
    unmount();

    setAuth({ permissions: ["books:read"] });
    renderNav();
    expect(
      screen.getByRole("button", { name: "Open search" }),
    ).toBeInTheDocument();
  });

  it("hides the settings gear when no settings page is permitted", () => {
    setAuth({ permissions: ["books:read", "series:read", "people:read"] });
    renderNav();

    expect(settingsLink()).toBeUndefined();
  });

  it("shows the settings gear when a settings page is permitted", () => {
    setAuth({ permissions: ["books:read", "libraries:read"] });
    renderNav();

    expect(settingsLink()).toBeDefined();
  });
});
