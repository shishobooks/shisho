import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";
import type {
  ContentCounts,
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
        { kind: "library", name: "Comics", path: "/media/comics" },
        { kind: "library", name: "Fiction", path: "/media/fiction" },
        { kind: "data", name: "Data", path: "/config" },
      ],
      total_bytes: 1000 * 1024 ** 3,
      used_bytes: 750 * 1024 ** 3,
      free_bytes: 250 * 1024 ** 3,
    },
    {
      paths: [{ kind: "cache", name: "Cache", path: "/cache" }],
      total_bytes: 100 * 1024 ** 3,
      used_bytes: 10 * 1024 ** 3,
      free_bytes: 90 * 1024 ** 3,
    },
  ],
  database_bytes: 48 * 1024 ** 2,
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
  reading: {
    year: 2026,
    this_year: {
      reading_seconds: 36 * 3600,
      listening_seconds: 12 * 3600 + 30 * 60,
      read_throughs_finished: 41,
    },
    all_time: {
      reading_seconds: 120 * 3600,
      listening_seconds: 50 * 3600,
      read_throughs_finished: 130,
    },
    active_users: 3,
  },
};

let query: {
  data?: StatsResponse;
  isLoading: boolean;
  error: Error | null;
  isEnabled: boolean;
  isFetching: boolean;
  refetch: () => void;
};

const loaded = (data?: StatsResponse, error: Error | null = null) => ({
  data,
  isLoading: false,
  error,
  isEnabled: true,
  isFetching: false,
  refetch: vi.fn(),
});

vi.mock("@/hooks/queries/serverStats", () => ({
  useServerStats: () => query,
}));

const renderPage = () =>
  render(
    <MemoryRouter>
      <AdminServerStats />
    </MemoryRouter>,
  );

const section = (name: string) => screen.getByRole("region", { name });

describe("AdminServerStats", () => {
  beforeEach(() => {
    setAuth({ permissions: ["config:read"] });
    query = loaded(stats);
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
  });

  it("shows each library's contents and a total row", () => {
    renderPage();

    const libraries = section("Libraries");
    const fiction = within(libraries).getByRole("row", { name: /Fiction/ });
    expect(within(fiction).getByText("18,307")).toBeInTheDocument();
    expect(
      within(fiction).getByText("EPUB 18,000 · M4B 400 · 7 supplements"),
    ).toBeInTheDocument();
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
    expect(within(volumes[0]).getByText("/media/fiction")).toBeInTheDocument();
    expect(within(volumes[0]).getByText("/config")).toBeInTheDocument();
    expect(within(volumes[0]).getByText("250 GB free")).toBeInTheDocument();
    expect(
      within(volumes[0]).getByText("750 GB used of 1000 GB"),
    ).toBeInTheDocument();
    expect(
      within(volumes[0]).getByRole("progressbar", { name: /used/i }),
    ).toHaveAttribute("aria-valuenow", "75");
    expect(within(volumes[1]).getByText("/cache")).toBeInTheDocument();
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
        },
        {
          paths: [],
          total_bytes: 10 * 1024 ** 3,
          used_bytes: 1 * 1024 ** 3,
          free_bytes: 9 * 1024 ** 3,
        },
      ],
    });
    renderPage();

    const volumes = within(section("Disk Space")).getAllByRole("listitem", {
      name: /volume/i,
    });
    expect(volumes).toHaveLength(2);
    expect(within(volumes[0]).getByText("60 GB free")).toBeInTheDocument();
    expect(within(volumes[0]).queryByRole("list")).not.toBeInTheDocument();
    expect(
      within(volumes[0]).getByText("Holds only library folders you can't see."),
    ).toBeInTheDocument();
  });

  it("shows the database and each cache's size", () => {
    renderPage();

    const data = section("Server Data");
    expect(within(data).getByText("48 MB")).toBeInTheDocument();
    expect(within(data).getByText("512 MB")).toBeInTheDocument();
    expect(within(data).getByText("9,000 files")).toBeInTheDocument();
    expect(
      within(data).getByRole("link", { name: "Manage caches" }),
    ).toHaveAttribute("href", "/admin/cache");
  });

  it("shows reading for this year and all time with no names", () => {
    renderPage();

    const reading = section("Reading");
    const thisYear = within(reading).getByRole("group", { name: "2026" });
    expect(within(thisYear).getByText("36h 0m")).toBeInTheDocument();
    expect(within(thisYear).getByText("12h 30m")).toBeInTheDocument();
    expect(within(thisYear).getByText("41")).toBeInTheDocument();
    const allTime = within(reading).getByRole("group", { name: "All time" });
    expect(within(allTime).getByText("130")).toBeInTheDocument();
    expect(within(reading).getByText("3")).toBeInTheDocument();
    expect(
      within(reading).getByText(/never broken down by user or by book/i),
    ).toBeInTheDocument();
  });

  it("charts books added by month", () => {
    renderPage();

    const growth = section("Library Growth");
    const table = within(growth).getByRole("table", {
      name: "Books added by month",
    });
    const rows = within(table).getAllByRole("row");
    // Header row plus one per month.
    expect(rows).toHaveLength(4);
    expect(within(rows[3]).getByText("Nov 2026")).toBeInTheDocument();
    expect(within(rows[3]).getByText("319")).toBeInTheDocument();
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
});
