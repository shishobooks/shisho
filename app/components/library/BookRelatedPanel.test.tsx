import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import {
  createMemoryRouter,
  MemoryRouter,
  RouterProvider,
  ScrollRestoration,
  useLocation,
} from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useBookRelatedTabs } from "@/hooks/useBookRelatedTabs";
import { ALL_PERMISSIONS, setAuth } from "@/testing/auth";
import { mockCoverDimensions } from "@/testing/coverDimensions";
import type { Book, BookDevice, List } from "@/types";

import BookRelatedPanel from "./BookRelatedPanel";

mockCoverDimensions();

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

// Each query hook returns the data a test sets; the permissions test of the
// page covers which requests the real hooks send.
const data = vi.hoisted(() => ({
  series: {} as Record<number, Book[]>,
  authorBooks: [] as Book[],
  lists: [] as List[],
  // Each list's entries: the book and its position in an ordered list.
  listBooks: {} as Record<number, { book: Book; sort_order?: number }[]>,
  devices: [] as BookDevice[],
  // Whether the book is on the reader's To Read, and the queue in order.
  onToRead: false,
  toRead: [] as Book[],
  // The sort each list's books were asked for in.
  listSorts: {} as Record<number, string | undefined>,
  // Which queries fail, by name.
  failing: new Set<string>(),
}));

// A query that failed with nothing loaded, as TanStack Query reports it.
const failed = () => ({
  data: undefined,
  error: new Error("Server unreachable"),
  isPending: false,
  isFetching: false,
  isEnabled: true,
  refetch: vi.fn(),
});

vi.mock("@/hooks/queries/series", () => ({
  useSeriesBooks: (seriesId: number) =>
    data.failing.has("series")
      ? failed()
      : {
          data: data.series[seriesId] && {
            items: data.series[seriesId],
            total: data.series[seriesId].length,
          },
        },
}));
vi.mock("@/hooks/queries/books", () => ({
  useBooks: (query: { to_read?: boolean }) => {
    if (query.to_read) {
      return { data: { items: data.toRead, total: data.toRead.length } };
    }
    return data.failing.has("author")
      ? failed()
      : { data: { items: data.authorBooks, total: data.authorBooks.length } };
  },
}));
vi.mock("@/hooks/queries/reading", () => ({
  useBookReading: () => ({ data: { on_to_read: data.onToRead } }),
}));
vi.mock("@/hooks/queries/lists", () => ({
  useBookLists: () =>
    data.failing.has("lists") ? failed() : { data: data.lists },
  useListBooks: (listId: number, query: { sort?: string }) => {
    data.listSorts[listId] = query.sort;
    return {
      data: {
        items: (data.listBooks[listId] ?? []).map((entry, index) => ({
          id: index + 1,
          book_id: entry.book.id,
          book: entry.book,
          sort_order: entry.sort_order,
        })),
        total: (data.listBooks[listId] ?? []).length,
      },
    };
  },
}));
vi.mock("@/hooks/queries/devices", () => ({
  useBookDevices: () => ({ data: data.devices }),
}));
// BookDevicesPanel.test.tsx covers what the Devices tab lists.
vi.mock("@/components/library/BookDevicesPanel", () => ({
  default: () => <div data-testid="devices-panel" />,
}));

const makeBook = (id: number, title: string, extra: Partial<Book> = {}): Book =>
  ({
    id,
    library_id: 1,
    title,
    cover_cache_key: "",
    files: [],
    ...extra,
  }) as unknown as Book;

const inSeries = (number: number) =>
  ({
    book_series: [
      {
        id: number,
        series_id: 3,
        series_number: number,
        series: { id: 3, name: "Saga" },
      },
    ],
  }) as unknown as Partial<Book>;

const current = makeBook(2, "Second", {
  ...inSeries(2),
  authors: [{ id: 1, person_id: 10, person: { id: 10, name: "Ada" } }],
} as Partial<Book>);

let location = "";
const LocationProbe = () => {
  const loc = useLocation();
  useEffect(() => {
    location = loc.pathname + loc.search;
  }, [loc]);
  return null;
};

// The page's wiring: the hook decides the tabs, the panel draws them.
const Panel = ({ book }: { book: Book }) => (
  <BookRelatedPanel book={book} related={useBookRelatedTabs(book)} />
);

const renderPanel = (book: Book = current, entry = "/libraries/1/books/2") =>
  render(
    <MemoryRouter initialEntries={[entry]}>
      <Panel book={book} />
      <LocationProbe />
    </MemoryRouter>,
  );

beforeEach(() => {
  setAuth({ permissions: ALL_PERMISSIONS });
  data.series = {
    3: [
      makeBook(1, "First", inSeries(1)),
      current,
      makeBook(3, "Third", inSeries(3)),
    ],
  };
  data.authorBooks = [];
  data.lists = [];
  data.listBooks = {};
  data.listSorts = {};
  data.devices = [];
  data.onToRead = false;
  data.toRead = [];
  data.failing = new Set();
});

const tabNames = () =>
  screen.getAllByRole("tab").map((tab) => tab.getAttribute("aria-label"));

describe("BookRelatedPanel", () => {
  it("shows nothing when no tab has anything", () => {
    data.series = {};
    const { container } = renderPanel(makeBook(2, "Alone"));

    expect(container).toBeEmptyDOMElement();
  });

  it("lists the whole series, marking this book instead of linking it", () => {
    renderPanel();

    const panel = screen.getByRole("tabpanel");
    expect(within(panel).getByText("First").closest("a")).toHaveAttribute(
      "href",
      "/libraries/1/books/1",
    );
    expect(within(panel).getByText("Second").closest("a")).toBeNull();
    expect(within(panel).getByText("#2 · This book")).toBeInTheDocument();
    expect(within(panel).getByText("Third")).toBeInTheDocument();
    expect(within(panel).getByRole("link", { name: "Saga" })).toHaveAttribute(
      "href",
      "/libraries/1/series/3",
    );
  });

  it("draws the placeholder cover for a book without one", () => {
    data.series = {
      3: [
        makeBook(1, "First", {
          ...inSeries(1),
          cover_cache_key: "abc",
        } as Partial<Book>),
        current,
      ],
    };
    renderPanel();

    const panel = screen.getByRole("tabpanel");
    const row = (title: string) =>
      within(panel)
        .getByText(title)
        .closest("a, [aria-current]") as HTMLElement;
    expect(row("Second").querySelector("img")).toBeNull();
    expect(row("Second").querySelector("svg")).not.toBeNull();
    expect(row("First").querySelector("img")).not.toBeNull();
  });

  it("draws the placeholder cover when a book's cover fails to load, until the cover changes", () => {
    const withCover = (key: string) => ({
      3: [
        makeBook(1, "First", {
          ...inSeries(1),
          cover_cache_key: key,
        } as Partial<Book>),
        current,
      ],
    });
    data.series = withCover("abc");
    const { rerender } = renderPanel();

    const row = () =>
      within(screen.getByRole("tabpanel"))
        .getByText("First")
        .closest("a") as HTMLElement;
    fireEvent.error(row().querySelector("img")!);

    expect(row().querySelector("img")).toBeNull();
    expect(row().querySelector("svg")).not.toBeNull();

    // A new cover gets its own try.
    data.series = withCover("def");
    rerender(
      <MemoryRouter initialEntries={["/libraries/1/books/2"]}>
        <Panel book={current} />
        <LocationProbe />
      </MemoryRouter>,
    );

    expect(row().querySelector("img")).not.toBeNull();
  });

  it("hides the Series tab without Series Read", () => {
    setAuth({ permissions: ["books:read"] });
    data.devices = [
      {
        device_id: "k",
        device_name: "Kobo",
        deliveries: [],
        edition_moves: [],
      },
    ] as unknown as BookDevice[];
    renderPanel();

    expect(tabNames()).toEqual(["Devices 1"]);
  });

  it("shows the Author tab only when the author has another book", () => {
    data.authorBooks = [current];
    renderPanel();
    expect(tabNames()).toEqual(["Series"]);
  });

  it("lists the author's books oldest first, this one included", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const dated = (id: number, title: string, date: string, number?: number) =>
      makeBook(id, title, {
        files: [{ release_date: date }],
        ...(number ? inSeries(number) : {}),
      } as Partial<Book>);
    data.authorBooks = [
      dated(5, "Newest", "2021-06-15T12:00:00Z"),
      // Two books with the same date fall back to series order.
      dated(4, "Volume Two", "2015-06-15T12:00:00Z", 2),
      dated(2, "Second", "2015-06-15T12:00:00Z", 1),
      dated(6, "Oldest", "2001-06-15T12:00:00Z"),
    ];
    renderPanel();

    await user.click(screen.getByRole("tab", { name: "Author" }));
    const panel = screen.getByRole("tabpanel");
    expect(
      within(panel)
        .getAllByText(/^(Newest|Volume Two|Second|Oldest)$/)
        .map((el) => el.textContent),
    ).toEqual(["Oldest", "Second", "Volume Two", "Newest"]);
    expect(within(panel).getByText("2015 · This book")).toBeInTheDocument();
  });

  it("lists undated books after the dated ones", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    data.authorBooks = [
      makeBook(8, "Undated"),
      makeBook(2, "Second", {
        files: [{ release_date: "2015-06-15T12:00:00Z" }],
      } as Partial<Book>),
    ];
    renderPanel();

    await user.click(screen.getByRole("tab", { name: "Author" }));
    expect(
      within(screen.getByRole("tabpanel"))
        .getAllByText(/^(Undated|Second)$/)
        .map((el) => el.textContent),
    ).toEqual(["Second", "Undated"]);
  });

  it("reports a series that fails to load in its tab", () => {
    data.failing = new Set(["series"]);
    renderPanel();

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Failed to load the series",
    );
  });

  it("keeps the Author and Lists tabs when they fail to load", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    data.failing = new Set(["author", "lists"]);
    renderPanel();

    expect(tabNames()).toEqual(["Series", "Author", "Lists"]);
    await user.click(screen.getByRole("tab", { name: "Lists" }));
    expect(
      within(screen.getByRole("tabpanel")).getByRole("alert"),
    ).toHaveTextContent("Failed to load the lists this book is on");
  });

  it("counts the lists and devices in their tabs and flags a device warning", () => {
    data.lists = [{ id: 8, name: "Queue", is_ordered: true } as List];
    data.listBooks = {
      8: [
        { book: makeBook(9, "Other"), sort_order: 1 },
        { book: current, sort_order: 2 },
      ],
    };
    data.devices = [
      {
        device_id: "k",
        device_name: "Kobo",
        deliveries: [],
        edition_moves: [{ from_file_id: 1, to_file_id: 2 }],
      },
    ] as unknown as BookDevice[];
    renderPanel();

    expect(tabNames()).toEqual([
      "Series",
      "Lists 1",
      "Devices 1, a Device's next sync changes this book",
    ]);
  });

  it("reads an ordered list in its own order and numbers books by position", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    data.lists = [{ id: 8, name: "Queue", is_ordered: true } as List];
    // Position 3 is a book in a library this reader cannot see, so the row
    // count and the position part ways.
    data.listBooks = {
      8: [
        { book: makeBook(9, "Other"), sort_order: 2 },
        { book: current, sort_order: 4 },
      ],
    };
    renderPanel();

    await user.click(screen.getByRole("tab", { name: /^Lists/ }));
    expect(data.listSorts[8]).toBe("manual");
    const panel = screen.getByRole("tabpanel");
    expect(within(panel).getByText("#4 · This book")).toBeInTheDocument();
    expect(within(panel).getByRole("link", { name: "Queue" })).toHaveAttribute(
      "href",
      "/lists/8",
    );
  });

  it("leads the Lists tab with To Read for a queued book, numbered by place", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    data.onToRead = true;
    data.toRead = [makeBook(9, "Next"), makeBook(5, "After"), current];
    data.lists = [{ id: 8, name: "Queue", is_ordered: false } as List];
    data.listBooks = { 8: [{ book: current }] };
    renderPanel();

    expect(tabNames()).toContain("Lists 2");
    await user.click(screen.getByRole("tab", { name: /^Lists/ }));
    const panel = screen.getByRole("tabpanel");
    const headings = within(panel).getAllByRole("link", {
      name: /^(To Read|Queue)$/,
    });
    expect(headings.map((link) => link.textContent)).toEqual([
      "To Read",
      "Queue",
    ]);
    expect(headings[0]).toHaveAttribute("href", "/to-read");
    expect(within(panel).getByText("#3 · This book")).toBeInTheDocument();
    expect(within(panel).getByText("#1")).toBeInTheDocument();
  });

  it("shows the Lists tab for a queued book on no list", () => {
    data.onToRead = true;
    data.toRead = [current];
    renderPanel();

    expect(tabNames()).toContain("Lists 1");
  });

  it("keeps the open tab in the URL and carries it to the next book", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    data.authorBooks = [current, makeBook(7, "Seventh")];
    renderPanel();

    await user.click(screen.getByRole("tab", { name: "Author" }));
    expect(location).toBe("/libraries/1/books/2?related=author");
    expect(
      within(screen.getByRole("tabpanel")).getByText("Seventh").closest("a"),
    ).toHaveAttribute("href", "/libraries/1/books/7?related=author");

    // The first tab is the default, with a clean URL.
    await user.click(screen.getByRole("tab", { name: "Series" }));
    expect(location).toBe("/libraries/1/books/2");
  });

  it("carries only a tab's name to the next book", () => {
    renderPanel(current, "/libraries/1/books/2?related=nonsense");

    expect(
      within(screen.getByRole("tabpanel")).getByText("Third").closest("a"),
    ).toHaveAttribute("href", "/libraries/1/books/3");
  });

  it("opens on the tab the URL names", () => {
    data.authorBooks = [current, makeBook(7, "Seventh")];
    renderPanel(current, "/libraries/1/books/2?related=author");

    expect(screen.getByRole("tab", { name: "Author" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("keeps the page where it is when the tab changes", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const scrollTo = vi.fn();
    vi.stubGlobal("scrollTo", scrollTo);
    data.authorBooks = [current, makeBook(7, "Seventh")];
    // The app's root renders ScrollRestoration, which scrolls to the top on
    // every navigation the caller does not exempt.
    const router = createMemoryRouter(
      [
        {
          path: "*",
          element: (
            <>
              <ScrollRestoration />
              <Panel book={current} />
            </>
          ),
        },
      ],
      { initialEntries: ["/libraries/1/books/2"] },
    );
    render(<RouterProvider router={router} />);
    scrollTo.mockClear();

    await user.click(screen.getByRole("tab", { name: "Author" }));

    expect(router.state.location.search).toBe("?related=author");
    expect(scrollTo).not.toHaveBeenCalledWith(0, 0);
    vi.unstubAllGlobals();
  });
});
