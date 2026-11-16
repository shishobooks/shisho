import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { DeviceResponse } from "@/types/generated/devices";
import type {
  LinkCandidate,
  UnrecognizedBook,
} from "@/types/generated/koreaderplugin";

import KOReaderUnrecognizedBooks from "./KOReaderUnrecognizedBooks";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
const toastSuccess = vi.hoisted(() => vi.fn());
vi.mock("sonner", () => ({
  toast: { success: toastSuccess, error: vi.fn(), dismiss: vi.fn() },
}));

const DOCUMENT = "0123456789abcdef0123456789abcdef";
const LIST = "/user/devices/koreader-1/koreader-unrecognized";

const device = {
  id: "koreader-1",
  device_type: "koreader",
  connections: ["koreader_plugin", "opds", "koreader_sync"],
} as DeviceResponse;

const candidate = (overrides: Partial<LinkCandidate> = {}): LinkCandidate => ({
  file_id: 41,
  book_id: 4,
  library_id: 1,
  title: "The Left Hand of Darkness",
  authors: ["Ursula K. Le Guin"],
  file_type: "epub",
  library_name: "Fiction",
  match: "identifier",
  ...overrides,
});

const unrecognized = (
  overrides: Partial<UnrecognizedBook> = {},
): UnrecognizedBook => ({
  document: DOCUMENT,
  title: "Left Hand of Darkness",
  authors: ["Ursula Le Guin"],
  identifiers: [],
  reading_seconds: 3 * 3600 + 25 * 60,
  last_read_at: new Date(Date.now() - 2 * 3600 * 1000).toISOString(),
  reading_deleted: false,
  suggestions: [
    candidate(),
    candidate({
      file_id: 52,
      book_id: 5,
      library_id: 2,
      file_type: "kepub",
      library_name: "Classics",
      match: "search",
    }),
  ],
  ...overrides,
});

// stubAPI answers the unrecognized books routes from an in-memory list,
// applying each link, unlink, dismiss, restore, and deletion, and records
// every request.
const stubAPI = (
  initial: UnrecognizedBook[],
  searchResults: LinkCandidate[] = [],
) => {
  let books = initial;
  return vi
    .spyOn(API, "request")
    .mockImplementation(async (method, path, body) => {
      if (method === "GET" && path === LIST) return books;
      if (method === "GET" && path === `${LIST}/candidates`) {
        return searchResults;
      }
      const link = path.match(/\/koreader-unrecognized\/(\w+)\/link$/);
      if (method === "POST" && link) {
        const { file_id } = body as { file_id: number };
        const all = [...books.flatMap((b) => b.suggestions), ...searchResults];
        books = books.map((b) =>
          b.document === link[1]
            ? {
                ...b,
                suggestions: [],
                linked_at: new Date().toISOString(),
                linked: all.find((c) => c.file_id === file_id) ?? null,
              }
            : b,
        );
        return undefined;
      }
      const unlink = path.match(/\/koreader-unrecognized\/(\w+)\/unlink$/);
      if (method === "POST" && unlink) {
        books = books.map((b) =>
          b.document === unlink[1]
            ? { ...b, linked_at: null, linked: null }
            : b,
        );
        return undefined;
      }
      if (method === "POST" && path === `${LIST}/dismiss-all`) {
        const { documents } = body as { documents: string[] };
        books = books.map((b) =>
          documents.includes(b.document)
            ? { ...b, suggestions: [], dismissed_at: new Date().toISOString() }
            : b,
        );
        return undefined;
      }
      const copy = path.match(
        /\/koreader-unrecognized\/(\w+)\/(dismiss|restore|delete-reading)$/,
      );
      if (method === "POST" && copy) {
        const [, document, action] = copy;
        books = books.map((b) => {
          if (b.document !== document) return b;
          if (action === "restore") {
            return { ...b, dismissed_at: null, reading_deleted: false };
          }
          return {
            ...b,
            suggestions: [],
            dismissed_at: b.dismissed_at ?? new Date().toISOString(),
            reading_deleted: action === "delete-reading",
          };
        });
        return undefined;
      }
      throw new Error(`unexpected request ${method} ${path}`);
    });
};

const renderSection = (client = new QueryClient()) =>
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <KOReaderUnrecognizedBooks device={device} />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const setupUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const posts = (request: ReturnType<typeof stubAPI>) =>
  request.mock.calls.filter((call) => call[0] === "POST");

const lastPost = (request: ReturnType<typeof stubAPI>) => {
  const all = posts(request);
  return all[all.length - 1];
};

describe("KOReaderUnrecognizedBooks", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));
  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it("shows each book as KOReader knows it, with its best match, and the rest under More", async () => {
    stubAPI([unrecognized()]);
    const user = setupUser();
    renderSection();

    const row = await screen.findByRole("listitem", {
      name: "Left Hand of Darkness",
    });
    expect(row).toHaveTextContent(
      "Ursula Le Guin · 3h 25m read in KOReader · Last read 2 hours ago",
    );
    // Collapsed, it offers its best match and Dismiss, in one click each.
    const best = within(row).getByRole("list", {
      name: "Best match for Left Hand of Darkness",
    });
    expect(within(best).getAllByRole("listitem")).toHaveLength(1);
    expect(best).toHaveTextContent("The Left Hand of Darkness");
    expect(best).toHaveTextContent("Same identifier");
    expect(
      within(row).getByRole("button", {
        name: "Dismiss Left Hand of Darkness",
      }),
    ).toBeInTheDocument();

    await user.click(
      within(row).getByRole("button", {
        name: "More options for Left Hand of Darkness",
      }),
    );
    const suggestions = within(row).getByRole("list", {
      name: "Suggestions for Left Hand of Darkness",
    });
    const items = within(suggestions).getAllByRole("listitem");
    expect(items).toHaveLength(2);
    // Identifier matches come first, and each names what tells it apart.
    expect(items[0]).toHaveTextContent("The Left Hand of Darkness");
    expect(items[0]).toHaveTextContent("Same identifier");
    expect(items[0]).toHaveTextContent("Ursula K. Le Guin · EPUB · Fiction");
    expect(items[1]).toHaveTextContent("Ursula K. Le Guin · KePub · Classics");
    expect(items[1]).not.toHaveTextContent("Same identifier");
  });

  it("names a book KOReader knows no title for by its hash", async () => {
    stubAPI([unrecognized({ title: null, authors: [] })]);
    renderSection();

    expect(
      await screen.findByRole("listitem", { name: "Untitled (01234567)" }),
    ).toBeInTheDocument();
  });

  it("links a book to a suggestion in one click, then shows the Book it's linked to", async () => {
    const request = stubAPI([unrecognized()]);
    const client = new QueryClient();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const user = setupUser();
    renderSection(client);

    await user.click(
      await screen.findByRole("button", {
        name: "Link to The Left Hand of Darkness, EPUB in Fiction",
      }),
    );

    expect(posts(request)).toEqual([
      ["POST", `${LIST}/${DOCUMENT}/link`, { file_id: 41 }],
    ]);
    // The held reading data now shows on the Book's page, cards, history,
    // and stats.
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["BookReading"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["ListBooks"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["ReadingHistory"] });

    const bookLink = await screen.findByRole("link", {
      name: "The Left Hand of Darkness",
    });
    expect(bookLink).toHaveAttribute("href", "/libraries/1/books/4");
    expect(
      screen.getByText(/syncs your place as a percentage only/),
    ).toHaveTextContent(
      "Download the book again from Shisho to sync your exact place.",
    );
    // A linked book has nothing left to link or dismiss.
    expect(screen.queryByRole("button", { name: /^Link to / })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Dismiss / })).toBeNull();
  });

  it("says when the File a book was linked to has been deleted", async () => {
    stubAPI([
      unrecognized({
        suggestions: [],
        linked_at: "2026-11-01T00:00:00Z",
        linked: null,
      }),
    ]);
    renderSection();

    expect(
      await screen.findByText("Linked to a file that has since been deleted."),
    ).toBeInTheDocument();
  });

  it("searches for the book when no suggestion fits, and links a result", async () => {
    const result = candidate({
      file_id: 77,
      book_id: 9,
      library_id: 3,
      title: "The Dispossessed",
      file_type: "epub",
      library_name: "Sci-Fi",
      match: "search",
    });
    const request = stubAPI([unrecognized({ suggestions: [] })], [result]);
    const user = setupUser();
    renderSection();

    const row = await screen.findByRole("listitem", {
      name: "Left Hand of Darkness",
    });
    expect(within(row).queryByRole("list", { name: /^Best match/ })).toBeNull();
    await user.click(
      screen.getByRole("button", {
        name: "More options for Left Hand of Darkness",
      }),
    );
    expect(
      screen.getByText("No suggestions. Search for the book to link it."),
    ).toBeInTheDocument();
    await user.type(
      screen.getByRole("textbox", {
        name: "Search for Left Hand of Darkness",
      }),
      "dispossessed",
    );
    act(() => vi.advanceTimersByTime(300));

    const results = await screen.findByRole("list", {
      name: "Search results for Left Hand of Darkness",
    });
    // The search waits for typing to pause, so it runs once.
    const searches = request.mock.calls.filter(
      (call) => call[1] === `${LIST}/candidates`,
    );
    expect(searches).toEqual([
      [
        "GET",
        `${LIST}/candidates`,
        null,
        { q: "dispossessed" },
        expect.anything(),
      ],
    ]);
    expect(results).toHaveTextContent("Ursula K. Le Guin · EPUB · Sci-Fi");

    await user.click(
      within(results).getByRole("button", {
        name: "Link to The Dispossessed, EPUB in Sci-Fi",
      }),
    );
    expect(posts(request)).toEqual([
      ["POST", `${LIST}/${DOCUMENT}/link`, { file_id: 77 }],
    ]);
    expect(
      await screen.findByRole("link", { name: "The Dispossessed" }),
    ).toHaveAttribute("href", "/libraries/3/books/9");
  });

  it("says when a search finds nothing", async () => {
    stubAPI([unrecognized({ suggestions: [] })], []);
    const user = setupUser();
    renderSection();

    await user.click(
      await screen.findByRole("button", {
        name: "More options for Left Hand of Darkness",
      }),
    );
    await user.type(
      await screen.findByRole("textbox", {
        name: "Search for Left Hand of Darkness",
      }),
      "nothing like it",
    );
    act(() => vi.advanceTimersByTime(300));

    expect(
      await screen.findByText("No books match your search."),
    ).toBeInTheDocument();
  });

  it("dismisses a book in one click, with Undo, keeping it under Dismissed", async () => {
    const request = stubAPI([unrecognized()]);
    const user = setupUser();
    renderSection();

    await user.click(
      await screen.findByRole("button", {
        name: "Dismiss Left Hand of Darkness",
      }),
    );

    expect(posts(request)).toEqual([["POST", `${LIST}/${DOCUMENT}/dismiss`]]);
    await waitFor(() =>
      expect(
        screen.queryByRole("list", { name: "Unrecognized books" }),
      ).toBeNull(),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(toastSuccess).toHaveBeenCalledWith(
      "Dismissed Left Hand of Darkness",
      expect.anything(),
    );

    // Undo, the app's own button in the message, restores it.
    const calls = toastSuccess.mock.calls;
    const [, options] = calls[calls.length - 1];
    const message = render(<>{options.description}</>);
    await user.click(
      within(message.container).getByRole("button", { name: "Undo" }),
    );
    expect(lastPost(request)).toEqual(["POST", `${LIST}/${DOCUMENT}/restore`]);
    await screen.findByRole("list", { name: "Unrecognized books" });
  });

  it("restores a dismissed book to link, and deletes its reading only once confirmed", async () => {
    const request = stubAPI([
      unrecognized({ suggestions: [], dismissed_at: "2026-11-01T00:00:00Z" }),
    ]);
    const user = setupUser();
    renderSection();

    await user.click(
      await screen.findByRole("button", { name: "Dismissed (1)" }),
    );
    const list = screen.getByRole("list", { name: "Dismissed books" });
    expect(list).toHaveTextContent("Left Hand of Darkness");
    expect(list).toHaveTextContent("3h 25m read in KOReader");

    await user.click(
      within(list).getByRole("button", {
        name: "Delete the reading history of Left Hand of Darkness",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("Send reading history again");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(posts(request)).toEqual([]);

    await user.click(
      within(list).getByRole("button", {
        name: "Delete the reading history of Left Hand of Darkness",
      }),
    );
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Delete",
      }),
    );
    expect(posts(request)).toEqual([
      ["POST", `${LIST}/${DOCUMENT}/delete-reading`],
    ]);
    await waitFor(() =>
      expect(
        screen.getByRole("list", { name: "Dismissed books" }),
      ).toHaveTextContent("Reading history deleted"),
    );
    expect(
      screen.queryByRole("button", {
        name: "Delete the reading history of Left Hand of Darkness",
      }),
    ).toBeNull();

    await user.click(
      screen.getByRole("button", { name: "Restore Left Hand of Darkness" }),
    );
    // Its history comes back only from the e-reader.
    expect(toastSuccess).toHaveBeenCalledWith(
      "Restored Left Hand of Darkness",
      expect.objectContaining({
        description: expect.stringContaining("Send reading history again"),
      }),
    );
    expect(lastPost(request)).toEqual(["POST", `${LIST}/${DOCUMENT}/restore`]);
    await screen.findByRole("list", { name: "Unrecognized books" });
    expect(screen.queryByRole("button", { name: /^Dismissed/ })).toBeNull();
  });

  it("deletes reading history without asking once told not to ask again", async () => {
    const second = "e".repeat(32);
    const request = stubAPI([
      unrecognized({ suggestions: [], dismissed_at: "2026-11-01T00:00:00Z" }),
      unrecognized({
        document: second,
        title: "Another Copy",
        suggestions: [],
        dismissed_at: "2026-11-01T00:00:00Z",
      }),
    ]);
    const user = setupUser();
    renderSection();

    await user.click(
      await screen.findByRole("button", { name: "Dismissed (2)" }),
    );
    await user.click(
      screen.getByRole("button", {
        name: "Delete the reading history of Left Hand of Darkness",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("checkbox", { name: "Don't ask again" }),
    );
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(
      screen.getByRole("button", {
        name: "Delete the reading history of Another Copy",
      }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(posts(request)).toEqual([
      ["POST", `${LIST}/${DOCUMENT}/delete-reading`],
      ["POST", `${LIST}/${second}/delete-reading`],
    ]);
  });

  it("keeps asking before deleting reading history unless told not to", async () => {
    stubAPI([
      unrecognized({ suggestions: [], dismissed_at: "2026-11-01T00:00:00Z" }),
      unrecognized({
        document: "e".repeat(32),
        title: "Another Copy",
        suggestions: [],
        dismissed_at: "2026-11-01T00:00:00Z",
      }),
    ]);
    const user = setupUser();
    renderSection();

    await user.click(
      await screen.findByRole("button", { name: "Dismissed (2)" }),
    );
    await user.click(
      screen.getByRole("button", {
        name: "Delete the reading history of Left Hand of Darkness",
      }),
    );
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Delete",
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(
      screen.getByRole("button", {
        name: "Delete the reading history of Another Copy",
      }),
    );
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });

  it("unlinks a book only once confirmed, listing it to link again", async () => {
    const request = stubAPI([
      unrecognized({
        suggestions: [],
        linked_at: "2026-11-01T00:00:00Z",
        linked: {
          file_id: 52,
          book_id: 5,
          library_id: 2,
          title: "The Left Hand of Darkness",
          authors: [],
          file_type: "epub",
          library_name: "Classics",
        },
      }),
    ]);
    const user = setupUser();
    renderSection();

    await user.click(
      await screen.findByRole("button", {
        name: "Unlink Left Hand of Darkness",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("listed to link again");
    expect(dialog).toHaveTextContent("the link added stay");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(posts(request)).toEqual([]);

    await user.click(
      screen.getByRole("button", { name: "Unlink Left Hand of Darkness" }),
    );
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Unlink",
      }),
    );
    expect(posts(request)).toEqual([["POST", `${LIST}/${DOCUMENT}/unlink`]]);
    expect(
      await screen.findByRole("button", {
        name: "More options for Left Hand of Darkness",
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "The Left Hand of Darkness" }),
    ).toBeNull();
  });

  it("shows the first books to link, the rest on request, and folds the linked ones away", async () => {
    const toLink = Array.from({ length: 12 }, (_, i) =>
      unrecognized({
        document: `${i}`.padStart(32, "a"),
        title: `Copy ${i + 1}`,
      }),
    );
    const linked = unrecognized({
      document: "f".repeat(32),
      title: "Linked Copy",
      suggestions: [],
      linked_at: "2026-11-01T00:00:00Z",
      linked: candidate({ match: undefined }),
    });
    stubAPI([...toLink, linked]);
    const user = setupUser();
    renderSection();

    const list = await screen.findByRole("list", {
      name: "Unrecognized books",
    });
    expect(screen.getByText("12 to link")).toBeInTheDocument();
    expect(
      within(list).getAllByRole("listitem", { name: /^Copy / }),
    ).toHaveLength(10);
    await user.click(screen.getByRole("button", { name: "Show 2 more" }));
    expect(
      within(list).getAllByRole("listitem", { name: /^Copy / }),
    ).toHaveLength(12);

    // Linked books wait behind their heading while there are books to link.
    expect(screen.queryByRole("list", { name: "Linked books" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Linked (1)" }));
    expect(
      within(screen.getByRole("list", { name: "Linked books" })).getByRole(
        "listitem",
        { name: "Linked Copy" },
      ),
    ).toHaveTextContent("Linked to The Left Hand of Darkness · EPUB · Fiction");
  });

  it("dismisses every book still to link only once confirmed, leaving the linked ones", async () => {
    const linked = unrecognized({
      document: "f".repeat(32),
      title: "Linked Copy",
      suggestions: [],
      linked_at: "2026-11-01T00:00:00Z",
      linked: candidate({ match: undefined }),
    });
    const request = stubAPI([
      unrecognized(),
      unrecognized({ document: "e".repeat(32), title: "Another Copy" }),
      linked,
    ]);
    const user = setupUser();
    renderSection();

    await user.click(
      await screen.findByRole("button", { name: "Dismiss all" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent(
      "All 2 books still to link move to Dismissed. Their reading history stays held",
    );
    expect(dialog).toHaveTextContent("Linked books stay.");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(posts(request)).toEqual([]);

    await user.click(screen.getByRole("button", { name: "Dismiss all" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Dismiss all",
      }),
    );
    // Only the books it listed.
    expect(posts(request)).toEqual([
      [
        "POST",
        `${LIST}/dismiss-all`,
        { documents: [DOCUMENT, "e".repeat(32)] },
      ],
    ]);
    await waitFor(() =>
      expect(
        screen.queryByRole("list", { name: "Unrecognized books" }),
      ).toBeNull(),
    );
    expect(
      screen.getByRole("list", { name: "Linked books" }),
    ).toHaveTextContent("Linked Copy");
    expect(
      screen.getByRole("button", { name: "Dismissed (2)" }),
    ).toBeInTheDocument();
  });

  it("keeps the linked books open while unlinking them", async () => {
    const linkedCopy = (n: number) =>
      unrecognized({
        document: `${n}`.repeat(32),
        title: `Linked ${n}`,
        suggestions: [],
        linked_at: "2026-11-01T00:00:00Z",
        linked: candidate({ match: undefined }),
      });
    stubAPI([linkedCopy(1), linkedCopy(2)]);
    const user = setupUser();
    renderSection();

    // With nothing to link, they start open.
    const list = await screen.findByRole("list", { name: "Linked books" });
    await user.click(
      within(list).getByRole("button", { name: "Unlink Linked 1" }),
    );
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Unlink",
      }),
    );

    await screen.findByRole("list", { name: "Unrecognized books" });
    expect(
      within(screen.getByRole("list", { name: "Linked books" })).getByRole(
        "listitem",
        { name: "Linked 2" },
      ),
    ).toBeInTheDocument();
  });

  it("offers Dismiss all only with more than one book to link", async () => {
    stubAPI([unrecognized()]);
    renderSection();

    await screen.findByRole("listitem", { name: "Left Hand of Darkness" });
    expect(screen.queryByRole("button", { name: "Dismiss all" })).toBeNull();
  });

  it("shows nothing when there are no unrecognized books", async () => {
    const request = stubAPI([]);
    renderSection();

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "GET",
        LIST,
        null,
        null,
        expect.anything(),
      ),
    );
    await act(async () => {});
    expect(
      screen.queryByRole("heading", { name: "Unrecognized Books" }),
    ).toBeNull();
  });
});
