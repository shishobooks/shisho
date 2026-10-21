import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { toast, Toaster } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AuthProvider } from "@/components/contexts/Auth";
import { MobileNavProvider } from "@/contexts/MobileNav";

import ListDetail from "./ListDetail";

// Stand in for the drag interaction: one button that asks to reorder and,
// like the real list, consumes the rejection.
vi.mock("@/components/library/DraggableBookList", () => ({
  DraggableBookList: ({
    onReorder,
  }: {
    onReorder: (ids: number[]) => Promise<unknown>;
  }) => (
    <button onClick={() => onReorder([2, 1]).catch(() => undefined)}>
      Reorder
    </button>
  ),
}));

const location = { search: "" };
const LocationProbe = () => {
  const { search } = useLocation();
  useEffect(() => {
    location.search = search;
  }, [search]);
  return null;
};

afterEach(() => {
  toast.dismiss();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const list = {
  id: 5,
  name: "Queue",
  description: "",
  is_ordered: true,
  permission: "owner",
  book_count: 2,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

const renderWithReorderFailure = (
  failure: Response,
  listOverrides: Partial<typeof list> = {},
  entry = "/lists/5",
  {
    libraries = [],
    listBooks = [],
  }: { libraries?: unknown[]; listBooks?: unknown[] } = {},
) => {
  vi.stubGlobal("__APP_VERSION__", "test");
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const path = String(input).split("?")[0];
    if (init?.method === "PATCH") return failure.clone();
    if (path === "/api/auth/status")
      return Response.json({ needs_setup: false, demo_mode: true });
    if (path === "/api/auth/me")
      return Response.json({
        id: 1,
        username: "reader",
        permissions: ["books:read"],
        role_name: "viewer",
      });
    if (path === "/api/lists/5")
      return Response.json({ ...list, ...listOverrides });
    if (path.endsWith("/shares")) return Response.json([]);
    if (path === "/api/user/libraries") return Response.json(libraries);
    if (path === "/api/lists/5/books")
      return Response.json({ items: listBooks, total: listBooks.length });
    if (path === "/api/settings/user")
      return Response.json({ gallery_size: "m" });
    return Response.json({ items: [], total: 0 });
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AuthProvider>
        <MobileNavProvider>
          <MemoryRouter initialEntries={[entry]}>
            <Routes>
              <Route element={<ListDetail />} path="/lists/:id" />
            </Routes>
            <LocationProbe />
            <Toaster />
          </MemoryRouter>
        </MobileNavProvider>
      </AuthProvider>
    </QueryClientProvider>,
  );
  return userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
};

describe("ListDetail reorder failures", () => {
  it("reports an ordinary failure with a toast", async () => {
    const user = renderWithReorderFailure(
      Response.json(
        { error: { code: "internal", message: "Could not save order" } },
        { status: 500 },
      ),
    );

    await user.click(await screen.findByRole("button", { name: "Reorder" }));

    expect(await screen.findByText("Could not save order")).toBeInTheDocument();
  });

  it("shows only the Demo Mode toast for a demo rejection", async () => {
    const user = renderWithReorderFailure(
      Response.json(
        {
          error: {
            code: "demo_mode",
            message: "This action is unavailable in the demo.",
          },
        },
        { status: 403 },
      ),
    );

    await user.click(await screen.findByRole("button", { name: "Reorder" }));

    await waitFor(() =>
      expect(
        screen.getAllByText("This action is unavailable in the demo."),
      ).toHaveLength(1),
    );
  });

  it("loads a whole ordered list to reorder, whatever the window fits", async () => {
    // 60 books is more than a window-sized page (24 at the fallback width).
    renderWithReorderFailure(Response.json({}), { book_count: 60 });

    expect(
      await screen.findByRole("button", { name: "Reorder" }),
    ).toBeInTheDocument();
    const booksRequest = vi
      .mocked(globalThis.fetch)
      .mock.calls.map(([input]) => String(input))
      .find((url) => url.startsWith("/api/lists/5/books"));
    expect(new URL(booksRequest!, "http://x").searchParams.get("limit")).toBe(
      "100",
    );
  });

  it("drops a page position from the URL when it loads the whole list", async () => {
    renderWithReorderFailure(Response.json({}), {}, "/lists/5?start=48");

    await screen.findByRole("button", { name: "Reorder" });
    await waitFor(() => expect(location.search).toBe(""));
  });
});

describe("ListDetail covers", () => {
  it("shapes each book's cover by its own library's cover aspect ratio", async () => {
    // No reorder happens here, so the failure response never gets served.
    renderWithReorderFailure(
      Response.json({}),
      { is_ordered: false, permission: "viewer", book_count: 1 },
      "/lists/5",
      {
        libraries: [{ id: 2, name: "Audio", cover_aspect_ratio: "audiobook" }],
        listBooks: [
          {
            id: 1,
            list_id: 5,
            book_id: 9,
            sort_order: 1,
            book: { id: 9, title: "Dune", library_id: 2, files: [] },
          },
        ],
      },
    );

    await screen.findByText("Dune");
    await waitFor(() =>
      expect(document.querySelector(".aspect-square")).not.toBeNull(),
    );
  });
});
