import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/libraries/utils";

// Closer to an end than this counts as at it. foliate nudges every section's
// start by Number.EPSILON, so the first chapter isn't quite at 0.
const EDGE = 0.001;

export interface ProgressMark {
  // A chapter's start, or a bookmark the reader added.
  kind: "chapter" | "bookmark";
  // From 0 (the start of the bar) to 1 (its end).
  position: number;
  // What the mark is, shown while the pointer rests on it.
  label: string;
  // A bookmark's note, shown under its label.
  note?: string | null;
  // Goes to the exact place, which the bar's own seeking, by the fraction
  // under the pointer, only comes close to.
  onSelect: () => void;
}

// Marks along a reader's progress or seek bar: where each chapter starts, the
// same in the audiobook player and the ebook, comic, and PDF readers, and
// the reader's bookmarks. Lay it over the bar inside a `relative` parent. A
// mark says what it is on hover and goes there on a click, without the click
// also seeking the bar.
//
// The marks stay out of the tab order and away from screen readers: the
// chapter menu and the Bookmarks panel beside the bar do the same from the
// keyboard, and a mark each would bury the controls after them.
export default function ProgressMarks({
  markers,
  rtl = false,
}: {
  markers: ProgressMark[];
  // For a bar that fills from the right, as a page-based File that reads
  // right to left shows it.
  rtl?: boolean;
}) {
  const shown = markers.filter(({ kind, position }) =>
    kind === "chapter"
      ? // A chapter at either end would sit on the bar's own rounded end.
        position > EDGE && position < 1 - EDGE
      : // A bookmark anywhere is worth showing, even seconds in.
        position >= 0 && position <= 1,
  );
  if (shown.length === 0) return null;
  // Bookmarks draw over chapter marks, so one at a chapter's start can still
  // be pointed at.
  shown.sort(
    (a, b) => Number(a.kind === "bookmark") - Number(b.kind === "bookmark"),
  );
  return (
    <div aria-hidden="true" className="pointer-events-none absolute inset-0">
      {shown.map((marker, i) => (
        <Tooltip key={i}>
          <TooltipTrigger asChild>
            {/* A hit area wider than the mark, centered on it. */}
            <Button
              aria-label={
                marker.note ? `${marker.label}: ${marker.note}` : marker.label
              }
              className={cn(
                "pointer-events-auto absolute top-1/2 flex h-4 w-3 -translate-y-1/2 items-center justify-center",
                rtl ? "translate-x-1/2" : "-translate-x-1/2",
              )}
              data-testid={`${marker.kind}-marker`}
              onClick={(e) => {
                e.stopPropagation();
                marker.onSelect();
              }}
              size="none"
              style={{
                [rtl ? "right" : "left"]: `${marker.position * 100}%`,
              }}
              tabIndex={-1}
              variant="unstyled"
            >
              {marker.kind === "chapter" ? (
                <span className="h-3 w-0.5 rounded-full bg-muted-foreground/60" />
              ) : (
                // Ringed in the page's color, so it shows on the filled part
                // of the bar as well as the empty part.
                <span className="h-2.5 w-2.5 rounded-full bg-primary ring-2 ring-background" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent className="max-w-64" side="top">
            <p>{marker.label}</p>
            {marker.note && <p className="opacity-80">{marker.note}</p>}
          </TooltipContent>
        </Tooltip>
      ))}
    </div>
  );
}
