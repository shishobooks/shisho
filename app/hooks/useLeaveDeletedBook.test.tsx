import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import {
  createMemoryRouter,
  Outlet,
  RouterProvider,
  useLocation,
} from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { NavigationHistoryProvider } from "@/contexts/NavigationHistory";
import type { RemovedEntities } from "@/types";

import { useLeaveDeletedBook } from "./useLeaveDeletedBook";

const book = { id: 7, library_id: 2 };

const nothingRemoved: RemovedEntities = {
  series_ids: [],
  person_ids: [],
  genre_ids: [],
  tag_ids: [],
  publisher_ids: [],
};
let removed: RemovedEntities = nothingRemoved;
// The latest callback, kept so a test can call it after the book page has
// unmounted, as a slow delete's response would.
const captured: { leave?: () => void } = {};

const DeleteButton = () => {
  const leaveDeletedBook = useLeaveDeletedBook();
  useEffect(() => {
    captured.leave = () => leaveDeletedBook(book, removed);
  });
  return (
    <button onClick={() => leaveDeletedBook(book, removed)} type="button">
      Delete
    </button>
  );
};

const Where = () => {
  const location = useLocation();
  return <p>{location.pathname + location.search}</p>;
};

const setup = (
  initialEntries: string[],
  {
    initialIndex,
    withProvider = true,
  }: { initialIndex?: number; withProvider?: boolean } = {},
) => {
  const router = createMemoryRouter(
    [
      {
        path: "/",
        element: withProvider ? (
          <NavigationHistoryProvider>
            <Outlet />
          </NavigationHistoryProvider>
        ) : (
          <Outlet />
        ),
        children: [
          { path: "libraries/:libraryId", element: <Where /> },
          { path: "libraries/:libraryId/series/:id", element: <Where /> },
          { path: "libraries/:libraryId/people/:id", element: <Where /> },
          { path: "libraries/:libraryId/books/:id", element: <DeleteButton /> },
          {
            path: "libraries/:libraryId/books/:bookId/files/:fileId",
            element: <DeleteButton />,
          },
        ],
      },
    ],
    { initialEntries, initialIndex },
  );
  render(<RouterProvider router={router} />);
  return router;
};

// One act per navigation, so the provider renders and records each, as it
// does for a reader's clicks.
const visit = async (router: ReturnType<typeof setup>, ...paths: string[]) => {
  for (const path of paths) await act(() => router.navigate(path));
};

const at = (router: ReturnType<typeof setup>) =>
  router.state.location.pathname + router.state.location.search;

const clickDelete = async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  await user.click(screen.getByRole("button", { name: "Delete" }));
};

describe("useLeaveDeletedBook", () => {
  beforeEach(() => {
    removed = nothingRemoved;
    captured.leave = undefined;
  });

  it("returns to the page the book was opened from", async () => {
    const router = setup(["/libraries/2/series/4"]);
    await visit(router, "/libraries/2?page=3", "/libraries/2/books/7");

    await clickDelete();

    expect(at(router)).toBe("/libraries/2?page=3");
  });

  it("walks back past the deleted book's own pages", async () => {
    const router = setup(["/libraries/2?page=3"]);
    await visit(router, "/libraries/2/books/7", "/libraries/2/books/7/files/9");

    await clickDelete();

    expect(at(router)).toBe("/libraries/2?page=3");
  });

  // Moving a book to another library changes its URL, and the old one is
  // still the deleted book's page.
  it("walks back past the book's pages from before it moved library", async () => {
    const router = setup(["/libraries/2?page=3"]);
    await visit(router, "/libraries/1/books/7", "/libraries/2/books/7");

    await clickDelete();

    expect(at(router)).toBe("/libraries/2?page=3");
  });

  it("goes to the book's library when only its own pages came before", async () => {
    const router = setup(["/libraries/2/books/7"]);
    await visit(router, "/libraries/2/books/7/files/9");

    await clickDelete();

    expect(at(router)).toBe("/libraries/2");
  });

  // The provider never saw /libraries/1, as after a reload, so it goes to the
  // library, in place of the book's entry: Back leads past it.
  it("replaces the book's entry when going to the library", async () => {
    const router = setup(["/libraries/1", "/libraries/2/books/7"], {
      initialIndex: 1,
    });

    await clickDelete();
    expect(at(router)).toBe("/libraries/2");

    await act(() => router.navigate(-1));
    expect(at(router)).toBe("/libraries/1");
  });

  it("follows Back and Forward through history", async () => {
    const router = setup(["/libraries/2/series/4"]);
    await visit(router, "/libraries/2/books/7");
    await act(() => router.navigate(-1));
    await visit(router, "/libraries/2?page=5", "/libraries/2/books/7");

    await clickDelete();

    expect(at(router)).toBe("/libraries/2?page=5");
  });

  it("follows an entry replaced in place", async () => {
    const router = setup(["/libraries/2/series/4"]);
    await visit(router, "/libraries/2");
    await act(() => router.navigate("/libraries/2?page=4", { replace: true }));
    await visit(router, "/libraries/2/books/7");

    await clickDelete();
    expect(at(router)).toBe("/libraries/2?page=4");

    await act(() => router.navigate(-1));
    expect(at(router)).toBe("/libraries/2/series/4");
  });

  // Going Back to an entry from before a reload starts the record over there.
  it("starts over from an entry it never saw", async () => {
    const router = setup(["/libraries/2?page=2", "/libraries/2/books/7"], {
      initialIndex: 1,
    });
    await act(() => router.navigate(-1));
    await visit(router, "/libraries/2/books/7");

    await clickDelete();

    expect(at(router)).toBe("/libraries/2?page=2");
  });

  // A slow delete can finish after the reader pressed the browser's Back.
  it("stays put when the reader already left the book", async () => {
    const router = setup(["/libraries/2/series/4"]);
    await visit(router, "/libraries/2/books/7");
    await act(() => router.navigate(-1));

    act(() => captured.leave?.());

    expect(at(router)).toBe("/libraries/2/series/4");
  });

  it("returns to a series page the delete did not remove", async () => {
    removed = { ...nothingRemoved, series_ids: [5] };
    const router = setup(["/libraries/2/series/4"]);
    await visit(router, "/libraries/2/books/7");

    await clickDelete();

    expect(at(router)).toBe("/libraries/2/series/4");
  });

  // Deleting a series' or author's last book removes the series or person,
  // whose page would then show Not Found.
  it.each([
    ["series", "/libraries/2/series/4", { series_ids: [4] }],
    ["person", "/libraries/2/people/4", { person_ids: [4] }],
  ])(
    "goes to the book's library when the delete removed the previous %s",
    async (_kind, previous, ids) => {
      removed = { ...nothingRemoved, ...ids };
      const router = setup([previous]);
      await visit(router, "/libraries/2/books/7");

      await clickDelete();

      expect(at(router)).toBe("/libraries/2");
    },
  );

  it("goes to the book's library outside the provider", async () => {
    const router = setup(["/libraries/1"], { withProvider: false });
    await visit(router, "/libraries/2/books/7");

    await clickDelete();

    expect(at(router)).toBe("/libraries/2");
  });
});
