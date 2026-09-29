import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import GlobalSearch from "./GlobalSearch";

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    demoMode: false,
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
  }),
}));

const renderSearch = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/libraries/1"]}>
        <Routes>
          <Route element={<GlobalSearch />} path="/libraries/:libraryId" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const searchRequests = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.filter((call: unknown[]) => call[1] === "/search");

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 350));
  });

describe("GlobalSearch", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("offers no search to a role without Books Read", async () => {
    auth.permissions = new Set(["shares:write"]);
    const request = vi.spyOn(API, "request").mockResolvedValue([]);

    renderSearch();
    await flush();

    expect(screen.queryByPlaceholderText("Search library...")).toBeNull();
    expect(searchRequests(request)).toHaveLength(0);
  });

  it("searches the library for a role with Books Read", async () => {
    auth.permissions = new Set(["books:read"]);
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, path) =>
        path === "/search" ? { books: [], series: [], people: [] } : [],
      );
    const user = userEvent.setup();

    renderSearch();
    await user.type(screen.getByPlaceholderText("Search library..."), "dune");
    await flush();

    expect(searchRequests(request)).toHaveLength(1);
  });
});
