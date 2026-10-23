import { ChevronDown, ChevronUp } from "lucide-react";
import { useLayoutEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";

// How far past the collapsed height text may run before it collapses, so a
// description one line too long shows whole instead of behind a button.
const SLACK_PX = 24;

const FADE = "linear-gradient(to bottom, black 55%, transparent 100%)";

interface FadedTextProps {
  text: string;
  /** The height the text collapses to, in pixels. */
  collapsedPx?: number;
  className?: string;
}

/**
 * Long text that fades out under a "Show more" button. It fades instead of
 * ending in an ellipsis because descriptions pulled from metadata sources
 * often end in one already, cut off at the source, and an ellipsis Shisho
 * added could not be told apart from that.
 */
const FadedText = ({ text, collapsedPx = 96, className }: FadedTextProps) => {
  const ref = useRef<HTMLParagraphElement>(null);
  const [overflows, setOverflows] = useState(false);
  const [expanded, setExpanded] = useState(false);

  // Measures again on resize: text that fits on a wide screen can overflow
  // on a narrow one, and a copy hidden by CSS measures zero until shown.
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () =>
      setOverflows(el.scrollHeight > collapsedPx + SLACK_PX);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [text, collapsedPx]);

  const collapsed = overflows && !expanded;
  return (
    <div className={className}>
      <p
        className="overflow-hidden whitespace-pre-wrap break-words text-sm leading-relaxed text-foreground/85"
        ref={ref}
        style={
          collapsed
            ? { maxHeight: collapsedPx, maskImage: FADE, WebkitMaskImage: FADE }
            : undefined
        }
      >
        {text}
      </p>
      {overflows && (
        <Button
          aria-expanded={expanded}
          className="mt-1 gap-1 font-medium"
          onClick={() => setExpanded(!expanded)}
          variant="link"
        >
          {expanded ? "Show less" : "Show more"}
          {expanded ? (
            <ChevronUp aria-hidden="true" className="size-4" />
          ) : (
            <ChevronDown aria-hidden="true" className="size-4" />
          )}
        </Button>
      )}
    </div>
  );
};

export default FadedText;
