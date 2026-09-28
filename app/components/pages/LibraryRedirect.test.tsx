import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import LibraryRedirect from "./LibraryRedirect";

const auth = vi.hoisted(() => ({
  permissions: new Set<string>(),
  libraryAccess: null as number[] | null,
}));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    user: { library_access: auth.libraryAccess },
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
  }),
}));

vi.mock("@/components/library/TopNav", () => ({
  default: () => <div data-testid="top-nav" />,
}));

const renderRedirect = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route element={<LibraryRedirect />} path="/" />
          <Route
            element={<div>library page</div>}
            path="/libraries/:libraryId"
          />
          <Route element={<div>lists page</div>} path="/lists" />
          <Route
            element={<div>library settings page</div>}
            path="/settings/libraries"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const librariesRequests = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.filter((call: unknown[]) => call[1] === "/libraries");

describe("LibraryRedirect", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    auth.permissions = new Set(["books:read", "libraries:read"]);
    auth.libraryAccess = null;
  });

  it("redirects a role with Libraries Read to the first listed library", async () => {
    const request = vi
      .spyOn(API, "request")
      .mockResolvedValue({ items: [{ id: 4, name: "Fiction" }], total: 1 });

    renderRedirect();

    expect(await screen.findByText("library page")).toBeInTheDocument();
    expect(librariesRequests(request)).toHaveLength(1);
  });

  it("sends a role with Libraries Read and no libraries to library settings", async () => {
    vi.spyOn(API, "request").mockResolvedValue({ items: [], total: 0 });

    renderRedirect();

    expect(
      await screen.findByText("library settings page"),
    ).toBeInTheDocument();
  });

  it("requests no libraries for a Books-Read-only role and opens its first accessible library", async () => {
    auth.permissions = new Set(["books:read"]);
    auth.libraryAccess = [7, 3];
    const request = vi.spyOn(API, "request").mockResolvedValue({});

    renderRedirect();

    expect(await screen.findByText("library page")).toBeInTheDocument();
    expect(librariesRequests(request)).toHaveLength(0);
    expect(screen.queryByText("Error Loading Libraries")).toBeNull();
  });

  it("sends a role without Libraries Read and with access to all libraries to lists", async () => {
    auth.permissions = new Set(["books:read"]);
    auth.libraryAccess = null;
    const request = vi.spyOn(API, "request").mockResolvedValue({});

    renderRedirect();

    expect(await screen.findByText("lists page")).toBeInTheDocument();
    expect(librariesRequests(request)).toHaveLength(0);
  });

  it("sends a role without Libraries Read and with no library access to lists", async () => {
    auth.permissions = new Set(["books:read"]);
    auth.libraryAccess = [];
    const request = vi.spyOn(API, "request").mockResolvedValue({});

    renderRedirect();

    expect(await screen.findByText("lists page")).toBeInTheDocument();
    expect(librariesRequests(request)).toHaveLength(0);
  });

  it("sends a shares-only role to lists rather than a library it cannot browse", async () => {
    auth.permissions = new Set(["shares:read", "shares:write"]);
    auth.libraryAccess = [7];
    const request = vi.spyOn(API, "request").mockResolvedValue({});

    renderRedirect();

    expect(await screen.findByText("lists page")).toBeInTheDocument();
    expect(librariesRequests(request)).toHaveLength(0);
  });
});
