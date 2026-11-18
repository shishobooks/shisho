import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";
import { queryClient } from "@/libraries/query-client";
import { setAuth } from "@/testing/auth";
import { mockCoverDimensions } from "@/testing/coverDimensions";
import type { Book, Bookmark, File } from "@/types";
import {
  AUTOPLAY_TIMEOUT_MS,
  RESTORE_SEEK_TIMEOUT_MS,
  SEEK_TIMEOUT_MS,
} from "@/utils/audioCodec";
import { loadBookSpeed, saveBookSpeed } from "@/utils/playbackSpeed";

import M4BReader from "./M4BReader";

mockCoverDimensions();

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

// The saved position, and what saves were sent.
const position = vi.hoisted(() => ({
  savedSeconds: null as number | null,
  // A saved position with no seconds, only a percentage, as a copy a user
  // linked stores (ADR 0017). It replaces savedSeconds when set.
  savedPercentageOnly: null as number | null,
  loading: false,
  loadFails: false,
  saves: [] as Array<Record<string, unknown>>,
}));

vi.mock("@/hooks/queries/reading", () => ({
  QueryKey: { FilePosition: "FilePosition" },
  invalidateAfterPositionUpdate: () => {},
  markProgressStale: () => {},
  useUndoAutoFinish: () => ({ mutate: () => {} }),
  useFilePosition: () => ({
    data:
      position.loading || position.loadFails
        ? undefined
        : {
            position:
              position.savedPercentageOnly !== null
                ? { percentage: position.savedPercentageOnly }
                : position.savedSeconds === null
                  ? null
                  : { seconds: position.savedSeconds, percentage: 0.5 },
          },
    error: position.loadFails ? new Error("offline") : null,
    isEnabled: true,
    isFetching: position.loading,
    isFetchedAfterMount: !position.loading,
  }),
  saveFilePosition: (_fileId: number, payload: Record<string, unknown>) => {
    position.saves.push(payload);
    return Promise.resolve({
      book_id: 7,
      accepted: true,
      started: false,
      resumed: false,
      reverted: false,
      finished_read_through_id: null,
    });
  },
}));

// The server's Bookmarks for the signed-in user, which the real bookmark
// query hooks reach through a stubbed API.request.
const bookmarkServer = {
  items: [] as Bookmark[],
  nextId: 1,
  failWrites: false,
};

const addServerBookmark = (seconds: number, note: string | null = null) => {
  const bookmark = {
    id: bookmarkServer.nextId++,
    file_id: 42,
    seconds,
    note,
    created_at: "2026-11-16T12:00:00Z",
    updated_at: "2026-11-16T12:00:00Z",
  } as Bookmark;
  bookmarkServer.items.push(bookmark);
  return bookmark;
};

const fakeBookmarksAPI = (
  method: string,
  path: string,
  body?: unknown,
): Promise<unknown> => {
  const payload = (body ?? {}) as { seconds?: number; note?: string | null };
  if (method !== "GET" && bookmarkServer.failWrites) {
    return Promise.reject(
      new ShishoAPIError("The server refused.", "internal_error", 500),
    );
  }
  if (method === "GET" && path === "/reading/files/42/bookmarks") {
    return Promise.resolve(
      [...bookmarkServer.items].sort((a, b) => a.seconds - b.seconds),
    );
  }
  if (method === "POST" && path === "/reading/files/42/bookmarks") {
    return Promise.resolve(
      addServerBookmark(payload.seconds ?? 0, payload.note || null),
    );
  }
  const match = path.match(/^\/reading\/bookmarks\/(\d+)$/);
  const bookmark = match
    ? bookmarkServer.items.find((b) => b.id === Number(match[1]))
    : undefined;
  if (bookmark && method === "PATCH") {
    bookmark.note = payload.note || null;
    return Promise.resolve({ ...bookmark });
  }
  if (bookmark && method === "DELETE") {
    bookmarkServer.items = bookmarkServer.items.filter((b) => b !== bookmark);
    return Promise.resolve(undefined);
  }
  return Promise.reject(new Error(`unexpected request ${method} ${path}`));
};

// jsdom does not implement the HTMLMediaElement playback pipeline. Stub the
// methods so the component can call them, and let tests drive currentTime /
// duration / paused manually plus dispatch the media events they assert on.
let playSpy: ReturnType<typeof vi.spyOn>;
let pauseSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  setAuth({ permissions: ["books:read"] });
  position.savedSeconds = null;
  position.savedPercentageOnly = null;
  position.loading = false;
  position.loadFails = false;
  position.saves = [];
  playSpy = vi
    .spyOn(window.HTMLMediaElement.prototype, "play")
    .mockResolvedValue(undefined);
  pauseSpy = vi
    .spyOn(window.HTMLMediaElement.prototype, "pause")
    .mockImplementation(() => {});
  vi.spyOn(window.HTMLMediaElement.prototype, "load").mockImplementation(
    () => {},
  );

  bookmarkServer.items = [];
  bookmarkServer.nextId = 1;
  bookmarkServer.failWrites = false;
  vi.spyOn(API, "request").mockImplementation(((
    method: string,
    path: string,
    body?: unknown,
  ) => fakeBookmarksAPI(method, path, body)) as never);
});

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  // Remove any per-test userAgent override (own property) so the prototype
  // getter is visible again for the next test.
  delete (window.navigator as { userAgent?: string }).userAgent;
  document.title = "";
});

const narratorPerson = { id: 9, name: "Jane Narrator" } as never;
const authorPerson = { id: 5, name: "Sam Author" } as never;

const file = {
  id: 42,
  book_id: 7,
  file_type: "m4b",
  filepath: "/lib/book.m4b",
  updated_at: "2024-01-01T00:00:00Z",
  audiobook_duration_seconds: 3600,
  narrators: [{ id: 1, file_id: 42, person_id: 9, person: narratorPerson }],
} as unknown as File;

// A file with three chapters spanning a 3600s book: 0–1200, 1200–2400, 2400–end.
// Chapter starts are in MILLISECONDS on the model.
const fileWithChapters = {
  ...file,
  chapters: [
    { id: 1, title: "Chapter One", start_timestamp_ms: 0, sort_order: 0 },
    {
      id: 2,
      title: "Chapter Two",
      start_timestamp_ms: 1200000,
      sort_order: 1,
    },
    {
      id: 3,
      title: "Chapter Three",
      start_timestamp_ms: 2400000,
      sort_order: 2,
    },
  ],
} as unknown as File;

const book = {
  id: 7,
  title: "The Test Audiobook",
  cover_cache_key: "7-123",
  authors: [{ id: 2, book_id: 7, person_id: 5, person: authorPerson }],
  files: [file],
} as unknown as Book;

// jsdom defines navigator.userAgent on the prototype; override it with an own
// property so codec-compatibility tests can impersonate specific browsers.
// vi.restoreAllMocks does not undo defineProperty, so remove it in afterEach.
const setUserAgent = (ua: string) => {
  Object.defineProperty(window.navigator, "userAgent", {
    value: ua,
    configurable: true,
  });
};

const CHROME_UA =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36";
const SAFARI_UA =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15";

// A fresh client per test with the app's defaults, so data stays as fresh
// as it does in the app: never stale until something invalidates it.
const appQueryClient = () =>
  new QueryClient({ defaultOptions: queryClient.getDefaultOptions() });

const renderReader = (props?: { book?: Book; file?: File }) =>
  render(
    <QueryClientProvider client={appQueryClient()}>
      <MemoryRouter>
        <M4BReader
          book={props?.book ?? book}
          file={props?.file ?? file}
          libraryId="1"
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const getAudio = () =>
  document.querySelector("audio") as HTMLAudioElement & {
    currentTime: number;
    duration: number;
  };

describe("M4BReader", () => {
  it("renders an audio element pointed at the stream endpoint", () => {
    renderReader();
    const audio = getAudio();
    expect(audio).toBeInTheDocument();
    expect(audio.getAttribute("src")).toBe("/api/books/files/42/stream");
  });

  it("does not autoplay on mount", () => {
    renderReader();
    expect(playSpy).not.toHaveBeenCalled();
    // Play button should be shown (paused state), not a pause button.
    expect(screen.getByRole("button", { name: /play/i })).toBeInTheDocument();
  });

  it("displays the book title, author, and narrator", () => {
    renderReader();
    expect(
      screen.getByRole("heading", { name: "The Test Audiobook" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/Sam Author/)).toBeInTheDocument();
    expect(screen.getByText(/Jane Narrator/)).toBeInTheDocument();
  });

  it("renders the selected file's cover with a cache-busted URL", () => {
    renderReader();
    const img = screen.getByRole("img") as HTMLImageElement;
    expect(img.getAttribute("src")).toBe(
      "/api/books/files/42/cover?v=1704067200000&size=512&aspect=book&r=2",
    );
  });

  it("sets the browser tab title to the book title", () => {
    renderReader();
    expect(document.title).toContain("The Test Audiobook");
  });

  it("shows a named file's name in the heading and tab title", () => {
    renderReader({ file: { ...file, name: "The Named Edition" } as File });
    expect(
      screen.getByRole("heading", { name: "The Named Edition" }),
    ).toBeInTheDocument();
    expect(document.title).toBe("Listening: The Named Edition - Shisho");
  });

  it.each([undefined, ""])(
    "falls back to the book title when the file's name is %j",
    (name) => {
      renderReader({ file: { ...file, name } as File });
      expect(
        screen.getByRole("heading", { name: "The Test Audiobook" }),
      ).toBeInTheDocument();
      expect(document.title).toBe("Listening: The Test Audiobook - Shisho");
    },
  );

  it("uses the file's audiobook_duration_seconds as the total time", () => {
    renderReader();
    // 3600s => 1:00:00
    expect(screen.getByText("1:00:00")).toBeInTheDocument();
  });

  it("plays when the play button is clicked and shows a pause control", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader();
    await user.click(screen.getByRole("button", { name: /play/i }));
    expect(playSpy).toHaveBeenCalled();

    // Simulate the audio element firing its play event.
    fireEvent.play(getAudio());
    expect(screen.getByRole("button", { name: /pause/i })).toBeInTheDocument();
  });

  it("pauses when the pause button is clicked while playing", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader();
    await user.click(screen.getByRole("button", { name: /play/i }));
    fireEvent.play(getAudio());
    await user.click(screen.getByRole("button", { name: /pause/i }));
    expect(pauseSpy).toHaveBeenCalled();
  });

  it("advances the elapsed time label as the audio reports timeupdate", () => {
    renderReader();
    const audio = getAudio();
    audio.currentTime = 65;
    fireEvent.timeUpdate(audio);
    expect(screen.getByText("1:05")).toBeInTheDocument();
  });

  it("seeks the audio when the seek bar value is committed", () => {
    renderReader();
    const audio = getAudio();
    // Radix sliders expose role=slider; simulate a keyboard seek which fires
    // value change + commit. Use the component's exposed test affordance: a
    // direct currentTime set via the commit path is asserted through the
    // slider's onValueCommit. We drive it via the slider thumb.
    const slider = screen.getByRole("slider", { name: /seek/i });
    slider.focus();
    // ArrowRight on a Radix slider increments by step (1s) and commits.
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    expect(audio.currentTime).toBeGreaterThan(0);
  });

  it("moves the focused seek bar by its own step, not the 30 second skip", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader();
    const slider = screen.getByRole("slider", { name: /seek/i });
    slider.focus();
    await user.keyboard("{ArrowRight}{ArrowRight}");
    expect(getAudio().currentTime).toBe(2);
    expect(screen.getByText("0:02")).toBeInTheDocument();
  });

  it("stays on the player and pauses at end of file", () => {
    renderReader();
    const audio = getAudio();
    audio.currentTime = 3600;
    fireEvent.play(audio);
    fireEvent.ended(audio);
    // After ended, the play (not pause) control is shown again.
    expect(screen.getByRole("button", { name: /play/i })).toBeInTheDocument();
  });

  it("toggles play/pause with the space key and prevents page scroll", () => {
    renderReader();
    // Initially paused: space should trigger play.
    const ev = new KeyboardEvent("keydown", {
      key: " ",
      bubbles: true,
      cancelable: true,
    });
    const prevented = !document.dispatchEvent(ev);
    expect(playSpy).toHaveBeenCalled();
    expect(prevented).toBe(true);
  });

  it("falls back to the audio element duration when no metadata duration", () => {
    const fileNoDuration = {
      ...file,
      audiobook_duration_seconds: undefined,
    } as unknown as File;
    renderReader({ file: fileNoDuration });
    const audio = getAudio();
    Object.defineProperty(audio, "duration", {
      value: 125,
      configurable: true,
    });
    fireEvent.loadedMetadata(audio);
    // 125s => 2:05
    expect(screen.getByText("2:05")).toBeInTheDocument();
  });

  describe("playback speed", () => {
    const speedButton = () => screen.getByRole("button", { name: /^speed /i });

    it("starts a Book with no saved speed at 1x", () => {
      saveBookSpeed(99, 2);
      renderReader();
      expect(speedButton()).toHaveTextContent("1x");
      expect(getAudio().playbackRate).toBe(1);
    });

    it("steps by 0.1 with the plus and minus buttons and remembers the Book's speed", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      const faster = screen.getByRole("button", { name: "Faster" });
      for (let i = 0; i < 5; i++) await user.click(faster);
      await user.click(screen.getByRole("button", { name: "Slower" }));

      expect(speedButton()).toHaveTextContent("1.4x");
      expect(getAudio().playbackRate).toBe(1.4);
      expect(loadBookSpeed(7)).toBe(1.4);
    });

    it("opens each Book at its own saved speed", () => {
      saveBookSpeed(7, 1.4);
      const view = renderReader();
      expect(speedButton()).toHaveTextContent("1.4x");
      expect(getAudio().playbackRate).toBe(1.4);

      const otherFile = { ...file, id: 43, book_id: 8 } as File;
      const otherBook = { ...book, id: 8, files: [otherFile] } as Book;
      view.rerender(
        <QueryClientProvider client={new QueryClient()}>
          <MemoryRouter>
            <M4BReader book={otherBook} file={otherFile} libraryId="1" />
          </MemoryRouter>
        </QueryClientProvider>,
      );
      expect(speedButton()).toHaveTextContent("1x");
      expect(getAudio().playbackRate).toBe(1);
      expect(loadBookSpeed(7)).toBe(1.4);
    });

    it("stops at 4x", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      saveBookSpeed(7, 3.9);
      renderReader();
      const faster = screen.getByRole("button", { name: "Faster" });
      await user.click(faster);
      expect(speedButton()).toHaveTextContent("4x");
      expect(faster).toBeDisabled();
      expect(getAudio().playbackRate).toBe(4);
      expect(loadBookSpeed(7)).toBe(4);
    });

    it("stops at 0.5x", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      saveBookSpeed(7, 0.6);
      renderReader();
      const slower = screen.getByRole("button", { name: "Slower" });
      await user.click(slower);
      expect(speedButton()).toHaveTextContent("0.5x");
      expect(slower).toBeDisabled();
      expect(loadBookSpeed(7)).toBe(0.5);
    });

    it("applies a preset from the speed menu", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      await user.click(speedButton());
      const presets = await screen.findAllByRole("menuitemradio");
      expect(presets.map((p) => p.textContent)).toEqual([
        "0.75x",
        "1x",
        "1.25x",
        "1.5x",
        "2x",
        "3x",
      ]);
      await user.click(screen.getByRole("menuitemradio", { name: "2x" }));

      expect(getAudio().playbackRate).toBe(2);
      expect(loadBookSpeed(7)).toBe(2);
    });

    it("leaves Space and the arrows to the open speed menu", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      await user.click(speedButton());
      const menu = await screen.findByRole("menu");
      for (const key of [" ", "ArrowRight", "ArrowLeft"]) {
        fireEvent.keyDown(menu, { key });
      }
      expect(playSpy).not.toHaveBeenCalled();
      expect(getAudio().currentTime).toBe(0);
    });

    it("re-applies the speed after the media load resets the rate", () => {
      saveBookSpeed(7, 2.5);
      renderReader();
      const audio = getAudio();

      // The HTML media load algorithm resets playbackRate to the default
      // (1) as part of loading the resource, which can land after the
      // component applied the saved speed. Simulate the reset, then the
      // metadata arriving.
      audio.playbackRate = 1;
      fireEvent.loadedMetadata(audio);

      expect(audio.playbackRate).toBe(2.5);
    });
  });

  describe("bookmarks", () => {
    const openBookmarks = async (user: ReturnType<typeof userEvent.setup>) => {
      await user.click(screen.getByRole("button", { name: "Bookmarks" }));
      return screen.findByRole("dialog", { name: "Bookmarks" });
    };

    it("lists the File's Bookmarks in time order", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      addServerBookmark(600);
      addServerBookmark(83, "test");
      renderReader();

      const panel = await openBookmarks(user);
      const jumps = await within(panel).findAllByRole("button", {
        name: /^jump to/i,
      });
      expect(jumps.map((b) => b.textContent)).toEqual([
        "Jump to 1:23 test",
        "Jump to 10:00",
      ]);
    });

    it("shows Bookmarks added elsewhere when the panel opens again", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      addServerBookmark(83, "test");
      renderReader();
      let panel = await openBookmarks(user);
      await within(panel).findByRole("button", { name: "Jump to 1:23 test" });
      await user.keyboard("{Escape}");
      expect(screen.queryByRole("dialog", { name: "Bookmarks" })).toBeNull();

      // Another browser or device adds one meanwhile.
      addServerBookmark(600);
      panel = await openBookmarks(user);
      expect(
        await within(panel).findByRole("button", { name: "Jump to 10:00" }),
      ).toBeInTheDocument();
    });

    it("says when there are none", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      const panel = await openBookmarks(user);
      expect(
        await within(panel).findByText(/no bookmarks yet/i),
      ).toBeInTheDocument();
    });

    it("adds a Bookmark at the current time with a note", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      const audio = getAudio();
      audio.currentTime = 83;
      fireEvent.timeUpdate(audio);

      await user.click(screen.getByRole("button", { name: "Add Bookmark" }));
      const dialog = await screen.findByRole("dialog", {
        name: "Add Bookmark",
      });
      expect(dialog).toHaveTextContent("Mark 1:23 to come back to.");
      // Playback moving on while the note is written doesn't move the mark.
      audio.currentTime = 90;
      fireEvent.timeUpdate(audio);
      await user.type(within(dialog).getByLabelText("Note"), "test");
      await user.click(
        within(dialog).getByRole("button", { name: "Add Bookmark" }),
      );

      expect(bookmarkServer.items).toMatchObject([
        { seconds: 83, note: "test" },
      ]);
      expect(screen.queryByRole("dialog", { name: "Add Bookmark" })).toBeNull();
      const panel = await openBookmarks(user);
      expect(
        await within(panel).findByRole("button", { name: "Jump to 1:23 test" }),
      ).toBeInTheDocument();
    });

    it("adds a Bookmark with no note", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      await user.click(screen.getByRole("button", { name: "Add Bookmark" }));
      const dialog = await screen.findByRole("dialog", {
        name: "Add Bookmark",
      });
      await user.click(
        within(dialog).getByRole("button", { name: "Add Bookmark" }),
      );
      expect(bookmarkServer.items).toMatchObject([{ seconds: 0, note: null }]);
    });

    it("keeps the draft and reports it when adding fails", async () => {
      const error = vi.spyOn(toast, "error");
      bookmarkServer.failWrites = true;
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      await user.click(screen.getByRole("button", { name: "Add Bookmark" }));
      const dialog = await screen.findByRole("dialog", {
        name: "Add Bookmark",
      });
      await user.type(within(dialog).getByLabelText("Note"), "keep me");
      await user.click(
        within(dialog).getByRole("button", { name: "Add Bookmark" }),
      );

      expect(error).toHaveBeenCalledWith("The server refused.", undefined);
      expect(within(dialog).getByLabelText("Note")).toHaveValue("keep me");
    });

    it("jumps to a Bookmark and saves the place", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      addServerBookmark(83, "test");
      renderReader();
      fireEvent.loadedMetadata(getAudio());
      const panel = await openBookmarks(user);
      await user.click(
        await within(panel).findByRole("button", { name: /jump to 1:23/i }),
      );
      act(() => vi.advanceTimersByTime(0));

      expect(getAudio().currentTime).toBe(83);
      expect(position.saves[position.saves.length - 1]).toMatchObject({
        seconds: 83,
      });
      expect(screen.getByText("1:23")).toBeInTheDocument();
      expect(screen.queryByRole("dialog", { name: "Bookmarks" })).toBeNull();
    });

    it("edits a Bookmark's note", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      addServerBookmark(83, "test");
      renderReader();
      const panel = await openBookmarks(user);
      await user.click(
        await within(panel).findByRole("button", {
          name: "Edit note for bookmark at 1:23",
        }),
      );
      const dialog = await screen.findByRole("dialog", {
        name: "Edit Bookmark Note",
      });
      const note = within(dialog).getByLabelText("Note");
      expect(note).toHaveValue("test");
      await user.clear(note);
      await user.type(note, "the duel");
      await user.click(within(dialog).getByRole("button", { name: "Save" }));

      expect(bookmarkServer.items[0].note).toBe("the duel");
      expect(
        await within(panel).findByRole("button", {
          name: "Jump to 1:23 the duel",
        }),
      ).toBeInTheDocument();
    });

    it("returns focus to the edit button after saving a note", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      addServerBookmark(83, "test");
      renderReader();
      const panel = await openBookmarks(user);
      const edit = await within(panel).findByRole("button", {
        name: "Edit note for bookmark at 1:23",
      });
      await user.click(edit);
      const dialog = await screen.findByRole("dialog", {
        name: "Edit Bookmark Note",
      });
      await user.type(within(dialog).getByLabelText("Note"), "!");
      await user.click(within(dialog).getByRole("button", { name: "Save" }));

      expect(
        screen.queryByRole("dialog", { name: "Edit Bookmark Note" }),
      ).toBeNull();
      expect(edit).toHaveFocus();
      fireEvent.keyDown(edit, { key: " " });
      expect(playSpy).not.toHaveBeenCalled();
    });

    it("deletes a Bookmark", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      addServerBookmark(83, "test");
      addServerBookmark(600);
      renderReader();
      const panel = await openBookmarks(user);
      await user.click(
        await within(panel).findByRole("button", {
          name: "Delete bookmark at 1:23",
        }),
      );

      expect(bookmarkServer.items.map((b) => b.seconds)).toEqual([600]);
      expect(
        await within(panel).findByRole("button", { name: "Jump to 10:00" }),
      ).toBeInTheDocument();
      expect(
        within(panel).queryByRole("button", { name: /jump to 1:23/i }),
      ).toBeNull();
    });

    it("leaves Space to a dialog's buttons instead of toggling playback", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      await user.click(screen.getByRole("button", { name: "Add Bookmark" }));
      const dialog = await screen.findByRole("dialog", {
        name: "Add Bookmark",
      });
      const cancel = within(dialog).getByRole("button", { name: "Cancel" });
      const ev = new KeyboardEvent("keydown", {
        key: " ",
        bubbles: true,
        cancelable: true,
      });
      cancel.dispatchEvent(ev);
      expect(playSpy).not.toHaveBeenCalled();
      expect(ev.defaultPrevented).toBe(false);
    });
  });

  describe("chapter navigation", () => {
    it("renders a chapter dropdown that jumps to the selected chapter's start", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      await user.click(screen.getByRole("combobox", { name: /chapter/i }));
      await user.click(
        await screen.findByRole("option", { name: /chapter three/i }),
      );
      // Chapter Three starts at 2400000ms => 2400s.
      expect(audio.currentTime).toBe(2400);
    });

    it("shows the current chapter title and updates it as playback crosses a boundary", () => {
      renderReader({ file: fileWithChapters });
      const audio = getAudio();
      const combobox = screen.getByRole("combobox", { name: /chapter/i });

      // Start of the file: first chapter is the displayed/selected chapter.
      expect(combobox).toHaveTextContent("Chapter One");

      // Advance past the second chapter boundary (1200s).
      audio.currentTime = 1300;
      fireEvent.timeUpdate(audio);
      expect(combobox).toHaveTextContent("Chapter Two");
      expect(combobox).not.toHaveTextContent("Chapter One");
    });

    it("renders chapter markers along the seek bar at the correct positions", () => {
      const { container } = renderReader({ file: fileWithChapters });
      // The first chapter starts at 0 (no marker); the two later chapters get
      // markers positioned by start/duration.
      const markers = within(container).queryAllByTestId("chapter-marker");
      const lefts = Array.from(markers).map(
        (m) => (m as HTMLElement).style.left,
      );
      // 1200/3600 => 33.33%, 2400/3600 => 66.66%
      expect(lefts).toHaveLength(2);
      expect(lefts[0]).toMatch(/^33\.33/);
      expect(lefts[1]).toMatch(/^66\.66/);
    });

    it("goes to the chapter's start on a click on its mark", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const { container } = renderReader({ file: fileWithChapters });
      const audio = getAudio();

      await user.click(within(container).getAllByTestId("chapter-marker")[1]);

      expect(audio.currentTime).toBe(2400);
    });

    it("advances to the next chapter with the next button", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      await user.click(screen.getByRole("button", { name: /next chapter/i }));
      // From 0 (chapter one), next chapter starts at 1200s.
      expect(audio.currentTime).toBe(1200);
    });

    it("restarts the current chapter with previous when more than ~5s in", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      // Move to 1300s (well into chapter two, which starts at 1200s).
      audio.currentTime = 1300;
      fireEvent.timeUpdate(audio);
      await user.click(
        screen.getByRole("button", { name: /previous chapter/i }),
      );
      // More than 5s in => restart current chapter (1200s).
      expect(audio.currentTime).toBe(1200);
    });

    it("goes to the prior chapter with previous when within ~5s of the start", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      // Move to 1203s (just 3s into chapter two).
      audio.currentTime = 1203;
      fireEvent.timeUpdate(audio);
      await user.click(
        screen.getByRole("button", { name: /previous chapter/i }),
      );
      // Within 5s => go to prior chapter (chapter one, start 0).
      expect(audio.currentTime).toBe(0);
    });

    it("skips forward and back by 30 seconds with the skip buttons", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      audio.currentTime = 100;
      fireEvent.timeUpdate(audio);

      await user.click(
        screen.getByRole("button", { name: /skip forward 30 seconds/i }),
      );
      expect(audio.currentTime).toBe(130);

      await user.click(
        screen.getByRole("button", { name: /skip back 30 seconds/i }),
      );
      expect(audio.currentTime).toBe(100);
    });

    it("skips with the left and right arrow keys", () => {
      renderReader({ file: fileWithChapters });
      const audio = getAudio();
      audio.currentTime = 100;
      fireEvent.timeUpdate(audio);

      const right = new KeyboardEvent("keydown", {
        key: "ArrowRight",
        bubbles: true,
        cancelable: true,
      });
      const rightPrevented = !document.dispatchEvent(right);
      expect(audio.currentTime).toBe(130);
      expect(rightPrevented).toBe(true);

      const left = new KeyboardEvent("keydown", {
        key: "ArrowLeft",
        bubbles: true,
        cancelable: true,
      });
      const leftPrevented = !document.dispatchEvent(left);
      expect(audio.currentTime).toBe(100);
      expect(leftPrevented).toBe(true);
    });

    it("clamps arrow-key skips to the start and end of the file", () => {
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      // Near the start: skip back clamps to 0.
      audio.currentTime = 10;
      fireEvent.timeUpdate(audio);
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowLeft", bubbles: true }),
      );
      expect(audio.currentTime).toBe(0);

      // Near the end: skip forward clamps to the duration (3600s).
      audio.currentTime = 3590;
      fireEvent.timeUpdate(audio);
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }),
      );
      expect(audio.currentTime).toBe(3600);
    });

    it("renders the player with chapter navigation absent for a file with no chapters", () => {
      renderReader({ file });
      // No chapter controls when there are no chapters.
      expect(
        screen.queryByRole("combobox", { name: /chapter/i }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /next chapter/i }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /previous chapter/i }),
      ).not.toBeInTheDocument();
      // The player itself still works.
      expect(screen.getByRole("button", { name: /play/i })).toBeInTheDocument();
      // Plain skip controls remain available even without chapters.
      expect(
        screen.getByRole("button", { name: /skip forward 30 seconds/i }),
      ).toBeInTheDocument();
    });
  });

  describe("codec compatibility", () => {
    const fileWithCodec = (codec: string | undefined): File =>
      ({ ...file, audiobook_codec: codec }) as unknown as File;

    it("shows a message recommending Safari when the stored codec is xHE-AAC and the browser cannot play it", () => {
      setUserAgent(CHROME_UA);
      renderReader({ file: fileWithCodec("xHE-AAC") });
      const alert = screen.getByRole("alert");
      expect(alert).toHaveTextContent(/xHE-AAC/);
      expect(alert).toHaveTextContent(/Safari/);
    });

    it("shows a message recommending Safari when the stored codec is ALAC and the browser cannot play it", () => {
      setUserAgent(CHROME_UA);
      renderReader({ file: fileWithCodec("ALAC") });
      const alert = screen.getByRole("alert");
      expect(alert).toHaveTextContent(/ALAC/);
      expect(alert).toHaveTextContent(/Safari/);
    });

    it("shows no message for supported codecs", () => {
      setUserAgent(CHROME_UA);
      for (const codec of ["AAC-LC", "HE-AAC"]) {
        const { unmount } = renderReader({ file: fileWithCodec(codec) });
        expect(screen.queryByRole("alert")).not.toBeInTheDocument();
        unmount();
      }
    });

    it("shows no message when the file has no stored codec", () => {
      setUserAgent(CHROME_UA);
      renderReader({ file: fileWithCodec(undefined) });
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("shows no message for xHE-AAC in Safari, which can play it", () => {
      setUserAgent(SAFARI_UA);
      renderReader({ file: fileWithCodec("xHE-AAC") });
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("leaves the player controls usable alongside the codec message", () => {
      setUserAgent(CHROME_UA);
      renderReader({ file: fileWithCodec("xHE-AAC") });
      expect(screen.getByRole("button", { name: /play/i })).toBeInTheDocument();
    });
  });

  describe("runtime playback failure backstop", () => {
    it("shows the failure message when the audio element errors, even for a codec believed supported", () => {
      setUserAgent(CHROME_UA);
      renderReader({
        file: { ...file, audiobook_codec: "AAC-LC" } as unknown as File,
      });
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();

      // jsdom leaves audio.error as null; the handler treats an error event
      // without an attached MediaError as a failure.
      fireEvent.error(getAudio());

      const alert = screen.getByRole("alert");
      expect(alert).toHaveTextContent(/AAC-LC/);
      expect(alert).toHaveTextContent(/Safari/);
    });

    it("ignores an aborted media error (user-initiated, not a codec problem)", () => {
      renderReader();
      const audio = getAudio();
      Object.defineProperty(audio, "error", {
        value: { code: 1 }, // MEDIA_ERR_ABORTED
        configurable: true,
      });
      fireEvent.error(audio);
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("shows the failure message when the stream stalls before any data is decodable", () => {
      renderReader();
      const audio = getAudio();
      // jsdom's readyState defaults to 0 (HAVE_NOTHING); the xHE-AAC stuck
      // signature is a stall at readyState <= HAVE_METADATA.
      fireEvent.stalled(audio);
      expect(screen.getByRole("alert")).toHaveTextContent(/Safari/);
    });

    it("does not treat a stall after data became playable as a codec failure", () => {
      renderReader();
      const audio = getAudio();
      Object.defineProperty(audio, "readyState", {
        value: 3, // HAVE_FUTURE_DATA: ordinary buffering hiccup
        configurable: true,
      });
      fireEvent.stalled(audio);
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("clears the failure message when the stream subsequently becomes playable", () => {
      renderReader();
      const audio = getAudio();
      // A transient stall on a slow first load looks like the stuck signature
      // (readyState is still HAVE_NOTHING while bytes trickle in), so the
      // message appears...
      fireEvent.stalled(audio);
      expect(screen.getByRole("alert")).toBeInTheDocument();

      // ...but once enough data arrives that the element reports it can play,
      // the failure was a false alarm and the message must go away.
      fireEvent.canPlay(audio);
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });
  });

  describe("seek timeout guard", () => {
    const markSeekingForever = (audio: HTMLAudioElement) => {
      Object.defineProperty(audio, "seeking", {
        value: true,
        configurable: true,
      });
    };

    it("pauses and shows the failure message when a seek never completes", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      const audio = getAudio();
      markSeekingForever(audio);

      await user.click(
        screen.getByRole("button", { name: /skip forward 30 seconds/i }),
      );
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();

      act(() => {
        vi.advanceTimersByTime(SEEK_TIMEOUT_MS);
      });

      expect(pauseSpy).toHaveBeenCalled();
      expect(screen.getByRole("alert")).toHaveTextContent(/Safari/);
    });

    it("does not flag a seek that completes normally", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      // jsdom's audio.seeking is false: the seek "completed" by the time the
      // guard fires, so nothing should happen.
      await user.click(
        screen.getByRole("button", { name: /skip forward 30 seconds/i }),
      );

      act(() => {
        vi.advanceTimersByTime(SEEK_TIMEOUT_MS);
      });

      expect(pauseSpy).not.toHaveBeenCalled();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("cancels the pending guard when the seeked event fires", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();
      const audio = getAudio();
      // Even though the element still reports seeking (stale mock), a seeked
      // event means the seek completed and the guard must be cancelled.
      markSeekingForever(audio);

      await user.click(
        screen.getByRole("button", { name: /skip forward 30 seconds/i }),
      );
      fireEvent.seeked(audio);

      act(() => {
        vi.advanceTimersByTime(SEEK_TIMEOUT_MS);
      });

      expect(pauseSpy).not.toHaveBeenCalled();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });
  });
});

describe("M4BReader autoplay", () => {
  // The element has loaded enough to play, as canplay reports.
  const canPlay = (audio: HTMLAudioElement) => {
    Object.defineProperty(audio, "readyState", {
      configurable: true,
      value: 3,
    });
    fireEvent.canPlay(audio);
  };

  it("starts playing once it can, with no press", () => {
    renderReader();
    const audio = getAudio();
    expect(playSpy).not.toHaveBeenCalled();

    fireEvent.loadedMetadata(audio);
    expect(playSpy).not.toHaveBeenCalled();

    canPlay(audio);
    expect(playSpy).toHaveBeenCalledTimes(1);

    canPlay(audio);
    expect(playSpy).toHaveBeenCalledTimes(1);
  });

  it("waits for the seek to the saved place, so it doesn't play from the start", () => {
    position.savedSeconds = 125;
    renderReader();
    const audio = getAudio();
    Object.defineProperty(audio, "seeking", {
      configurable: true,
      value: true,
    });
    fireEvent.loadedMetadata(audio);
    expect(audio.currentTime).toBe(125);

    canPlay(audio);
    expect(playSpy).not.toHaveBeenCalled();

    Object.defineProperty(audio, "seeking", {
      configurable: true,
      value: false,
    });
    fireEvent.seeked(audio);
    expect(playSpy).toHaveBeenCalledTimes(1);
  });

  it("waits for the saved position to load", () => {
    position.loading = true;
    position.savedSeconds = 125;
    const client = new QueryClient();
    const tree = () => (
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <M4BReader book={book} file={file} libraryId="1" />
        </MemoryRouter>
      </QueryClientProvider>
    );
    const { rerender } = render(tree());
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);
    canPlay(audio);
    expect(playSpy).not.toHaveBeenCalled();

    position.loading = false;
    rerender(tree());
    expect(audio.currentTime).toBe(125);
    expect(playSpy).toHaveBeenCalledTimes(1);
  });

  it("does not play a stream the browser can't", () => {
    setUserAgent(CHROME_UA);
    renderReader({ file: { ...file, audiobook_codec: "xHE-AAC" } as File });
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);
    canPlay(audio);

    expect(playSpy).not.toHaveBeenCalled();
  });

  it("leaves a press before it can play alone", () => {
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);

    fireEvent.click(screen.getByRole("button", { name: "Play" }));
    expect(playSpy).toHaveBeenCalledTimes(1);

    canPlay(audio);
    expect(playSpy).toHaveBeenCalledTimes(1);
  });

  // Playing from the very end restarts the book, which reads as going back and
  // would undo a finish.
  it("does not play from the very end of the file", () => {
    position.savedSeconds = 3600;
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);
    canPlay(audio);

    expect(playSpy).not.toHaveBeenCalled();
  });

  it("does not play from the end when the element's own length is shorter", () => {
    position.savedSeconds = 3590;
    renderReader();
    const audio = getAudio();
    Object.defineProperty(audio, "duration", {
      configurable: true,
      value: 3590,
    });
    fireEvent.loadedMetadata(audio);
    canPlay(audio);

    expect(playSpy).not.toHaveBeenCalled();
  });

  it("does not restart a file the listener already took to its end", () => {
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);
    Object.defineProperty(audio, "ended", { configurable: true, value: true });
    canPlay(audio);

    expect(playSpy).not.toHaveBeenCalled();
  });

  it("calls it off when the seek to the saved place gives up", () => {
    position.savedSeconds = 125;
    renderReader();
    const audio = getAudio();
    Object.defineProperty(audio, "seeking", {
      configurable: true,
      value: true,
    });
    fireEvent.loadedMetadata(audio);
    act(() => vi.advanceTimersByTime(RESTORE_SEEK_TIMEOUT_MS));
    expect(screen.getByRole("alert")).toBeInTheDocument();

    Object.defineProperty(audio, "seeking", {
      configurable: true,
      value: false,
    });
    fireEvent.seeked(audio);
    canPlay(audio);

    expect(playSpy).not.toHaveBeenCalled();
  });

  it("does not play from the start when the saved position fails to load", () => {
    position.loadFails = true;
    position.savedSeconds = 125;
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);
    canPlay(audio);

    expect(playSpy).not.toHaveBeenCalled();
  });

  it("does not start out of nowhere if it takes too long to load", () => {
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);

    act(() => vi.advanceTimersByTime(AUTOPLAY_TIMEOUT_MS));
    canPlay(audio);

    expect(playSpy).not.toHaveBeenCalled();
  });

  it("stays paused when the browser blocks it", async () => {
    playSpy.mockRejectedValue(new DOMException("blocked", "NotAllowedError"));
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);
    canPlay(audio);
    await act(async () => {});

    expect(playSpy).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("M4BReader position", () => {
  const SAVE_EVERY_MS = 15_000;

  // Plays on to the given second, as the element reports it.
  const playTo = (seconds: number) => {
    const audio = getAudio();
    Object.defineProperty(audio, "paused", {
      configurable: true,
      value: false,
    });
    audio.currentTime = seconds;
    fireEvent.timeUpdate(audio);
  };
  const startPlaying = () => {
    const audio = getAudio();
    Object.defineProperty(audio, "paused", {
      configurable: true,
      value: false,
    });
    fireEvent.play(audio);
  };
  const pausePlaying = () => {
    const audio = getAudio();
    Object.defineProperty(audio, "paused", {
      configurable: true,
      value: true,
    });
    fireEvent.pause(audio);
  };

  it("seeks to the saved second once metadata loads, saving nothing", () => {
    position.savedSeconds = 125;
    renderReader();
    const audio = getAudio();

    expect(audio.currentTime).toBe(0);
    fireEvent.loadedMetadata(audio);

    expect(audio.currentTime).toBe(125);
    expect(screen.getByText("2:05")).toBeInTheDocument();
    act(() => vi.advanceTimersByTime(SAVE_EVERY_MS));
    expect(position.saves).toEqual([]);
  });

  it("waits for the saved position when metadata loads first", () => {
    position.savedSeconds = 125;
    position.loading = true;
    const client = new QueryClient();
    const tree = () => (
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <M4BReader book={book} file={file} libraryId="1" />
        </MemoryRouter>
      </QueryClientProvider>
    );
    const { rerender } = render(tree());
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);
    expect(audio.currentTime).toBe(0);

    position.loading = false;
    rerender(tree());
    expect(audio.currentTime).toBe(125);
  });

  it("seeks a saved place with no seconds to its percentage of the book", () => {
    // The file is an hour long.
    position.savedPercentageOnly = 0.25;
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);

    expect(audio.currentTime).toBe(900);
    act(() => vi.advanceTimersByTime(SAVE_EVERY_MS));
    expect(position.saves).toEqual([]);
  });

  it("stays at the start with nothing saved", () => {
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);

    expect(audio.currentTime).toBe(0);
    expect(position.saves).toEqual([]);
  });

  it("saves every 15 seconds while playing", () => {
    renderReader();
    fireEvent.loadedMetadata(getAudio());
    startPlaying();

    playTo(10);
    act(() => vi.advanceTimersByTime(SAVE_EVERY_MS - 1));
    playTo(14);
    expect(position.saves).toEqual([]);

    act(() => vi.advanceTimersByTime(1));
    expect(position.saves).toHaveLength(1);
    expect(position.saves[0]).toMatchObject({
      seconds: 14,
      percentage: 14 / 3600,
    });
  });

  it("does not take a timeupdate from restoring for a move", () => {
    position.savedSeconds = 125;
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);

    audio.currentTime = 124.998;
    fireEvent.timeUpdate(audio);
    act(() => vi.advanceTimersByTime(SAVE_EVERY_MS));

    expect(position.saves).toEqual([]);
  });

  it("reports how long the open session has played with each save", () => {
    renderReader();
    fireEvent.loadedMetadata(getAudio());
    startPlaying();

    playTo(10);
    act(() => vi.advanceTimersByTime(SAVE_EVERY_MS));

    expect(position.saves[0]).toMatchObject({ listened_seconds: 15 });
    expect(position.saves[0].listening_since).toEqual(expect.any(String));
  });

  it("saves on pause, with the session", () => {
    renderReader();
    fireEvent.loadedMetadata(getAudio());
    startPlaying();
    act(() => vi.advanceTimersByTime(5000));
    playTo(5);
    pausePlaying();

    expect(position.saves).toHaveLength(1);
    expect(position.saves[0]).toMatchObject({
      seconds: 5,
      session: { end_percentage: 5 / 3600 },
    });
  });

  it("saves at once on a seek", () => {
    renderReader();
    fireEvent.loadedMetadata(getAudio());

    fireEvent.click(
      screen.getByRole("button", { name: "Skip forward 30 seconds" }),
    );

    expect(position.saves).toHaveLength(1);
    expect(position.saves[0]).toMatchObject({
      seconds: 30,
      percentage: 30 / 3600,
    });
  });

  it("reports the end when playback ends", () => {
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);
    startPlaying();
    playTo(3600);
    Object.defineProperty(audio, "ended", { configurable: true, value: true });
    pausePlaying();
    fireEvent.ended(audio);

    expect(position.saves[0]).toMatchObject({
      seconds: 3600,
      percentage: 1,
      at_end: true,
    });
  });

  it("counts a session that plays in a hidden tab", () => {
    let visibility: DocumentVisibilityState = "visible";
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => visibility,
    });
    try {
      renderReader();
      fireEvent.loadedMetadata(getAudio());
      startPlaying();

      visibility = "hidden";
      act(() => {
        document.dispatchEvent(new Event("visibilitychange"));
      });
      act(() => vi.advanceTimersByTime(10 * 60_000));
      playTo(600);
      pausePlaying();

      const session = position.saves.flatMap((save) =>
        save.session ? [save.session as Record<string, string>] : [],
      );
      expect(session).toHaveLength(1);
      expect(
        Date.parse(session[0].ended_at) - Date.parse(session[0].started_at),
      ).toBe(10 * 60_000);
    } finally {
      delete (document as { visibilityState?: string }).visibilityState;
    }
  });

  it("gives a seek to the saved place longer than a seek by the listener", () => {
    position.savedSeconds = 30000;
    renderReader();
    const audio = getAudio();
    Object.defineProperty(audio, "seeking", {
      configurable: true,
      value: true,
    });
    fireEvent.loadedMetadata(audio);

    act(() => vi.advanceTimersByTime(SEEK_TIMEOUT_MS));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();

    act(() => vi.advanceTimersByTime(RESTORE_SEEK_TIMEOUT_MS));
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("saves nothing while the audio can't play", () => {
    renderReader();
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);
    Object.defineProperty(audio, "seeking", {
      configurable: true,
      value: true,
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Skip forward 30 seconds" }),
    );
    expect(position.saves).toHaveLength(1);

    act(() => vi.advanceTimersByTime(SEEK_TIMEOUT_MS));
    expect(screen.getByRole("alert")).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Skip forward 30 seconds" }),
    );
    expect(position.saves).toHaveLength(1);
  });

  it("does not save a skip in a stream the browser can't play", () => {
    setUserAgent(CHROME_UA);
    renderReader({ file: { ...file, audiobook_codec: "xHE-AAC" } as File });
    fireEvent.loadedMetadata(getAudio());

    fireEvent.click(
      screen.getByRole("button", { name: "Skip forward 30 seconds" }),
    );

    expect(position.saves).toEqual([]);
  });

  it("does not seek a stream the browser can't play", () => {
    setUserAgent(CHROME_UA);
    position.savedSeconds = 125;
    renderReader({ file: { ...file, audiobook_codec: "xHE-AAC" } as File });
    const audio = getAudio();
    fireEvent.loadedMetadata(audio);

    expect(audio.currentTime).toBe(0);
  });
});
