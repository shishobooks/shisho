// Closer to an end than this counts as at it. foliate nudges every section's
// start by Number.EPSILON, so the first chapter isn't quite at 0.
const EDGE = 0.001;

// Ticks along a reader's progress or seek bar where each chapter starts, the
// same in the audiobook player and the ebook, comic, and PDF readers. Lay it
// over the bar inside a `relative` parent. Positions run from 0 (the start
// of the bar) to 1 (its end).
//
// The ticks are decoration: the chapter's name shows beside the bar and in
// the chapter menu, so screen readers skip them and they take no clicks,
// which go to the bar under them.
export default function ChapterMarkers({ positions }: { positions: number[] }) {
  // A chapter at either end would sit on the bar's own rounded end.
  const inside = positions.filter((p) => p > EDGE && p < 1 - EDGE);
  if (inside.length === 0) return null;
  return (
    <div aria-hidden="true" className="pointer-events-none absolute inset-0">
      {inside.map((position, i) => (
        <span
          className="absolute top-1/2 h-3 w-0.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-muted-foreground/60"
          data-testid="chapter-marker"
          key={i}
          style={{ left: `${position * 100}%` }}
        />
      ))}
    </div>
  );
}
