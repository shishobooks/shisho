import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";

import UserMenu from "./UserMenu";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const openMenu = async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  render(
    <MemoryRouter>
      <UserMenu />
    </MemoryRouter>,
  );
  await user.click(screen.getByRole("button", { name: "Account menu" }));
};

const administration = () =>
  screen.queryByRole("menuitem", { name: "Administration" });

describe("UserMenu", () => {
  beforeEach(() => setAuth());

  it("links to Administration when a settings page is permitted", async () => {
    setAuth({ permissions: ["books:read", "libraries:read"] });
    await openMenu();

    expect(administration()).toHaveAttribute("href", "/admin");
  });

  it("hides Administration when no settings page is permitted", async () => {
    setAuth({ permissions: ["books:read", "series:read", "people:read"] });
    await openMenu();

    expect(
      screen.getByRole("menuitem", { name: "Preferences" }),
    ).toBeInTheDocument();
    expect(administration()).toBeNull();
  });

  it("hides Administration in Demo Mode", async () => {
    setAuth({ permissions: ["books:read", "libraries:read"], demoMode: true });
    await openMenu();

    expect(
      screen.getByRole("menuitem", { name: "Preferences" }),
    ).toBeInTheDocument();
    expect(administration()).toBeNull();
  });
});
