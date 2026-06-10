import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  afterAll,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import type { PluginSearchResponse } from "@/hooks/queries/plugins";
import {
  DataSourceManual,
  FileRoleMain,
  FileTypeEPUB,
  type Book,
  type File,
} from "@/types";

import { IdentifyBookDialog } from "./IdentifyBookDialog";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

// Silence the act() warnings that async query resolution triggers in jsdom.
const originalConsoleError = console.error;
beforeAll(() => {
  console.error = (...args: unknown[]) => {
    if (
      typeof args[0] === "string" &&
      args[0].includes("was not wrapped in act")
    ) {
      return;
    }
    originalConsoleError(...args);
  };
});
afterAll(() => {
  console.error = originalConsoleError;
});

// Mock the ancillary plugin hooks so they don't hit the network, but leave
// usePluginSearch (the hook under test) using its real implementation against
// the mocked API.request below.
vi.mock("@/hooks/queries/plugins", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/queries/plugins")
  >("@/hooks/queries/plugins");
  return {
    ...actual,
    usePluginIdentifierTypes: () => ({ data: [] }),
    usePluginOrder: () => ({ data: [{ scope: "shisho", id: "test" }] }),
  };
});

// Deferred-promise harness keyed by the query string in the search payload.
// Lets a test resolve searches out of submission order.
type Deferred = {
  resolve: (value: PluginSearchResponse) => void;
  promise: Promise<PluginSearchResponse>;
};
const deferredsByQuery = new Map<string, Deferred>();
function deferredFor(query: string): Deferred {
  let d = deferredsByQuery.get(query);
  if (!d) {
    let resolve!: (value: PluginSearchResponse) => void;
    const promise = new Promise<PluginSearchResponse>((res) => {
      resolve = res;
    });
    d = { resolve, promise };
    deferredsByQuery.set(query, d);
  }
  return d;
}

const requestMock = vi.fn();
vi.mock("@/libraries/api", async () => {
  const actual =
    await vi.importActual<typeof import("@/libraries/api")>("@/libraries/api");
  return {
    ...actual,
    API: {
      request: (...args: unknown[]) => requestMock(...args),
    },
  };
});

const createUser = () =>
  userEvent.setup({
    advanceTimers: vi.advanceTimersByTime,
    delay: null,
  });

function makeFile(overrides: Partial<File> = {}): File {
  return {
    id: 1,
    book_id: 1,
    library_id: 1,
    filepath: "/test/book.epub",
    file_type: FileTypeEPUB,
    file_role: FileRoleMain,
    filesize_bytes: 1000,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    narrators: [],
    identifiers: [],
    ...overrides,
  } as File;
}

function makeBook(overrides: Partial<Book> = {}): Book {
  return {
    id: 1,
    library_id: 1,
    filepath: "/test/book.epub",
    title: "Query A",
    title_source: DataSourceManual,
    sort_title: "",
    sort_title_source: DataSourceManual,
    author_source: DataSourceManual,
    authors: [],
    files: [makeFile()],
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    ...overrides,
  } as Book;
}

// PluginSearchResult extends the generated ParsedMetadata, whose non-pointer
// Go fields are always present on the wire; fill them with zero values here.
function response(title: string): PluginSearchResponse {
  return {
    results: [
      {
        title,
        subtitle: "",
        authors: [],
        narrators: [],
        series: "",
        genres: [],
        tags: [],
        description: "",
        publisher: "",
        url: "",
        cover_mime_type: "",
        cover_url: "",
        duration: 0,
        bitrate_bps: 0,
        identifiers: [],
        chapters: [],
        plugin_scope: "shisho",
        plugin_id: "test",
      },
    ],
    total_plugins: 1,
  };
}

function renderDialog(book: Book) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  });
  const onOpenChange = vi.fn();
  const view = render(
    <QueryClientProvider client={queryClient}>
      <IdentifyBookDialog book={book} onOpenChange={onOpenChange} open />
    </QueryClientProvider>,
  );
  return { ...view, onOpenChange };
}

describe("IdentifyBookDialog search sequencing", () => {
  beforeEach(() => {
    deferredsByQuery.clear();
    requestMock.mockReset();
    // Each /plugins/search call returns the deferred keyed by its query, so
    // the test controls resolution order. The query's AbortSignal is the last
    // arg; record it so we can assert supersession cancels the older request.
    requestMock.mockImplementation(
      (
        _method: string,
        path: string,
        payload: { query: string } | null,
        _query: unknown,
        signal?: AbortSignal,
      ) => {
        if (path === "/plugins/search" && payload) {
          const d = deferredFor(payload.query);
          // Reject (abort) when the query's signal fires so a superseded
          // request settles as cancelled instead of resolving stale data.
          return new Promise<PluginSearchResponse>((resolve, reject) => {
            d.promise.then(resolve);
            signal?.addEventListener("abort", () =>
              reject(new DOMException("aborted", "AbortError")),
            );
          });
        }
        return Promise.resolve(null);
      },
    );
  });

  it("renders the most-recently-submitted query's results even when an earlier search resolves last", async () => {
    const user = createUser();
    // Opening the dialog auto-searches "Query A".
    renderDialog(makeBook({ title: "Query A" }));

    // Wait for the auto-search request to fire.
    await waitFor(() =>
      expect(requestMock).toHaveBeenCalledWith(
        "POST",
        "/plugins/search",
        expect.objectContaining({ query: "Query A" }),
        null,
        expect.anything(),
      ),
    );

    // Submit a new query "Query B" while "Query A" is still in flight.
    const input = screen.getByPlaceholderText(/Search by title/i);
    await user.clear(input);
    await user.type(input, "Query B");
    await user.keyboard("{Enter}");

    await waitFor(() =>
      expect(requestMock).toHaveBeenCalledWith(
        "POST",
        "/plugins/search",
        expect.objectContaining({ query: "Query B" }),
        null,
        expect.anything(),
      ),
    );

    // Resolve B first, then resolve the superseded A LAST.
    deferredFor("Query B").resolve(response("Result B"));
    await waitFor(() =>
      expect(screen.getByText("Result B")).toBeInTheDocument(),
    );

    deferredFor("Query A").resolve(response("Result A"));

    // The stale A result must never replace B's results.
    await waitFor(() => {
      expect(screen.getByText("Result B")).toBeInTheDocument();
    });
    expect(screen.queryByText("Result A")).not.toBeInTheDocument();
  });

  it("aborts the superseded request when a newer search is submitted", async () => {
    const user = createUser();
    renderDialog(makeBook({ title: "Query A" }));

    let abortedA = false;
    await waitFor(() => expect(requestMock).toHaveBeenCalled());
    // Find the AbortSignal handed to the first (auto-search) request and watch
    // it for abort.
    const firstCall = requestMock.mock.calls.find(
      (c) => (c[2] as { query?: string })?.query === "Query A",
    );
    const signalA = firstCall?.[4] as AbortSignal | undefined;
    expect(signalA).toBeInstanceOf(AbortSignal);
    signalA?.addEventListener("abort", () => {
      abortedA = true;
    });

    const input = screen.getByPlaceholderText(/Search by title/i);
    await user.clear(input);
    await user.type(input, "Query B");
    await user.keyboard("{Enter}");

    await waitFor(() => expect(abortedA).toBe(true));
  });
});
