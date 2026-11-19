import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { SEARCH_DELAY_MS, type SearchableBook } from "@/hooks/useBookSearch";
import { Overlayer } from "@/libraries/foliate/overlayer.js";

import ReaderSearch from "./ReaderSearch";

interface Excerpt {
  pre: string;
  match: string;
  post: string;
}

type SearchYield =
  | "done"
  | { progress: number }
  | {
      label: string;
      subitems: Array<{ cfi: string; excerpt: Excerpt; label?: string }>;
    };

// One call to the fake foliate search: the test pushes what foliate would
// find, and the panel reads it as it arrives.
interface SearchRun {
  query: string;
  push: (...items: SearchYield[]) => void;
  stopped: () => boolean;
}

const fakeBook = () => {
  const runs: SearchRun[] = [];
  const makeRun = (query: string) => {
    const queue: SearchYield[] = [];
    let wake: (() => void) | null = null;
    let stopped = false;
    async function* iterate(): AsyncGenerator<SearchYield> {
      try {
        for (;;) {
          while (!queue.length) {
            await new Promise<void>((resolve) => {
              wake = resolve;
            });
          }
          const item = queue.shift()!;
          yield item;
          if (item === "done") return;
        }
      } finally {
        stopped = true;
      }
    }
    const run: SearchRun = {
      query,
      push: (...items) => {
        queue.push(...items);
        wake?.();
        wake = null;
      },
      stopped: () => stopped,
    };
    runs.push(run);
    return iterate();
  };
  const view = Object.assign(document.createElement("div"), {
    search: vi.fn((opts: { query: string }) => makeRun(opts.query)),
    clearSearch: vi.fn(),
    goTo: vi.fn().mockResolvedValue(undefined),
    addAnnotation: vi.fn().mockResolvedValue(undefined),
    deleteAnnotation: vi.fn().mockResolvedValue(undefined),
  });
  return { view: view as typeof view & SearchableBook, runs };
};

const excerpt = (pre: string, match: string, post: string) => ({
  pre,
  match,
  post,
});

// A word found in two chapters, twice in the first.
const TWO_CHAPTERS: SearchYield[] = [
  { progress: 0.25 },
  {
    label: "Chapter Two",
    subitems: [
      { cfi: "cfi-1", excerpt: excerpt("A ", "lantern", " glows.") },
      { cfi: "cfi-2", excerpt: excerpt("The ", "lantern", " dims.") },
    ],
  },
  { progress: 0.75 },
  {
    label: "Chapter Four",
    subitems: [{ cfi: "cfi-3", excerpt: excerpt("No ", "lantern", ".") }],
  },
  { progress: 1 },
  "done",
];

// Moves without a history of jumps, which the reader's tests cover.
const justGo = async (go: () => Promise<unknown>) => {
  await go();
};

function Harness({ view }: { view: SearchableBook | null }) {
  const [open, setOpen] = useState(false);
  return (
    <ReaderSearch
      onJump={justGo}
      onOpenChange={setOpen}
      open={open}
      view={view}
    />
  );
}

const setup = () => {
  const book = fakeBook();
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  render(<Harness view={book.view} />);
  return { ...book, user };
};

const openSearch = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(screen.getByRole("button", { name: "Search" }));
  return screen.getByRole("searchbox", { name: "Search in book" });
};

// Lets the debounce pass, so the search starts.
const startSearch = async () => {
  await act(async () => {
    vi.advanceTimersByTime(SEARCH_DELAY_MS);
  });
};

// Lets the panel read what the fake search has found so far.
const flush = async () => {
  await act(async () => {
    for (let i = 0; i < 10; i++) await Promise.resolve();
  });
};

const status = () => screen.getByRole("status");

describe("ReaderSearch", () => {
  it("lists results in book order under their chapters, with the match marked", async () => {
    const { user, view, runs } = setup();
    await user.type(await openSearch(user), "lantern");
    await startSearch();
    expect(view.search).toHaveBeenCalledTimes(1);
    // A phrase, ignoring case and accents.
    expect(view.search).toHaveBeenCalledWith(
      expect.objectContaining({
        query: "lantern",
        matchCase: false,
        matchDiacritics: false,
        matchWholeWords: false,
      }),
    );

    runs[0].push(...TWO_CHAPTERS);
    await flush();

    const chapters = screen.getAllByRole("heading", { level: 3 });
    expect(chapters.map((h) => h.textContent)).toEqual([
      "Chapter Two",
      "Chapter Four",
    ]);
    const results = screen.getAllByRole("button", { name: /lantern/ });
    expect(results.map((r) => r.textContent)).toEqual([
      "A lantern glows.",
      "The lantern dims.",
      "No lantern.",
    ]);
    expect(within(results[0]).getByText("lantern").tagName).toBe("MARK");
    expect(status()).toHaveTextContent("3 results");
  });

  it("says it's still searching while it works through the book", async () => {
    const { user, runs } = setup();
    await user.type(await openSearch(user), "lantern");
    await startSearch();
    expect(status()).toHaveTextContent("Searching…");

    runs[0].push(TWO_CHAPTERS[0], TWO_CHAPTERS[1]);
    await flush();
    expect(status()).toHaveTextContent("2 results, still searching…");
    expect(
      screen
        .getAllByRole("button", { name: /lantern/ })
        .map((r) => r.textContent),
    ).toEqual(["A lantern glows.", "The lantern dims."]);

    runs[0].push(...TWO_CHAPTERS.slice(2));
    await flush();
    expect(status()).toHaveTextContent("3 results");
    expect(status()).not.toHaveTextContent("searching");
  });

  it("says when nothing matches", async () => {
    const { user, runs } = setup();
    await user.type(await openSearch(user), "dragon");
    await startSearch();
    runs[0].push({ progress: 1 }, "done");
    await flush();
    expect(status()).toHaveTextContent("No results");
  });

  it("keeps a chapter split across sections under one heading", async () => {
    const { user, runs } = setup();
    await user.type(await openSearch(user), "lantern");
    await startSearch();
    runs[0].push(
      {
        label: "Chapter Two",
        subitems: [{ cfi: "a", excerpt: excerpt("", "lantern", " one") }],
      },
      {
        label: "Chapter Two",
        subitems: [{ cfi: "b", excerpt: excerpt("", "lantern", " two") }],
      },
      "done",
    );
    await flush();
    expect(
      screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent),
    ).toEqual(["Chapter Two"]);
  });

  it("cancels a search in progress when the query changes", async () => {
    const { user, view, runs } = setup();
    const field = await openSearch(user);
    await user.type(field, "lamp");
    await startSearch();
    await user.clear(field);
    await user.type(field, "lantern");
    await startSearch();
    expect(view.search).toHaveBeenCalledTimes(2);

    // The first search finds something after the second started.
    runs[0].push({
      label: "Chapter One",
      subitems: [{ cfi: "old", excerpt: excerpt("A ", "lamp", ".") }],
    });
    runs[1].push(...TWO_CHAPTERS);
    await flush();

    expect(screen.queryByRole("button", { name: /lamp/ })).toBeNull();
    expect(screen.getAllByRole("button", { name: /lantern/ })).toHaveLength(3);
    expect(runs[0].stopped()).toBe(true);
  });

  it("waits for typing to pause before searching", async () => {
    const { user, view } = setup();
    await user.type(await openSearch(user), "lan");
    expect(view.search).not.toHaveBeenCalled();
    await startSearch();
    expect(view.search).toHaveBeenCalledWith(
      expect.objectContaining({ query: "lan" }),
    );
  });

  it("goes to a chosen result and marks it on the page", async () => {
    const { user, view, runs } = setup();
    await user.type(await openSearch(user), "lantern");
    await startSearch();
    runs[0].push(...TWO_CHAPTERS);
    await flush();

    await user.click(screen.getByRole("button", { name: "No lantern." }));
    await flush();
    expect(view.goTo).toHaveBeenCalledWith("cfi-3");
    expect(view.addAnnotation).toHaveBeenCalledWith({ value: "cfi-3" });
    expect(status()).toHaveTextContent("3 of 3");
    // The list gets out of the way of the page; the field stays for the next
    // result.
    expect(screen.queryByRole("button", { name: "No lantern." })).toBeNull();
    expect(
      screen.getByRole("searchbox", { name: "Search in book" }),
    ).toHaveFocus();

    // foliate asks how to draw the chosen match.
    const draw = vi.fn();
    view.dispatchEvent(
      new CustomEvent("draw-annotation", {
        detail: { draw, annotation: { value: "cfi-3" } },
      }),
    );
    expect(draw).toHaveBeenCalledWith(Overlayer.outline, expect.anything());

    // The list comes back on demand, showing the chosen result.
    await user.click(screen.getByRole("button", { name: "Results" }));
    expect(screen.getByRole("button", { name: "No lantern." })).toHaveAttribute(
      "aria-current",
      "true",
    );
  });

  it("marks the chosen result again when its section loads again", async () => {
    const { user, view, runs } = setup();
    await user.type(await openSearch(user), "lantern");
    await startSearch();
    runs[0].push(...TWO_CHAPTERS);
    await flush();
    await user.click(screen.getByRole("button", { name: "A lantern glows." }));
    await flush();
    view.addAnnotation.mockClear();

    view.dispatchEvent(new CustomEvent("create-overlay", { detail: {} }));
    expect(view.addAnnotation).toHaveBeenCalledWith({ value: "cfi-1" });
  });

  it("steps through results with the next and previous buttons", async () => {
    const { user, view, runs } = setup();
    await user.type(await openSearch(user), "lantern");
    await startSearch();
    runs[0].push(...TWO_CHAPTERS);
    await flush();

    const next = screen.getByRole("button", { name: "Next result" });
    const previous = screen.getByRole("button", { name: "Previous result" });
    await user.click(next);
    await flush();
    expect(view.goTo).toHaveBeenLastCalledWith("cfi-1");
    await user.click(next);
    await flush();
    expect(view.goTo).toHaveBeenLastCalledWith("cfi-2");
    // The last result's mark moves to the new one.
    expect(view.deleteAnnotation).toHaveBeenLastCalledWith({ value: "cfi-1" });
    expect(view.addAnnotation).toHaveBeenLastCalledWith({ value: "cfi-2" });
    expect(status()).toHaveTextContent("2 of 3");

    await user.click(previous);
    await flush();
    expect(view.goTo).toHaveBeenLastCalledWith("cfi-1");
    // Past the first, it wraps to the last.
    await user.click(previous);
    await flush();
    expect(view.goTo).toHaveBeenLastCalledWith("cfi-3");
    await user.click(next);
    await flush();
    expect(view.goTo).toHaveBeenLastCalledWith("cfi-1");
  });

  it("steps through results with Enter and Shift+Enter in the field", async () => {
    const { user, view, runs } = setup();
    const field = await openSearch(user);
    await user.type(field, "lantern");
    await startSearch();
    runs[0].push(...TWO_CHAPTERS);
    await flush();

    await user.type(field, "{Enter}");
    await flush();
    expect(view.goTo).toHaveBeenLastCalledWith("cfi-1");
    await user.type(field, "{Enter}");
    await flush();
    expect(view.goTo).toHaveBeenLastCalledWith("cfi-2");
    await user.type(field, "{Shift>}{Enter}{/Shift}");
    await flush();
    expect(view.goTo).toHaveBeenLastCalledWith("cfi-1");
    expect(field).toHaveValue("lantern");
  });

  it("goes to the first result once found when Enter comes first", async () => {
    const { user, view, runs } = setup();
    const field = await openSearch(user);
    await user.type(field, "lantern{Enter}");
    // Enter starts the search at once.
    await flush();
    expect(view.search).toHaveBeenCalledTimes(1);
    expect(view.goTo).not.toHaveBeenCalled();

    runs[0].push(...TWO_CHAPTERS);
    await flush();
    expect(view.goTo).toHaveBeenCalledTimes(1);
    expect(view.goTo).toHaveBeenCalledWith("cfi-1");
  });

  it("removes the highlights when the field is cleared", async () => {
    const { user, view, runs } = setup();
    const field = await openSearch(user);
    await user.type(field, "lantern");
    await startSearch();
    runs[0].push(...TWO_CHAPTERS);
    await flush();
    await user.type(field, "{Enter}");
    await flush();
    view.clearSearch.mockClear();

    await user.clear(field);
    await flush();
    expect(view.clearSearch).toHaveBeenCalled();
    expect(view.deleteAnnotation).toHaveBeenLastCalledWith({ value: "cfi-1" });
    expect(screen.queryAllByRole("button", { name: /lantern/ })).toHaveLength(
      0,
    );
    await startSearch();
    expect(view.search).toHaveBeenCalledTimes(1);
  });

  it("clears the field with its clear button", async () => {
    const { user, view, runs } = setup();
    const field = await openSearch(user);
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
    await user.type(field, "lantern");
    await startSearch();
    runs[0].push(...TWO_CHAPTERS);
    await flush();
    view.clearSearch.mockClear();

    await user.click(screen.getByRole("button", { name: "Clear search" }));
    await flush();
    expect(field).toHaveValue("");
    expect(field).toHaveFocus();
    expect(view.clearSearch).toHaveBeenCalled();
    expect(screen.queryAllByRole("button", { name: /lantern/ })).toHaveLength(
      0,
    );
  });

  it("removes the highlights when the panel closes", async () => {
    const { user, view, runs } = setup();
    const field = await openSearch(user);
    await user.type(field, "lantern");
    await startSearch();
    runs[0].push(...TWO_CHAPTERS);
    await flush();
    await user.type(field, "{Enter}");
    await flush();
    view.clearSearch.mockClear();

    await user.click(screen.getByRole("button", { name: "Close search" }));
    await flush();
    expect(
      screen.queryByRole("searchbox", { name: "Search in book" }),
    ).toBeNull();
    expect(view.clearSearch).toHaveBeenCalled();
    expect(view.deleteAnnotation).toHaveBeenLastCalledWith({ value: "cfi-1" });

    // It opens again empty.
    expect(await openSearch(user)).toHaveValue("");
  });

  it("stops a search in progress when the panel closes", async () => {
    const { user, runs } = setup();
    await user.type(await openSearch(user), "lantern");
    await startSearch();
    await user.keyboard("{Escape}");
    runs[0].push(TWO_CHAPTERS[1]);
    await flush();
    expect(runs[0].stopped()).toBe(true);
  });

  it("can't open before the book does", () => {
    render(<Harness view={null} />);
    expect(screen.getByRole("button", { name: "Search" })).toBeDisabled();
  });

  it("shows a long list a page at a time", async () => {
    const { user, runs } = setup();
    await user.type(await openSearch(user), "an");
    await startSearch();
    runs[0].push(
      {
        label: "Chapter One",
        subitems: Array.from({ length: 250 }, (_, i) => ({
          cfi: `cfi-${i}`,
          excerpt: excerpt(`${i} `, "an", ""),
        })),
      },
      "done",
    );
    await flush();
    expect(status()).toHaveTextContent("250 results");
    expect(screen.getAllByRole("listitem")).toHaveLength(100);
    await user.click(screen.getByRole("button", { name: "Show more results" }));
    expect(screen.getAllByRole("listitem")).toHaveLength(200);
  });

  it("files each match under the chapter it's in, in a section holding several", async () => {
    const { user, runs } = setup();
    await user.type(await openSearch(user), "lantern");
    await startSearch();
    runs[0].push(
      {
        label: "Chapter One",
        subitems: [
          {
            cfi: "a",
            excerpt: excerpt("", "lantern", " one"),
            label: "Chapter One",
          },
          {
            cfi: "b",
            excerpt: excerpt("", "lantern", " two"),
            label: "Chapter Two",
          },
        ],
      },
      "done",
    );
    await flush();
    expect(
      screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent),
    ).toEqual(["Chapter One", "Chapter Two"]);
  });

  it("goes only to the last of several results chosen in quick succession", async () => {
    const { user, view, runs } = setup();
    // foliate doesn't queue moves: a move within the shown section can land
    // before an earlier one that loads another section.
    const moves: Array<() => void> = [];
    view.goTo.mockImplementation(
      () => new Promise<void>((resolve) => moves.push(resolve)),
    );
    await user.type(await openSearch(user), "lantern");
    await startSearch();
    runs[0].push(...TWO_CHAPTERS);
    await flush();

    const next = screen.getByRole("button", { name: "Next result" });
    await user.click(next);
    await user.click(next);
    await user.click(next);
    await flush();
    expect(status()).toHaveTextContent("3 of 3");
    expect(view.goTo).toHaveBeenCalledTimes(1);
    expect(view.goTo).toHaveBeenLastCalledWith("cfi-1");

    await act(async () => moves[0]());
    await flush();
    expect(view.goTo).toHaveBeenCalledTimes(2);
    expect(view.goTo).toHaveBeenLastCalledWith("cfi-3");
    await act(async () => moves[1]());
    await flush();
    expect(view.addAnnotation).toHaveBeenCalledTimes(1);
    expect(view.addAnnotation).toHaveBeenCalledWith({ value: "cfi-3" });
  });

  it("waits for Enter to search for a single character", async () => {
    const { user, view, runs } = setup();
    const field = await openSearch(user);
    await user.type(field, "a");
    await startSearch();
    expect(view.search).not.toHaveBeenCalled();
    expect(status()).toHaveTextContent("Press Enter to search");

    await user.type(field, "{Enter}");
    await flush();
    expect(view.search).toHaveBeenCalledWith(
      expect.objectContaining({ query: "a" }),
    );
    runs[0].push(...TWO_CHAPTERS);
    await flush();
    expect(view.goTo).toHaveBeenCalledWith("cfi-1");
  });

  it("leaves Enter to an input method composing text", async () => {
    const { user, view, runs } = setup();
    const field = await openSearch(user);
    await user.type(field, "lantern");
    await startSearch();
    runs[0].push(...TWO_CHAPTERS);
    await flush();

    fireEvent.keyDown(field, { key: "Enter", isComposing: true });
    fireEvent.keyDown(field, { key: "Enter", keyCode: 229 });
    await flush();
    expect(view.goTo).not.toHaveBeenCalled();
  });

  it("still says it's searching once a result is chosen", async () => {
    const { user, runs } = setup();
    await user.type(await openSearch(user), "lantern");
    await startSearch();
    runs[0].push(TWO_CHAPTERS[1]);
    await flush();
    await user.click(screen.getByRole("button", { name: "A lantern glows." }));
    await flush();
    expect(status()).toHaveTextContent("1 of 2, still searching…");
  });
});
