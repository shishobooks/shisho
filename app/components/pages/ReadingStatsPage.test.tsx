import {
  keepPreviousData,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { toast, Toaster } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AuthProvider } from "@/components/contexts/Auth";
import { MobileNavProvider } from "@/contexts/MobileNav";
import { mockCoverDimensions } from "@/testing/coverDimensions";
import type {
  ReadingStatsBookRef,
  ReadingStatsCalendarResponse,
  ReadingStatsDayResponse,
  ReadingStatsFinishedBook,
  ReadingStatsHeatmapResponse,
  ReadingStatsResponse,
  ReadThrough,
} from "@/types";
import { formatCount } from "@/utils/format";

import ReadingStatsPage from "./ReadingStatsPage";

beforeEach(() => {
  vi.setSystemTime(new Date("2026-06-15T12:00:00Z"));
});

afterEach(() => {
  toast.dismiss();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const readThrough = (
  id: number,
  overrides: Partial<ReadThrough> = {},
): ReadThrough => ({
  id,
  created_at: "2026-03-01T00:00:00Z",
  updated_at: "2026-03-01T00:00:00Z",
  book_id: id,
  book_title: null,
  book_authors: null,
  begin_day: "2026-03-01",
  end_day: "2026-03-09",
  outcome: "finished",
  paused: false,
  ...overrides,
});

const finishedRow = (
  id: number,
  title: string,
  overrides: Partial<ReadingStatsFinishedBook> = {},
): ReadingStatsFinishedBook => ({
  read_through: readThrough(id),
  title,
  authors: [],
  removed: false,
  book: { id, library_id: 7, cover_cache_key: "" },
  days_taken: 9,
  seconds: 5400,
  rating: null,
  ...overrides,
});

const emptyStats = (year: number | null): ReadingStatsResponse => ({
  year,
  years: [],
  totals: {
    finished: 0,
    rereads: 0,
    did_not_finish: 0,
    reading_seconds: 0,
    listening_seconds: 0,
    median_days_to_finish: null,
    average_rating: null,
  },
  periods: [],
  genres: [],
  tags: [],
  authors: [],
  kinds: [],
  ratings: [],
  places: [],
});

const busyStats = (year: number | null): ReadingStatsResponse => ({
  ...emptyStats(year),
  years: [2026, 2024],
  totals: {
    finished: 3,
    rereads: 1,
    did_not_finish: 1,
    reading_seconds: 7200,
    listening_seconds: 1800,
    median_days_to_finish: 9,
    average_rating: 4.5,
  },
  periods: Array.from({ length: 12 }, (_, i) => ({
    year: 2026,
    month: i + 1,
    finished: i === 2 ? 3 : 0,
    reading_seconds: i === 2 ? 7200 : 0,
    listening_seconds: i === 2 ? 1800 : 0,
  })),
  genres: [
    { name: "Classics", books: 2 },
    { name: "Fantasy", books: 1 },
  ],
  tags: Array.from({ length: 150 }, (_, i) => ({
    name: `Tag ${String(i).padStart(3, "0")}`,
    books: 150 - i,
  })),
  kinds: [{ kind: "audiobook", books: 1 }],
  ratings: [{ rating: 4.5, books: 1 }],
  places: [
    {
      source: "kobo",
      device_name: "Bedside Kobo",
      reading_seconds: 3600,
      listening_seconds: 0,
    },
    {
      source: "koreader",
      device_name: null,
      reading_seconds: 600,
      listening_seconds: 0,
    },
  ],
});

const finishedRows = [
  finishedRow(1, "Dune", { rating: 4.5 }),
  finishedRow(2, "Gone", {
    read_through: readThrough(2, { book_id: null, book_title: "Gone" }),
    removed: true,
    book: null,
    seconds: null,
  }),
  finishedRow(3, "Elsewhere", { book: null }),
];

// The page's path and query string, for asserting what the URL says.
const LocationProbe = () => {
  const location = useLocation();
  return (
    <>
      <output data-testid="location">{location.search}</output>
      <output data-testid="pathname">{location.pathname}</output>
    </>
  );
};

const emptyHeatmap: ReadingStatsHeatmapResponse = {
  today: "2026-06-15",
  from: "2025-06-16",
  first_day: "2025-03-01",
  days: [],
};

const emptyCalendar = (
  year: number,
  month: number,
): ReadingStatsCalendarResponse => ({ year, month, books: [], days: [] });

const emptyDay = (day: string): ReadingStatsDayResponse => ({
  day,
  time_zone: "UTC",
  sessions: [],
  finished: [],
});

const renderPage = ({
  stats = busyStats,
  finished = finishedRows,
  finishedTotal = finished.length,
  calendar = emptyCalendar,
  day = emptyDay,
  heatmap = emptyHeatmap,
  deleteSession = () => new Response(null, { status: 204 }),
  initialEntry = "/reading-stats",
}: {
  stats?: (year: number | null) => ReadingStatsResponse;
  finished?: ReadingStatsFinishedBook[];
  finishedTotal?: number;
  calendar?: (year: number, month: number) => ReadingStatsCalendarResponse;
  day?: (day: string) => ReadingStatsDayResponse;
  heatmap?: ReadingStatsHeatmapResponse;
  deleteSession?: () => Response;
  initialEntry?: string;
} = {}) => {
  mockCoverDimensions();
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
      const url = new URL(String(input), "http://localhost");
      const path = url.pathname;
      if (
        init?.method === "DELETE" &&
        path.startsWith("/api/reading/sessions/")
      )
        return deleteSession();
      if (path === "/api/reading/stats/heatmap") return Response.json(heatmap);
      if (path === "/api/reading/stats/calendar")
        return Response.json(
          calendar(
            Number(url.searchParams.get("year")),
            Number(url.searchParams.get("month")),
          ),
        );
      if (path === "/api/reading/stats/day")
        return Response.json(day(url.searchParams.get("day")!));
      if (path === "/api/auth/status")
        return Response.json({ needs_setup: false, demo_mode: false });
      if (path === "/api/auth/me")
        return Response.json({
          id: 1,
          username: "reader",
          permissions: ["books:read"],
          role_name: "viewer",
        });
      if (path === "/api/user/libraries") return Response.json([]);
      if (path === "/api/settings/user") return Response.json({});
      if (path === "/api/reading/stats/finished")
        return Response.json({ items: finished, total: finishedTotal });
      if (path === "/api/reading/stats") {
        const year = url.searchParams.get("year");
        return Response.json(stats(year ? Number(year) : null));
      }
      return Response.json({ items: [], total: 0 });
    });
  // As the app's client does, a new range keeps the last one's figures
  // until it loads.
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, placeholderData: keepPreviousData },
    },
  });
  render(
    <QueryClientProvider client={client}>
      <AuthProvider>
        <MobileNavProvider>
          <MemoryRouter initialEntries={[initialEntry]}>
            <Routes>
              <Route
                element={<ReadingStatsPage />}
                path="/reading-stats/:tab?"
              />
            </Routes>
            <LocationProbe />
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

const finishedRequests = (
  fetchMock: ReturnType<typeof renderPage>["fetchMock"],
) =>
  fetchMock.mock.calls
    .map(([input]) => new URL(String(input), "http://localhost"))
    .filter((url) => url.pathname === "/api/reading/stats/finished")
    .map((url) => url.searchParams.toString());

const statsRequests = (fetchMock: ReturnType<typeof renderPage>["fetchMock"]) =>
  fetchMock.mock.calls
    .map(([input]) => new URL(String(input), "http://localhost"))
    .filter((url) => url.pathname === "/api/reading/stats")
    .map((url) => url.searchParams.get("year"));

describe("ReadingStatsPage", () => {
  it("opens on the current year and shows the totals", async () => {
    const { fetchMock } = renderPage();

    expect(await screen.findByText("Did Not Finish")).toBeInTheDocument();
    expect(statsRequests(fetchMock)).toEqual(["2026"]);
    expect(screen.getByRole("combobox", { name: "Range" })).toHaveTextContent(
      "2026",
    );
    expect(screen.getByText("1 reread")).toBeInTheDocument();
    expect(screen.getByText("2h 0m")).toBeInTheDocument();
    expect(screen.getByText("30m")).toBeInTheDocument();
    expect(
      within(screen.getByRole("group", { name: "Average Rating" })).getByRole(
        "img",
        { name: "Rated 4.5 stars" },
      ),
    ).toHaveTextContent("4.5");
    expect(screen.getByText("Bedside Kobo")).toBeInTheDocument();
    expect(screen.getByText("KOReader")).toBeInTheDocument();
    expect(screen.getByText(/time from them is estimated/)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Manage reading history" }),
    ).toHaveAttribute("href", "/reading-history");
  });

  it("offers the years with activity and all time", async () => {
    const { fetchMock, user } = renderPage();
    await screen.findByText("Did Not Finish");

    await user.click(screen.getByRole("combobox", { name: "Range" }));
    const options = screen
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(options).toEqual(["2026", "2024", "All time"]);

    await user.click(screen.getByRole("option", { name: "2024" }));
    await waitFor(() =>
      expect(statsRequests(fetchMock)).toEqual(["2026", "2024"]),
    );

    await user.click(screen.getByRole("combobox", { name: "Range" }));
    await user.click(screen.getByRole("option", { name: "All time" }));
    await waitFor(() =>
      expect(statsRequests(fetchMock)).toEqual(["2026", "2024", null]),
    );
    expect(
      await screen.findByText("Books finished by year"),
    ).toBeInTheDocument();
  });

  it("opens the range in the URL", async () => {
    const { fetchMock } = renderPage({
      initialEntry: "/reading-stats?year=all",
    });
    expect(await screen.findByText("Did Not Finish")).toBeInTheDocument();
    expect(statsRequests(fetchMock)).toEqual([null]);
    expect(screen.getByRole("combobox", { name: "Range" })).toHaveTextContent(
      "All time",
    );
  });

  it("shows an empty state for a range with no reading", async () => {
    renderPage({ stats: emptyStats });

    expect(
      await screen.findByText(/No reading in 2026 yet/),
    ).toBeInTheDocument();
    expect(screen.queryByText("Did Not Finish")).not.toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Range" })).toHaveTextContent(
      "2026",
    );
  });

  it("links only the finished books the user can still reach", async () => {
    renderPage();

    const list = await screen.findByRole("list", { name: "Books finished" });
    const rows = within(list).getAllByRole("listitem");
    expect(rows).toHaveLength(3);

    expect(within(rows[0]).getByRole("link", { name: "Dune" })).toHaveAttribute(
      "href",
      "/libraries/7/books/1",
    );
    // Narrow and wide layouts both render here: jsdom loads no Tailwind.
    expect(rows[0]).toHaveTextContent("9 days");
    expect(rows[0]).toHaveTextContent("1h 30m");

    expect(within(rows[1]).queryByRole("link")).not.toBeInTheDocument();
    expect(within(rows[1]).getByText("Gone")).toBeInTheDocument();
    expect(
      within(rows[1]).getByText("Removed from library"),
    ).toBeInTheDocument();

    expect(within(rows[2]).queryByRole("link")).not.toBeInTheDocument();
    expect(
      within(rows[2]).getByText("No access to this library"),
    ).toBeInTheDocument();

    // View-only: nothing on the list edits a Read-through.
    expect(within(list).queryByRole("button")).not.toBeInTheDocument();
  });

  it("pages through the finished books, and starts over on a new range", async () => {
    const { fetchMock, user } = renderPage({ finishedTotal: 120 });

    expect(
      await screen.findByText("Showing 1-50 of 120 books"),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Go to next page" }));
    await waitFor(() =>
      expect(finishedRequests(fetchMock)).toEqual([
        "year=2026&limit=50&offset=0",
        "year=2026&limit=50&offset=50",
      ]),
    );

    await user.click(screen.getByRole("combobox", { name: "Range" }));
    await user.click(screen.getByRole("option", { name: "All time" }));
    await waitFor(() =>
      expect(finishedRequests(fetchMock).slice(-1)[0]).toBe(
        "limit=50&offset=0",
      ),
    );
    expect(
      await screen.findByText("Books finished by year"),
    ).toBeInTheDocument();
  });

  it.each(["abc", "99999", "999", "0000", "20x6"])(
    "drops year=%s from the URL and shows the current year",
    async (value) => {
      const { fetchMock } = renderPage({
        initialEntry: `/reading-stats?year=${value}&page=2`,
      });
      await screen.findByText("Did Not Finish");
      await waitFor(() =>
        expect(screen.getByTestId("location")).toHaveTextContent(/^$/),
      );
      expect(new Set(statsRequests(fetchMock))).toEqual(new Set(["2026"]));
      expect(screen.getByRole("combobox", { name: "Range" })).toHaveTextContent(
        "2026",
      );
    },
  );

  it("shows a year with no reading in it as empty, and keeps it in the URL", async () => {
    const { fetchMock } = renderPage({
      stats: emptyStats,
      initialEntry: "/reading-stats?year=1800",
    });
    expect(
      await screen.findByText(/No reading in 1800 yet/),
    ).toBeInTheDocument();
    expect(statsRequests(fetchMock)).toEqual(["1800"]);
    expect(screen.getByTestId("location")).toHaveTextContent("?year=1800");
    expect(screen.getByRole("combobox", { name: "Range" })).toHaveTextContent(
      "1800",
    );
  });

  it.each(["2024", "2026"])("keeps year=%s in the URL", async (value) => {
    renderPage({ initialEntry: `/reading-stats?year=${value}` });
    await screen.findByText("Did Not Finish");
    expect(screen.getByTestId("location")).toHaveTextContent(`?year=${value}`);
  });

  it("notes that device time is estimated only when there is some", async () => {
    renderPage({
      stats: (year) => ({
        ...busyStats(year),
        places: [
          {
            source: "web_reader",
            device_name: null,
            reading_seconds: 600,
            listening_seconds: 0,
          },
        ],
      }),
    });
    await screen.findByText("Did Not Finish");
    expect(
      screen.queryByText(/time from them is estimated/),
    ).not.toBeInTheDocument();
  });

  it("shows the first five tags, then all of them with a search", async () => {
    const { user } = renderPage();
    const tags = await screen.findByRole("region", { name: "Tags" });
    expect(
      within(tags).getAllByRole("button", { pressed: false }),
    ).toHaveLength(5);
    expect(
      within(tags).queryByRole("textbox", { name: "Search tags" }),
    ).not.toBeInTheDocument();

    await user.click(
      within(tags).getByRole("button", { name: "Show all 150 tags" }),
    );
    expect(
      within(tags).getAllByRole("button", { pressed: false }),
    ).toHaveLength(150);
    await user.type(
      within(tags).getByRole("textbox", { name: "Search tags" }),
      "tag 14",
    );
    expect(
      within(tags)
        .getAllByRole("button", { pressed: false })
        .map((button) => button.textContent),
    ).toEqual(
      Array.from(
        { length: 10 },
        (_, i) => `Tag 14${i}${formatCount(150 - 140 - i, "book")}`,
      ),
    );
    await user.click(within(tags).getByRole("button", { name: "Show fewer" }));
    expect(
      within(tags).getAllByRole("button", { pressed: false }),
    ).toHaveLength(5);
  });

  it("filters the finished books by a breakdown row, until its chip is removed", async () => {
    const { fetchMock, user } = renderPage();
    const genres = await screen.findByRole("region", { name: "Genres" });

    await user.click(within(genres).getByRole("button", { name: /^Fantasy/ }));
    await waitFor(() =>
      expect(finishedRequests(fetchMock).slice(-1)[0]).toBe(
        "year=2026&genre=Fantasy&limit=50&offset=0",
      ),
    );
    expect(
      within(genres).getByRole("button", { name: /^Fantasy/ }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("location")).toHaveTextContent("?genre=Fantasy");

    const kinds = screen.getByRole("region", { name: "Kind" });
    await user.click(
      within(kinds).getByRole("button", { name: /^Audiobooks/ }),
    );
    await waitFor(() =>
      expect(finishedRequests(fetchMock).slice(-1)[0]).toBe(
        "year=2026&genre=Fantasy&kind=audiobook&limit=50&offset=0",
      ),
    );

    const chips = screen.getByRole("group", { name: "Filters" });
    await user.click(
      within(chips).getByRole("button", { name: "Remove Fantasy" }),
    );
    await waitFor(() =>
      expect(finishedRequests(fetchMock).slice(-1)[0]).toBe(
        "year=2026&kind=audiobook&limit=50&offset=0",
      ),
    );

    // Choosing the selected row again clears it.
    await user.click(
      within(kinds).getByRole("button", { name: /^Audiobooks/ }),
    );
    await waitFor(() =>
      expect(finishedRequests(fetchMock).slice(-1)[0]).toBe(
        "year=2026&limit=50&offset=0",
      ),
    );
    expect(
      screen.queryByRole("group", { name: "Filters" }),
    ).not.toBeInTheDocument();
  });

  it("filters by rating from the URL and says when nothing matches", async () => {
    const { fetchMock } = renderPage({
      finished: [],
      initialEntry: "/reading-stats?rating=4.5&kind=scroll",
    });
    expect(
      await screen.findByText("No finished books match these filters."),
    ).toBeInTheDocument();
    expect(finishedRequests(fetchMock)).toEqual([
      "year=2026&rating=4.5&limit=50&offset=0",
    ]);
    expect(
      within(screen.getByRole("group", { name: "Filters" })).getByRole(
        "button",
        { name: "Remove 4.5 stars" },
      ),
    ).toBeInTheDocument();
  });

  it("puts a finished book's rating in its row", async () => {
    renderPage();
    const list = await screen.findByRole("list", { name: "Books finished" });
    const [dune, gone] = within(list).getAllByRole("listitem");
    expect(
      within(dune).getAllByRole("img", { name: "Rated 4.5 stars" }).length,
    ).toBeGreaterThan(0);
    expect(
      within(gone).queryByRole("img", { name: /^Rated/ }),
    ).not.toBeInTheDocument();
  });

  it("shows a month's figures on hover, with an axis in round steps", async () => {
    const { user } = renderPage();
    const chart = await screen.findByRole("region", {
      name: "Reading and listening by month",
    });
    // The busiest month's two and a half hours round the axis up to 3h.
    expect(chart).toHaveTextContent(/0\s*1h\s*2h\s*3h/);

    await user.hover(
      chart.querySelector('[data-period="2026-3"]') as HTMLElement,
    );
    const tooltip = within(chart).getByText("March 2026").parentElement!;
    expect(tooltip).toHaveTextContent("2h 30m");
    expect(tooltip).toHaveTextContent("Reading2h 0m");
    expect(tooltip).toHaveTextContent("Listening30m");

    const finished = screen.getByRole("region", {
      name: "Books finished by month",
    });
    await user.hover(
      finished.querySelector('[data-period="2026-3"]') as HTMLElement,
    );
    expect(
      within(finished).getByText("March 2026").parentElement,
    ).toHaveTextContent("3 books");
  });

  it("separates a finished book's details with dots, and leaves out time it has none of", async () => {
    renderPage({
      finished: [
        finishedRow(1, "Dune", { rating: 4.5 }),
        finishedRow(2, "Emma", { seconds: 0 }),
        finishedRow(3, "Ubik", { seconds: 30 }),
      ],
    });
    const list = await screen.findByRole("list", { name: "Books finished" });
    const [dune, emma, ubik] = within(list).getAllByRole("listitem");
    const duneLine = within(dune).getByText(
      /^Finished Mar 9, 2026 · 9 days · 1h 30m ·$/,
    );
    expect(
      within(duneLine).getByRole("img", { name: "Rated 4.5 stars" }),
    ).toBeInTheDocument();
    expect(
      within(emma).getByText("Finished Mar 9, 2026 · 9 days"),
    ).toBeInTheDocument();
    expect(
      within(ubik).getByText("Finished Mar 9, 2026 · 9 days · <1m"),
    ).toBeInTheDocument();
  });

  it("starts panels collapsed again on a new range", async () => {
    const { user } = renderPage();
    const tags = await screen.findByRole("region", { name: "Tags" });
    await user.click(
      within(tags).getByRole("button", { name: "Show all 150 tags" }),
    );
    expect(
      within(tags).getByRole("textbox", { name: "Search tags" }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: "Range" }));
    await user.click(screen.getByRole("option", { name: "2024" }));
    const after = await screen.findByRole("region", { name: "Tags" });
    await waitFor(() =>
      expect(
        within(after).queryByRole("textbox", { name: "Search tags" }),
      ).not.toBeInTheDocument(),
    );
    expect(
      within(after).getByRole("button", { name: "Show all 150 tags" }),
    ).toBeInTheDocument();
  });

  it("shows reading and listening time apart for each place", async () => {
    renderPage({
      stats: (year) => ({
        ...busyStats(year),
        places: [
          {
            source: "web_reader",
            device_name: null,
            reading_seconds: 3600,
            listening_seconds: 1200,
          },
        ],
      }),
    });
    const places = await screen.findByRole("region", {
      name: "Where you read",
    });
    expect(places).toHaveTextContent("1h 0m reading, 20m listening");
  });

  describe("heatmap and calendar", () => {
    const dune = {
      key: "book-1",
      title: "Dune",
      removed: false,
      book: { id: 1, library_id: 7, cover_cache_key: "" },
    };
    const emma = { key: "book-2", title: "Emma", removed: false, book: null };
    const gone = {
      key: "read-through-9",
      title: "Gone",
      removed: true,
      book: null,
    };
    const entry = (key: string, seconds: number, finished = false) => ({
      key,
      seconds,
      finished,
    });
    // June 2026 starts on a Monday, so June 1 is in the second column.
    const june = (year: number, month: number) =>
      month === 6
        ? {
            year,
            month,
            books: [dune, emma, gone],
            days: [
              { day: "2026-06-01", books: [entry("book-1", 600)] },
              {
                day: "2026-06-02",
                books: [entry("book-1", 600), entry("book-2", 300)],
              },
              { day: "2026-06-03", books: [entry("book-1", 600, true)] },
              { day: "2026-06-05", books: [entry("book-1", 600)] },
              { day: "2026-06-10", books: [entry("read-through-9", 0, true)] },
            ],
          }
        : month === 5
          ? {
              year,
              month,
              books: [dune],
              days: [{ day: "2026-05-29", books: [entry("book-1", 900)] }],
            }
          : emptyCalendar(year, month);
    const CALENDAR = "/reading-stats/calendar";

    const bars = (key: string) =>
      [
        ...document.querySelectorAll<HTMLElement>(
          `[data-book="${key}"][data-span]`,
        ),
      ].map((bar) => ({
        span: bar.dataset.span,
        color: bar.style.backgroundColor,
      }));

    it("puts Overview and Calendar on their own tabs, each at its own path", async () => {
      const { user, fetchMock } = renderPage();
      expect(await screen.findByText("Did Not Finish")).toBeInTheDocument();
      expect(screen.getByRole("tab", { name: "Overview" })).toHaveAttribute(
        "aria-selected",
        "true",
      );
      expect(
        screen.queryByRole("region", { name: "Calendar" }),
      ).not.toBeInTheDocument();

      await user.click(screen.getByRole("tab", { name: "Calendar" }));
      expect(
        await screen.findByRole("region", { name: "Calendar" }),
      ).toBeInTheDocument();
      expect(screen.getByTestId("pathname")).toHaveTextContent(CALENDAR);
      expect(
        screen.queryByRole("combobox", { name: "Range" }),
      ).not.toBeInTheDocument();
      expect(statsRequests(fetchMock)).toEqual(["2026"]);
    });

    it("opens the Calendar tab without loading a year's figures, and sends an unknown tab to Overview", async () => {
      const { fetchMock } = renderPage({ initialEntry: CALENDAR });
      await screen.findByRole("region", { name: "Calendar" });
      expect(statsRequests(fetchMock)).toEqual([]);
      cleanup();

      renderPage({ initialEntry: "/reading-stats/nope?year=2024" });
      await screen.findByText("Did Not Finish");
      expect(screen.getByTestId("pathname")).toHaveTextContent(
        /^\/reading-stats$/,
      );
    });

    it("shades the heatmap by each day's time, and opens a day from it", async () => {
      const { user } = renderPage({
        initialEntry: CALENDAR,
        heatmap: {
          ...emptyHeatmap,
          days: [
            { day: "2026-03-01", reading_seconds: 60, listening_seconds: 0 },
            { day: "2026-03-02", reading_seconds: 3600, listening_seconds: 0 },
          ],
        },
      });
      expect(
        await screen.findByRole("region", { name: "Reading by day" }),
      ).toHaveTextContent("1h 1m in the last year");
      const cell = (day: string) =>
        document.querySelector<HTMLElement>(`[data-day="${day}"]`);
      expect(cell("2026-03-01")?.dataset.level).toBe("1");
      expect(cell("2026-03-02")?.dataset.level).toBe("4");
      expect(cell("2026-03-03")?.dataset.level).toBe("0");
      expect(cell("2025-06-16")).not.toBeNull();
      expect(cell("2025-06-15")).toBeNull();
      expect(cell("2026-06-16")).toBeNull();
      expect(
        screen.getByText(
          /most reading in the last year was 1h 0m on Monday, March 2/,
        ),
      ).toBeInTheDocument();

      await user.click(cell("2026-03-02")!);
      expect(screen.getByTestId("location")).toHaveTextContent(
        "?month=2026-03&day=2026-03-02",
      );
      expect(
        await screen.findByRole("heading", { name: "March 2026" }),
      ).toBeInTheDocument();
    });

    it("opens on this month and goes back to the first month with reading", async () => {
      const { user } = renderPage({ calendar: june, initialEntry: CALENDAR });
      const calendar = await screen.findByRole("region", { name: "Calendar" });
      expect(
        within(calendar).getByRole("heading", { name: "June 2026" }),
      ).toBeInTheDocument();
      expect(
        within(calendar).getByRole("button", { name: "Next month" }),
      ).toBeDisabled();

      await user.click(
        within(calendar).getByRole("button", { name: "Previous month" }),
      );
      expect(
        await within(calendar).findByRole("heading", { name: "May 2026" }),
      ).toBeInTheDocument();
      expect(screen.getByTestId("location")).toHaveTextContent(
        "?month=2026-05",
      );
      cleanup();

      renderPage({ initialEntry: `${CALENDAR}?month=2025-03` });
      const first = await screen.findByRole("region", { name: "Calendar" });
      expect(
        within(first).getByRole("heading", { name: "March 2025" }),
      ).toBeInTheDocument();
      expect(
        within(first).getByRole("button", { name: "Previous month" }),
      ).toBeDisabled();
    });

    it("joins a book's consecutive days into one bar in its own colour", async () => {
      const { user } = renderPage({ calendar: june, initialEntry: CALENDAR });
      await screen.findByRole("button", { name: /^Monday, June 1, 2026/ });

      const duneBars = bars("book-1");
      expect(duneBars.map((bar) => bar.span)).toEqual(["1-3", "5-5"]);
      expect(duneBars[0].color).toBe(duneBars[1].color);
      expect(bars("book-2").map((bar) => bar.span)).toEqual(["2-2"]);
      expect(bars("book-2")[0].color).not.toBe("");
      expect(bars("book-2")[0].color).not.toBe(duneBars[0].color);

      await user.click(screen.getByRole("button", { name: "Previous month" }));
      await screen.findByRole("heading", { name: "May 2026" });
      await waitFor(() => expect(bars("book-1")).toHaveLength(1));
      expect(bars("book-1")[0].color).toBe(duneBars[0].color);
    });

    it("marks the day a book was finished, even with no reading that day", async () => {
      renderPage({ calendar: june, initialEntry: CALENDAR });
      expect(
        await screen.findByRole("button", {
          name: "Wednesday, June 3, 2026. Dune, finished",
        }),
      ).toBeInTheDocument();
      expect(
        document.querySelectorAll('[data-finish][data-book="book-1"]'),
      ).toHaveLength(1);
      expect(
        screen.getByRole("button", {
          name: "Wednesday, June 10, 2026. Gone, finished",
        }),
      ).toBeInTheDocument();
      expect(
        document.querySelectorAll('[data-finish][data-book="read-through-9"]'),
      ).toHaveLength(1);
      expect(bars("read-through-9")).toEqual([]);
    });

    it("closes the delete dialog when Demo Mode refuses it", async () => {
      const { user } = renderPage({
        calendar: june,
        initialEntry: CALENDAR,
        day: (day) => ({
          ...emptyDay(day),
          sessions: [
            {
              id: 41,
              book: dune,
              started_at: "2026-06-01T20:00:00Z",
              ended_at: "2026-06-01T21:00:00Z",
              seconds: 3600,
              start_percentage: 0.1,
              end_percentage: 0.25,
              kind: "reading",
              source: "web_reader",
              measured: true,
            },
          ],
        }),
        deleteSession: () =>
          Response.json(
            {
              error: {
                code: "demo_mode",
                message: "This action is unavailable in the demo.",
              },
            },
            { status: 403 },
          ),
      });
      await user.click(
        await screen.findByRole("button", { name: /^Monday, June 1, 2026/ }),
      );
      await user.click(
        await screen.findByRole("button", { name: "Delete session of Dune" }),
      );
      const dialog = await screen.findByRole("dialog", {
        name: "Delete this reading session?",
      });
      await user.click(within(dialog).getByRole("button", { name: "Delete" }));

      expect(
        await screen.findByText("This action is unavailable in the demo."),
      ).toBeInTheDocument();
      await waitFor(() =>
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
      );
      expect(
        screen.getAllByText("This action is unavailable in the demo."),
      ).toHaveLength(1);
    });

    it("opens a day's sessions, and deletes one after asking", async () => {
      const { user, fetchMock } = renderPage({
        calendar: june,
        initialEntry: CALENDAR,
        day: (day) => ({
          ...emptyDay(day),
          time_zone: "America/Chicago",
          sessions: [
            {
              id: 41,
              book: dune,
              started_at: "2026-06-01T04:30:00Z",
              ended_at: "2026-06-01T06:00:00Z",
              seconds: 5400,
              start_percentage: 0.1,
              end_percentage: 0.25,
              kind: "reading",
              source: "koreader",
              measured: false,
            },
          ],
        }),
      });
      await user.click(
        await screen.findByRole("button", { name: /^Monday, June 1, 2026/ }),
      );
      expect(screen.getByTestId("location")).toHaveTextContent(
        "?day=2026-06-01",
      );
      const sessions = await screen.findByRole("list", {
        name: "Reading sessions",
      });
      // 04:30 UTC is still May 31 in Chicago.
      expect(sessions).toHaveTextContent(
        "May 31, 11:30 PM to 1:00 AM · 1h 30m · 10% to 25% · KOReader (estimated)",
      );
      expect(
        within(sessions).getByRole("link", { name: "Dune" }),
      ).toHaveAttribute("href", "/libraries/7/books/1");

      await user.click(
        within(sessions).getByRole("button", {
          name: "Delete session of Dune",
        }),
      );
      const dialog = await screen.findByRole("dialog", {
        name: "Delete this reading session?",
      });
      expect(
        fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE"),
      ).toBe(false);
      await user.click(within(dialog).getByRole("button", { name: "Delete" }));
      await waitFor(() =>
        expect(
          fetchMock.mock.calls
            .filter(([, init]) => init?.method === "DELETE")
            .map(
              ([input]) => new URL(String(input), "http://localhost").pathname,
            ),
        ).toEqual(["/api/reading/sessions/41"]),
      );
      await waitFor(() =>
        expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
      );

      await user.click(screen.getByRole("button", { name: "Close day" }));
      expect(
        screen.queryByRole("list", { name: "Reading sessions" }),
      ).toBeNull();
    });

    it("lays a day out by hour, a line per book, and groups its sessions by book", async () => {
      const session = (
        id: number,
        book: ReadingStatsBookRef,
        started_at: string,
        ended_at: string,
        seconds: number,
      ) => ({
        id,
        book,
        started_at,
        ended_at,
        seconds,
        start_percentage: 0.1,
        end_percentage: 0.2,
        kind: "reading" as const,
        source: "web_reader" as const,
        measured: true,
      });
      renderPage({
        calendar: june,
        initialEntry: `${CALENDAR}?day=2026-06-02`,
        day: (day) => ({
          ...emptyDay(day),
          sessions: [
            session(
              1,
              dune,
              "2026-06-02T07:00:00Z",
              "2026-06-02T07:30:00Z",
              1800,
            ),
            session(
              2,
              emma,
              "2026-06-02T12:00:00Z",
              "2026-06-02T13:00:00Z",
              3600,
            ),
            session(
              3,
              dune,
              "2026-06-02T18:00:00Z",
              "2026-06-02T18:45:00Z",
              2700,
            ),
          ],
        }),
      });
      const sessions = await screen.findByRole("list", {
        name: "Reading sessions",
      });

      const lanes = [
        ...document.querySelectorAll<HTMLElement>(
          "[data-timeline] [data-lane]",
        ),
      ];
      expect(lanes.map((lane) => lane.dataset.lane)).toEqual([
        "book-1",
        "book-2",
      ]);
      const morning =
        lanes[0].querySelector<HTMLElement>('[data-session="1"]')!;
      expect(morning.style.left).toBe(`${((7 * 60) / 1440) * 100}%`);
      expect(lanes[0].querySelectorAll("[data-session]")).toHaveLength(2);

      const duneSessions = within(sessions).getByRole("list", {
        name: "Sessions of Dune",
      });
      expect(within(duneSessions).getAllByRole("listitem")).toHaveLength(2);
      expect(duneSessions).toHaveTextContent("7:00 AM to 7:30 AM");
      expect(duneSessions).toHaveTextContent("6:00 PM to 6:45 PM");
      // Dune's day total sits beside its title, once.
      expect(
        within(sessions).getAllByRole("link", { name: "Dune" }),
      ).toHaveLength(1);
      expect(sessions).toHaveTextContent("1h 15m");
      expect(
        within(sessions).getByRole("list", { name: "Sessions of Emma" }),
      ).toBeInTheDocument();
    });
  });
});
