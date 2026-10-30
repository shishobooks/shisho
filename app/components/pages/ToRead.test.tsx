import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { toast, Toaster } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AuthProvider } from "@/components/contexts/Auth";
import { MobileNavProvider } from "@/contexts/MobileNav";

import ToRead from "./ToRead";

// Stand in for the drag interaction: the queue's titles in order, and one
// button that moves the last book to the top and, like the real list,
// consumes the rejection.
vi.mock("@/components/library/DraggableBookList", () => ({
  DraggableBookList: ({
    books,
    onReorder,
  }: {
    books: { id: number; title: string }[];
    onReorder: (ids: number[]) => Promise<unknown>;
  }) => (
    <div>
      <ol aria-label="Queue">
        {books.map((book) => (
          <li key={book.id}>{book.title}</li>
        ))}
      </ol>
      <button
        onClick={() => {
          const ids = books.map((book) => book.id);
          onReorder([ids[ids.length - 1], ...ids.slice(0, -1)]).catch(
            () => undefined,
          );
        }}
      >
        Move last to top
      </button>
    </div>
  ),
}));

afterEach(() => {
  toast.dismiss();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const queued = [
  { id: 1, title: "Dune", library_id: 1, files: [] },
  { id: 2, title: "Emma", library_id: 1, files: [] },
  { id: 3, title: "Ubik", library_id: 1, files: [] },
];

const renderToRead = ({
  reorder = () => new Response(null, { status: 204 }),
  demoMode = false,
  books = queued,
}: {
  reorder?: () => Response;
  demoMode?: boolean;
  books?: unknown[];
} = {}) => {
  vi.stubGlobal("__APP_VERSION__", "test");
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (input, init) => {
      const path = String(input).split("?")[0];
      if (init?.method === "PUT" && path === "/api/reading/to-read/order")
        return reorder();
      if (path === "/api/auth/status")
        return Response.json({ needs_setup: false, demo_mode: demoMode });
      if (path === "/api/auth/me")
        return Response.json({
          id: 1,
          username: "reader",
          permissions: ["books:read"],
          role_name: "viewer",
        });
      if (path === "/api/books")
        return Response.json({ items: books, total: books.length });
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
          <MemoryRouter initialEntries={["/to-read"]}>
            <Routes>
              <Route element={<ToRead />} path="/to-read" />
            </Routes>
            <Toaster />
          </MemoryRouter>
        </MobileNavProvider>
      </AuthProvider>
    </QueryClientProvider>,
  );
  return {
    fetchMock,
    user: userEvent.setup({ advanceTimers: vi.advanceTimersByTime }),
  };
};

// The book listings requested, as their query params. The library picker
// also asks for To Read's count.
const booksRequests = () =>
  vi
    .mocked(globalThis.fetch)
    .mock.calls.map(([input]) => String(input))
    .filter((url) => url.startsWith("/api/books?"))
    .map((url) => Object.fromEntries(new URL(url, "http://x").searchParams));

describe("ToRead", () => {
  it("lists the whole queue in order", async () => {
    renderToRead();

    const queue = await screen.findByRole("list", { name: "Queue" });
    expect(queue.textContent).toBe("DuneEmmaUbik");
    expect(booksRequests()).toContainEqual({
      to_read: "true",
      limit: "100",
      offset: "0",
    });
  });

  it("saves a reorder", async () => {
    const { fetchMock, user } = renderToRead();

    await user.click(
      await screen.findByRole("button", { name: "Move last to top" }),
    );

    await waitFor(() => {
      const put = fetchMock.mock.calls.find(
        ([, init]) => init?.method === "PUT",
      );
      expect(put?.[0]).toBe("/api/reading/to-read/order");
      expect(JSON.parse(String(put?.[1]?.body))).toEqual({
        book_ids: [3, 1, 2],
      });
    });
  });

  it("reports a failed reorder with a toast", async () => {
    const { user } = renderToRead({
      reorder: () =>
        Response.json(
          { error: { code: "internal", message: "Could not save order" } },
          { status: 500 },
        ),
    });

    await user.click(
      await screen.findByRole("button", { name: "Move last to top" }),
    );

    expect(await screen.findByText("Could not save order")).toBeInTheDocument();
  });

  it("shows the queue without dragging in Demo Mode", async () => {
    renderToRead({ demoMode: true });

    expect(await screen.findByText("Ubik")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Move last to top" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText("This action is unavailable in the demo."),
    ).toBeInTheDocument();
  });

  it("explains how to add books when the queue is empty", async () => {
    renderToRead({ books: [] });

    expect(
      await screen.findByText(/Nothing on To Read yet/),
    ).toBeInTheDocument();
  });
});
