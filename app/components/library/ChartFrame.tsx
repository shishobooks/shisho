import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "@/libraries/utils";

/**
 * The frame every app chart draws in: y-axis labels at its ticks, a
 * recessive gridline at each, the plot, and a row of x-axis labels under
 * it. `ticks` run from 0 to the top of the scale (see `niceTicks`), and a
 * value's height in the plot is `yPercent(value)`. The frame is for sighted
 * readers only; the chart beside it carries its figures in a table for
 * screen readers.
 */
export const ChartFrame = ({
  ticks,
  formatTick,
  xLabels,
  plotProps,
  children,
}: {
  ticks: number[];
  formatTick: (value: number) => string;
  xLabels?: ReactNode;
  /** Pointer handlers and the like for the plot area. */
  plotProps?: HTMLAttributes<HTMLDivElement>;
  children: ReactNode;
}) => {
  const top = ticks[ticks.length - 1] || 1;
  return (
    <div aria-hidden="true" className="flex gap-2">
      {/* Y-axis labels, aligned with the gridlines. */}
      <div className="relative h-48 w-10 shrink-0 text-xs tabular-nums text-muted-foreground">
        {ticks.map((t) => (
          <span
            className="absolute right-0 translate-y-1/2 leading-none"
            key={t}
            style={{ bottom: `${(t / top) * 100}%` }}
          >
            {formatTick(t)}
          </span>
        ))}
      </div>
      <div className="relative min-w-0 flex-1">
        <div
          {...plotProps}
          className={cn("relative h-48 touch-none", plotProps?.className)}
        >
          {ticks.map((t) => (
            <div
              className="absolute inset-x-0 border-t border-border"
              key={t}
              style={{ bottom: `${(t / top) * 100}%` }}
            />
          ))}
          {children}
        </div>
        {xLabels}
      </div>
    </div>
  );
};

/**
 * The tooltip for the point or bar at `xPercent` across the plot. It sits
 * beside it on the side with more room, so it never covers what it
 * describes, at the top of the plot unless `atBottom`.
 */
export const ChartTooltip = ({
  xPercent,
  atBottom = false,
  offset = 8,
  children,
}: {
  xPercent: number;
  atBottom?: boolean;
  /** Pixels between the tooltip and the point or bar's edge. */
  offset?: number;
  children: ReactNode;
}) => (
  <div
    className="pointer-events-none absolute z-10 whitespace-nowrap rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs shadow-md"
    style={{
      ...(xPercent > 50
        ? { right: `calc(${100 - xPercent}% + ${offset}px)` }
        : { left: `calc(${xPercent}% + ${offset}px)` }),
      ...(atBottom ? { bottom: 0 } : { top: 0 }),
    }}
  >
    {children}
  </div>
);
