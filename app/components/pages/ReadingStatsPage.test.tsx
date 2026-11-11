import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AuthProvider } from "@/components/contexts/Auth";
import { MobileNavProvider } from "@/contexts/MobileNav";
import { mockCoverDimensions } from "@/testing/coverDimensions";
import type {
  ReadingStatsFinishedBook,
  ReadingStatsResponse,
  ReadThrough,
} from "@/types";

import ReadingStatsPage from "./ReadingStatsPage";

beforeEach(() => {
  vi.setSystemTime(new Date("2026-06-15T12:00:00Z"));
});

afterEach(() => {
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
  ...overrides,
});

const emptyBreakdown = { items: [], other: null };

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
  genres: emptyBreakdown,
  tags: emptyBreakdown,
  authors: emptyBreakdown,
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
  genres: {
    items: [{ name: "Classics", books: 2 }],
    other: { names: 3, books: 2 },
  },
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
  finishedRow(1, "Dune"),
  finishedRow(2, "Gone", {
    read_through: readThrough(2, { book_id: null, book_title: "Gone" }),
    removed: true,
    book: null,
    seconds: null,
  }),
  finishedRow(3, "Elsewhere", { book: null }),
];

// The page's query string, for asserting what the URL says.
const LocationProbe = () => (
  <output data-testid="location">{useLocation().search}</output>
);

const renderPage = ({
  stats = busyStats,
  finished = finishedRows,
  finishedTotal = finished.length,
  initialEntry = "/reading-stats",
}: {
  stats?: (year: number | null) => ReadingStatsResponse;
  finished?: ReadingStatsFinishedBook[];
  finishedTotal?: number;
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
    .mockImplementation(async (input) => {
      const url = new URL(String(input), "http://localhost");
      const path = url.pathname;
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
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AuthProvider>
        <MobileNavProvider>
          <MemoryRouter initialEntries={[initialEntry]}>
            <Routes>
              <Route element={<ReadingStatsPage />} path="/reading-stats" />
            </Routes>
            <LocationProbe />
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
    expect(screen.getByText("4.5 ★")).toBeInTheDocument();
    expect(screen.getByText("Bedside Kobo")).toBeInTheDocument();
    expect(screen.getByText("KOReader")).toBeInTheDocument();
    expect(screen.getByText("3 other genres")).toBeInTheDocument();
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
    expect(within(rows[0]).getByText("9 days")).toBeInTheDocument();
    expect(within(rows[0]).getByText("1h 30m")).toBeInTheDocument();

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
});
