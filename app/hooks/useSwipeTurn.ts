import { useCallback, useRef, type TouchEvent } from "react";

// How far a finger travels before a drag counts as a swipe, in CSS pixels.
const MIN_SWIPE_DISTANCE = 50;

// Touch handlers that turn a page on a horizontal swipe: to the left is the
// next page, to the right the previous one. React touch events bubble, so these
// go on an element around whatever a swipe may start on. A second finger makes
// the gesture a pinch, and a drag on a zoomed page pans it, so neither turns.
export const useSwipeTurn = (onTurn: (turn: "next" | "prev") => void) => {
  const startRef = useRef<{ x: number; y: number } | null>(null);

  const onTouchStart = useCallback((e: TouchEvent) => {
    const touch = e.touches[0];
    startRef.current =
      e.touches.length === 1 ? { x: touch.clientX, y: touch.clientY } : null;
  }, []);

  const onTouchEnd = useCallback(
    (e: TouchEvent) => {
      const start = startRef.current;
      startRef.current = null;
      const touch = e.changedTouches[0];
      if (!start || !touch) return;
      if ((globalThis.visualViewport?.scale ?? 1) > 1) return;
      const dx = touch.clientX - start.x;
      const dy = touch.clientY - start.y;
      if (Math.abs(dx) < MIN_SWIPE_DISTANCE || Math.abs(dx) <= Math.abs(dy)) {
        return;
      }
      onTurn(dx < 0 ? "next" : "prev");
    },
    [onTurn],
  );

  const onTouchCancel = useCallback(() => {
    startRef.current = null;
  }, []);

  return { onTouchCancel, onTouchEnd, onTouchStart };
};
