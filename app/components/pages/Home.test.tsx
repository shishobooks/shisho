import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import Home from "./Home";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
  window.matchMedia = vi.fn(() => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/components/library/SelectionToolbar", () => ({
  SelectionToolbar: () => null,
}));

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const renderHome = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/libraries/1"]}>
        <Routes>
          <Route element={<Home />} path="/libraries/:libraryId" />
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

describe("Home permission gating", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("requests no books, genres, or tags for a role without Books Read", async () => {
    setAuth({ permissions: ["shares:write"] });
    const request = vi
      .spyOn(API, "request")
      .mockResolvedValue({ items: [], total: 0 });

    renderHome();
    await flush();

    const paths = requestedPaths(request);
    expect(paths).not.toContain("/books");
    expect(paths).not.toContain("/genres");
    expect(paths).not.toContain("/tags");
  });

  it("requests books for a role with Books Read", async () => {
    setAuth({ permissions: ["books:read"] });
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, path) =>
        path === "/user/libraries" || path.endsWith("/languages")
          ? []
          : { items: [], total: 0 },
      );

    renderHome();
    await flush();

    expect(requestedPaths(request)).toContain("/books");
  });
});
