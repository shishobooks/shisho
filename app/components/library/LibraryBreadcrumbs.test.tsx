import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import LibraryBreadcrumbs from "./LibraryBreadcrumbs";

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
  }),
}));

const renderCrumbs = (libraryName?: string) =>
  render(
    <MemoryRouter>
      <LibraryBreadcrumbs
        items={[{ label: "Test Book" }]}
        libraryId="1"
        libraryName={libraryName}
      />
    </MemoryRouter>,
  );

describe("LibraryBreadcrumbs", () => {
  beforeEach(() => {
    auth.permissions = new Set(["books:read", "libraries:read"]);
  });

  it("links the library by name", () => {
    renderCrumbs("Fiction");

    expect(screen.getByRole("link", { name: "Fiction" })).toHaveAttribute(
      "href",
      "/libraries/1",
    );
  });

  it("shows a placeholder while the name loads for a role that can read libraries", () => {
    renderCrumbs();

    expect(screen.getByRole("link", { name: "Library" })).toBeInTheDocument();
  });

  it("shows a known library name to a role without Libraries Read", () => {
    auth.permissions = new Set(["books:read"]);
    renderCrumbs("Fiction");

    expect(screen.getByRole("link", { name: "Fiction" })).toBeInTheDocument();
  });

  it("drops the library crumb when a role without Libraries Read has no name for it", () => {
    auth.permissions = new Set(["books:read"]);
    renderCrumbs();

    expect(screen.queryByText("Library")).not.toBeInTheDocument();
    expect(screen.queryByText("›")).not.toBeInTheDocument();
    expect(screen.getByText("Test Book")).toBeInTheDocument();
  });
});
