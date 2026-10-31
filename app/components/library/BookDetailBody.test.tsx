import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import {
  createMemoryRouter,
  MemoryRouter,
  Outlet,
  Route,
  RouterProvider,
  Routes,
} from "react-router-dom";
import { toast } from "sonner";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { NavigationHistoryProvider } from "@/contexts/NavigationHistory";
import { setAuth } from "@/testing/auth";
import { mockCoverDimensions } from "@/testing/coverDimensions";
import { emulatePhoneWidth } from "@/testing/phoneWidth";
import type {
  Book,
  File,
  RemovedEntities,
  Rule,
  SharingSettingsResponse,
} from "@/types";
import type { Permission } from "@/utils/permissions";

import BookDetailBody, { type ShareLinkContext } from "./BookDetailBody";

mockCoverDimensions();

beforeAll(() => {
  vi.stubGlobal("__APP_VERSION__", "test");
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.stubGlobal("__APP_VERSION__", "test");
});

// Full permissions by default, so every control the Share Link context hides
// would otherwise be visible. Tests narrow the set to exercise one role.
const ALL_PERMISSIONS: Permission[] = [
  "books:read",
  "books:write",
  "config:read",
  "people:read",
  "series:read",
  "shares:read",
  "shares:write",
];

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

// BookDetailBody.permissions.test.tsx checks which requests the real sharing
// and plugin hooks send; these stand-ins only return data.
const sharing = vi.hoisted(() => ({
  settings: { enabled: false, require_expiration: false } as
    SharingSettingsResponse | undefined,
}));

vi.mock("@/hooks/queries/sharing", () => ({
  useSharingSettings: (options?: { enabled?: boolean }) => ({
    data: options?.enabled === false ? undefined : sharing.settings,
  }),
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
  setAuth({ permissions: ALL_PERMISSIONS });
  sharing.settings = { enabled: false, require_expiration: false };
  resync.file = undefined;
  resync.book = undefined;
  deletion.book = undefined;
  delivery.rules = undefined;
  delivery.pending = false;
  review.criteria = { book_fields: [], audio_fields: [] };
  relatedTabs.tabs = [{ key: "series", label: "Series" }];
});

const { idle, review, resync, deletion } = vi.hoisted(() => ({
  idle: () => ({ mutateAsync: vi.fn(), mutate: vi.fn(), isPending: false }),
  review: {
    criteria: { book_fields: [] as string[], audio_fields: [] as string[] },
  },
  resync: {
    file: undefined as undefined | ((...args: never[]) => Promise<unknown>),
    book: undefined as undefined | ((...args: never[]) => Promise<unknown>),
  },
  deletion: {
    book: undefined as undefined | ((...args: never[]) => Promise<unknown>),
  },
}));

vi.mock("@/hooks/queries/books", () => ({
  useDeleteBook: () => ({
    ...idle(),
    ...(deletion.book ? { mutateAsync: deletion.book } : {}),
  }),
  useDeleteFile: idle,
  useResyncBook: () => ({
    ...idle(),
    ...(resync.book ? { mutateAsync: resync.book } : {}),
  }),
  useResyncFile: () => ({
    ...idle(),
    ...(resync.file ? { mutateAsync: resync.file } : {}),
  }),
}));
// BookDetailBody.permissions.test.tsx checks the request; this stand-in
// returns the user's default Delivery Rules.
const delivery = vi.hoisted(() => ({
  rules: undefined as Rule[] | undefined,
  pending: false,
}));
vi.mock("@/hooks/queries/delivery", () => ({
  useDeliveryRules: (options?: { enabled?: boolean }) => ({
    isPending: options?.enabled === false || delivery.pending,
    data:
      options?.enabled === false || !delivery.rules
        ? undefined
        : { follows_built_in: false, rules: delivery.rules, options: [] },
  }),
}));
// BookReadingControls.test.tsx drives the reading controls through the API.
vi.mock("@/hooks/queries/reading", () => ({
  useBookReading: () => ({
    data: {
      book_id: 1,
      status: "read",
      rating: 4,
      on_to_read: false,
      read_throughs: [
        {
          id: 1,
          book_id: 1,
          outcome: "finished",
          paused: false,
          end_day: "2024-03-02",
        },
      ],
    },
  }),
  useReadingTransition: idle,
  useSetRating: idle,
  useSetOnToRead: idle,
  useUpdateReadThrough: idle,
  useDeleteReadThrough: idle,
}));
vi.mock("@/hooks/queries/plugins", () => ({
  usePluginIdentifierTypes: () => ({ data: [] }),
}));
vi.mock("@/hooks/queries/review", () => ({
  useReviewCriteria: () => ({ data: review.criteria }),
}));
vi.mock("@/components/library/AddToListPopover", () => ({
  default: ({ trigger }: { trigger: ReactNode }) => (
    <div data-testid="add-to-list">{trigger}</div>
  ),
}));
vi.mock("@/components/library/BookEditDialog", () => ({
  BookEditDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="book-edit-dialog" /> : null,
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
  FileEditDialog: ({ file }: { file: File }) => (
    <div data-testid="file-edit-dialog">{file.id}</div>
  ),
}));
// BookRelatedPanel.test.tsx covers the panel and the hook that loads it;
// here the panel only marks its place, and a test can empty its tabs.
const relatedTabs = vi.hoisted(() => ({
  tabs: [{ key: "series", label: "Series" }] as {
    key: string;
    label: string;
  }[],
}));
vi.mock("@/hooks/useBookRelatedTabs", () => ({
  useBookRelatedTabs: (_book: unknown, options?: { enabled?: boolean }) => ({
    tabs: options?.enabled === false ? [] : relatedTabs.tabs,
    series: [],
    lists: [],
    linkPeople: true,
  }),
}));
vi.mock("@/components/library/BookRelatedPanel", () => ({
  default: () => <div data-testid="related-panel" />,
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
  reachable_file_types: ["epub", "kepub"],
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
  reachable_file_types: ["m4b"],
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
  downloadAsUrl: (file, fileType) =>
    `/api/share/tok/files/${file.id}/download/${fileType}`,
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

// Opens a file's menu and its File details dialog, and returns the dialog.
const openFileDetails = async (index = 0) => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  await user.click(
    screen.getAllByRole("button", { name: "File actions" })[index],
  );
  await user.click(screen.getByRole("menuitem", { name: "File details" }));
  return screen.getByRole("dialog");
};

const menuItems = async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  await user.click(screen.getByRole("button", { name: "Book actions" }));
  return within(screen.getByRole("menu"))
    .getAllByRole("menuitem")
    .map((item) => item.textContent);
};

describe("BookDetailBody in Share Link context", () => {
  it("renders resource names as plain text with no links into the app", () => {
    renderBody({
      book: { ...book, files: [{ ...epub, publisher: undefined }, m4b] },
      shareLink,
    });

    for (const name of [
      "Ada Author",
      "Great Series #2",
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
    renderBody({ shareLink });

    const dialog = await openFileDetails();
    expect(within(dialog).getByText("Tor Books").closest("a")).toBeNull();
  });

  it("hides every action control even with full permissions", async () => {
    renderBody({ book: { ...book, files: [epub, m4b] }, shareLink });

    expect(screen.queryByLabelText("Book actions")).not.toBeInTheDocument();
    for (const name of ["Edit", "Identify", "Share"]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
    expect(screen.queryByTestId("add-to-list")).not.toBeInTheDocument();
    expect(screen.queryByTestId("related-panel")).not.toBeInTheDocument();
    // The recipient isn't the reader: no reading status, rating, or history.
    expect(
      screen.queryByRole("button", { name: /^Reading status/ }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("slider")).not.toBeInTheDocument();
    expect(screen.queryByText("Reading history")).not.toBeInTheDocument();
    expect(screen.queryAllByRole("link")).toHaveLength(0);

    // A file's menu keeps only its details, which change nothing.
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(
      screen.getAllByRole("button", { name: "File actions" })[0],
    );
    expect(
      within(screen.getByRole("menu"))
        .getAllByRole("menuitem")
        .map((item) => item.textContent),
    ).toEqual(["File details"]);
  });

  it("shows no Needs review badge", () => {
    renderBody({
      book: { ...book, files: [{ ...epub, reviewed: false }] },
      shareLink,
    });

    expect(screen.queryByText("Needs review")).not.toBeInTheDocument();
  });

  it("offers no Share entry", () => {
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody({ shareLink });

    expect(screen.queryByLabelText("Book actions")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Share" }),
    ).not.toBeInTheDocument();
  });

  it("omits the sort title and the created and updated times", () => {
    renderBody({
      book: { ...book, sort_title: "Book, Test" },
      shareLink,
    });

    expect(screen.queryByText(/Sort title/)).not.toBeInTheDocument();
    expect(screen.queryByText("Book, Test")).not.toBeInTheDocument();
    expect(screen.queryByText("Added")).not.toBeInTheDocument();
    expect(screen.queryByText("Updated")).not.toBeInTheDocument();
  });

  it("omits file identifiers, the file URL, and the server's dates but keeps reader-facing details", async () => {
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

    const dialog = await openFileDetails();
    expect(within(dialog).getByText("Tor Books")).toBeInTheDocument();
    expect(within(dialog).getByText("Released")).toBeInTheDocument();
    expect(within(dialog).getByText("Language")).toBeInTheDocument();
    expect(within(dialog).queryByText("urn:uuid:1234")).not.toBeInTheDocument();
    expect(within(dialog).queryByText("URL")).not.toBeInTheDocument();
    expect(within(dialog).queryByText("Added")).not.toBeInTheDocument();
    expect(within(dialog).queryByText("Path")).not.toBeInTheDocument();
  });

  it("falls back to the file type when the payload blanks the path", () => {
    renderBody({ shareLink });

    const files = screen.getByRole("region", { name: "1 file" });
    // The name and the format pill both read EPUB.
    expect(within(files).getAllByText("EPUB").length).toBeGreaterThan(1);
  });

  it("uses the supplied cover URL builders", () => {
    const { container } = renderBody({ shareLink });

    expect(screen.getByAltText("Test Book Cover")).toHaveAttribute(
      "src",
      "/api/share/tok/cover?v=42-1&size=512&aspect=book&r=2",
    );
    const thumbnails = Array.from(container.querySelectorAll("img")).map(
      (img) => img.getAttribute("src"),
    );
    expect(thumbnails).toContain(
      "/api/share/tok/files/42/cover?size=512&aspect=book&r=2",
    );
  });

  it("downloads through the supplied download URL builder", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const fetchMock = vi.fn().mockResolvedValue(new Response(null));
    vi.stubGlobal("fetch", fetchMock);
    const assign = vi.fn();
    vi.stubGlobal("location", { ...window.location, assign });

    renderBody({ shareLink });

    await user.click(screen.getByRole("button", { name: "Download EPUB" }));

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/share/tok/files/42/download",
      expect.objectContaining({ method: "HEAD" }),
    );
    expect(assign).toHaveBeenCalledWith("/api/share/tok/files/42/download");
  });

  it("downloads each File Type a file can become through the supplied builder", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const fetchMock = vi.fn().mockResolvedValue(new Response(null));
    vi.stubGlobal("fetch", fetchMock);
    const assign = vi.fn();
    vi.stubGlobal("location", { ...window.location, assign });

    renderBody({ shareLink });

    await user.click(screen.getAllByRole("button", { name: "Download as" })[0]);
    await user.click(
      screen.getByRole("menuitem", {
        name: "Download Test Book as KePub (converted)",
      }),
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/share/tok/files/42/download/kepub",
      expect.objectContaining({ method: "HEAD" }),
    );
    expect(assign).toHaveBeenCalledWith(
      "/api/share/tok/files/42/download/kepub",
    );
  });

  it.each([
    [
      "the API's message",
      () =>
        Response.json(
          {
            error: { code: "conversion_failed", message: "Conversion failed" },
          },
          { status: 500 },
        ),
      "Conversion failed",
    ],
    [
      "the fallback for a proxy's error page",
      () =>
        new Response("<html>Bad gateway</html>", {
          status: 502,
          statusText: "Bad Gateway",
        }),
      "Failed to download file",
    ],
    [
      "the fallback for the server's generic internal error",
      () =>
        Response.json(
          {
            error: {
              code: "internal_server_error",
              message: "Internal Server Error",
            },
          },
          { status: 500 },
        ),
      "Failed to download file",
    ],
  ])("reports %s when a download fails", async (_, errorResponse, message) => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(new Response(null, { status: 500 }))
        .mockResolvedValueOnce(errorResponse()),
    );

    renderBody({ shareLink });
    await user.click(screen.getByRole("button", { name: "Download EPUB" }));

    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(screen.queryByText(/Request failed/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Internal Server Error/)).not.toBeInTheDocument();
  });

  it("toasts the fallback when the download cannot reach the server", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const error = vi.spyOn(toast, "error");
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new TypeError("Failed to fetch")),
    );

    renderBody({ shareLink });
    await user.click(screen.getByRole("button", { name: "Download EPUB" }));

    await waitFor(() => {
      expect(error).toHaveBeenCalledWith("Failed to download file", undefined);
    });
  });

  it("reports nothing when the download is cancelled while reading the error", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const aborted = new ReadableStream({
      start: (controller) =>
        controller.error(new DOMException("cancelled", "AbortError")),
    });
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(new Response(null, { status: 500 }))
        .mockResolvedValueOnce(new Response(aborted, { status: 500 })),
    );

    renderBody({ shareLink });
    await user.click(screen.getByRole("button", { name: "Download EPUB" }));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
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

    await user.click(screen.getByRole("button", { name: "Download PDF" }));

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

// The tags in a LabelGrid row: the pills in the dd after its dt.
const rowPills = (label: string) => {
  const dt = screen.getByText(label, { selector: "dt" });
  const pills = dt.nextElementSibling!.firstElementChild!;
  return Array.from(pills.children).map((el) => el.textContent);
};

const namespacedBook = {
  ...book,
  book_tags: [
    { id: 1, tag_id: 6, tag: { id: 6, name: "Favorite" } },
    { id: 2, tag_id: 7, tag: { id: 7, name: "Source: Digital purchase" } },
    { id: 3, tag_id: 8, tag: { id: 8, name: "circle:Moon Factory" } },
    { id: 4, tag_id: 9, tag: { id: 9, name: "Star Wars: Clone Wars" } },
  ],
} as unknown as Book;

describe("BookDetailBody tag namespaces", () => {
  it("lists each namespace as its own row, in the library's order, without the prefix", () => {
    renderBody({
      book: namespacedBook,
      library: {
        id: 1,
        name: "Doujin",
        cover_aspect_ratio: "book",
        organize_file_structure: false,
        tag_namespaces: ["Circle", "Magazine", "Source"],
      },
    });

    const labels = Array.from(document.querySelectorAll("dt")).map(
      (dt) => dt.textContent,
    );
    expect(labels).toEqual(
      expect.arrayContaining(["Tags", "Circle", "Source"]),
    );
    expect(labels).not.toContain("Magazine");
    expect(labels.indexOf("Tags")).toBeLessThan(labels.indexOf("Circle"));
    expect(labels.indexOf("Circle")).toBeLessThan(labels.indexOf("Source"));

    expect(rowPills("Tags")).toEqual(["Favorite", "Star Wars: Clone Wars"]);
    expect(rowPills("Circle")).toEqual(["Moon Factory"]);
    expect(screen.getByText("Moon Factory").closest("a")).toHaveAttribute(
      "href",
      "/libraries/1?tag_ids=8",
    );
  });

  it("groups tags on a Share Link page by the payload's namespaces", () => {
    renderBody({
      book: namespacedBook,
      shareLink: { ...shareLink, tagNamespaces: ["Circle"] },
    });

    expect(rowPills("Circle")).toEqual(["Moon Factory"]);
    expect(rowPills("Tags")).toEqual([
      "Favorite",
      "Source: Digital purchase",
      "Star Wars: Clone Wars",
    ]);
  });

  it("collapses a long row of tags behind a button that expands it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const manyTags = Array.from({ length: 20 }, (_, i) => ({
      id: i + 1,
      tag_id: i + 100,
      tag: { id: i + 100, name: `Tag ${i + 1}` },
    }));
    renderBody({ book: { ...book, book_tags: manyTags } as unknown as Book });

    expect(rowPills("Tags")).toEqual([
      ...manyTags.slice(0, 12).map((t) => t.tag.name),
      "+8 more",
    ]);

    await user.click(screen.getByRole("button", { name: "+8 more" }));
    expect(rowPills("Tags")).toEqual([
      ...manyTags.map((t) => t.tag.name),
      "Show less",
    ]);

    await user.click(screen.getByRole("button", { name: "Show less" }));
    expect(rowPills("Tags")).toHaveLength(13);
  });

  it.each([
    [14, 14, null],
    [15, 13, "+3 more"],
  ])(
    "shows %i tags as %i pills, hiding no fewer than three",
    (count, pills, button) => {
      const tags = Array.from({ length: count }, (_, i) => ({
        id: i + 1,
        tag_id: i + 100,
        tag: { id: i + 100, name: `Tag ${i + 1}` },
      }));
      renderBody({ book: { ...book, book_tags: tags } as unknown as Book });

      expect(rowPills("Tags")).toHaveLength(pills);
      if (button) {
        expect(screen.getByRole("button", { name: button })).toBeVisible();
      } else {
        expect(screen.queryByRole("button", { name: /more/ })).toBeNull();
      }
    },
  );

  it("lists every tag under Tags when the library has no namespaces", () => {
    renderBody({ book: namespacedBook });

    expect(rowPills("Tags")).toHaveLength(4);
    expect(screen.queryByText("Circle", { selector: "dt" })).toBeNull();
  });
});

describe("BookDetailBody without Share Link context", () => {
  it("shows the reader's status, rating, and history", () => {
    renderBody();

    expect(
      screen.getByRole("button", { name: "Reading status: Read" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Rating" })).toHaveAttribute(
      "aria-valuetext",
      "4 stars",
    );
    expect(screen.getByText("Reading history")).toBeInTheDocument();
    // The cover carries the status bookmark book cards show.
    expect(screen.getByRole("img", { name: "Read" })).toHaveAttribute(
      "data-status",
      "read",
    );
  });

  it("links resource names into the library", () => {
    renderBody();

    expect(screen.getByText("Ada Author").closest("a")).toHaveAttribute(
      "href",
      "/libraries/1/people/10",
    );
    expect(screen.getByText("Great Series #2").closest("a")).toHaveAttribute(
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

  it("names a comic's volume instead of numbering it", () => {
    renderBody({
      book: { ...book, files: [{ ...epub, file_type: "cbz" }] } as Book,
    });

    expect(screen.getByText("Great Series Vol. 2")).toBeInTheDocument();
  });

  it("links each file's name to its details page", () => {
    renderBody({
      book: { ...book, files: [{ ...epub, display_name: "Deluxe" }] } as Book,
    });

    expect(screen.getByRole("link", { name: "Deluxe" })).toHaveAttribute(
      "href",
      "/libraries/1/books/7/files/42",
    );
  });

  it("renders each file's Read and Listen action as one named link", () => {
    renderBody({ book: { ...book, files: [epub, m4b] } as Book });

    expect(screen.getByRole("link", { name: "Read" })).toHaveAttribute(
      "href",
      "/libraries/1/books/7/files/42/read",
    );
    const listen = screen.getByRole("link", { name: "Listen" });
    expect(listen).toHaveAttribute(
      "href",
      "/libraries/1/books/7/files/43/read",
    );
    // A button nested in the link would be a second tab stop with no name.
    for (const link of [screen.getByRole("link", { name: "Read" }), listen]) {
      expect(within(link).queryByRole("button")).toBeNull();
    }
  });

  it("renders author and series names as text without People and Series Read", () => {
    setAuth({ permissions: ["books:read"] });
    renderBody();

    expect(screen.getByText("Ada Author").closest("a")).toBeNull();
    expect(screen.getByText("Great Series #2").closest("a")).toBeNull();
    expect(screen.getByText("Fantasy").closest("a")).toHaveAttribute(
      "href",
      "/libraries/1?genre_ids=4",
    );
  });

  it("puts Edit and Identify in the header and the rest in the menu for Books Write", async () => {
    renderBody();

    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Identify" }),
    ).toBeInTheDocument();
    expect(await menuItems()).toEqual([
      "Book details",
      "Add to list",
      "Rescan book",
      "Merge into another book",
      "Delete book",
    ]);
    expect(
      within(screen.getByRole("menu")).getAllByRole("separator"),
    ).toHaveLength(2);
  });

  it("keeps Book details and Add to list for a read-only role", async () => {
    setAuth({ permissions: ["books:read"] });
    renderBody();

    expect(
      screen.queryByRole("button", { name: "Edit" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Identify" }),
    ).not.toBeInTheDocument();
    expect(await menuItems()).toEqual(["Book details", "Add to list"]);
  });

  it("shows the library, folder, sort title, and dates in Book details", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderBody({
      book: {
        ...book,
        filepath: "/books/Ada Author/Test Book",
        sort_title: "Test Book, The",
        library: { id: 1, name: "Main" },
      } as Book,
    });

    await user.click(screen.getByRole("button", { name: "Book actions" }));
    await user.click(screen.getByRole("menuitem", { name: "Book details" }));
    const dialog = screen.getByRole("dialog", { name: "Book details" });
    expect(within(dialog).getByText("Main")).toBeInTheDocument();
    expect(within(dialog).getByText("Test Book, The")).toBeInTheDocument();
    expect(
      within(dialog).getByText("Folder").nextElementSibling,
    ).toHaveTextContent("/books/Ada Author/Test Book");
    expect(within(dialog).getByText("Added")).toBeInTheDocument();
    expect(within(dialog).getByText("Updated")).toBeInTheDocument();
  });

  it("places the Related panel", () => {
    renderBody();

    // One beside the header on wide screens, one after the files otherwise.
    expect(screen.getAllByTestId("related-panel")).toHaveLength(2);
  });

  it("gives the header and the Related panel below it their own keys", () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    renderBody();

    expect(
      errors.mock.calls.filter((args) =>
        String(args[0]).includes("two children with the same key"),
      ),
    ).toEqual([]);
    errors.mockRestore();
  });

  it("leaves out the Related panel and its space when no tab has anything", () => {
    relatedTabs.tabs = [];
    const { container } = renderBody();

    expect(screen.queryByTestId("related-panel")).not.toBeInTheDocument();
    expect(container.querySelector("aside")).toBeNull();
  });
});

describe("BookDetailBody at phone width", () => {
  emulatePhoneWidth();

  it("keeps the header buttons' names, where their labels hide", () => {
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    for (const name of ["Edit", "Identify", "Share"]) {
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
    }
  });
});

describe("BookDetailBody Share button", () => {
  it("shows Share when sharing is enabled", () => {
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    expect(screen.getByRole("button", { name: "Share" })).toBeInTheDocument();
  });

  it("shows Share but not Edit or Identify for Shares Write without Books Write", async () => {
    setAuth({ permissions: ["books:read", "shares:read", "shares:write"] });
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    expect(screen.getByRole("button", { name: "Share" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Edit" }),
    ).not.toBeInTheDocument();
    expect(await menuItems()).toEqual(["Book details", "Add to list"]);
  });

  it("offers Share while sharing is disabled so links can be revoked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    setAuth({ permissions: ["books:read", "shares:read", "shares:write"] });
    renderBody();

    await user.click(screen.getByRole("button", { name: "Share" }));

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
    setAuth({ permissions: ["books:read", "shares:read", "shares:write"] });
    sharing.settings = undefined;
    renderBody();

    expect(
      screen.queryByRole("button", { name: "Share" }),
    ).not.toBeInTheDocument();
  });

  it("hides Share without a shares permission", () => {
    setAuth({ permissions: ["books:read", "books:write"] });
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    expect(
      screen.queryByRole("button", { name: "Share" }),
    ).not.toBeInTheDocument();
  });

  it("opens the dialog with the form, the policy, and the settings link for Config Write", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    setAuth({ permissions: [...ALL_PERMISSIONS, "config:write"] });
    sharing.settings = { enabled: true, require_expiration: true };
    renderBody();

    await user.click(screen.getByRole("button", { name: "Share" }));

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
    setAuth({ permissions: ["books:read", "shares:write"] });
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    await user.click(screen.getByRole("button", { name: "Share" }));

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
    setAuth({ permissions: ["books:read", "shares:read"] });
    sharing.settings = { enabled: true, require_expiration: false };
    renderBody();

    await user.click(screen.getByRole("button", { name: "Share" }));

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
  it("edits a file from its own button, as the book has", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderBody({ book: { ...book, files: [epub, m4b] } });

    await user.click(screen.getAllByRole("button", { name: "Edit file" })[1]);

    expect(screen.getByTestId("file-edit-dialog")).toHaveTextContent("43");
  });

  it("opens a file's details from its menu", async () => {
    renderBody({
      book: {
        ...book,
        files: [
          {
            ...epub,
            filepath: "/books/Test Book/test.epub",
            identifiers: [{ id: 1, type: "isbn_13", value: "9780000000001" }],
          } as File,
        ],
      },
    });

    const dialog = await openFileDetails();
    expect(within(dialog).getByText("Tor Books").closest("a")).toHaveAttribute(
      "href",
      "/libraries/1/publishers/5",
    );
    expect(
      within(dialog).getByText("/books/Test Book/test.epub"),
    ).toBeInTheDocument();
    expect(within(dialog).getByText("9780000000001")).toBeInTheDocument();
  });

  it("says Yes for an abridged file instead of repeating the label", async () => {
    renderBody({ book: { ...book, files: [{ ...epub, abridged: true }] } });

    const dialog = await openFileDetails();
    const label = within(dialog).getByText("Abridged");
    expect(label.nextElementSibling).toHaveTextContent("Yes");
  });

  it("says No for an unabridged audiobook", async () => {
    renderBody({ book: { ...book, files: [{ ...m4b, abridged: false }] } });

    const dialog = await openFileDetails();
    const label = within(dialog).getByText("Abridged");
    expect(label.nextElementSibling).toHaveTextContent("No");
  });

  it("shows a file's cover large from its row", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderBody({
      book: {
        ...book,
        files: [
          { ...epub, display_name: "Deluxe", cover_image_filename: "c.jpg" },
        ],
      } as Book,
    });

    await user.click(
      screen.getByRole("button", { name: "View cover of Deluxe" }),
    );
    const dialog = screen.getByRole("dialog", { name: "Deluxe" });
    expect(within(dialog).getByAltText("Deluxe cover")).toBeInTheDocument();
  });

  it("offers no cover view for a file without a cover", () => {
    renderBody();

    expect(
      screen.queryByRole("button", { name: /^View cover of/ }),
    ).not.toBeInTheDocument();
  });

  it("moves a main file but not a supplement from its menu", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const supplement = {
      ...epub,
      id: 44,
      file_type: "pdf",
      file_role: "supplement",
    } as File;
    renderBody({ book: { ...book, files: [epub, supplement] } });

    const [main, extra] = screen.getAllByRole("button", {
      name: "File actions",
    });
    await user.click(main);
    expect(
      screen.getByRole("menuitem", { name: "Move to another book" }),
    ).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await user.click(extra);
    expect(
      screen.queryByRole("menuitem", { name: "Move to another book" }),
    ).not.toBeInTheDocument();
  });

  it.each<[string, Rule[] | undefined, string, string]>([
    ["without rules", undefined, "CBZ", "/api/books/files/42/download"],
    [
      "under rules that deliver CBZ as itself",
      [{ source_file_type: "cbz", delivered_file_type: "cbz" }],
      "CBZ",
      "/api/books/files/42/download",
    ],
    [
      "under rules that deliver CBZ as EPUB",
      [
        { source_file_type: "epub", delivered_file_type: "epub" },
        { source_file_type: "cbz", delivered_file_type: "epub" },
      ],
      "EPUB",
      "/api/books/files/42/download/epub",
    ],
  ])(
    "downloads a CBZ from the main button %s",
    async (_, rules, label, endpoint) => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const fetchMock = vi.fn().mockResolvedValue(new Response(null));
      vi.stubGlobal("fetch", fetchMock);
      const assign = vi.fn();
      vi.stubGlobal("location", { ...window.location, assign });
      delivery.rules = rules;
      const cbz = {
        ...epub,
        file_type: "cbz",
        reachable_file_types: ["cbz", "kepub", "epub"],
      } as File;
      renderBody({ book: { ...book, files: [cbz] } });

      // The main button names the File Type it delivers.
      await user.click(
        screen.getByRole("button", { name: `Download ${label}` }),
      );

      expect(fetchMock).toHaveBeenCalledWith(
        endpoint,
        expect.objectContaining({ method: "HEAD" }),
      );
      expect(assign).toHaveBeenCalledWith(endpoint);
    },
  );

  it("offers the unmodified file when a download fails", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const assign = vi.fn();
    vi.stubGlobal("location", { ...window.location, assign });
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(new Response(null, { status: 422 }))
        .mockResolvedValueOnce(
          Response.json(
            {
              error: {
                code: "invalid_state",
                message: "Cannot write metadata",
              },
            },
            { status: 422 },
          ),
        ),
    );
    renderBody();

    await user.click(screen.getByRole("button", { name: "Download EPUB" }));
    const dialog = await screen.findByRole("dialog", {
      name: "Download Failed",
    });
    expect(dialog).toHaveTextContent("without the metadata Shisho writes");
    await user.click(
      within(dialog).getByRole("button", { name: "Download unmodified file" }),
    );

    expect(assign).toHaveBeenCalledWith(
      expect.stringContaining("/api/books/files/42/download/original"),
    );
  });

  it("disables the main button until the Delivery Rules load", () => {
    delivery.pending = true;
    renderBody({ book: { ...book, files: [epub] } });

    expect(
      screen.getByRole("button", { name: "Download EPUB" }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: "Download as" })).toBeEnabled();
  });

  it("downloads a CBZ as EPUB from the download menu", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const fetchMock = vi.fn().mockResolvedValue(new Response(null));
    vi.stubGlobal("fetch", fetchMock);
    const assign = vi.fn();
    vi.stubGlobal("location", { ...window.location, assign });
    const cbz = {
      ...epub,
      file_type: "cbz",
      name: "Deluxe",
      reachable_file_types: ["cbz", "kepub", "epub"],
    } as File;
    renderBody({ book: { ...book, files: [cbz] } });

    await user.click(screen.getByRole("button", { name: "Download as" }));
    expect(
      within(screen.getByRole("menu"))
        .getAllByRole("menuitem")
        .map((item) => item.getAttribute("aria-label")),
    ).toEqual([
      "Download Deluxe as CBZ (original)",
      "Download Deluxe as KePub (converted)",
      "Download Deluxe as EPUB (converted)",
    ]);
    await user.click(
      screen.getByRole("menuitem", {
        name: "Download Deluxe as EPUB (converted)",
      }),
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/books/files/42/download/epub",
      expect.objectContaining({ method: "HEAD" }),
    );
    expect(assign).toHaveBeenCalledWith("/api/books/files/42/download/epub");
  });

  it("tells audiobooks apart by bitrate and leaves the language and filename to File details", () => {
    renderBody({
      book: {
        ...book,
        files: [
          {
            ...m4b,
            audiobook_duration_seconds: 31440,
            audiobook_bitrate_bps: 64000,
            release_date: "2015-01-01T00:00:00Z",
            language: "en",
            filepath: "/books/Test Book/Test Book {Reader}.m4b",
            display_name: "Test Book",
          },
        ],
      } as Book,
    });

    // The year reads in UTC, so January 1 stays in 2015 west of UTC.
    expect(
      screen.getByText("8h 44m · 64 kbps · 2015 · 1.95 KB"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/English/)).not.toBeInTheDocument();
    expect(
      screen.queryByText("Test Book {Reader}.m4b"),
    ).not.toBeInTheDocument();
  });

  it("uses the singular for a one-page file", () => {
    renderBody({
      book: { ...book, files: [{ ...epub, file_type: "cbz", page_count: 1 }] },
    });

    expect(screen.getByText(/1 page\b/)).toBeInTheDocument();
    expect(screen.queryByText(/1 pages/)).not.toBeInTheDocument();
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
    reachable_file_types: ["pdf"],
  } as unknown as File;

  it("labels a supplement with the server's display name, not its stale stored name", () => {
    renderBody({ book: { ...book, files: [epub, supplement] } });

    expect(screen.getByText("The Lighthouse Keeper.pdf")).toBeInTheDocument();
    expect(screen.queryByText("Original Title")).not.toBeInTheDocument();
  });

  it("shows a supplement's cover, or its first page when it has none", () => {
    renderBody({
      book: {
        ...book,
        files: [
          epub,
          { ...supplement, cover_image_filename: "cover.jpg" },
          {
            ...supplement,
            id: 45,
            display_name: "Map.pdf",
            page_count: 3,
          } as File,
        ],
      },
    });

    expect(
      screen.getByRole("button", {
        name: "View cover of The Lighthouse Keeper.pdf",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "View first page of Map.pdf" }),
    ).toBeInTheDocument();
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

describe("BookDetailBody review badge", () => {
  const needsReview = (overrides: Partial<File> = {}) =>
    ({ ...epub, reviewed: false, ...overrides }) as File;

  it("shows no badge on a reviewed book", () => {
    renderBody();

    expect(screen.queryByText("Needs review")).not.toBeInTheDocument();
  });

  it("says what the book and its files are missing", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    review.criteria = {
      book_fields: ["description", "language"],
      audio_fields: [],
    };
    renderBody({
      book: {
        ...book,
        files: [needsReview({ display_name: "Deluxe" })],
      } as Book,
    });

    await user.hover(screen.getByRole("button", { name: "Needs review" }));
    act(() => vi.advanceTimersByTime(800));

    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("Book: missing description");
    expect(tooltip).toHaveTextContent("Deluxe: missing language");
  });

  it("opens the book editor when a book field is missing", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    review.criteria = { book_fields: ["description"], audio_fields: [] };
    renderBody({ book: { ...book, files: [needsReview()] } });

    await user.click(screen.getByRole("button", { name: "Needs review" }));

    expect(screen.getByTestId("book-edit-dialog")).toBeInTheDocument();
  });

  it("opens the file's editor when only a file field is missing", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    review.criteria = { book_fields: ["language"], audio_fields: [] };
    renderBody({ book: { ...book, files: [needsReview()] } });

    await user.click(screen.getByRole("button", { name: "Needs review" }));

    expect(screen.getByTestId("file-edit-dialog")).toHaveTextContent("42");
    expect(screen.queryByTestId("book-edit-dialog")).not.toBeInTheDocument();
  });

  it("opens no editor for a role without Books Write", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    setAuth({ permissions: ["books:read"] });
    review.criteria = { book_fields: ["description"], audio_fields: [] };
    renderBody({ book: { ...book, files: [needsReview()] } });

    await user.click(screen.getByRole("button", { name: "Needs review" }));

    expect(screen.queryByTestId("book-edit-dialog")).not.toBeInTheDocument();
  });
});

describe("BookDetailBody rescans that remove the book", () => {
  const renderOnBookPage = () =>
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={["/libraries/1/books/1"]}>
          <Routes>
            <Route element={<div>Library page</div>} path="/libraries/1" />
            <Route element={<BookDetailBody book={book} />} path="*" />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

  const rescanFirstFile = async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await user.click(screen.getAllByLabelText("File actions")[0]);
    await user.click(screen.getByRole("menuitem", { name: "Rescan file" }));
    await user.click(screen.getByRole("button", { name: "Rescan" }));
  };

  it("leaves the book page when a file rescan removes the book", async () => {
    resync.file = vi.fn(async () => ({
      file_deleted: true,
      book_deleted: true,
    }));
    renderOnBookPage();

    await rescanFirstFile();

    expect(await screen.findByText("Library page")).toBeInTheDocument();
  });

  it("stays on the book page when a file rescan removes only the file", async () => {
    resync.file = vi.fn(async () => ({
      file_deleted: true,
      book_deleted: false,
    }));
    renderOnBookPage();

    const success = vi.spyOn(toast, "success");
    await rescanFirstFile();

    await waitFor(() =>
      expect(success).toHaveBeenCalledWith(
        "File removed (no longer on disk, or DRM-protected)",
      ),
    );
    expect(screen.queryByText("Library page")).not.toBeInTheDocument();
  });

  it("leaves the book page when a book rescan removes the book", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    resync.book = vi.fn(async () => ({ book_deleted: true }));
    renderOnBookPage();

    await user.click(screen.getByLabelText("Book actions"));
    await user.click(screen.getByRole("menuitem", { name: "Rescan book" }));
    await user.click(screen.getByRole("button", { name: "Rescan" }));

    expect(await screen.findByText("Library page")).toBeInTheDocument();
  });
});

describe("BookDetailBody leaving a removed book", () => {
  const nothingRemoved: RemovedEntities = {
    series_ids: [],
    person_ids: [],
    genre_ids: [],
    tag_ids: [],
    publisher_ids: [],
  };

  const deleteBook = async (user: ReturnType<typeof userEvent.setup>) => {
    await user.click(screen.getByLabelText("Book actions"));
    await user.click(screen.getByRole("menuitem", { name: "Delete book" }));
    await user.click(screen.getByRole("button", { name: "Delete" }));
  };
  const rescanBook = async (user: ReturnType<typeof userEvent.setup>) => {
    await user.click(screen.getByLabelText("Book actions"));
    await user.click(screen.getByRole("menuitem", { name: "Rescan book" }));
    await user.click(screen.getByRole("button", { name: "Rescan" }));
  };

  // Opens the book from Series 4's page, then removes it with `action`.
  const removeFromSeriesPage = async (
    action: typeof deleteBook,
    removed: RemovedEntities,
  ) => {
    deletion.book = vi.fn(async () => ({ files_deleted: 1, removed }));
    resync.book = vi.fn(async () => ({ book_deleted: true, removed }));
    const router = createMemoryRouter(
      [
        {
          path: "/",
          element: (
            <NavigationHistoryProvider>
              <Outlet />
            </NavigationHistoryProvider>
          ),
          children: [
            { path: "libraries/1", element: <div>Library page</div> },
            { path: "libraries/1/series/4", element: <div>Series page</div> },
            {
              path: "libraries/1/books/7",
              element: <BookDetailBody book={book} />,
            },
          ],
        },
      ],
      { initialEntries: ["/libraries/1/series/4"] },
    );
    render(
      <QueryClientProvider client={new QueryClient()}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
    await act(() => router.navigate("/libraries/1/books/7"));

    await action(userEvent.setup({ advanceTimers: vi.advanceTimersByTime }));
  };

  it.each([
    ["a delete", deleteBook],
    ["a rescan", rescanBook],
  ])(
    "returns to the page the book was opened from after %s",
    async (_name, action) => {
      await removeFromSeriesPage(action, nothingRemoved);

      expect(await screen.findByText("Series page")).toBeInTheDocument();
    },
  );

  it.each([
    ["a delete", deleteBook],
    ["a rescan", rescanBook],
  ])(
    "goes to the library when %s removed that page's series",
    async (_name, action) => {
      await removeFromSeriesPage(action, {
        ...nothingRemoved,
        series_ids: [4],
      });

      expect(await screen.findByText("Library page")).toBeInTheDocument();
    },
  );
});
