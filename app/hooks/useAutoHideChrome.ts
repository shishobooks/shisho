import { useCallback, useEffect, useRef, useState } from "react";

const INITIAL_HIDE_DELAY = 2000;
const INACTIVITY_HIDE_DELAY = 3000;

export function useAutoHideChrome(enabled: boolean) {
  const [visible, setVisible] = useState(true);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!enabled) {
      setVisible(true);
      return;
    }

    const startHideTimer = (delay: number) => {
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => setVisible(false), delay);
    };

    const handleMouseMove = () => {
      setVisible(true);
      startHideTimer(INACTIVITY_HIDE_DELAY);
    };

    startHideTimer(INITIAL_HIDE_DELAY);
    window.addEventListener("mousemove", handleMouseMove);
    return () => {
      window.removeEventListener("mousemove", handleMouseMove);
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [enabled]);

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

  return { chromeVisible: !enabled || visible, toggleChrome };
}
