import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import CreateUser from "./CreateUser";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

// The real unsaved-changes blocker, which needs a data router.
const renderPage = () => {
  vi.spyOn(API, "request").mockImplementation(async (_method, path) =>
    path === "/roles" || path === "/libraries"
      ? { items: [], total: 0 }
      : undefined,
  );
  const router = createMemoryRouter(
    [
      { path: "/admin/users/create", element: <CreateUser /> },
      { path: "/admin/users", element: <div>Users page</div> },
    ],
    { initialEntries: ["/admin/users/create?method=password"] },
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
};

describe("CreateUser unsaved changes", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["users:read", "users:write"] });
  });

  it("switches between the two forms without asking, but asks before leaving", async () => {
    const router = renderPage();
    const user = userEvent.setup({
      advanceTimers: vi.advanceTimersByTime,
      delay: null,
    });

    await user.type(screen.getByLabelText("Username"), "newuser");
    await user.click(screen.getByRole("radio", { name: "Invite link" }));

    expect(screen.queryByText("Unsaved Changes")).not.toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Invite link" })).toBeChecked();
    expect(router.state.location.search).toBe("");

    // Switching back finds the input kept.
    await user.click(screen.getByRole("radio", { name: "Set a password" }));
    expect(screen.getByLabelText("Username")).toHaveValue("newuser");

    act(() => {
      void router.navigate("/admin/users");
    });
    expect(await screen.findByText("Unsaved Changes")).toBeVisible();
    expect(router.state.location.pathname).toBe("/admin/users/create");
  });
});
