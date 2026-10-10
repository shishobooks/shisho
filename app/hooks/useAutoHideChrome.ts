import { useCallback, useEffect, useRef, useState } from "react";

const INITIAL_HIDE_DELAY = 2000;
const INACTIVITY_HIDE_DELAY = 3000;

/** True for movement of a real mouse. A tap also fires pointer and mouse
 *  events before its click; counting those as movement would show the
 *  controls and let the tap's toggle hide them again in the same gesture. */
export const isMouseMove = (e: Event): boolean =>
  (e as PointerEvent).pointerType === "mouse";

export function useAutoHideChrome(enabled: boolean) {
  const [visible, setVisible] = useState(true);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const startHideTimer = useCallback((delay: number) => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setVisible(false), delay);
  }, []);

  // Shows the controls and restarts the inactivity timer. The window's
  // pointer listener calls it, and so does a reader for movement the window
  // never sees, such as inside a book's iframe.
  const revealChrome = useCallback(() => {
    if (!enabled) return;
    setVisible(true);
    startHideTimer(INACTIVITY_HIDE_DELAY);
  }, [enabled, startHideTimer]);

  useEffect(() => {
    if (!enabled) {
      setVisible(true);
      return;
    }

    const handlePointerMove = (e: PointerEvent) => {
      if (isMouseMove(e)) revealChrome();
    };

    startHideTimer(INITIAL_HIDE_DELAY);
    window.addEventListener("pointermove", handlePointerMove);
    return () => {
      window.removeEventListener("pointermove", handlePointerMove);
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [enabled, revealChrome, startHideTimer]);

  const toggleChrome = useCallback(() => {
    if (!enabled) return;
    setVisible((v) => {
      if (timerRef.current) clearTimeout(timerRef.current);
      if (!v) {
        timerRef.current = setTimeout(
          () => setVisible(false),
          INACTIVITY_HIDE_DELAY,
        );
      }
      return !v;
    });
  }, [enabled]);

  return { chromeVisible: !enabled || visible, revealChrome, toggleChrome };
}
