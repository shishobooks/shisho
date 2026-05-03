import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
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

import { Dialog } from "@/components/ui/dialog";
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

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

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

vi.mock("@/hooks/queries/entity-search", () => ({
  usePeopleSearch: () => ({ data: [], isLoading: false }),
  useSeriesSearch: () => ({ data: [], isLoading: false }),
  usePublisherSearch: () => ({ data: [], isLoading: false }),
  useImprintSearch: () => ({ data: [], isLoading: false }),
  useGenreSearch: () => ({ data: [], isLoading: false }),
  useTagSearch: () => ({ data: [], isLoading: false }),
}));

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
      <Dialog open>
        <IdentifyReviewForm
          book={opts.book ?? makeBook()}
          fileId={opts.fileId}
          onBack={onBack}
          onClose={onClose}
          result={opts.result ?? makeResult()}
        />
      </Dialog>
    </QueryClientProvider>,
  );
  return { ...view, onClose, onBack };
}

/** Helper to find the apply button regardless of count text. */
function getApplyButton() {
  return screen.getByRole("button", { name: /^Apply (\d+ )?changes?$/i });
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

  it("clears series when the Remove button is pressed on the series row", async () => {
    const user = createUser();
    renderForm({
      result: makeResult({ series: "Some Series", series_number: 1 }),
    });

    const removeButton = await screen.findByRole("button", {
      name: /remove some series/i,
    });
    await user.click(removeButton);

    await user.click(getApplyButton());

    await waitFor(() => {
      expect(applyMock).toHaveBeenCalledTimes(1);
    });

    const payload = applyMock.mock.calls[0][0];
    expect(payload.fields.series).toEqual([]);
  });

  it("does not auto-select a broken plugin cover_url as the default cover", async () => {
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

      await waitFor(() => {
        expect(screen.queryByAltText("New cover")).toBeNull();
      });

      await user.click(getApplyButton());

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

  // -------------------------------------------------------------------------
  // New per-field decisions tests
  // -------------------------------------------------------------------------

  it("omits unchecked fields from the apply payload", async () => {
    const user = createUser();
    renderForm({
      // Single-file book → primary, so book-changed defaults ON.
      book: makeBook({
        title: "Old Title",
        // Authors are also "changed" because the result proposes a new author.
      }),
      result: makeResult({
        title: "New Title",
        authors: [{ name: "New Author" }],
      }),
    });

    // Title checkbox should be checked by default (book-changed on primary).
    const titleCheckbox = screen.getByRole("checkbox", {
      name: /apply title/i,
    });
    expect(titleCheckbox).toHaveAttribute("data-state", "checked");

    // Uncheck title.
    await user.click(titleCheckbox);
    expect(titleCheckbox).toHaveAttribute("data-state", "unchecked");

    // Apply.
    await user.click(getApplyButton());

    await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1));
    const payload = applyMock.mock.calls[0][0];
    expect(payload.fields.title).toBeUndefined();
    // Authors stays checked → still in the payload.
    expect(payload.fields.authors).toEqual([{ name: "New Author" }]);
  });

  it("defaults book-changed fields OFF on a non-primary file", async () => {
    const user = createUser();
    // Two main files, primary is the OTHER one.
    const file1 = makeFile({ id: 1, filepath: "/test/book1.epub" });
    const file2 = makeFile({ id: 2, filepath: "/test/book2.epub" });
    renderForm({
      book: makeBook({
        title: "Old Title",
        primary_file_id: 1,
        files: [file1, file2],
      }),
      fileId: 2, // identifying the non-primary file
      result: makeResult({ title: "New Title" }),
    });

    // Book section is collapsed by default when nothing is selected; expand
    // it so the title checkbox becomes visible.
    await user.click(
      screen.getByRole("button", { name: /toggle book section/i }),
    );

    const titleCheckbox = screen.getByRole("checkbox", {
      name: /apply title/i,
    });
    expect(titleCheckbox).toHaveAttribute("data-state", "unchecked");
  });

  it("defaults book-changed fields ON when identifying the primary file", () => {
    const file1 = makeFile({ id: 1, filepath: "/test/book1.epub" });
    const file2 = makeFile({ id: 2, filepath: "/test/book2.epub" });
    renderForm({
      book: makeBook({
        title: "Old Title",
        primary_file_id: 1,
        files: [file1, file2],
      }),
      fileId: 1,
      result: makeResult({ title: "New Title" }),
    });

    const titleCheckbox = screen.getByRole("checkbox", {
      name: /apply title/i,
    });
    expect(titleCheckbox).toHaveAttribute("data-state", "checked");
  });

  it("defaults file-level fields ON regardless of primary status", () => {
    const file1 = makeFile({ id: 1 });
    const file2 = makeFile({
      id: 2,
      filepath: "/test/book2.epub",
      release_date: "2020-01-01T00:00:00Z",
    });
    renderForm({
      book: makeBook({ primary_file_id: 1, files: [file1, file2] }),
      fileId: 2, // non-primary
      result: makeResult({ release_date: "2024-06-15" }),
    });

    const releaseDateCheckbox = screen.getByRole("checkbox", {
      name: /apply release date/i,
    });
    expect(releaseDateCheckbox).toHaveAttribute("data-state", "checked");
  });

  it("section-level checkbox toggles all child rows", async () => {
    const user = createUser();
    renderForm({
      book: makeBook({ title: "Old Title" }),
      result: makeResult({
        title: "New Title",
        authors: [{ name: "New Author" }],
        genres: ["Fantasy"],
      }),
    });

    // The default "Changed" filter hides unchanged rows, so flip to "All"
    // first to make every book-section checkbox visible.
    await user.click(screen.getByRole("button", { name: /^all$/i }));

    const sectionCheckbox = screen.getByRole("checkbox", {
      name: /apply all book fields/i,
    });
    // Title/authors/genres default ON, but other book fields (subtitle,
    // series, tags, description) are unchanged → off. Aggregate is
    // indeterminate. Clicking indeterminate sets all to true.
    expect(sectionCheckbox).toHaveAttribute("data-state", "indeterminate");
    await user.click(sectionCheckbox);
    expect(sectionCheckbox).toHaveAttribute("data-state", "checked");
    expect(
      screen.getByRole("checkbox", { name: /apply subtitle/i }),
    ).toHaveAttribute("data-state", "checked");

    // Clicking again sets all to false.
    await user.click(sectionCheckbox);
    expect(
      screen.getByRole("checkbox", { name: /apply title/i }),
    ).toHaveAttribute("data-state", "unchecked");
    expect(
      screen.getByRole("checkbox", { name: /apply authors/i }),
    ).toHaveAttribute("data-state", "unchecked");
    expect(
      screen.getByRole("checkbox", { name: /apply genres/i }),
    ).toHaveAttribute("data-state", "unchecked");
  });

  it("emits file_name and file_name_source when Name is checked", async () => {
    const user = createUser();
    renderForm({
      result: makeResult({ title: "Plugin Title" }),
    });

    // Name field defaults ON (file-level) and value defaults to plugin's title.
    await user.click(getApplyButton());

    await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1));
    const payload = applyMock.mock.calls[0][0];
    expect(payload.file_name).toBe("Plugin Title");
    expect(payload.file_name_source).toBe("plugin");
  });

  it("marks file_name_source as user when the Name field is edited", async () => {
    const user = createUser();
    renderForm({
      result: makeResult({ title: "Plugin Title" }),
    });

    // Switch to "All" filter so the row stays visible when temporarily empty.
    await user.click(screen.getByText("All"));

    const nameLabel = screen.getByText("Name");
    const nameRow = nameLabel.closest("div.grid");
    expect(nameRow).not.toBeNull();
    const nameInput = within(nameRow as HTMLElement).getByDisplayValue(
      "Plugin Title",
    );
    await user.clear(nameInput);
    await user.type(nameInput, "Edition Suffix");

    await user.click(getApplyButton());

    await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1));
    const payload = applyMock.mock.calls[0][0];
    expect(payload.file_name).toBe("Edition Suffix");
    expect(payload.file_name_source).toBe("user");
  });

  it("hides unchanged rows in the default Changed filter, shows them in All", async () => {
    const user = createUser();
    renderForm({
      book: makeBook({ title: "Old Title", subtitle: "" }),
      result: makeResult({ title: "New Title" }),
    });

    // Subtitle is unchanged (book has none, result has none) — should be
    // hidden under the default "Changed" filter.
    expect(
      screen.queryByRole("checkbox", { name: /apply subtitle/i }),
    ).toBeNull();

    // Title is changed — visible.
    expect(
      screen.getByRole("checkbox", { name: /apply title/i }),
    ).toBeInTheDocument();

    // Switch to "All" — subtitle row becomes visible.
    await user.click(screen.getByRole("button", { name: /^all$/i }));
    expect(
      screen.getByRole("checkbox", { name: /apply subtitle/i }),
    ).toBeInTheDocument();
  });

  it("disables row inputs when the apply checkbox is unchecked", async () => {
    const user = createUser();
    renderForm({
      book: makeBook({ title: "Old Title" }),
      result: makeResult({ title: "New Title" }),
    });

    // Find the Title row's input-disabled wrapper. The label "Title" is
    // unique to the book Title row.
    const titleLabel = screen.getByText("Title", { selector: "label" });
    const titleRow = titleLabel.closest("div.grid");
    expect(titleRow).not.toBeNull();
    const wrapper = titleRow!.querySelector("[aria-disabled]");
    expect(wrapper).not.toBeNull();
    expect(wrapper).toHaveAttribute("aria-disabled", "false");

    // Uncheck the row checkbox — the wrapper flips to aria-disabled=true.
    await user.click(screen.getByRole("checkbox", { name: /apply title/i }));
    expect(wrapper).toHaveAttribute("aria-disabled", "true");
  });

  it("Restore suggestions resets the form", async () => {
    const user = createUser();
    renderForm({
      book: makeBook({ title: "Old Title" }),
      result: makeResult({ title: "New Title" }),
    });

    // Uncheck title
    const titleCheckbox = screen.getByRole("checkbox", {
      name: /apply title/i,
    });
    await user.click(titleCheckbox);
    expect(titleCheckbox).toHaveAttribute("data-state", "unchecked");

    // Click Restore
    await user.click(
      screen.getByRole("button", { name: /restore suggestions/i }),
    );

    expect(titleCheckbox).toHaveAttribute("data-state", "checked");
  });
});
