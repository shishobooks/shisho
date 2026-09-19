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
  DataSourceFilepath,
  DataSourceManual,
  FileRoleMain,
  FileTypeCBZ,
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
  useGenreSearch: () => ({ data: [], isLoading: false }),
  useTagSearch: () => ({ data: [], isLoading: false }),
  useGenreItemCounts: () => new Map(),
  useTagItemCounts: () => new Map(),
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

// PluginSearchResult extends the generated ParsedMetadata, whose non-pointer
// Go fields are always present on the wire; fill them with zero values here so
// tests only spell out what they assert on.
function makeResult(
  overrides: Partial<PluginSearchResult> = {},
): PluginSearchResult {
  return {
    title: "Some Title",
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
    onHasChangesChange?: (hasChanges: boolean) => void;
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
          onHasChangesChange={opts.onHasChangesChange}
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

  it("surfaces an incoming omnibus series range", () => {
    renderForm({
      result: makeResult({
        series: "Some Series",
        series_number: 1,
        series_number_end: 3,
      }),
    });

    expect(
      screen.getByRole("spinbutton", { name: "Series start" }),
    ).toHaveValue(1);
    expect(screen.getByRole("spinbutton", { name: "Series end" })).toHaveValue(
      3,
    );
  });

  it("submits an incoming omnibus range as one series number group", async () => {
    const user = createUser();
    renderForm({
      result: makeResult({
        series: "Some Series",
        series_number: 1,
        series_number_end: 3,
        series_number_unit: "volume",
      }),
    });

    await user.click(getApplyButton());

    await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1));
    expect(applyMock.mock.calls[0][0].fields.series).toEqual([
      {
        name: "Some Series",
        number: 1,
        series_number_end: 3,
        series_number_unit: "volume",
      },
    ]);
  });

  it("rejects incomplete and reversed series ranges", async () => {
    const user = createUser();
    renderForm({
      result: makeResult({
        series: "Some Series",
        series_number: 1,
        series_number_end: 3,
      }),
    });

    const start = screen.getByRole("spinbutton", { name: "Series start" });
    const apply = getApplyButton();

    await user.clear(start);
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Series range end requires a start",
    );
    expect(apply).toBeDisabled();

    await user.type(start, "3");
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Series range end must be greater than its start",
    );
    expect(apply).toBeDisabled();

    expect(applyMock).not.toHaveBeenCalled();
  });

  it("treats the seriesNumber disabled-field alias as the complete series group", () => {
    renderForm({
      result: makeResult({
        disabled_fields: ["seriesNumber"],
        series: "Some Series",
        series_number: 1,
        series_number_end: 3,
      }),
    });

    expect(
      screen.queryByRole("checkbox", { name: "Apply Series" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("spinbutton", { name: "Series start" }),
    ).not.toBeInTheDocument();
  });

  it("tracks series end edits and restores the suggested range", async () => {
    const user = createUser();
    const onHasChangesChange = vi.fn();
    renderForm({
      onHasChangesChange,
      result: makeResult({
        series: "Some Series",
        series_number: 1,
        series_number_end: 3,
      }),
    });

    const end = screen.getByRole("spinbutton", { name: "Series end" });
    await user.clear(end);
    await user.type(end, "4");
    await waitFor(() =>
      expect(onHasChangesChange).toHaveBeenLastCalledWith(true),
    );

    await user.click(
      screen.getByRole("button", { name: "Restore suggestions" }),
    );
    expect(screen.getByRole("spinbutton", { name: "Series end" })).toHaveValue(
      3,
    );
  });

  it("accepting a single series number replaces an existing range group", async () => {
    const user = createUser();
    renderForm({
      book: makeBook({
        book_series: [
          {
            book_id: 1,
            series_id: 10,
            series_number: 1,
            series_number_end: 3,
            series_number_unit: "volume",
            series: { id: 10, library_id: 1, name: "Some Series" },
          } as never,
        ],
      }),
      result: makeResult({
        series: "Some Series",
        series_number: 1,
        series_number_unit: "volume",
      }),
    });

    await user.click(getApplyButton());

    await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1));
    expect(applyMock.mock.calls[0][0].fields.series).toEqual([
      {
        name: "Some Series",
        number: 1,
        series_number_end: undefined,
        series_number_unit: "volume",
      },
    ]);
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
      // Low-priority sources → book-changed defaults ON.
      book: makeBook({
        title: "Old Title",
        title_source: DataSourceFilepath,
        author_source: DataSourceFilepath,
      }),
      result: makeResult({
        title: "New Title",
        authors: [{ name: "New Author", role: "" }],
      }),
    });

    // Title checkbox should be checked by default (low-priority source).
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
    expect(payload.fields.authors).toEqual([{ name: "New Author", role: "" }]);
  });

  it("defaults book-changed fields OFF when source is high-priority", async () => {
    const user = createUser();
    renderForm({
      book: makeBook({
        title: "Old Title",
        title_source: DataSourceManual, // high-priority → default OFF
      }),
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

  it("defaults book-changed fields ON when source is low-priority", () => {
    renderForm({
      book: makeBook({
        title: "Old Title",
        title_source: DataSourceFilepath, // low-priority → default ON
      }),
      result: makeResult({ title: "New Title" }),
    });

    const titleCheckbox = screen.getByRole("checkbox", {
      name: /apply title/i,
    });
    expect(titleCheckbox).toHaveAttribute("data-state", "checked");
  });

  it("defaults file-level fields ON regardless of source", () => {
    renderForm({
      book: makeBook({
        files: [
          makeFile({
            release_date: "2020-01-01T00:00:00Z",
          }),
        ],
      }),
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
      book: makeBook({
        title: "Old Title",
        title_source: DataSourceFilepath,
        author_source: DataSourceFilepath,
        genre_source: DataSourceFilepath,
      }),
      result: makeResult({
        title: "New Title",
        authors: [{ name: "New Author", role: "" }],
        genres: ["Fantasy"],
      }),
    });

    // The default "Changed" filter hides unchanged rows, so flip to "All"
    // first to make every book-section checkbox visible.
    await user.click(screen.getByRole("button", { name: /^all$/i }));

    const sectionCheckbox = screen.getByRole("checkbox", {
      name: /apply all book fields/i,
    });
    // Title/authors/genres default ON (low-priority source), but other book
    // fields (subtitle, series, tags, description) are unchanged → off.
    // Aggregate is indeterminate. Clicking indeterminate sets all to true.
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

  it("emits file_name with a plugin intent when Name is checked", async () => {
    const user = createUser();
    renderForm({
      result: makeResult({ title: "Plugin Title" }),
    });

    // Name field defaults ON (file-level) and value defaults to plugin's title.
    await user.click(getApplyButton());

    await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1));
    const payload = applyMock.mock.calls[0][0];
    expect(payload.file_name).toBe("Plugin Title");
    expect(payload.sources.file_name).toBe("plugin");
  });

  it("marks the Name intent as user when the Name field is edited", async () => {
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
    expect(payload.sources.file_name).toBe("user");
  });

  it("blocks a selected blank Title with a visible validation error", async () => {
    const user = createUser();
    renderForm({
      book: makeBook({
        title: "Saved Title",
        title_source: DataSourceFilepath,
      }),
      result: makeResult({ title: "Suggested Title" }),
    });

    const titleRow = screen
      .getByText("Title", { selector: "label" })
      .closest("div.grid");
    expect(titleRow).not.toBeNull();
    await user.clear(
      within(titleRow as HTMLElement).getByDisplayValue("Suggested Title"),
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Title cannot be blank",
    );
    expect(getApplyButton()).toBeDisabled();
    expect(applyMock).not.toHaveBeenCalled();
  });

  it("submits selected clears for strings, collections, abridged, and Name", async () => {
    const user = createUser();
    renderForm({
      book: makeBook({
        subtitle: "Saved Subtitle",
        files: [
          makeFile({
            name: "Saved Edition",
            abridged: true,
            identifiers: [
              {
                id: 1,
                file_id: 1,
                type: "isbn_13",
                value: "9780316769488",
                source: DataSourceManual,
                created_at: "2024-01-01T00:00:00Z",
                updated_at: "2024-01-01T00:00:00Z",
              },
            ],
          }),
        ],
      }),
      result: makeResult({ abridged: true }),
    });

    await user.click(screen.getByRole("button", { name: /^all$/i }));
    await user.click(
      screen.getByRole("button", { name: /toggle book section/i }),
    );

    await user.click(screen.getByRole("checkbox", { name: /apply subtitle/i }));
    await user.clear(screen.getByDisplayValue("Saved Subtitle"));

    const nameRow = screen.getByText("Name").closest("div.grid");
    expect(nameRow).not.toBeNull();
    await user.clear(
      within(nameRow as HTMLElement).getByDisplayValue("Some Title"),
    );

    await user.click(
      screen.getByRole("checkbox", { name: /apply identifiers/i }),
    );
    await user.click(screen.getByRole("button", { name: /remove isbn-13/i }));

    await user.click(screen.getByRole("checkbox", { name: /apply abridged/i }));
    await user.click(
      screen.getByRole("checkbox", { name: /mark as abridged/i }),
    );

    await user.click(getApplyButton());

    await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1));
    const payload = applyMock.mock.calls[0][0];
    expect(payload.fields.subtitle).toBe("");
    expect(payload.fields.identifiers).toEqual([]);
    expect(payload.fields.abridged).toBeNull();
    expect(payload.file_name).toBe("");
    expect(payload.sources.file_name).toBe("user");
  });

  // The browser only reports whether the final value equals the Plugin
  // Proposal. Whether the value is a no-op against stored metadata is the
  // server's call (ADR 0006), so an unchanged value still carries an intent.
  describe("source intents for scalars", () => {
    const scalarBook = () =>
      makeBook({
        title: "Saved Title",
        title_source: DataSourceFilepath,
        subtitle: "Saved Subtitle",
        subtitle_source: DataSourceFilepath,
        description: "Saved description",
        description_source: DataSourceFilepath,
        files: [
          makeFile({
            url: "https://example.com/saved",
            language: "en",
            release_date: "2020-01-02T00:00:00Z",
            abridged: false,
          }),
        ],
      });

    // Name also defaults to the proposed title, so scope to the Title row.
    const getTitleInput = () => {
      const row = screen
        .getByText("Title", { selector: "label" })
        .closest("div.grid");
      expect(row).not.toBeNull();
      return within(row as HTMLElement).getByDisplayValue("Proposed Title");
    };

    const submit = async (user: ReturnType<typeof createUser>) => {
      await user.click(getApplyButton());
      await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1));
      return applyMock.mock.calls[0][0];
    };

    it("sends plugin for accepted proposals", async () => {
      const user = createUser();
      renderForm({
        book: scalarBook(),
        result: makeResult({
          title: "Proposed Title",
          subtitle: "Proposed Subtitle",
          description: "Proposed description",
          publisher: "Proposed Publisher",
          url: "https://example.com/proposed",
          language: "fr",
          release_date: "2021-03-04T00:00:00Z",
          abridged: true,
        }),
      });

      const payload = await submit(user);
      expect(payload.fields).toMatchObject({
        title: "Proposed Title",
        release_date: "2021-03-04",
        abridged: true,
      });
      expect(payload.sources).toMatchObject({
        title: "plugin",
        subtitle: "plugin",
        description: "plugin",
        publisher: "plugin",
        url: "plugin",
        language: "plugin",
        release_date: "plugin",
        abridged: "plugin",
      });
    });

    it("sends user when the final value has no matching proposal", async () => {
      const user = createUser();
      renderForm({
        book: scalarBook(),
        // The plugin proposes nothing for URL, so the file-level row submits
        // the saved value unchanged. The server resolves that as a no-op.
        result: makeResult({ title: "Saved Title" }),
      });

      // Unchanged rows are hidden and default OFF, so opt in explicitly.
      await user.click(screen.getByRole("button", { name: /^all$/i }));
      await user.click(screen.getByRole("checkbox", { name: /apply url/i }));
      const payload = await submit(user);
      expect(payload.fields.url).toBe("https://example.com/saved");
      expect(payload.sources.url).toBe("user");
    });

    it("sends user for an edited value", async () => {
      const user = createUser();
      renderForm({
        book: scalarBook(),
        result: makeResult({ title: "Proposed Title" }),
      });

      const titleInput = getTitleInput();
      await user.clear(titleInput);
      await user.type(titleInput, "My Title");
      await user.click(getApplyButton());
      await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1));
      expect(applyMock.mock.calls[0][0].fields.title).toBe("My Title");
      expect(applyMock.mock.calls[0][0].sources.title).toBe("user");
    });

    it("sends plugin when an edit is restored to the proposal", async () => {
      const user = createUser();
      renderForm({
        book: scalarBook(),
        result: makeResult({ title: "Proposed Title" }),
      });

      const titleInput = getTitleInput();
      await user.clear(titleInput);
      await user.type(titleInput, "My Title");
      await user.clear(titleInput);
      // Surrounding whitespace is not an edit: equality is trimmed.
      await user.type(titleInput, "Proposed Title ");

      const payload = await submit(user);
      expect(payload.sources.title).toBe("plugin");
    });

    it("omits both the value and the intent for unchecked fields", async () => {
      const user = createUser();
      renderForm({
        book: scalarBook(),
        result: makeResult({
          title: "Proposed Title",
          subtitle: "Proposed Subtitle",
        }),
      });

      await user.click(
        screen.getByRole("checkbox", { name: /apply subtitle/i }),
      );
      const payload = await submit(user);
      expect(payload.fields).not.toHaveProperty("subtitle");
      expect(payload.sources).not.toHaveProperty("subtitle");
      expect(payload.sources.title).toBe("plugin");
    });

    it("sends user for an Explicit Clear", async () => {
      const user = createUser();
      renderForm({
        book: scalarBook(),
        result: makeResult({
          title: "Saved Title",
          subtitle: "Proposed Subtitle",
        }),
      });

      await user.clear(screen.getByDisplayValue("Proposed Subtitle"));
      const payload = await submit(user);
      expect(payload.fields.subtitle).toBe("");
      expect(payload.sources.subtitle).toBe("user");
    });
  });

  describe("source intents for relationships", () => {
    const proposal = () =>
      makeResult({
        authors: [
          { name: "Proposed Author", role: "" },
          { name: "Second Author", role: "writer" },
        ],
        narrators: ["Proposed Narrator", "Second Narrator"],
        genres: ["Fantasy", "Adventure"],
        tags: ["Favorite", "Unread"],
      });

    const relationshipBook = () =>
      makeBook({
        author_source: DataSourceFilepath,
        genre_source: DataSourceFilepath,
        tag_source: DataSourceFilepath,
        files: [makeFile({ file_type: FileTypeM4B })],
      });

    const submit = async (user: ReturnType<typeof createUser>) => {
      await user.click(getApplyButton());
      await waitFor(() => expect(applyMock).toHaveBeenCalledTimes(1));
      return applyMock.mock.calls[0][0];
    };

    const savedRelationshipBook = () => {
      const resource = (name: string, id: number) =>
        ({
          id,
          name,
          library_id: 1,
          created_at: "2024-01-01T00:00:00Z",
          updated_at: "2024-01-01T00:00:00Z",
          sort_name: name,
          sort_name_source: DataSourceManual,
          book_count: 1,
        }) as const;
      return makeBook({
        ...relationshipBook(),
        authors: [
          {
            id: 1,
            book_id: 1,
            person_id: 1,
            sort_order: 0,
            person: resource("Proposed Author", 1),
          },
          {
            id: 2,
            book_id: 1,
            person_id: 2,
            sort_order: 1,
            role: "writer",
            person: resource("Second Author", 2),
          },
        ],
        book_genres: [
          { id: 1, book_id: 1, genre_id: 1, genre: resource("Fantasy", 1) },
          { id: 2, book_id: 1, genre_id: 2, genre: resource("Adventure", 2) },
        ],
        book_tags: [
          { id: 1, book_id: 1, tag_id: 1, tag: resource("Favorite", 1) },
          { id: 2, book_id: 1, tag_id: 2, tag: resource("Unread", 2) },
        ],
        files: [
          makeFile({
            file_type: FileTypeM4B,
            narrators: [
              {
                id: 1,
                file_id: 1,
                person_id: 3,
                sort_order: 0,
                person: resource("Proposed Narrator", 3),
              },
              {
                id: 2,
                file_id: 1,
                person_id: 4,
                sort_order: 1,
                person: resource("Second Narrator", 4),
              },
            ],
          }),
        ],
      });
    };

    it("shows and selects reordered author and narrator proposals as changes", async () => {
      const user = createUser();
      renderForm({
        book: savedRelationshipBook(),
        result: makeResult({
          ...proposal(),
          authors: [
            { name: "Second Author", role: "writer" },
            { name: "Proposed Author", role: "" },
          ],
          narrators: ["Second Narrator", "Proposed Narrator"],
        }),
      });
      expect(
        screen.getByRole("checkbox", { name: "Apply Authors" }),
      ).toBeChecked();
      expect(
        screen.getByRole("checkbox", { name: "Apply Narrators" }),
      ).toBeChecked();
      const payload = await submit(user);
      expect(payload.fields.authors).toEqual([
        { name: "Second Author", role: "writer" },
        { name: "Proposed Author", role: "" },
      ]);
      expect(payload.fields.narrators).toEqual([
        "Second Narrator",
        "Proposed Narrator",
      ]);
      expect(payload.sources).toMatchObject({
        authors: "plugin",
        narrators: "plugin",
      });
    });

    const labels = ["Authors", "Narrators", "Genres", "Tags"];

    const addEntry = async (
      user: ReturnType<typeof createUser>,
      label: string,
      name: string,
    ) => {
      await user.click(screen.getByText(new RegExp(`^Add ${label}`, "i")));
      await user.type(screen.getByPlaceholderText(`Search ${label}...`), name);
      await user.click(
        screen.getByRole("option", {
          name: `Create new ${label} "${name.trim()}"`,
        }),
      );
      if (label === "genre" || label === "tag") await user.keyboard("{Escape}");
    };

    it.each([
      { matchingProposal: true, intent: "plugin" },
      { matchingProposal: false, intent: "user" },
    ])(
      "sends $intent for selected unchanged relationships with matchingProposal=$matchingProposal",
      async ({ matchingProposal, intent }) => {
        const user = createUser();
        renderForm({
          book: savedRelationshipBook(),
          result: matchingProposal ? proposal() : makeResult(),
        });
        for (const label of labels) {
          expect(
            screen.queryByRole("checkbox", { name: `Apply ${label}` }),
          ).not.toBeInTheDocument();
        }
        await user.click(screen.getByRole("button", { name: "All" }));
        await user.click(
          screen.getByRole("button", { name: "Toggle BOOK section" }),
        );
        for (const label of labels) {
          const checkbox = screen.getByRole("checkbox", {
            name: `Apply ${label}`,
          });
          expect(checkbox).not.toBeChecked();
          await user.click(checkbox);
        }
        const payload = await submit(user);
        expect(payload.fields).toMatchObject({
          authors: [
            { name: "Proposed Author", role: undefined },
            { name: "Second Author", role: "writer" },
          ],
          narrators: ["Proposed Narrator", "Second Narrator"],
          genres: ["Fantasy", "Adventure"],
          tags: ["Favorite", "Unread"],
        });
        expect(payload.sources).toMatchObject({
          authors: intent,
          narrators: intent,
          genres: intent,
          tags: intent,
        });
      },
    );

    it("sends user for partial removals from each relationship", async () => {
      const user = createUser();
      renderForm({ book: relationshipBook(), result: proposal() });
      for (const name of [
        "Second Author",
        "Second Narrator",
        "Adventure",
        "Unread",
      ]) {
        await user.click(
          screen.getByRole("button", { name: `Remove ${name}` }),
        );
      }
      const payload = await submit(user);
      expect(payload.fields).toMatchObject({
        authors: [{ name: "Proposed Author", role: "" }],
        narrators: ["Proposed Narrator"],
        genres: ["Fantasy"],
        tags: ["Favorite"],
      });
      expect(payload.sources).toMatchObject({
        authors: "user",
        narrators: "user",
        genres: "user",
        tags: "user",
      });
    });

    it("sends user for Explicit Clears of each relationship", async () => {
      const user = createUser();
      renderForm({ book: relationshipBook(), result: proposal() });
      await user.click(screen.getByRole("button", { name: "All" }));
      for (const name of [
        "Proposed Author",
        "Second Author",
        "Proposed Narrator",
        "Second Narrator",
        "Fantasy",
        "Adventure",
        "Favorite",
        "Unread",
      ]) {
        await user.click(
          screen.getByRole("button", { name: `Remove ${name}` }),
        );
      }
      const payload = await submit(user);
      expect(payload.fields).toMatchObject({
        authors: [],
        narrators: [],
        genres: [],
        tags: [],
      });
      expect(payload.sources).toMatchObject({
        authors: "user",
        narrators: "user",
        genres: "user",
        tags: "user",
      });
    });

    it("omits unchecked relationship values and intents", async () => {
      const user = createUser();
      renderForm({ book: relationshipBook(), result: proposal() });
      for (const label of labels)
        await user.click(
          screen.getByRole("checkbox", { name: `Apply ${label}` }),
        );
      const payload = await submit(user);
      for (const field of ["authors", "narrators", "genres", "tags"]) {
        expect(payload.fields).not.toHaveProperty(field);
        expect(payload.sources).not.toHaveProperty(field);
      }
    });

    it("sends plugin after restoring relationship suggestions", async () => {
      const user = createUser();
      renderForm({ book: relationshipBook(), result: proposal() });
      for (const name of [
        "Second Author",
        "Second Narrator",
        "Adventure",
        "Unread",
      ]) {
        await user.click(
          screen.getByRole("button", { name: `Remove ${name}` }),
        );
      }
      await user.click(
        screen.getByRole("button", { name: "Restore suggestions" }),
      );
      const payload = await submit(user);
      expect(payload.sources).toMatchObject({
        authors: "plugin",
        narrators: "plugin",
        genres: "plugin",
        tags: "plugin",
      });
    });

    it("sends user for author and narrator order edits and keeps their changed status visible", async () => {
      const user = createUser();
      renderForm({ book: savedRelationshipBook(), result: proposal() });
      await user.click(screen.getByRole("button", { name: "All" }));
      await user.click(
        screen.getByRole("button", { name: "Toggle BOOK section" }),
      );
      for (const label of ["Authors", "Narrators"])
        await user.click(
          screen.getByRole("checkbox", { name: `Apply ${label}` }),
        );
      await user.click(
        screen.getByRole("button", { name: "Remove Proposed Author" }),
      );
      await addEntry(user, "author", "Proposed Author");
      await user.click(
        screen.getByRole("button", { name: "Remove Proposed Narrator" }),
      );
      await addEntry(user, "narrator", "Proposed Narrator");
      await user.click(screen.getByRole("button", { name: "Changed" }));
      expect(
        screen.getByRole("checkbox", { name: "Apply Authors" }),
      ).toBeChecked();
      expect(
        screen.getByRole("checkbox", { name: "Apply Narrators" }),
      ).toBeChecked();
      const payload = await submit(user);
      expect(payload.fields.authors).toEqual([
        { name: "Second Author", role: "writer" },
        { name: "Proposed Author", role: undefined },
      ]);
      expect(payload.fields.narrators).toEqual([
        "Second Narrator",
        "Proposed Narrator",
      ]);
      expect(payload.sources).toMatchObject({
        authors: "user",
        narrators: "user",
      });
    });

    it("ignores genre and tag proposal order for defaults and intent", async () => {
      const user = createUser();
      renderForm({
        book: savedRelationshipBook(),
        result: makeResult({
          ...proposal(),
          genres: ["Adventure", "Fantasy"],
          tags: ["Unread", "Favorite"],
        }),
      });
      expect(
        screen.queryByRole("checkbox", { name: "Apply Genres" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("checkbox", { name: "Apply Tags" }),
      ).not.toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: "All" }));
      await user.click(
        screen.getByRole("button", { name: "Toggle BOOK section" }),
      );
      for (const label of ["Genres", "Tags"])
        await user.click(
          screen.getByRole("checkbox", { name: `Apply ${label}` }),
        );
      const payload = await submit(user);
      expect(payload.fields.genres).toEqual(["Fantasy", "Adventure"]);
      expect(payload.fields.tags).toEqual(["Favorite", "Unread"]);
      expect(payload.sources).toMatchObject({
        genres: "plugin",
        tags: "plugin",
      });
    });

    it.each([
      { restore: false, intent: "user", status: "Changed", role: "writer" },
      { restore: true, intent: "plugin", status: "Unchanged", role: undefined },
    ])(
      "sends $intent for an author role edit with restore=$restore",
      async ({ restore, intent, status, role }) => {
        const user = createUser();
        const book = savedRelationshipBook();
        renderForm({
          book: makeBook({
            ...book,
            authors: book.authors?.slice(0, 1),
            files: [makeFile({ file_type: FileTypeCBZ })],
          }),
          result: makeResult({
            authors: [{ name: "Proposed Author", role: "" }],
          }),
        });
        await user.click(screen.getByRole("button", { name: "All" }));
        await user.click(
          screen.getByRole("button", { name: "Toggle BOOK section" }),
        );
        await user.click(
          screen.getByRole("checkbox", { name: "Apply Authors" }),
        );
        await user.click(screen.getByText("No role").closest("button")!);
        await user.click(screen.getByRole("option", { name: "Writer" }));
        if (restore) {
          await user.click(screen.getByText("Writer").closest("button")!);
          await user.click(screen.getByRole("option", { name: "No role" }));
        }
        const row = screen
          .getByText("Authors", { selector: "label" })
          .closest("div.grid");
        expect(
          within(row as HTMLElement).getByText(status),
        ).toBeInTheDocument();
        const payload = await submit(user);
        expect(payload.fields.authors).toEqual([
          { name: "Proposed Author", role },
        ]);
        expect(payload.sources.authors).toBe(intent);
      },
    );

    it("sends plugin when removed entries are re-added with trimmed proposal names", async () => {
      const user = createUser();
      renderForm({
        book: relationshipBook(),
        result: makeResult({
          authors: [{ name: " Proposed Author ", role: "" }],
          narrators: [" Proposed Narrator "],
          genres: [" Fantasy ", "Adventure"],
          tags: [" Favorite ", "Unread"],
        }),
      });
      await user.click(screen.getByRole("button", { name: "All" }));
      for (const [label, name] of [
        ["author", "Proposed Author"],
        ["narrator", "Proposed Narrator"],
        ["genre", "Fantasy"],
        ["tag", "Favorite"],
      ]) {
        await user.click(
          screen.getByRole("button", { name: `Remove ${name}` }),
        );
        await addEntry(user, label, name);
      }
      const payload = await submit(user);
      expect(payload.fields).toMatchObject({
        authors: [{ name: "Proposed Author", role: undefined }],
        narrators: ["Proposed Narrator"],
        genres: ["Adventure", "Fantasy"],
        tags: ["Unread", "Favorite"],
      });
      expect(payload.sources).toMatchObject({
        authors: "plugin",
        narrators: "plugin",
        genres: "plugin",
        tags: "plugin",
      });
    });

    it("compares genre and tag entries as trimmed raw sets", async () => {
      const user = createUser();
      renderForm({
        book: relationshipBook(),
        result: makeResult({
          ...proposal(),
          genres: [" Fantasy ", "Adventure", "Fantasy"],
          tags: [" Favorite ", "Unread", "Favorite"],
        }),
      });
      // Removing duplicate trimmed names leaves the same set, not an edit.
      await user.click(
        screen.getAllByRole("button", {
          name: "Remove Fantasy",
        })[1],
      );
      await user.click(
        screen.getAllByRole("button", {
          name: "Remove Favorite",
        })[1],
      );
      const payload = await submit(user);
      expect(payload.fields.genres).toEqual([" Fantasy ", "Adventure"]);
      expect(payload.fields.tags).toEqual([" Favorite ", "Unread"]);
      expect(payload.sources).toMatchObject({
        genres: "plugin",
        tags: "plugin",
      });
    });

    it("sends plugin for accepted relationship proposals", async () => {
      const user = createUser();
      renderForm({ book: relationshipBook(), result: proposal() });

      const payload = await submit(user);
      expect(payload.fields).toMatchObject({
        authors: [
          { name: "Proposed Author", role: "" },
          { name: "Second Author", role: "writer" },
        ],
        narrators: ["Proposed Narrator", "Second Narrator"],
        genres: ["Fantasy", "Adventure"],
        tags: ["Favorite", "Unread"],
      });
      expect(payload.sources).toMatchObject({
        authors: "plugin",
        narrators: "plugin",
        genres: "plugin",
        tags: "plugin",
      });
    });
  });

  it("hides unchanged rows in the default Changed filter, shows them in All", async () => {
    const user = createUser();
    renderForm({
      book: makeBook({
        title: "Old Title",
        title_source: DataSourceFilepath, // low-priority → defaults ON
        subtitle: "",
      }),
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
      book: makeBook({
        title: "Old Title",
        title_source: DataSourceFilepath, // low-priority → defaults ON
      }),
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
      book: makeBook({
        title: "Old Title",
        title_source: DataSourceFilepath, // low-priority → defaults ON
      }),
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
