import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Overlayer } from "@/libraries/foliate/overlayer.js";
import type { FoliateSearchExcerpt } from "@/types/foliate";

// How long typing pauses before the search starts. Each search reads the
// whole book, so one per keystroke would mostly be thrown away.
export const SEARCH_DELAY_MS = 300;

// A phrase anywhere in the text, ignoring case and accents, matching the
// Shisho app's search. foliate's own default would also be this, but its
// options are spelled out so a change upstream can't change what readers get.
export const SEARCH_OPTIONS = {
  matchCase: false,
  matchDiacritics: false,
  matchWholeWords: false,
} as const;

// Every match is shaded; the chosen one is also outlined, so it stands out
// on a page with several. Fixed colors, since they sit over the book's own
// colors in every reader theme.
const MATCH_COLOR = "#facc15";
const CHOSEN_COLOR = "#f97316";

// The parts of foliate's <foliate-view> that search uses.
export type SearchableBook = Pick<
  HTMLElementTagNameMap["foliate-view"],
  | "search"
  | "clearSearch"
  | "goTo"
  | "addAnnotation"
  | "deleteAnnotation"
  | "addEventListener"
  | "removeEventListener"
>;

export interface SearchResult {
  cfi: string;
  excerpt: FoliateSearchExcerpt;
  chapter: string;
}

// Results under one chapter heading, with each result's place in the list.
export interface SearchGroup {
  chapter: string;
  results: Array<{ result: SearchResult; index: number }>;
}

// Neighbouring results in one chapter share a heading. A chapter split
// across several sections, as converted books often are, comes in pieces.
const groupByChapter = (results: SearchResult[]): SearchGroup[] => {
  const groups: SearchGroup[] = [];
  results.forEach((result, index) => {
    const last = groups[groups.length - 1];
    if (last && last.chapter === result.chapter) {
      last.results.push({ result, index });
    } else {
      groups.push({ chapter: result.chapter, results: [{ result, index }] });
    }
  });
  return groups;
};

// A query this short matches nearly everywhere, so it searches only when
// asked with Enter: the search and its highlights would cost the most for the
// least. One character is still a word in Chinese or Japanese.
const searchesAsTyped = (query: string) => Array.from(query).length > 1;

const warn = (what: string) => (err: unknown) =>
  console.warn(`useBookSearch: ${what}`, err);

/**
 * Search inside a Book open in foliate. The query searches once typing
 * pauses, and a new one cancels the last. Matches are highlighted on the
 * page, and the chosen one is outlined. Unmounting clears every highlight.
 */
export function useBookSearch(view: SearchableBook | null) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  // A query that searches only on Enter is waiting for it.
  const [waiting, setWaiting] = useState(false);
  const [current, setCurrent] = useState<number | null>(null);

  // Bumped by each new search, which leaves the last one's results unread.
  const runRef = useRef(0);
  // The query typed, trimmed, and whether a search for it has started.
  const queryRef = useRef("");
  const startedRef = useRef(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // The chosen result, outlined on the page.
  const chosenRef = useRef<string | null>(null);
  // Enter came before any result did, so the first one found is chosen.
  const goToFirstRef = useRef(false);
  // The move to the last chosen result. foliate doesn't queue moves, and one
  // within the shown section can land before an earlier one still loading
  // another, so each waits for the last.
  const movingRef = useRef<Promise<unknown>>(Promise.resolve());

  const unmarkChosen = useCallback(() => {
    const chosen = chosenRef.current;
    chosenRef.current = null;
    return chosen && view
      ? view.deleteAnnotation({ value: chosen })
      : Promise.resolve();
  }, [view]);

  const goTo = useCallback(
    (index: number, cfi: string) => {
      if (!view) return;
      unmarkChosen().catch(warn("unmark failed"));
      chosenRef.current = cfi;
      setCurrent(index);
      // Only the latest choice moves once the last move lands, so quick
      // steps end on the result the panel shows.
      movingRef.current = movingRef.current
        .then(async () => {
          if (chosenRef.current !== cfi) return;
          // A move, like following a link: the reader saves it as a Reading
          // Position when foliate reports it.
          await view.goTo(cfi);
          if (chosenRef.current === cfi) {
            await view.addAnnotation({ value: cfi });
          }
        })
        .catch(warn("go to result failed"));
    },
    [view, unmarkChosen],
  );

  const run = useCallback(
    async (trimmed: string) => {
      if (!view) return;
      const id = ++runRef.current;
      const stale = () => id !== runRef.current;
      startedRef.current = true;
      setWaiting(false);
      setSearching(true);
      try {
        const found = view.search({
          ...SEARCH_OPTIONS,
          query: trimmed,
          draw: Overlayer.highlight,
          drawOptions: { color: MATCH_COLOR },
        });
        let count = 0;
        // Leaving the loop stops foliate's search, which reads no further.
        for await (const item of found) {
          if (stale()) return;
          if (item === "done") break;
          if (!("subitems" in item)) continue;
          const added = item.subitems.map(({ cfi, excerpt, label }) => ({
            cfi,
            excerpt,
            chapter: label ?? item.label,
          }));
          setResults((prev) => [...prev, ...added]);
          if (count === 0 && added.length && goToFirstRef.current) {
            goToFirstRef.current = false;
            goTo(0, added[0].cfi);
          }
          count += added.length;
        }
      } catch (err) {
        warn("search failed")(err);
      } finally {
        if (!stale()) {
          goToFirstRef.current = false;
          setSearching(false);
        }
      }
    },
    [view, goTo],
  );

  const cancel = useCallback(() => {
    runRef.current++;
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = null;
    goToFirstRef.current = false;
  }, []);

  const changeQuery = useCallback(
    (next: string) => {
      setQuery(next);
      const trimmed = next.trim();
      // Spaces typed between words don't search again.
      if (trimmed === queryRef.current) return;
      queryRef.current = trimmed;
      startedRef.current = false;
      cancel();
      unmarkChosen().catch(warn("unmark failed"));
      setCurrent(null);
      setResults([]);
      setSearching(false);
      setWaiting(false);
      if (!trimmed || !view) {
        view?.clearSearch();
        return;
      }
      if (!searchesAsTyped(trimmed)) {
        view.clearSearch();
        setWaiting(true);
        return;
      }
      setSearching(true);
      timerRef.current = setTimeout(() => {
        timerRef.current = null;
        void run(trimmed);
      }, SEARCH_DELAY_MS);
    },
    [view, cancel, unmarkChosen, run],
  );

  // Next or previous from the chosen result, wrapping at the ends. With none
  // chosen yet, next is the first and previous the last.
  const step = useCallback(
    (direction: 1 | -1) => {
      if (queryRef.current && !startedRef.current) {
        // Typing hasn't paused yet, or the query waits for Enter: search
        // now, and go to the first result found.
        cancel();
        void run(queryRef.current);
        if (direction === 1) goToFirstRef.current = true;
        return;
      }
      if (!results.length) {
        if (searching && direction === 1) goToFirstRef.current = true;
        return;
      }
      const index =
        current === null
          ? direction === 1
            ? 0
            : results.length - 1
          : (current + direction + results.length) % results.length;
      goTo(index, results[index].cfi);
    },
    [results, current, searching, cancel, run, goTo],
  );

  // foliate draws an annotation when asked to add it and again whenever its
  // section loads, which it announces with "create-overlay".
  useEffect(() => {
    if (!view) return;
    const handleDraw = (evt: Event) => {
      const { draw, annotation } = (
        evt as CustomEvent<{
          draw: (func: unknown, opts: Record<string, unknown>) => void;
          annotation: { value: string };
        }>
      ).detail;
      if (annotation.value !== chosenRef.current) return;
      draw(Overlayer.outline, { color: CHOSEN_COLOR, width: 2 });
    };
    const handleOverlay = () => {
      const chosen = chosenRef.current;
      if (chosen) {
        view.addAnnotation({ value: chosen }).catch(warn("mark failed"));
      }
    };
    view.addEventListener("draw-annotation", handleDraw);
    view.addEventListener("create-overlay", handleOverlay);
    return () => {
      view.removeEventListener("draw-annotation", handleDraw);
      view.removeEventListener("create-overlay", handleOverlay);
    };
  }, [view]);

  // Leaving search takes every highlight off the page.
  useEffect(() => {
    if (!view) return;
    return () => {
      cancel();
      queryRef.current = "";
      view.clearSearch();
      // Leaving the reader closes the book before this runs, and a closed
      // book has no marks left and fails to look one up.
      unmarkChosen().catch(() => {});
    };
  }, [view, cancel, unmarkChosen]);

  const groups = useMemo(() => groupByChapter(results), [results]);

  const goToResult = useCallback(
    (index: number) => goTo(index, results[index].cfi),
    [goTo, results],
  );

  return {
    query,
    changeQuery,
    results,
    groups,
    searching,
    waiting,
    current,
    goToResult,
    next: useCallback(() => step(1), [step]),
    previous: useCallback(() => step(-1), [step]),
  };
}
