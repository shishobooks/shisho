import { useEffect, useMemo, useState } from "react";

import type { PageTurn } from "@/utils/readerKeys";

// How far a finger travels before a drag counts as a swipe, in CSS pixels.
const MIN_SWIPE_DISTANCE = 50;
// How far it travels before a drag has a direction, so a page doesn't twitch
// sideways at the start of a vertical scroll.
const DIRECTION_LOCK_DISTANCE = 10;

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

export const isZoomed = () => (globalThis.visualViewport?.scale ?? 1) > 1;

// The gesture in progress, kept apart from the handlers so that handlers rebuilt
// for a new page carry on with it.
interface SwipeGesture {
  start: TouchPoint | null;
  direction: "horizontal" | "vertical" | null;
  dragging: boolean;
}

const newSwipeGesture = (): SwipeGesture => ({
  direction: null,
  dragging: false,
  start: null,
});

// Touch handlers that turn a page on a horizontal swipe: to the left is the
// next page, to the right the previous one. A second finger makes the gesture a
// pinch, and a drag on a zoomed page pans it, so neither turns. Positions are
// screen coordinates, which a scaled frame doesn't change, so one set of
// handlers works across several documents.
export const createSwipeHandlers = (
  onTurn: (turn: PageTurn) => void,
  drag?: SwipeDragCallbacks,
  gesture: SwipeGesture = newSwipeGesture(),
) => {
  const stop = () => {
    const wasDragging = gesture.dragging;
    Object.assign(gesture, newSwipeGesture());
    if (wasDragging) drag?.onDragEnd();
  };

  return {
    onTouchCancel: stop,
    onTouchEnd: (e: SwipeTouchEvent) => {
      const from = gesture.start;
      const to = e.changedTouches[0];
      if (!from || !to || gesture.direction === "vertical" || isZoomed()) {
        stop();
        return;
      }
      const dx = to.screenX - from.screenX;
      const dy = to.screenY - from.screenY;
      if (Math.abs(dx) < MIN_SWIPE_DISTANCE || Math.abs(dx) <= Math.abs(dy)) {
        stop();
        return;
      }
      Object.assign(gesture, newSwipeGesture());
      onTurn(dx < 0 ? "next" : "prev");
    },
    onTouchMove: (e: SwipeTouchEvent) => {
      const from = gesture.start;
      const to = e.touches[0];
      if (!from || !to || !drag || isZoomed()) return;
      const dx = to.screenX - from.screenX;
      const dy = to.screenY - from.screenY;
      if (!gesture.direction) {
        if (Math.max(Math.abs(dx), Math.abs(dy)) < DIRECTION_LOCK_DISTANCE) {
          return;
        }
        gesture.direction =
          Math.abs(dx) > Math.abs(dy) ? "horizontal" : "vertical";
      }
      if (gesture.direction === "vertical") return;
      gesture.dragging = true;
      drag.onDrag(dx);
    },
    onTouchStart: (e: SwipeTouchEvent) => {
      stop();
      gesture.start = e.touches.length === 1 ? e.touches[0] : null;
    },
  };
};

// Whether the page is pinch-zoomed in, which gives a one-finger drag to the
// browser so it can pan.
export const usePinchZoomed = () => {
  const [zoomed, setZoomed] = useState(isZoomed);
  useEffect(() => {
    const viewport = globalThis.visualViewport;
    if (!viewport) return;
    const update = () => setZoomed(isZoomed());
    viewport.addEventListener("resize", update);
    return () => viewport.removeEventListener("resize", update);
  }, []);
  return zoomed;
};

// The handlers for React props. Touch events bubble in React, so these go on an
// element around whatever a swipe may start on. A gesture survives new
// callbacks, so a swipe can start while the last turn is landing.
export const useSwipeTurn = (
  onTurn: (turn: PageTurn) => void,
  drag?: SwipeDragCallbacks,
) => {
  const [gesture] = useState(newSwipeGesture);
  return useMemo(
    () => createSwipeHandlers(onTurn, drag, gesture),
    [drag, gesture, onTurn],
  );
};
