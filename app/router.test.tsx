import { render, screen } from "@testing-library/react";
import { createMemoryRouter, Outlet, RouterProvider } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { routes } from "./router";

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    user: { id: 1, must_change_password: false, library_access: null },
    isAuthenticated: true,
    isLoading: false,
    needsSetup: false,
    demoMode: false,
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
    canWrite: (resource: string) => auth.permissions.has(`${resource}:write`),
    hasLibraryAccess: () => true,
  }),
}));

// Stand in for the layouts and pages so only the route guards run.
vi.mock("@/components/pages/Root", () => ({ default: () => <Outlet /> }));
vi.mock("@/components/pages/AdminLayout", () => ({
  default: () => <Outlet />,
}));
vi.mock("@/components/pages/AdminSettings", () => ({
  default: () => <div>server settings page</div>,
}));
vi.mock("@/components/pages/AdminLibraries", () => ({
  default: () => <div>libraries settings page</div>,
}));
vi.mock("@/components/pages/AdminUsers", () => ({
  default: () => <div>users settings page</div>,
}));
vi.mock("@/components/pages/Home", () => ({
  default: () => <div>library home page</div>,
}));
vi.mock("@/components/pages/SeriesList", () => ({
  default: () => <div>series list page</div>,
}));
vi.mock("@/components/pages/PersonList", () => ({
  default: () => <div>people list page</div>,
}));
vi.mock("@/components/pages/GenresList", () => ({
  default: () => <div>genres list page</div>,
}));
vi.mock("@/components/pages/BookDetail", () => ({
  default: () => <div>book detail page</div>,
}));
vi.mock("@/components/pages/LibrarySettings", () => ({
  default: () => <div>library settings page</div>,
}));

const renderAt = (path: string) => {
  const router = createMemoryRouter(routes, { initialEntries: [path] });
  render(<RouterProvider router={router} />);
  return router;
};

describe("router permission guards", () => {
  beforeEach(() => {
    auth.permissions = new Set();
  });

  describe("/settings index", () => {
    it("opens the first permitted settings page for a role without Config Read", async () => {
      auth.permissions = new Set(["books:read", "libraries:read"]);
      const router = renderAt("/settings");

      expect(
        await screen.findByText("libraries settings page"),
      ).toBeInTheDocument();
      expect(router.state.location.pathname).toBe("/settings/libraries");
    });

    it("opens server settings for a role with Config Read", async () => {
      auth.permissions = new Set(["config:read", "libraries:read"]);
      const router = renderAt("/settings");

      expect(
        await screen.findByText("server settings page"),
      ).toBeInTheDocument();
      expect(router.state.location.pathname).toBe("/settings/server");
    });

    it("denies access when no settings page is permitted", async () => {
      auth.permissions = new Set(["books:read"]);
      renderAt("/settings");

      expect(await screen.findByText("Access Denied")).toBeInTheDocument();
    });
  });

  describe("library pages", () => {
    it.each([
      ["/libraries/1", "library home page"],
      ["/libraries/1/genres", "genres list page"],
      ["/libraries/1/books/2", "book detail page"],
    ])("requires Books Read for %s", async (path, page) => {
      auth.permissions = new Set(["shares:write"]);
      renderAt(path);
      expect(await screen.findByText("Access Denied")).toBeInTheDocument();

      auth.permissions = new Set(["books:read"]);
      renderAt(path);
      expect(await screen.findByText(page)).toBeInTheDocument();
    });

    it.each([
      ["/libraries/1/series", "series:read", "series list page"],
      ["/libraries/1/people", "people:read", "people list page"],
    ])("requires its own Read permission for %s", async (path, perm, page) => {
      auth.permissions = new Set(["books:read"]);
      renderAt(path);
      expect(await screen.findByText("Access Denied")).toBeInTheDocument();

      auth.permissions = new Set(["books:read", perm]);
      renderAt(path);
      expect(await screen.findByText(page)).toBeInTheDocument();
    });

    it("requires Libraries Read and Write for library settings", async () => {
      auth.permissions = new Set(["books:read", "libraries:write"]);
      renderAt("/libraries/1/settings");
      expect(await screen.findByText("Access Denied")).toBeInTheDocument();

      auth.permissions = new Set([
        "books:read",
        "libraries:read",
        "libraries:write",
      ]);
      renderAt("/libraries/1/settings");
      expect(
        await screen.findByText("library settings page"),
      ).toBeInTheDocument();
    });
  });
});
