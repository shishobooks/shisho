import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useMediaQuery } from "@/hooks/useMediaQuery";
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
  // A phone's taps can't hover, and iOS takes a tap that shows something on
  // hover as only a hover, so a tooltip there would cost every mark a second
  // tap.
  const canHover = !useMediaQuery("(hover: none)");
  const shown = markers.filter(({ kind, position }) =>
    kind === "chapter"
      ? // A chapter at either end would sit on the bar's own rounded end.
        position > EDGE && position < 1 - EDGE
      : // A bookmark anywhere is worth showing, even seconds in.
        position >= 0 && position <= 1,
  );
  if (shown.length === 0) return null;
  // How close each mark's nearest neighbor is, as a share of the bar. Its
  // hit area stays within that, so pointing at a mark in a crowded stretch
  // (a book with a hundred short chapters) picks that mark.
  const byPosition = [...shown].sort((a, b) => a.position - b.position);
  const room = new Map(
    byPosition.map((mark, i) => [
      mark,
      Math.min(
        i > 0 ? mark.position - byPosition[i - 1].position : Infinity,
        i < byPosition.length - 1
          ? byPosition[i + 1].position - mark.position
          : Infinity,
      ),
    ]),
  );
  // Bookmarks draw over chapter marks, so one at a chapter's start can still
  // be pointed at.
  shown.sort(
    (a, b) => Number(a.kind === "bookmark") - Number(b.kind === "bookmark"),
  );
  return (
    <div aria-hidden="true" className="pointer-events-none absolute inset-0">
      {shown.map((marker, i) => {
        // Only marks this close need the limit: past it, the gap is wider
        // than the hit area on any bar a reader shows.
        const gap = room.get(marker) ?? Infinity;
        const target = (
          // A hit area wider than the mark, centered on it.
          <Button
            aria-label={
              marker.note ? `${marker.label}: ${marker.note}` : marker.label
            }
            className={cn(
              "pointer-events-auto absolute top-1/2 flex h-4 w-3 -translate-y-1/2 items-center justify-center",
              rtl ? "translate-x-1/2" : "-translate-x-1/2",
            )}
            data-testid={`${marker.kind}-marker`}
            key={canHover ? undefined : i}
            onClick={(e) => {
              e.stopPropagation();
              marker.onSelect();
            }}
            size="none"
            style={{
              [rtl ? "right" : "left"]: `${marker.position * 100}%`,
              maxWidth:
                gap < Infinity && gap < 0.05 ? `${gap * 100}%` : undefined,
            }}
            tabIndex={-1}
            variant="unstyled"
          >
            {marker.kind === "chapter" ? (
              <span className="h-3 w-0.5 shrink-0 rounded-full bg-muted-foreground/60" />
            ) : (
              // Ringed in the page's color, so it shows on the filled part
              // of the bar as well as the empty part.
              <span className="h-2.5 w-2.5 shrink-0 rounded-full bg-primary ring-2 ring-background" />
            )}
          </Button>
        );
        if (!canHover) return target;
        return (
          <Tooltip key={i}>
            <TooltipTrigger asChild>{target}</TooltipTrigger>
            <TooltipContent className="max-w-64" side="top">
              <p>{marker.label}</p>
              {marker.note && <p className="opacity-80">{marker.note}</p>}
            </TooltipContent>
          </Tooltip>
        );
      })}
    </div>
  );
}
