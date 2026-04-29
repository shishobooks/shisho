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

import type { PluginSearchResult } from "@/hooks/queries/plugins";
import {
  DataSourceManual,
  FileRoleMain,
  FileTypeEPUB,
  FileTypeM4B,
  type Book,
  type File,
} from "@/types";

import { resolveIdentifiers } from "./identify-utils";
import { IdentifyReviewForm } from "./IdentifyReviewForm";

// ---------------------------------------------------------------------------
// Original test (kept intact)
// ---------------------------------------------------------------------------

describe("resolveIdentifiers (incoming wins on type conflict)", () => {
  it("replaces an existing identifier when incoming has the same type with a different value", () => {
    const current = [{ type: "asin", value: "B01ABC1234" }];
    const incoming = [{ type: "asin", value: "B02DEF5678" }];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([{ type: "asin", value: "B02DEF5678" }]);
  });
});

// ---------------------------------------------------------------------------
// Component-level tests
// ---------------------------------------------------------------------------

// Vite normally injects __APP_VERSION__; provide a stub so any code that
// touches it under jsdom doesn't blow up.
beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

// Mock the apply mutation so we can inspect the payload without a real
// network. usePluginIdentifierTypes returns the same shape the component
// expects.
const applyMock = vi.fn();
vi.mock("@/hooks/queries/plugins", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/queries/plugins")
  >("@/hooks/queries/plugins");
  return {
    ...actual,
    usePluginApply: () => ({
      mutateAsync: applyMock,
      isPending: false,
    }),
    usePluginIdentifierTypes: () => ({ data: [] }),
  };
});

// Stub every entity-list hook with empty results. Spread the real module so
// helper exports the auto-match hook depends on (QueryKey enums, types) keep
// working.
vi.mock("@/hooks/queries/people", async () => {
  const actual = await vi.importActual<typeof import("@/hooks/queries/people")>(
    "@/hooks/queries/people",
  );
  return {
    ...actual,
    usePeopleList: () => ({
      data: { people: [], total: 0 },
      isLoading: false,
    }),
  };
});
vi.mock("@/hooks/queries/series", async () => {
  const actual = await vi.importActual<typeof import("@/hooks/queries/series")>(
    "@/hooks/queries/series",
  );
  return {
    ...actual,
    useSeriesList: () => ({
      data: { series: [], total: 0 },
      isLoading: false,
    }),
  };
});
vi.mock("@/hooks/queries/publishers", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/queries/publishers")
  >("@/hooks/queries/publishers");
  return {
    ...actual,
    usePublishersList: () => ({
      data: { publishers: [], total: 0 },
      isLoading: false,
    }),
  };
});
vi.mock("@/hooks/queries/imprints", async () => {
  const actual = await vi.importActual<
    typeof import("@/hooks/queries/imprints")
  >("@/hooks/queries/imprints");
  return {
    ...actual,
    useImprintsList: () => ({
      data: { imprints: [], total: 0 },
      isLoading: false,
    }),
  };
});
vi.mock("@/hooks/queries/genres", async () => {
  const actual = await vi.importActual<typeof import("@/hooks/queries/genres")>(
    "@/hooks/queries/genres",
  );
  return {
    ...actual,
    useGenresList: () => ({
      data: { genres: [], total: 0 },
      isLoading: false,
    }),
  };
});
vi.mock("@/hooks/queries/tags", async () => {
  const actual = await vi.importActual<typeof import("@/hooks/queries/tags")>(
    "@/hooks/queries/tags",
  );
  return {
    ...actual,
    useTagsList: () => ({
      data: { tags: [], total: 0 },
      isLoading: false,
    }),
  };
});

// useAutoMatchEntities calls API.request directly via useQueries. Returning
// empty lists means any incoming entity name is treated as pending creation.
vi.mock("@/libraries/api", async () => {
  const actual =
    await vi.importActual<typeof import("@/libraries/api")>("@/libraries/api");
  return {
    ...actual,
    API: {
      request: vi.fn(async (_method: string, path: string) => {
        if (path === "/people") return { people: [], total: 0 };
        if (path === "/series") return { series: [], total: 0 };
        if (path === "/publishers") return { publishers: [], total: 0 };
        if (path === "/imprints") return { imprints: [], total: 0 };
        if (path === "/genres") return { genres: [], total: 0 };
        if (path === "/tags") return { tags: [], total: 0 };
        return null;
      }),
    },
  };
});

// Suppress Radix-internal act() warnings — same workaround used by
// FileEditDialog.test.tsx.
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

// Match FileEditDialog's userEvent setup so chained interactions don't stall
// under fake timers.
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
    title: "Some Title",
    title_source: DataSourceManual,
    sort_title: "",
    sort_title_source: DataSourceManual,
    author_source: DataSourceManual,
    files: [makeFile()],
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    ...overrides,
  } as Book;
}

function makeResult(
  overrides: Partial<PluginSearchResult> = {},
): PluginSearchResult {
  return {
    title: "Some Title",
    plugin_scope: "library",
    plugin_id: "test",
    ...overrides,
  };
}

function renderForm(
  opts: {
    book?: Book;
    result?: PluginSearchResult;
    fileId?: number;
  } = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const onClose = vi.fn();
  const onBack = vi.fn();
  const view = render(
    <QueryClientProvider client={queryClient}>
      <IdentifyReviewForm
        book={opts.book ?? makeBook()}
        fileId={opts.fileId}
        onBack={onBack}
        onClose={onClose}
        result={opts.result ?? makeResult()}
      />
    </QueryClientProvider>,
  );
  return { ...view, onClose, onBack };
}

describe("IdentifyReviewForm component", () => {
  beforeEach(() => {
    applyMock.mockReset();
    applyMock.mockResolvedValue(undefined);
  });

  it("hides the Narrators field for non-M4B files", () => {
    renderForm({
      book: makeBook({ files: [makeFile({ file_type: FileTypeEPUB })] }),
      result: makeResult({ narrators: ["Some Narrator"] }),
    });

    // Even when the plugin proposes narrators, the field must stay hidden for
    // formats that don't carry narrator metadata.
    expect(screen.queryByText("Narrators")).toBeNull();
  });

  it("shows the Narrators field for M4B files", () => {
    renderForm({
      book: makeBook({
        files: [
          makeFile({ file_type: FileTypeM4B, filepath: "/test/book.m4b" }),
        ],
      }),
      result: makeResult({ narrators: ["Some Narrator"] }),
    });

    expect(screen.getByText("Narrators")).toBeInTheDocument();
  });

  it("clears series and series_number when the Clear series button is pressed", async () => {
    const user = createUser();
    renderForm({
      result: makeResult({ series: "Some Series", series_number: 1 }),
    });

    // The clear button is rendered next to the series combobox when a value
    // is set. Confirm it picked up the incoming series first.
    const clearButton = await screen.findByRole("button", {
      name: /clear series/i,
    });
    await user.click(clearButton);

    await user.click(screen.getByRole("button", { name: /apply changes/i }));

    await waitFor(() => {
      expect(applyMock).toHaveBeenCalledTimes(1);
    });

    const payload = applyMock.mock.calls[0][0];
    expect(payload.fields.series).toBe("");
    // series_number is omitted when the input is empty (parseFloat fallback).
    expect(payload.fields.series_number).toBeUndefined();
  });

  it("does not auto-select a broken plugin cover_url as the default cover", async () => {
    // Simulate every Image() load failing — mirrors plugins returning a
    // cover_url that 404s (e.g. a goodreads `nophoto` placeholder URL or a
    // dead CDN link). With the bug, the form would default coverSelection to
    // "new" and POST cover_url to the apply endpoint anyway.
    const OriginalImage = globalThis.Image;
    class FailingImage {
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      naturalWidth = 0;
      naturalHeight = 0;
      _src = "";
      get src() {
        return this._src;
      }
      set src(v: string) {
        this._src = v;
        Promise.resolve().then(() => this.onerror?.());
      }
    }
    // @ts-expect-error - jsdom Image stub
    globalThis.Image = FailingImage;

    try {
      const user = createUser();
      renderForm({
        book: makeBook({
          files: [
            makeFile({
              file_type: FileTypeEPUB,
              cover_image_filename: "book.cover.jpg",
            }),
          ],
        }),
        result: makeResult({
          cover_url: "https://example.com/broken-cover.jpg",
        }),
      });

      // Wait for the useImageDimensions effects to fire onerror and unmount
      // the cover swap UI.
      await waitFor(() => {
        expect(screen.queryByAltText("New cover")).toBeNull();
      });

      await user.click(screen.getByRole("button", { name: /apply changes/i }));

      await waitFor(() => {
        expect(applyMock).toHaveBeenCalledTimes(1);
      });

      const payload = applyMock.mock.calls[0][0];
      expect(payload.fields.cover_url).toBeUndefined();
      expect(payload.fields.cover_page).toBeUndefined();
    } finally {
      globalThis.Image = OriginalImage;
    }
  });

  it("renders the publisher combobox with the pendingCreate dashed border for unmatched names", async () => {
    renderForm({
      result: makeResult({ publisher: "Brand New Publisher" }),
    });

    // EntityCombobox renders the value as the trigger label inside a span,
    // so the button has no accessible name attribute we can target with
    // getByRole. Find the button by its visible text content instead, then
    // walk up to the actual <button role="combobox"> wrapper.
    await waitFor(() => {
      const label = screen.getByText("Brand New Publisher");
      const trigger = label.closest('button[role="combobox"]');
      expect(trigger).not.toBeNull();
      expect(trigger?.className).toContain("border-dashed");
    });
  });
});
