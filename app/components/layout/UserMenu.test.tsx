import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  createMemoryRouter,
  MemoryRouter,
  RouterProvider,
} from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { auth, setAuth } from "@/testing/auth";

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

  it("links to Administration when an administration page is permitted", async () => {
    setAuth({ permissions: ["books:read", "libraries:read"] });
    await openMenu();

    expect(administration()).toHaveAttribute("href", "/admin");
  });

  it("hides Administration when no administration page is permitted", async () => {
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

  // Signing out re-renders the signed-in page as signed out, and its route
  // guard sends it to /login?redirect=... in place. The menu's own move to
  // /login replaces that entry, so Back skips the sign-in page.
  it("leaves a single sign-in page in history on Sign out", async () => {
    const router = createMemoryRouter(
      [
        { path: "/", element: <p>Home</p> },
        { path: "/books", element: <UserMenu /> },
        { path: "/login", element: <p>Sign in</p> },
      ],
      { initialEntries: ["/", "/books"], initialIndex: 1 },
    );
    auth.logout.mockImplementation(async () => {
      await router.navigate("/login?redirect=%2Fbooks", { replace: true });
    });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<RouterProvider router={router} />);
    await user.click(screen.getByRole("button", { name: "Account menu" }));
    await user.click(screen.getByRole("menuitem", { name: "Sign out" }));
    await waitFor(() => expect(router.state.location.search).toBe(""));
    expect(router.state.location.pathname).toBe("/login");

    await act(() => router.navigate(-1));
    expect(router.state.location.pathname).toBe("/");
  });
});
