import { useCallback, useEffect, useState, type ReactNode } from "react";

import HoverTipBox, {
  type HoverTipProps,
} from "@/components/library/HoverTipBox";

/**
 * A tooltip that shows the moment the pointer reaches something small, like
 * a heatmap square, rather than after the usual delay. It sits above what it
 * describes (below, near the top of the window), on the page's top layer so
 * a scrolling container can't clip it, and goes away on any scroll or
 * resize, which would leave it out of place. It's for
 * pointers only: what it says must be reachable some other way.
 *
 * Call `show` from `onPointerEnter` with the element and what to say, `hide`
 * from `onPointerLeave`, and render `tip`.
 */
export const useHoverTip = () => {
  const [tip, setTip] = useState<HoverTipProps | null>(null);
  const visible = tip !== null;

  useEffect(() => {
    if (!visible) return;
    const hide = () => setTip(null);
    window.addEventListener("scroll", hide, true);
    window.addEventListener("resize", hide);
    return () => {
      window.removeEventListener("scroll", hide, true);
      window.removeEventListener("resize", hide);
    };
  }, [visible]);

  // Stable, so a long list of hover targets can be memoized around them.
  const show = useCallback(
    (element: Element, content: ReactNode) =>
      setTip({ rect: element.getBoundingClientRect(), content }),
    [],
  );
  const hide = useCallback(() => setTip(null), []);

  return { show, hide, tip: tip && <HoverTipBox {...tip} /> };
};
