import { useEffect, useState, type ReactNode } from "react";

import HoverTipBox, {
  type HoverTipProps,
} from "@/components/library/HoverTipBox";

/**
 * A tooltip that shows the moment the pointer reaches something small, like
 * a heatmap square, rather than after the usual delay. It sits above what it
 * describes (below, near the top of the window), on the page's top layer so
 * a scrolling container can't clip it, and goes away on any scroll. It's for
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
    return () => window.removeEventListener("scroll", hide, true);
  }, [visible]);

  return {
    show: (element: Element, content: ReactNode) =>
      setTip({ rect: element.getBoundingClientRect(), content }),
    hide: () => setTip(null),
    tip: tip && <HoverTipBox {...tip} />,
  };
};
