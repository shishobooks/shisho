import { memo } from "react";

import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { cn } from "@/libraries/utils";

// Closer to an end than this counts as at it: foliate nudges every section's
// start by Number.EPSILON, so the first chapter isn't quite at 0. Anything
// further in is a real place, such as a minute-long last chapter of a long
// audiobook.
const EDGE = 1e-6;

// Marks of one kind closer than this are one place: a part and its first
// chapter on one page, or two chapters starting at one moment.
const SAME_PLACE = 1e-6;

// One mark per place for each kind, named after everything there, with
// every bookmark's note, and going to the first, in the order given.
const mergeSamePlace = (marks: ProgressMark[]) => {
  const merged: ProgressMark[] = [];
  for (const mark of marks) {
    const same = merged.find(
      (m) =>
        m.kind === mark.kind &&
        Math.abs(m.position - mark.position) < SAME_PLACE,
    );
    if (!same) {
      merged.push({ ...mark });
      continue;
    }
    if (!same.label.split(" / ").includes(mark.label)) {
      same.label = `${same.label} / ${mark.label}`;
    }
    if (mark.note) {
      same.note = same.note ? `${same.note} / ${mark.note}` : mark.note;
    }
  }
  return merged;
};

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
function ProgressMarks({
  marks,
  rtl = false,
}: {
  marks: ProgressMark[];
  // For a bar that fills from the right, as a page-based File that reads
  // right to left shows it.
  rtl?: boolean;
}) {
  // A phone's taps can't hover, and iOS takes a tap that shows something on
  // hover as only a hover, so a tooltip there would cost every mark a second
  // tap.
  const canHover = !useMediaQuery("(hover: none)");
  const shown = mergeSamePlace(marks).filter(({ kind, position }) =>
    kind === "chapter"
      ? // A chapter at either end would sit on the bar's own rounded end.
        position > EDGE && position < 1 - EDGE
      : // A bookmark anywhere is worth showing, even seconds in.
        position >= 0 && position <= 1,
  );
  if (shown.length === 0) return null;
  // How close each mark's nearest neighbor of its own kind is, as a share
  // of the bar. Its hit area stays within that, so pointing at a mark in a
  // crowded stretch (a book with a hundred short chapters) picks that mark.
  // A bookmark draws over a chapter mark, so the two kinds may overlap.
  const room = new Map<ProgressMark, number>();
  for (const kind of ["chapter", "bookmark"] as const) {
    const ofKind = shown
      .filter((mark) => mark.kind === kind)
      .sort((a, b) => a.position - b.position);
    ofKind.forEach((mark, i) => {
      room.set(
        mark,
        Math.min(
          i > 0 ? mark.position - ofKind[i - 1].position : Infinity,
          i < ofKind.length - 1
            ? ofKind[i + 1].position - mark.position
            : Infinity,
        ),
      );
    });
  }
  // Bookmarks draw over chapter marks, so one at a chapter's start can still
  // be pointed at.
  shown.sort(
    (a, b) => Number(a.kind === "bookmark") - Number(b.kind === "bookmark"),
  );
  return (
    <div aria-hidden="true" className="pointer-events-none absolute inset-0">
      {shown.map((mark) => {
        // Only marks this close need the limit: past it, the gap is wider
        // than the hit area on any bar a reader shows.
        const gap = room.get(mark) ?? Infinity;
        // Stable as bookmarks come and go, so an open tooltip stays with
        // its mark.
        const key = `${mark.kind}:${mark.position}`;
        const target = (
          // A hit area wider than the mark, centered on it.
          <Button
            aria-label={mark.note ? `${mark.label}: ${mark.note}` : mark.label}
            className={cn(
              "pointer-events-auto absolute top-1/2 flex h-4 w-3 -translate-y-1/2 items-center justify-center",
              rtl ? "translate-x-1/2" : "-translate-x-1/2",
            )}
            data-testid={`${mark.kind}-mark`}
            key={key}
            onClick={(e) => {
              e.stopPropagation();
              mark.onSelect();
            }}
            // A mark takes no focus on a press: inside the audiobook's seek
            // bar, a focused mark would hand the arrow keys to the bar.
            onMouseDown={(e) => e.preventDefault()}
            // The audiobook's seek bar starts a drag on a press, which a
            // press on a mark isn't.
            onPointerDown={(e) => e.stopPropagation()}
            style={{
              [rtl ? "right" : "left"]: `${mark.position * 100}%`,
              maxWidth: gap < 0.05 ? `${gap * 100}%` : undefined,
            }}
            tabIndex={-1}
            variant="unstyled"
          >
            {mark.kind === "chapter" ? (
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
          <Tooltip key={key}>
            <TooltipTrigger asChild>{target}</TooltipTrigger>
            <TooltipContent className="max-w-64" side="top">
              <p>{mark.label}</p>
              {mark.note && <p className="opacity-80">{mark.note}</p>}
            </TooltipContent>
          </Tooltip>
        );
      })}
    </div>
  );
}

// Readers re-render on every page turn and every second of playback, and
// each mark is a tooltip of its own, so the marks re-render only when they
// change.
export default memo(ProgressMarks);
