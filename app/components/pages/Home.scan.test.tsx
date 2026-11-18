import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { useSSE } from "@/hooks/useSSE";
import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import { emulatePhoneWidth } from "@/testing/phoneWidth";
import {
  StepPlugins,
  type GettingStartedResponse,
} from "@/types/generated/gettingstarted";
import type { Permission } from "@/utils/permissions";

import Home from "./Home";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
  window.matchMedia = vi.fn(() => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("@/libraries/build", () => ({ checkServerBuild: vi.fn() }));

vi.mock("@/components/library/SelectionToolbar", () => ({
  SelectionToolbar: () => null,
}));

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 50));
  });

class MockEventSource {
  static instances: MockEventSource[] = [];
  url: string;
  onerror: (() => void) | null = null;
  onopen: (() => void) | null = null;
  listeners: Record<string, ((event: MessageEvent) => void)[]> = {};
  close = vi.fn();

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    (this.listeners[type] ??= []).push(listener);
  }

  removeEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners[type] = (this.listeners[type] ?? []).filter(
      (l) => l !== listener,
    );
  }

  // Query cache updates reach components on a timer, so this flushes too.
  async emit(type: string, data: object) {
    const event = new MessageEvent(type, { data: JSON.stringify(data) });
    act(() => {
      this.listeners[type]?.forEach((l) => l(event));
    });
    await flush();
  }
}

const stream = () => MockEventSource.instances[0];

// The app mounts useSSE once at the root; the page reads what it receives.
const WithSSE = ({ children }: { children: ReactNode }) => {
  useSSE();
  return <>{children}</>;
};

interface ServerState {
  gettingStarted?: GettingStartedResponse;
  books: { id: number; library_id: number; title: string; files: [] }[];
  job: {
    id: number;
    status: string;
    data: object;
    library_id?: number | null;
  } | null;
}

const book = (id: number, title: string) => ({
  id,
  library_id: 1,
  title,
  files: [] as [],
});

const renderLibraryPage = (
  server: ServerState,
  permissions: Permission[] = ["books:read", "jobs:read", "jobs:write"],
) => {
  setAuth({ permissions, user: { username: "admin" } });
  const request = vi
    .spyOn(API, "request")
    .mockImplementation(async (method, path) => {
      if (method === "POST" && path === "/jobs") {
        return { id: 10, status: "pending", type: "scan", data: {} };
      }
      if (path === "/settings/user") return { gallery_size: "m" };
      if (path === "/settings/libraries/1") return { sort_spec: null };
      if (path === "/user/libraries" || path.endsWith("/languages")) return [];
      if (path === "/libraries/1") return { id: 1, name: "Books" };
      if (path === "/jobs") {
        return server.job
          ? {
              items: [
                {
                  library_id: 1,
                  ...server.job,
                  type: "scan",
                  created_at: new Date().toISOString(),
                  updated_at: new Date().toISOString(),
                },
              ],
              total: 1,
            }
          : { items: [], total: 0 };
      }
      if (path === "/getting-started") {
        return (
          server.gettingStarted ?? {
            enabled: false,
            dismissed: false,
            auto_open: false,
            steps: [],
          }
        );
      }
      if (path === "/books") {
        return { items: server.books, total: server.books.length };
      }
      return { items: [], total: 0 };
    });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter initialEntries={["/libraries/1"]}>
        <WithSSE>
          <Routes>
            <Route element={<Home />} path="/libraries/:libraryId" />
          </Routes>
        </WithSSE>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return request;
};

const scanHeader = () =>
  screen.queryByRole("status", { name: /scanning/i }) ?? null;

describe("Library page during a scan", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    MockEventSource.instances = [];
    vi.stubGlobal("EventSource", MockEventSource);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows a scanning header with the live counts", async () => {
    renderLibraryPage({
      books: [],
      job: { id: 9, status: "in_progress", data: {} },
    });
    await flush();

    expect(scanHeader()).toBeInTheDocument();
    expect(scanHeader()).toHaveTextContent("Scanning this library");

    await stream().emit("scan.progress", {
      job_id: 9,
      library_id: 1,
      scan_for_new_metadata: false,
      files_found: 1204,
      files_scanned: 10,
      books_imported: 1,
      done: false,
    });

    expect(scanHeader()).toHaveTextContent("1,204 files found");
    expect(scanHeader()).toHaveTextContent("1 book imported");
  });

  it("ignores progress for another library", async () => {
    renderLibraryPage({
      books: [],
      job: { id: 9, status: "in_progress", data: {} },
    });
    await flush();

    await stream().emit("scan.progress", {
      job_id: 9,
      library_id: 2,
      scan_for_new_metadata: false,
      files_found: 50,
      files_scanned: 0,
      books_imported: 0,
      done: false,
    });

    expect(scanHeader()).not.toHaveTextContent("50 files found");
  });

  it("shows Books as the scan imports them, before it ends", async () => {
    const server: ServerState = {
      books: [],
      job: { id: 9, status: "in_progress", data: {} },
    };
    renderLibraryPage(server);
    await flush();

    // An empty gallery mid-scan says Books are on their way, not that the
    // library is empty.
    expect(screen.queryByText("No books yet")).not.toBeInTheDocument();
    expect(
      screen.getByText(/books show up here as the scan imports them/i),
    ).toBeInTheDocument();

    server.books = [book(1, "Dune"), book(2, "Emma")];
    await stream().emit("scan.progress", {
      job_id: 9,
      library_id: 1,
      scan_for_new_metadata: false,
      files_found: 300,
      files_scanned: 2,
      books_imported: 2,
      done: false,
    });
    await act(async () => {
      vi.advanceTimersByTime(5_000);
    });
    await flush();

    expect(screen.getByText("Dune")).toBeInTheDocument();
    expect(screen.getByText("Emma")).toBeInTheDocument();
    expect(scanHeader()).toBeInTheDocument();
  });

  it("hands back to the normal gallery and empty state when the scan ends", async () => {
    const server: ServerState = {
      books: [],
      job: { id: 9, status: "in_progress", data: {} },
    };
    renderLibraryPage(server);
    await flush();
    expect(scanHeader()).toBeInTheDocument();

    server.job = { id: 9, status: "completed", data: {} };
    await stream().emit("job.status_changed", {
      job_id: 9,
      status: "completed",
      type: "scan",
    });
    await flush();

    expect(scanHeader()).not.toBeInTheDocument();
    expect(screen.getByText("No books yet")).toBeInTheDocument();
  });

  it("doesn't bring the header back when the job ends after the library is done", async () => {
    // The job's status reaches the page only on its next refetch, so it
    // still reads in progress when the job's end is announced.
    renderLibraryPage({
      books: [],
      job: { id: 9, status: "in_progress", data: {} },
    });
    await flush();

    await stream().emit("scan.progress", {
      job_id: 9,
      library_id: 1,
      scan_for_new_metadata: false,
      files_found: 3,
      files_scanned: 3,
      books_imported: 3,
      done: true,
    });
    expect(scanHeader()).not.toBeInTheDocument();

    await stream().emit("job.status_changed", {
      job_id: 9,
      status: "completed",
      type: "scan",
    });
    expect(scanHeader()).not.toBeInTheDocument();
  });

  it("keeps a library's counts when another scan ends", async () => {
    renderLibraryPage({
      books: [],
      job: { id: 9, status: "in_progress", data: {} },
    });
    await flush();
    await stream().emit("scan.progress", {
      job_id: 9,
      library_id: 1,
      scan_for_new_metadata: false,
      files_found: 30,
      files_scanned: 5,
      books_imported: 5,
      done: false,
    });

    await stream().emit("job.status_changed", {
      job_id: 10,
      status: "completed",
      type: "scan",
      library_id: 2,
    });

    expect(scanHeader()).toHaveTextContent("30 files found");
  });

  it("names a scan of every library until it reports on this one", async () => {
    renderLibraryPage({
      books: [],
      job: { id: 9, status: "in_progress", data: {}, library_id: null },
    });
    await flush();

    expect(scanHeader()).toHaveTextContent("Scanning your libraries");
    expect(scanHeader()).not.toHaveTextContent("Looking for files");

    await stream().emit("scan.progress", {
      job_id: 9,
      library_id: 1,
      scan_for_new_metadata: false,
      files_found: 8,
      files_scanned: 0,
      books_imported: 0,
      done: false,
    });
    expect(scanHeader()).toHaveTextContent("Scanning this library");
    expect(scanHeader()).toHaveTextContent("8 files found");
  });

  it("refetches Books at most every few seconds, and only when some were imported", async () => {
    const server: ServerState = {
      books: [],
      job: { id: 9, status: "in_progress", data: {} },
    };
    const request = renderLibraryPage(server);
    await flush();
    const booksRequests = () =>
      request.mock.calls.filter((call) => call[1] === "/books").length;
    const progress = (imported: number) => ({
      job_id: 9,
      library_id: 1,
      scan_for_new_metadata: false,
      files_found: 100,
      files_scanned: imported,
      books_imported: imported,
      done: false,
    });

    // Nothing imported yet: no refetch.
    const before = booksRequests();
    await stream().emit("scan.progress", progress(0));
    expect(booksRequests()).toBe(before);

    // The first import refetches at once.
    server.books = [book(1, "Dune")];
    await stream().emit("scan.progress", progress(1));
    expect(screen.getByText("Dune")).toBeInTheDocument();

    // The next one waits out the interval.
    server.books = [book(1, "Dune"), book(2, "Emma")];
    await stream().emit("scan.progress", progress(2));
    expect(screen.queryByText("Emma")).not.toBeInTheDocument();
    await act(async () => {
      vi.advanceTimersByTime(3_000);
    });
    await flush();
    expect(screen.getByText("Emma")).toBeInTheDocument();
  });

  it("shows the header from progress events to a role that can't read jobs", async () => {
    const request = renderLibraryPage({ books: [], job: null }, ["books:read"]);
    await flush();
    expect(request.mock.calls.map((call) => call[1])).not.toContain("/jobs");
    expect(scanHeader()).not.toBeInTheDocument();

    await stream().emit("scan.progress", {
      job_id: 9,
      library_id: 1,
      scan_for_new_metadata: false,
      files_found: 20,
      files_scanned: 5,
      books_imported: 5,
      done: false,
    });
    expect(scanHeader()).toHaveTextContent("5 books imported");

    await stream().emit("scan.progress", {
      job_id: 9,
      library_id: 1,
      scan_for_new_metadata: false,
      files_found: 20,
      files_scanned: 20,
      books_imported: 20,
      done: true,
    });
    expect(scanHeader()).not.toBeInTheDocument();
  });

  it("names the library-wide Scan for new metadata", async () => {
    renderLibraryPage({
      books: [],
      job: { id: 9, status: "in_progress", data: {} },
    });
    await flush();

    // While it is still finding files, there is nothing scanned to count.
    await stream().emit("scan.progress", {
      job_id: 9,
      library_id: 1,
      scan_for_new_metadata: true,
      files_found: 40,
      files_scanned: 0,
      books_imported: 0,
      done: false,
    });
    expect(scanHeader()).toHaveTextContent("40 files found");
    expect(scanHeader()).not.toHaveTextContent("of 40");

    await stream().emit("scan.progress", {
      job_id: 9,
      library_id: 1,
      scan_for_new_metadata: true,
      files_found: 400,
      files_scanned: 120,
      books_imported: 0,
      done: false,
    });

    expect(scanHeader()).toHaveTextContent("Scanning for new metadata");
    expect(scanHeader()).toHaveTextContent("120 of 400 files scanned");
  });

  it("points to Getting Started while the user has steps left", async () => {
    renderLibraryPage({
      books: [],
      job: { id: 9, status: "in_progress", data: {} },
      gettingStarted: {
        enabled: true,
        dismissed: false,
        auto_open: false,
        steps: [
          {
            key: StepPlugins,
            applies: true,
            done: false,
            not_for_me: false,
          },
        ],
      },
    });
    await flush();

    expect(
      within(scanHeader()!).getByRole("button", { name: /getting started/i }),
    ).toBeInTheDocument();
  });

  it("doesn't point to a Getting Started list the user dismissed", async () => {
    renderLibraryPage({
      books: [],
      job: { id: 9, status: "in_progress", data: {} },
      gettingStarted: {
        enabled: true,
        dismissed: true,
        auto_open: false,
        steps: [],
      },
    });
    await flush();

    expect(scanHeader()).toBeInTheDocument();
    expect(
      within(scanHeader()!).queryByRole("button", { name: /getting started/i }),
    ).not.toBeInTheDocument();
  });

  describe("at phone width", () => {
    emulatePhoneWidth();

    it("still shows the scanning header", async () => {
      renderLibraryPage({
        books: [],
        job: { id: 9, status: "in_progress", data: {} },
      });
      await flush();
      await stream().emit("scan.progress", {
        job_id: 9,
        library_id: 1,
        scan_for_new_metadata: false,
        files_found: 12,
        files_scanned: 3,
        books_imported: 3,
        done: false,
      });

      expect(scanHeader()).toBeVisible();
      expect(within(scanHeader()!).getByText(/12 files found/)).toBeVisible();
    });
  });
});

describe("Library page Scan for new metadata action", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    MockEventSource.instances = [];
    vi.stubGlobal("EventSource", MockEventSource);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("starts the library-wide scan after confirming", async () => {
    const request = renderLibraryPage({ books: [book(1, "Dune")], job: null });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await flush();

    await user.click(screen.getByRole("button", { name: "Library actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Scan for new metadata" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent(/manual edits/i);
    await user.click(
      within(dialog).getByRole("button", { name: "Start scan" }),
    );
    await flush();

    expect(request).toHaveBeenCalledWith(
      "POST",
      "/jobs",
      {
        type: "scan",
        library_id: 1,
        data: { scan_for_new_metadata: true },
      },
      null,
    );
  });

  it("is offered only to a role that can start a resync", async () => {
    renderLibraryPage({ books: [book(1, "Dune")], job: null }, ["books:read"]);
    await flush();

    expect(
      screen.queryByRole("button", { name: "Library actions" }),
    ).not.toBeInTheDocument();
  });
});
