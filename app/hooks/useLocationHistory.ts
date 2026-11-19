import { useCallback, useEffect, useRef, useState } from "react";

import { LocationHistory } from "@/libraries/locationHistory";

// The parts of foliate's <foliate-view> the history uses.
type HistoryBook = HTMLElement & {
  goTo?: (target: string) => unknown;
  resolveNavigation?: (target: string) => {
    index?: number;
    anchor?: (doc: Document) => Range | Node | null;
  } | null;
};

// Where the reader is, as foliate last reported it.
interface Here {
  cfi: string;
  section?: number;
  // What the screen shows. A fixed-layout book's renderer doesn't say, and
  // there a section is a page.
  range?: Range | null;
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
      const detail = (
        evt as CustomEvent<{
          cfi?: string;
          range?: Range | null;
          section?: { current?: number } | null;
        }>
      ).detail;
      if (!detail?.cfi) return;
      hereRef.current = {
        cfi: detail.cfi,
        section: detail.section?.current,
        range: detail.range,
      };
    };
    view.addEventListener("relocate", handleRelocate);
    return () => view.removeEventListener("relocate", handleRelocate);
  }, [view]);

  // Whether a place is on the screen the reader shows now. In a fixed-layout
  // book that's its page, so a jump between the two pages of a spread counts
  // as leaving the screen.
  const onScreen = useCallback(
    (cfi: string) => {
      const here = hereRef.current;
      if (!here) return false;
      try {
        const resolved = (view as HistoryBook | null)?.resolveNavigation?.(cfi);
        if (!resolved || resolved.index !== here.section) return false;
        if (!here.range) return true;
        const node = here.range.startContainer;
        const target = resolved.anchor?.(
          node.ownerDocument ?? (node as Document),
        );
        if (!target) return false;
        return "startContainer" in target
          ? here.range.comparePoint(
              target.startContainer,
              target.startOffset,
            ) === 0
          : here.range.comparePoint(target, 0) === 0;
      } catch {
        return false;
      }
    },
    [view],
  );

  /**
   * Makes a jump, recording where it left from if it leaves the screen.
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
        // Nothing moved, or the jump stayed on the screen.
        if (hereRef.current === origin || onScreen(origin.cfi)) return;
        history.recordJump(origin.cfi, group);
        sync();
      }),
    [takeTurn, onScreen, sync],
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
            ? history.back(here.cfi, onScreen)
            : history.forward(here.cfi, onScreen);
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
    [view, takeTurn, onScreen, sync],
  );

  return {
    ...state,
    jump,
    reset,
    goBack: useCallback(() => void move("back"), [move]),
    goForward: useCallback(() => void move("forward"), [move]),
  };
}
