import { useCallback, useEffect, useRef, useState } from "react";

import { LocationHistory } from "@/libraries/locationHistory";

// The parts of foliate's <foliate-view> the history uses.
type HistoryBook = HTMLElement & {
  goTo?: (target: string) => unknown;
};

// Where the reader is, as foliate last reported it: the CFI of what the
// screen shows.
interface Here {
  cfi: string;
}

const warn = (err: unknown) =>
  console.warn("useLocationHistory: couldn't move", err);

/**
 * Back and forward through the places a reader jumped from in a Book open
 * in foliate, by the rules in `LocationHistory`. Every jump the reader makes
 * goes through `jump`, which records where it left from. foliate keeps a
 * history of its own, but it counts every result of a search as a jump and
 * can't tell a jump from turning into the next chapter, which it reports
 * the same way.
 *
 * foliate doesn't queue moves, and one can land before an earlier one still
 * loading a section, so jumps and moves back and forward take turns.
 */
export function useLocationHistory(view: HTMLElement | null) {
  const historyRef = useRef(new LocationHistory());
  const hereRef = useRef<Here | null>(null);
  const turnRef = useRef<Promise<unknown>>(Promise.resolve());
  const [state, setState] = useState({ canGoBack: false, canGoForward: false });

  const sync = useCallback(() => {
    const history = historyRef.current;
    setState({
      canGoBack: history.canGoBack,
      canGoForward: history.canGoForward,
    });
  }, []);

  // Runs `task` once the moves before it have landed.
  const takeTurn = useCallback(<T>(task: () => Promise<T>) => {
    const turn = turnRef.current.then(task);
    turnRef.current = turn.catch(() => {});
    return turn;
  }, []);

  /** Starts over, for a Book just opened. */
  const reset = useCallback(() => {
    historyRef.current = new LocationHistory();
    hereRef.current = null;
    sync();
  }, [sync]);

  useEffect(() => {
    if (!view) return;
    const handleRelocate = (evt: Event) => {
      const cfi = (evt as CustomEvent<{ cfi?: string }>).detail?.cfi;
      if (cfi) hereRef.current = { cfi };
    };
    view.addEventListener("relocate", handleRelocate);
    return () => view.removeEventListener("relocate", handleRelocate);
  }, [view]);

  /**
   * Makes a jump, recording where it left from if it moved the page.
   * Jumps in one `group`, such as one search's results, record one entry.
   * A failed jump records nothing and rejects.
   */
  const jump = useCallback(
    (go: () => unknown, group?: object) =>
      takeTurn(async () => {
        const history = historyRef.current;
        const origin = hereRef.current;
        // foliate reports where a jump landed before the jump resolves.
        await go();
        // A Book opened again meanwhile has a history of its own.
        if (!origin || historyRef.current !== history) return;
        // A jump that didn't move the view, such as to a search result on
        // the page shown, leaves nothing to go back to. Any move counts,
        // however short, such as to the page just before or a short scroll
        // up, so going back has somewhere to return to.
        const landed = hereRef.current;
        if (!landed || landed === origin || landed.cfi === origin.cfi) return;
        history.recordJump(origin.cfi, group);
        sync();
      }),
    [takeTurn, sync],
  );

  const move = useCallback(
    (direction: "back" | "forward") =>
      takeTurn(async () => {
        const here = hereRef.current;
        if (!view || !here) return;
        const history = historyRef.current;
        const before = history.save();
        const target =
          direction === "back"
            ? history.back(here.cfi)
            : history.forward(here.cfi);
        sync();
        if (target === null) return;
        try {
          // A move like any other, which the reader saves as the Reading
          // Position.
          await (view as HistoryBook).goTo?.(target);
        } finally {
          // foliate ignores a move for a moment after a page turn: the
          // history stays as it was, to try again.
          if (hereRef.current === here && historyRef.current === history) {
            history.restore(before);
            sync();
          }
        }
      }).catch(warn),
    [view, takeTurn, sync],
  );

  return {
    ...state,
    jump,
    reset,
    goBack: useCallback(() => void move("back"), [move]),
    goForward: useCallback(() => void move("forward"), [move]),
  };
}
