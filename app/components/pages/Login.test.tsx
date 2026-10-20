import { render, screen, waitFor } from "@testing-library/react";
import {
  createMemoryRouter,
  MemoryRouter,
  RouterProvider,
} from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";

import Login from "./Login";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const renderLogin = () =>
  render(
    <MemoryRouter>
      <Login />
    </MemoryRouter>,
  );

describe("Login Demo Mode", () => {
  beforeEach(() => {
    setAuth({ demoMode: true, user: null });
  });

  it("shows the demo notice and pre-fills the shared credentials", () => {
    renderLogin();

    expect(
      screen.getByText(
        "This is a read-only public demo of Shisho. Sign in with the pre-filled credentials. Changes and downloads are disabled.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toHaveValue("demo");
    expect(screen.getByLabelText("Password")).toHaveValue("shishodemo");
  });

  it("does not show or pre-fill demo details outside Demo Mode", () => {
    setAuth({ user: null });

    renderLogin();

    expect(
      screen.queryByText(/read-only public demo/i),
    ).not.toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toHaveValue("");
    expect(screen.getByLabelText("Password")).toHaveValue("");
  });
});

describe("Login redirect", () => {
  const renderSignedInAt = (path: string) => {
    setAuth({ user: { id: 1 } });
    const router = createMemoryRouter(
      [
        { path: "/login", element: <Login /> },
        { path: "*", element: null },
      ],
      { initialEntries: [path] },
    );
    render(<RouterProvider router={router} />);
    return router;
  };

  it("sends a signed-in user to the in-app page it names", async () => {
    const router = renderSignedInAt("/login?redirect=%2Flists");

    await waitFor(() => expect(router.state.location.pathname).toBe("/lists"));
  });

  it("sends a signed-in user home instead of to another site", async () => {
    const router = renderSignedInAt("/login?redirect=%2F%2Fevil.example");

    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
  });
});
