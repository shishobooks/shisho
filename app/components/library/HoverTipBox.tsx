import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

// Pixels between the tip and what it describes, and kept from the window's
// edges.
const OFFSET = 6;
const EDGE = 8;

export interface HoverTipProps {
  rect: DOMRect;
  content: ReactNode;
}

/** The box `useHoverTip` shows; see it for how it behaves. */
const HoverTipBox = ({ rect, content }: HoverTipProps) => {
  const ref = useRef<HTMLDivElement>(null);
  const [left, setLeft] = useState<number | null>(null);
  const below = rect.top < 64;

  // Centre it on what it describes, kept inside the window.
  useLayoutEffect(() => {
    const width = ref.current?.offsetWidth ?? 0;
    const centre = rect.left + rect.width / 2;
    setLeft(
      Math.min(
        Math.max(EDGE, centre - width / 2),
        window.innerWidth - width - EDGE,
      ),
    );
  }, [rect, content]);

  return createPortal(
    <div
      className="pointer-events-none fixed z-50 whitespace-nowrap rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs text-popover-foreground shadow-md"
      ref={ref}
      role="tooltip"
      style={{
        left: left ?? -9999,
        top: below ? rect.bottom + OFFSET : rect.top - OFFSET,
        transform: below ? undefined : "translateY(-100%)",
      }}
    >
      {content}
    </div>,
    document.body,
  );
};

export default HoverTipBox;
