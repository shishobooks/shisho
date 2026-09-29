import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";

import ListDetail from "./ListDetail";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
  window.matchMedia = vi.fn(() => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

const auth = vi.hoisted(() => ({
  permissions: new Set<string>(),
  demoMode: false,
}));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    user: { id: 1, username: "owner", library_access: null },
    demoMode: auth.demoMode,
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
    canWrite: (resource: string) => auth.permissions.has(`${resource}:write`),
    hasLibraryAccess: () => true,
  }),
}));

vi.mock("@/components/library/TopNav", () => ({ default: () => null }));

const list = {
  id: 5,
  user_id: 1,
  name: "Queue",
  description: "",
  is_ordered: false,
  permission: "owner",
  book_count: 1,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

const listBook = {
  id: 9,
  list_id: 5,
  book_id: 3,
  book: {
    id: 3,
    library_id: 1,
    title: "Dune",
    cover_cache_key: "abc",
    authors: [],
    files: [],
  },
};

const respond = async (_method: string, path: string) => {
  if (path === "/lists/5") return list;
  if (path === "/lists/5/books") return { items: [listBook], total: 1 };
  if (path === "/lists/5/shares") return [];
  if (path === "/users/directory") return [{ id: 2, username: "friend" }];
  if (path === "/users") return { items: [], total: 0 };
  if (path === "/settings/user") return { gallery_size: "m" };
  return { items: [], total: 0 };
};

const renderList = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/lists/5"]}>
        <Routes>
          <Route element={<ListDetail />} path="/lists/:id" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const requestedPaths = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.map((call: unknown[]) => call[1]);

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 50));
  });

const bookCovers = () =>
  Array.from(document.querySelectorAll("img")).filter((img) =>
    img.getAttribute("src")?.startsWith("/api/books/"),
  );

describe("ListDetail permission gating", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    auth.demoMode = false;
  });

  it("lets an owner without Users Read share through the user directory", async () => {
    auth.permissions = new Set(["books:read"]);
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(respond as typeof API.request);
    const user = userEvent.setup();

    renderList();
    await user.click(await screen.findByRole("button", { name: "Share" }));
    await flush();

    const paths = requestedPaths(request);
    expect(paths).toContain("/users/directory");
    expect(paths).toContain("/lists/5/shares");
    expect(paths).not.toContain("/users");
    expect(screen.queryByText("No users available")).toBeNull();
  });

  it("does not ask for the user directory in Demo Mode, where it is absent", async () => {
    auth.permissions = new Set(["books:read"]);
    auth.demoMode = true;
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(respond as typeof API.request);
    const user = userEvent.setup();

    renderList();
    await user.click(await screen.findByRole("button", { name: "Share" }));
    await flush();

    const paths = requestedPaths(request);
    expect(paths).not.toContain("/users/directory");
    expect(paths).not.toContain("/users");
    expect(
      screen.getByText(/Sharing with other users is unavailable in the demo/),
    ).toBeInTheDocument();
  });

  it("requests no books or covers for a role without Books Read", async () => {
    auth.permissions = new Set(["shares:read"]);
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(respond as typeof API.request);

    renderList();
    expect(await screen.findByText("Queue")).toBeInTheDocument();
    await flush();

    expect(requestedPaths(request)).not.toContain("/lists/5/books");
    expect(bookCovers()).toHaveLength(0);
    expect(screen.getByText(/Your role cannot view books/)).toBeInTheDocument();
  });

  it("shows the books and their covers for a role with Books Read", async () => {
    auth.permissions = new Set(["books:read"]);
    vi.spyOn(API, "request").mockImplementation(respond as typeof API.request);

    renderList();

    expect(await screen.findByText("Dune")).toBeInTheDocument();
    expect(bookCovers().length).toBeGreaterThan(0);
  });
});
