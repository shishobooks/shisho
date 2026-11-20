import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { AvailablePlugin } from "@/hooks/queries/plugins";
import { setAuth } from "@/testing/auth";
import type { DetectFileTypesResponse } from "@/types";
import { PluginStatusActive, type Plugin } from "@/types/generated/models";

import { DiscoverTab } from "./DiscoverTab";

// The plugin page offers Setup's plugin recommendations, worked out from the
// libraries' folders, so Getting Started's "Add metadata plugins" step leads
// somewhere that says what to install.

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/hooks/queries/plugins", () => ({
  useInstallPlugin: () => ({ isPending: false, mutate: vi.fn() }),
  usePluginRepositories: () => ({ data: [] }),
  usePluginsAvailable: () => ({
    data: mockAvailable,
    error: null,
    isLoading: false,
  }),
  usePluginOrder: () => ({ data: mockEnricherOrder, isSuccess: true }),
  usePluginsInstalled: () => ({ data: mockInstalled, isSuccess: true }),
  useUpdatePluginVersion: () => ({ isPending: false, mutate: vi.fn() }),
}));

const detectedPaths: string[][] = [];
vi.mock("@/hooks/queries/filesystem", () => ({
  DETECT_MAX_PATHS: 100,
  useDetectFileTypes: (paths: string[]) => {
    detectedPaths.push(paths);
    return paths.length > 0
      ? mockDetection
      : { data: undefined, isEnabled: false, isPending: true };
  },
}));

vi.mock("@/hooks/queries/libraries", () => ({
  useLibraries: ({ enabled }: { enabled: boolean }) =>
    mockListsLibraries && enabled
      ? { data: mockLibraries, isEnabled: true, isPending: false }
      : { data: undefined, isEnabled: false, isPending: true },
}));

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const plugin = (id: string, name: string): AvailablePlugin => ({
  compatible: true,
  description: `${name} metadata`,
  homepage: "",
  id,
  imageUrl: "",
  is_official: true,
  name,
  overview: "",
  scope: "shisho",
  versions: [
    {
      capabilities: {
        identifierTypes: [],
        metadataEnricher: { description: "", fields: [], fileTypes: [] },
      },
      changelog: "",
      compatible: true,
      downloadUrl: "",
      manifestVersion: 1,
      minShishoVersion: "0.1.0",
      releaseDate: "2024-01-01",
      sha256: "",
      version: "1.0.0",
    },
  ],
});

const GOODREADS = plugin("goodreads-enricher", "Goodreads");
const OPEN_LIBRARY = plugin("open-library-enricher", "Open Library");
const AUDIBLE = plugin("audible-enricher", "Audible");
const MANGA = plugin("manga-enricher", "Manga");
const KOBO_STORE = plugin("kobo-store-enricher", "Kobo Store");

const installed = (p: AvailablePlugin): Plugin => ({
  auto_update: true,
  id: p.id,
  installed_at: "2024-01-01T00:00:00Z",
  name: p.name,
  scope: p.scope,
  status: PluginStatusActive,
  version: "1.0.0",
});

const detected = (
  counts: Partial<DetectFileTypesResponse>,
): DetectFileTypesResponse => ({
  audiobooks: 0,
  comics: 0,
  ebooks: 0,
  stopped_early: false,
  ...counts,
});

let mockAvailable: AvailablePlugin[] = [];
let mockInstalled: Plugin[] = [];
// The installed metadata enrichers, by hook order.
let mockEnricherOrder: unknown[] = [];
let mockListsLibraries = true;
let mockLibraries: unknown;
let mockDetection: {
  data: DetectFileTypesResponse | undefined;
  isEnabled: boolean;
  isPending: boolean;
};

const library = (id: number, paths: string[]) => ({
  id,
  library_paths: paths.map((filepath, i) => ({ filepath, id: id * 10 + i })),
  name: `Library ${id}`,
});

const renderTab = (canWrite = true) =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <DiscoverTab canWrite={canWrite} />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const recommendedGroup = () =>
  screen.getByRole("group", { name: "Recommended for your library" });

const rowNames = (container: HTMLElement) =>
  within(container)
    .getAllByRole("link")
    .map((link) => link.textContent);

describe("DiscoverTab recommendations", () => {
  beforeEach(() => {
    setAuth({ permissions: ["config:read", "config:write"] });
    detectedPaths.length = 0;
    mockAvailable = [KOBO_STORE, MANGA, AUDIBLE, OPEN_LIBRARY, GOODREADS];
    mockInstalled = [];
    mockEnricherOrder = [];
    mockListsLibraries = true;
    mockLibraries = {
      items: [library(1, ["/books", "/audio"]), library(2, ["/comics/manga"])],
      total: 2,
    };
    mockDetection = {
      data: detected({ audiobooks: 3, ebooks: 1200 }),
      isEnabled: true,
      isPending: false,
    };
  });

  it("recommends plugins for what the libraries hold, above the rest", () => {
    renderTab();

    expect(detectedPaths).toContainEqual(["/books", "/audio", "/comics/manga"]);
    const group = recommendedGroup();
    expect(
      within(group).getByText(
        "We found about 1,200 ebooks and 3 audiobooks in your libraries.",
      ),
    ).toBeInTheDocument();
    expect(rowNames(group)).toEqual([
      expect.stringContaining("Goodreads"),
      expect.stringContaining("Audible"),
      expect.stringContaining("Open Library"),
    ]);

    // The rest follow once, with no recommended plugin repeated.
    const rest = screen.getByRole("group", { name: "All plugins" });
    expect(rowNames(rest)).toEqual([
      expect.stringContaining("Kobo Store"),
      expect.stringContaining("Manga"),
    ]);
  });

  it("leaves out recommended plugins already installed, such as one turned off", () => {
    mockInstalled = [installed(GOODREADS)];
    renderTab();

    expect(rowNames(recommendedGroup())).toEqual([
      expect.stringContaining("Audible"),
      expect.stringContaining("Open Library"),
    ]);
  });

  it("recommends nothing once a metadata enricher is installed, and skips the folder check", () => {
    mockInstalled = [installed(GOODREADS)];
    mockEnricherOrder = [{ plugin_id: GOODREADS.id, scope: "shisho" }];
    renderTab();

    expect(
      screen.queryByRole("group", { name: "Recommended for your library" }),
    ).toBeNull();
    expect(detectedPaths.every((paths) => paths.length === 0)).toBe(true);
    // The list has no headings of its own then.
    expect(screen.queryByRole("group", { name: "All plugins" })).toBeNull();
  });

  it("looks through at most 100 folders, and says it looked through part of them", () => {
    mockLibraries = {
      items: [
        library(
          1,
          Array.from({ length: 120 }, (_, i) => `/books/${i}`),
        ),
      ],
      total: 1,
    };
    renderTab();

    expect(detectedPaths[detectedPaths.length - 1]).toHaveLength(100);
    expect(
      within(recommendedGroup()).getByText(
        "Your libraries are large, so we looked through part of them and found about 1,200 ebooks and 3 audiobooks.",
      ),
    ).toBeInTheDocument();
  });

  it("gives the main plugins to a role that can't list libraries", () => {
    mockListsLibraries = false;
    renderTab();

    const group = recommendedGroup();
    expect(within(group).queryByText(/your libraries/)).toBeNull();
    expect(rowNames(group)).toEqual([
      expect.stringContaining("Goodreads"),
      expect.stringContaining("Open Library"),
      expect.stringContaining("Audible"),
      expect.stringContaining("Manga"),
    ]);
    expect(detectedPaths.every((paths) => paths.length === 0)).toBe(true);
  });

  it("says it is looking while the folder check runs", () => {
    mockDetection = { data: undefined, isEnabled: true, isPending: true };
    renderTab();

    expect(
      within(recommendedGroup()).getByText("Looking through your libraries…"),
    ).toBeInTheDocument();
  });

  it("falls back to the main plugins, with Goodreads first, for a role that can't check the folders", () => {
    mockDetection = { data: undefined, isEnabled: false, isPending: true };
    renderTab();

    const group = recommendedGroup();
    expect(within(group).queryByText(/your libraries/)).toBeNull();
    expect(rowNames(group)).toEqual([
      expect.stringContaining("Goodreads"),
      expect.stringContaining("Open Library"),
      expect.stringContaining("Audible"),
      expect.stringContaining("Manga"),
    ]);
  });

  it("recommends nothing with no library yet, to a role that can't install, or while searching", async () => {
    mockLibraries = { items: [], total: 0 };
    const { unmount } = renderTab();
    expect(
      screen.queryByRole("group", { name: "Recommended for your library" }),
    ).toBeNull();
    unmount();

    mockLibraries = { items: [library(1, ["/books"])], total: 1 };
    const readOnly = renderTab(false);
    expect(
      screen.queryByRole("group", { name: "Recommended for your library" }),
    ).toBeNull();
    readOnly.unmount();

    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderTab();
    expect(recommendedGroup()).toBeInTheDocument();
    await user.type(screen.getByLabelText("Search plugins"), "a");
    expect(
      screen.queryByRole("group", { name: "Recommended for your library" }),
    ).toBeNull();
  });
});
