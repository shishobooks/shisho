import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import type { Book, File, SharingSettingsResponse } from "@/types";

import BookDetailBody, { type ShareLinkContext } from "./BookDetailBody";

beforeAll(() => {
  vi.stubGlobal("__APP_VERSION__", "test");
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.stubGlobal("__APP_VERSION__", "test");
});

// Full permissions by default, so every control the Share Link context hides
// would otherwise be visible. Tests narrow the set to exercise one role.
const ALL_PERMISSIONS = [
  "books:read",
  "books:write",
  "config:read",
  "people:read",
  "series:read",
  "shares:read",
  "shares:write",
];
const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    demoMode: false,
    canWrite: (resource: string) => auth.permissions.has(`${resource}:write`),
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
  }),
}));

const sharing = vi.hoisted(() => ({
  settings: { enabled: false, require_expiration: false } as
    SharingSettingsResponse | undefined,
  options: [] as Array<{ enabled?: boolean } | undefined>,
}));

vi.mock("@/hooks/queries/sharing", () => ({
  useSharingSettings: (options?: { enabled?: boolean }) => {
    sharing.options.push(options);
    return { data: options?.enabled === false ? undefined : sharing.settings };
  },
}));

vi.mock("@/components/library/ShareLinkDialog", () => ({
  ShareLinkDialog: (props: {
    open: boolean;
    canWrite: boolean;
    canList: boolean;
    requireExpiration: boolean;
    sharingEnabled: boolean;
    canManageSharing: boolean;
  }) =>
    props.open ? (
      <div data-testid="share-dialog">
        {JSON.stringify({
          canWrite: props.canWrite,
          canList: props.canList,
          requireExpiration: props.requireExpiration,
          sharingEnabled: props.sharingEnabled,
          canManageSharing: props.canManageSharing,
        })}
      </div>
    ) : null,
}));

beforeEach(() => {
  auth.permissions = new Set(ALL_PERMISSIONS);
  sharing.settings = { enabled: false, require_expiration: false };
  sharing.options.length = 0;
});

const { idle } = vi.hoisted(() => ({
  idle: () => ({ mutateAsync: vi.fn(), mutate: vi.fn(), isPending: false }),
}));

vi.mock("@/hooks/queries/books", () => ({
  useDeleteBook: idle,
  useDeleteFile: idle,
  useResyncBook: idle,
  useResyncFile: idle,
}));
const { identifierTypesOptions } = vi.hoisted(() => ({
  identifierTypesOptions: [] as Array<{ enabled?: boolean } | undefined>,
}));

vi.mock("@/hooks/queries/plugins", () => ({
  usePluginIdentifierTypes: (options?: { enabled?: boolean }) => {
    identifierTypesOptions.push(options);
    return { data: [] };
  },
}));
vi.mock("@/hooks/queries/review", () => ({
  useSetBookReview: idle,
  useReviewCriteria: () => ({
    data: { book_fields: [], audio_fields: [] },
  }),
}));
vi.mock("@/components/library/AddToListPopover", () => ({
  default: ({ trigger }: { trigger: ReactNode }) => (
    <div data-testid="add-to-list">{trigger}</div>
  ),
}));
vi.mock("@/components/library/BookEditDialog", () => ({
  BookEditDialog: () => null,
}));
vi.mock("@/components/library/IdentifyBookDialog", () => ({
  IdentifyBookDialog: () => null,
}));
vi.mock("@/components/library/MergeIntoDialog", () => ({
  MergeIntoDialog: () => null,
}));
vi.mock("@/components/library/MoveFilesDialog", () => ({
  MoveFilesDialog: () => null,
}));
vi.mock("@/components/library/FileEditDialog", () => ({
  FileEditDialog: () => null,
}));

const timestamps = {
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

const epub = {
  ...timestamps,
  id: 42,
  book_id: 7,
  library_id: 1,
  file_type: "epub",
  file_role: "main",
  filepath: "",
  filesize_bytes: 1000,
  reviewed: true,
  is_preferred_cover: false,
  cover_image_filename: "",
  publisher: { id: 5, name: "Tor Books" },
  release_date: "2020-01-01T00:00:00Z",
} as unknown as File;

const m4b = {
  ...timestamps,
  id: 43,
  book_id: 7,
  library_id: 1,
  file_type: "m4b",
  file_role: "main",
  filepath: "",
  filesize_bytes: 2000,
  reviewed: true,
  is_preferred_cover: false,
  narrators: [
    { id: 1, person_id: 11, person: { id: 11, name: "Nora Reader" } },
  ],
} as unknown as File;

const book = {
  ...timestamps,
  id: 7,
  library_id: 1,
  title: "Test Book",
  filepath: "",
  cover_cache_key: "42-1",
  authors: [{ id: 1, person_id: 10, person: { id: 10, name: "Ada Author" } }],
  book_series: [
    {
      id: 1,
      series_id: 3,
      series_number: 2,
      series: { id: 3, name: "Great Series" },
    },
  ],
  book_genres: [{ id: 1, genre_id: 4, genre: { id: 4, name: "Fantasy" } }],
  book_tags: [{ id: 1, tag_id: 6, tag: { id: 6, name: "Favorite" } }],
  files: [epub],
} as unknown as Book;

const shareLink: ShareLinkContext = {
  downloadUrl: (file) => `/api/share/tok/files/${file.id}/download`,
  bookCoverUrl: (b) => `/api/share/tok/cover?v=${b.cover_cache_key}`,
  fileCoverUrl: (file) => `/api/share/tok/files/${file.id}/cover`,
};

const renderBody = (
  props: Partial<React.ComponentProps<typeof BookDetailBody>> = {},
) =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <BookDetailBody book={book} {...props} />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("BookDetailBody in Share Link context", () => {
  it("renders resource names as plain text with no links into the app", () => {
    renderBody({
      book: { ...book, files: [{ ...epub, publisher: undefined }, m4b] },
      shareLink,
    });

    for (const name of [
      "Ada Author",
      "Great Series",
      "Fantasy",
      "Favorite",
      "Nora Reader",
    ]) {
      const el = screen.getByText(name);
      expect(el.closest("a")).toBeNull();
    }
    expect(screen.queryAllByRole("link")).toHaveLength(0);
  });

  it("renders the file publisher as plain text", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderBody({ shareLink });

    await user.click(screen.getByRole("button", { name: "Show file details" }));
    expect(screen.getByText("Tor Books").closest("a")).toBeNull();
  });

  it("hides every action control even with full permissions", () => {
    renderBody({ book: { ...book, files: [epub, m4b] }, shareLink });

    expect(screen.queryByLabelText("Book actions")).not.toBeInTheDocument();
    expect(screen.queryByTestId("add-to-list")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("File actions")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Select" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
    expect(screen.queryAllByRole("link")).toHaveLength(0);
    expect(screen.queryByText("Library")).not.toBeInTheDocument();
    expect(screen.queryByText("File Path")).not.toBeInTheDocument();
  });

  it("makes no authenticated identifier types request", () => {
    identifierTypesOptions.length = 0;
    renderBody({ shareLink });

    expect(identifierTypesOptions.length).toBeGreaterThan(0);
    for (const options of identifierTypesOptions) {
      expect(options?.enabled).toBe(false);
    }
  });

  it("makes no sharing settings request and offers no Share entry", () => {
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody({ shareLink });

    expect(sharing.options.length).toBeGreaterThan(0);
    for (const options of sharing.options) {
      expect(options?.enabled).toBe(false);
    }
    expect(screen.queryByLabelText("Book actions")).not.toBeInTheDocument();
  });

  it("omits the sort title and the created and updated times", () => {
    renderBody({
      book: { ...book, sort_title: "Book, Test" },
      shareLink,
    });

    expect(screen.queryByText(/Sort title/)).not.toBeInTheDocument();
    expect(screen.queryByText("Created")).not.toBeInTheDocument();
    expect(screen.queryByText("Updated")).not.toBeInTheDocument();
  });

  it("omits file identifiers and the file URL but keeps reader-facing details", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderBody({
      book: {
        ...book,
        files: [
          {
            ...epub,
            language: "en",
            url: "https://example.com/book",
            identifiers: [{ id: 1, type: "uuid", value: "urn:uuid:1234" }],
          } as File,
        ],
      },
      shareLink,
    });

    await user.click(screen.getByRole("button", { name: "Show file details" }));
    expect(screen.getByText("Tor Books")).toBeInTheDocument();
    expect(screen.getByText("Released")).toBeInTheDocument();
    expect(screen.getByText("Language")).toBeInTheDocument();
    expect(screen.queryByText("urn:uuid:1234")).not.toBeInTheDocument();
    expect(screen.queryByText("URL")).not.toBeInTheDocument();
  });

  it("offers no expander when a file's only details are hidden ones", () => {
    renderBody({
      book: {
        ...book,
        files: [
          {
            ...epub,
            publisher: undefined,
            release_date: undefined,
            url: "https://example.com/book",
            identifiers: [{ id: 1, type: "uuid", value: "urn:uuid:1234" }],
          } as File,
        ],
      },
      shareLink,
    });

    expect(
      screen.queryByRole("button", { name: "Show file details" }),
    ).not.toBeInTheDocument();
  });

  it("falls back to the file type when the payload blanks the path", () => {
    renderBody({ shareLink });

    expect(screen.getByTitle("EPUB")).toHaveTextContent("EPUB");
  });

  it("uses the supplied cover URL builders", () => {
    const { container } = renderBody({ shareLink });

    expect(screen.getByAltText("Test Book Cover")).toHaveAttribute(
      "src",
      "/api/share/tok/cover?v=42-1",
    );
    const thumbnails = Array.from(container.querySelectorAll("img")).map(
      (img) => img.getAttribute("src"),
    );
    expect(thumbnails).toContain("/api/share/tok/files/42/cover");
  });

  it("downloads through the supplied download URL builder", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const fetchMock = vi.fn().mockResolvedValue(new Response(null));
    vi.stubGlobal("fetch", fetchMock);
    const assign = vi.fn();
    vi.stubGlobal("location", { ...window.location, assign });

    renderBody({ shareLink });

    const downloads = screen.getAllByRole("button", { name: "Download" });
    await user.click(downloads[0]);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/share/tok/files/42/download",
      expect.objectContaining({ method: "HEAD" }),
    );
    expect(assign).toHaveBeenCalledWith("/api/share/tok/files/42/download");
  });

  it("downloads supplements through the supplied download URL builder", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const fetchMock = vi.fn().mockResolvedValue(new Response(null));
    vi.stubGlobal("fetch", fetchMock);
    const assign = vi.fn();
    vi.stubGlobal("location", { ...window.location, assign });

    const supplement = {
      ...epub,
      id: 44,
      file_type: "pdf",
      file_role: "supplement",
      name: "Map",
    } as File;
    renderBody({ book: { ...book, files: [supplement] }, shareLink });

    await user.click(screen.getAllByRole("button", { name: "Download" })[0]);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/share/tok/files/44/download",
      expect.objectContaining({ method: "HEAD" }),
    );
    expect(assign).toHaveBeenCalledWith("/api/share/tok/files/44/download");
    expect(assign).not.toHaveBeenCalledWith(
      expect.stringContaining("/download/original"),
    );
  });
});

describe("BookDetailBody without Share Link context", () => {
  it("links resource names into the library", () => {
    renderBody();

    expect(screen.getByText("Ada Author").closest("a")).toHaveAttribute(
      "href",
      "/libraries/1/people/10",
    );
    expect(screen.getByText("Great Series").closest("a")).toHaveAttribute(
      "href",
      "/libraries/1/series/3",
    );
    expect(screen.getByText("Fantasy").closest("a")).toHaveAttribute(
      "href",
      "/libraries/1?genre_ids=4",
    );
    expect(screen.getByText("Favorite").closest("a")).toHaveAttribute(
      "href",
      "/libraries/1?tag_ids=6",
    );
  });

  it("renders author and series names as text without People and Series Read", () => {
    auth.permissions = new Set(["books:read"]);
    renderBody();

    expect(screen.getByText("Ada Author").closest("a")).toBeNull();
    expect(screen.getByText("Great Series").closest("a")).toBeNull();
    expect(screen.getByText("Fantasy").closest("a")).toHaveAttribute(
      "href",
      "/libraries/1?genre_ids=4",
    );
  });

  it("keeps the same book menu entries for Books Write", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderBody();

    await user.click(screen.getByLabelText("Book actions"));
    const menu = screen.getByRole("menu");
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((item) => item.textContent),
    ).toEqual([
      "Edit",
      "Add to list",
      "Share",
      "Rescan book",
      "Identify book",
      "Merge into another book",
      "Delete book",
    ]);
    expect(within(menu).getAllByRole("separator")).toHaveLength(4);
  });
});

describe("BookDetailBody Share entry", () => {
  const menuItems = async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(screen.getByLabelText("Book actions"));
    return within(screen.getByRole("menu"))
      .getAllByRole("menuitem")
      .map((item) => item.textContent);
  };

  it("adds Share after Add to list when sharing is enabled", async () => {
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    expect(await menuItems()).toEqual([
      "Edit",
      "Add to list",
      "Share",
      "Rescan book",
      "Identify book",
      "Merge into another book",
      "Delete book",
    ]);
  });

  it("shows the menu with Share and Add to list for Shares Write without Books Write", async () => {
    auth.permissions = new Set(["books:read", "shares:read", "shares:write"]);
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    expect(await menuItems()).toEqual(["Add to list", "Share"]);
  });

  it("offers Share while sharing is disabled so links can be revoked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    auth.permissions = new Set(["books:read", "shares:read", "shares:write"]);
    renderBody();

    expect(await menuItems()).toEqual(["Add to list", "Share"]);
    await user.click(screen.getByRole("menuitem", { name: "Share" }));

    expect(
      JSON.parse(screen.getByTestId("share-dialog").textContent ?? ""),
    ).toEqual({
      canWrite: true,
      canList: true,
      requireExpiration: false,
      sharingEnabled: false,
      canManageSharing: false,
    });
  });

  it("hides Share until the sharing settings have loaded", () => {
    auth.permissions = new Set(["books:read", "shares:read", "shares:write"]);
    sharing.settings = undefined;
    renderBody();

    expect(screen.queryByLabelText("Book actions")).not.toBeInTheDocument();
    expect(screen.getByTestId("add-to-list")).toHaveTextContent("Add to list");
  });

  it("hides Share and skips the settings request without the shares permission", () => {
    auth.permissions = new Set(["books:read", "books:write"]);
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    for (const options of sharing.options) {
      expect(options?.enabled).toBe(false);
    }
    expect(screen.queryByText("Share")).not.toBeInTheDocument();
  });

  it("opens the dialog with the form, the policy, and the settings link for Config Write", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    auth.permissions = new Set([...ALL_PERMISSIONS, "config:write"]);
    sharing.settings = { enabled: true, require_expiration: true };
    renderBody();

    await user.click(screen.getByLabelText("Book actions"));
    await user.click(screen.getByRole("menuitem", { name: "Share" }));

    expect(
      JSON.parse(screen.getByTestId("share-dialog").textContent ?? ""),
    ).toEqual({
      canWrite: true,
      canList: true,
      requireExpiration: true,
      sharingEnabled: true,
      canManageSharing: true,
    });
  });

  it("offers Share with the form and the list for Shares Write without Shares Read", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    auth.permissions = new Set(["books:read", "shares:write"]);
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    for (const options of sharing.options) {
      expect(options?.enabled).toBe(true);
    }
    await user.click(screen.getByLabelText("Book actions"));
    await user.click(screen.getByRole("menuitem", { name: "Share" }));

    expect(
      JSON.parse(screen.getByTestId("share-dialog").textContent ?? ""),
    ).toEqual({
      canWrite: true,
      canList: true,
      requireExpiration: false,
      sharingEnabled: true,
      canManageSharing: false,
    });
  });

  it("opens the dialog without the form for Shares Read only", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    auth.permissions = new Set(["books:read", "shares:read"]);
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    await user.click(screen.getByLabelText("Book actions"));
    await user.click(screen.getByRole("menuitem", { name: "Share" }));

    expect(
      JSON.parse(screen.getByTestId("share-dialog").textContent ?? ""),
    ).toEqual({
      canWrite: false,
      canList: true,
      requireExpiration: false,
      sharingEnabled: true,
      canManageSharing: false,
    });
  });
});

describe("BookDetailBody file rows", () => {
  it("names the details toggle and reports its state", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderBody();

    const toggle = screen.getByRole("button", { name: "Show file details" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");

    await user.click(toggle);
    expect(
      screen.getByRole("button", { name: "Hide file details" }),
    ).toHaveAttribute("aria-expanded", "true");
  });

  it("says Yes for an abridged file instead of repeating the label", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderBody({ book: { ...book, files: [{ ...epub, abridged: true }] } });

    await user.click(screen.getByRole("button", { name: "Show file details" }));
    const label = screen.getByText("Abridged");
    expect(label.nextElementSibling).toHaveTextContent("Yes");
  });

  it("says No for an unabridged audiobook", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderBody({ book: { ...book, files: [{ ...m4b, abridged: false }] } });

    await user.click(screen.getByRole("button", { name: "Show file details" }));
    const label = screen.getByText("Abridged");
    expect(label.nextElementSibling).toHaveTextContent("No");
  });

  it("uses the singular for a one-page file", () => {
    renderBody({
      book: { ...book, files: [{ ...epub, file_type: "cbz", page_count: 1 }] },
    });

    expect(screen.getAllByText("1 page").length).toBeGreaterThan(0);
    expect(screen.queryByText("1 pages")).not.toBeInTheDocument();
  });
});

describe("BookDetailBody supplement names", () => {
  const supplement = {
    ...timestamps,
    id: 44,
    book_id: 7,
    library_id: 1,
    file_type: "pdf",
    file_role: "supplement",
    filepath: "/lib/The Lighthouse Keeper/The Lighthouse Keeper.pdf",
    filesize_bytes: 500,
    name: "Original Title",
    display_name: "The Lighthouse Keeper.pdf",
  } as unknown as File;

  it("labels a supplement with the server's display name, not its stale stored name", () => {
    renderBody({ book: { ...book, files: [epub, supplement] } });

    expect(screen.getByText("The Lighthouse Keeper.pdf")).toBeInTheDocument();
    expect(screen.queryByText("Original Title")).not.toBeInTheDocument();
  });

  it("uses the display name in the Share Link view, where the path is blank", () => {
    renderBody({
      book: { ...book, files: [epub, { ...supplement, filepath: "" }] },
      shareLink,
    });

    expect(screen.getByText("The Lighthouse Keeper.pdf")).toBeInTheDocument();
    expect(screen.queryByText("Original Title")).not.toBeInTheDocument();
  });
});
