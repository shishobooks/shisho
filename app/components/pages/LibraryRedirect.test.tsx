import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import LibraryRedirect from "./LibraryRedirect";

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
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

const requestedPaths = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.map((call: unknown[]) => call[1]);

describe("LibraryRedirect", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    auth.permissions = new Set(["books:read"]);
  });

  it("opens the first accessible library for a role with Books Read, without Libraries Read", async () => {
    const request = vi
      .spyOn(API, "request")
      .mockResolvedValue([{ id: 4, name: "Fiction" }]);

    renderRedirect();

    expect(await screen.findByText("library page")).toBeInTheDocument();
    expect(requestedPaths(request)).toEqual(["/user/libraries"]);
  });

  it("sends a role with Libraries Read and no libraries to library settings", async () => {
    auth.permissions = new Set(["books:read", "libraries:read"]);
    vi.spyOn(API, "request").mockResolvedValue([]);

    renderRedirect();

    expect(
      await screen.findByText("library settings page"),
    ).toBeInTheDocument();
  });

  it("sends a role without Libraries Read and no libraries to lists", async () => {
    vi.spyOn(API, "request").mockResolvedValue([]);

    renderRedirect();

    expect(await screen.findByText("lists page")).toBeInTheDocument();
  });

  it("sends a role without Books Read to lists without requesting libraries", async () => {
    auth.permissions = new Set([
      "shares:read",
      "shares:write",
      "libraries:read",
    ]);
    const request = vi.spyOn(API, "request").mockResolvedValue([]);

    renderRedirect();

    expect(await screen.findByText("lists page")).toBeInTheDocument();
    expect(requestedPaths(request)).toHaveLength(0);
  });
});
