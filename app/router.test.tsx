import { render, screen } from "@testing-library/react";
import { createMemoryRouter, Outlet, RouterProvider } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";
import type { Permission } from "@/utils/permissions";

import { routes } from "./router";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

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
    setAuth();
  });

  describe("/settings index", () => {
    it("opens the first permitted settings page for a role without Config Read", async () => {
      setAuth({ permissions: ["books:read", "libraries:read"] });
      const router = renderAt("/settings");

      expect(
        await screen.findByText("libraries settings page"),
      ).toBeInTheDocument();
      expect(router.state.location.pathname).toBe("/settings/libraries");
    });

    it("opens server settings for a role with Config Read", async () => {
      setAuth({ permissions: ["config:read", "libraries:read"] });
      const router = renderAt("/settings");

      expect(
        await screen.findByText("server settings page"),
      ).toBeInTheDocument();
      expect(router.state.location.pathname).toBe("/settings/server");
    });

    it("denies access when no settings page is permitted", async () => {
      setAuth({ permissions: ["books:read"] });
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
      setAuth({ permissions: ["shares:write"] });
      renderAt(path);
      expect(await screen.findByText("Access Denied")).toBeInTheDocument();

      setAuth({ permissions: ["books:read"] });
      renderAt(path);
      expect(await screen.findByText(page)).toBeInTheDocument();
    });

    it.each<[string, Permission, string]>([
      ["/libraries/1/series", "series:read", "series list page"],
      ["/libraries/1/people", "people:read", "people list page"],
    ])("requires its own Read permission for %s", async (path, perm, page) => {
      setAuth({ permissions: ["books:read"] });
      renderAt(path);
      expect(await screen.findByText("Access Denied")).toBeInTheDocument();

      setAuth({ permissions: ["books:read", perm] });
      renderAt(path);
      expect(await screen.findByText(page)).toBeInTheDocument();
    });

    it("requires Libraries Read and Write for library settings", async () => {
      setAuth({ permissions: ["books:read", "libraries:write"] });
      renderAt("/libraries/1/settings");
      expect(await screen.findByText("Access Denied")).toBeInTheDocument();

      setAuth({
        permissions: ["books:read", "libraries:read", "libraries:write"],
      });
      renderAt("/libraries/1/settings");
      expect(
        await screen.findByText("library settings page"),
      ).toBeInTheDocument();
    });
  });
});
