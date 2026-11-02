import { useMemo } from "react";

// How far a finger travels before a drag counts as a swipe, in CSS pixels.
const MIN_SWIPE_DISTANCE = 50;

type Turn = "next" | "prev";

interface TouchPoint {
  screenX: number;
  screenY: number;
}

// The part of a touch event a swipe needs, which DOM and React events share.
interface SwipeTouchEvent {
  touches: ArrayLike<TouchPoint>;
  changedTouches: ArrayLike<TouchPoint>;
}

// Touch handlers that turn a page on a horizontal swipe: to the left is the
// next page, to the right the previous one. A second finger makes the gesture a
// pinch, and a drag on a zoomed page pans it, so neither turns. Positions are
// screen coordinates, which a scaled frame doesn't change, so one set of
// handlers works across several documents.
export const createSwipeHandlers = (onTurn: (turn: Turn) => void) => {
  let start: TouchPoint | null = null;
  return {
    onTouchCancel: () => {
      start = null;
    },
    onTouchEnd: (e: SwipeTouchEvent) => {
      const from = start;
      start = null;
      const to = e.changedTouches[0];
      if (!from || !to) return;
      if ((globalThis.visualViewport?.scale ?? 1) > 1) return;
      const dx = to.screenX - from.screenX;
      const dy = to.screenY - from.screenY;
      if (Math.abs(dx) < MIN_SWIPE_DISTANCE || Math.abs(dx) <= Math.abs(dy)) {
        return;
      }
      onTurn(dx < 0 ? "next" : "prev");
    },
    onTouchStart: (e: SwipeTouchEvent) => {
      start = e.touches.length === 1 ? e.touches[0] : null;
    },
  };
};

// The handlers for React props. Touch events bubble in React, so these go on an
// element around whatever a swipe may start on.
export const useSwipeTurn = (onTurn: (turn: Turn) => void) =>
  useMemo(() => createSwipeHandlers(onTurn), [onTurn]);
