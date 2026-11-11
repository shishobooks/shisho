import { useMemo, useState, type PointerEvent } from "react";

import type { GrowthMonth } from "@/types/generated/serverstats";
import { formatCount } from "@/utils/format";
import { growthSeries, niceTicks } from "@/utils/growthSeries";

// Axis values in full below 10,000 and compact from there (10K, 20K), one
// style per axis.
const axisFormatter = (top: number) =>
  top >= 10_000
    ? new Intl.NumberFormat(undefined, { notation: "compact" })
    : new Intl.NumberFormat();

const unitHeading = { month: "Month", quarter: "Quarter", year: "Year" };

/**
 * Library Growth: the running total of Books, as an area under a line, with
 * a crosshair and tooltip on hover. The figures are also in a table for
 * screen readers, so the plot itself is hidden from them.
 */
const LibraryGrowthChart = ({ growth }: { growth: GrowthMonth[] }) => {
  const { unit, points } = useMemo(() => growthSeries(growth), [growth]);
  const [active, setActive] = useState<number | null>(null);

  if (points.length === 0) return null;

  const last = points.length - 1;
  const ticks = niceTicks(points[last].total);
  const top = ticks[ticks.length - 1];
  const format = axisFormatter(top);
  // Percent positions inside the plot, origin at the bottom left.
  const x = (i: number) => (i / last) * 100;
  const y = (total: number) => (total / top) * 100;

  const line = points
    .map((p, i) => `${i === 0 ? "M" : "L"}${x(i)},${100 - y(p.total)}`)
    .join(" ");
  const area = `${line} L100,100 L0,100 Z`;

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width === 0) return;
    const fraction = (event.clientX - rect.left) / rect.width;
    setActive(Math.min(last, Math.max(0, Math.round(fraction * last))));
  };

  const shown = active ?? last;
  const point = points[shown];

  return (
    <>
      <div aria-hidden="true" className="flex gap-2">
        {/* Y-axis labels, aligned with the gridlines. */}
        <div className="relative w-10 shrink-0 h-48 text-xs text-muted-foreground tabular-nums">
          {ticks.map((t) => (
            <span
              className="absolute right-0 translate-y-1/2 leading-none"
              key={t}
              style={{ bottom: `${y(t)}%` }}
            >
              {format.format(t)}
            </span>
          ))}
        </div>
        <div className="relative flex-1 min-w-0">
          <div
            className="relative h-48 touch-none"
            onPointerLeave={() => setActive(null)}
            onPointerMove={onPointerMove}
          >
            {ticks.map((t) => (
              <div
                className="absolute inset-x-0 border-t border-border"
                key={t}
                style={{ bottom: `${y(t)}%` }}
              />
            ))}
            <svg
              className="absolute inset-0 h-full w-full overflow-visible"
              preserveAspectRatio="none"
              viewBox="0 0 100 100"
            >
              <path className="fill-primary/10" d={area} />
              <path
                className="fill-none stroke-primary"
                d={line}
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                vectorEffect="non-scaling-stroke"
              />
            </svg>
            {active !== null && (
              <div
                className="absolute inset-y-0 w-px bg-foreground/30"
                style={{ left: `${x(shown)}%` }}
              />
            )}
            {/* The point the tooltip reads: the hovered one, or the latest. */}
            <div
              className="absolute size-2.5 -translate-x-1/2 translate-y-1/2 rounded-full bg-primary ring-2 ring-background"
              style={{ left: `${x(shown)}%`, bottom: `${y(point.total)}%` }}
            />
            {active !== null && (
              <div
                className="absolute z-10 rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs shadow-md whitespace-nowrap"
                // Beside the crosshair, on the side with more room, and at
                // the opposite end of the plot from the point, so it never
                // covers what it describes.
                style={{
                  ...(x(shown) > 50
                    ? { right: `calc(${100 - x(shown)}% + 8px)` }
                    : { left: `calc(${x(shown)}% + 8px)` }),
                  ...(y(point.total) > 50 ? { bottom: 0 } : { top: 0 }),
                }}
              >
                <div className="text-sm font-semibold tabular-nums text-foreground">
                  {formatCount(point.total, "book")}
                </div>
                <div className="text-muted-foreground">{point.label}</div>
                <div className="text-muted-foreground tabular-nums">
                  +{point.added.toLocaleString()} added
                </div>
              </div>
            )}
          </div>
          <div className="mt-1 flex justify-between text-xs text-muted-foreground">
            <span>{points[0].label}</span>
            <span>{points[last].label}</span>
          </div>
        </div>
      </div>
      <p className="mt-2 text-xs text-muted-foreground">
        {formatCount(points[last].total, "book")} now, with{" "}
        {points[last].added.toLocaleString()} added in {points[last].label}.
      </p>
      <table className="sr-only">
        <caption>Library growth by {unit}</caption>
        <thead>
          <tr>
            <th scope="col">{unitHeading[unit]}</th>
            <th scope="col">Books added</th>
            <th scope="col">Total books</th>
          </tr>
        </thead>
        <tbody>
          {points.map((p) => (
            <tr key={p.label}>
              <th scope="row">{p.label}</th>
              <td>{p.added.toLocaleString()}</td>
              <td>{p.total.toLocaleString()}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
};

export default LibraryGrowthChart;
