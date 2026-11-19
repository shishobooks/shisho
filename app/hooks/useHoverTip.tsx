import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";

import HoverTipBox, {
  type HoverTipProps,
} from "@/components/library/HoverTipBox";

/**
 * A tooltip that shows the moment the pointer reaches something small, like
 * a heatmap square, rather than after the usual delay. It sits above what it
 * describes (below, near the top of the window), on the page's top layer so
 * a scrolling container can't clip it. A scroll or resize carries it along
 * with what it describes, and takes it down once that's scrolled out of
 * view or under something else. What it says must be reachable some other
 * way.
 *
 * A finger leaves the moment it lifts, so a tap's tip stays up until the
 * next press lands outside what it describes. A touch the browser takes
 * over to scroll was never a tap, and takes its tip down.
 *
 * Call `show` from `onPointerEnter` with the element and what to say, pass
 * `hide` itself as `onPointerLeave` (it reads the event to keep a tap's tip),
 * and render `tip`.
 */
export const useHoverTip = () => {
  const [tip, setTip] = useState<HoverTipProps | null>(null);
  const visible = tip !== null;
  // Set as the pointer arrives, which is before it presses, so a press on a
  // new target never reads the last one.
  const target = useRef<Element | null>(null);

  useEffect(() => {
    if (!visible) return;
    const hide = () => setTip(null);
    // Re-measured rather than hidden: a tap can shift the page under it (a
    // panel below loading in), and the tip should ride that out.
    const follow = () => {
      const el = target.current;
      if (!el?.isConnected) return hide();
      const rect = el.getBoundingClientRect();
      // Optional only because jsdom doesn't have it.
      const hit = document.elementFromPoint?.(
        rect.left + rect.width / 2,
        rect.top + rect.height / 2,
      );
      if (!hit || !el.contains(hit)) return hide();
      setTip((current) => current && { ...current, rect });
    };
    const pressElsewhere = (event: PointerEvent) => {
      const onTarget =
        event.target instanceof Node && target.current?.contains(event.target);
      if (!onTarget) hide();
    };
    window.addEventListener("scroll", follow, true);
    window.addEventListener("resize", follow);
    window.addEventListener("pointerdown", pressElsewhere, true);
    window.addEventListener("pointercancel", hide, true);
    return () => {
      window.removeEventListener("scroll", follow, true);
      window.removeEventListener("resize", follow);
      window.removeEventListener("pointerdown", pressElsewhere, true);
      window.removeEventListener("pointercancel", hide, true);
    };
  }, [visible]);

  // Stable, so a long list of hover targets can be memoized around them.
  const show = useCallback((element: Element, content: ReactNode) => {
    target.current = element;
    setTip({ rect: element.getBoundingClientRect(), content });
  }, []);
  const hide = useCallback((event?: { pointerType?: string }) => {
    if (event?.pointerType === "touch") return;
    setTip(null);
  }, []);

  return { show, hide, tip: tip && <HoverTipBox {...tip} /> };
};
