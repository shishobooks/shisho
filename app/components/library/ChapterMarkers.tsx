import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

// Closer to an end than this counts as at it. foliate nudges every section's
// start by Number.EPSILON, so the first chapter isn't quite at 0.
const EDGE = 0.001;

export interface ChapterMarker {
  // From 0 (the start of the bar) to 1 (its end).
  position: number;
  // The chapter's name, shown while the pointer rests on its mark.
  label: string;
  // Goes to the chapter's exact start, which the bar's own seeking, by the
  // fraction under the pointer, only comes close to.
  onSelect: () => void;
}

// Marks along a reader's progress or seek bar where each chapter starts, the
// same in the audiobook player and the ebook, comic, and PDF readers. Lay it
// over the bar inside a `relative` parent. A mark names its chapter on hover
// and goes to it on a click, without the click also seeking the bar.
//
// The marks stay out of the tab order and away from screen readers: the
// chapter menu beside every bar does the same from the keyboard, and a mark
// per chapter would bury the controls after them.
export default function ChapterMarkers({
  markers,
}: {
  markers: ChapterMarker[];
}) {
  // A chapter at either end would sit on the bar's own rounded end.
  const inside = markers.filter(
    ({ position }) => position > EDGE && position < 1 - EDGE,
  );
  if (inside.length === 0) return null;
  return (
    <div aria-hidden="true" className="pointer-events-none absolute inset-0">
      {inside.map((marker, i) => (
        <Tooltip key={i}>
          <TooltipTrigger asChild>
            {/* A hit area wider than the tick, centered on it. */}
            <Button
              aria-label={marker.label}
              className="pointer-events-auto absolute top-1/2 flex h-4 w-3 -translate-x-1/2 -translate-y-1/2 items-center justify-center"
              data-testid="chapter-marker"
              onClick={(e) => {
                e.stopPropagation();
                marker.onSelect();
              }}
              size="none"
              style={{ left: `${marker.position * 100}%` }}
              tabIndex={-1}
              variant="unstyled"
            >
              <span className="h-3 w-0.5 rounded-full bg-muted-foreground/60" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="top">{marker.label}</TooltipContent>
        </Tooltip>
      ))}
    </div>
  );
}
