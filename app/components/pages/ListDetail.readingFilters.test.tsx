import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AuthProvider } from "@/components/contexts/Auth";
import { MobileNavProvider } from "@/contexts/MobileNav";

import ListDetail from "./ListDetail";

vi.mock("@/components/library/DraggableBookList", () => ({
  DraggableBookList: () => <p>Draggable list</p>,
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const baseList = {
  id: 5,
  name: "Picks",
  description: "",
  is_ordered: false,
  default_sort: "added_at_desc",
  permission: "owner",
  book_count: 2,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

const listBooks = [
  {
    id: 1,
    book_id: 1,
    book: { id: 1, title: "Dune", library_id: 1, files: [] },
  },
  {
    id: 2,
    book_id: 2,
    book: { id: 2, title: "Emma", library_id: 1, files: [] },
  },
];

const renderList = (entry: string, listOverrides = {}) => {
  vi.stubGlobal("__APP_VERSION__", "test");
  // Matching renders the Radix Sheet, which opens synchronously in jsdom.
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  const bookRequests: URLSearchParams[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const [path, search] = String(input).split("?");
    if (path === "/api/auth/status")
      return Response.json({ needs_setup: false, demo_mode: false });
    if (path === "/api/auth/me")
      return Response.json({
        id: 1,
        username: "reader",
        permissions: ["books:read"],
        role_name: "viewer",
      });
    if (path === "/api/lists/5")
      return Response.json({ ...baseList, ...listOverrides });
    if (path === "/api/lists/5/books") {
      bookRequests.push(new URLSearchParams(search));
      return Response.json({ items: listBooks, total: listBooks.length });
    }
    if (path === "/api/user/libraries")
      return Response.json([
        { id: 1, name: "Books", cover_aspect_ratio: "book" },
      ]);
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
          </MemoryRouter>
        </MobileNavProvider>
      </AuthProvider>
    </QueryClientProvider>,
  );
  return {
    bookRequests,
    user: userEvent.setup({ advanceTimers: vi.advanceTimersByTime }),
  };
};

describe("ListDetail reading filters", () => {
  it("filters the List's books by status and shows what it filters by", async () => {
    const { bookRequests, user } = renderList("/lists/5");

    await user.click(await screen.findByRole("button", { name: "Filter" }));
    await user.click(screen.getByRole("button", { name: "Did Not Finish" }));

    await waitFor(() =>
      expect(
        bookRequests[bookRequests.length - 1]?.getAll("reading_status"),
      ).toEqual(["did_not_finish"]),
    );
    await user.click(screen.getByRole("button", { name: "Done" }));
    expect(
      screen.getByRole("button", { name: "Remove Did Not Finish" }),
    ).toBeInTheDocument();
  });

  it("reads the filters from the URL and offers the reading sorts", async () => {
    const { bookRequests, user } = renderList(
      "/lists/5?reading_status=read&rating_min=4",
    );

    await waitFor(() => expect(bookRequests.length).toBeGreaterThan(0));
    expect(bookRequests[0].getAll("reading_status")).toEqual(["read"]);
    expect(bookRequests[0].get("rating_min")).toBe("4");
    expect(
      await screen.findByRole("button", { name: "Remove 4+ stars" }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: "Sort by:" }));
    expect(
      screen.getByRole("option", { name: "Recently Finished" }),
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole("option", { name: "Rating (high to low)" }),
    );
    await waitFor(() =>
      expect(bookRequests[bookRequests.length - 1]?.get("sort")).toBe(
        "rating_desc",
      ),
    );
  });

  it("doesn't offer reordering while filtered, since only some books show", async () => {
    renderList("/lists/5?reading_status=read", { is_ordered: true });

    expect(
      await screen.findByRole("button", { name: "Remove Read" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Draggable list")).not.toBeInTheDocument();
    expect(screen.queryByText("(drag to reorder)")).not.toBeInTheDocument();
  });
});
