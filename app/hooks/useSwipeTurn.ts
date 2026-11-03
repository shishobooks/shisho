import { useMemo } from "react";

// How far a finger travels before a drag counts as a swipe, in CSS pixels.
const MIN_SWIPE_DISTANCE = 50;
// How far it travels before a drag has a direction, so a page doesn't twitch
// sideways at the start of a vertical scroll.
const DIRECTION_LOCK_DISTANCE = 10;

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

// For showing a swipe as it happens.
export interface SwipeDragCallbacks {
  // The finger is dx pixels sideways of where it started.
  onDrag: (dx: number) => void;
  // The gesture ended without asking for a turn.
  onDragEnd: () => void;
}

const isZoomed = () => (globalThis.visualViewport?.scale ?? 1) > 1;

// Touch handlers that turn a page on a horizontal swipe: to the left is the
// next page, to the right the previous one. A second finger makes the gesture a
// pinch, and a drag on a zoomed page pans it, so neither turns. Positions are
// screen coordinates, which a scaled frame doesn't change, so one set of
// handlers works across several documents.
export const createSwipeHandlers = (
  onTurn: (turn: Turn) => void,
  drag?: SwipeDragCallbacks,
) => {
  let start: TouchPoint | null = null;
  let direction: "horizontal" | "vertical" | null = null;
  let dragging = false;

  const stop = () => {
    start = null;
    direction = null;
    if (dragging) drag?.onDragEnd();
    dragging = false;
  };

  return {
    onTouchCancel: stop,
    onTouchEnd: (e: SwipeTouchEvent) => {
      const from = start;
      const to = e.changedTouches[0];
      if (!from || !to || direction === "vertical" || isZoomed()) {
        stop();
        return;
      }
      const dx = to.screenX - from.screenX;
      const dy = to.screenY - from.screenY;
      if (Math.abs(dx) < MIN_SWIPE_DISTANCE || Math.abs(dx) <= Math.abs(dy)) {
        stop();
        return;
      }
      start = null;
      direction = null;
      dragging = false;
      onTurn(dx < 0 ? "next" : "prev");
    },
    onTouchMove: (e: SwipeTouchEvent) => {
      const to = e.touches[0];
      if (!start || !to || !drag || isZoomed()) return;
      const dx = to.screenX - start.screenX;
      const dy = to.screenY - start.screenY;
      if (!direction) {
        if (Math.max(Math.abs(dx), Math.abs(dy)) < DIRECTION_LOCK_DISTANCE) {
          return;
        }
        direction = Math.abs(dx) > Math.abs(dy) ? "horizontal" : "vertical";
      }
      if (direction === "vertical") return;
      dragging = true;
      drag.onDrag(dx);
    },
    onTouchStart: (e: SwipeTouchEvent) => {
      stop();
      start = e.touches.length === 1 ? e.touches[0] : null;
    },
  };
};

// The handlers for React props. Touch events bubble in React, so these go on an
// element around whatever a swipe may start on. Pass stable callbacks: new ones
// forget a gesture in progress.
export const useSwipeTurn = (
  onTurn: (turn: Turn) => void,
  drag?: SwipeDragCallbacks,
) => useMemo(() => createSwipeHandlers(onTurn, drag), [onTurn, drag]);
