import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";
import type {
  ContentCounts,
  ServerReadingQuery,
  ServerReadingResponse,
  StatsResponse,
} from "@/types/generated/serverstats";

import AdminServerStats from "./AdminServerStats";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const emptyCounts: ContentCounts = {
  books: 0,
  files: [],
  supplement_files: 0,
  series: 0,
  persons: 0,
  genres: 0,
  tags: 0,
  publishers: 0,
  size_bytes: 0,
};

const stats: StatsResponse = {
  libraries: [
    {
      id: 1,
      name: "Comics",
      ...emptyCounts,
      books: 12,
      files: [{ file_type: "cbz", count: 12 }],
      size_bytes: 3 * 1024 ** 3,
    },
    {
      id: 2,
      name: "Fiction",
      ...emptyCounts,
      books: 18307,
      files: [
        { file_type: "epub", count: 18000 },
        { file_type: "m4b", count: 400 },
      ],
      supplement_files: 7,
      series: 210,
      persons: 4100,
      genres: 32,
      tags: 120,
      publishers: 88,
      size_bytes: 200 * 1024 ** 3,
    },
  ],
  totals: {
    books: 18319,
    files: [
      { file_type: "cbz", count: 12 },
      { file_type: "epub", count: 18000 },
      { file_type: "m4b", count: 400 },
    ],
    supplement_files: 7,
    series: 210,
    persons: 4100,
    genres: 32,
    tags: 120,
    publishers: 88,
    size_bytes: 203 * 1024 ** 3,
  },
  volumes: [
    {
      paths: [
        {
          kind: "library",
          name: "Comics",
          path: "/media/comics",
          size_bytes: 3 * 1024 ** 3,
        },
        {
          kind: "library",
          name: "Fiction",
          path: "/media/fiction",
          size_bytes: 200 * 1024 ** 3,
        },
        { kind: "data", name: "Data", path: "/config", size_bytes: 1024 ** 3 },
      ],
      total_bytes: 1000 * 1024 ** 3,
      used_bytes: 750 * 1024 ** 3,
      free_bytes: 250 * 1024 ** 3,
      library_bytes: 203 * 1024 ** 3,
      database_bytes: 1024 ** 3,
      cache_bytes: 0,
    },
    {
      paths: [
        {
          kind: "cache",
          name: "Cache",
          path: "/cache",
          size_bytes: 10 * 1024 ** 3,
        },
      ],
      total_bytes: 100 * 1024 ** 3,
      used_bytes: 10 * 1024 ** 3,
      free_bytes: 90 * 1024 ** 3,
      library_bytes: 0,
      database_bytes: 0,
      cache_bytes: 10 * 1024 ** 3,
    },
  ],
  caches: [
    {
      id: "cover_thumbnails",
      name: "Cover Thumbnails",
      size_bytes: 512 * 1024 ** 2,
      file_count: 9000,
    },
    { id: "downloads", name: "Downloads", size_bytes: 0, file_count: 0 },
  ],
  users: 4,
  devices: 6,
  growth: [
    { month: "2026-09", books: 18000 },
    { month: "2026-10", books: 0 },
    { month: "2026-11", books: 319 },
  ],
  active_readers: 3,
  current_year: 2026,
};

let query: {
  data?: StatsResponse;
  isLoading: boolean;
  error: Error | null;
  isEnabled: boolean;
  isFetching: boolean;
  refetch: () => void;
};

const loaded = <T = StatsResponse,>(data?: T, error: Error | null = null) => ({
  data,
  isLoading: false,
  error,
  isEnabled: true,
  isFetching: false,
  refetch: vi.fn(),
});

const reading = (year: number | null): ServerReadingResponse => ({
  year,
  years: [2026, 2025],
  reading_seconds: year === null ? 120 * 3600 : 36 * 3600,
  listening_seconds: year === null ? 50 * 3600 : 12 * 3600 + 30 * 60,
  read_throughs_finished: year === null ? 130 : 41,
  did_not_finish: year === null ? 9 : 2,
  readers: year === null ? 5 : 4,
});

// The range each render asked for, as the page passes it to the hook.
let readingCalls: { query: ServerReadingQuery; enabled?: boolean }[] = [];
const lastReadingCall = () => readingCalls[readingCalls.length - 1];

vi.mock("@/hooks/queries/serverStats", () => ({
  useServerStats: () => query,
  useServerStatsReading: (
    readingQuery: ServerReadingQuery,
    options: { enabled?: boolean } = {},
  ) => {
    readingCalls.push({ query: readingQuery, enabled: options.enabled });
    return readingResult ?? loaded(reading(readingQuery.year ?? null));
  },
}));

// Replaces what the reading hook returns, for loading and failure states.
let readingResult: ReturnType<typeof loaded> | null = null;

// Shows the URL's query string, so tests can see what the range picker set.
const SearchProbe = () => (
  <output data-testid="search">{useLocation().search}</output>
);

const renderPage = (url = "/admin/server-stats") =>
  render(
    <MemoryRouter initialEntries={[url]}>
      <AdminServerStats />
      <SearchProbe />
    </MemoryRouter>,
  );

const search = () => screen.getByTestId("search").textContent;

const section = (name: string) => screen.getByRole("region", { name });

describe("AdminServerStats", () => {
  beforeEach(() => {
    setAuth({ permissions: ["config:read"] });
    query = loaded(stats);
    readingCalls = [];
    readingResult = null;
  });

  it("shows the server's totals", () => {
    renderPage();

    expect(
      screen.getByRole("heading", { level: 1, name: "Server Stats" }),
    ).toBeInTheDocument();
    const overview = section("Overview");
    expect(within(overview).getByText("18,319")).toBeInTheDocument();
    expect(within(overview).getByText("18,419")).toBeInTheDocument();
    expect(within(overview).getByText("203 GB")).toBeInTheDocument();
    expect(within(overview).getByText("4")).toBeInTheDocument();
    expect(within(overview).getByText("6")).toBeInTheDocument();
    // Readers in the last 30 days sits with the users it's a share of.
    expect(
      within(overview).getByRole("group", { name: "Active readers" }),
    ).toHaveTextContent("3Last 30 days");
  });

  it("shows each library's contents and a total row", () => {
    renderPage();

    const libraries = section("Libraries");
    const fiction = within(libraries).getByRole("row", { name: /Fiction/ });
    expect(within(fiction).getByText("18,307")).toBeInTheDocument();
    // Each format is its own item, most common first, then supplements.
    const files = within(fiction).getByRole("list", {
      name: "Files in Fiction",
    });
    expect(
      within(files)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["EPUB18,000", "M4B400", "Supplements7"]);
    expect(within(fiction).getByText("4,100")).toBeInTheDocument();
    expect(within(fiction).getByText("200 GB")).toBeInTheDocument();
    const total = within(libraries).getByRole("row", { name: /All libraries/ });
    expect(within(total).getByText("18,319")).toBeInTheDocument();
  });

  it("shows each volume once with the paths on it", () => {
    renderPage();

    const disk = section("Disk Space");
    const volumes = within(disk).getAllByRole("listitem", { name: /volume/i });
    expect(volumes).toHaveLength(2);
    const paths = within(volumes[0]).getByRole("table", {
      name: "Volume 1 paths",
    });
    const fiction = within(paths).getByRole("row", { name: /Fiction/ });
    expect(fiction).toHaveTextContent("/media/fiction");
    expect(fiction).toHaveTextContent("200 GB");
    expect(
      within(paths).getByRole("row", { name: /Database/ }),
    ).toHaveTextContent("/config");
    expect(within(volumes[0]).getByText("250 GB free")).toBeInTheDocument();
    expect(
      within(volumes[0]).getByText("750 GB used of 1000 GB"),
    ).toBeInTheDocument();
    expect(within(volumes[1]).getByText("/cache")).toBeInTheDocument();
  });

  it("breaks each volume's used space into what Shisho keeps there", () => {
    renderPage();

    const volumes = within(section("Disk Space")).getAllByRole("listitem", {
      name: /volume/i,
    });
    const legend = within(volumes[0]).getByRole("list", {
      name: "Volume 1 space",
    });
    const entry = (name: string) =>
      within(legend).getByText(name).closest("li") as HTMLElement;
    expect(entry("Libraries")).toHaveTextContent("203 GB");
    expect(entry("Database")).toHaveTextContent("1 GB");
    // The rest of the used space is other data on the volume.
    expect(entry("Other")).toHaveTextContent("546 GB");
    expect(entry("Free")).toHaveTextContent("250 GB");
    // A kind with nothing on this volume is left out.
    expect(within(legend).queryByText("Cache")).not.toBeInTheDocument();

    const cacheLegend = within(volumes[1]).getByRole("list", {
      name: "Volume 2 space",
    });
    expect(within(cacheLegend).queryByText("Other")).not.toBeInTheDocument();
    expect(within(cacheLegend).getByText("Cache")).toBeInTheDocument();
  });

  it("caps Shisho's share at the used space when files are compressed", () => {
    query = loaded({
      ...stats,
      volumes: [
        {
          paths: [],
          total_bytes: 100 * 1024 ** 3,
          used_bytes: 40 * 1024 ** 3,
          free_bytes: 60 * 1024 ** 3,
          // More than the volume uses, as on a compressed filesystem.
          library_bytes: 50 * 1024 ** 3,
          database_bytes: 0,
          cache_bytes: 0,
        },
      ],
    });
    renderPage();

    const legend = within(section("Disk Space")).getByRole("list", {
      name: "Volume 1 space",
    });
    expect(within(legend).queryByText("Other")).not.toBeInTheDocument();
    // The legend still states the stored size.
    expect(
      within(legend).getByText("Libraries").closest("li"),
    ).toHaveTextContent("50 GB");
  });

  it("gives a compressed segment's share as the bar draws it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    query = loaded({
      ...stats,
      volumes: [
        {
          paths: [],
          total_bytes: 100 * 1024 ** 3,
          used_bytes: 40 * 1024 ** 3,
          free_bytes: 60 * 1024 ** 3,
          library_bytes: 150 * 1024 ** 3,
          database_bytes: 0,
          cache_bytes: 0,
        },
      ],
    });
    const { container } = renderPage();

    await user.hover(
      container.querySelector('[data-segment="Libraries"]') as HTMLElement,
    );
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "150 GB · 40% of the volume",
    );
  });

  it("shows a volume's space even when none of its paths are listed", () => {
    query = loaded({
      ...stats,
      volumes: [
        {
          paths: [],
          total_bytes: 100 * 1024 ** 3,
          used_bytes: 40 * 1024 ** 3,
          free_bytes: 60 * 1024 ** 3,
          library_bytes: 10 * 1024 ** 3,
          database_bytes: 0,
          cache_bytes: 0,
        },
        {
          paths: [],
          total_bytes: 10 * 1024 ** 3,
          used_bytes: 1 * 1024 ** 3,
          free_bytes: 9 * 1024 ** 3,
          library_bytes: 0,
          database_bytes: 0,
          cache_bytes: 0,
        },
      ],
    });
    renderPage();

    const volumes = within(section("Disk Space")).getAllByRole("listitem", {
      name: /volume/i,
    });
    expect(volumes).toHaveLength(2);
    expect(within(volumes[0]).getByText("60 GB free")).toBeInTheDocument();
    expect(
      within(volumes[0]).queryByRole("table", { name: "Volume 1 paths" }),
    ).not.toBeInTheDocument();
    expect(
      within(volumes[0]).getByText("Holds only library folders you can't see."),
    ).toBeInTheDocument();
  });

  it("lists each cache under the cache path, with no separate section", () => {
    renderPage();

    const volumes = within(section("Disk Space")).getAllByRole("listitem", {
      name: /volume/i,
    });
    const paths = within(volumes[1]).getByRole("table", {
      name: "Volume 2 paths",
    });
    expect(
      within(paths).getByRole("row", { name: /Cover Thumbnails/ }),
    ).toHaveTextContent("512 MB");
    expect(
      within(paths).getByRole("row", { name: /Downloads/ }),
    ).toHaveTextContent("0 B");
    expect(
      within(paths).getByRole("link", { name: "Manage caches" }),
    ).toHaveAttribute("href", "/admin/cache");
    // The link has a row of its own, after the caches, so it never wraps
    // into the location.
    expect(
      within(paths).getByRole("row", { name: /^Cache/ }),
    ).not.toHaveTextContent("Manage caches");
    expect(
      screen.queryByRole("region", { name: "Server Data" }),
    ).not.toBeInTheDocument();
  });

  it("names a bar segment and its size on hover", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { container } = renderPage();

    const segment = container.querySelector(
      '[data-segment="Libraries"]',
    ) as HTMLElement;
    await user.hover(segment);

    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("Libraries");
    expect(tooltip).toHaveTextContent("203 GB");
    expect(tooltip).toHaveTextContent("20% of the volume");
  });

  it("shows the server's current year of reading by default", () => {
    renderPage();

    const readingSection = section("Reading");
    expect(lastReadingCall().query).toEqual({ year: 2026 });
    expect(
      within(readingSection).getByRole("combobox", { name: "Range" }),
    ).toHaveTextContent("2026");
    const tile = (name: string) =>
      within(readingSection).getByRole("group", { name });
    expect(tile("Reading time")).toHaveTextContent("36h 0m");
    expect(tile("Listening time")).toHaveTextContent("12h 30m");
    expect(tile("Read-throughs finished")).toHaveTextContent("41");
    expect(tile("Did Not Finish")).toHaveTextContent("2");
    expect(tile("Readers")).toHaveTextContent("4");
    expect(
      within(readingSection).getByText("Totals across every user for 2026."),
    ).toBeInTheDocument();
  });

  it("shows all time or a past year from the URL", () => {
    const { unmount } = renderPage("/admin/server-stats?year=all");
    expect(lastReadingCall().query).toEqual({});
    const readingSection = section("Reading");
    expect(
      within(readingSection).getByRole("combobox", { name: "Range" }),
    ).toHaveTextContent("All time");
    expect(
      within(readingSection).getByRole("group", {
        name: "Read-throughs finished",
      }),
    ).toHaveTextContent("130");
    unmount();

    renderPage("/admin/server-stats?year=2025");
    expect(lastReadingCall().query).toEqual({ year: 2025 });
  });

  it("waits for the server's year before asking for reading", () => {
    query = { ...loaded(undefined), isLoading: true };
    renderPage();
    expect(readingCalls.every((c) => c.enabled === false)).toBe(true);
  });

  it("charts the running total, padded out to a year", () => {
    renderPage();

    const growth = section("Library Growth");
    const table = within(growth).getByRole("table", {
      name: "Library growth by month",
    });
    const rows = within(table).getAllByRole("row");
    // Header row plus twelve months: three with data and nine before them.
    expect(rows).toHaveLength(13);
    expect(within(rows[1]).getByText("Dec 2025")).toBeInTheDocument();
    const latest = rows[12];
    expect(within(latest).getByText("Nov 2026")).toBeInTheDocument();
    expect(within(latest).getByText("319")).toBeInTheDocument();
    expect(within(latest).getByText("18,319")).toBeInTheDocument();
    expect(
      within(growth).getByText("18,319 books now, with 319 added in Nov 2026."),
    ).toBeInTheDocument();
  });

  it("says when no books have been added yet", () => {
    query = loaded({
      ...stats,
      libraries: [],
      totals: emptyCounts,
      growth: [],
    });
    renderPage();

    expect(
      within(section("Library Growth")).getByText("No books yet."),
    ).toBeInTheDocument();
    expect(
      within(section("Libraries")).getByText("No libraries yet."),
    ).toBeInTheDocument();
  });

  it("shows the error where the figures go when loading fails", () => {
    query = loaded(undefined, new Error("boom"));
    renderPage();

    expect(
      screen.getByRole("heading", { level: 1, name: "Server Stats" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Failed to load Server Stats",
    );
  });

  it("sets the range in the URL, leaving it out for the current year", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage();
    const range = () =>
      within(section("Reading")).getByRole("combobox", { name: "Range" });

    await user.click(range());
    expect(
      screen.getAllByRole("option").map((option) => option.textContent),
    ).toEqual(["2026", "2025", "All time"]);
    await user.click(screen.getByRole("option", { name: "2025" }));
    expect(search()).toBe("?year=2025");
    expect(lastReadingCall().query).toEqual({ year: 2025 });

    await user.click(range());
    await user.click(screen.getByRole("option", { name: "All time" }));
    expect(search()).toBe("?year=all");

    await user.click(range());
    await user.click(screen.getByRole("option", { name: "2026" }));
    expect(search()).toBe("");
  });

  it.each(["abc", "0000", "0999"])(
    "drops year=%s from the URL and shows the current year",
    (value) => {
      renderPage(`/admin/server-stats?year=${value}`);
      expect(search()).toBe("");
      expect(
        within(section("Reading")).getByRole("combobox", { name: "Range" }),
      ).toHaveTextContent("2026");
      expect(lastReadingCall().query).toEqual({ year: 2026 });
    },
  );

  it("marks the figures as updating while another range loads", () => {
    readingResult = {
      ...loaded(reading(2026)),
      isPlaceholderData: true,
    } as ReturnType<typeof loaded>;
    renderPage("/admin/server-stats?year=2025");

    const readingSection = section("Reading");
    expect(
      within(readingSection).getByRole("status", { name: "Loading reading" }),
    ).toBeInTheDocument();
    expect(
      within(readingSection).getByRole("group", { name: "Reading time" })
        .parentElement,
    ).toHaveAttribute("aria-busy", "true");
  });

  it("reports a failed reading load in its section only", () => {
    readingResult = loaded<ServerReadingResponse>(undefined, new Error("boom"));
    renderPage();

    expect(within(section("Reading")).getByRole("alert")).toHaveTextContent(
      "Failed to load reading",
    );
    expect(section("Overview")).toBeInTheDocument();
    expect(section("Disk Space")).toBeInTheDocument();
  });

  it("totals every library even when the viewer sees only some", () => {
    query = loaded({ ...stats, libraries: [stats.libraries[1]] });
    renderPage();

    const libraries = section("Libraries");
    expect(
      within(libraries).getByRole("row", { name: /All libraries/ }),
    ).toHaveTextContent("18,319");
  });

  it("says when the viewer can reach none of the libraries", () => {
    query = loaded({ ...stats, libraries: [] });
    renderPage();

    expect(
      within(section("Libraries")).getByText("No libraries you can access."),
    ).toBeInTheDocument();
  });

  it("still lists the caches when their volume can't be read", () => {
    query = loaded({ ...stats, volumes: [stats.volumes[0]] });
    renderPage();

    const caches = within(section("Disk Space")).getByRole("table", {
      name: "Caches",
    });
    expect(
      within(caches).getByRole("row", { name: /Cover Thumbnails/ }),
    ).toHaveTextContent("512 MB");
    expect(
      within(caches).getByRole("link", { name: "Manage caches" }),
    ).toBeInTheDocument();
  });
});

describe("AdminServerStats on a phone", () => {
  // jsdom has no matchMedia; this one matches only the phone query.
  beforeEach(() => {
    setAuth({ permissions: ["config:read"] });
    query = loaded(stats);
    readingCalls = [];
    readingResult = null;
    vi.stubGlobal(
      "matchMedia",
      vi.fn((media: string) => ({
        matches: media === "(max-width: 639px)",
        media,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    );
    return () => vi.unstubAllGlobals();
  });

  it("lists each library's counts without a table to scroll", () => {
    renderPage();

    const libraries = section("Libraries");
    expect(within(libraries).queryByRole("table")).not.toBeInTheDocument();
    const fiction = within(libraries).getByRole("listitem", {
      name: "Fiction",
    });
    const figure = (label: string) =>
      within(fiction).getByText(label, { selector: "dt" }).nextElementSibling;
    expect(figure("Books")).toHaveTextContent("18,307");
    expect(figure("Series")).toHaveTextContent("210");
    expect(figure("Publishers")).toHaveTextContent("88");
    expect(figure("Size")).toHaveTextContent("200 GB");
    expect(
      within(fiction).getByRole("list", { name: "Files in Fiction" }),
    ).toBeInTheDocument();
    expect(
      within(libraries).getByRole("listitem", { name: "All libraries" }),
    ).toHaveTextContent("18,319");
  });
});
