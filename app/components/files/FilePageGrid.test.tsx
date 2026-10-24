import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { File } from "@/types";

import FilePageGrid from "./FilePageGrid";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

// Each test opens its own file: the grid remembers an opened grid by
// history entry and file, and a memory router's first entry is always
// keyed "default".
let nextId = 42;
const comic = (pageCount: number) =>
  ({
    id: nextId++,
    book_id: 7,
    library_id: 1,
    file_type: "cbz",
    file_role: "main",
    page_count: pageCount,
    updated_at: "2024-01-01T00:00:00Z",
  }) as File;

// jsdom does no layout, so the grid is as wide as 11 tiles (96px, 12px
// apart): two rows hold 22.
beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1176);
});
afterEach(() => vi.restoreAllMocks());

const renderGrid = (file: File) => {
  const router = createMemoryRouter(
    [
      { path: "/file", element: <FilePageGrid file={file} /> },
      { path: "*", element: <p>Reader</p> },
    ],
    { initialEntries: ["/file"] },
  );
  render(<RouterProvider router={router} />);
  return router;
};

const user = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

describe("FilePageGrid", () => {
  it("links each page to the reader on that page", () => {
    renderGrid(comic(3));

    expect(screen.getByRole("link", { name: "Page 2" })).toHaveAttribute(
      "href",
      expect.stringMatching(
        /^\/libraries\/1\/books\/7\/files\/\d+\/read\?page=1$/,
      ),
    );
  });

  it("shows every page with no Show all when they fit in two rows", () => {
    renderGrid(comic(22));

    expect(screen.getAllByRole("link")).toHaveLength(22);
    expect(
      screen.queryByRole("button", { name: /Show all/ }),
    ).not.toBeInTheDocument();
  });

  it("opens two rows tall and lays out every page on Show all", async () => {
    renderGrid(comic(194));

    // Exactly two rows: nothing clipped for the keyboard to land on.
    expect(screen.getAllByRole("link")).toHaveLength(22);
    await user().click(
      screen.getByRole("button", { name: "Show all 194 pages" }),
    );
    expect(screen.getAllByRole("link")).toHaveLength(194);
    expect(screen.getByRole("button", { name: "Show fewer" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("is still open when the reader comes back to the page", async () => {
    const router = renderGrid(comic(194));
    await user().click(
      screen.getByRole("button", { name: "Show all 194 pages" }),
    );

    await act(() => router.navigate("/libraries/1/books/7/files/42/read"));
    await act(() => router.navigate(-1));

    expect(screen.getAllByRole("link")).toHaveLength(194);
  });

  it("starts closed on a new visit to the page", async () => {
    const router = renderGrid(comic(194));
    await user().click(
      screen.getByRole("button", { name: "Show all 194 pages" }),
    );

    await act(() => router.navigate("/elsewhere"));
    await act(() => router.navigate("/file"));

    expect(screen.getAllByRole("link")).toHaveLength(22);
  });

  it("loads a PDF's pages one at a time, in order", () => {
    const { container } = render(
      <RouterProviderFor file={{ ...comic(5), file_type: "pdf" } as File} />,
    );
    const images = () => container.querySelectorAll("img");

    expect(images()).toHaveLength(1);
    fireEvent.load(images()[0]);
    expect(images()).toHaveLength(2);
    // A page that fails to load lets the next one go too.
    fireEvent.error(images()[1]);
    expect(images()).toHaveLength(3);
  });

  it("loads a few CBZ pages at once", () => {
    const { container } = render(<RouterProviderFor file={comic(10)} />);

    expect(container.querySelectorAll("img")).toHaveLength(4);
  });
});

const RouterProviderFor = ({ file }: { file: File }) => (
  <RouterProvider
    router={createMemoryRouter(
      [{ path: "/file", element: <FilePageGrid file={file} /> }],
      { initialEntries: ["/file"] },
    )}
  />
);
