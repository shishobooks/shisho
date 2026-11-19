import { render, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { newSetupDraft, saveSetupDraft } from "@/components/setup/setupDraft";
import { setAuth } from "@/testing/auth";

import Setup from "./Setup";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

// The account form once an account exists: the test auth reports no setup
// needed and a signed-in admin.
const renderAccountStep = () => {
  const router = createMemoryRouter(
    [
      { path: "/setup", element: <Setup /> },
      { path: "/setup/library", element: <p>Library step</p> },
      { path: "/", element: <p>The app</p> },
    ],
    { initialEntries: ["/setup"] },
  );
  render(<RouterProvider router={router} />);
  return router;
};

describe("Setup account step", () => {
  beforeEach(() => {
    window.localStorage.clear();
    setAuth({
      permissions: ["libraries:write"],
      user: { id: 1, username: "admin" },
    });
  });

  afterEach(() => window.localStorage.clear());

  it("sends an admin partway through Setup on to the library step", () => {
    saveSetupDraft({ ...newSetupDraft(1), name: "Books" });

    const router = renderAccountStep();

    expect(router.state.location.pathname).toBe("/setup/library");
    expect(screen.getByText("Library step")).toBeInTheDocument();
  });

  it("sends anyone else into the app", () => {
    // A draft another user on this browser left behind isn't theirs.
    saveSetupDraft({ ...newSetupDraft(2), name: "Theirs" });

    const router = renderAccountStep();

    expect(router.state.location.pathname).toBe("/");
  });
});
